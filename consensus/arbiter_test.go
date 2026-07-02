package consensus

import "testing"

func TestDeriveRecordsFromDecisionsPrefersKeepWithCaveatForCaveatedStrategyRecommendation(t *testing.T) {
	request := StartRequest{
		TaskSpec: TaskSpec{
			TaskType: CaseTaskTypeStrategy,
		},
	}
	claims := []ClaimNode{{
		ClaimID:            "claim-1",
		Statement:          "渐进式收敛方向优于大爆炸式迁移，但具体层级优先级仍需数据验证。",
		ClaimType:          ClaimTypeRecommendation,
		Status:             ClaimStatusRevised,
		Applicability:      "仅在补齐归因数据后适用",
		BoundaryConditions: []string{"需确认平台团队容量"},
		Caveats:            []string{"具体路径仍待验证"},
	}}
	decisions := []ArbiterDecision{{
		ClaimID:    "claim-1",
		Verdict:    ClaimVerdictInsufficientEvidence,
		Confidence: 0.45,
		Rationale:  "方向有初步支撑，但路径细节未证实。",
	}}

	records := deriveRecordsFromDecisions(request, claims, decisions)
	if len(records) != 1 {
		t.Fatalf("expected one adjudication record, got %#v", records)
	}
	if records[0].Disposition != ClaimDispositionKeepWithCaveat {
		t.Fatalf("expected keep_with_caveat, got %#v", records[0])
	}
	if records[0].Actionability != ActionabilityGated {
		t.Fatalf("expected gated actionability, got %#v", records[0])
	}
}

func TestDeriveRecordsFromDecisionsKeepsFactsUnresolvedWhenEvidenceIsInsufficient(t *testing.T) {
	request := StartRequest{
		TaskSpec: TaskSpec{
			TaskType: CaseTaskTypeStrategy,
		},
	}
	claims := []ClaimNode{{
		ClaimID:            "claim-1",
		Statement:          "过去两个季度的跨仓重构主要集中在基础设施层。",
		ClaimType:          ClaimTypeFact,
		Status:             ClaimStatusRevised,
		BoundaryConditions: []string{"样本量不足"},
	}}
	decisions := []ArbiterDecision{{
		ClaimID:    "claim-1",
		Verdict:    ClaimVerdictInsufficientEvidence,
		Confidence: 0.45,
		Rationale:  "缺少分层归因数据。",
	}}

	records := deriveRecordsFromDecisions(request, claims, decisions)
	if len(records) != 1 {
		t.Fatalf("expected one adjudication record, got %#v", records)
	}
	if records[0].Disposition != ClaimDispositionUnresolved {
		t.Fatalf("expected unresolved for fact claim, got %#v", records[0])
	}
}

func TestDeriveRecordsFromDecisionsPrefersKeepWithCaveatForCaveatedRecommendationUndetermined(t *testing.T) {
	request := StartRequest{
		TaskSpec: TaskSpec{
			TaskType: CaseTaskTypeStrategy,
		},
	}
	claims := []ClaimNode{{
		ClaimID:            "claim-1",
		Statement:          "渐进式收敛仍是候选方向，但具体 rollout 顺序取决于容量与发布治理约束。",
		ClaimType:          ClaimTypeRecommendation,
		Status:             ClaimStatusRevised,
		Applicability:      "仅在补齐团队容量数据后适用",
		BoundaryConditions: []string{"需确认发布治理约束"},
		Caveats:            []string{"具体执行顺序仍未闭合"},
	}}
	decisions := []ArbiterDecision{{
		ClaimID:    "claim-1",
		Verdict:    ClaimVerdictUndetermined,
		Confidence: 0.5,
		Rationale:  "方向有支撑，但 rollout 细节仍存在混合信号。",
	}}

	records := deriveRecordsFromDecisions(request, claims, decisions)
	if len(records) != 1 {
		t.Fatalf("expected one adjudication record, got %#v", records)
	}
	if records[0].Disposition != ClaimDispositionKeepWithCaveat {
		t.Fatalf("expected keep_with_caveat, got %#v", records[0])
	}
	if records[0].Actionability != ActionabilityGated {
		t.Fatalf("expected gated actionability, got %#v", records[0])
	}
}
