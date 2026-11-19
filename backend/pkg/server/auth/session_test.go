package auth

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Literals from an independent PBKDF2: a changed derivation signs out every issued token and cookie.
func TestSession_MakeJWTSigningKey_DerivesAStableKeyPerSaltApartFromTheCookieKeys(t *testing.T) {
	for range 2 {
		assert.Equal(t, "6b95a23ff5ad60cd4b380f8a75a5890de112064dbbd06e1a95edd70e7776ba58",
			hex.EncodeToString(MakeJWTSigningKey("kat_salt")), "derived first, then served from the cache")
	}

	other := MakeJWTSigningKey("another_salt")
	assert.Len(t, other, 32)
	assert.NotEqual(t, MakeJWTSigningKey("kat_salt"), other)

	cookieKeys := MakeCookieStoreKey("kat_salt")
	assert.NotEqual(t, MakeJWTSigningKey("kat_salt"), cookieKeys[0])
	assert.NotEqual(t, MakeJWTSigningKey("kat_salt"), cookieKeys[1])
}

func TestSession_MakeCookieStoreKey_DerivesAStableAuthAndEncryptionKey(t *testing.T) {
	for range 2 {
		keys := MakeCookieStoreKey("kat_salt")
		require.Len(t, keys, 2)
		require.Len(t, keys[0], 64)
		require.Len(t, keys[1], 32)
		assert.Equal(t, "e7886e3f2c63e6fead563a4b333f9f1f", hex.EncodeToString(keys[0][:16]))
		assert.Equal(t, "106bae2bffc8c17699da8c1b796744f7", hex.EncodeToString(keys[1][:16]))
	}
}
