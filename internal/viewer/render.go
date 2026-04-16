package viewer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/artifact"
	"github.com/suchasplus/til-consensus/internal/consensus"
)

const (
	FormatText     = "text"
	FormatMarkdown = "markdown"
	FormatJSON     = "json"

	SectionAll           = "all"
	SectionOverview      = "overview"
	SectionClaims        = "claims"
	SectionChallenges    = "challenges"
	SectionVerifications = "verifications"
	SectionObservations  = "observations"
	SectionFollowups     = "followups"
	SectionArtifacts     = "artifacts"
	SectionRounds        = "rounds"
	SectionVotes         = "votes"
	SectionStatements    = "statements"
	SectionConvergence   = "convergence"
)

type RunFiles struct {
	RunDir       string
	ResultPath   string
	LedgerPath   string
	SummaryPath  string
	ManifestPath string
}

type RenderOptions struct {
	Format       string
	Sections     []string
	ClaimVerdict consensus.ClaimVerdict
	Limit        int
	Verbose      bool
}

type Bundle struct {
	Result   consensus.RunResult
	Ledger   []consensus.EvidenceRecord
	Manifest []consensus.ArtifactManifestEntry
	Files    RunFiles
	Missing  []string
}

type Overview struct {
	RequestID         string         `json:"requestId"`
	SessionID         string         `json:"sessionId"`
	ParentRequestID   string         `json:"parentRequestId,omitempty"`
	ParentSessionID   string         `json:"parentSessionId,omitempty"`
	ParentCaseID      string         `json:"parentCaseId,omitempty"`
	Trigger           string         `json:"trigger,omitempty"`
	Mode              string         `json:"mode"`
	PrimaryResult     string         `json:"primaryResult"`
	TerminalState     string         `json:"terminalState,omitempty"`
	Elapsed           string         `json:"elapsed"`
	RunDir            string         `json:"runDir"`
	Goal              string         `json:"goal"`
	TaskType          string         `json:"taskType,omitempty"`
	RiskLevel         string         `json:"riskLevel,omitempty"`
	RequiredEvidence  string         `json:"requiredEvidence,omitempty"`
	SuccessCriteria   []string       `json:"successCriteria,omitempty"`
	WorkspaceRoot     string         `json:"workspaceRoot,omitempty"`
	WorkspaceRev      string         `json:"workspaceRevision,omitempty"`
	WorkspacePaths    []string       `json:"workspacePaths,omitempty"`
	ClaimCounts       map[string]int `json:"claimCounts,omitempty"`
	ChallengeCount    int            `json:"challengeCount"`
	VerificationCount int            `json:"verificationCount"`
	ArtifactCount     int            `json:"artifactCount"`
	RoundCount        int            `json:"roundCount"`
}

