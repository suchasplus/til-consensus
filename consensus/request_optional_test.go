package consensus

import "testing"

func TestNormalizeStartRequestDefaultsOnlyUnsetBooleans(t *testing.T) {
	falseValue := false
	request := baseRequest()
	request.Mode = WorkflowModeFreeDebate
	request.Roles = RoleAssignments{Participants: []string{"participant-1", "participant-2"}}
	request.VerificationPolicy.AllowSemanticVerifier = &falseValue
	request.ArbiterPolicy.AllowUndetermined = &falseValue
	request.ArbiterPolicy.BlindReview = &falseValue
	request.DebatePolicy = DebatePolicy{
		MinRounds: 1, MaxRounds: 2, VoteThreshold: 0.7,
		EnableEarlyStop: &falseValue,
	}
	request.DelphiPolicy.Anonymous = &falseValue

	normalized, err := NormalizeStartRequest(request)
	if err != nil {
		t.Fatalf("NormalizeStartRequest failed: %v", err)
	}
	for name, value := range map[string]*bool{
		"allow_semantic_verifier": normalized.VerificationPolicy.AllowSemanticVerifier,
		"allow_undetermined":      normalized.ArbiterPolicy.AllowUndetermined,
		"blind_review":            normalized.ArbiterPolicy.BlindReview,
		"enable_early_stop":       normalized.DebatePolicy.EnableEarlyStop,
		"anonymous":               normalized.DelphiPolicy.Anonymous,
	} {
		if value == nil || *value {
			t.Fatalf("explicit %s=false did not survive normalization: %#v", name, value)
		}
	}

	request.VerificationPolicy.AllowSemanticVerifier = nil
	request.ArbiterPolicy.AllowUndetermined = nil
	request.ArbiterPolicy.BlindReview = nil
	request.DebatePolicy.EnableEarlyStop = nil
	request.DelphiPolicy.Anonymous = nil
	normalized, err = NormalizeStartRequest(request)
	if err != nil {
		t.Fatalf("NormalizeStartRequest failed: %v", err)
	}
	for name, value := range map[string]*bool{
		"allow_semantic_verifier": normalized.VerificationPolicy.AllowSemanticVerifier,
		"allow_undetermined":      normalized.ArbiterPolicy.AllowUndetermined,
		"blind_review":            normalized.ArbiterPolicy.BlindReview,
		"enable_early_stop":       normalized.DebatePolicy.EnableEarlyStop,
		"anonymous":               normalized.DelphiPolicy.Anonymous,
	} {
		if value == nil || !*value {
			t.Fatalf("unset %s should retain the default true value: %#v", name, value)
		}
	}
}
