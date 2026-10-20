package delegate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/runtime/mock"
)

type Delegate struct {
	options    Options
	mu         sync.Mutex
	callbackMu sync.Mutex
	tasks      map[string]*entry
	seq        uint64
	fatal      error
}

type entry struct {
	task        consensus.Task
	cancel      context.CancelFunc
	done        chan struct{}
	result      consensus.AwaitedTask
	input       []byte
	contextHash string
	silent      bool
}

func New(options Options) *Delegate {
	if options.Mode == "" {
		options.Mode = "cli"
	}
	if options.AttemptID == "" {
		options.AttemptID = "attempt"
		if options.Mode == "mock" {
			options.AttemptID = "mock"
		}
	}
	if options.Delay < 0 {
		options.Delay = 0
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}
	return &Delegate{options: options, tasks: make(map[string]*entry)}
}

func (d *Delegate) Dispatch(ctx context.Context, task consensus.Task) (consensus.DispatchReceipt, error) {
	return d.dispatch(ctx, task, false)
}

func (d *Delegate) dispatch(ctx context.Context, task consensus.Task, silent bool) (consensus.DispatchReceipt, error) {
	if err := ctx.Err(); err != nil {
		return consensus.DispatchReceipt{}, err
	}
	if task == nil || task.Meta().AgentID == "" {
		return consensus.DispatchReceipt{}, errors.New("task requires agentId")
	}
	if len(d.options.Providers) > 0 {
		allowed := false
		for _, p := range d.options.Providers {
			if p.ID == task.Meta().AgentID && providers.IsEnabled(p) {
				allowed = true
				break
			}
		}
		if !allowed {
			return consensus.DispatchReceipt{}, fmt.Errorf("CLI %s 未包含在冻结的启用配置中", task.Meta().AgentID)
		}
	}
	if d.options.Mode != "cli" && d.options.Mode != "mock" {
		return consensus.DispatchReceipt{}, fmt.Errorf("unknown delegate mode %q", d.options.Mode)
	}
	input, err := json.Marshal(task)
	if err != nil {
		return consensus.DispatchReceipt{}, fmt.Errorf("freeze task context: %w", err)
	}
	// 同时复制切片和元数据，确保独立派发的参与者不会看到其他参与者的运行中修改。
	frozen := reflect.New(reflect.TypeOf(task))
	if err := json.Unmarshal(input, frozen.Interface()); err != nil {
		return consensus.DispatchReceipt{}, err
	}
	task = frozen.Elem().Interface().(consensus.Task)
	hash := sha256.Sum256(input)
	d.mu.Lock()
	if d.fatal != nil {
		err := d.fatal
		d.mu.Unlock()
		return consensus.DispatchReceipt{}, err
	}
	d.seq++
	id := fmt.Sprintf("%s-inv-%04d", safeName(d.options.AttemptID), d.seq)
	taskCtx, cancel := context.WithTimeout(ctx, d.options.Timeout)
	job := &entry{task: task, cancel: cancel, done: make(chan struct{}), input: input, contextHash: hex.EncodeToString(hash[:]), silent: silent}
	d.tasks[id] = job
	d.mu.Unlock()
	go d.run(taskCtx, id, job)
	return consensus.DispatchReceipt{TaskID: id, AgentID: task.Meta().AgentID, Kind: task.Kind()}, nil
}

func (d *Delegate) Await(ctx context.Context, taskID string, timeout time.Duration) (consensus.AwaitedTask, error) {
	d.mu.Lock()
	job := d.tasks[taskID]
	d.mu.Unlock()
	if job == nil {
		return consensus.AwaitedTask{Error: "unknown_task_id:" + taskID}, nil
	}
	if timeout <= 0 {
		timeout = time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return consensus.AwaitedTask{}, ctx.Err()
	case <-timer.C:
		job.cancel()
		return consensus.AwaitedTask{Error: "timeout"}, nil
	case <-job.done:
		return job.result, nil
	}
}

