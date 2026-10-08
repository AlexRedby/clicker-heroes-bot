package transcension

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

var (
	ErrJournalLocked   = errors.New("Transcension journal is locked by another process")
	ErrJournalClosed   = errors.New("Transcension journal is closed")
	ErrJournalPoisoned = errors.New("Transcension journal requires reopening after a persistence failure")
)

const maxJournalBytes = 128 << 10

type journalState struct {
	ProfileID    string
	Stage        Stage
	Snapshot     Snapshot
	Targets      []ancientcalc.OutsiderTarget
	Pending      Command
	InputAt      time.Time
	Reward       int
	EarningAfter int
	EarningSouls string
}

type journalEnvelope struct {
	Version int
	Payload json.RawMessage
	Digest  string
}

// Journal holds one exclusive process lock for the lifetime of the controller.
// It contains sanitized export facts, never raw saves or the raw game identity.
type Journal struct {
	mu                      sync.Mutex
	path, profile           string
	lock                    *os.File
	state                   journalState
	closed, poisoned, bound bool
}

var journalSyncFile = func(file *os.File) error { return file.Sync() }
var journalRename = replaceJournalFile
var journalSyncDirectory = syncJournalDirectory

func journalDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func checkJournalFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxJournalBytes {
		return errors.New("journal must be a bounded regular file")
	}
	return checkJournalMode(info)
}

