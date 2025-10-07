package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Fingerprint returns a SHA-256 hex digest of arbitrary bytes.
func Fingerprint(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

// FingerprintJSON marshals v with encoding/json and hashes the bytes.
// Use with already-normalized/order-independent shapes (see specnorm).
func FingerprintJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return Fingerprint(b), nil
}

// MustFingerprintJSON is a helper that panics on marshal error.
func MustFingerprintJSON(v any) string {
	b, _ := json.Marshal(v)
	return Fingerprint(b)
}
