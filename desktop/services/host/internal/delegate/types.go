// Package delegate 将 til-consensus 公开任务协议适配为可审计的桌面 CLI 任务。
package delegate

import (
	"context"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
)

type Options struct {
	// Mode 的默认值是 cli；mock 仅供显式指定的自动化测试使用。
	Mode         string
	RunID        string
	AttemptID    string
	ArtifactDir  string
	Delay        time.Duration
	Timeout      time.Duration
	Providers    []providers.Config
	OnMessage    func(Message) error
	OnReview     func(Review) error
	OnProgress   func(ProgressEvent) error
	OnBallot     func(Ballot) error
	OnBeforeCall func(invocationID, agentID string) error
}

type ProgressEvent struct {
	RunID, InvocationID, AgentID, CLIType, Phase, Kind, TaskKind                                                            string
	Text, Delta, Status, Model, RequestedModel, ActualModel, RequestedReasoning, ReportedReasoning, ModelSource, CLIVersion string
	Usage                                                                                                                   map[string]any
	Round                                                                                                                   int
	Complete                                                                                                                bool
}

type BallotBinding struct {
	ProposalHash string `json:"proposalHash"`
	ManifestHash string `json:"manifestHash"`
}

type Ballot struct {
	InvocationID, AgentID, ProposalHash, ManifestHash string
	Valid                                             bool
}

type Message struct {
	AgentID, Role, Label, Content, Phase, Kind                                                                  string
	InvocationID, ArtifactPath, ContextHash                                                                     string
	CLIType, CLIVersion, Model, RequestedModel, ActualModel, RequestedReasoning, ReportedReasoning, ModelSource string
	ProposalContent, ProposalHash, ManifestHash                                                                 string
	Usage                                                                                                       map[string]any
	Cost                                                                                                        *float64
	Complete                                                                                                    bool
	Round                                                                                                       int
}

// Review 引用协调器已分配的 claimId，或尚未获得 ID 的 invocation 草稿。
// 宿主在内核提交批次后解析草稿引用；ClaimDraftIndex 从 0 开始。
type Review struct {
	ID              string   `json:"id"`
	ClaimID         string   `json:"claimId,omitempty"`
	InvocationID    string   `json:"invocationId,omitempty"`
	ClaimDraftIndex *int     `json:"claimDraftIndex,omitempty"`
	DraftStatement  string   `json:"draftStatement,omitempty"`
	RaisedBy        string   `json:"raisedBy"`
	Severity        string   `json:"severity"`
	Status          string   `json:"status"`
	Statement       string   `json:"statement"`
	ResolutionRefs  []string `json:"resolutionRefs"`
	ExclusiveWith   []string `json:"exclusiveWith"`
}

type Envelope struct {
	SchemaVersion int            `json:"schemaVersion"`
	Mock          bool           `json:"mock"`
	InvocationID  string         `json:"invocationId"`
	TaskKind      string         `json:"taskKind"`
	ContextHash   string         `json:"contextHash"`
	CoreOutput    map[string]any `json:"coreOutput"`
	DesktopReview []Review       `json:"desktopReview"`
	BallotBinding *BallotBinding `json:"ballotBinding,omitempty"`
}

var _ consensus.TaskDelegate = (*Delegate)(nil)

// AwaitFinished 等待取消任务的审计产物落盘，供桌面关闭流程使用。
func (d *Delegate) AwaitFinished(ctx context.Context) error {
	d.mu.Lock()
	entries := make([]*entry, 0, len(d.tasks))
	for _, task := range d.tasks {
		entries = append(entries, task)
	}
	d.mu.Unlock()
	for _, task := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-task.done:
		}
	}
	return nil
}
