package delegate

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/suchasplus/til-consensus/consensus"
	tcruntime "github.com/suchasplus/til-consensus/runtime"
)

// EnvelopeSchema 在 Provider 边界校验桌面扩展，避免 DTO 映射丢弃未知字段或转换错误类型。
func EnvelopeSchema(task consensus.Task) map[string]any {
	str := func() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
	strings := map[string]any{"type": "array", "items": str(), "uniqueItems": true}
	review := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"id", "raisedBy", "severity", "status", "statement", "resolutionRefs", "exclusiveWith"},
		"properties": map[string]any{
			"id": str(), "claimId": str(), "invocationId": str(),
			"claimDraftIndex": map[string]any{"type": "integer", "minimum": 0},
			"draftStatement":  str(),
			"raisedBy":        str(), "statement": str(),
			"severity":       map[string]any{"type": "string", "enum": []string{"blocking", "major", "minor", "suggestion"}},
			"status":         map[string]any{"type": "string", "enum": []string{"open", "resolved", "dismissed"}},
			"resolutionRefs": strings, "exclusiveWith": strings,
		},
		"oneOf": []any{
			map[string]any{"required": []string{"claimId"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"invocationId"}}, map[string]any{"required": []string{"claimDraftIndex"}}, map[string]any{"required": []string{"draftStatement"}}}}},
			map[string]any{"required": []string{"invocationId", "claimDraftIndex", "draftStatement"}, "not": map[string]any{"required": []string{"claimId"}}},
		},
		"allOf": []any{map[string]any{
			"if":   map[string]any{"properties": map[string]any{"status": map[string]any{"const": "resolved"}}},
			"then": map[string]any{"properties": map[string]any{"resolutionRefs": map[string]any{"minItems": 1}}},
		}},
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object", "additionalProperties": false,
		"required": []string{"schemaVersion", "mock", "invocationId", "taskKind", "contextHash", "coreOutput", "desktopReview"},
		"properties": map[string]any{
			"schemaVersion": map[string]any{"const": 1}, "mock": map[string]any{"type": "boolean"},
			"invocationId": str(), "taskKind": map[string]any{"const": string(task.Kind())},
			"contextHash":   map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"},
			"coreOutput":    tcruntime.TaskOutputJSONSchema(task),
			"desktopReview": map[string]any{"type": "array", "items": review},
			"ballotBinding": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"proposalHash", "manifestHash"}, "properties": map[string]any{"proposalHash": str(), "manifestHash": str()}},
		},
	}
}

// ValidateEnvelope 直接校验 JSON，检查未知扩展字段、字符串数字和无效引用结构。
func ValidateEnvelope(task consensus.Task, raw []byte) (Envelope, consensus.TaskResult, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return Envelope{}, nil, fmt.Errorf("decode desktop envelope: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	// 转成编译器接收的 JSON 原生数组和对象类型。
	schemaBytes, err := json.Marshal(EnvelopeSchema(task))
	if err != nil {
		return Envelope{}, nil, err
	}
	var schemaValue any
	if err := json.Unmarshal(schemaBytes, &schemaValue); err != nil {
		return Envelope{}, nil, err
	}
	if err := compiler.AddResource("urn:tcview:desktop-envelope", schemaValue); err != nil {
		return Envelope{}, nil, fmt.Errorf("load envelope schema: %w", err)
	}
	schema, err := compiler.Compile("urn:tcview:desktop-envelope")
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("compile envelope schema: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return Envelope{}, nil, fmt.Errorf("validate desktop envelope: %w", err)
	}
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Envelope{}, nil, err
	}
	result, err := tcruntime.StrictDecodeTaskOutput(task, envelope.CoreOutput)
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("decode consensus core: %w", err)
	}
	if err := validateReferences(task, envelope); err != nil {
		return Envelope{}, nil, err
	}
	return envelope, result, nil
}

func validateReferences(task consensus.Task, envelope Envelope) error {
	ids := taskClaimIDs(task)
	seen := make(map[string]bool)
	drafts := draftClaims(task, envelope.CoreOutput)
	for _, review := range envelope.DesktopReview {
		if seen[review.ID] {
			return fmt.Errorf("duplicate desktop review id %q", review.ID)
		}
		seen[review.ID] = true
		if review.RaisedBy != task.Meta().AgentID {
			return fmt.Errorf("review raisedBy must match dispatched agent")
		}
		if review.ClaimID != "" && !ids[review.ClaimID] {
			return fmt.Errorf("review references unknown claim %q", review.ClaimID)
		}
		if review.ClaimDraftIndex != nil && (review.InvocationID != envelope.InvocationID || *review.ClaimDraftIndex >= len(drafts)) {
			return fmt.Errorf("review references unknown invocation draft")
		}
		if review.ClaimDraftIndex != nil && review.DraftStatement != drafts[*review.ClaimDraftIndex]["statement"] {
			return fmt.Errorf("review draftStatement must match referenced coreOutput claim")
		}
		for _, other := range review.ExclusiveWith {
			if other == review.ClaimID || !ids[other] {
				return fmt.Errorf("exclusiveWith references invalid claim %q", other)
			}
		}
	}
	return nil
}

func draftClaims(task consensus.Task, core map[string]any) []map[string]any {
	if task.Kind() == consensus.TaskKindDebateRound {
		return objects(core["newClaims"])
	}
	if task.Kind() == consensus.TaskKindSynthesis {
		if claim, ok := core["claim"].(map[string]any); ok {
			return []map[string]any{claim}
		}
	}
	return objects(core["claims"])
}
