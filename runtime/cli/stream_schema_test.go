package cli

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
)

func TestStreamingSchemaIsEnforced(t *testing.T) {
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"answer"}, "properties": map[string]any{"answer": map[string]any{"type": "integer"}}}
	for _, invalid := range []string{`done`, `{}`, `{"answer":"7"}`, `{"answer":7,"extra":true}`} {
		if err := validateStreamingSchema(schema, invalid); err == nil {
			t.Fatalf("invalid output passed schema: %s", invalid)
		}
	}
	if err := validateStreamingSchema(schema, `{"answer":7}`); err != nil {
		t.Fatal(err)
	}
}

func TestStreamingSchemaNormalizesNestedGoContainersWithoutWeakeningValidation(t *testing.T) {
	// 对应桌面评审的两种互斥引用：已有 claim，或本次 invocation 的草稿。
	// enum/required 使用 []string，组合关键字还包含 []map[string]any。
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"desktopReview"},
		"properties": map[string]any{
			"desktopReview": map[string]any{
				"type": "array", "minItems": 1,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"severity", "status"},
					"properties": map[string]any{
						"severity":        map[string]any{"type": "string", "enum": []string{"blocking", "major", "minor"}},
						"status":          map[string]any{"type": "string", "enum": []string{"open", "resolved"}},
						"claimId":         map[string]any{"type": "string", "minLength": 1},
						"invocationId":    map[string]any{"type": "string", "minLength": 1},
						"claimDraftIndex": map[string]any{"type": "integer", "minimum": 0},
						"draftStatement":  map[string]any{"type": "string", "minLength": 1},
					},
					"oneOf": []map[string]any{
						{
							"required": []string{"claimId"},
							"not": map[string]any{"anyOf": []map[string]any{
								{"required": []string{"invocationId"}},
								{"required": []string{"claimDraftIndex"}},
								{"required": []string{"draftStatement"}},
							}},
						},
						{
							"required": []string{"invocationId", "claimDraftIndex", "draftStatement"},
							"not":      map[string]any{"required": []string{"claimId"}},
						},
					},
				},
			},
		},
	}
	before, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, valid := range []string{
		`{"desktopReview":[{"severity":"blocking","status":"open","claimId":"claim-1"}]}`,
		`{"desktopReview":[{"severity":"minor","status":"resolved","invocationId":"inv-1","claimDraftIndex":0,"draftStatement":"需要补充证据"}]}`,
	} {
		if err := validateStreamingSchema(schema, valid); err != nil {
			t.Fatalf("valid review rejected after JSON normalization: %v", err)
		}
	}
	for _, invalid := range []string{
		`{}`,
		`{"desktopReview":[{"severity":"critical","status":"open","claimId":"claim-1"}]}`,
		`{"desktopReview":[{"severity":"major","claimId":"claim-1"}]}`,
		`{"desktopReview":[{"severity":"major","status":"open"}]}`,
		`{"desktopReview":[{"severity":"major","status":"open","claimId":"claim-1","invocationId":"inv-1","claimDraftIndex":0,"draftStatement":"冲突引用"}]}`,
		`{"desktopReview":[{"severity":"major","status":"open","invocationId":"inv-1","claimDraftIndex":0}]}`,
		`{"desktopReview":[{"severity":"major","status":"open","invocationId":"inv-1","claimDraftIndex":"0","draftStatement":"错误类型"}]}`,
	} {
		if err := validateStreamingSchema(schema, invalid); err == nil || !strings.Contains(err.Error(), "violates output schema") {
			t.Fatalf("review constraints were weakened: output=%s err=%v", invalid, err)
		}
	}
	after, err := json.Marshal(schema)
	if err != nil || string(after) != string(before) {
		t.Fatalf("normalization modified caller schema: err=%v", err)
	}
	if _, ok := schema["required"].([]string); !ok {
		t.Fatal("normalization replaced the caller's typed slice")
	}
}

func TestStreamingSchemaPreservesIntegerPrecision(t *testing.T) {
	schema := map[string]any{"type": "integer", "const": uint64(9007199254740993)}
	if err := validateStreamingSchema(schema, `9007199254740993`); err != nil {
		t.Fatalf("exact integer rejected: %v", err)
	}
	if err := validateStreamingSchema(schema, `9007199254740992`); err == nil {
		t.Fatal("neighboring integer passed after losing JSON number precision")
	}
}

func TestStreamingSchemaInvalidInputsReturnErrors(t *testing.T) {
	cycle := map[string]any{}
	cycle["properties"] = cycle
	for _, test := range []struct {
		name   string
		schema map[string]any
		output string
		want   string
	}{
		{"nil schema", nil, `{}`, "expected a JSON object"},
		{"unsupported Go value", map[string]any{"enum": []any{make(chan int)}}, `{}`, "encode JSON"},
		{"cyclic Go map", cycle, `{}`, "encode JSON"},
		{"nonfinite number", map[string]any{"minimum": math.NaN()}, `1`, "encode JSON"},
		{"malformed schema", map[string]any{"type": "unknown-type"}, `{}`, "CLI output schema"},
		{"invalid JSON", map[string]any{"type": "object"}, `done`, "must return JSON"},
		{"multiple JSON documents", map[string]any{"type": "object"}, `{} {}`, "must return JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 输入错误必须返回 error；任何 panic 都会使本项回归失败。
			err := validateStreamingSchema(test.schema, test.output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestGrokStreamingSchemaPromptLimitFailsBeforeExecution(t *testing.T) {
	runner := NewRunner(config.ProviderConfig{CLIType: config.CLITypeGrok, Command: "must-not-execute", OutputFormat: "stream-json"})
	_, err := runner.RunTaskDetailed(context.Background(), realPromptTransportTestTask{}, strings.Repeat("x", maxCLIPromptBytes-20), "agent", "reviewer", "", "", nil, map[string]any{"type": "object"})
	if err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("expected bounded validation error, got %v", err)
	}
}

func TestManagedTransportAndReadOnlyCannotBeOverridden(t *testing.T) {
	for _, provider := range []config.ProviderConfig{
		{CLIType: config.CLITypeAntigravity, Args: []string{"--conversation=old"}},
		{CLIType: config.CLITypeAntigravity, Args: []string{"-c"}},
		{CLIType: config.CLITypeGrok, Args: []string{"--output-format=plain"}},
		{CLIType: config.CLITypeClaude, PermissionMode: "read-only", Args: []string{"--dangerously-skip-permissions"}},
		{CLIType: config.CLITypeCodex, PermissionMode: "read-only", Args: []string{"-c", "sandbox_mode=\"danger-full-access\""}},
		{CLIType: config.CLITypeCodex, PermissionMode: "read-only", Args: []string{"-csandbox_mode=\"danger-full-access\""}},
		{CLIType: config.CLITypeCodex, Workdir: "/private/run", Args: []string{"--cd", "/other"}},
	} {
		if err := validateOneShotArgs(provider); err == nil {
			t.Fatalf("managed execution silently overridden: %+v", provider)
		}
	}
}
