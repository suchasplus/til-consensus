package delegate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/codexserver"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	tcruntime "github.com/suchasplus/til-consensus/runtime"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

func (d *Delegate) runCLI(ctx context.Context, id string, job *entry) {
	started := time.Now().UTC()
	artifact := map[string]any{"schemaVersion": 2, "mock": false, "invocationId": id, "taskKind": job.task.Kind(), "contextHash": job.contextHash, "input": json.RawMessage(job.input), "complete": false, "status": "running", "startedAt": started}
	var info providers.Info
	var actualModel string
	var requestedModel, reportedModel, requestedReasoning, reportedReasoning string
	modelSource := "unreported"
	var usage map[string]any
	var cost *float64
	emit := func(kind, text, status string, complete bool) error {
		if d.options.OnProgress == nil {
			return nil
		}
		return d.options.OnProgress(ProgressEvent{RunID: d.options.RunID, InvocationID: id, AgentID: job.task.Meta().AgentID, CLIType: info.CLIType, Phase: phase(job.task), Kind: kind, TaskKind: string(job.task.Kind()), Text: text, Delta: text, Status: status, Model: actualModel, RequestedModel: requestedModel, ActualModel: reportedModel, RequestedReasoning: requestedReasoning, ReportedReasoning: reportedReasoning, ModelSource: modelSource, CLIVersion: info.Version, Usage: usage, Round: round(job.task), Complete: complete})
	}
	finish := func(failure error) {
		status := "failed"
		if ctx.Err() != nil || errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
			status = "cancelled"
		}
		artifact["status"], artifact["complete"] = status, false
		artifact["finishedAt"], artifact["durationMs"] = time.Now().UTC(), time.Since(started).Milliseconds()
		artifact["error"] = failure.Error()
		ref, persistErr := d.persist(id+".output.json", artifact)
		job.result = consensus.AwaitedTask{Error: failure.Error(), Artifact: ref}
		if persistErr != nil {
			job.result.Error += "; persist failure: " + persistErr.Error()
		}
		_ = emit("failed", "", status, true)
	}
	// 输入先落盘，因此缺少可执行文件、认证失败或原生schema失败也有
	// 独立 invocation 记录，不能看起来像从未派发过。
	if _, err := d.persist(id+".input.json", artifact); err != nil {
		d.stopAfterPersistenceFailure(err)
		finish(err)
		return
	}
	providerConfig, resolvedInfo, err := providers.Resolve(job.task.Meta().AgentID, d.options.Providers)
	if err != nil {
		finish(err)
		return
	}
	info = providers.Probe(ctx, providerConfig)
	if !info.Available {
		artifact["source"] = info
		finish(errors.New(info.Error))
		return
	}
	// Model 只在 CLI 明确选择或报告后记录；绝不猜测用户的默认配置。
	actualModel = providerConfig.Model
	requestedModel = providerConfig.Model
	requestedReasoning = providerConfig.Reasoning
	if actualModel != "" {
		modelSource = "explicit-request"
	}
	artifact["source"] = info
	artifact["requestedModel"] = providerConfig.Model
	artifact["requestedReasoning"] = providerConfig.Reasoning
	artifact["cost"] = nil
	workdir := ""
	if d.options.ArtifactDir != "" {
		workdir = filepath.Join(d.options.ArtifactDir, safeName(d.options.AttemptID), id, "workdir")
		err = os.MkdirAll(workdir, 0700)
	} else {
		workdir, err = os.MkdirTemp("", "tcview-cli-task-*")
		if err == nil {
			defer os.RemoveAll(workdir)
		}
	}
	if err != nil {
		finish(err)
		return
	}
	workdir, err = filepath.Abs(workdir)
	if err != nil {
		finish(err)
		return
	}
	provider := providers.Runtime(providerConfig, info, workdir)
	agent := tcruntime.ResolvedAgentRuntime{AgentConfig: config.AgentConfig{ID: job.task.Meta().AgentID, Role: job.task.Meta().Role, SystemPrompt: "使用简体中文完成任务。仅讨论和评审提供的任务内容，不调用工具、不读取文件、不执行外部操作；工作目录为空是预期情况。"}, Provider: provider, ProviderModel: providerConfig.Model}
	binding, err := expectedBallot(job.task)
	if err != nil {
		finish(err)
		return
	}
	schema := nativeEnvelopeSchema(job.task, agent, id, job.contextHash, binding)
	prompt := buildDesktopPrompt(job.task, agent, schema, id, job.contextHash)
	artifact["prompt"], artifact["outputSchema"] = prompt, schema
	artifact["validationSchema"] = EnvelopeSchema(job.task)
	artifact["workdir"] = workdir
	if _, err := d.persist(id+".input.json", artifact); err != nil {
		finish(err)
		return
	}
	if err := emit("started", "", "running", false); err != nil {
		d.stopAfterPersistenceFailure(err)
		finish(err)
		return
	}
	publicEvents := make([]clirunner.CLIEvent, 0)
	var stream answerStream
	progress := func(event clirunner.CLIEvent) error {
		if event.Model != "" {
			actualModel = event.Model
			reportedModel = event.Model
			modelSource = "cli-report"
		}
		if event.Usage != nil {
			usage = event.Usage
		}
		publicEvents = append(publicEvents, event)
		// CLI 的正文是结构化 envelope；界面只逐字展示公开 summary，
		// 不把 JSON 键、哈希、schema 或推理事件展示为聊天内容。
		delta := stream.consume(event)
		if err := emit(event.Type, delta, event.Status, false); err != nil {
			d.stopAfterPersistenceFailure(err)
			return err
		}
		return nil
	}
	if d.options.OnBeforeCall != nil {
		if err := d.options.OnBeforeCall(id, job.task.Meta().AgentID); err != nil {
			finish(err)
			return
		}
	}
	result, err := runProvider(ctx, job.task, provider, providerConfig, prompt, schema, progress)
	artifact["command"] = result.Command
	artifact["rawOutput"] = providers.RedactSensitive(result.RawOutput)
	artifact["publicEvents"] = publicEvents
	if strings.TrimSpace(result.RawOutput) != "" {
		model, rawUsage, rawCost := outputMetadata(result.RawOutput)
		if model != "" {
			actualModel = model
			reportedModel = model
			modelSource = "cli-report"
		}
		if rawUsage != nil {
			usage = rawUsage
		}
		cost = rawCost
	}
	reportedReasoning = providers.ReportedReasoning(result.RawOutput)
	artifact["reportedReasoning"] = reportedReasoning
	artifact["model"], artifact["actualModel"], artifact["usage"], artifact["cost"] = actualModel, reportedModel, usage, cost
	artifact["modelSource"] = modelSource
	if err != nil {
		var commandError *clirunner.CommandError
		if errors.As(err, &commandError) {
			artifact["rawOutput"], artifact["stderr"], artifact["exitCode"] = providers.RedactSensitive(commandError.Stdout), providers.RedactSensitive(commandError.Stderr), commandError.ExitCode
		}
		finish(errors.New(providers.SafeError(resolvedInfo.CLIType, err)))
		return
	}
	if requestedReasoning != "" && reportedReasoning != "" && requestedReasoning != reportedReasoning {
		finish(fmt.Errorf("%s CLI 报告推理强度 %s 与冻结请求 %s 不一致", info.CLIType, reportedReasoning, requestedReasoning))
		return
	}
	artifact["normalizedOutput"] = result.Output
	envelope, output, err := ValidateEnvelope(job.task, []byte(result.Output))
	if err == nil {
		err = validateInvocation(job.task, envelope, id, job.contextHash, binding)
	}
	if err != nil {
		finish(fmt.Errorf("%s CLI structured output: %w", info.CLIType, err))
		return
	}
	if err := ctx.Err(); err != nil {
		finish(err)
		return
	}
	artifact["status"], artifact["complete"] = "completed", true
	artifact["finishedAt"], artifact["durationMs"] = time.Now().UTC(), time.Since(started).Milliseconds()
	ref, err := d.persist(id+".output.json", artifact)
	if err != nil {
		finish(err)
		return
	}
	message := Message{AgentID: job.task.Meta().AgentID, Role: job.task.Meta().Role, Label: agentLabel(job.task.Meta().AgentID), Content: messageContent(job.task, envelope.CoreOutput), Phase: phase(job.task), Kind: string(job.task.Kind()), InvocationID: id, ContextHash: job.contextHash, CLIType: info.CLIType, CLIVersion: info.Version, Model: actualModel, RequestedModel: requestedModel, ActualModel: reportedModel, RequestedReasoning: requestedReasoning, ReportedReasoning: reportedReasoning, ModelSource: modelSource, Usage: usage, Cost: cost, Complete: true, Round: round(job.task)}
	if claim, ok := envelope.CoreOutput["claim"].(map[string]any); ok {
		message.ProposalContent = firstString(claim, "statement")
	}
	if binding != nil {
		message.ProposalHash, message.ManifestHash = binding.ProposalHash, binding.ManifestHash
	}
	if ref != nil {
		message.ArtifactPath = ref.Path
	}
	if !job.silent {
		if err := d.publish(ctx, message, envelope.DesktopReview); err != nil {
			d.stopAfterPersistenceFailure(err)
			finish(err)
			return
		}
		if binding != nil && d.options.OnBallot != nil {
			if err := d.options.OnBallot(Ballot{InvocationID: id, AgentID: job.task.Meta().AgentID, ProposalHash: binding.ProposalHash, ManifestHash: binding.ManifestHash, Valid: true}); err != nil {
				d.stopAfterPersistenceFailure(err)
				finish(err)
				return
			}
		}
	}
	if err := emit("completed", "", "completed", true); err != nil {
		d.stopAfterPersistenceFailure(err)
		finish(err)
		return
	}
	job.result = consensus.AwaitedTask{OK: true, Output: output, Artifact: ref}
}

