package runmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/workflow"
	"strings"
	"time"
)

type ProviderInfo struct {
	Enabled              bool               `json:"enabled"`
	RequestAttempted     bool               `json:"requestAttempted"`
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	CLIType              string             `json:"cliType"`
	BinaryPath           string             `json:"binaryPath,omitempty"`
	Version              string             `json:"version,omitempty"`
	Available            bool               `json:"available"`
	Model                string             `json:"model,omitempty"`
	Reasoning            string             `json:"reasoning,omitempty"`
	Status               string             `json:"status"`
	Error                string             `json:"error,omitempty"`
	PermissionScope      string             `json:"permissionScope"`
	LastCheckedAt        string             `json:"lastCheckedAt,omitempty"`
	Defaults             providers.Config   `json:"defaults"`
	Saved                *providers.Config  `json:"saved,omitempty"`
	Effective            providers.Config   `json:"effective"`
	Sources              providers.Sources  `json:"sources"`
	Catalog              *providers.Catalog `json:"catalog,omitempty"`
	ExecutableFound      bool               `json:"executableFound"`
	VersionOK            bool               `json:"versionOK"`
	RequestOK            bool               `json:"requestOK"`
	CheckedConfiguration string             `json:"checkedConfiguration,omitempty"`
	RequestedModel       string             `json:"requestedModel,omitempty"`
	ReportedModel        string             `json:"reportedModel,omitempty"`
	RequestedReasoning   string             `json:"requestedReasoning,omitempty"`
	ReportedReasoning    string             `json:"reportedReasoning,omitempty"`
	ModelSource          string             `json:"modelSource,omitempty"`
	LatencyMS            int64              `json:"latencyMs,omitempty"`
	Usage                map[string]any     `json:"usage,omitempty"`
	Cost                 *float64           `json:"cost"`
}
type providerCheckJob struct {
	config string
	done   chan struct{}
	result ProviderInfo
	err    error
}
type catalogJob struct {
	done   chan struct{}
	result providers.Catalog
	err    error
}

