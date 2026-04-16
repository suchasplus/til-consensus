package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLatestRun(t *testing.T) {
	tmp := t.TempDir()
	for _, requestID := range []string{"tc_1710000000000_aaaaaa", "tc_1710000000001_bbbbbb"} {
		dir := filepath.Join(tmp, requestID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write result: %v", err)
		}
	}
	latest, err := ResolveLatestRun(filepath.Join(tmp, "{requestId}", "result.json"))
	if err != nil {
		t.Fatalf("ResolveLatestRun failed: %v", err)
	}
	if latest == nil || latest.RequestID != "tc_1710000000001_bbbbbb" {
		t.Fatalf("unexpected latest: %#v", latest)
	}
}

func TestResolveLatestRunEmpty(t *testing.T) {
	latest, err := ResolveLatestRun(filepath.Join(t.TempDir(), "{requestId}", "result.json"))
	if err != nil {
		t.Fatalf("ResolveLatestRun failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil latest run, got %#v", latest)
	}
}
