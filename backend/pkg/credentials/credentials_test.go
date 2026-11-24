package credentials

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func newTestKey(t *testing.T) string {
	t.Helper()

	raw := make([]byte, KeySize)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("draw a key: %v", err)
	}

	return hex.EncodeToString(raw)
}

func TestCredentials_Sealer_OpensWhatItSealed(t *testing.T) {
	sealer, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	secret := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nnot a real one\n")

	sealed, err := sealer.Seal(secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, secret) {
		t.Fatal("the sealed value carries the plaintext")
	}

	opened, err := sealer.Open(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(opened, secret) {
		t.Fatalf("opened = %q, want %q", opened, secret)
	}
}

func TestCredentials_Sealer_SealsTheSameSecretDifferentlyEachTime(t *testing.T) {
	sealer, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	first, err := sealer.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	second, err := sealer.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if bytes.Equal(first, second) {
		t.Fatal("two seals of one secret are identical, so the nonce is reused")
	}
}

func TestCredentials_Sealer_RefusesASealedValueFromAnotherKey(t *testing.T) {
	one, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}
	other, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	sealed, err := one.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if _, err := other.Open(sealed); !errors.Is(err, ErrSealed) {
		t.Fatalf("open with the wrong key = %v, want ErrSealed", err)
	}
}

func TestCredentials_Sealer_RefusesATamperedBlob(t *testing.T) {
	sealer, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	sealed, err := sealer.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	sealed[len(sealed)-1] ^= 0xff

	if _, err := sealer.Open(sealed); !errors.Is(err, ErrSealed) {
		t.Fatalf("open a tampered blob = %v, want ErrSealed", err)
	}
}

func TestCredentials_Sealer_RefusesATruncatedBlob(t *testing.T) {
	sealer, err := NewSealer(newTestKey(t))
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	if _, err := sealer.Open([]byte("short")); !errors.Is(err, ErrSealed) {
		t.Fatalf("open a short blob = %v, want ErrSealed", err)
	}
}

func TestCredentials_Sealer_WithoutAKeySealsAndOpensNothing(t *testing.T) {
	var sealer *Sealer

	if _, err := sealer.Seal([]byte("hunter2")); !errors.Is(err, ErrNoKey) {
		t.Fatalf("seal without a sealer = %v, want ErrNoKey", err)
	}
	if _, err := sealer.Open([]byte("whatever")); !errors.Is(err, ErrNoKey) {
		t.Fatalf("open without a sealer = %v, want ErrNoKey", err)
	}
}

func TestCredentials_ParseKey_AcceptsTheFormsAnOperatorWrites(t *testing.T) {
	raw := make([]byte, KeySize)
	for i := range raw {
		raw[i] = byte(i)
	}

	tests := []struct {
		name string
		key  string
	}{
		{name: "hex", key: hex.EncodeToString(raw)},
		{name: "hex with whitespace", key: " " + hex.EncodeToString(raw) + "\n"},
		{name: "base64 standard", key: base64.StdEncoding.EncodeToString(raw)},
		{name: "base64 unpadded", key: base64.RawStdEncoding.EncodeToString(raw)},
		{name: "base64 url", key: base64.URLEncoding.EncodeToString(raw)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseKey(tc.key)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("key = %x, want %x", got, raw)
			}
		})
	}
}

func TestCredentials_ParseKey_RefusesWhatCannotBeAKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want error
	}{
		{name: "empty", key: "", want: ErrNoKey},
		{name: "only whitespace", key: "  \n ", want: ErrNoKey},
		{name: "too short", key: strings.Repeat("ab", 16), want: ErrMalformedKey},
		{name: "not encoded", key: strings.Repeat("z", 64), want: ErrMalformedKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseKey(tc.key); !errors.Is(err, tc.want) {
				t.Fatalf("parse %q = %v, want %v", tc.key, err, tc.want)
			}
		})
	}
}
