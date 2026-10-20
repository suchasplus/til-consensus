package model

import "github.com/suchasplus/til-consensus/consensus"

// Build scripts inject the locked dependency commit. Direct go builds remain
// explicitly unversioned instead of reporting a different historical commit.
var EngineVersion = "dev-unversioned"

const PolicyVersion = "m6-provider-selection-v1"
const SchemaVersion = 6

type ChatMessage struct {
	ID           string `json:"id"`
	AgentID      string `json:"agentId"`
	Role         string `json:"role"`
	Label        string `json:"label"`
	Content      string `json:"content"`
	At           string `json:"at"`
	Phase        string `json:"phase"`
	AttemptID    string `json:"attemptId"`
	Kind         string `json:"kind"`
	InvocationID string `json:"invocationId,omitempty"`
	Streaming    bool   `json:"streaming,omitempty"`
	Round        int    `json:"round,omitempty"`
	Model        string `json:"model,omitempty"`
}
type Proposal struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Hash         string `json:"hash"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"createdAt,omitempty"`
	InvocationID string `json:"invocationId,omitempty"`
	Kind         string `json:"kind,omitempty"`
}
type ReviewIssue struct {
	ID             string   `json:"id"`
	ClaimID        string   `json:"claimId,omitempty"`
	RaisedBy       string   `json:"raisedBy"`
	Severity       string   `json:"severity"`
	Status         string   `json:"status"`
	Statement      string   `json:"statement"`
	ResolutionRefs []string `json:"resolutionRefs"`
}
type ClaimView struct {
	ID           string   `json:"id"`
	Statement    string   `json:"statement"`
	Category     string   `json:"category"`
	SupportScore *float64 `json:"supportScore,omitempty"`
	Accepted     *bool    `json:"accepted,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Adjudication string   `json:"adjudication,omitempty"`
}
type Vote struct {
	AgentID   string  `json:"agentId"`
	ClaimID   string  `json:"claimId"`
	Vote      string  `json:"vote"`
	Support   float64 `json:"support"`
	Rationale string  `json:"rationale,omitempty"`
}
type BallotBinding struct {
	AgentID      string `json:"agentId"`
	InvocationID string `json:"invocationId"`
	ProposalHash string `json:"proposalHash"`
	ManifestHash string `json:"manifestHash"`
	Valid        bool   `json:"valid"`
	Reason       string `json:"reason,omitempty"`
}
type Decision struct {
	Outcome          string   `json:"outcome"`
	CoreOutcome      string   `json:"coreOutcome"`
	Summary          string   `json:"summary"`
	Mock             bool     `json:"mock"`
	Votes            []Vote   `json:"votes"`
	ProposalHash     string   `json:"proposalHash,omitempty"`
	ManifestHash     string   `json:"manifestHash,omitempty"`
	MinorityOpinions []string `json:"minorityOpinions"`
	Assumptions      []string `json:"assumptions"`
}
type ConfigSource struct {
	Kind string `json:"kind"`
	Env  string `json:"env,omitempty"`
}
type ProviderConfig struct {
	Enabled         *bool                   `json:"enabled,omitempty"`
	Sources         map[string]ConfigSource `json:"sources,omitempty"`
	EnvironmentKeys []string                `json:"environmentKeys,omitempty"`
	ID              string                  `json:"id"`
	Command         string                  `json:"command"`
	Model           string                  `json:"model"`
	Reasoning       string                  `json:"reasoning,omitempty"`
	Version         string                  `json:"version"`
	Permission      string                  `json:"permission"`
	Available       bool                    `json:"available"`
}
type Usage struct {
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
	CachedTokens int64   `json:"cachedTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
	Source       string  `json:"source,omitempty"`
	Estimated    bool    `json:"estimated,omitempty"`
	Calls        int     `json:"calls"`
}
type Evidence struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	Summary            string `json:"summary"`
	Source             string `json:"source"`
	ArtifactRef        string `json:"artifactRef,omitempty"`
	VerificationStatus string `json:"verificationStatus"`
}
type RunSummary struct {
	DeletedAt    string `json:"deletedAt,omitempty"`
	ProviderMode string `json:"providerMode"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Goal         string `json:"goal"`
	Mode         string `json:"mode"`
	Status       string `json:"status"`
	Phase        string `json:"phase"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}
