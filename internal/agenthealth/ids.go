package agenthealth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

func randomID() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.New("random identifier unavailable")
	}
	return hex.EncodeToString(b[:]), nil
}

// NewInstanceID returns a fresh, valid process-instance identifier.
func NewInstanceID() (string, error) { return randomID() }