func (d *Delegate) Cancel(_ context.Context, taskID string) error {
	d.mu.Lock()
	job := d.tasks[taskID]
	d.mu.Unlock()
	if job == nil {
		return fmt.Errorf("unknown task %q", taskID)
	}
	job.cancel()
	return nil
}

func (d *Delegate) CancelAll() {
	d.mu.Lock()
	jobs := make([]*entry, 0, len(d.tasks))
	for _, job := range d.tasks {
		jobs = append(jobs, job)
	}
	d.mu.Unlock()
	for _, job := range jobs {
		job.cancel()
	}
}

// Close 取消全部已派发任务，并等待审计产物落盘。
func (d *Delegate) Close() error {
	d.mu.Lock()
	if d.fatal == nil {
		d.fatal = errors.New("delegate closed")
	}
	d.mu.Unlock()
	d.CancelAll()
	return d.AwaitFinished(context.Background())
}

func (d *Delegate) Wait() { _ = d.AwaitFinished(context.Background()) }

func (d *Delegate) run(ctx context.Context, id string, job *entry) {
	defer close(job.done)
	defer job.cancel()
	if d.options.Mode == "cli" {
		d.runCLI(ctx, id, job)
		return
	}
	artifact := map[string]any{"schemaVersion": 1, "mock": true, "invocationId": id, "taskKind": job.task.Kind(), "contextHash": job.contextHash, "input": json.RawMessage(job.input), "complete": false, "status": "running"}
	inputRef, err := d.persist(id+".input.json", artifact)
	if err != nil {
		job.result.Error = err.Error()
		return
	}
	_ = inputRef
	finishFailure := func(failure error) {
		artifact["status"] = "failed"
		artifact["complete"] = false
		if errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
			artifact["status"] = "cancelled"
		}
		artifact["error"] = failure.Error()
		ref, persistErr := d.persist(id+".output.json", artifact)
		job.result = consensus.AwaitedTask{Error: failure.Error(), Artifact: ref}
		if persistErr != nil {
			job.result.Error += "; persist failure: " + persistErr.Error()
		}
	}
	if err := waitDelay(ctx, d.options.Delay); err != nil {
		finishFailure(err)
		return
	}
	if d.options.OnBeforeCall != nil {
		if err := d.options.OnBeforeCall(id, job.task.Meta().AgentID); err != nil {
			finishFailure(err)
			return
		}
	}
	raw, err := mock.RunTask(ctx, job.task, config.AgentConfig{ID: job.task.Meta().AgentID}, config.ProviderConfig{})
	if err != nil {
		finishFailure(err)
		return
	}
	artifact["mockRuntimeOutput"] = raw
	core, err := localizeCore(job.task, raw)
	if err != nil {
		finishFailure(err)
		return
	}
	envelope := Envelope{SchemaVersion: 1, Mock: true, InvocationID: id, TaskKind: string(job.task.Kind()), ContextHash: job.contextHash, CoreOutput: core, DesktopReview: buildReviews(job.task, id, core)}
	if binding, err := expectedBallot(job.task); err == nil {
		envelope.BallotBinding = binding
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		finishFailure(err)
		return
	}
	// 即使校验失败也保留 Provider 输出封装原文。
	artifact["rawOutput"] = json.RawMessage(payload)
	validated, result, err := ValidateEnvelope(job.task, payload)
	if err != nil {
		finishFailure(err)
		return
	}
	if err := ctx.Err(); err != nil {
		finishFailure(err)
		return
	}
	artifact["status"], artifact["complete"] = "completed", true
	ref, err := d.persist(id+".output.json", artifact)
	if err != nil {
		finishFailure(err)
		return
	}
	message := Message{AgentID: job.task.Meta().AgentID, Role: job.task.Meta().Role, Label: agentLabel(job.task.Meta().AgentID) + " · Mock", Content: messageContent(job.task, core), Phase: phase(job.task), Kind: string(job.task.Kind()), InvocationID: id, ContextHash: job.contextHash, Complete: true, Round: round(job.task)}
	if ref != nil {
		message.ArtifactPath = ref.Path
	}
	if !job.silent {
		if err := d.publish(ctx, message, validated.DesktopReview); err != nil {
			d.stopAfterPersistenceFailure(err)
			finishFailure(err)
			return
		}
		if envelope.BallotBinding != nil && d.options.OnBallot != nil {
			if err := d.options.OnBallot(Ballot{InvocationID: id, AgentID: job.task.Meta().AgentID, ProposalHash: envelope.BallotBinding.ProposalHash, ManifestHash: envelope.BallotBinding.ManifestHash, Valid: true}); err != nil {
				finishFailure(err)
				return
			}
		}
	}
	job.result = consensus.AwaitedTask{OK: true, Output: result, Artifact: ref}
}

