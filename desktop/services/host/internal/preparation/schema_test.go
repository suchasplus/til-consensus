package preparation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func needsFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/needs.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func fixtureObject(t *testing.T) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(needsFixture(t)), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func classification(value map[string]any) map[string]any {
	return value["classification"].(map[string]any)
}
func analysisObject(value map[string]any) map[string]any {
	return value["materialAnalysis"].(map[string]any)
}

func TestJointSchemaAndSemanticReferences(t *testing.T) {
	materials := sampleMaterials(t)
	c, a, err := ValidateOutput(needsFixture(t), materials)
	if err != nil {
		t.Fatal(err)
	}
	if c.Recommendation != "needs_clarification" || len(c.MissingInformation) != 1 || a.Materials[0].KeyPoints[0].Quote != "预算不得超过 100 万元。" {
		t.Fatal("丢失实际任务／材料分析")
	}
	mutations := map[string]func(map[string]any){
		"empty-needs":         func(v map[string]any) { classification(v)["missingInformation"] = []any{} },
		"non-needs-estimated": func(v map[string]any) { classification(v)["recommendation"] = "not_suitable" },
		"concrete-missing": func(v map[string]any) {
			c := classification(v)
			c["recommendation"] = "free_debate"
			c["estimatedModeAfterClarification"] = ""
			c["estimatedModeReason"] = ""
		},
		"confidence-string":        func(v map[string]any) { classification(v)["confidence"] = "0.8" },
		"unknown-core-key":         func(v map[string]any) { classification(v)["raw"] = "private" },
		"unknown-outer-key":        func(v map[string]any) { v["extra"] = true },
		"missing-classifier-field": func(v map[string]any) { delete(classification(v), "why") },
		"omitted-summary":          func(v map[string]any) { analysisObject(v)["materials"] = []any{} },
		"unknown-material": func(v map[string]any) {
			analysisObject(v)["materials"].([]any)[0].(map[string]any)["materialId"] = "unknown"
		},
		"invented-quote": func(v map[string]any) {
			analysisObject(v)["materials"].([]any)[0].(map[string]any)["keyPoints"].([]any)[0].(map[string]any)["quote"] = "原文没有的预算数据"
		},
		"unknown-gap-ref": func(v map[string]any) {
			analysisObject(v)["gaps"].([]any)[0].(map[string]any)["materialIds"] = []any{"unknown"}
		},
		"duplicate-gap-ref": func(v map[string]any) {
			analysisObject(v)["gaps"].([]any)[0].(map[string]any)["materialIds"] = []any{"spec", "spec"}
		},
		"bad-importance": func(v map[string]any) {
			analysisObject(v)["gaps"].([]any)[0].(map[string]any)["importance"] = "optional"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			v := fixtureObject(t)
			mutate(v)
			raw, _ := json.Marshal(v)
			if _, _, err := ValidateOutput(string(raw), materials); err == nil {
				t.Fatal("接受无效联合分析")
			}
		})
	}
}

func TestOnlyOneStrictObjectAndNoMaterialMeansEmptyCoverage(t *testing.T) {
	for _, raw := range []string{needsFixture(t) + "\n{}", "[" + needsFixture(t) + "]", needsFixture(t) + " trailing", strings.Replace(needsFixture(t), `"confidence": 0.82`, `"confidence": 0.81,"confidence": 0.82`, 1)} {
		if _, _, err := ValidateOutput(raw, sampleMaterials(t)); err == nil {
			t.Fatal("接受重复字段、多个值或尾随内容")
		}
	}
	empty := mockOutput("目标", []model.Material{})
	raw, _ := json.Marshal(empty)
	if _, _, err := ValidateOutput(string(raw), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateOutput(needsFixture(t), nil); err == nil {
		t.Fatal("无材料却接受了虚构的材料摘要")
	}
	schema, _ := json.Marshal(OutputSchema(sampleMaterials(t)))
	for _, unsupported := range []string{`"$schema"`, `"uniqueItems"`, `"oneOf"`, `"allOf"`} {
		if strings.Contains(string(schema), unsupported) {
			t.Fatal("原生传输含不支持的schema关键字")
		}
	}
}

func TestGeminiNativeProjectionPreservesEmptyEnumBranchLocally(t *testing.T) {
	materials := sampleMaterials(t)
	for _, cliType := range []string{config.CLITypeAntigravity, config.CLITypeGemini} {
		native := NativeOutputSchema(materials, cliType)
		classification := native["properties"].(map[string]any)["classification"].(map[string]any)["properties"].(map[string]any)
		estimated := classification["estimatedModeAfterClarification"].(map[string]any)
		if _, exists := estimated["enum"]; exists {
			t.Fatal("Gemini原生仍包含空串enum")
		}
		if estimated["type"] != "string" {
			t.Fatal("原生投影删除了合法空串的字符串类型")
		}
	}
	strict := OutputSchema(materials)["properties"].(map[string]any)["classification"].(map[string]any)["properties"].(map[string]any)["estimatedModeAfterClarification"].(map[string]any)
	if strict["enum"].([]string)[0] != "" {
		t.Fatal("修改了本地完整til枚举")
	}
	valid := fixtureObject(t)
	c := classification(valid)
	c["recommendation"] = "free_debate"
	c["missingInformation"] = []any{}
	c["estimatedModeAfterClarification"] = ""
	c["estimatedModeReason"] = ""
	raw, _ := json.Marshal(valid)
	if _, _, err := ValidateOutput(string(raw), materials); err != nil {
		t.Fatal("原生兼容修复丢掉合法空串分支", err)
	}
	c["recommendation"] = "needs_clarification"
	c["missingInformation"] = []any{"补充期限"}
	c["estimatedModeAfterClarification"] = "invented-mode"
	c["estimatedModeReason"] = "reason"
	raw, _ = json.Marshal(valid)
	if _, _, err := ValidateOutput(string(raw), materials); err == nil {
		t.Fatal("原生宽化导致本地接受未知模式")
	}
	for _, cliType := range []string{config.CLITypeCodex, config.CLITypeClaude, config.CLITypeGrok} {
		schema := NativeOutputSchema(materials, cliType)["properties"].(map[string]any)["classification"].(map[string]any)["properties"].(map[string]any)["estimatedModeAfterClarification"].(map[string]any)
		if _, ok := schema["enum"]; !ok {
			t.Fatal("无依据地降级了其他CLI枚举约束")
		}
	}
}
