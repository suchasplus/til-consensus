package preparation

import (
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func sampleMaterials(t *testing.T) []model.Material {
	t.Helper()
	materials, err := Normalize("制定迁移方案", []model.MaterialInput{{ID: "spec", Title: "预算说明", Content: "\uFEFF预算不得超过 100 万元。\r\n\n", Source: "pasted"}})
	if err != nil {
		t.Fatal(err)
	}
	return materials
}

func TestNormalizePreservesUTF8BytesAndBothFingerprints(t *testing.T) {
	raw := "\uFEFF 预算不得超过 100 万元。\r\n\n"
	input := model.MaterialInput{ID: "spec", Title: "原始说明", Content: raw, Source: "file", RawSHA256: hashBytes([]byte(raw))}
	materials, err := Normalize("分析材料", []model.MaterialInput{input})
	if err != nil {
		t.Fatal(err)
	}
	m := materials[0]
	if m.Content != raw || m.Normalization != "none-utf8" || m.ByteLength != len(raw) || m.SHA256 != input.RawSHA256 || m.RawSHA256 != input.RawSHA256 {
		t.Fatalf("材料被改写或 hash 不实：%#v", m)
	}
	pasted := sampleMaterials(t)[0]
	if pasted.RawSHA256 != pasted.SHA256 {
		t.Fatal("粘贴材料没有成对实算指纹")
	}
}

func TestNormalizeRejectsUnsafeOrOversizedMaterialInputs(t *testing.T) {
	base := model.MaterialInput{ID: "spec", Title: "说明", Content: "材料", Source: "pasted"}
	cases := map[string]func() (string, []model.MaterialInput){
		"duplicate": func() (string, []model.MaterialInput) { return "目标", []model.MaterialInput{base, base} },
		"path-id": func() (string, []model.MaterialInput) {
			v := base
			v.ID = "../secret"
			return "目标", []model.MaterialInput{v}
		},
		"source-path": func() (string, []model.MaterialInput) {
			v := base
			v.Source = "/private/secret"
			return "目标", []model.MaterialInput{v}
		},
		"invalid-utf8": func() (string, []model.MaterialInput) {
			v := base
			v.Content = string([]byte{0xff})
			return "目标", []model.MaterialInput{v}
		},
		"empty": func() (string, []model.MaterialInput) {
			v := base
			v.Content = ""
			return "目标", []model.MaterialInput{v}
		},
		"bom-only": func() (string, []model.MaterialInput) {
			v := base
			v.Content = " \uFEFF\r\n\uFEFF "
			return "目标", []model.MaterialInput{v}
		},
		"nul": func() (string, []model.MaterialInput) {
			v := base
			v.Content = "text\x00binary"
			return "目标", []model.MaterialInput{v}
		},
		"long-title": func() (string, []model.MaterialInput) {
			v := base
			v.Title = strings.Repeat("长", 161)
			return "目标", []model.MaterialInput{v}
		},
		"large-file": func() (string, []model.MaterialInput) {
			v := base
			v.Content = strings.Repeat("a", MaxMaterialBytes+1)
			return "目标", []model.MaterialInput{v}
		},
		"large-total": func() (string, []model.MaterialInput) {
			v := base
			v.Content = strings.Repeat("a", MaxMaterialBytes)
			w := v
			w.ID = "second"
			return "目标", []model.MaterialInput{v, w}
		},
		"too-many": func() (string, []model.MaterialInput) {
			v := make([]model.MaterialInput, 9)
			for i := range v {
				v[i] = base
				v[i].ID = string(rune('a' + i))
			}
			return "目标", v
		},
		"file-no-sha": func() (string, []model.MaterialInput) {
			v := base
			v.Source = "file"
			return "目标", []model.MaterialInput{v}
		},
		"file-wrong-sha": func() (string, []model.MaterialInput) {
			v := base
			v.Source = "file"
			v.RawSHA256 = strings.Repeat("0", 64)
			return "目标", []model.MaterialInput{v}
		},
		"long-goal": func() (string, []model.MaterialInput) {
			return strings.Repeat("目", MaxGoalRunes+1), []model.MaterialInput{base}
		},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			goal, materials := input()
			if _, err := Normalize(goal, materials); err == nil {
				t.Fatal("接受无效材料输入")
			}
		})
	}
	base.Content = strings.Repeat("a", MaxMaterialBytes)
	if _, err := Normalize(strings.Repeat("目", MaxGoalRunes), []model.MaterialInput{base}); err != nil {
		t.Fatal("合法边界被拒绝", err)
	}
}

func TestInputHashBindsFullMaterialSnapshot(t *testing.T) {
	materials := sampleMaterials(t)
	original := HashInput("目标", materials)
	if original != HashInput("目标", materials) || HashInput("目标", nil) != HashInput("目标", []model.Material{}) {
		t.Fatal("输入指纹不稳定")
	}
	for _, mutate := range []func(*model.Material){func(m *model.Material) { m.Content += " " }, func(m *model.Material) { m.Title = "不同标题" }, func(m *model.Material) { m.Source = "file" }, func(m *model.Material) { m.SHA256 = "wrong" }} {
		copy := append([]model.Material(nil), materials...)
		mutate(&copy[0])
		if HashInput("目标", copy) == original {
			t.Fatal("原文、来源或 hash 改变未生成新指纹")
		}
	}
}
