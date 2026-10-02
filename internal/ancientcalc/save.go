package ancientcalc

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	// MaxSaveInput bounds exported save files before decoding.
	MaxSaveInput         = 4 * 1024 * 1024
	maxAncientSaveOutput = 16 * 1024 * 1024
)

type ancientSave struct {
	HeroSouls                  json.Number          `json:"heroSouls"`
	HeroSoulsSacrificed        json.Number          `json:"heroSoulsSacrificed"`
	HighestFinishedZonePersist json.Number          `json:"highestFinishedZonePersist"`
	AncientSoulsTotal          json.Number          `json:"ancientSoulsTotal"`
	NumWorldResets             json.Number          `json:"numWorldResets"`
	Transcendent               *bool                `json:"transcendent"`
	Ancients                   ancientSaveAncients  `json:"ancients"`
	Outsiders                  ancientSaveOutsiders `json:"outsiders"`
	Items                      json.RawMessage      `json:"items"`
	AncientSouls               json.Number          `json:"ancientSouls"`
	PrimalSouls                json.Number          `json:"primalSouls"`
	TotalHeroLevels            json.Number          `json:"totalHeroLevels"`
	NumberOfTranscensions      json.Number          `json:"numberOfTranscensions"`
	AscensionsThisTranscension json.Number          `json:"numAscensionsThisTranscension"`
	Version                    json.Number          `json:"version"`
	Build                      string               `json:"readPatchNumber"`
	Stats                      prestigeStats        `json:"stats"`
}

type ancientSaveAncients struct {
	Ancients map[string]ancientSaveEntry `json:"ancients"`
}

type ancientSaveOutsiders struct {
	Outsiders map[string]ancientSaveEntry `json:"outsiders"`
}

type ancientSaveEntry struct {
	Level             json.Number `json:"level"`
	SpentHeroSouls    json.Number `json:"spentHeroSouls"`
	SpentAncientSouls json.Number `json:"spentAncientSouls"`
}

type saveContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r saveContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}
	return r.r.Read(p)
}

func inflateAncientSave(ctx context.Context, input []byte, raw bool) ([]byte, error) {
	var reader io.ReadCloser
	var err error
	source := saveContextReader{ctx: ctx, r: bytes.NewReader(input)}
	if raw {
		reader = flate.NewReader(source)
	} else {
		reader, err = zlib.NewReader(source)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, errors.New("invalid compressed save")
		}
	}
	defer reader.Close()
	out := make([]byte, 0, len(input)*2)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			if len(out)+n > maxAncientSaveOutput {
				return nil, errors.New("decoded save is too large")
			}
			out = append(out, buf[:n]...)
		}
		if readErr == io.EOF {
			return out, nil
		}
		if readErr != nil {
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return nil, readErr
			}
			return nil, errors.New("invalid compressed save")
		}
	}
}

func decodeSavePayload(ctx context.Context, exported []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(exported) == 0 || len(exported) > MaxSaveInput {
		return nil, errors.New("save must be non-empty and at most 4 MiB")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	text := strings.TrimSpace(string(exported))
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if len(text) < 32 {
		return nil, errors.New("invalid save")
	}
	header := text[:32]
	var payload []byte
	switch header {
	case "7a990d405d2c6fb93aa8fbb0ec1a3b23", "7e8bb5a89f2842ac4af01b3b7e228592":
		encoded := text[32:]
		if !validBase64(encoded) {
			return nil, errors.New("invalid save base64")
		}
		var decodeErr error
		payload, decodeErr = decodeBase64(encoded)
		if decodeErr != nil || len(payload) == 0 {
			return nil, errors.New("invalid save payload")
		}
		var err error
		payload, err = inflateAncientSave(ctx, payload, header[0] == '7' && header[1] == 'e')
		if err != nil {
			return nil, err
		}
	default:
		encoded := text
		if strings.Contains(encoded, "ClickerHeroesAccountSO") {
			if len(encoded) <= 54 {
				return nil, errors.New("invalid legacy save")
			}
			encoded = encoded[53 : len(encoded)-1]
		} else if marker := strings.Index(encoded, "Fe12NAfA3R6z4k0z"); marker >= 0 {
			prefix, checksum := encoded[:marker], encoded[marker+len("Fe12NAfA3R6z4k0z"):]
			if len(prefix)%2 != 0 || len(checksum) != 32 {
				return nil, errors.New("invalid legacy save checksum")
			}
			clean := make([]byte, len(prefix)/2)
			for i := range clean {
				clean[i] = prefix[i*2]
			}
			h := md5.Sum(append(clean, []byte("af0ik392jrmt0nsfdghy0")...))
			if hex.EncodeToString(h[:]) != checksum {
				return nil, errors.New("invalid legacy save checksum")
			}
			encoded = string(clean)
		}
		if !validBase64(encoded) {
			return nil, errors.New("invalid legacy save")
		}
		var decodeErr error
		payload, decodeErr = decodeBase64(encoded)
		if decodeErr != nil {
			return nil, errors.New("invalid legacy save")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return payload, nil
}

func decodeAncientSave(ctx context.Context, exported []byte) (ancientSave, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var save ancientSave
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return save, err
	}
	if err := json.Unmarshal(payload, &save); err != nil {
		return save, errors.New("save payload is not valid JSON")
	}
	for _, number := range []json.Number{save.HeroSouls, save.HeroSoulsSacrificed, save.HighestFinishedZonePersist, save.AncientSoulsTotal, save.NumWorldResets} {
		if number == "" {
			return save, errors.New("save is missing required data")
		}
	}
	if save.Ancients.Ancients == nil || save.Outsiders.Outsiders == nil {
		return save, errors.New("save is missing ancient data")
	}
	for _, entries := range []map[string]ancientSaveEntry{save.Ancients.Ancients, save.Outsiders.Outsiders} {
		for _, entry := range entries {
			if entry.Level == "" {
				return save, errors.New("save has malformed ancient data")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return save, err
	}
	return save, nil
}

func validBase64(s string) bool {
	if len(s)%4 == 1 || strings.IndexAny(s, "\r\n \t") >= 0 {
		return false
	}
	for i, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' {
			continue
		}
		if c == '=' && i >= len(s)-2 {
			continue
		}
		return false
	}
	return true
}

func decodeBase64(s string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "="))
}
