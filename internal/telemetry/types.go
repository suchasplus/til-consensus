package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

type ProviderReadinessFile struct {
	Version     int                      `json:"version"`
	GeneratedAt string                   `json:"generatedAt"`
	Providers   []ProviderReadinessEntry `json:"providers,omitempty"`
}

type ProviderReadinessEntry struct {
	Provider        string         `json:"provider"`
	ProviderType    string         `json:"providerType,omitempty"`
	Protocol        string         `json:"protocol,omitempty"`
	Model           string         `json:"model,omitempty"`
	BaseURL         string         `json:"baseUrl,omitempty"`
	APIKeyEnv       string         `json:"apiKeyEnv,omitempty"`
	Agent           string         `json:"agent,omitempty"`
	Command         []string       `json:"command,omitempty"`
	RequestContext  map[string]any `json:"requestContext,omitempty"`
	ResponseContext map[string]any `json:"responseContext,omitempty"`
	InputArtifact   string         `json:"inputArtifact,omitempty"`
	RawArtifact     string         `json:"rawArtifact,omitempty"`
	ErrorArtifact   string         `json:"errorArtifact,omitempty"`
	Ready           bool           `json:"ready"`
	StrictJSON      bool           `json:"strictJSON"`
	RecoverableJSON bool           `json:"recoverableJSON"`
	DurationMs      int64          `json:"durationMs"`
	StdoutPreview   string         `json:"stdoutPreview,omitempty"`
	StdoutFull      string         `json:"-"`
	StderrPreview   string         `json:"stderrPreview,omitempty"`
	Error           string         `json:"error,omitempty"`
}

type ComplianceSummaryFile struct {
	Version     int                      `json:"version"`
	GeneratedAt string                   `json:"generatedAt,omitempty"`
	Entries     []ComplianceSummaryEntry `json:"entries,omitempty"`
}

type ComplianceSummaryEntry struct {
	Provider      string             `json:"provider"`
	ProviderType  string             `json:"providerType"`
	ProviderModel string             `json:"providerModel"`
	TaskKind      consensus.TaskKind `json:"taskKind"`
	Total         int                `json:"total"`
	Strict        int                `json:"strict"`
	Normalized    int                `json:"normalized"`
	Repaired      int                `json:"repaired"`
	Failed        int                `json:"failed"`
}

type RunTelemetryFile struct {
	Version             int                       `json:"version"`
	GeneratedAt         string                    `json:"generatedAt"`
	RequestID           string                    `json:"requestId"`
	SessionID           string                    `json:"sessionId"`
	Mode                consensus.WorkflowMode    `json:"mode"`
	Providers           []string                  `json:"providers,omitempty"`
	TaskSummary         []RunTaskSummary          `json:"taskSummary,omitempty"`
	WorkflowSummary     WorkflowSummary           `json:"workflowSummary"`
	VerificationSummary VerificationSummary       `json:"verificationSummary,omitempty"`
	Result              RunTelemetryResult        `json:"result"`
	Timing              RunTelemetryTiming        `json:"timing"`
	SourceSummary       RunTelemetrySourceSummary `json:"sourceSummary"`
}

type RunTaskSummary struct {
	TaskKind   consensus.TaskKind `json:"taskKind"`
	Total      int                `json:"total"`
	Strict     int                `json:"strict"`
	Normalized int                `json:"normalized"`
	Repaired   int                `json:"repaired"`
	Failed     int                `json:"failed"`
}

type WorkflowSummary struct {
	Claims               int `json:"claims"`
	SupportedClaims      int `json:"supportedClaims"`
	RefutedClaims        int `json:"refutedClaims"`
	InsufficientClaims   int `json:"insufficientClaims"`
	UndeterminedClaims   int `json:"undeterminedClaims"`
	KeepClaims           int `json:"keepClaims"`
	KeepWithCaveatClaims int `json:"keepWithCaveatClaims"`
	UnresolvedClaims     int `json:"unresolvedClaims"`
	RejectClaims         int `json:"rejectClaims"`
	ChallengeCount       int `json:"challengeCount"`
	ObservationCount     int `json:"observationCount"`
	FreeDebateRoundCount int `json:"freeDebateRoundCount,omitempty"`
	FreeDebateClaimCount int `json:"freeDebateClaimCount,omitempty"`
	FreeDebateVoteCount  int `json:"freeDebateVoteCount,omitempty"`
	DelphiRoundCount     int `json:"delphiRoundCount,omitempty"`
	DelphiStatementCount int `json:"delphiStatementCount,omitempty"`
}

