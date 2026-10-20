package runmanager

import (
	"context"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"strings"
)

type stableIDs struct{ session string }

func (s stableIDs) NewSessionID() string             { return s.session }
func (s stableIDs) NewEntityID(prefix string) string { return newID(prefix) }

type engineObserver struct {
	manager *Manager
	runID   string
}

func (o *engineObserver) OnEvent(_ context.Context, e consensus.RunEvent) error {
	return o.manager.update(o.runID, string(e.Type), func(r *model.StoredRun) error {
		if e.Phase != "" {
			r.Snapshot.Phase = string(e.Phase)
		}
		return nil
	}, e.Payload)
}

type trackedStore struct {
	base    consensus.SessionStore
	manager *Manager
	runID   string
}

func (s *trackedStore) Load(ctx context.Context, id string) (*consensus.SessionSnapshot, error) {
	return s.base.Load(ctx, id)
}
func (s *trackedStore) Save(ctx context.Context, snapshot consensus.SessionSnapshot) error {
	if err := s.base.Save(ctx, snapshot); err != nil {
		return err
	}
	return s.track(snapshot.SessionID)
}
func (s *trackedStore) Patch(ctx context.Context, id string, p consensus.SessionPatch) error {
	if err := s.base.Patch(ctx, id, p); err != nil {
		return err
	}
	if p.Checkpoint == nil {
		return nil
	}
	return s.track(id)
}
func (s *trackedStore) track(id string) error {
	snapshot, err := s.base.Load(context.Background(), id)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return nil
	}
	return s.manager.update(s.runID, "checkpoint.saved", func(r *model.StoredRun) error {
		if snapshot.Checkpoint != nil {
			r.Snapshot.Checkpoint = checkpointLabel(snapshot.Checkpoint)
		}
		resolveReviewSources(r, snapshot)
		if len(snapshot.DebateClaims) > 0 {
			r.Snapshot.Claims = []model.ClaimView{}
			for _, c := range snapshot.DebateClaims {
				if c.Active {
					r.Snapshot.Claims = append(r.Snapshot.Claims, model.ClaimView{ID: c.ClaimID, Statement: c.Statement, Category: string(c.Category)})
				}
			}
		}
		if len(snapshot.ClaimGraph) > 0 {
			r.Snapshot.Claims = []model.ClaimView{}
			for _, c := range snapshot.ClaimGraph {
				r.Snapshot.Claims = append(r.Snapshot.Claims, model.ClaimView{ID: c.ClaimID, Statement: c.Statement, Category: string(c.ClaimType), EvidenceRefs: c.EvidenceRefs, Adjudication: string(c.Disposition)})
			}
		}
		return nil
	}, nil)
}

// A desktop draft reference is resolved only after the coordinator has assigned
// canonical IDs. Keep original provenance and merge aliases for later audits.
func resolveReviewSources(r *model.StoredRun, s *consensus.SessionSnapshot) {
	if r.ClaimAliases == nil {
		r.ClaimAliases = map[string]string{}
	}
	for _, claim := range s.DebateClaims {
		if claim.MergedInto != "" {
			r.ClaimAliases[claim.ClaimID] = claim.MergedInto
		}
	}
	canonical := func(id string) string {
		seen := map[string]bool{}
		for r.ClaimAliases[id] != "" && !seen[id] {
			seen[id] = true
			id = r.ClaimAliases[id]
		}
		return id
	}
	for i := range r.Snapshot.Reviews {
		issue := &r.Snapshot.Reviews[i]
		source, ok := r.ReviewSources[issue.ID]
		if !ok {
			continue
		}
		if issue.ClaimID == "" && source.DraftStatement != "" {
			for _, claim := range s.DebateClaims {
				if sameStatement(claim.Statement, source.DraftStatement) && (contains(claim.ProposedBy, issue.RaisedBy) || claim.OwnerID == issue.RaisedBy) {
					issue.ClaimID = claim.ClaimID
					break
				}
			}
			if issue.ClaimID == "" {
				for _, claim := range s.ClaimGraph {
					if sameStatement(claim.Statement, source.DraftStatement) && contains(claim.ProposedBy, issue.RaisedBy) {
						issue.ClaimID = claim.ClaimID
						break
					}
				}
			}
		}
		if issue.ClaimID != "" {
			if source.OriginalClaimID == "" {
				source.OriginalClaimID = issue.ClaimID
			}
			issue.ClaimID = canonical(issue.ClaimID)
		}
		r.ReviewSources[issue.ID] = source
	}
}
func sameStatement(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}
func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func checkpointLabel(c *consensus.SessionCheckpoint) string {
	if c.Step != "" {
		return c.Step
	}
	return string(c.LastCompletedPhase)
}

// A model cannot close an issue merely by writing status=resolved. References
// must already exist in the persisted run (or the current completed output).
func validResolutionRefs(r *model.StoredRun, refs []string, currentInvocation string) bool {
	if len(refs) == 0 {
		return false
	}
	known := map[string]bool{}
	for _, c := range r.Snapshot.Claims {
		known[c.ID] = true
	}
	for id := range r.ClaimAliases {
		known[id] = true
		known[r.ClaimAliases[id]] = true
	}
	for _, m := range r.MessageSources {
		if m.Complete {
			known[m.InvocationID] = true
			known[m.ArtifactPath] = true
		}
	}
	for _, e := range r.Snapshot.Evidence {
		known[e.ID] = true
		if e.ArtifactRef != "" {
			known[e.ArtifactRef] = true
		}
	}
	for _, p := range r.Snapshot.ProposalHistory {
		known[p.ID] = true
		known[p.Hash] = true
	}
	if currentInvocation != "" {
		for _, source := range r.MessageSources {
			if source.InvocationID == currentInvocation && source.Complete {
				known[currentInvocation] = true
			}
		}
	}
	for _, ref := range refs {
		if ref == "" || !known[ref] {
			return false
		}
	}
	return true
}