func OpenJournal(path, profileID string) (*Journal, error) {
	if path == "" || filepath.Base(path) == "." || !journalDigest(profileID) {
		return nil, errors.New("journal path and hashed profile ID required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	if err := checkJournalFile(lockPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	j := &Journal{path: path, profile: profileID, lock: file, state: journalState{ProfileID: profileID}}
	fail := func(err error) (*Journal, error) { file.Close(); return nil, err }
	// Opening a raced symlink must not grant ownership of its target.
	opened, err := file.Stat()
	named, namedErr := os.Lstat(lockPath)
	if err != nil || namedErr != nil || !named.Mode().IsRegular() || !os.SameFile(opened, named) {
		return fail(errors.New("journal lock changed during opening"))
	}
	if err := protectJournalFile(file); err != nil {
		return fail(err)
	}
	if err := lockJournal(file); err != nil {
		return fail(err)
	}
	if err := checkJournalFile(path); os.IsNotExist(err) {
		if err := j.persist(j.state); err != nil {
			j.Close()
			return nil, err
		}
		return j, nil
	} else if err != nil {
		j.Close()
		return nil, err
	}
	reader, err := os.Open(path)
	var b []byte
	if err == nil {
		opened, statErr := reader.Stat()
		named, namedErr := os.Lstat(path)
		if statErr != nil || namedErr != nil || !named.Mode().IsRegular() || !os.SameFile(opened, named) || opened.Size() > maxJournalBytes {
			err = errors.New("journal changed during opening")
		} else if err = protectJournalFile(reader); err == nil {
			b, err = io.ReadAll(io.LimitReader(reader, maxJournalBytes+1))
		}
		if closeErr := reader.Close(); err == nil {
			err = closeErr
		}
	}
	if err == nil && len(b) > maxJournalBytes {
		err = errors.New("oversized Transcension journal")
	}
	var envelope journalEnvelope
	if err == nil {
		err = decodeJournalJSON(b, &envelope)
	}
	if err == nil {
		digest := sha256.Sum256(envelope.Payload)
		if envelope.Version != 1 || envelope.Digest != hex.EncodeToString(digest[:]) {
			err = errors.New("invalid Transcension journal version/checksum")
		}
	}
	if err == nil {
		err = decodeJournalJSON(envelope.Payload, &j.state)
	}
	if err == nil {
		err = validateJournalState(j.state, profileID)
	}
	if err != nil {
		j.Close()
		return nil, err
	}
	return j, nil
}

func decodeJournalJSON(b []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing Transcension journal data")
	}
	return nil
}

func validateJournalState(s journalState, profile string) error {
	if s.ProfileID != profile || !journalDigest(profile) || s.Stage > ReadyForAllocation {
		return errors.New("journal profile or stage mismatch")
	}
	if s.Stage == Ordinary {
		if s.Pending != (Command{}) || s.Reward != 0 || len(s.Targets) != 0 || s.Snapshot.State.SaveHash != "" {
			return errors.New("ordinary journal contains unfinished ownership")
		}
		return nil
	}
	if err := validateSnapshot(context.Background(), s.Snapshot); err != nil {
		return err
	}
	if s.Snapshot.State.ProfileID != profile || s.Snapshot.ExportedAt.IsZero() {
		return errors.New("journal source identity/time missing")
	}
	if s.EarningAfter < 0 || s.EarningAfter > s.Snapshot.State.AscensionsThisTranscension {
		return errors.New("invalid journal earning cycle")
	}
	if s.EarningSouls != "" {
		if _, err := ancientcalc.Value(s.EarningSouls); err != nil {
			return errors.New("invalid journal earning wallet")
		}
	}
	if s.Stage == AwaitResetReceipt || s.Stage == AwaitFeedReceipt {
		expected := ConfirmReset
		if s.Stage == AwaitFeedReceipt {
			expected = FeedOutsider
		}
		if s.Pending.Action != expected || s.Pending.SaveHash != s.Snapshot.State.SaveHash || s.Pending.Generation != s.Snapshot.Generation || s.Pending.Frame == 0 || s.InputAt.IsZero() || s.InputAt.Before(s.Snapshot.ExportedAt) {
			return errors.New("journal pending input ownership mismatch")
		}
		if expected == ConfirmReset {
			if s.Reward <= 0 || s.Pending.ID != 0 || s.Pending.Quantity != 0 || s.Pending.Cost != 0 {
				return errors.New("invalid pending reset")
			}
		} else {
			if s.Pending.Quantity != 1 && s.Pending.Quantity != 10 && s.Pending.Quantity != 100 && s.Pending.Quantity != 1000 {
				return errors.New("invalid pending FEED quantity")
			}
			found := false
			for _, row := range s.Snapshot.State.Outsiders {
				if row.ID == s.Pending.ID && row.Name == s.Pending.Name {
					level, _ := strconv.Atoi(row.Level)
					cost, err := ancientcalc.OutsiderFeedCost(row.ID, level, s.Pending.Quantity)
					found = err == nil && cost == s.Pending.Cost && cost > 0 && cost <= s.Snapshot.State.AncientSouls
				}
			}
			if !found {
				return errors.New("invalid pending FEED cost/row")
			}
		}
	} else if s.Pending != (Command{}) {
		return errors.New("journal has an unexpected pending input")
	}
	if s.Stage == SpendOutsiders || s.Stage == AwaitFeedReceipt || s.Stage == RestoreHeroes {
		if len(s.Targets) != 9 {
			return errors.New("journal is missing fixed Outsider targets")
		}
		seen, remaining := make(map[int]bool), 0
		for _, target := range s.Targets {
			found := false
			for _, row := range s.Snapshot.State.Outsiders {
				if row.ID != target.ID || row.Name != target.Name || seen[target.ID] {
					continue
				}
				level, _ := strconv.Atoi(row.Level)
				if target.Current < 0 || target.Current > level || target.Target < level {
					return errors.New("journal Outsider target regressed")
				}
				if target.Target > level {
					cost, err := ancientcalc.OutsiderFeedCost(row.ID, level, target.Target-level)
					if err != nil || cost > s.Snapshot.State.AncientSouls-remaining {
						return errors.New("journal remaining targets exceed the actual wallet")
					}
					remaining += cost
				}
				seen[target.ID], found = true, true
			}
			if !found {
				return errors.New("invalid journal Outsider target")
			}
		}
		if s.Stage == RestoreHeroes && remaining != 0 {
			return errors.New("hero restoration cannot skip unfinished Outsider targets")
		}
		if s.Stage == AwaitFeedReceipt {
			found := false
			for _, target := range s.Targets {
				if target.ID == s.Pending.ID {
					for _, row := range s.Snapshot.State.Outsiders {
						level, _ := strconv.Atoi(row.Level)
						if row.ID == target.ID && s.Pending.Quantity <= target.Target-level {
							found = true
						}
					}
				}
			}
			if !found {
				return errors.New("pending FEED exceeds its fixed target")
			}
		}
	}
	return nil
}

func (j *Journal) ensure() error {
	if j == nil || j.closed {
		return ErrJournalClosed
	}
	if j.poisoned {
		return ErrJournalPoisoned
	}
	return nil
}

func (j *Journal) persist(s journalState) error {
	if err := validateJournalState(s, j.profile); err != nil {
		return err
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	b, err := json.Marshal(journalEnvelope{Version: 1, Payload: payload, Digest: hex.EncodeToString(digest[:])})
	if err != nil || len(b) > maxJournalBytes {
		return errors.New("unsupported Transcension journal payload")
	}
	dir := filepath.Dir(j.path)
	file, err := os.CreateTemp(dir, ".transcension-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = protectJournalFile(file); err == nil {
		_, err = file.Write(b)
	}
	if err == nil {
		err = journalSyncFile(file)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := journalRename(file.Name(), j.path); err != nil {
		return err
	}
	return journalSyncDirectory(dir)
}

func (j *Journal) save(s journalState) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.ensure(); err != nil {
		return err
	}
	if err := j.persist(s); err != nil {
		j.poisoned = true
		return err
	}
	j.state = s
	return nil
}

func (j *Journal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	err := unlockJournal(j.lock)
	if closeErr := j.lock.Close(); err == nil {
		err = closeErr
	}
	return err
}
