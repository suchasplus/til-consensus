package runmanager

import (
	"encoding/json"
	"fmt"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

const MaxBranchInputBytes = 128 * 1024

type selectionManifest struct {
	Source         string   `json:"source"`
	Representation string   `json:"representation"`
	SourceCount    int      `json:"sourceCount"`
	SelectedRanges [][2]int `json:"selectedRanges"`
	OmittedRanges  [][2]int `json:"omittedRanges"`
	OmittedFields  []string `json:"omittedFields,omitempty"`
}
type parentContextManifest struct {
	Policy                string              `json:"policy"`
	SourceRequestHash     string              `json:"sourceRequestHash,omitempty"`
	SourceSnapshotHash    string              `json:"sourceSnapshotHash"`
	SourceSnapshotBytes   int                 `json:"sourceSnapshotBytes"`
	SelectedBodyBytes     int                 `json:"selectedBodyBytes"`
	MaxInputBytes         int                 `json:"maxInputBytes"`
	RangeConvention       string              `json:"rangeConvention"`
	FieldContentTruncated bool                `json:"fieldContentTruncated"`
	Selections            []selectionManifest `json:"selections"`
}
type parentDecisionContext struct {
	Outcome          string   `json:"outcome"`
	CoreOutcome      string   `json:"coreOutcome"`
	Summary          string   `json:"summary"`
	MinorityOpinions []string `json:"minorityOpinions"`
	Assumptions      []string `json:"assumptions"`
}
type parentEvidenceIndex struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	Source             string `json:"source"`
	ArtifactRef        string `json:"artifactRef,omitempty"`
	VerificationStatus string `json:"verificationStatus"`
}
type parentMaterialIndex struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Source        string `json:"source"`
	SHA256        string `json:"sha256"`
	RawSHA256     string `json:"rawSha256,omitempty"`
	ByteLength    int    `json:"byteLength"`
	Normalization string `json:"normalization"`
}
type parentContextData struct {
	Materials           []parentMaterialIndex     `json:"materials"`
	ParentRunID         string                    `json:"parentRunId"`
	ProviderMode        string                    `json:"providerMode"`
	Goal                string                    `json:"goal"`
	HardConstraints     consensus.TaskConstraints `json:"hardConstraints"`
	Proposal            *model.Proposal           `json:"proposal,omitempty"`
	Decision            *parentDecisionContext    `json:"decision,omitempty"`
	UnresolvedClaims    []model.ClaimView         `json:"unresolvedClaims"`
	UnresolvedIssues    []model.ReviewIssue       `json:"unresolvedIssues"`
	UnresolvedQuestions []string                  `json:"unresolvedQuestions"`
	EvidenceIndex       []parentEvidenceIndex     `json:"evidenceIndex"`
}
type parentContext struct {
	parentContextData
	ContextManifest parentContextManifest `json:"contextManifest"`
}

