package config

import (
	"strings"
	"testing"
)

func TestMergeDebatePolicyVoteQuorum(t *testing.T) {
	base := DebatePolicyConfig{VoteQuorum: float64Ptr(0.5)}
	if out := mergeDebatePolicy(base, DebatePolicyConfig{}); optionalValue(0.0, out.VoteQuorum) != 0.5 {
		t.Fatalf("expected base vote_quorum preserved, got %#v", out.VoteQuorum)
	}
	if out := mergeDebatePolicy(base, DebatePolicyConfig{VoteQuorum: float64Ptr(0.8)}); optionalValue(0.0, out.VoteQuorum) != 0.8 {
		t.Fatalf("expected overlay vote_quorum to win, got %#v", out.VoteQuorum)
	}
	if out := mergeDebatePolicy(base, DebatePolicyConfig{VoteQuorum: float64Ptr(0)}); out.VoteQuorum == nil || *out.VoteQuorum != 0 {
		t.Fatalf("expected explicit zero vote_quorum to disable inherited quorum, got %#v", out.VoteQuorum)
	}
}

func TestValidateDefaultsVoteQuorumRange(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.DebatePolicy.VoteQuorum = float64Ptr(1.5)
	err := validateDefaults(cfg)
	if err == nil || !strings.Contains(err.Error(), "vote_quorum") {
		t.Fatalf("expected vote_quorum range error, got %v", err)
	}
	cfg.Defaults.DebatePolicy.VoteQuorum = float64Ptr(-0.1)
	err = validateDefaults(cfg)
	if err == nil || !strings.Contains(err.Error(), "vote_quorum") {
		t.Fatalf("expected vote_quorum range error for negative value, got %v", err)
	}
	cfg.Defaults.DebatePolicy.VoteQuorum = float64Ptr(0.6)
	if err := validateDefaults(cfg); err != nil {
		t.Fatalf("expected valid vote_quorum to pass, got %v", err)
	}
}

func TestFreeDebateTemplateSetsVoteQuorum(t *testing.T) {
	selection, err := ResolveTemplateSelection("", "free-debate", "mock", "")
	if err != nil {
		t.Fatalf("ResolveTemplateSelection failed: %v", err)
	}
	cfg, err := BuildTemplateConfig(selection)
	if err != nil {
		t.Fatalf("BuildTemplateConfig failed: %v", err)
	}
	if optionalValue(0.0, cfg.Defaults.DebatePolicy.VoteQuorum) != 0.6 {
		t.Fatalf("expected free-debate template vote_quorum 0.6, got %#v", cfg.Defaults.DebatePolicy.VoteQuorum)
	}
	if cfg.Defaults.DebatePolicy.SupportThreshold != 0.67 {
		t.Fatalf("expected free-debate template support_threshold 0.67, got %.2f", cfg.Defaults.DebatePolicy.SupportThreshold)
	}
	if cfg.Defaults.DebatePolicy.VoteAggregation != "median" {
		t.Fatalf("expected free-debate template vote_aggregation median, got %q", cfg.Defaults.DebatePolicy.VoteAggregation)
	}
	if cfg.Defaults.DebatePolicy.MaxNewClaimsPerRound != 5 {
		t.Fatalf("expected free-debate template max_new_claims_per_round 5, got %d", cfg.Defaults.DebatePolicy.MaxNewClaimsPerRound)
	}
	if cfg.Defaults.DebatePolicy.MaxActiveClaims != 30 {
		t.Fatalf("expected free-debate template max_active_claims 30, got %d", cfg.Defaults.DebatePolicy.MaxActiveClaims)
	}
}

func TestValidateDefaultsSemanticDedupCadenceEnum(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.DebatePolicy.SemanticDedup.Cadence = "hourly"
	err := validateDefaults(cfg)
	if err == nil || !strings.Contains(err.Error(), "cadence") {
		t.Fatalf("expected cadence enum error, got %v", err)
	}
	for _, valid := range []string{"", "per_round", "final"} {
		cfg.Defaults.DebatePolicy.SemanticDedup.Cadence = valid
		if err := validateDefaults(cfg); err != nil {
			t.Fatalf("expected cadence %q to pass, got %v", valid, err)
		}
	}
}

func TestValidateDefaultsClaimBudgets(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.DebatePolicy.MaxNewClaimsPerRound = -1
	if err := validateDefaults(cfg); err == nil || !strings.Contains(err.Error(), "max_new_claims_per_round") {
		t.Fatalf("expected max_new_claims_per_round error, got %v", err)
	}
	cfg = Config{}
	cfg.Defaults.DebatePolicy.MaxActiveClaims = -1
	if err := validateDefaults(cfg); err == nil || !strings.Contains(err.Error(), "max_active_claims") {
		t.Fatalf("expected max_active_claims error, got %v", err)
	}
}

func TestMergeDebatePolicySupportThresholdAndAggregation(t *testing.T) {
	base := DebatePolicyConfig{SupportThreshold: 0.7, VoteAggregation: "median"}
	out := mergeDebatePolicy(base, DebatePolicyConfig{})
	if out.SupportThreshold != 0.7 || out.VoteAggregation != "median" {
		t.Fatalf("expected base values preserved, got %#v", out)
	}
	out = mergeDebatePolicy(base, DebatePolicyConfig{SupportThreshold: 0.9, VoteAggregation: "mean"})
	if out.SupportThreshold != 0.9 || out.VoteAggregation != "mean" {
		t.Fatalf("expected overlay values to win, got %#v", out)
	}
}

func TestValidateDefaultsVoteAggregationEnum(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.DebatePolicy.VoteAggregation = "mode"
	err := validateDefaults(cfg)
	if err == nil || !strings.Contains(err.Error(), "vote_aggregation") {
		t.Fatalf("expected vote_aggregation enum error, got %v", err)
	}
	for _, valid := range []string{"", "median", "mean"} {
		cfg.Defaults.DebatePolicy.VoteAggregation = valid
		if err := validateDefaults(cfg); err != nil {
			t.Fatalf("expected vote_aggregation %q to pass, got %v", valid, err)
		}
	}
}

func TestValidateDefaultsSupportThresholdRange(t *testing.T) {
	cfg := Config{}
	cfg.Defaults.DebatePolicy.SupportThreshold = 1.2
	err := validateDefaults(cfg)
	if err == nil || !strings.Contains(err.Error(), "support_threshold") {
		t.Fatalf("expected support_threshold range error, got %v", err)
	}
}
