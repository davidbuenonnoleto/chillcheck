package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Gateway keys are looked up by hash on every ingest request, so we use a fast
// sha256 (not bcrypt). The key has enough entropy that a hash lookup is safe.
// Format: "chk_gw_" + 32 random bytes hex. We return the plaintext once; the DB
// stores only the hash and a short prefix for display.

func GenerateGatewayKey() (key, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	key = "chk_gw_" + hex.EncodeToString(buf)
	hash = HashGatewayKey(key)
	prefix = key[:14] // "chk_gw_" + 7 hex chars
	return key, hash, prefix, nil
}

func HashGatewayKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
