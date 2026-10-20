package runmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
)

type preparationJob struct {
	requestHash string
	cancel      context.CancelFunc
	done        chan struct{}
	result      model.PreparationRecord
	err         error
}

var preparationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (m *Manager) AnalyzeTopic(ctx context.Context, in model.PreparationInput) (model.PreparationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.PreparationRecord{}, err
	}
	in.Goal = strings.TrimSpace(in.Goal)
	if !preparationIDPattern.MatchString(in.ID) {
		return model.PreparationRecord{}, fmt.Errorf("准备ID必须是1至128个安全字符")
	}
	materials, err := preparation.Normalize(in.Goal, in.Materials)
	if err != nil {
		return model.PreparationRecord{}, err
	}
	savedSettings, err := m.configuredProviders()
	if err != nil {
		return model.PreparationRecord{}, err
	}
	if in.ProviderID == "" {
		// 继续优先 AGY；停用后按提案者顺序明确选择，不依据调用失败回退。
		for _, id := range []string{"agy", "codex", "claude", "grok"} {
			c, _, e := providers.Effective(id, savedSettings)
			if e != nil {
				return model.PreparationRecord{}, e
			}
			if providers.IsEnabled(c) {
				in.ProviderID = id
				break
			}
		}
		if in.ProviderID == "" {
			return model.PreparationRecord{}, fmt.Errorf("尚未启用任何 CLI，请在 CLI 与连接检查中启用至少一家后分析")
		}
	}
	config, _, err := providers.Effective(in.ProviderID, savedSettings)
	if err != nil {
		return model.PreparationRecord{}, err
	}
	if !providers.IsEnabled(config) {
		return model.PreparationRecord{}, fmt.Errorf("CLI %s 已停用，不参与新的准备分析；请改选启用的 CLI", in.ProviderID)
	}
	if m.opt.ProviderMode == "cli" {
		catalog := m.cachedCatalog(config.ID)
		if config.Reasoning != "" && catalog != nil {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			info := providers.Probe(probeCtx, config)
			cancel()
			catalog = providers.CatalogForVersion(catalog, info.Version)
		}
		if err := providers.Validate(config, catalog); err != nil {
			return model.PreparationRecord{}, err
		}
	}
	inputHash := preparation.HashInput(in.Goal, materials)
	requestBody, _ := json.Marshal(struct {
		InputHash    string
		Config       providers.Config
		ProviderMode string
	}{inputHash, config, m.opt.ProviderMode})
	requestHash := workflow.Hash(string(requestBody))
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return model.PreparationRecord{}, fmt.Errorf("后台正在关闭")
	}
	if job := m.preparations[in.ID]; job != nil {
		if job.requestHash != requestHash {
			m.mu.Unlock()
			return model.PreparationRecord{}, fmt.Errorf("准备ID已用于不同输入或模型配置")
		}
		m.mu.Unlock()
		return awaitPreparation(ctx, job)
	}
	exists, err := m.store.PreparationExists(context.Background(), in.ID)
	if err != nil {
		m.mu.Unlock()
		return model.PreparationRecord{}, err
	}
	if exists {
		record, err := m.store.GetPreparation(context.Background(), in.ID)
		m.mu.Unlock()
		if err != nil {
			return record, err
		}
		if record.Status == "cancelled" && record.RequestHash == "" {
			return record, context.Canceled
		}
		if record.RequestHash != requestHash {
			return record, fmt.Errorf("准备ID已用于不同输入或模型配置")
		}
		return record, preparationStatusError(record)
	}
	created := time.Now().UTC()
	record := model.PreparationRecord{ID: in.ID, InputHash: inputHash, RequestHash: requestHash, Goal: in.Goal, Materials: materials, Status: "running", CreatedAt: created.Format(time.RFC3339Nano), UpdatedAt: created.Format(time.RFC3339Nano), DiagnosticsDir: filepath.Join(m.opt.DataDir, "preparations", in.ID)}
	if err := m.store.SavePreparation(context.Background(), &record); err != nil {
		m.mu.Unlock()
		return record, err
	}
	jobCtx, cancel := context.WithDeadline(ctx, created.Add(m.opt.Deadline))
	job := &preparationJob{requestHash: requestHash, cancel: cancel, done: make(chan struct{})}
	m.preparations[in.ID] = job
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer cancel()
		out, executionErr := m.opt.ExecutePreparation(jobCtx, record, config, m.opt.ProviderMode, m.opt.Delay)
		m.mu.Lock()
		defer m.mu.Unlock()
		out.ID = record.ID
		out.InputHash = record.InputHash
		out.RequestHash = record.RequestHash
		out.Goal = record.Goal
		out.Materials = record.Materials
		out.CreatedAt = record.CreatedAt
		out.UpdatedAt = now()
		if jobCtx.Err() != nil {
			executionErr = jobCtx.Err()
		}
		switch {
		case errors.Is(executionErr, context.Canceled):
			out.Status = "cancelled"
			out.Error = "准备分析已取消"
		case executionErr != nil:
			out.Status = "failed"
			out.Error = executionErr.Error()
		default:
			out.Status = "completed"
			out.Error = ""
		}
		if executionErr != nil {
			out.Classification = nil
			out.MaterialAnalysis = nil
		}
		if saveErr := m.store.SavePreparation(context.Background(), &out); saveErr != nil {
			executionErr = saveErr
			out.Status = "failed"
			out.Error = saveErr.Error()
		}
		job.result = out
		job.err = executionErr
		close(job.done)
		delete(m.preparations, in.ID)
	}()
	return awaitPreparation(ctx, job)
}
func awaitPreparation(ctx context.Context, job *preparationJob) (model.PreparationRecord, error) {
	select {
	case <-ctx.Done():
		return model.PreparationRecord{}, ctx.Err()
	case <-job.done:
		return job.result, job.err
	}
}
func preparationStatusError(record model.PreparationRecord) error {
	switch record.Status {
	case "completed":
		return nil
	case "cancelled":
		return context.Canceled
	default:
		if record.Error != "" {
			return fmt.Errorf("%s", record.Error)
		}
		return fmt.Errorf("准备记录状态为 %s，请使用新ID重新分析", record.Status)
	}
}
func (m *Manager) GetPreparation(id string) (model.PreparationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.GetPreparation(context.Background(), id)
}
func (m *Manager) CancelPreparation(id string) (model.PreparationRecord, error) {
	if !preparationIDPattern.MatchString(id) {
		return model.PreparationRecord{}, fmt.Errorf("准备ID无效")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if job := m.preparations[id]; job != nil {
		job.cancel()
	}
	exists, err := m.store.PreparationExists(context.Background(), id)
	if err != nil {
		return model.PreparationRecord{}, err
	}
	record := model.PreparationRecord{ID: id, Status: "cancelled", Error: "准备分析已取消", CreatedAt: now(), UpdatedAt: now(), Materials: []model.Material{}}
	if exists {
		record, err = m.store.GetPreparation(context.Background(), id)
		if err != nil {
			return record, err
		}
		if record.Status != "running" {
			return record, nil
		}
		record.Status = "cancelled"
		record.Error = "准备分析已取消"
		record.UpdatedAt = now()
	}
	err = m.store.SavePreparation(context.Background(), &record)
	return record, err
}
func (m *Manager) reconcilePreparations() error {
	records, err := m.store.Preparations(context.Background())
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Status == "running" {
			record.Status = "interrupted"
			record.Error = "后台已退出，准备分析未完成；请使用新ID重新分析"
			record.UpdatedAt = now()
			if err := m.store.SavePreparation(context.Background(), &record); err != nil {
				return err
			}
		}
	}
	return nil
}