func runProvider(ctx context.Context, task consensus.Task, provider config.ProviderConfig, settings providers.Config, prompt string, schema map[string]any, progress func(clirunner.CLIEvent) error) (clirunner.RunTaskResult, error) {
	if provider.CLIType == config.CLITypeCodex {
		environment := clirunner.BuildEnvironment(clirunner.SnapshotHostEnvironment(os.Environ()), provider, task, task.Meta().AgentID, task.Meta().Role, settings.Model)
		result, err := codexserver.Run(ctx, codexserver.Options{Command: provider.Command, Cwd: provider.Workdir, Model: settings.Model, Reasoning: settings.Reasoning, Environment: environment.Values, OnEvent: progress}, prompt, schema)
		return clirunner.RunTaskResult{Output: result.Output, RawOutput: result.RawOutput, Command: result.Command}, err
	}
	runner := clirunner.NewRunnerWithOptions(provider, clirunner.RunnerOptions{OnEvent: progress})
	return runner.RunTaskDetailed(ctx, task, prompt, task.Meta().AgentID, task.Meta().Role, settings.Model, settings.Reasoning, nil, schema)
}

func buildDesktopPrompt(task consensus.Task, agent tcruntime.ResolvedAgentRuntime, schema map[string]any, id, hash string) string {
	base := tcruntime.BuildTaskPrompt(task, agent, false)
	schemaJSON, _ := json.Marshal(schema)
	return base + "\n\nDesktop transport extension (authoritative outer response):\n" +
		"The task-specific output rules above describe coreOutput ONLY. Your entire response must use the following desktop envelope schema, which includes the complete coreOutput schema.\n" +
		"Set schemaVersion=1, mock=false, invocationId=" + id + ", contextHash=" + hash + ". Put the genuine task result in coreOutput.\n" +
		"desktopReview contains only concrete disagreements or implementation risks; [] is correct when no such issue exists. Never invent disagreements.\n" +
		"resolutionRefs and exclusiveWith must contain unique entries. The complete local validator still enforces uniqueness, mutually exclusive references, and evidence for resolved issues even when these constraints cannot be represented in the provider's native schema subset.\n" +
		"A review must reference an existing task claimId, OR the exact new draft using invocationId + zero-based claimDraftIndex + verbatim draftStatement. Drafts are coreOutput.claims for proposals, coreOutput.newClaims for debate rounds, and the single coreOutput.claim at index 0 for synthesis. raisedBy must match the dispatched agentId. Use unique review ids. resolved requires nonempty resolutionRefs containing real supporting references.\n" +
		"When ballotBinding is present, echo both frozen hashes exactly and assess only the supplied frozen claims. Do not reinterpret the manifest.\n" +
		"Do not inspect files or call tools. Return JSON only. Use Simplified Chinese for user-facing strings.\n" + string(schemaJSON)
}

