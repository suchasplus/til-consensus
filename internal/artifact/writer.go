package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func WriteRunArtifacts(result *consensus.RunResult, resultPath, summaryPath string) error {
	if err := writeJSON(resultPath, result); err != nil {
		return err
	}
	if err := writeText(summaryPath, BuildSummary(result)); err != nil {
		return err
	}
	return nil
}

func WriteErrorArtifact(requestID, errorPath string, cause error) error {
	payload := map[string]any{
		"requestId": requestID,
		"error":     cause.Error(),
		"at":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	return writeJSON(errorPath, payload)
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json artifact: %w", err)
	}
	return writeText(path, string(body)+"\n")
}

func writeText(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	return nil
}
