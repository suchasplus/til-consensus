package viewer

import (
	"slices"

	"github.com/suchasplus/til-consensus/consensus"
)

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
		DebugEvents:       buildDebugEvents(bundle.Events, bundle.Files.RunDir),
		Telemetry:         buildTelemetry(bundle, limit),
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
			ClaimID:    vote.ClaimID,
			AgentID:    vote.AgentID,
			Vote:       string(vote.Vote),
			Confidence: vote.Confidence,
			Rationale:  vote.Rationale,
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
