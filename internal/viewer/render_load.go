package viewer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/observer"
	"github.com/suchasplus/til-consensus/telemetry"
)

func InferRunFiles(resultPath string) RunFiles {
	runDir := filepath.Dir(resultPath)
	artifactsDir := filepath.Join(runDir, "artifacts")
	return RunFiles{
		RunDir:                runDir,
		ArtifactsDir:          artifactsDir,
		ResultPath:            resultPath,
		LedgerPath:            filepath.Join(runDir, "ledger.jsonl"),
		SummaryPath:           filepath.Join(runDir, "summary.md"),
		ManifestPath:          filepath.Join(artifactsDir, "manifest.jsonl"),
		EventsPath:            filepath.Join(runDir, "events.jsonl"),
		ComplianceSummaryPath: filepath.Join(artifactsDir, "strict-compliance-summary.json"),
		ProviderReadinessPath: filepath.Join(artifactsDir, "provider-readiness.json"),
		RunTelemetryPath:      filepath.Join(artifactsDir, "run-telemetry.json"),
	}
}

func LoadBundle(files RunFiles) (Bundle, error) {
	if strings.TrimSpace(files.ArtifactsDir) == "" {
		if strings.TrimSpace(files.ManifestPath) != "" {
			files.ArtifactsDir = filepath.Dir(files.ManifestPath)
		} else if strings.TrimSpace(files.RunDir) != "" {
			files.ArtifactsDir = filepath.Join(files.RunDir, "artifacts")
		}
	}
	if strings.TrimSpace(files.ComplianceSummaryPath) == "" && strings.TrimSpace(files.ArtifactsDir) != "" {
		files.ComplianceSummaryPath = filepath.Join(files.ArtifactsDir, "strict-compliance-summary.json")
	}
	if strings.TrimSpace(files.ProviderReadinessPath) == "" && strings.TrimSpace(files.ArtifactsDir) != "" {
		files.ProviderReadinessPath = filepath.Join(files.ArtifactsDir, "provider-readiness.json")
	}
	if strings.TrimSpace(files.RunTelemetryPath) == "" && strings.TrimSpace(files.ArtifactsDir) != "" {
		files.RunTelemetryPath = filepath.Join(files.ArtifactsDir, "run-telemetry.json")
	}
	body, err := os.ReadFile(files.ResultPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read result file: %w", err)
	}
	result, err := consensus.DecodeRunResult(body)
	if err != nil {
		return Bundle{}, fmt.Errorf("decode result file: %w", err)
	}
	ledger, _, err := observer.ReadVerifiedLedger(context.Background(), files.LedgerPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("verify ledger integrity: %w", err)
	}
	manifest, missing, err := readOptionalJSONL[consensus.ArtifactManifestEntry](files.ManifestPath, "artifacts/manifest.jsonl")
	if err != nil {
		return Bundle{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	events, err := readOptionalJSONLNoMissing[consensus.RunEventRecord](files.EventsPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read events file: %w", err)
	}
	complianceSummary, err := readOptionalJSONFile[ComplianceSummaryFile](files.ComplianceSummaryPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read strict compliance summary: %w", err)
	}
	complianceReports, err := readOptionalComplianceReports(files.ArtifactsDir)
	if err != nil {
		return Bundle{}, fmt.Errorf("read compliance reports: %w", err)
	}
	providerReadiness, err := readOptionalJSONFile[telemetry.ProviderReadinessFile](files.ProviderReadinessPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read provider readiness: %w", err)
	}
	runTelemetry, err := readOptionalJSONFile[telemetry.RunTelemetryFile](files.RunTelemetryPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read run telemetry: %w", err)
	}
	return Bundle{
		Result:            result,
		Ledger:            ledger,
		Manifest:          manifest,
		Events:            events,
		ComplianceSummary: complianceSummary,
		ComplianceReports: complianceReports,
		ProviderReadiness: providerReadiness,
		RunTelemetry:      runTelemetry,
		Files:             files,
		Missing:           missing,
	}, nil
}

func readOptionalJSONFile[T any](path string) (T, error) {
	var zero T
	if strings.TrimSpace(path) == "" {
		return zero, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, nil
		}
		return zero, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return zero, nil
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, err
	}
	return out, nil
}

func readOptionalComplianceReports(artifactsDir string) ([]ComplianceReportFile, error) {
	if strings.TrimSpace(artifactsDir) == "" {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(artifactsDir, "compliance-report-*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	slices.Sort(paths)
	out := make([]ComplianceReportFile, 0, len(paths))
	for _, path := range paths {
		item, err := readOptionalJSONFile[ComplianceReportFile](path)
		if err != nil {
			return nil, err
		}
		if item.Task.TaskID == "" && item.Agent.ID == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func cloneMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneGenericMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func mapValue(values map[string]any, key string) any {
	if len(values) == 0 {
		return nil
	}
	return values[key]
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	out := make([]T, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func readOptionalJSONL[T any](path string, missingName string) ([]T, []string, error) {
	items, err := readJSONL[T](path)
	if err == nil {
		return items, nil, nil
	}
	if os.IsNotExist(err) {
		return nil, []string{missingName}, nil
	}
	return nil, nil, err
}

func readOptionalJSONLNoMissing[T any](path string) ([]T, error) {
	items, err := readJSONL[T](path)
	if err == nil {
		return items, nil
	}
	if os.IsNotExist(err) {
		return nil, nil
	}
	return nil, err
}
