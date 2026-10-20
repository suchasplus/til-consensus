package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/providers"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/runmanager"
)

const MaxFrameBytes = 4 * 1024 * 1024

type Server struct {
	context   context.Context
	cancel    context.CancelFunc
	pending   sync.WaitGroup
	manager   *runmanager.Manager
	output    chan any
	done      chan struct{}
	writeDone chan struct{}
	once      sync.Once
}
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(writer io.Writer) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{context: ctx, cancel: cancel, output: make(chan any, 256), done: make(chan struct{}), writeDone: make(chan struct{})}
	go func() {
		defer close(s.writeDone)
		encoder := json.NewEncoder(writer)
		for {
			select {
			case <-s.done:
				for {
					select {
					case v := <-s.output:
						if err := encoder.Encode(v); err != nil {
							return
						}
					default:
						return
					}
				}
			case v := <-s.output:
				if err := encoder.Encode(v); err != nil {
					log.Printf("桌面协议输出已断开: %v", err)
					return
				}
			}
		}
	}()
	return s
}
func (s *Server) Bind(manager *runmanager.Manager) { s.manager = manager }

// Notify never blocks the engine. Missing notifications are replayable via run.events.
func (s *Server) Notify(event model.DesktopEvent) {
	select {
	case <-s.done:
		return
	default:
	}
	select {
	case s.output <- map[string]any{"jsonrpc": "2.0", "method": "event", "params": event}:
	default:
	}
}
func (s *Server) Close() {
	s.once.Do(func() { s.cancel(); s.pending.Wait(); close(s.done) })
	<-s.writeDone
}

// CancelPending stops RPC-owned model checks and catalog readers before the
// manager closes its store. Long-running discussions are closed by the manager.
func (s *Server) CancelPending() { s.cancel() }
func (s *Server) Serve(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), MaxFrameBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if json.Valid(line) {
				s.respond(nil, nil, &rpcError{-32600, "需要 JSON-RPC 请求对象"})
			} else {
				s.respond(nil, nil, &rpcError{-32700, "无效 JSON"})
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" || !validID(req.ID) {
			s.respond(nil, nil, &rpcError{-32600, "需要 JSON-RPC 2.0 请求和字符串或数字 id"})
			continue
		}

		if req.Method == "provider.check" || req.Method == "provider.models" || req.Method == "topic.analyze" || req.Method == "provider.list" || req.Method == "provider.probe" || req.Method == "provider.settings.get" || req.Method == "provider.settings.save" || req.Method == "provider.settings.reset" {
			// Detach the request bytes before Scanner reuses its input buffer.
			cloned := req
			cloned.ID = append(json.RawMessage(nil), req.ID...)
			cloned.Params = append(json.RawMessage(nil), req.Params...)
			s.pending.Add(1)
			go func(request request) { defer s.pending.Done(); s.handle(request) }(cloned)
		} else {
			s.handle(req)
		}
	}
	return scanner.Err()
}
func (s *Server) handle(req request) {
	result, err := s.dispatch(req)
	if err != nil {
		s.respond(req.ID, nil, err)
	} else {
		s.respond(req.ID, result, nil)
	}
}
func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return false
	}
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}
func (s *Server) respond(id json.RawMessage, result any, err *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if err != nil {
		response["error"] = err
	} else {
		response["result"] = result
	}
	select {
	case s.output <- response:
	case <-s.writeDone:
	case <-s.done:
	}
}
func (s *Server) dispatch(req request) (any, *rpcError) {
	if s.manager == nil {
		return nil, &rpcError{-32000, "后台尚未初始化"}
	}
	var result any
	var err error
	switch req.Method {
	case "hello":
		return map[string]any{"protocolVersion": "1.4", "appVersion": "0.6.0", "engineVersion": model.EngineVersion, "capabilities": []string{"cli", "free_debate", "adjudication", "delphi", "durable_events", "checkpoint_resume", "attempt_isolation", "provider_probe", "proposal_history", "branch", "artifact_export", "streaming", "usage", "diagnostics_cleanup", "topic_analysis", "auto_mode", "materials", "provider_settings", "model_catalog", "reasoning_config", "provider_selection", "dynamic_roles", "recoverable_delete"}, "providerMode": s.manager.ProviderMode()}, nil
	case "provider.list", "provider.probe":
		result = s.manager.Providers()
	case "provider.check":
		var in struct {
			ProviderID string            `json:"providerId"`
			Config     *providers.Config `json:"config,omitempty"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.CheckProviderPreviewContext(s.context, in.ProviderID, in.Config)
	case "provider.settings.get":
		result, err = s.manager.ProviderSettings()
	case "provider.settings.save":
		var in struct {
			Configs []providers.Config `json:"configs"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.SaveProviderSettings(in.Configs)
	case "provider.settings.reset":
		result, err = s.manager.ResetProviderSettings()
	case "provider.models":
		var in struct {
			ProviderID string `json:"providerId"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.ReloadProviderCatalog(s.context, in.ProviderID)
	case "topic.analyze":
		var in model.PreparationInput
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.AnalyzeTopic(s.context, in)
	case "topic.get", "topic.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		if in.ID == "" {
			return nil, &rpcError{-32602, "缺少准备记录 id"}
		}
		if req.Method == "topic.get" {
			result, err = s.manager.GetPreparation(in.ID)
		} else {
			result, err = s.manager.CancelPreparation(in.ID)
		}
	case "artifact.export":
		var in struct {
			RunID  string `json:"runId"`
			Format string `json:"format"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.Export(in.RunID, in.Format)
	case "diagnostics.clear":
		var in struct {
			RunID string `json:"runId"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.ClearDiagnostics(in.RunID)
	case "run.branch":
		var in model.BranchRunInput
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.Branch(in)
	case "run.list":
		result, err = s.manager.List()
	case "run.trash":
		result, err = s.manager.ListDeleted()
	case "run.start":
		var in model.StartRunInput
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.Start(in)
	case "run.resume":
		var in model.ResumeRunInput
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		result, err = s.manager.Resume(in)
	case "run.get", "run.stop", "run.delete", "run.restore":
		var in struct {
			RunID string `json:"runId"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		if in.RunID == "" {
			return nil, &rpcError{-32602, "缺少 runId"}
		}
		switch req.Method {
		case "run.get":
			result, err = s.manager.Get(in.RunID)
		case "run.stop":
			result, err = s.manager.Stop(in.RunID)
		case "run.delete":
			result, err = s.manager.DeleteRun(in.RunID)
		case "run.restore":
			result, err = s.manager.RestoreRun(in.RunID)
		}
	case "run.events":
		var in struct {
			RunID        string `json:"runId"`
			AfterEventID int64  `json:"afterEventId"`
		}
		if e := params(req.Params, &in); e != nil {
			return nil, e
		}
		if in.RunID == "" {
			return nil, &rpcError{-32602, "缺少 runId"}
		}
		result, err = s.manager.Events(in.RunID, in.AfterEventID)
	default:
		return nil, &rpcError{-32601, "未知方法: " + req.Method}
	}
	if err != nil {
		return nil, &rpcError{-32000, err.Error()}
	}
	return result, nil
}
func params(raw json.RawMessage, target any) *rpcError {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return &rpcError{-32602, "缺少请求参数"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &rpcError{-32602, fmt.Sprintf("无效参数: %s", err)}
	}
	return nil
}
