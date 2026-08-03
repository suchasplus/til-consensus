package config

import "testing"

func TestPolicyMergesPreserveExplicitFalseAndZero(t *testing.T) {
	debateBase := DebatePolicyConfig{
		VoteQuorum:      float64Ptr(0.6),
		EnableEarlyStop: boolPtr(true),
		SemanticDedup:   DebateSemanticDedupConfig{Enabled: boolPtr(true)},
		Synthesis:       DebateSynthesisConfig{Enabled: boolPtr(true), AmendmentRounds: intPtr(2)},
	}
	debateOverlay := DebatePolicyConfig{
		VoteQuorum:      float64Ptr(0),
		EnableEarlyStop: boolPtr(false),
		SemanticDedup:   DebateSemanticDedupConfig{Enabled: boolPtr(false)},
		Synthesis:       DebateSynthesisConfig{Enabled: boolPtr(false), AmendmentRounds: intPtr(0)},
	}
	debate := mergeDebatePolicy(debateBase, debateOverlay)
	if debate.VoteQuorum == nil || *debate.VoteQuorum != 0 ||
		optionalValue(true, debate.EnableEarlyStop) ||
		optionalValue(true, debate.SemanticDedup.Enabled) ||
		optionalValue(true, debate.Synthesis.Enabled) ||
		optionalValue(-1, debate.Synthesis.AmendmentRounds) != 0 {
		t.Fatalf("explicit debate false/zero values were not preserved: %#v", debate)
	}

	delphi := mergeDelphiPolicy(
		DelphiPolicyConfig{Anonymous: boolPtr(true)},
		DelphiPolicyConfig{Anonymous: boolPtr(false)},
	)
	if optionalValue(true, delphi.Anonymous) {
		t.Fatalf("explicit anonymous=false was not preserved: %#v", delphi)
	}

	verification := mergeVerificationPolicy(
		VerificationPolicyConfig{AllowSemanticVerifier: boolPtr(true)},
		VerificationPolicyConfig{AllowSemanticVerifier: boolPtr(false)},
	)
	arbiter := mergeArbiterPolicy(
		ArbiterPolicyConfig{AllowUndetermined: boolPtr(true), BlindReview: boolPtr(true)},
		ArbiterPolicyConfig{AllowUndetermined: boolPtr(false), BlindReview: boolPtr(false)},
	)
	if optionalValue(true, verification.AllowSemanticVerifier) ||
		optionalValue(true, arbiter.AllowUndetermined) || optionalValue(true, arbiter.BlindReview) {
		t.Fatalf("explicit adjudication false values were not preserved: verification=%#v arbiter=%#v", verification, arbiter)
	}
}
