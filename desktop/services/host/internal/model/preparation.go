package model

import tilrunner "github.com/suchasplus/til-consensus/runner"

// MaterialInput 只包含明确提供的文本；Source 是 pasted/file，不是可读取路径。
type MaterialInput struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	RawSHA256 string `json:"rawSha256,omitempty"`
}

type Material struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Source        string `json:"source"`
	RawSHA256     string `json:"rawSha256"`
	SHA256        string `json:"sha256"`
	ByteLength    int    `json:"byteLength"`
	Normalization string `json:"normalization"`
}

type PreparationInput struct {
	ID         string          `json:"id"`
	Goal       string          `json:"goal"`
	Materials  []MaterialInput `json:"materials"`
	ProviderID string          `json:"providerId,omitempty"`
}

type MaterialKeyPoint struct {
	Quote   string `json:"quote"`
	Meaning string `json:"meaning"`
}
type MaterialSummary struct {
	MaterialID string             `json:"materialId"`
	Summary    string             `json:"summary"`
	KeyPoints  []MaterialKeyPoint `json:"keyPoints"`
}
type MaterialGap struct {
	Question    string   `json:"question"`
	Importance  string   `json:"importance"`
	MaterialIDs []string `json:"materialIds"`
}
type MaterialAnalysis struct {
	Summary     string            `json:"summary"`
	Materials   []MaterialSummary `json:"materials"`
	Gaps        []MaterialGap     `json:"gaps"`
	Limitations []string          `json:"limitations"`
}

type PreparationSource struct {
	ProviderID        string `json:"providerId"`
	CLIType           string `json:"cliType"`
	CLIVersion        string `json:"cliVersion"`
	Command           string `json:"command,omitempty"`
	RequestedModel    string `json:"requestedModel"`
	ActualModel       string `json:"actualModel,omitempty"`
	Reasoning         string `json:"reasoning,omitempty"`
	ReportedReasoning string `json:"reportedReasoning,omitempty"`
	ModelSource       string `json:"modelSource"`
	ElapsedMS         int64  `json:"elapsedMs"`
	Mock              bool   `json:"mock,omitempty"`
}

// PreparationRecord 的状态由 RunManager 管理。原始诊断只能私有落盘，
// 不能进入 RPC、普通导出或模型可见的下一轮材料。
type PreparationRecord struct {
	ID               string                    `json:"id"`
	InputHash        string                    `json:"inputHash"`
	RequestHash      string                    `json:"requestHash"`
	Goal             string                    `json:"goal"`
	Materials        []Material                `json:"materials"`
	Status           string                    `json:"status"`
	CreatedAt        string                    `json:"createdAt"`
	UpdatedAt        string                    `json:"updatedAt"`
	Classification   *tilrunner.ClassifyResult `json:"classification,omitempty"`
	MaterialAnalysis *MaterialAnalysis         `json:"materialAnalysis,omitempty"`
	Source           *PreparationSource        `json:"source,omitempty"`
	Usage            Usage                     `json:"usage"`
	CostUSD          *float64                  `json:"costUsd"`
	Error            string                    `json:"error,omitempty"`
	UsedByRunID      string                    `json:"usedByRunId,omitempty"`
	DiagnosticsDir   string                    `json:"-"`
	RawDiagnostics   string                    `json:"-"`
}
