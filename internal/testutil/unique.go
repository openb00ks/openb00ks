package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// UniqueSuffix returns a time-based suffix with extra entropy for test data.
func UniqueSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(b[:])
}
