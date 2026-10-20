package preparation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	tilrunner "github.com/suchasplus/til-consensus/runner"
)

type jointOutput struct {
	Classification   tilrunner.ClassifyResult `json:"classification"`
	MaterialAnalysis model.MaterialAnalysis   `json:"materialAnalysis"`
}

// OutputSchema 复用 til 的分类协议，外加桌面材料分析；使用各 CLI 支持的
// 原生 JSON Schema 子集，不携带 $schema/uniqueItems。语义约束仍本地校验。
func OutputSchema(materials []model.Material) map[string]any {
	text := func() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}
	ids := make([]string, 0, len(materials))
	for _, material := range materials {
		ids = append(ids, material.ID)
	}
	ref := text()
	if len(ids) > 0 {
		ref["enum"] = ids
	}
	refs := map[string]any{"type": "array", "items": ref}
	if len(ids) == 0 {
		refs["maxItems"] = 0
	}
	keyPoint := object(map[string]any{"quote": text(), "meaning": text()}, []string{"quote", "meaning"})
	summary := object(map[string]any{"materialId": ref, "summary": text(), "keyPoints": map[string]any{"type": "array", "items": keyPoint}}, []string{"materialId", "summary", "keyPoints"})
	gap := object(map[string]any{"question": text(), "importance": map[string]any{"type": "string", "enum": []string{"required", "helpful"}}, "materialIds": refs}, []string{"question", "importance", "materialIds"})
	analysis := object(map[string]any{"summary": text(), "materials": map[string]any{"type": "array", "items": summary, "minItems": len(materials), "maxItems": len(materials)}, "gaps": map[string]any{"type": "array", "items": gap}, "limitations": map[string]any{"type": "array", "items": text()}}, []string{"summary", "materials", "gaps", "limitations"})
	return object(map[string]any{"classification": tilrunner.ClassifyJSONSchema(), "materialAnalysis": analysis}, []string{"classification", "materialAnalysis"})
}

// NativeOutputSchema adapts only the native transport copy. Gemini's function
// schema rejects empty string enum members, including the legitimate empty
// estimatedModeAfterClarification branch in til's classifier. Widening that
// one native enum preserves the empty branch; OutputSchema and ValidateOutput
// continue to enforce the complete enum and its conditional semantics locally.
func NativeOutputSchema(materials []model.Material, cliType string) map[string]any {
	full := OutputSchema(materials)
	if cliType != config.CLITypeAntigravity && cliType != config.CLITypeGemini {
		return full
	}
	encoded, _ := json.Marshal(full)
	var native map[string]any
	_ = json.Unmarshal(encoded, &native)
	var project func(any)
	project = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if values, ok := typed["enum"].([]any); ok {
				for _, item := range values {
					if text, ok := item.(string); ok && text == "" {
						delete(typed, "enum")
						break
					}
				}
			}
			for _, child := range typed {
				project(child)
			}
		case []any:
			for _, child := range typed {
				project(child)
			}
		}
	}
	project(native)
	return native
}

