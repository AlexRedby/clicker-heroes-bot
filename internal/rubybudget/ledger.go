// Package rubybudget implements a small, durable, fail-closed ruby budget.
package rubybudget

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrLocked                = errors.New("ruby budget is locked by another process")
	ErrPending               = errors.New("ruby spending has an unresolved pending reservation")
	ErrNoFunds               = errors.New("ruby budget has insufficient spendable rubies")
	ErrProtectedBalance      = errors.New("protected ruby balance would be breached")
	ErrCap                   = errors.New("per-Ascension ruby cap exceeded")
	ErrUnknownReservation    = errors.New("unknown ruby reservation")
	ErrClosed                = errors.New("ruby budget is closed")
	ErrPoisoned              = errors.New("ruby budget is poisoned after a persistence failure")
	ErrUnsupportedDurability = errors.New("ruby budget durability is unsupported on this platform")
)

type Config struct {
	Path             string
	ProfileID        string
	InitialAllowance uint64
	ProtectedBalance uint64
	PerAscensionCap  uint64
}

type Reservation struct {
	ID        string `json:"id"`
	Amount    uint64 `json:"amount"`
	Ascension string `json:"ascension"`
}

type Snapshot struct {
	ProfileID        string
	InitialAllowance uint64
	ProtectedBalance uint64
	PerAscensionCap  uint64
	Spent            uint64
	ObservedBalance  uint64
	Observed         bool
	Pending          *Reservation
	SpentByAscension map[string]uint64
}

type diskState struct {
	Version          uint32            `json:"version"`
	ProfileID        string            `json:"profile_id"`
	InitialAllowance uint64            `json:"initial_allowance"`
	ProtectedBalance uint64            `json:"protected_balance"`
	PerAscensionCap  uint64            `json:"per_ascension_cap"`
	Spent            uint64            `json:"spent"`
	ObservedBalance  uint64            `json:"observed_balance"`
	Observed         bool              `json:"observed"`
	Pending          *Reservation      `json:"pending,omitempty"`
	SpentByAscension map[string]uint64 `json:"spent_by_ascension"`
}

type Ledger struct {
	cfg      Config
	lock     *os.File
	st       diskState
	mu       sync.Mutex
	dead     bool
	poisoned bool
}

var syncFile = func(f *os.File) error { return f.Sync() }
var renameFile = os.Rename

