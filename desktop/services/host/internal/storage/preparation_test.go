package storage

import (
	"context"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func TestPreparationConsumeAndRunCreationRollbackTogether(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	p := model.PreparationRecord{ID: "atomic-preparation", Status: "completed", InputHash: "input-hash", RequestHash: "request-hash", UpdatedAt: "now"}
	if err := s.SavePreparation(ctx, &p); err != nil {
		t.Fatal(err)
	}
	existing := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "existing", UpdatedAt: "now"}}}
	if err := s.Save(ctx, existing, nil, "collision", "start", "other-input"); err != nil {
		t.Fatal(err)
	}
	candidate := &model.StoredRun{ConsumePreparationID: p.ID, Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "candidate", UpdatedAt: "now"}, Preparation: &p}}
	if err := s.Save(ctx, candidate, nil, "collision", "start", "candidate-input"); err == nil {
		t.Fatal("没有触发幂等键冲突")
	}
	after, _ := s.GetPreparation(ctx, p.ID)
	if after.UsedByRunID != "" {
		t.Fatal("run事务失败仍然消费准备")
	}
	if _, err := s.Load(ctx, "candidate"); err == nil {
		t.Fatal("事务失败留下run")
	}
	if err := s.Save(ctx, candidate, nil, "unique", "start", "candidate-input"); err != nil {
		t.Fatal(err)
	}
	if candidate.ConsumePreparationID != "" {
		t.Fatal("成功Save未清空transient消费字段")
	}
	if err := s.Save(ctx, candidate, nil, "", "", ""); err != nil {
		t.Fatal("同指针后续Save重复消费", err)
	}
	after, _ = s.GetPreparation(ctx, p.ID)
	if after.UsedByRunID != "candidate" {
		t.Fatal("准备和run没有绑定")
	}
	second := &model.StoredRun{ConsumePreparationID: p.ID, Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "second", UpdatedAt: "now"}, Preparation: &p}}
	if err := s.Save(ctx, second, nil, "second-key", "start", "same-preparation"); err == nil {
		t.Fatal("同结果批准第二个run")
	}
	if id, err := s.LookupKey(ctx, "second-key", "start", "same-preparation"); err != nil || id != "" {
		t.Fatal("拒绝重用时幂等键没有回滚")
	}
}
