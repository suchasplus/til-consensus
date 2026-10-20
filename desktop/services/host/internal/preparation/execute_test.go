package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

func recordFixture(t *testing.T) model.PreparationRecord {
	t.Helper()
	materials := sampleMaterials(t)
	return model.PreparationRecord{ID: "preparation-test", Goal: "制定迁移方案", Materials: materials, InputHash: HashInput("制定迁移方案", materials), RequestHash: "manager-frozen-request", Status: "running", DiagnosticsDir: t.TempDir()}
}
func probeFixture(_ context.Context, c providers.Config) providers.Info {
	_, info, _ := providers.Resolve(c.ID, []providers.Config{c})
	info.Path = "/fixture/" + c.ID
	info.Version = "native-fixture-v1"
	info.Available = true
	return info
}

func TestExecuteUsesOneReadOnlyRequestAndKeepsMetadataPrivate(t *testing.T) {
	r := recordFixture(t)
	calls := 0
	raw := "{\"type\":\"thinking\",\"text\":\"PRIVATE-REASONING\"}\n{\"type\":\"end\",\"stopReason\":\"end_turn\",\"modelUsage\":{\"grok-4.7-build\":{}},\"usage\":{\"input_tokens\":100,\"output_tokens\":20},\"total_cost_usd\":0.004}\n"
	executor := Executor{Probe: probeFixture, Run: func(ctx context.Context, request CLIRequest) (clirunner.RunTaskResult, error) {
		calls++
		if request.Provider.PermissionMode != "read-only" || request.Provider.OutputFormat != "stream-json" {
			t.Fatal("准备检查未使用只读流式profile")
		}
		if !strings.HasPrefix(request.Provider.Workdir, r.DiagnosticsDir) {
			t.Fatal("准备检查工作目录未隔离")
		}
		if files, err := os.ReadDir(request.Provider.Workdir); err != nil || len(files) != 0 {
			t.Fatal("私有cwd不是空目录", err)
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > ExecutionTimeout {
			t.Fatal("缺少120秒上限")
		}
		if !strings.Contains(request.Prompt, "none-utf8") || !strings.Contains(request.Prompt, r.Materials[0].SHA256) {
			t.Fatal("请求未包含冻结材料及指纹")
		}
		for i := 0; i < 2; i++ {
			if err := request.OnEvent(clirunner.CLIEvent{Type: "usage", Usage: map[string]any{"input_tokens": float64(100), "output_tokens": float64(20)}}); err != nil {
				t.Fatal(err)
			}
		}
		_ = request.OnEvent(clirunner.CLIEvent{Type: "thinking", Text: "PRIVATE-REASONING", Model: "private-thinking-model"})
		return clirunner.RunTaskResult{Output: needsFixture(t), RawOutput: raw, Command: []string{"/fixture/grok"}}, nil
	}}
	out, err := ExecuteWithExecutor(context.Background(), r, providers.Config{ID: "grok", Model: "grok-4.7"}, "cli", 0, executor)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || out.Status != "running" || out.RequestHash != r.RequestHash || out.Classification.Recommendation != "needs_clarification" {
		t.Fatal("改写管理状态／请求指纹或重复调用")
	}
	if out.Source.ActualModel != "grok-4.7-build" || out.Source.RequestedModel != "grok-4.7" || out.Source.ModelSource != "cli-report" || out.Source.Mock {
		t.Fatal("混淆CLI选择与报告来源")
	}
	if out.Usage.InputTokens != 100 || out.Usage.OutputTokens != 20 || out.Usage.Calls != 1 || out.CostUSD == nil || *out.CostUSD != 0.004 {
		t.Fatal("usage快照重复相加或费用来源丢失")
	}
	serialized, _ := json.Marshal(out)
	if strings.Contains(string(serialized), "PRIVATE-REASONING") || strings.Contains(string(serialized), "RawDiagnostics") || strings.Contains(string(serialized), r.DiagnosticsDir) {
		t.Fatal("RPC记录暴露私有诊断")
	}
	files, err := filepath.Glob(filepath.Join(r.DiagnosticsDir, "request-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("未独立保存私有诊断", err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "executionRequestHash") || !strings.Contains(string(data), "PRIVATE-REASONING") {
		t.Fatal("诊断未保留真实原输出或实际请求hash")
	}
	if info, err := os.Stat(files[0]); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("原始诊断文件权限不私有")
	}
}

func TestExecuteCannotInferModelsOrFallbackAfterInvalidResponse(t *testing.T) {
	r := recordFixture(t)
	calls := 0
	executor := Executor{Probe: probeFixture, Run: func(_ context.Context, _ CLIRequest) (clirunner.RunTaskResult, error) {
		calls++
		return clirunner.RunTaskResult{Output: needsFixture(t), RawOutput: "{}"}, nil
	}}
	out, err := ExecuteWithExecutor(context.Background(), r, providers.Config{ID: "agy", Model: "gemini-3.8-flash-medium"}, "cli", 0, executor)
	if err != nil {
		t.Fatal(err)
	}
	if out.Source.ActualModel != "" || out.Source.ModelSource != "explicit-request" || out.CostUSD != nil {
		t.Fatal("虚构实际模型或未知费用")
	}
	executor.Run = func(_ context.Context, _ CLIRequest) (clirunner.RunTaskResult, error) {
		calls++
		return clirunner.RunTaskResult{Output: `{"classification":{}}`, RawOutput: "recorded-invalid-response"}, nil
	}
	out, err = ExecuteWithExecutor(context.Background(), r, providers.Config{ID: "agy"}, "cli", 0, executor)
	if err == nil || calls != 2 || out.Classification != nil || out.MaterialAnalysis != nil || out.Source.Mock || out.Error == "" {
		t.Fatal("无效输出被自动修复、mock回退或沿用旧结果")
	}
}

func TestPreparationCancellationAndSnapshotTamper(t *testing.T) {
	r := recordFixture(t)
	calls := 0
	executor := Executor{Probe: probeFixture, Run: func(ctx context.Context, _ CLIRequest) (clirunner.RunTaskResult, error) {
		calls++
		<-ctx.Done()
		return clirunner.RunTaskResult{}, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	out, err := ExecuteWithExecutor(ctx, r, providers.Config{ID: "agy"}, "cli", 0, executor)
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 || out.Classification != nil {
		t.Fatal("超时未取消或被当成成功", err)
	}
	r.Materials[0].Content += "tampered"
	if _, err := ExecuteWithExecutor(context.Background(), r, providers.Config{ID: "agy"}, "cli", 0, executor); err == nil || calls != 1 {
		t.Fatal("被篡改快照仍触发模型")
	}
}

func TestExplicitMockHasNoKeywordClassificationOrNativeProbe(t *testing.T) {
	for _, goal := range []string{"事实核查", "匿名评分", "随意说明"} {
		materials := sampleMaterials(t)
		r := model.PreparationRecord{Goal: goal, Materials: materials}
		out, err := ExecuteWithExecutor(context.Background(), r, providers.Config{}, "mock", 0, Executor{Probe: func(context.Context, providers.Config) providers.Info {
			t.Fatal("mock探测了真实CLI")
			return providers.Info{}
		}, Run: func(context.Context, CLIRequest) (clirunner.RunTaskResult, error) {
			t.Fatal("mock调用了真实CLI")
			return clirunner.RunTaskResult{}, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		if out.Classification.Recommendation != "free_debate" || out.Source == nil || !out.Source.Mock || out.Source.ModelSource != "mock" {
			t.Fatal("Mock使用关键词伪判断或缺少标记")
		}
	}
}
