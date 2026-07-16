package cortex

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func scopedRequestKey(actorID, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(actorID + "\x00" + idempotencyKey))
	return hex.EncodeToString(sum[:])
}