func expectedBallot(task consensus.Task) (*BallotBinding, error) {
	if task.Kind() != consensus.TaskKindFinalVote {
		return nil, nil
	}
	metadata := task.Meta().Metadata
	proposalHash, _ := metadata["proposalHash"].(string)
	manifestHash, _ := metadata["ballotManifestHash"].(string)
	if proposalHash == "" || manifestHash == "" {
		return nil, errors.New("final vote requires a frozen proposalHash and ballotManifestHash")
	}
	return &BallotBinding{ProposalHash: proposalHash, ManifestHash: manifestHash}, nil
}

func validateInvocation(task consensus.Task, envelope Envelope, id, hash string, binding *BallotBinding) error {
	if envelope.Mock {
		return errors.New("real CLI returned mock=true")
	}
	if envelope.InvocationID != id || envelope.ContextHash != hash || envelope.TaskKind != string(task.Kind()) {
		return errors.New("response does not match frozen invocation context")
	}
	if binding != nil && (envelope.BallotBinding == nil || *envelope.BallotBinding != *binding) {
		return errors.New("ballot does not match frozen proposal and manifest")
	}
	if binding == nil && envelope.BallotBinding != nil {
		return errors.New("non-vote task returned ballotBinding")
	}
	if vote, ok := task.(consensus.FinalVoteTask); ok {
		expected := make(map[string]bool, len(vote.Claims))
		for _, claim := range vote.Claims {
			expected[claim.ClaimID] = true
		}
		seen := make(map[string]bool, len(expected))
		for _, row := range objects(envelope.CoreOutput["votes"]) {
			claimID := firstString(row, "claimId")
			if !expected[claimID] || seen[claimID] {
				return fmt.Errorf("invalid or duplicate ballot claim %q", claimID)
			}
			seen[claimID] = true
		}
		if len(seen) != len(expected) {
			return errors.New("ballot does not cover the frozen claim manifest")
		}
	}
	return nil
}