// ValidateOutput 只接受一个对象、拒绝未知字段及重复 JSON 键，并在 til 解码后
// 检查任务路由语义、材料覆盖、引用与逐字引文。
func ValidateOutput(raw string, materials []model.Material) (tilrunner.ClassifyResult, model.MaterialAnalysis, error) {
	var classification tilrunner.ClassifyResult
	var analysis model.MaterialAnalysis
	if err := oneObject([]byte(raw)); err != nil {
		return classification, analysis, err
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return classification, analysis, err
	}
	schemaBytes, err := json.Marshal(OutputSchema(materials))
	if err != nil {
		return classification, analysis, err
	}
	var schemaValue any
	if err = json.Unmarshal(schemaBytes, &schemaValue); err != nil {
		return classification, analysis, err
	}
	compiler := jsonschema.NewCompiler()
	if err = compiler.AddResource("urn:tcview:preparation", schemaValue); err != nil {
		return classification, analysis, err
	}
	schema, err := compiler.Compile("urn:tcview:preparation")
	if err != nil {
		return classification, analysis, err
	}
	if err = schema.Validate(value); err != nil {
		return classification, analysis, fmt.Errorf("准备检查 JSON schema 校验失败：%w", err)
	}
	var outer struct {
		Classification   json.RawMessage        `json:"classification"`
		MaterialAnalysis model.MaterialAnalysis `json:"materialAnalysis"`
	}
	if err = json.Unmarshal([]byte(raw), &outer); err != nil {
		return classification, analysis, err
	}
	classification, err = tilrunner.DecodeClassifyOutput(string(outer.Classification))
	if err != nil {
		return classification, analysis, fmt.Errorf("任务分类无效：%w", err)
	}
	if classification.Recommendation == "needs_clarification" {
		if len(classification.MissingInformation) == 0 {
			return classification, analysis, fmt.Errorf("needs_clarification 必须列出 missingInformation")
		}
	} else if classification.EstimatedModeAfterClarification != "" || classification.EstimatedModeReason != "" {
		return classification, analysis, fmt.Errorf("非 needs_clarification 必须清空 estimatedMode 字段")
	}
	if isMode(classification.Recommendation) && len(classification.MissingInformation) > 0 {
		return classification, analysis, fmt.Errorf("具体讨论模式不能同时保留 missingInformation")
	}
	for _, text := range append(append([]string{classification.Summary}, classification.Why...), classification.MissingInformation...) {
		if strings.TrimSpace(text) == "" {
			return classification, analysis, fmt.Errorf("分类结果不能包含空白说明或缺失问题")
		}
	}
	analysis = outer.MaterialAnalysis
	if strings.TrimSpace(analysis.Summary) == "" {
		return classification, analysis, fmt.Errorf("材料分析摘要不能为空白")
	}
	lookup := make(map[string]model.Material, len(materials))
	for _, material := range materials {
		lookup[material.ID] = material
	}
	seen := map[string]bool{}
	for _, item := range analysis.Materials {
		material, ok := lookup[item.MaterialID]
		if !ok || seen[item.MaterialID] {
			return classification, analysis, fmt.Errorf("材料摘要引用未知或重复 ID：%s", item.MaterialID)
		}
		seen[item.MaterialID] = true
		if strings.TrimSpace(item.Summary) == "" {
			return classification, analysis, fmt.Errorf("材料 %s 摘要不能为空白", item.MaterialID)
		}
		for _, point := range item.KeyPoints {
			if strings.TrimSpace(point.Quote) == "" || strings.TrimSpace(point.Meaning) == "" || !strings.Contains(material.Content, point.Quote) {
				return classification, analysis, fmt.Errorf("材料 %s 的 quote 必须是非空原文子串，meaning 不能为空白", item.MaterialID)
			}
		}
	}
	if len(seen) != len(materials) {
		return classification, analysis, fmt.Errorf("每份材料必须恰有一项摘要")
	}
	for _, gap := range analysis.Gaps {
		if strings.TrimSpace(gap.Question) == "" {
			return classification, analysis, fmt.Errorf("材料缺口问题不能为空白")
		}
		refs := map[string]bool{}
		for _, id := range gap.MaterialIDs {
			if _, ok := lookup[id]; !ok || refs[id] {
				return classification, analysis, fmt.Errorf("材料缺口引用未知或重复 ID：%s", id)
			}
			refs[id] = true
		}
	}
	for _, limitation := range analysis.Limitations {
		if strings.TrimSpace(limitation) == "" {
			return classification, analysis, fmt.Errorf("材料分析限制不能为空白")
		}
	}
	classification.Raw = ""
	return classification, analysis, nil
}

func isMode(value string) bool {
	return value == "adjudication" || value == "free_debate" || value == "delphi"
}

func oneObject(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func(json.Token) error
	walk = func(token json.Token) error {
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON 对象字段必须是字符串")
				}
				if seen[key] {
					return fmt.Errorf("JSON 字段重复：%s", key)
				}
				seen[key] = true
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err = walk(value); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return fmt.Errorf("JSON 对象未完整结束")
			}
		case '[':
			for decoder.More() {
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err = walk(value); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return fmt.Errorf("JSON 数组未完整结束")
			}
		default:
			return fmt.Errorf("意外的 JSON 分隔符")
		}
		return nil
	}
	first, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("准备检查必须返回一个完整 JSON 对象：%w", err)
	}
	if first != json.Delim('{') {
		return fmt.Errorf("准备检查只能返回一个 JSON 对象")
	}
	if err = walk(first); err != nil {
		return fmt.Errorf("准备检查 JSON 无效：%w", err)
	}
	if _, err = decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("准备检查返回了多个 JSON 值")
		}
		return fmt.Errorf("准备检查包含尾随内容：%w", err)
	}
	return nil
}
