package consensus

import (
	"context"
	"strings"
	"testing"
)

func collectProcessNotes(result *RunResult) []string {
	notes := make([]string, 0)
	for _, round := range result.FreeDebate.Rounds {
		for _, participant := range round.ParticipantOutputs {
			notes = append(notes, participant.ProcessNotes...)
		}
	}
	return notes
}

// Regression for claim c56b67a65a54 from run tc_1783428965128_64bf7a: its
// wording ("语义重叠/冗余/合并") slipped past the keyword blacklist and was
// voted into the consensus set. With schema self-classification the model's
// own category=process label routes it to process notes regardless of
// phrasing.
func TestFreeDebateCategorizedProcessClaimNeverEntersVote(t *testing.T) {
	metaStatement := "当前34条peer claims中存在大量语义重叠：至少7条'综合推荐'表述了实质相同的立场。这种冗余使共识文档难以作为工程实践指南使用，建议将所有相似立场合并为主题维度。"
	delegate := &stubDelegate{debateDrafts: []ClaimDraft{{
		Title:     "共识文档结构建议",
		Statement: metaStatement,
		Category:  DebateClaimCategoryProcess,
	}}}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newDebateDegradationRequest("debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	for _, claim := range result.FreeDebate.Claims {
		if strings.Contains(claim.Statement, "语义重叠") {
			t.Fatalf("process-labeled claim leaked into the claim graph: %#v", claim)
		}
	}
	notes := collectProcessNotes(result)
	if len(notes) == 0 || !strings.Contains(notes[0], "语义重叠") {
		t.Fatalf("expected process claim preserved as a note, got %#v", notes)
	}
}

// Unlabeled drafts still go through the keyword backstop, but instead of
// being silently dropped they are preserved as process notes.
func TestFreeDebateKeywordBackstopRoutesToNotes(t *testing.T) {
	delegate := &stubDelegate{debateDrafts: []ClaimDraft{{
		Title:     "43 条 peer claims 可合并为约 12 条独立论点",
		Statement: "本轮 43 条 peer claims 的实际独立论点约 12 个，建议系统层面实施去重，将声明数量控制在 15 条以内。",
	}}}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newDebateDegradationRequest("debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	for _, claim := range result.FreeDebate.Claims {
		if strings.Contains(claim.Statement, "去重") {
			t.Fatalf("keyword-flagged claim leaked into the claim graph: %#v", claim)
		}
	}
	if notes := collectProcessNotes(result); len(notes) == 0 {
		t.Fatal("expected keyword-flagged claim preserved as a process note")
	}
}

func TestFreeDebateDomainCategoryClaimEntersDebate(t *testing.T) {
	delegate := &stubDelegate{debateDrafts: []ClaimDraft{{
		Title:     "Delta 方法适合实时看板",
		Statement: "Delta 方法以 O(N) 成本提供方差估计，适合实时看板场景。",
		Category:  DebateClaimCategoryDomain,
	}}}
	engine := newDegradationEngine(delegate)
	result, err := engine.Start(context.Background(), newDebateDegradationRequest("debater-1", "debater-2"))
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	found := false
	for _, claim := range result.FreeDebate.Claims {
		if strings.Contains(claim.Statement, "Delta 方法") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected domain claim to enter the debate, got %#v", result.FreeDebate.Claims)
	}
	if notes := collectProcessNotes(result); len(notes) != 0 {
		t.Fatalf("expected no process notes for domain claims, got %#v", notes)
	}
}
