package consensus

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

var RequestIDPattern = regexp.MustCompile(`^tc_\d+_[a-f0-9]{6}$`)

func NewRequestID(now time.Time) string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("tc_%d_000000", now.UTC().UnixMilli())
	}
	return fmt.Sprintf("tc_%d_%s", now.UTC().UnixMilli(), hex.EncodeToString(buf))
}