type VerificationSummary struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Inconclusive int `json:"inconclusive"`
}

type RunTelemetryResult struct {
	PrimaryResult string                          `json:"primaryResult,omitempty"`
	TaskVerdict   string                          `json:"taskVerdict,omitempty"`
	TerminalState consensus.WorkflowTerminalState `json:"terminalState,omitempty"`
}

type RunTelemetryTiming struct {
	ElapsedMs int64 `json:"elapsedMs"`
}

type RunTelemetrySourceSummary struct {
	ComplianceSummaryPath string `json:"complianceSummaryPath,omitempty"`
}

type DailyReport struct {
	Version          int                     `json:"version"`
	GeneratedAt      string                  `json:"generatedAt"`
	Root             string                  `json:"root"`
	Since            string                  `json:"since"`
	Readiness        []DailyReadinessSummary `json:"readiness,omitempty"`
	TaskCompliance   []DailyTaskSummary      `json:"taskCompliance,omitempty"`
	Workflow         []DailyWorkflowSummary  `json:"workflow,omitempty"`
	RecentRunReports []DailyRunReportSummary `json:"recentRunReports,omitempty"`
}

type DailyReadinessSummary struct {
	Provider             string  `json:"provider"`
	Samples              int     `json:"samples"`
	ReadyCount           int     `json:"readyCount"`
	StrictJSONCount      int     `json:"strictJSONCount"`
	RecoverableJSONCount int     `json:"recoverableJSONCount"`
	MeanDurationMs       float64 `json:"meanDurationMs"`
	LastError            string  `json:"lastError,omitempty"`
}

type DailyTaskSummary struct {
	Provider      string `json:"provider"`
	ProviderModel string `json:"providerModel"`
	Mode          string `json:"mode"`
	TaskKind      string `json:"taskKind"`
	Runs          int    `json:"runs"`
	Total         int    `json:"total"`
	Strict        int    `json:"strict"`
	Normalized    int    `json:"normalized"`
	Repaired      int    `json:"repaired"`
	Failed        int    `json:"failed"`
}

type DailyWorkflowSummary struct {
	Mode                     string  `json:"mode"`
	Runs                     int     `json:"runs"`
	Completed                int     `json:"completed"`
	MeanKeepWithCaveatClaims float64 `json:"meanKeepWithCaveatClaims"`
	MeanUnresolvedClaims     float64 `json:"meanUnresolvedClaims"`
	MeanElapsedMs            float64 `json:"meanElapsedMs"`
}

type DailyRunReportSummary struct {
	RequestID     string `json:"requestId"`
	Mode          string `json:"mode"`
	PrimaryResult string `json:"primaryResult,omitempty"`
	TaskVerdict   string `json:"taskVerdict,omitempty"`
	TerminalState string `json:"terminalState,omitempty"`
	RunDir        string `json:"runDir"`
}

func WriteProviderReadinessFile(path string, entries []ProviderReadinessEntry, now time.Time) error {
	payload := ProviderReadinessFile{
		Version:     1,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Providers:   append([]ProviderReadinessEntry(nil), entries...),
	}
	return writeJSON(path, payload)
}

func ReadProviderReadinessFile(path string) (ProviderReadinessFile, error) {
	return readJSONFile[ProviderReadinessFile](path)
}

func ReadComplianceSummaryFile(path string) (ComplianceSummaryFile, error) {
	return readJSONFile[ComplianceSummaryFile](path)
}

func WriteRunTelemetryFile(path string, value RunTelemetryFile) error {
	return writeJSON(path, value)
}

func ReadRunTelemetryFile(path string) (RunTelemetryFile, error) {
	return readJSONFile[RunTelemetryFile](path)
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func readJSONFile[T any](path string) (T, error) {
	var zero T
	body, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return zero, err
	}
	return out, nil
}