func (m *Manager) ProviderMode() string { return m.opt.ProviderMode }
func defaultProviderConfigs() []providers.Config {
	out := []providers.Config{}
	for _, d := range providers.Defaults() {
		c, _, _ := providers.Effective(d.ID, nil)
		out = append(out, c)
	}
	return out
}
func (m *Manager) configuredProviders() ([]providers.Config, error) {
	settings, err := m.store.ProviderSettings(context.Background())
	if err != nil {
		return nil, err
	}
	out := []providers.Config{}
	for _, item := range settings.Providers {
		out = append(out, providers.Config{ID: item.ID, Enabled: item.Enabled, Model: item.Model, Reasoning: item.Reasoning})
	}
	return out, nil
}
func (m *Manager) effectiveProviders() ([]providers.Config, error) {
	saved, err := m.configuredProviders()
	if err != nil {
		return nil, err
	}
	out := []providers.Config{}
	for _, d := range providers.Defaults() {
		c, _, err := providers.Effective(d.ID, saved)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
func (m *Manager) cachedCatalog(id string) *providers.Catalog {
	body, err := m.store.ProviderCatalog(context.Background(), id)
	if err != nil || len(body) == 0 {
		return nil
	}
	var cat providers.Catalog
	if json.Unmarshal(body, &cat) != nil {
		return nil
	}
	return &cat
}
func (m *Manager) settingsView(c providers.Config, saved []providers.Config) ProviderInfo {
	effective, sources, _ := providers.Effective(c.ID, saved)
	view := ProviderInfo{ID: c.ID, Enabled: providers.IsEnabled(effective), Name: map[string]string{"codex": "Codex", "claude": "Claude Code", "grok": "Grok Build", "agy": "AGY"}[c.ID], CLIType: c.ID, Model: effective.Model, Reasoning: effective.Reasoning, Effective: effective, Sources: sources, PermissionScope: providerPermission(c.ID), Status: "unprobed"}
	for _, d := range providers.ApplicationDefaults() {
		if d.ID == c.ID {
			view.Defaults = d
		}
	}
	for _, d := range saved {
		if d.ID == c.ID {
			copy := d
			view.Saved = &copy
		}
	}
	view.Catalog = m.cachedCatalog(c.ID)
	return view
}
func (m *Manager) ProviderSettings() ([]ProviderInfo, error) {
	saved, err := m.configuredProviders()
	if err != nil {
		return nil, err
	}
	out := []ProviderInfo{}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	for _, d := range providers.Defaults() {
		if _, _, err := providers.Effective(d.ID, saved); err != nil {
			return nil, err
		}
		v := m.settingsView(d, saved)
		if m.opt.ProviderMode == "mock" {
			v.Name += " · Mock"
			v.Available = true
			v.ExecutableFound = true
			v.VersionOK = true
			v.Status = "available"
			v.Version = "mock-v1"
			v.PermissionScope = "显式模拟测试"
		} else {
			applyProbe(&v, providers.Probe(ctx, v.Effective))
		}
		out = append(out, v)
	}
	return out, nil
}
func (m *Manager) SaveProviderSettings(input []providers.Config) ([]ProviderInfo, error) {
	m.providerMu.Lock()
	defer m.providerMu.Unlock()
	if len(input) != 4 {
		return nil, fmt.Errorf("需要同时保存四家 CLI 的配置")
	}
	settings := model.CLISettings{Version: 1, Providers: []model.CLIProviderSetting{}}
	seen := map[string]bool{}
	for _, c := range input {
		c.Model = strings.TrimSpace(c.Model)
		c.Reasoning = strings.TrimSpace(c.Reasoning)
		base, _, err := providers.Resolve(c.ID, nil)
		if err != nil {
			return nil, err
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("重复的 CLI 配置：%s", c.ID)
		}
		seen[c.ID] = true
		if c.Command != "" && c.Command != base.Command {
			return nil, fmt.Errorf("此页面不允许修改 CLI 可执行文件路径")
		}
		effective, _, err := providers.Effective(c.ID, []providers.Config{c})
		if err != nil {
			return nil, err
		}
		validateCapabilities := providers.IsEnabled(c) && providers.IsEnabled(effective)
		catalog := m.cachedCatalog(c.ID)
		if validateCapabilities && m.opt.ProviderMode == "cli" && c.Reasoning != "" && catalog != nil {
			probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			info := providers.Probe(probeCtx, c)
			cancel()
			catalog = providers.CatalogForVersion(catalog, info.Version)
		}
		validate := providers.ValidateSyntax(c)
		if validateCapabilities {
			validate = providers.Validate(c, catalog)
		}
		if validate != nil {
			return nil, validate
		}
		settings.Providers = append(settings.Providers, model.CLIProviderSetting{ID: c.ID, Enabled: c.Enabled, Model: c.Model, Reasoning: c.Reasoning})
	}
	if err := m.store.SaveProviderSettings(context.Background(), settings); err != nil {
		return nil, err
	}
	return m.ProviderSettings()
}
func (m *Manager) ResetProviderSettings() ([]ProviderInfo, error) {
	m.providerMu.Lock()
	defer m.providerMu.Unlock()
	if err := m.store.ResetProviderSettings(context.Background()); err != nil {
		return nil, err
	}
	return m.ProviderSettings()
}
func (m *Manager) ReloadProviderCatalog(ctx context.Context, id string) (providers.Catalog, error) {
	c, _, err := providers.Effective(id, nil)
	if err != nil {
		return providers.Catalog{}, err
	}
	m.providerMu.Lock()
	if job := m.catalogs[id]; job != nil {
		m.providerMu.Unlock()
		select {
		case <-ctx.Done():
			return providers.Catalog{}, ctx.Err()
		case <-job.done:
			return job.result, job.err
		}
	}
	job := &catalogJob{done: make(chan struct{})}
	m.catalogs[id] = job
	m.providerMu.Unlock()
	defer func() { m.providerMu.Lock(); close(job.done); delete(m.catalogs, id); m.providerMu.Unlock() }()
	if m.opt.ProviderMode == "mock" {
		// An explicitly selected test substitute; never used by the CLI application.
		job.result = providers.Catalog{ID: id, Models: []providers.ModelOption{{ID: "mock", Label: "Mock 测试模型", ReasoningMode: "unsupported", ReasoningValues: []string{}, Verified: false}}, Source: "mock", ManualAllowed: true, LoadedAt: now(), CLIVersion: "mock-v1"}
	} else {
		job.result, job.err = m.opt.DiscoverModels(ctx, c)
	}
	if job.err != nil {
		if old := m.cachedCatalog(id); old != nil {
			old.Source = "cli-directory-cached"
			old.Error = job.result.Error
			job.result = *old
		}
	}
	body, _ := json.Marshal(job.result)
	if saveErr := m.store.SaveProviderCatalog(context.Background(), id, body); saveErr != nil && job.err == nil {
		job.err = saveErr
	}
	// Directory failure is data shown alongside manual entry, not a discarded RPC result.
	job.err = nil
	return job.result, nil
}
func (m *Manager) Providers() []ProviderInfo {
	views, err := m.ProviderSettings()
	if err != nil {
		return []ProviderInfo{{ID: "settings", Status: "error", Error: err.Error()}}
	}
	return views
}
func applyProbe(v *ProviderInfo, p providers.Info) {
	v.BinaryPath = p.Path
	v.Version = p.Version
	v.Available = p.Available
	v.ExecutableFound = p.Path != ""
	v.VersionOK = p.Available
	v.CLIType = p.CLIType
	v.Status = "available"
	v.Error = p.Error
	v.Catalog = providers.CatalogForVersion(v.Catalog, p.Version)
	if !p.Available {
		v.Status = "unavailable"
	}
}
func (m *Manager) CheckProvider(id string) (ProviderInfo, error) {
	return m.CheckProviderContext(context.Background(), id)
}
func (m *Manager) CheckProviderContext(ctx context.Context, id string) (ProviderInfo, error) {
	return m.CheckProviderPreviewContext(ctx, id, nil)
}
func (m *Manager) CheckProviderPreviewContext(ctx context.Context, id string, editing *providers.Config) (ProviderInfo, error) {
	saved, err := m.configuredProviders()
	if err != nil {
		return ProviderInfo{}, err
	}
	originalSaved := append([]providers.Config(nil), saved...)
	checked := "effective"
	if editing != nil {
		if editing.ID != "" && editing.ID != id {
			return ProviderInfo{}, fmt.Errorf("检查配置的 CLI ID 不匹配")
		}
		base, _, err := providers.Resolve(id, nil)
		if err != nil {
			return ProviderInfo{}, err
		}
		if editing.Command != "" && editing.Command != base.Command {
			return ProviderInfo{}, fmt.Errorf("不允许覆盖 CLI 可执行文件路径")
		}
		replacement := providers.Config{ID: id, Enabled: editing.Enabled, Model: strings.TrimSpace(editing.Model), Reasoning: strings.TrimSpace(editing.Reasoning)}
		found := false
		for i := range saved {
			if saved[i].ID == id {
				saved[i] = replacement
				found = true
			}
		}
		if !found {
			saved = append(saved, replacement)
		}
		checked = "editing"
	}
	c, _, err := providers.Effective(id, saved)
	if err != nil {
		return ProviderInfo{}, err
	}
	view := m.settingsView(c, originalSaved)
	view.CheckedConfiguration = checked
	view.RequestedModel = c.Model
	view.RequestedReasoning = c.Reasoning
	if m.opt.ProviderMode == "mock" {
		view.Available = true
		view.VersionOK = true
		view.ExecutableFound = true
		view.Version = "mock-v1"
	} else {
		applyProbe(&view, providers.Probe(ctx, c))
	}
	if err := providers.Validate(c, providers.CatalogForVersion(m.cachedCatalog(id), view.Version)); err != nil {
		view.Status = "error"
		view.Error = err.Error()
		return view, nil
	}
	hash, _ := json.Marshal(c)
	m.providerMu.Lock()
	if job := m.providerChecks[id]; job != nil {
		m.providerMu.Unlock()
		if job.config != string(hash) {
			return ProviderInfo{}, fmt.Errorf("%s 正在检查另一组配置，请等待当前检查完成", id)
		}
		select {
		case <-ctx.Done():
			return ProviderInfo{}, ctx.Err()
		case <-job.done:
			return job.result, job.err
		}
	}
	job := &providerCheckJob{config: string(hash), done: make(chan struct{})}
	m.providerChecks[id] = job
	m.providerMu.Unlock()
	defer func() {
		m.providerMu.Lock()
		job.result = view
		close(job.done)
		delete(m.providerChecks, id)
		m.providerMu.Unlock()
	}()
	if m.opt.ProviderMode == "mock" {
		view.Name += " · Mock"
		view.Status = "ready"
		view.Available = true
		view.ExecutableFound = true
		view.VersionOK = true
		view.RequestOK = true
		view.RequestAttempted = true
		view.ModelSource = "mock"
		view.LastCheckedAt = now()
		return view, nil
	}
	result, checkErr := m.opt.CheckCLI(ctx, c)
	applyProbe(&view, result.Provider)
	view.LastCheckedAt = now()
	view.LatencyMS = result.LatencyMS
	view.RequestAttempted = result.RequestAttempted
	view.RequestedModel = result.RequestedModel
	view.ReportedModel = result.ActualModel
	view.RequestedReasoning = c.Reasoning
	view.ReportedReasoning = result.ActualReasoning
	view.ModelSource = result.ModelSource
	view.Usage = result.Usage
	view.Cost = result.Cost
	if checkErr != nil {
		view.Status = "error"
		view.Error = checkErr.Error()
	} else {
		view.Status = "ready"
		view.Error = ""
		view.RequestOK = true
	}
	return view, nil
}
func (m *Manager) freezeProviders() ([]model.ProviderConfig, error) {
	out := []model.ProviderConfig{}
	saved, err := m.configuredProviders()
	if err != nil {
		return nil, err
	}
	enabled := []providers.Config{}
	sourceByID := map[string]providers.Sources{}
	for _, d := range providers.Defaults() {
		c, sources, err := providers.Effective(d.ID, saved)
		if err != nil {
			return nil, err
		}
		if providers.IsEnabled(c) {
			enabled = append(enabled, c)
			sourceByID[c.ID] = sources
		}
	}
	if len(enabled) < 3 {
		return nil, fmt.Errorf("新讨论至少需要启用 3 家 CLI（1 位提案者与至少 2 位独立评审）；当前启用 %d 家，请在 CLI 与连接检查中调整", len(enabled))
	}
	if m.opt.ProviderMode == "mock" {
		for _, c := range enabled {
			out = append(out, model.ProviderConfig{ID: c.ID, Enabled: providers.Bool(true), Command: c.Command, Model: c.Model, Reasoning: c.Reasoning, Sources: sourceByID[c.ID], Version: "mock-v1", Permission: "mock", Available: true})
		}
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	for _, c := range enabled {
		p := providers.Probe(ctx, c)
		if !p.Available {
			return nil, fmt.Errorf("启用的 CLI %s 不可用：%s；可先停用此家再新建讨论", c.ID, p.Error)
		}
		if err := providers.Validate(c, providers.CatalogForVersion(m.cachedCatalog(c.ID), p.Version)); err != nil {
			return nil, err
		}
		command := p.Path
		if command == "" {
			command = p.Command
		}
		out = append(out, model.ProviderConfig{ID: p.ID, Enabled: providers.Bool(true), Command: command, Model: c.Model, Reasoning: c.Reasoning, Sources: sourceByID[c.ID], Version: p.Version, Permission: providerPermission(p.ID), EnvironmentKeys: providers.EnvironmentKeys(p.ID), Available: p.Available})
	}
	return out, nil
}
func delegateConfigs(f model.FrozenConfig) []providers.Config {
	out := []providers.Config{}
	for _, p := range f.Providers {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		out = append(out, providers.Config{ID: p.ID, Enabled: p.Enabled, Command: p.Command, Model: p.Model, Reasoning: p.Reasoning})
	}
	return out
}
func providerPermission(id string) string {
	switch id {
	case "codex":
		return "read-only sandbox；禁止审批提权"
	case "claude":
		return "--tools 空集；隔离工作目录（非 OS sandbox）"
	case "grok":
		return "--tools 空集、禁 MCP 和网页搜索；隔离工作目录（非 OS sandbox）"
	case "agy":
		return "--mode plan、CLI sandbox；空工作目录（非 OS 级只读保证）"
	}
	return "未知"
}
func verifyProviderVersions(f model.FrozenConfig) error {
	if f.ProviderMode != "cli" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	for _, p := range f.Providers {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		info := providers.Probe(ctx, providers.Config{ID: p.ID, Command: p.Command, Model: p.Model})
		if !info.Available {
			return fmt.Errorf("冻结 CLI %s 不可用：%s", p.ID, info.Error)
		}
		if info.Version != p.Version {
			return fmt.Errorf("CLI %s 版本已由 %s 变为 %s，请创建关联讨论", p.ID, p.Version, info.Version)
		}
	}
	return nil
}

// frozenRolePlan 必须在首次启动时写入冻结配置；恢复时只读取该快照。
func frozenRolePlan(configs []model.ProviderConfig) (*model.RolePlan, error) {
	return workflow.NewRolePlan(configs)
}