type RunSnapshot struct {
	RolePlan           *RolePlan            `json:"rolePlan,omitempty"`
	Materials          []Material           `json:"materials"`
	Preparation        *PreparationRecord   `json:"preparation,omitempty"`
	RequestedMode      string               `json:"requestedMode,omitempty"`
	ChainCalls         int                  `json:"chainCalls"`
	ChainLimit         int                  `json:"chainLimit"`
	DelphiDistribution map[string][]float64 `json:"delphiDistribution,omitempty"`
	RunSummary
	HostPhase             string           `json:"hostPhase"`
	AttemptID             string           `json:"attemptId"`
	SessionID             string           `json:"sessionId,omitempty"`
	Checkpoint            string           `json:"checkpoint,omitempty"`
	DeadlineAt            string           `json:"deadlineAt"`
	Proposal              *Proposal        `json:"proposal,omitempty"`
	Decision              *Decision        `json:"decision,omitempty"`
	Error                 string           `json:"error,omitempty"`
	Messages              []ChatMessage    `json:"messages"`
	Reviews               []ReviewIssue    `json:"reviews"`
	Claims                []ClaimView      `json:"claims"`
	LastEventID           int64            `json:"lastEventId"`
	Providers             []ProviderConfig `json:"providers"`
	ProposalHistory       []Proposal       `json:"proposalHistory"`
	ParentRunID           string           `json:"parentRunId,omitempty"`
	LineageRootID         string           `json:"lineageRootId,omitempty"`
	SourceProposalVersion int              `json:"sourceProposalVersion,omitempty"`
	SourceProposalHash    string           `json:"sourceProposalHash,omitempty"`
	Usage                 Usage            `json:"usage"`
	Ballots               []BallotBinding  `json:"ballots"`
	Evidence              []Evidence       `json:"evidence"`
}
type DesktopEvent struct {
	EventID   int64          `json:"eventId"`
	RunID     string         `json:"runId"`
	AttemptID string         `json:"attemptId"`
	Type      string         `json:"type"`
	At        string         `json:"at"`
	Payload   map[string]any `json:"payload"`
}
type StartRunInput struct {
	Materials      []MaterialInput `json:"materials,omitempty"`
	PreparationID  string          `json:"preparationId,omitempty"`
	Goal           string          `json:"goal"`
	Mode           string          `json:"mode"`
	IdempotencyKey string          `json:"idempotencyKey"`
}
type BranchRunInput struct {
	Materials      []MaterialInput `json:"materials,omitempty"`
	RunID          string          `json:"runId"`
	Goal           string          `json:"goal"`
	IdempotencyKey string          `json:"idempotencyKey"`
}
type ResumeRunInput struct {
	RunID          string `json:"runId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// StoredRun keeps desktop presentation separate from the frozen engine request.
type ModelBinding struct {
	Model          string `json:"model"`
	RequestedModel string `json:"requestedModel,omitempty"`
	InvocationID   string `json:"invocationId"`
	BoundAt        string `json:"boundAt"`
}

type CallReservation struct {
	AgentID   string `json:"agentId"`
	AttemptID string `json:"attemptId"`
	At        string `json:"at"`
}

type StoredRun struct {
	ConsumePreparationID string                     `json:"-"`
	CallReservations     map[string]CallReservation `json:"callReservations,omitempty"`
	CallCountBase        int                        `json:"callCountBase,omitempty"`
	ModelBindings        map[string]ModelBinding    `json:"modelBindings,omitempty"`
	P0InvocationID       string                     `json:"p0InvocationId,omitempty"`
	P0AttemptID          string                     `json:"p0AttemptId,omitempty"`
	Snapshot             RunSnapshot                `json:"snapshot"`
	Request              *consensus.StartRequest    `json:"request,omitempty"`
	Frozen               FrozenConfig               `json:"frozen"`
	ConfigHash           string                     `json:"configHash"`
	StopReason           string                     `json:"stopReason,omitempty"`
	ReviewSources        map[string]ReviewSource    `json:"reviewSources,omitempty"`
	MessageSources       map[string]MessageSource   `json:"messageSources,omitempty"`
	ClaimAliases         map[string]string          `json:"claimAliases,omitempty"`
	ReviewContexts       map[string]jsonContext     `json:"reviewContexts,omitempty"`
	ActualMetadata       map[string]map[string]any  `json:"actualMetadata,omitempty"`
	UsageByInvocation    map[string]Usage           `json:"usageByInvocation,omitempty"`
	ParentMaterial       string                     `json:"parentMaterial,omitempty"`
	CoreResult           *consensus.RunResult       `json:"coreResult,omitempty"`
}
type jsonContext = map[string]any
type ReviewSource struct {
	InvocationID    string   `json:"invocationId,omitempty"`
	ClaimDraftIndex *int     `json:"claimDraftIndex,omitempty"`
	DraftStatement  string   `json:"draftStatement,omitempty"`
	AttemptID       string   `json:"attemptId"`
	OriginalClaimID string   `json:"originalClaimId,omitempty"`
	ExclusiveWith   []string `json:"exclusiveWith"`
}
type MessageSource struct {
	InvocationID string `json:"invocationId"`
	ArtifactPath string `json:"artifactPath"`
	ContextHash  string `json:"contextHash"`
	Complete     bool   `json:"complete"`
	Round        int    `json:"round"`
}
type FrozenConfig struct {
	RolePlan           *RolePlan        `json:"rolePlan,omitempty"`
	MaterialsHash      string           `json:"materialsHash,omitempty"`
	PreparationHash    string           `json:"preparationHash,omitempty"`
	RequestedMode      string           `json:"requestedMode,omitempty"`
	ChainRootID        string           `json:"chainRootId,omitempty"`
	ChainLimit         int              `json:"chainLimit,omitempty"`
	ParentMaterialHash string           `json:"parentMaterialHash,omitempty"`
	EngineVersion      string           `json:"engineVersion"`
	PolicyVersion      string           `json:"policyVersion"`
	ProviderMode       string           `json:"providerMode"`
	DelayMS            int64            `json:"delayMs"`
	DeadlineAt         string           `json:"deadlineAt"`
	GoalHash           string           `json:"goalHash"`
	Mode               string           `json:"mode"`
	RequestHash        string           `json:"requestHash,omitempty"`
	Providers          []ProviderConfig `json:"providers,omitempty"`
}

// RolePlan is frozen with each new discussion. Older discussions without this
// field retain the original Codex proposer and Claude/Grok/AGY reviewer roles.
type RolePlan struct {
	Proposer  string   `json:"proposer"`
	Reviewers []string `json:"reviewers"`
}
