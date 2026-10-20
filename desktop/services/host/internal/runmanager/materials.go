package runmanager

import (
	"encoding/json"
	"fmt"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/preparation"
)

func branchMaterials(goal string, parent []model.Material, added []model.MaterialInput) ([]model.MaterialInput, error) {
	result := make([]model.MaterialInput, 0, len(parent)+len(added))
	seen := map[string]model.MaterialInput{}
	for _, material := range parent {
		input := model.MaterialInput{ID: material.ID, Title: material.Title, Content: material.Content, Source: material.Source, RawSHA256: material.RawSHA256}
		seen[input.ID] = input
		result = append(result, input)
	}
	normalizedAdded, err := preparation.Normalize(goal, added)
	if err != nil {
		return nil, err
	}
	for _, m := range normalizedAdded {
		input := model.MaterialInput{ID: m.ID, Title: m.Title, Content: m.Content, Source: m.Source, RawSHA256: m.RawSHA256}
		if old, ok := seen[input.ID]; ok {
			if old != input {
				return nil, fmt.Errorf("材料ID %s 已用于不同内容或来源，请使用新ID", input.ID)
			}
			continue
		}
		seen[input.ID] = input
		result = append(result, input)
	}
	if _, err := preparation.Normalize(goal, result); err != nil {
		return nil, err
	}
	return result, nil
}
func checkBranchInputSize(goal, parentContext string, materials []model.MaterialInput) error {
	body, err := json.Marshal(struct {
		Goal          string                `json:"goal"`
		ParentContext string                `json:"parentContext"`
		Materials     []model.MaterialInput `json:"materials"`
	}{goal, parentContext, materials})
	if err != nil {
		return err
	}
	if len(body) > MaxBranchInputBytes {
		return fmt.Errorf("关联讨论目标、父上下文和原文材料合计超过 %d KiB，不能静默截断，请拆分讨论", MaxBranchInputBytes/1024)
	}
	return nil
}
