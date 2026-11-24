// Package credentials seals the secrets the remote tools dial with.
//
// The master key never reaches the database: it is read once at start-up from
// the environment and the rows hold only what that key sealed. A deployment that
// loses the key loses the ability to open its stored credentials, which is the
// intended failure mode — a backup of the table alone is not a backup of the
// keys.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// KeySize is the length of an AES-256 key.
const KeySize = 32

// VarKey is the environment variable the master key is read from.
const VarKey = "OPSAGI_SECRET_KEY"

var (
	// ErrNoKey is returned when a sealed value is opened without a sealer.
	ErrNoKey = errors.New("no credential master key is configured")
	// ErrMalformedKey is returned for a key that is not 32 bytes once decoded.
	ErrMalformedKey = errors.New("the credential master key must decode to 32 bytes")
	// ErrSealed is returned when a sealed value cannot be authenticated.
	ErrSealed = errors.New("the sealed credential cannot be opened with this master key")
)

// Sealer turns a plaintext secret into the bytes stored in the credentials
// table and back. It is safe for concurrent use.
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer builds a sealer from the configured master key. The value may be
// 64 hex characters or the base64 form of the same 32 bytes; both are accepted
// because operators generate it with whichever tool they already have.
func NewSealer(key string) (*Sealer, error) {
	raw, err := ParseKey(key)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("build the cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build the GCM mode: %w", err)
	}

	return &Sealer{aead: aead}, nil
}

// ParseKey decodes a master key written as hex or base64.
func ParseKey(key string) ([]byte, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrNoKey
	}

	candidates := []func(string) ([]byte, error){
		hex.DecodeString,
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	for _, decode := range candidates {
		raw, err := decode(key)
		if err != nil {
			continue
		}
		if len(raw) == KeySize {
			return raw, nil
		}
	}

	return nil, ErrMalformedKey
}

// Seal returns nonce||ciphertext for the plaintext. Every call uses a fresh
// nonce, so the same secret sealed twice produces different bytes.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrNoKey
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("draw a nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal. A wrong key or a modified blob fails authentication
// rather than returning garbage.
func (s *Sealer) Open(sealed []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrNoKey
	}

	size := s.aead.NonceSize()
	if len(sealed) < size+s.aead.Overhead() {
		return nil, ErrSealed
	}

	plaintext, err := s.aead.Open(nil, sealed[:size], sealed[size:], nil)
	if err != nil {
		return nil, ErrSealed
	}

	return plaintext, nil
}
