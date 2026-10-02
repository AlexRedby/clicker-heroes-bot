package ancientcalc

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

const testSaveJSON = `{"heroSouls":"123.5","heroSoulsSacrificed":4,"highestFinishedZonePersist":"42","ancientSoulsTotal":7,"numWorldResets":"3","transcendent":true,"ancients":{"ancients":{"1":{"level":"2","spentHeroSouls":"9"}}},"outsiders":{"outsiders":{"1":{"level":0}}}}`

func encodeModernSave(t *testing.T, raw bool) []byte {
	t.Helper()
	var b bytes.Buffer
	var w io.WriteCloser
	if raw {
		w, _ = flate.NewWriter(&b, flate.DefaultCompression)
	} else {
		w = zlib.NewWriter(&b)
	}
	if _, err := w.Write([]byte(testSaveJSON)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	header := "7a990d405d2c6fb93aa8fbb0ec1a3b23"
	if raw {
		header = "7e8bb5a89f2842ac4af01b3b7e228592"
	}
	return []byte(header + base64.StdEncoding.EncodeToString(b.Bytes()))
}

func TestDecodeAncientSaveModernFormats(t *testing.T) {
	for _, raw := range []bool{false, true} {
		save, err := decodeAncientSave(context.Background(), encodeModernSave(t, raw))
		if err != nil {
			t.Fatal(err)
		}
		if save.HeroSouls != "123.5" || save.Ancients.Ancients["1"].Level != "2" || save.Ancients.Ancients["1"].SpentHeroSouls != "9" {
			t.Fatalf("decoded save: %+v", save)
		}
	}
}

func TestDecodeAncientSaveLegacySprinkle(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(testSaveJSON))
	var noisy bytes.Buffer
	for _, c := range encoded {
		noisy.WriteByte(byte(c))
		noisy.WriteByte('x')
	}
	clean := []byte(encoded)
	h := md5.Sum(append(clean, []byte("af0ik392jrmt0nsfdghy0")...))
	exported := append(noisy.Bytes(), []byte("Fe12NAfA3R6z4k0z"+hex.EncodeToString(h[:]))...)
	if _, err := decodeAncientSave(context.Background(), exported); err != nil {
		t.Fatal(err)
	}
	exported[len(exported)-1] ^= 1
	if _, err := decodeAncientSave(context.Background(), exported); err == nil {
		t.Fatal("bad legacy checksum accepted")
	}
}

func TestDecodeAncientSaveRejectsMalformedAndCancelled(t *testing.T) {
	cases := [][]byte{
		[]byte("{}"),
		[]byte("7a990d405d2c6fb93aa8fbb0ec1a3b23!!!!"),
		[]byte(`{"heroSouls":1,"heroSoulsSacrificed":1,"highestFinishedZonePersist":1,"ancientSoulsTotal":1,"ancients":{"ancients":null},"outsiders":{"outsiders":{}}}`),
	}
	for _, input := range cases {
		if _, err := decodeAncientSave(context.Background(), input); err == nil {
			t.Fatalf("malformed save accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := decodeAncientSave(ctx, encodeModernSave(t, false))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
