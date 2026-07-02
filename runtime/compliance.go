package runtime

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

type complianceStatus string

const (
	complianceStatusStrict     complianceStatus = "strict"
	complianceStatusNormalized complianceStatus = "normalized"
	complianceStatusRepaired   complianceStatus = "repaired"
	complianceStatusFailed     complianceStatus = "failed"
)

type complianceTelemetry struct {
	StrictCompliant      bool
	StrictError          error
	NormalizedWithoutFix bool
	RepairAttempted      bool
	RepairSucceeded      bool
	FinalStatus          complianceStatus
	RawArtifact          *consensus.ArtifactRef
	InitialErrorArtifact *consensus.ArtifactRef
	FinalArtifact        *consensus.ArtifactRef
	FinalError           error
}

type complianceSummaryEntry struct {
	Provider      string             `json:"provider"`
	ProviderType  string             `json:"providerType"`
	ProviderModel string             `json:"providerModel"`
	TaskKind      consensus.TaskKind `json:"taskKind"`
	Total         int                `json:"total"`
	Strict        int                `json:"strict"`
	Normalized    int                `json:"normalized"`
	Repaired      int                `json:"repaired"`
	Failed        int                `json:"failed"`
}

func (d *Delegate) persistComplianceTelemetry(taskID string, task consensus.Task, agent ResolvedAgentRuntime, telemetry complianceTelemetry) (*consensus.ArtifactRef, error) {
	if strings.TrimSpace(d.artifactDir) == "" {
		return nil, nil
	}
	reportArtifact, err := d.persistComplianceReportArtifact(taskID, task, agent, telemetry)
	if err != nil {
		return nil, err
	}
	summaryArtifact, summaryErr := d.persistComplianceSummaryArtifact(task, agent, telemetry)
	if summaryErr != nil {
		return reportArtifact, summaryErr
	}
	return chooseArtifact(reportArtifact, summaryArtifact), nil
}

func (d *Delegate) persistComplianceReportArtifact(taskID string, task consensus.Task, agent ResolvedAgentRuntime, telemetry complianceTelemetry) (*consensus.ArtifactRef, error) {
	payload := map[string]any{
		"version": 1,
		"agent": map[string]any{
			"id":            agent.ID,
			"role":          agent.Role,
			"provider":      agent.ProviderName,
			"providerType":  agent.Provider.Type,
			"providerModel": agent.ProviderModel,
		},
		"task": map[string]any{
			"taskId":    taskID,
			"kind":      task.Kind(),
			"requestId": task.Meta().RequestID,
			"sessionId": task.Meta().SessionID,
			"agentId":   task.Meta().AgentID,
		},
		"compliance": map[string]any{
			"strictCompliant":      telemetry.StrictCompliant,
			"normalizedWithoutFix": telemetry.NormalizedWithoutFix,
			"repairAttempted":      telemetry.RepairAttempted,
			"repairSucceeded":      telemetry.RepairSucceeded,
			"finalStatus":          telemetry.FinalStatus,
		},
	}
	compliance := payload["compliance"].(map[string]any)
	if telemetry.StrictError != nil {
		compliance["strictError"] = telemetry.StrictError.Error()
	}
	if telemetry.RawArtifact != nil {
		compliance["rawArtifact"] = telemetry.RawArtifact
	}
	if telemetry.InitialErrorArtifact != nil {
		compliance["initialErrorArtifact"] = telemetry.InitialErrorArtifact
	}
	if telemetry.FinalArtifact != nil {
		compliance["finalArtifact"] = telemetry.FinalArtifact
	}
	if telemetry.FinalError != nil {
		compliance["finalError"] = telemetry.FinalError.Error()
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal compliance report: %w", err)
	}
	body = append(body, '\n')
	filename := filepath.Join(d.artifactDir, buildComplianceReportFilename(task, taskID))
	return writeArtifact(filename, body, "application/json")
}

func (d *Delegate) persistComplianceSummaryArtifact(task consensus.Task, agent ResolvedAgentRuntime, telemetry complianceTelemetry) (*consensus.ArtifactRef, error) {
	d.mu.Lock()
	key := strings.Join([]string{
		agent.ProviderName,
		string(agent.Provider.Type),
		agent.ProviderModel,
		string(task.Kind()),
	}, "|")

	entry := d.compliance[key]
	if entry == nil {
		entry = &complianceSummaryEntry{
			Provider:      agent.ProviderName,
			ProviderType:  string(agent.Provider.Type),
			ProviderModel: agent.ProviderModel,
			TaskKind:      task.Kind(),
		}
		d.compliance[key] = entry
	}
	entry.Total++
	switch telemetry.FinalStatus {
	case complianceStatusStrict:
		entry.Strict++
	case complianceStatusNormalized:
		entry.Normalized++
	case complianceStatusRepaired:
		entry.Repaired++
	case complianceStatusFailed:
		entry.Failed++
	}

	snapshot := make([]complianceSummaryEntry, 0, len(d.compliance))
	for _, item := range d.compliance {
		snapshot = append(snapshot, *item)
	}
	sort.Slice(snapshot, func(i, j int) bool {
		if snapshot[i].Provider != snapshot[j].Provider {
			return snapshot[i].Provider < snapshot[j].Provider
		}
		if snapshot[i].ProviderModel != snapshot[j].ProviderModel {
			return snapshot[i].ProviderModel < snapshot[j].ProviderModel
		}
		return snapshot[i].TaskKind < snapshot[j].TaskKind
	})
	payload := map[string]any{
		"version":     1,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"entries":     snapshot,
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		d.mu.Unlock()
		return nil, fmt.Errorf("marshal compliance summary: %w", err)
	}
	body = append(body, '\n')
	filename := filepath.Join(d.artifactDir, "strict-compliance-summary.json")
	artifact, writeErr := writeArtifact(filename, body, "application/json")
	d.mu.Unlock()
	return artifact, writeErr
}

func buildComplianceReportFilename(task consensus.Task, taskID string) string {
	safeAgent := sanitizeFilename(task.Meta().AgentID)
	return fmt.Sprintf("compliance-report-%s-%s-%s.json", safeAgent, task.Kind(), sanitizeFilename(taskID))
}
