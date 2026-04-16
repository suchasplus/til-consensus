package artifact

import (
	"testing"
	"time"
)

func TestNewRequestIDMatchesPattern(t *testing.T) {
	now := time.UnixMilli(1710000000000).UTC()
	value := NewRequestID(now)
	if !RequestIDPattern.MatchString(value) {
		t.Fatalf("request id does not match pattern: %s", value)
	}
}