func Open(cfg Config) (*Ledger, error) {
	if cfg.Path == "" || cfg.ProfileID == "" || filepath.Base(cfg.Path) == "." {
		return nil, errors.New("invalid ruby budget path or profile ID")
	}
	if cfg.PerAscensionCap == 0 {
		return nil, errors.New("per-Ascension cap must be positive")
	}
	if cfg.InitialAllowance > 0 && !durabilityAvailable() {
		return nil, ErrUnsupportedDurability
	}
	if cfg.ProtectedBalance > ^uint64(0)-cfg.InitialAllowance {
		return nil, errors.New("ruby budget limits overflow")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0700); err != nil {
		return nil, err
	}
	lockPath := cfg.Path + ".lock"
	if info, statErr := os.Stat(lockPath); statErr == nil && checkStateFileMode(info) != nil {
		return nil, errors.New("ruby budget lock must have mode 0600")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lf.Chmod(0600); err != nil {
		lf.Close()
		return nil, err
	}
	if err = lockFile(lf); err != nil {
		lf.Close()
		return nil, err
	}

	l := &Ledger{cfg: cfg, lock: lf}
	b, readErr := os.ReadFile(cfg.Path)
	if os.IsNotExist(readErr) {
		l.st = diskState{Version: 1, ProfileID: cfg.ProfileID, InitialAllowance: cfg.InitialAllowance, ProtectedBalance: cfg.ProtectedBalance, PerAscensionCap: cfg.PerAscensionCap, SpentByAscension: map[string]uint64{}}
		if err = l.persist(); err != nil {
			l.Close()
			return nil, err
		}
		return l, nil
	}
	if readErr != nil {
		l.Close()
		return nil, readErr
	}
	if info, statErr := os.Stat(cfg.Path); statErr != nil || checkStateFileMode(info) != nil {
		l.Close()
		return nil, errors.New("ruby budget file must have mode 0600")
	}
	if err = decodeStrict(b, &l.st); err != nil {
		l.Close()
		return nil, fmt.Errorf("corrupt ruby budget: %w", err)
	}
	if err = validateState(l.st, cfg); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}

func validateState(s diskState, c Config) error {
	if s.Version != 1 || s.ProfileID != c.ProfileID || s.InitialAllowance != c.InitialAllowance || s.ProtectedBalance != c.ProtectedBalance || s.PerAscensionCap != c.PerAscensionCap {
		return errors.New("ruby budget identity or configuration mismatch")
	}
	if s.Spent > s.InitialAllowance || s.SpentByAscension == nil {
		return errors.New("invalid ruby budget state")
	}
	var sum uint64
	for a, n := range s.SpentByAscension {
		if a == "" || n > c.PerAscensionCap {
			return errors.New("invalid ruby Ascension accounting")
		}
		if sum > ^uint64(0)-n {
			return errors.New("ruby accounting overflow")
		}
		sum += n
	}
	if sum != s.Spent {
		return errors.New("ruby accounting total mismatch")
	}
	if s.Pending != nil {
		if s.Pending.ID == "" || s.Pending.Amount == 0 || s.Pending.Ascension == "" || s.Pending.Amount > c.PerAscensionCap {
			return errors.New("invalid pending reservation")
		}
		if s.Pending.Amount > s.InitialAllowance-s.Spent {
			return errors.New("pending reservation exceeds allowance")
		}
		used := s.SpentByAscension[s.Pending.Ascension]
		if used > c.PerAscensionCap || s.Pending.Amount > c.PerAscensionCap-used {
			return errors.New("pending reservation exceeds Ascension cap")
		}
	}
	return nil
}

func (l *Ledger) ensure() error {
	if l == nil || l.dead {
		return ErrClosed
	}
	if l.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (l *Ledger) save() error {
	if err := l.persist(); err != nil {
		l.poisoned = true
		return err
	}
	return nil
}

func (l *Ledger) Snapshot() (Snapshot, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensure(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{ProfileID: l.st.ProfileID, InitialAllowance: l.st.InitialAllowance, ProtectedBalance: l.st.ProtectedBalance, PerAscensionCap: l.st.PerAscensionCap, Spent: l.st.Spent, ObservedBalance: l.st.ObservedBalance, Observed: l.st.Observed, SpentByAscension: map[string]uint64{}}
	for k, v := range l.st.SpentByAscension {
		s.SpentByAscension[k] = v
	}
	if l.st.Pending != nil {
		p := *l.st.Pending
		s.Pending = &p
	}
	return s, nil
}

func (l *Ledger) Observe(balance uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensure(); err != nil {
		return err
	}
	l.st.ObservedBalance = balance
	l.st.Observed = true
	return l.save()
}

func (l *Ledger) Reserve(amount uint64, ascension string, balance uint64) (Reservation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensure(); err != nil {
		return Reservation{}, err
	}
	if amount == 0 || ascension == "" {
		return Reservation{}, errors.New("invalid reservation")
	}
	if l.st.Pending != nil {
		return Reservation{}, ErrPending
	}
	l.st.ObservedBalance = balance
	l.st.Observed = true
	if amount > l.st.InitialAllowance-l.st.Spent {
		if err := l.save(); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, ErrNoFunds
	}
	used := l.st.SpentByAscension[ascension]
	if used > l.cfg.PerAscensionCap || amount > l.cfg.PerAscensionCap-used {
		if err := l.save(); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, ErrCap
	}
	if balance < l.cfg.ProtectedBalance {
		if err := l.save(); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, ErrProtectedBalance
	}
	spendable := balance - l.cfg.ProtectedBalance
	if amount > spendable {
		if err := l.save(); err != nil {
			return Reservation{}, err
		}
		return Reservation{}, ErrNoFunds
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Reservation{}, err
	}
	r := Reservation{ID: hex.EncodeToString(id), Amount: amount, Ascension: ascension}
	l.st.Pending = &r
	if err := l.save(); err != nil {
		return Reservation{}, err
	}
	return r, nil
}

func (l *Ledger) ConfirmSpent(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensure(); err != nil {
		return err
	}
	if l.st.Pending == nil || l.st.Pending.ID != id {
		return ErrUnknownReservation
	}
	p := *l.st.Pending
	if l.st.Spent > l.st.InitialAllowance-p.Amount {
		return errors.New("ruby accounting overflow")
	}
	l.st.Spent += p.Amount
	l.st.SpentByAscension[p.Ascension] += p.Amount
	l.st.Pending = nil
	return l.save()
}

// ProveCanceled is an explicit reconciliation operation, never an automatic
// timeout, restart, unchanged-wallet or dialog-closure refund. The caller must
// have independent proof that payment never occurred. The Timelapse controller
// does not expose or call this method after submission.
func (l *Ledger) ProveCanceled(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensure(); err != nil {
		return err
	}
	if l.st.Pending == nil || l.st.Pending.ID != id {
		return ErrUnknownReservation
	}
	l.st.Pending = nil
	return l.save()
}

func (l *Ledger) persist() error {
	if err := validateState(l.st, l.cfg); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l.st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(l.cfg.Path)
	f, err := os.CreateTemp(dir, ".rubybudget-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = syncFile(f)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = renameFile(name, l.cfg.Path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func (l *Ledger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dead {
		return nil
	}
	l.dead = true
	uerr := unlockFile(l.lock)
	cerr := l.lock.Close()
	if uerr != nil {
		return uerr
	}
	return cerr
}