func materialsHash(materials []model.Material) string {
	if materials == nil {
		materials = []model.Material{}
	}
	body, _ := json.Marshal(materials)
	return workflow.Hash(string(body))
}
func preparationHash(record *model.PreparationRecord) string {
	if record == nil {
		return ""
	}
	body, _ := json.Marshal(record)
	return workflow.Hash(string(body))
}
func publicPreparation(record model.PreparationRecord, runID string) *model.PreparationRecord {
	body, _ := json.Marshal(record)
	var out model.PreparationRecord
	_ = json.Unmarshal(body, &out)
	out.Materials = []model.Material{}
	out.UsedByRunID = runID
	return &out
}
func (m *Manager) matchingPreparationLocked(in model.StartRunInput, materials []model.Material) (*model.PreparationRecord, error) {
	if in.PreparationID == "" {
		if in.Mode == "auto" {
			return nil, fmt.Errorf("自动模式需要先完成准备分析")
		}
		return nil, nil
	}
	record, err := m.store.GetPreparation(context.Background(), in.PreparationID)
	if err != nil {
		return nil, err
	}
	if record.Status != "completed" {
		return nil, fmt.Errorf("准备分析未完成，当前状态：%s", record.Status)
	}
	if record.InputHash != preparation.HashInput(in.Goal, materials) {
		return nil, fmt.Errorf("目标或材料已改变，准备结果已失效；请重新分析")
	}
	if record.UsedByRunID != "" {
		return nil, fmt.Errorf("准备结果已被讨论 %s 采用，不能重复批准", record.UsedByRunID)
	}
	if record.Source == nil || record.Source.Mock != (m.opt.ProviderMode == "mock") {
		return nil, fmt.Errorf("准备来源与当前CLI模式不一致，请重新分析")
	}
	created, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil || !time.Now().Before(created.Add(m.opt.Deadline)) {
		return nil, fmt.Errorf("准备结果已超过原讨论期限，请重新分析")
	}
	if in.Mode == "auto" {
		if record.Classification == nil {
			return nil, fmt.Errorf("准备结果缺少模式建议")
		}
		recommendation := record.Classification.Recommendation
		if recommendation != "free_debate" && recommendation != "adjudication" && recommendation != "delphi" {
			return nil, fmt.Errorf("准备建议为 %s，尚不能自动开始；请补充信息或手动选择模式", recommendation)
		}
		if record.Classification.Confidence < 0.6 {
			return nil, fmt.Errorf("模式建议信心不足0.6，不能自动开始；请补充信息或手动选择模式")
		}
		if record.MaterialAnalysis != nil {
			for _, gap := range record.MaterialAnalysis.Gaps {
				if gap.Importance == "required" {
					return nil, fmt.Errorf("存在必须补充的信息：%s", gap.Question)
				}
			}
		}
	}
	return &record, nil
}
func adoptPreparation(r *model.StoredRun, record *model.PreparationRecord) {
	if record == nil {
		return
	}
	r.Snapshot.Preparation = publicPreparation(*record, r.Snapshot.ID)
	r.ConsumePreparationID = record.ID
	r.Frozen.PreparationHash = preparationHash(r.Snapshot.Preparation)
	invocation := "preparation-" + record.ID
	agent := record.Source.ProviderID
	r.CallReservations = map[string]model.CallReservation{invocation: {AgentID: agent, AttemptID: r.Snapshot.AttemptID, At: record.CreatedAt}}
	usage := record.Usage
	usage.Calls = 1
	if record.CostUSD != nil {
		usage.CostUSD = *record.CostUSD
	}
	r.UsageByInvocation = map[string]model.Usage{invocation: usage}
	r.Snapshot.Usage = usage
	r.ActualMetadata = map[string]map[string]any{invocation: {"agentId": agent, "requestedModel": record.Source.RequestedModel, "model": record.Source.ActualModel, "modelSource": record.Source.ModelSource, "cliVersion": record.Source.CLIVersion, "preparationId": record.ID, "purpose": "topic_preparation", "requestedReasoning": record.Source.Reasoning, "reportedReasoning": record.Source.ReportedReasoning}}
	if record.Source.ModelSource == "cli-report" && record.Source.ActualModel != "" {
		r.ActualMetadata[invocation]["reportedModel"] = record.Source.ActualModel
		r.ModelBindings = map[string]model.ModelBinding{agent: {Model: record.Source.ActualModel, RequestedModel: record.Source.RequestedModel, InvocationID: invocation, BoundAt: record.UpdatedAt}}
	}
}

func validatePreparationSource(r *model.StoredRun, record *model.PreparationRecord) error {
	if record == nil {
		return nil
	}
	for _, provider := range r.Frozen.Providers {
		if provider.ID != record.Source.ProviderID {
			continue
		}
		if provider.Enabled != nil && !*provider.Enabled {
			return fmt.Errorf("准备分析来源 CLI 已停用，请重新分析")
		}
		if r.Frozen.ProviderMode == "mock" {
			return nil
		}
		source := record.Source
		if provider.Version != source.CLIVersion || provider.Model != source.RequestedModel || provider.Reasoning != source.Reasoning || (source.Command != "" && provider.Command != source.Command) {
			return fmt.Errorf("准备分析后CLI版本或模型配置已改变，请重新分析")
		}
		return nil
	}
	return fmt.Errorf("准备分析来源 CLI 已停用或未包含在冻结配置中，请重新分析")
}