func nativeEnvelopeSchema(task consensus.Task, agent tcruntime.ResolvedAgentRuntime, id, hash string, binding *BallotBinding) map[string]any {
	schema := EnvelopeSchema(task)
	properties := schema["properties"].(map[string]any)
	properties["schemaVersion"] = map[string]any{"const": 1, "type": "integer"}
	properties["taskKind"] = map[string]any{"const": string(task.Kind()), "type": "string"}
	properties["mock"] = map[string]any{"const": false, "type": "boolean"}
	properties["invocationId"] = map[string]any{"const": id, "type": "string"}
	properties["contextHash"] = map[string]any{"const": hash, "type": "string"}
	properties["coreOutput"] = tcruntime.TaskOutputJSONSchemaForAgent(task, agent)
	required := schema["required"].([]string)
	if binding != nil {
		properties["ballotBinding"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"proposalHash", "manifestHash"}, "properties": map[string]any{"proposalHash": map[string]any{"type": "string", "const": binding.ProposalHash}, "manifestHash": map[string]any{"type": "string", "const": binding.ManifestHash}}}
		required = append(required, "ballotBinding")
	} else {
		delete(properties, "ballotBinding")
	}
	schema["required"] = required
	removeDialectDeclarations(schema)
	if agent.Provider.CLIType == config.CLITypeCodex {
		// OpenAI structured output requires every declared property and does not
		// accept the local validator's oneOf/not/if reference rules. Two closed
		// anyOf branches preserve the exact mutually exclusive reference shapes.
		review := properties["desktopReview"].(map[string]any)["items"].(map[string]any)
		all := review["properties"].(map[string]any)
		branch := func(keys []string) map[string]any {
			props := make(map[string]any)
			for _, key := range keys {
				props[key] = all[key]
			}
			return map[string]any{"type": "object", "additionalProperties": false, "required": keys, "properties": props}
		}
		common := []string{"id", "raisedBy", "severity", "status", "statement", "resolutionRefs", "exclusiveWith"}
		properties["desktopReview"] = map[string]any{"type": "array", "items": map[string]any{"anyOf": []any{branch(append(append([]string{}, common...), "claimId")), branch(append(append([]string{}, common...), "invocationId", "claimDraftIndex", "draftStatement"))}}}
		codexTransportSchema(schema)
	}
	return schema
}