type ClaimView struct {
	ClaimID       string                 `json:"claimId"`
	Title         string                 `json:"title,omitempty"`
	Statement     string                 `json:"statement"`
	ClaimType     string                 `json:"claimType,omitempty"`
	Verdict       consensus.ClaimVerdict `json:"verdict"`
	Disposition   string                 `json:"disposition,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	Scope         string                 `json:"scope,omitempty"`
	Rationale     string                 `json:"rationale,omitempty"`
	Caveats       []string               `json:"caveats,omitempty"`
	EvidenceRefs  []string               `json:"evidenceRefs,omitempty"`
	ChallengeRefs []string               `json:"challengeRefs,omitempty"`
}

type ChallengeView struct {
	TicketID          string                    `json:"ticketId"`
	ClaimID           string                    `json:"claimId"`
	Kind              string                    `json:"kind"`
	AttackType        string                    `json:"attackType,omitempty"`
	Severity          string                    `json:"severity,omitempty"`
	Status            consensus.ChallengeStatus `json:"status"`
	Statement         string                    `json:"statement"`
	RequestedChecks   []string                  `json:"requestedChecks,omitempty"`
	ResolutionSummary string                    `json:"resolutionSummary,omitempty"`
}

type VerificationView struct {
	EntryID           string                       `json:"entryId"`
	VerificationID    string                       `json:"verificationId,omitempty"`
	ClaimID           string                       `json:"claimId"`
	ChallengeID       string                       `json:"challengeId,omitempty"`
	Kind              string                       `json:"kind"`
	Status            consensus.VerificationStatus `json:"status"`
	FailureCode       string                       `json:"failureCode,omitempty"`
	Summary           string                       `json:"summary"`
	VerdictSuggestion consensus.ClaimVerdict       `json:"verdictSuggestion,omitempty"`
	Confidence        float64                      `json:"confidence,omitempty"`
	ArtifactPath      string                       `json:"artifactPath,omitempty"`
}

type ObservationView struct {
	ObservationID     string `json:"observationId"`
	Outcome           string `json:"outcome"`
	Summary           string `json:"summary"`
	Reopen            bool   `json:"reopen,omitempty"`
	FollowUpCaseID    string `json:"followUpCaseId,omitempty"`
	FollowUpRequestID string `json:"followUpRequestId,omitempty"`
	FollowUpArtifact  string `json:"followUpArtifact,omitempty"`
}

type FollowUpView struct {
	ObservationID     string `json:"observationId,omitempty"`
	ParentRequestID   string `json:"parentRequestId,omitempty"`
	ParentSessionID   string `json:"parentSessionId,omitempty"`
	ParentCaseID      string `json:"parentCaseId,omitempty"`
	Trigger           string `json:"trigger,omitempty"`
	FollowUpCaseID    string `json:"followUpCaseId"`
	FollowUpRequestID string `json:"followUpRequestId,omitempty"`
	ArtifactPath      string `json:"artifactPath,omitempty"`
}

type ArtifactView struct {
	Seq          int                    `json:"seq"`
	EntryID      string                 `json:"entryId"`
	ClaimID      string                 `json:"claimId,omitempty"`
	ChallengeID  string                 `json:"challengeId,omitempty"`
	Kind         consensus.EvidenceKind `json:"kind"`
	ProducerRole string                 `json:"producerRole,omitempty"`
	Path         string                 `json:"path"`
	Hash         string                 `json:"hash,omitempty"`
	MediaType    string                 `json:"mediaType,omitempty"`
}

type RoundView struct {
	Round       int      `json:"round"`
	Phase       string   `json:"phase"`
	Summary     string   `json:"summary,omitempty"`
	Participants []string `json:"participants,omitempty"`
}

type VoteView struct {
	ClaimID   string `json:"claimId"`
	AgentID   string `json:"agentId"`
	Vote      string `json:"vote"`
	Rationale string `json:"rationale,omitempty"`
}

type StatementView struct {
	StatementID    string   `json:"statementId"`
	Statement      string   `json:"statement"`
	MeanRating     float64  `json:"meanRating"`
	ConsensusLevel float64  `json:"consensusLevel"`
	ResponseCount  int      `json:"responseCount"`
	Reasons        []string `json:"reasons,omitempty"`
}

type ConvergenceView struct {
	ConsensusLevel float64  `json:"consensusLevel"`
	Recommendation string   `json:"recommendation"`
	DissentSummary []string `json:"dissentSummary,omitempty"`
}

type RiskView struct {
	Category string `json:"category"`
	TargetID string `json:"targetId"`
	Summary  string `json:"summary"`
}

type FileView struct {
	RunDir   string   `json:"runDir"`
	Result   string   `json:"result"`
	Ledger   string   `json:"ledger"`
	Summary  string   `json:"summary"`
	Manifest string   `json:"manifest"`
	Missing  []string `json:"missing,omitempty"`
}

type Document struct {
	Format            string             `json:"format"`
	RequestedSections []string           `json:"requestedSections"`
	Overview          Overview           `json:"overview"`
	Claims            []ClaimView        `json:"claims,omitempty"`
	Challenges        []ChallengeView    `json:"challenges,omitempty"`
	Verifications     []VerificationView `json:"verifications,omitempty"`
	Observations      []ObservationView  `json:"observations,omitempty"`
	FollowUps         []FollowUpView     `json:"followups,omitempty"`
	Rounds            []RoundView        `json:"rounds,omitempty"`
	Votes             []VoteView         `json:"votes,omitempty"`
	Statements        []StatementView    `json:"statements,omitempty"`
	Convergence       *ConvergenceView   `json:"convergence,omitempty"`
	Artifacts         []ArtifactView     `json:"artifacts,omitempty"`
	Risks             []RiskView         `json:"risks,omitempty"`
	Files             FileView           `json:"files"`
}

func InferRunFiles(resultPath string) RunFiles {
	runDir := filepath.Dir(resultPath)
	return RunFiles{
		RunDir:       runDir,
		ResultPath:   resultPath,
		LedgerPath:   filepath.Join(runDir, "ledger.jsonl"),
		SummaryPath:  filepath.Join(runDir, "summary.md"),
		ManifestPath: filepath.Join(runDir, "artifacts", "manifest.jsonl"),
	}
}

func LoadBundle(files RunFiles) (Bundle, error) {
	body, err := os.ReadFile(files.ResultPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read result file: %w", err)
	}
	result, err := consensus.DecodeRunResult(body)
	if err != nil {
		return Bundle{}, fmt.Errorf("decode result file: %w", err)
	}
	ledger, err := readJSONL[consensus.EvidenceRecord](files.LedgerPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("read ledger file: %w", err)
	}
	manifest, missing, err := readOptionalJSONL[consensus.ArtifactManifestEntry](files.ManifestPath, "artifacts/manifest.jsonl")
	if err != nil {
		return Bundle{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	return Bundle{
		Result:   result,
		Ledger:   ledger,
		Manifest: manifest,
		Files:    files,
		Missing:  missing,
	}, nil
}

func BuildDocument(bundle Bundle, options RenderOptions) Document {
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	doc := Document{
		Format:            normalizeFormat(options.Format),
		RequestedSections: normalizeSections(options.Sections),
		Overview:          buildOverview(bundle),
		Verifications:     limitVerifications(extractVerifications(bundle.Ledger), limit),
		Observations:      buildObservations(bundle.Result, bundle.Files.RunDir, limit),
		FollowUps:         buildFollowUps(bundle.Result, bundle.Files.RunDir, limit),
		Artifacts:         limitArtifacts(extractArtifacts(bundle.Manifest, bundle.Files.RunDir), limit),
		Risks:             buildRisks(bundle.Result, bundle.Ledger),
		Files: FileView{
			RunDir:   displayRunDir(bundle.Files.RunDir),
			Result:   displayCompanionPath(bundle.Files.RunDir, bundle.Files.ResultPath),
			Ledger:   displayCompanionPath(bundle.Files.RunDir, bundle.Files.LedgerPath),
			Summary:  displayCompanionPath(bundle.Files.RunDir, bundle.Files.SummaryPath),
			Manifest: displayCompanionPath(bundle.Files.RunDir, bundle.Files.ManifestPath),
			Missing:  slices.Clone(bundle.Missing),
		},
	}
	switch bundle.Result.Mode {
	case consensus.WorkflowModeAdjudication:
		fillAdjudicationViews(&doc, bundle, options, limit)
	case consensus.WorkflowModeFreeDebate:
		fillDebateViews(&doc, bundle, limit)
	case consensus.WorkflowModeDelphi:
		fillDelphiViews(&doc, bundle, limit)
	}
	return doc
}

func fillAdjudicationViews(doc *Document, bundle Bundle, options RenderOptions, limit int) {
	if bundle.Result.Adjudication == nil {
		return
	}
	claims := sortClaims(bundle.Result.Adjudication.ClaimGraph)
	if options.ClaimVerdict != "" {
		filtered := claims[:0]
		for _, claim := range claims {
			if claim.Verdict == options.ClaimVerdict {
				filtered = append(filtered, claim)
			}
		}
		claims = filtered
	}
	for _, claim := range claims[:min(limit, len(claims))] {
		doc.Claims = append(doc.Claims, ClaimView{
			ClaimID:       claim.ClaimID,
			Title:         claim.Title,
			Statement:     claim.Statement,
			ClaimType:     string(claim.ClaimType),
			Verdict:       claim.Verdict,
			Disposition:   string(claim.Disposition),
			Confidence:    claim.Confidence,
			Scope:         claim.Scope,
			Rationale:     claim.Rationale,
			Caveats:       slices.Clone(claim.Caveats),
			EvidenceRefs:  slices.Clone(claim.EvidenceRefs),
			ChallengeRefs: slices.Clone(claim.ChallengeRefs),
		})
	}
	for _, ticket := range bundle.Result.Adjudication.ChallengeTickets {
		doc.Challenges = append(doc.Challenges, ChallengeView{
			TicketID:          ticket.TicketID,
			ClaimID:           ticket.ClaimID,
			Kind:              ticket.Kind,
			AttackType:        ticket.AttackType,
			Severity:          string(ticket.Severity),
			Status:            ticket.Status,
			Statement:         ticket.Statement,
			RequestedChecks:   slices.Clone(ticket.RequestedChecks),
			ResolutionSummary: ticket.ResolutionSummary,
		})
	}
}

func fillDebateViews(doc *Document, bundle Bundle, limit int) {
	if bundle.Result.FreeDebate == nil {
		return
	}
	section := bundle.Result.FreeDebate
	for _, round := range section.Rounds[:min(limit, len(section.Rounds))] {
		participants := make([]string, 0, len(round.ParticipantOutputs))
		for _, item := range round.ParticipantOutputs {
			participants = append(participants, item.AgentID)
		}
		doc.Rounds = append(doc.Rounds, RoundView{
			Round:        round.Round,
			Phase:        round.Phase,
			Summary:      round.Summary,
			Participants: participants,
		})
	}
	for _, vote := range section.Votes[:min(limit, len(section.Votes))] {
		doc.Votes = append(doc.Votes, VoteView{
			ClaimID:   vote.ClaimID,
			AgentID:   vote.AgentID,
			Vote:      string(vote.Vote),
			Rationale: vote.Rationale,
		})
	}
}

func fillDelphiViews(doc *Document, bundle Bundle, limit int) {
	if bundle.Result.Delphi == nil {
		return
	}
	section := bundle.Result.Delphi
	for _, round := range section.Rounds[:min(limit, len(section.Rounds))] {
		doc.Rounds = append(doc.Rounds, RoundView{
			Round:   round.Round,
			Phase:   round.Phase,
			Summary: round.Summary,
		})
	}
	for _, item := range section.Statements[:min(limit, len(section.Statements))] {
		doc.Statements = append(doc.Statements, StatementView{
			StatementID:    item.StatementID,
			Statement:      item.Statement,
			MeanRating:     item.MeanRating,
			ConsensusLevel: item.ConsensusLevel,
			ResponseCount:  item.ResponseCount,
			Reasons:        slices.Clone(item.RepresentativeReasons),
		})
	}
	doc.Convergence = &ConvergenceView{
		ConsensusLevel: section.ConsensusLevel,
		Recommendation: section.Recommendation,
		DissentSummary: slices.Clone(section.DissentSummary),
	}
}

func RenderDocument(doc Document, options RenderOptions) (string, error) {
	format := normalizeFormat(options.Format)
	switch format {
	case FormatMarkdown:
		return renderMarkdown(doc, options.Verbose), nil
	case FormatJSON:
		body, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal view json: %w", err)
		}
		return string(body) + "\n", nil
	default:
		return renderText(doc, options.Verbose), nil
	}
}

func renderText(doc Document, verbose bool) string {
	var b strings.Builder
	requested := sectionSet(doc.RequestedSections)
	if requested[SectionAll] || requested[SectionOverview] {
		writeTextHeading(&b, "运行头部")
		fmt.Fprintf(&b, "requestId: %s\n", doc.Overview.RequestID)
		fmt.Fprintf(&b, "mode: %s\n", doc.Overview.Mode)
		if doc.Overview.ParentRequestID != "" || doc.Overview.ParentSessionID != "" {
			fmt.Fprintf(&b, "parent: request=%s session=%s case=%s trigger=%s\n", firstNonEmpty(doc.Overview.ParentRequestID, "-"), firstNonEmpty(doc.Overview.ParentSessionID, "-"), firstNonEmpty(doc.Overview.ParentCaseID, "-"), firstNonEmpty(doc.Overview.Trigger, "-"))
		}
		fmt.Fprintf(&b, "result: %s\n", doc.Overview.PrimaryResult)
		if doc.Overview.TerminalState != "" {
			fmt.Fprintf(&b, "terminal state: %s\n", doc.Overview.TerminalState)
		}
		fmt.Fprintf(&b, "elapsed: %s\n", doc.Overview.Elapsed)
		fmt.Fprintf(&b, "result dir: %s\n", doc.Files.RunDir)

		writeTextHeading(&b, "任务摘要")
		fmt.Fprintf(&b, "goal: %s\n", doc.Overview.Goal)
		if doc.Overview.TaskType != "" {
			fmt.Fprintf(&b, "task type: %s\n", doc.Overview.TaskType)
		}
		if doc.Overview.RiskLevel != "" || doc.Overview.RequiredEvidence != "" {
			fmt.Fprintf(&b, "risk/evidence: %s / %s\n", firstNonEmpty(doc.Overview.RiskLevel, "-"), firstNonEmpty(doc.Overview.RequiredEvidence, "-"))
		}
		if len(doc.Overview.SuccessCriteria) > 0 {
			b.WriteString("success criteria:\n")
			for _, item := range doc.Overview.SuccessCriteria {
				fmt.Fprintf(&b, "  - %s\n", item)
			}
		}
		if doc.Overview.WorkspaceRoot != "" {
			b.WriteString("workspace:\n")
			fmt.Fprintf(&b, "  root: %s\n", doc.Overview.WorkspaceRoot)
			if doc.Overview.WorkspaceRev != "" {
				fmt.Fprintf(&b, "  revision: %s\n", doc.Overview.WorkspaceRev)
			}
		}

		writeTextHeading(&b, "统计")
		if len(doc.Overview.ClaimCounts) > 0 {
			fmt.Fprintf(&b, "claims: %d\n", sumCounts(doc.Overview.ClaimCounts))
			fmt.Fprintf(&b, "supported: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictSupported)])
			fmt.Fprintf(&b, "refuted: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictRefuted)])
			fmt.Fprintf(&b, "insufficient evidence: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictInsufficientEvidence)])
			fmt.Fprintf(&b, "undetermined: %d\n", doc.Overview.ClaimCounts[string(consensus.ClaimVerdictUndetermined)])
			fmt.Fprintf(&b, "challenges: %d\n", doc.Overview.ChallengeCount)
			fmt.Fprintf(&b, "verifications: %d\n", doc.Overview.VerificationCount)
		}
		if doc.Overview.RoundCount > 0 {
			fmt.Fprintf(&b, "rounds: %d\n", doc.Overview.RoundCount)
		}
		fmt.Fprintf(&b, "artifacts: %d\n", doc.Overview.ArtifactCount)
	}

	if requested[SectionAll] || requested[SectionClaims] {
		writeTextHeading(&b, "关键 Claims")
		if len(doc.Claims) == 0 {
			b.WriteString("(无 claims)\n")
		}
		for _, claim := range doc.Claims {
			fmt.Fprintf(&b, "[%s/%s] %s (%.2f)\n", claim.Verdict, firstNonEmpty(claim.Disposition, "-"), firstNonEmpty(claim.Title, claim.ClaimID), claim.Confidence)
			fmt.Fprintf(&b, "  statement: %s\n", claim.Statement)
			if verbose && claim.ClaimType != "" {
				fmt.Fprintf(&b, "  claim type: %s\n", claim.ClaimType)
			}
			if verbose && claim.Rationale != "" {
				fmt.Fprintf(&b, "  rationale: %s\n", claim.Rationale)
			}
			if verbose && len(claim.Caveats) > 0 {
				fmt.Fprintf(&b, "  caveats: %s\n", strings.Join(claim.Caveats, "; "))
			}
		}
	}

	if requested[SectionAll] || requested[SectionChallenges] {
		writeTextHeading(&b, "挑战明细")
		if len(doc.Challenges) == 0 {
			b.WriteString("(无 challenges)\n")
		}
		for _, item := range doc.Challenges {
			fmt.Fprintf(&b, "- %s | %s | %s | %s\n", item.TicketID, item.ClaimID, firstNonEmpty(item.AttackType, item.Kind), item.Status)
			fmt.Fprintf(&b, "  %s\n", item.Statement)
			if verbose && item.Severity != "" {
				fmt.Fprintf(&b, "  severity: %s\n", item.Severity)
			}
		}
	}

	if requested[SectionAll] || requested[SectionVerifications] {
		writeTextHeading(&b, "验证明细")
		if len(doc.Verifications) == 0 {
			b.WriteString("(无 verifications)\n")
		}
		for _, item := range doc.Verifications {
			fmt.Fprintf(&b, "- %s | %s | %s\n", item.ClaimID, item.Kind, item.Status)
			fmt.Fprintf(&b, "  %s\n", item.Summary)
			if verbose && item.ArtifactPath != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.ArtifactPath)
			}
		}
	}

	if requested[SectionObservations] || (requested[SectionAll] && len(doc.Observations) > 0) {
		writeTextHeading(&b, "Observations")
		if len(doc.Observations) == 0 {
			b.WriteString("(无 observation 数据)\n")
		}
		for _, item := range doc.Observations {
			fmt.Fprintf(&b, "- %s | %s\n", item.ObservationID, item.Outcome)
			fmt.Fprintf(&b, "  %s\n", item.Summary)
			if item.Reopen {
				fmt.Fprintf(&b, "  reopen: true\n")
			}
			if item.FollowUpCaseID != "" || item.FollowUpRequestID != "" {
				fmt.Fprintf(&b, "  follow-up: case=%s request=%s\n", firstNonEmpty(item.FollowUpCaseID, "-"), firstNonEmpty(item.FollowUpRequestID, "-"))
			}
			if verbose && item.FollowUpArtifact != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.FollowUpArtifact)
			}
		}
	}

	if requested[SectionFollowups] || (requested[SectionAll] && len(doc.FollowUps) > 0) {
		writeTextHeading(&b, "Follow-ups")
		if len(doc.FollowUps) == 0 {
			b.WriteString("(无 follow-up 数据)\n")
		}
		for _, item := range doc.FollowUps {
			fmt.Fprintf(&b, "- child request=%s | case=%s\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"))
			fmt.Fprintf(&b, "  parent request=%s session=%s case=%s\n", firstNonEmpty(item.ParentRequestID, "-"), firstNonEmpty(item.ParentSessionID, "-"), firstNonEmpty(item.ParentCaseID, "-"))
			if item.ObservationID != "" {
				fmt.Fprintf(&b, "  triggered by observation=%s\n", item.ObservationID)
			}
			if item.Trigger != "" {
				fmt.Fprintf(&b, "  trigger=%s\n", item.Trigger)
			}
			if verbose && item.ArtifactPath != "" {
				fmt.Fprintf(&b, "  artifact: %s\n", item.ArtifactPath)
			}
		}
	}

	if requested[SectionAll] || requested[SectionRounds] {
		writeTextHeading(&b, "Rounds")
		if len(doc.Rounds) == 0 {
			b.WriteString("(无 round 数据)\n")
		}
		for _, item := range doc.Rounds {
			fmt.Fprintf(&b, "- round %d | %s\n", item.Round, item.Phase)
			if item.Summary != "" {
				fmt.Fprintf(&b, "  %s\n", item.Summary)
			}
			if verbose && len(item.Participants) > 0 {
				fmt.Fprintf(&b, "  participants: %s\n", strings.Join(item.Participants, ", "))
			}
		}
	}

	if requested[SectionAll] || requested[SectionVotes] {
		writeTextHeading(&b, "Votes")
		if len(doc.Votes) == 0 {
			b.WriteString("(无 vote 数据)\n")
		}
		for _, item := range doc.Votes {
			fmt.Fprintf(&b, "- %s | %s | %s\n", item.ClaimID, item.AgentID, item.Vote)
			if verbose && item.Rationale != "" {
				fmt.Fprintf(&b, "  %s\n", item.Rationale)
			}
		}
	}

	if requested[SectionAll] || requested[SectionStatements] {
		writeTextHeading(&b, "Statements")
		if len(doc.Statements) == 0 {
			b.WriteString("(无 statement 数据)\n")
		}
		for _, item := range doc.Statements {
			fmt.Fprintf(&b, "- %s | mean=%.2f | consensus=%.2f\n", item.Statement, item.MeanRating, item.ConsensusLevel)
			if verbose && len(item.Reasons) > 0 {
				fmt.Fprintf(&b, "  reasons: %s\n", strings.Join(item.Reasons, "; "))
			}
		}
	}

	if requested[SectionAll] || requested[SectionConvergence] {
		writeTextHeading(&b, "Convergence")
		if doc.Convergence == nil {
			b.WriteString("(无 convergence 数据)\n")
		} else {
			fmt.Fprintf(&b, "consensus level: %.2f\n", doc.Convergence.ConsensusLevel)
			fmt.Fprintf(&b, "recommendation: %s\n", firstNonEmpty(doc.Convergence.Recommendation, "未形成明确推荐"))
			for _, item := range doc.Convergence.DissentSummary {
				fmt.Fprintf(&b, "- %s\n", item)
			}
		}
	}

	writeTextHeading(&b, "风险与未决项")
	if len(doc.Risks) == 0 {
		b.WriteString("(无明显风险)\n")
	}
	for _, risk := range doc.Risks {
		fmt.Fprintf(&b, "- %s | %s | %s\n", risk.Category, risk.TargetID, risk.Summary)
	}

	if requested[SectionAll] || requested[SectionArtifacts] {
		writeTextHeading(&b, "相关文件")
		fmt.Fprintf(&b, "- result.json: %s\n", doc.Files.Result)
		fmt.Fprintf(&b, "- ledger.jsonl: %s\n", doc.Files.Ledger)
		fmt.Fprintf(&b, "- summary.md: %s\n", doc.Files.Summary)
		if len(doc.Files.Missing) > 0 {
			fmt.Fprintf(&b, "- artifacts/manifest.jsonl: 缺失 (%s)\n", strings.Join(doc.Files.Missing, ", "))
		} else {
			fmt.Fprintf(&b, "- artifacts/manifest.jsonl: %s\n", doc.Files.Manifest)
		}
		for _, item := range doc.Artifacts {
			fmt.Fprintf(&b, "- %s | %s\n", item.Path, item.Kind)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func renderMarkdown(doc Document, verbose bool) string {
	var b strings.Builder
	b.WriteString("# til-consensus 结果浏览\n\n")
	fmt.Fprintf(&b, "- requestId: `%s`\n", doc.Overview.RequestID)
	fmt.Fprintf(&b, "- mode: `%s`\n", doc.Overview.Mode)
	if doc.Overview.ParentRequestID != "" || doc.Overview.ParentSessionID != "" {
		fmt.Fprintf(&b, "- parent: request=`%s` session=`%s` case=`%s` trigger=`%s`\n", firstNonEmpty(doc.Overview.ParentRequestID, "-"), firstNonEmpty(doc.Overview.ParentSessionID, "-"), firstNonEmpty(doc.Overview.ParentCaseID, "-"), firstNonEmpty(doc.Overview.Trigger, "-"))
	}
	fmt.Fprintf(&b, "- result: `%s`\n", doc.Overview.PrimaryResult)
	if doc.Overview.TerminalState != "" {
		fmt.Fprintf(&b, "- terminal state: `%s`\n", doc.Overview.TerminalState)
	}
	fmt.Fprintf(&b, "- elapsed: `%s`\n", doc.Overview.Elapsed)
	fmt.Fprintf(&b, "- result dir: `%s`\n\n", doc.Files.RunDir)
	b.WriteString("## Task\n\n")
	fmt.Fprintf(&b, "%s\n\n", doc.Overview.Goal)
	if len(doc.Claims) > 0 {
		b.WriteString("## Claims\n\n")
		for _, claim := range doc.Claims {
			fmt.Fprintf(&b, "- `%s` | `%s` | %.2f | %s\n", claim.ClaimID, claim.Verdict, claim.Confidence, claim.Statement)
			if verbose && claim.Rationale != "" {
				fmt.Fprintf(&b, "  rationale: %s\n", claim.Rationale)
			}
		}
		b.WriteString("\n")
	}
	if len(doc.Rounds) > 0 {
		b.WriteString("## Rounds\n\n")
		for _, round := range doc.Rounds {
			fmt.Fprintf(&b, "- round %d | `%s` | %s\n", round.Round, round.Phase, round.Summary)
		}
		b.WriteString("\n")
	}
	if len(doc.Statements) > 0 {
		b.WriteString("## Statements\n\n")
		for _, item := range doc.Statements {
			fmt.Fprintf(&b, "- `%s` | mean=%.2f | consensus=%.2f\n", item.Statement, item.MeanRating, item.ConsensusLevel)
		}
		b.WriteString("\n")
	}
	if len(doc.Observations) > 0 {
		b.WriteString("## Observations\n\n")
		for _, item := range doc.Observations {
			fmt.Fprintf(&b, "- `%s` | `%s` | %s\n", item.ObservationID, item.Outcome, item.Summary)
			if item.FollowUpRequestID != "" || item.FollowUpCaseID != "" {
				fmt.Fprintf(&b, "  follow-up: request=`%s` case=`%s`\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"))
			}
			if verbose && item.FollowUpArtifact != "" {
				fmt.Fprintf(&b, "  artifact: `%s`\n", item.FollowUpArtifact)
			}
		}
		b.WriteString("\n")
	}
	if len(doc.FollowUps) > 0 {
		b.WriteString("## Follow-ups\n\n")
		for _, item := range doc.FollowUps {
			fmt.Fprintf(&b, "- child request=`%s` case=`%s` <- parent request=`%s`\n", firstNonEmpty(item.FollowUpRequestID, "-"), firstNonEmpty(item.FollowUpCaseID, "-"), firstNonEmpty(item.ParentRequestID, "-"))
			if item.ObservationID != "" {
				fmt.Fprintf(&b, "  observation: `%s`\n", item.ObservationID)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## 风险与未决项\n\n")
	if len(doc.Risks) == 0 {
		b.WriteString("_无明显风险_\n\n")
	} else {
		for _, risk := range doc.Risks {
			fmt.Fprintf(&b, "- %s | `%s` | %s\n", risk.Category, risk.TargetID, risk.Summary)
		}
		b.WriteString("\n")
	}
	b.WriteString("## 相关文件\n\n")
	fmt.Fprintf(&b, "- result.json: `%s`\n", doc.Files.Result)
	fmt.Fprintf(&b, "- ledger.jsonl: `%s`\n", doc.Files.Ledger)
	fmt.Fprintf(&b, "- summary.md: `%s`\n", doc.Files.Summary)
	fmt.Fprintf(&b, "- artifacts/manifest.jsonl: `%s`\n", doc.Files.Manifest)
	return b.String()
}

func buildOverview(bundle Bundle) Overview {
	result := bundle.Result
	overview := Overview{
		RequestID:       result.RequestID,
		SessionID:       result.SessionID,
		Mode:            string(result.Mode),
		ParentRequestID: lineageParentRequestID(result.Lineage),
		ParentSessionID: lineageParentSessionID(result.Lineage),
		ParentCaseID:    lineageParentCaseID(result.Lineage),
		Trigger:         lineageTrigger(result.Lineage),
		PrimaryResult:   primaryResult(result),
		TerminalState:   string(result.TerminalState),
		Elapsed:         artifact.FormatDuration(time.Duration(result.Metrics.ElapsedMs) * time.Millisecond),
		RunDir:          displayRunDir(bundle.Files.RunDir),
		Goal:            result.TaskSpec.Goal,
		SuccessCriteria: slices.Clone(result.TaskSpec.SuccessCriteria),
		ArtifactCount:   len(bundle.Manifest),
		WorkspacePaths:  slices.Clone(workspacePaths(result.TaskSpec.WorkspaceSnapshot)),
	}
	if result.CaseManifest != nil {
		overview.TaskType = string(result.CaseManifest.TaskType)
		overview.RiskLevel = string(result.CaseManifest.RiskLevel)
		overview.RequiredEvidence = string(result.CaseManifest.RequiredEvidenceLevel)
	}
	if snapshot := result.TaskSpec.WorkspaceSnapshot; snapshot != nil {
		overview.WorkspaceRoot = snapshot.Root
		overview.WorkspaceRev = snapshot.Revision
	}
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.Adjudication != nil {
			overview.ClaimCounts = countClaimViews(result.Adjudication.ClaimGraph)
			overview.ChallengeCount = len(result.Adjudication.ChallengeTickets)
			overview.VerificationCount = len(extractVerifications(bundle.Ledger))
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			overview.RoundCount = len(result.FreeDebate.Rounds)
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi != nil {
			overview.RoundCount = len(result.Delphi.Rounds)
		}
	}
	return overview
}

func buildObservations(result consensus.RunResult, runDir string, limit int) []ObservationView {
	out := make([]ObservationView, 0, len(result.Observations))
	for _, item := range result.Observations[:min(limit, len(result.Observations))] {
		out = append(out, ObservationView{
			ObservationID:     item.ObservationID,
			Outcome:           string(item.Outcome),
			Summary:           item.Summary,
			Reopen:            item.Reopen,
			FollowUpCaseID:    item.FollowUpCaseID,
			FollowUpRequestID: item.FollowUpRequestID,
			FollowUpArtifact:  displayCompanionPath(runDir, artifactRefPath(item.FollowUpArtifact)),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildFollowUps(result consensus.RunResult, runDir string, limit int) []FollowUpView {
	out := make([]FollowUpView, 0)
	for _, item := range result.Observations {
		if item.FollowUpCaseID == "" && item.FollowUpRequestID == "" && item.FollowUpArtifact == nil {
			continue
		}
		out = append(out, FollowUpView{
			ObservationID:     item.ObservationID,
			ParentRequestID:   result.RequestID,
			ParentSessionID:   result.SessionID,
			ParentCaseID:      lineageCaseID(result),
			Trigger:           "observe_contradiction",
			FollowUpCaseID:    item.FollowUpCaseID,
			FollowUpRequestID: item.FollowUpRequestID,
			ArtifactPath:      displayCompanionPath(runDir, artifactRefPath(item.FollowUpArtifact)),
		})
	}
	if result.Lineage != nil {
		out = append(out, FollowUpView{
			ParentRequestID: result.Lineage.ParentRequestID,
			ParentSessionID: result.Lineage.ParentSessionID,
			ParentCaseID:    result.Lineage.ParentCaseID,
			Trigger:         result.Lineage.Trigger,
			FollowUpRequestID: result.RequestID,
			FollowUpCaseID:    lineageCaseID(result),
		})
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}

func primaryResult(result consensus.RunResult) string {
	switch result.Mode {
	case consensus.WorkflowModeAdjudication:
		if result.Adjudication != nil {
			return string(result.Adjudication.TaskVerdict)
		}
	case consensus.WorkflowModeFreeDebate:
		if result.FreeDebate != nil {
			return string(result.FreeDebate.Outcome)
		}
	case consensus.WorkflowModeDelphi:
		if result.Delphi != nil {
			return firstNonEmpty(result.Delphi.Recommendation, fmt.Sprintf("consensus=%.2f", result.Delphi.ConsensusLevel))
		}
	}
	return ""
}

func countClaimViews(claims []consensus.ClaimNode) map[string]int {
	counts := map[string]int{
		string(consensus.ClaimVerdictSupported):            0,
		string(consensus.ClaimVerdictRefuted):              0,
		string(consensus.ClaimVerdictInsufficientEvidence): 0,
		string(consensus.ClaimVerdictUndetermined):         0,
	}
	for _, claim := range claims {
		counts[string(claim.Verdict)]++
	}
	return counts
}

func workspacePaths(snapshot *consensus.WorkspaceSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	return snapshot.Paths
}

func sortClaims(claims []consensus.ClaimNode) []consensus.ClaimNode {
	out := append([]consensus.ClaimNode(nil), claims...)
	slices.SortFunc(out, func(left, right consensus.ClaimNode) int {
		if left.Verdict != right.Verdict {
			return cmpVerdict(left.Verdict) - cmpVerdict(right.Verdict)
		}
		return strings.Compare(left.ClaimID, right.ClaimID)
	})
	return out
}

func cmpVerdict(verdict consensus.ClaimVerdict) int {
	switch verdict {
	case consensus.ClaimVerdictSupported:
		return 0
	case consensus.ClaimVerdictUndetermined:
		return 1
	case consensus.ClaimVerdictInsufficientEvidence:
		return 2
	case consensus.ClaimVerdictRefuted:
		return 3
	default:
		return 4
	}
}

func extractVerifications(entries []consensus.EvidenceRecord) []VerificationView {
	out := make([]VerificationView, 0)
	for _, entry := range entries {
		switch entry.Kind {
		case consensus.EvidenceKindDeterministicCheck, consensus.EvidenceKindSemanticVerification:
			item := VerificationView{
				EntryID:      entry.EntryID,
				VerificationID: entry.VerificationID,
				ClaimID:      entry.ClaimID,
				ChallengeID:  entry.ChallengeID,
				Summary:      entry.Summary,
			}
			if entry.Artifact != nil {
				item.ArtifactPath = entry.Artifact.Path
			}
			if kind, ok := entry.Metadata["kind"].(string); ok {
				item.Kind = kind
			}
			if status, ok := entry.Metadata["status"].(string); ok {
				item.Status = consensus.VerificationStatus(status)
			}
			if failureCode, ok := entry.Metadata["failureCode"].(string); ok {
				item.FailureCode = failureCode
			}
			if verdict, ok := entry.Metadata["verdictSuggestion"].(string); ok {
				item.VerdictSuggestion = consensus.ClaimVerdict(verdict)
			}
			if confidence, ok := entry.Metadata["confidence"].(float64); ok {
				item.Confidence = confidence
			}
			out = append(out, item)
		}
	}
	return out
}

func limitVerifications(items []VerificationView, limit int) []VerificationView {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func extractArtifacts(entries []consensus.ArtifactManifestEntry, runDir string) []ArtifactView {
	out := make([]ArtifactView, 0, len(entries))
	for _, entry := range entries {
		out = append(out, ArtifactView{
			Seq:          entry.Seq,
			EntryID:      entry.EntryID,
			ClaimID:      entry.ClaimID,
			ChallengeID:  entry.ChallengeID,
			Kind:         entry.Kind,
			ProducerRole: entry.ProducerRole,
			Path:         displayCompanionPath(runDir, entry.Artifact.Path),
			Hash:         entry.Artifact.Hash,
			MediaType:    entry.Artifact.MediaType,
		})
	}
	return out
}

func limitArtifacts(items []ArtifactView, limit int) []ArtifactView {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func buildRisks(result consensus.RunResult, ledger []consensus.EvidenceRecord) []RiskView {
	out := make([]RiskView, 0)
	if result.Adjudication != nil {
		if result.TerminalState != "" {
			out = append(out, RiskView{Category: "terminal_state", TargetID: result.RequestID, Summary: string(result.TerminalState)})
		}
		for _, ticket := range result.Adjudication.ChallengeTickets {
			if ticket.Status == consensus.ChallengeStatusOpen {
				out = append(out, RiskView{Category: "open challenge", TargetID: ticket.ClaimID, Summary: ticket.Statement})
			}
		}
		for _, claim := range result.Adjudication.ClaimGraph {
			if claim.Verdict == consensus.ClaimVerdictInsufficientEvidence || claim.Verdict == consensus.ClaimVerdictUndetermined {
				category := "undetermined"
				if claim.Verdict == consensus.ClaimVerdictInsufficientEvidence {
					category = "insufficient evidence"
				}
				out = append(out, RiskView{Category: category, TargetID: claim.ClaimID, Summary: claim.Statement})
			}
			if claim.Disposition == consensus.ClaimDispositionKeepWithCaveat {
				out = append(out, RiskView{Category: "keep_with_caveat", TargetID: claim.ClaimID, Summary: claim.Statement})
			}
		}
		for _, item := range extractVerifications(ledger) {
			if item.Status == consensus.VerificationStatusFailed || item.Status == consensus.VerificationStatusInconclusive {
				category := "verification failed"
				if item.Status == consensus.VerificationStatusInconclusive {
					category = "verification inconclusive"
				}
				out = append(out, RiskView{Category: category, TargetID: item.ClaimID, Summary: item.Summary})
			}
		}
		for _, item := range result.Observations {
			switch item.Outcome {
			case consensus.ObservationOutcomeContradicted:
				out = append(out, RiskView{Category: "observation_contradicted", TargetID: item.ObservationID, Summary: item.Summary})
			case consensus.ObservationOutcomeFollowUp:
				out = append(out, RiskView{Category: "follow_up_recommended", TargetID: item.ObservationID, Summary: item.Summary})
			}
		}
	}
	if result.FreeDebate != nil && result.FreeDebate.Outcome != consensus.FreeDebateOutcomeConsensus {
		out = append(out, RiskView{Category: "debate_outcome", TargetID: string(result.FreeDebate.Outcome), Summary: "自由辩论未形成完整共识"})
	}
	if result.Delphi != nil {
		for idx, item := range result.Delphi.DissentSummary {
			out = append(out, RiskView{Category: "delphi_dissent", TargetID: fmt.Sprintf("dissent-%d", idx+1), Summary: item})
		}
	}
	return out
}

func normalizeFormat(value string) string {
	switch strings.TrimSpace(value) {
	case FormatMarkdown:
		return FormatMarkdown
	case FormatJSON:
		return FormatJSON
	default:
		return FormatText
	}
}

func normalizeSections(sections []string) []string {
	if len(sections) == 0 {
		return []string{SectionAll}
	}
	out := make([]string, 0, len(sections))
	seen := map[string]struct{}{}
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{SectionAll}
	}
	return out
}

func lineageParentRequestID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentRequestID
}

func lineageParentSessionID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentSessionID
}

func lineageParentCaseID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentCaseID
}

func lineageTrigger(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.Trigger
}

func lineageCaseID(result consensus.RunResult) string {
	if result.CaseManifest == nil {
		return ""
	}
	return result.CaseManifest.CaseID
}

func artifactRefPath(ref *consensus.ArtifactRef) string {
	if ref == nil {
		return ""
	}
	return ref.Path
}

func sectionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func displayRunDir(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Base(path)
}

func displayCompanionPath(runDir string, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(runDir, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "./"
		}
		return "./" + filepath.ToSlash(rel)
	}
	return path
}

func writeTextHeading(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}

func sumCounts(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
