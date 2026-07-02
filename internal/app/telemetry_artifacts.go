package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/telemetry"
)

func writeRunTelemetryArtifact(result *consensus.RunResult, artifactsDir string) error {
	if result == nil || artifactsDir == "" {
		return nil
	}
	summaryPath := filepath.Join(artifactsDir, "strict-compliance-summary.json")
	summary, err := readOptionalComplianceSummary(summaryPath)
	if err != nil {
		return err
	}
	payload := telemetry.BuildRunTelemetry(*result, summary, artifactsDir, time.Now().UTC())
	return telemetry.WriteRunTelemetryFile(filepath.Join(artifactsDir, "run-telemetry.json"), payload)
}

func readOptionalComplianceSummary(path string) (telemetry.ComplianceSummaryFile, error) {
	var zero telemetry.ComplianceSummaryFile
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return zero, nil
		}
		return zero, err
	}
	return telemetry.ReadComplianceSummaryFile(path)
}