// Native providers choose their own JSON Schema dialect. In particular,
// Claude's current AJV loader rejects a draft/2020-12 $schema reference.
// Keep that declaration solely in the authoritative local validation schema.
func removeDialectDeclarations(schema map[string]any) {
	delete(schema, "$schema")
	for _, key := range []string{"properties", "$defs", "definitions"} {
		if children, ok := schema[key].(map[string]any); ok {
			for _, value := range children {
				if child, ok := value.(map[string]any); ok {
					removeDialectDeclarations(child)
				}
			}
		}
	}
	for _, key := range []string{"items", "not", "if", "then", "else"} {
		if child, ok := schema[key].(map[string]any); ok {
			removeDialectDeclarations(child)
		}
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if children, ok := schema[key].([]any); ok {
			for _, value := range children {
				if child, ok := value.(map[string]any); ok {
					removeDialectDeclarations(child)
				}
			}
		}
	}
}

// codexTransportSchema adapts only the transport copy to the official strict
// Structured Outputs subset. The authoritative EnvelopeSchema is persisted
// separately and always checked locally before a result is published. In
// particular, native generation cannot enforce uniqueItems (observed 400),
// while the local validator still rejects duplicate review references.
// https://developers.openai.com/api/docs/guides/structured-outputs#supported-schemas
func codexTransportSchema(schema map[string]any) {
	for _, key := range []string{"$schema", "uniqueItems", "oneOf", "allOf", "not", "dependentRequired", "dependentSchemas", "if", "then", "else"} {
		delete(schema, key)
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		required := make([]string, 0, len(properties))
		for name, value := range properties {
			required = append(required, name)
			if child, ok := value.(map[string]any); ok {
				codexTransportSchema(child)
			}
		}
		sort.Strings(required)
		schema["required"], schema["additionalProperties"] = required, false
	}
	if items, ok := schema["items"].(map[string]any); ok {
		codexTransportSchema(items)
	}
	if alternatives, ok := schema["anyOf"].([]any); ok {
		for _, value := range alternatives {
			if child, ok := value.(map[string]any); ok {
				codexTransportSchema(child)
			}
		}
	}
	if definitions, ok := schema["$defs"].(map[string]any); ok {
		for _, value := range definitions {
			if child, ok := value.(map[string]any); ok {
				codexTransportSchema(child)
			}
		}
	}
}

func outputMetadata(raw string) (string, map[string]any, *float64) {
	return providers.OutputMetadata(raw)
}