// compactParentContext selects only current decision material. No model is called
// and no selected statement is shortened. Every excluded range is declared.
func compactParentContext(parent *model.StoredRun) ([]byte, error) {
	s := parent.Snapshot
	data := parentContextData{ParentRunID: s.ID, ProviderMode: s.ProviderMode, Goal: s.Goal, Proposal: s.Proposal, UnresolvedClaims: []model.ClaimView{}, UnresolvedIssues: []model.ReviewIssue{}, EvidenceIndex: []parentEvidenceIndex{}}
	data.Materials = []parentMaterialIndex{}
	for _, m := range s.Materials {
		data.Materials = append(data.Materials, parentMaterialIndex{ID: m.ID, Title: m.Title, Source: m.Source, SHA256: m.SHA256, RawSHA256: m.RawSHA256, ByteLength: m.ByteLength, Normalization: m.Normalization})
	}
	if parent.CoreResult != nil {
		data.UnresolvedQuestions = append([]string{}, parent.CoreResult.Report.UnresolvedQuestions...)
	} else {
		data.UnresolvedQuestions = []string{}
	}
	if parent.Request != nil {
		data.HardConstraints = parent.Request.TaskSpec.Constraints
	}
	if s.Decision != nil {
		d := s.Decision
		data.Decision = &parentDecisionContext{Outcome: d.Outcome, CoreOutcome: d.CoreOutcome, Summary: d.Summary, MinorityOpinions: d.MinorityOpinions, Assumptions: d.Assumptions}
	}
	claims := make([]bool, len(s.Claims))
	for i, c := range s.Claims {
		if c.Accepted == nil || !*c.Accepted {
			claims[i] = true
			data.UnresolvedClaims = append(data.UnresolvedClaims, c)
		}
	}
	issues := make([]bool, len(s.Reviews))
	for i, issue := range s.Reviews {
		if issue.Status != "resolved" {
			issues[i] = true
			data.UnresolvedIssues = append(data.UnresolvedIssues, issue)
		}
	}
	for _, e := range s.Evidence {
		data.EvidenceIndex = append(data.EvidenceIndex, parentEvidenceIndex{ID: e.ID, Kind: e.Kind, Source: e.Source, ArtifactRef: e.ArtifactRef, VerificationStatus: e.VerificationStatus})
	}
	original, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	selections := []selectionManifest{
		{Source: "materials", Representation: "complete frozen source snapshots inherited separately in task materials; this context contains hashes and source index only", SourceCount: len(s.Materials), SelectedRanges: fullRange(len(s.Materials)), OmittedRanges: [][2]int{}, OmittedFields: []string{"content (provided separately without truncation)"}},
		{Source: "goal", Representation: "full UTF-8 byte range", SourceCount: len(s.Goal), SelectedRanges: fullRange(len(s.Goal)), OmittedRanges: [][2]int{}},
		{Source: "hardConstraints", Representation: "all declared constraints; goal retains unstructured user constraints", SourceCount: boolCount(parent.Request != nil), SelectedRanges: fullRange(boolCount(parent.Request != nil)), OmittedRanges: [][2]int{}},
		{Source: "proposal", Representation: "full current proposal only", SourceCount: boolCount(s.Proposal != nil), SelectedRanges: fullRange(boolCount(s.Proposal != nil)), OmittedRanges: [][2]int{}},
		rangeSelection("claims", "unresolved claims only", claims), rangeSelection("reviews", "all unresolved issues including blocking items", issues),
		{Source: "coreResult.report.unresolvedQuestions", Representation: "all unresolved report questions", SourceCount: len(data.UnresolvedQuestions), SelectedRanges: fullRange(len(data.UnresolvedQuestions)), OmittedRanges: [][2]int{}},
		{Source: "evidence", Representation: "index-only; content remains in parent run and immutable ledger", SourceCount: len(s.Evidence), SelectedRanges: fullRange(len(s.Evidence)), OmittedRanges: [][2]int{}, OmittedFields: []string{"summary"}},
		{Source: "decision", Representation: "outcome, coreOutcome, summary, minorityOpinions, assumptions", SourceCount: boolCount(s.Decision != nil), SelectedRanges: fullRange(boolCount(s.Decision != nil)), OmittedRanges: [][2]int{}, OmittedFields: []string{"votes", "ballot bindings"}},
		{Source: "messages", Representation: "omitted history; no raw model output enters branch", SourceCount: len(s.Messages), SelectedRanges: [][2]int{}, OmittedRanges: fullRange(len(s.Messages))},
		{Source: "proposalHistory", Representation: "omitted history; current version is retained in proposal", SourceCount: len(s.ProposalHistory), SelectedRanges: [][2]int{}, OmittedRanges: fullRange(len(s.ProposalHistory))},
	}
	envelope := parentContext{parentContextData: data, ContextManifest: parentContextManifest{Policy: "current-decision-context-v1", SourceRequestHash: parent.Frozen.RequestHash, SourceSnapshotHash: workflow.Hash(string(original)), SourceSnapshotBytes: len(original), SelectedBodyBytes: len(body), MaxInputBytes: MaxBranchInputBytes, RangeConvention: "zero-based [start,end), collection indices except explicitly labeled UTF-8 byte ranges", FieldContentTruncated: false, Selections: selections}}
	material, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if len(material) > MaxBranchInputBytes {
		return nil, fmt.Errorf("父讨论必要材料为 %d 字节，超过 %d KiB 上限；已保留目标、约束、当前方案、少数意见和未决项，不能静默删减，请缩小目标或拆分讨论", len(material), MaxBranchInputBytes/1024)
	}
	return material, nil
}
func fullRange(count int) [][2]int {
	if count == 0 {
		return [][2]int{}
	}
	return [][2]int{{0, count}}
}
func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
func rangeSelection(source, representation string, selected []bool) selectionManifest {
	result := selectionManifest{Source: source, Representation: representation, SourceCount: len(selected), SelectedRanges: [][2]int{}, OmittedRanges: [][2]int{}}
	for start := 0; start < len(selected); {
		end := start + 1
		for end < len(selected) && selected[end] == selected[start] {
			end++
		}
		if selected[start] {
			result.SelectedRanges = append(result.SelectedRanges, [2]int{start, end})
		} else {
			result.OmittedRanges = append(result.OmittedRanges, [2]int{start, end})
		}
		start = end
	}
	return result
}
