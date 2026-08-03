package consensus

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
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

// ValidateRequestID keeps custom request IDs safe for use as a single output
// path segment. Backslashes are rejected as well so IDs remain safe when a
// run configuration is shared with Windows.
func ValidateRequestID(requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if requestID == "." || strings.Contains(requestID, "..") {
		return fmt.Errorf("request_id must not contain ..")
	}
	if strings.ContainsAny(requestID, `/\\`) {
		return fmt.Errorf("request_id must not contain path separators")
	}
	if strings.ContainsRune(requestID, '\x00') {
		return fmt.Errorf("request_id must not contain NUL")
	}
	return nil
}
