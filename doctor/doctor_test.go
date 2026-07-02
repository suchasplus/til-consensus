package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/suchasplus/til-consensus/config"
)

func TestRunReportsHealthyMockConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.InitTemplate()
	cfg.Output.Directory = filepath.Join(tmp, "out", "{requestId}")
	path := filepath.Join(tmp, "til-consensus.yaml")
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}
	report := Run(context.Background(), Options{ConfigPath: path})
	if report.Summary.Fail != 0 {
		t.Fatalf("expected no failures: %#v", report)
	}
	if report.Summary.OK == 0 || report.Summary.Total == 0 {
		t.Fatalf("expected successful checks: %#v", report.Summary)
	}
}
