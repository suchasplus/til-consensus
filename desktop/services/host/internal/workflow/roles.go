package workflow

import (
	"fmt"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

// NewRolePlan 固定优先级而非根据一次调用结果静默更换提供方。
func NewRolePlan(configs []model.ProviderConfig) (*model.RolePlan, error) {
	enabled := map[string]bool{}
	for _, p := range configs {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		if enabled[p.ID] {
			return nil, fmt.Errorf("重复的启用 CLI：%s", p.ID)
		}
		enabled[p.ID] = true
	}
	ordered := []string{}
	for _, id := range []string{"codex", "claude", "grok", "agy"} {
		if enabled[id] {
			ordered = append(ordered, id)
		}
	}
	if len(ordered) != len(enabled) {
		return nil, fmt.Errorf("启用配置包含未知 CLI")
	}
	if len(ordered) < 3 {
		return nil, fmt.Errorf("新讨论至少需要 3 家 CLI：1 位提案者与至少 2 位独立评审")
	}
	return &model.RolePlan{Proposer: ordered[0], Reviewers: ordered[1:]}, nil
}

// RolePlanFor 对缺少新字段的历史讨论保持原四家角色，绝不查询当前设置。
func RolePlanFor(f model.FrozenConfig) model.RolePlan {
	if f.RolePlan != nil {
		return *f.RolePlan
	}
	return model.RolePlan{Proposer: "codex", Reviewers: []string{"claude", "grok", "agy"}}
}

func SnapshotRolePlan(s model.RunSnapshot) model.RolePlan {
	if s.RolePlan != nil {
		return *s.RolePlan
	}
	return RolePlanFor(model.FrozenConfig{})
}

func AssignRoles(plan model.RolePlan) consensus.RoleAssignments {
	// 首位评审裁决，末位评审进行语义复核；二者至少为两个不同 CLI。
	return consensus.RoleAssignments{Proposers: []string{plan.Proposer}, Challengers: append([]string(nil), plan.Reviewers...), Participants: append([]string(nil), plan.Reviewers...), Synthesizer: plan.Proposer, Arbiter: plan.Reviewers[0], SemanticVerifier: plan.Reviewers[len(plan.Reviewers)-1], Facilitator: plan.Proposer}
}

func AgentLabel(id string) string {
	if label := map[string]string{"codex": "Codex", "claude": "Claude Code", "grok": "Grok Build", "agy": "AGY"}[id]; label != "" {
		return label
	}
	return id
}

func AnonymousIdentity(plan model.RolePlan, id string) (string, string) {
	for i, reviewer := range plan.Reviewers {
		if id == reviewer {
			return fmt.Sprintf("expert-%c", 'a'+i), fmt.Sprintf("专家 %c", 'A'+i)
		}
	}
	if id == plan.Proposer {
		return "facilitator", "匿名汇总"
	}
	return "participant", "匿名参与者"
}
