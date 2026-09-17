package sidecar

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// KeySize is the required HMAC key length in bytes (256-bit).
const KeySize = 32

// Sign returns the hex-encoded HMAC-SHA256 of req's canonical string under key.
func Sign(key []byte, req CanonicalRequest) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(req.canonicalString()))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks signature against req under key and enforces the replay window
// relative to now. It uses a constant-time comparison and never reveals whether
// the timestamp or the signature failed beyond the returned error text.
func Verify(key []byte, req CanonicalRequest, signature string, now time.Time) error {
	ts, err := strconv.ParseInt(req.Timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp %q: %w", req.Timestamp, err)
	}
	drift := now.Unix() - ts
	if drift < 0 {
		drift = -drift
	}
	if drift > MaxTimestampDriftSeconds {
		return fmt.Errorf("timestamp drift %ds exceeds limit %ds", drift, MaxTimestampDriftSeconds)
	}
	expected := Sign(key, req)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("HMAC signature mismatch")
	}
	return nil
}

// DecodeKey parses a hex-encoded key and checks its length. The sidecar key is
// distributed as a 64-char hex string (32 bytes) via env/file.
func DecodeKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("key is not valid hex: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes (%d hex chars), got %d", KeySize, KeySize*2, len(key))
	}
	return key, nil
}