func (d *Delegate) publish(ctx context.Context, message Message, reviews []Review) error {
	d.callbackMu.Lock()
	defer d.callbackMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.options.OnMessage != nil {
		if err := d.options.OnMessage(message); err != nil {
			return fmt.Errorf("persist desktop message: %w", err)
		}
	}
	for _, review := range reviews {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.options.OnReview != nil {
			if err := d.options.OnReview(review); err != nil {
				return fmt.Errorf("persist desktop review: %w", err)
			}
		}
	}
	return nil
}

func (d *Delegate) stopAfterPersistenceFailure(err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	d.mu.Lock()
	if d.fatal == nil {
		d.fatal = err
	}
	d.mu.Unlock()
	d.CancelAll()
}

func (d *Delegate) persist(name string, value any) (*consensus.ArtifactRef, error) {
	if d.options.ArtifactDir == "" {
		return nil, nil
	}
	dir := filepath.Join(d.options.ArtifactDir, safeName(d.options.AttemptID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	data = providers.RedactJSON(data)
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".cli-artifact-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	return &consensus.ArtifactRef{Path: path, Hash: hex.EncodeToString(hash[:]), MediaType: "application/json"}, nil
}

// ProposeP0 不触发桌面回调。宿主将返回的 P0 正文、摘要和消息放入同一 SQLite 事务。
func (d *Delegate) ProposeP0(ctx context.Context, goal string) (string, error) {
	return d.ProposeP0WithMaterials(ctx, goal, nil)
}

func (d *Delegate) ProposeP0WithMaterials(ctx context.Context, goal string, materials []consensus.MaterialRef) (string, error) {
	return d.ProposeP0For(ctx, "codex", goal, materials)
}

// ProposeP0For 保留旧入口兼容；新讨论使用宿主已冻结的提案者。
func (d *Delegate) ProposeP0For(ctx context.Context, proposer, goal string, materials []consensus.MaterialRef) (string, error) {
	if proposer == "" {
		return "", errors.New("P0 requires a frozen proposer")
	}
	task := consensus.ProposalTask{TaskMeta: consensus.TaskMeta{AgentID: proposer, Role: "proposer"}, TaskSpec: consensus.TaskSpec{Goal: goal, Materials: materials}, MaxClaims: 2}
	receipt, err := d.dispatch(ctx, task, true)
	if err != nil {
		return "", err
	}
	output, err := d.Await(ctx, receipt.TaskID, d.options.Timeout)
	if err != nil {
		return "", err
	}
	if !output.OK {
		return "", errors.New(output.Error)
	}
	proposal := output.Output.(consensus.ProposalTaskResult).Output
	var text strings.Builder
	label := ""
	if d.options.Mode == "mock" {
		label = " · Mock"
	}
	fmt.Fprintf(&text, "## 初始方案 P0%s\n\n目标：%s\n\n%s\n", label, goal, proposal.Summary)
	for i, claim := range proposal.Claims {
		fmt.Fprintf(&text, "\n%d. **%s**：%s\n", i+1, claim.Title, claim.Statement)
	}
	if d.options.Mode == "mock" {
		text.WriteString("\n待评审：正确性、替代路线、异常与实施成本。当前内容是演示数据，尚未调用真实 CLI。")
	}
	return text.String(), nil
}

func waitDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func safeName(s string) string {
	var out strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "attempt"
	}
	return out.String()
}
