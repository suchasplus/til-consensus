// Package preparation 验证材料快照，并以一次隔离 CLI 请求分析任务准备情况。
package preparation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

const (
	MaxMaterials     = 8
	MaxMaterialBytes = 64 * 1024
	MaxInputBytes    = 128 * 1024
	MaxGoalRunes     = 6000
	Normalization    = "none-utf8"
)

var materialID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$`)

// Normalize 保留材料的 UTF-8 原文，包括 BOM、CRLF 和首尾空白，不截断。
func Normalize(goal string, input []model.MaterialInput) ([]model.Material, error) {
	if !utf8.ValidString(goal) || strings.TrimSpace(strings.ReplaceAll(goal, "\uFEFF", "")) == "" || strings.ContainsRune(goal, 0) || utf8.RuneCountInString(goal) > MaxGoalRunes {
		return nil, fmt.Errorf("目标必须是 1 至 %d 个字符的有效 UTF-8 文本", MaxGoalRunes)
	}
	if len(input) > MaxMaterials {
		return nil, fmt.Errorf("最多提供 %d 份材料", MaxMaterials)
	}
	total := len(goal)
	seen := make(map[string]bool, len(input))
	out := make([]model.Material, 0, len(input))
	for _, material := range input {
		if !materialID.MatchString(material.ID) || seen[material.ID] {
			return nil, fmt.Errorf("材料 ID 必须安全且唯一：%q", material.ID)
		}
		seen[material.ID] = true
		if !utf8.ValidString(material.Title) || strings.TrimSpace(material.Title) == "" || strings.ContainsRune(material.Title, 0) || utf8.RuneCountInString(material.Title) > 160 {
			return nil, fmt.Errorf("材料 %s 的标题必须是 1 至 160 个字符的 UTF-8 文本", material.ID)
		}
		if material.Source != "pasted" && material.Source != "file" {
			return nil, fmt.Errorf("材料 %s 的 source 只能是 pasted 或 file", material.ID)
		}
		if !utf8.ValidString(material.Content) || strings.ContainsRune(material.Content, 0) || strings.TrimSpace(strings.ReplaceAll(material.Content, "\uFEFF", "")) == "" {
			return nil, fmt.Errorf("材料 %s 必须是非空、无 NUL 的有效 UTF-8 文本", material.ID)
		}
		if len(material.Content) > MaxMaterialBytes {
			return nil, fmt.Errorf("材料 %s 超过 %d KiB，不能静默截断", material.ID, MaxMaterialBytes/1024)
		}
		total += len(material.Content) + len(material.Title)
		if total > MaxInputBytes {
			return nil, fmt.Errorf("目标与材料合计超过 %d KiB，不能静默截断", MaxInputBytes/1024)
		}
		hash := hashBytes([]byte(material.Content))
		if material.Source == "file" && material.RawSHA256 == "" {
			return nil, fmt.Errorf("文件材料 %s 必须提供 rawSha256", material.ID)
		}
		if material.RawSHA256 != "" && material.RawSHA256 != hash {
			return nil, fmt.Errorf("材料 %s 的 rawSha256 与 UTF-8 原文不一致", material.ID)
		}
		out = append(out, model.Material{ID: material.ID, Title: material.Title, Content: material.Content, Source: material.Source, RawSHA256: hash, SHA256: hash, ByteLength: len(material.Content), Normalization: Normalization})
	}
	return out, nil
}

// HashInput 覆盖原始目标及完整材料快照；顺序或来源变化都会生成新 brief。
func HashInput(goal string, materials []model.Material) string {
	if materials == nil {
		materials = []model.Material{}
	}
	body, _ := json.Marshal(struct {
		Version   int              `json:"version"`
		Goal      string           `json:"goal"`
		Materials []model.Material `json:"materials"`
	}{1, goal, materials})
	return hashBytes(body)
}

func hashBytes(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
