package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/suchasplus/til-consensus/internal/consensus"
)

const (
	TemplatePresetQuickstart = "quickstart"
	TemplatePresetOpenAI     = "openai"
	TemplatePresetCoding     = "coding"
	TemplatePresetDebate     = "debate"
	TemplatePresetDelphi     = "delphi"
	TemplatePresetGeneric    = "generic"
	TemplatePresetCodex      = "codex"
	TemplatePresetClaude     = "claude"
	TemplatePresetGemini     = "gemini"
)

func InitTemplate() Config {
	return Config{
		SchemaVersion: 1,
		Defaults: DefaultsConfig{
			SuccessCriteria:   []string{"给出 claim 级裁决", "允许 undetermined"},
			AllowedTools:      []string{"repo", "tests", "benchmarks"},
			PerTaskTimeout:    Duration{Duration: 20 * time.Minute},
			TaskRetryAttempts: consensus.DefaultTaskRetryAttempts,
			ProposalPolicy: ProposalPolicyConfig{
				MaxPasses:          1,
				MaxClaimsPerWorker: 3,
				DedupeStrategy:     "normalized-statement",
			},
			VerificationPolicy: VerificationPolicyConfig{
				AllowSemanticVerifier: true,
				MaxParallelChecks:     4,
				RequiredChecks: []consensus.VerificationCheck{
					{Name: "allowed_paths", Kind: "allowed_paths"},
				},
			},
			ArbiterPolicy: ArbiterPolicyConfig{
				AllowUndetermined: true,
				BlindReview:       true,
			},
		},
		Output: OutputConfig{
			Directory: "./out/{requestId}",
		},
		Providers: map[string]ProviderConfig{
			"mock": {
				Type:     ProviderTypeMock,
				Behavior: "deterministic",
				Models: map[string]ProviderModelConfig{
					"default": {
						ProviderModel: "mock-default",
					},
				},
			},
		},
		Agents: []AgentConfig{
			{ID: "proposer-a", Provider: "mock", Model: "default", Role: "proposer"},
			{ID: "challenger-a", Provider: "mock", Model: "default", Role: "challenger"},
			{ID: "arbiter-a", Provider: "mock", Model: "default", Role: "arbiter"},
			{ID: "verifier-a", Provider: "mock", Model: "default", Role: "semantic-verifier"},
			{ID: "reporter-a", Provider: "mock", Model: "default", Role: "reporter"},
			{ID: "actor-a", Provider: "mock", Model: "default", Role: "actor"},
		},
		Roles: RolesConfig{
			Proposers:        []string{"proposer-a"},
			Challengers:      []string{"challenger-a"},
			Arbiter:          "arbiter-a",
			SemanticVerifier: "verifier-a",
			Reporter:         "reporter-a",
			Actor:            "actor-a",
		},
	}
}

func RenderTemplate(preset string) (string, error) {
	switch normalizePreset(preset) {
	case TemplatePresetQuickstart:
		return quickstartTemplate, nil
	case TemplatePresetOpenAI:
		return openaiTemplate, nil
	case TemplatePresetCoding:
		return codingTemplate, nil
	case TemplatePresetDebate:
		return debateTemplate, nil
	case TemplatePresetDelphi:
		return delphiTemplate, nil
	case TemplatePresetGeneric:
		return genericTemplate, nil
	case TemplatePresetCodex:
		return codexTemplate, nil
	case TemplatePresetClaude:
		return claudeTemplate, nil
	case TemplatePresetGemini:
		return geminiTemplate, nil
	default:
		return "", fmt.Errorf("unsupported config preset: %s", preset)
	}
}

func WritePresetTemplate(path string, preset string, force bool) error {
	body, err := RenderTemplate(preset)
	if err != nil {
		return err
	}
	if !force {
		if _, statErr := os.Stat(path); statErr == nil {
			return fmt.Errorf("config already exists: %s", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write template config: %w", err)
	}
	return nil
}

func normalizePreset(preset string) string {
	value := strings.TrimSpace(strings.ToLower(preset))
	if value == "" {
		return TemplatePresetQuickstart
	}
	return value
}

const quickstartTemplate = `# til-consensus quickstart 配置
# 适合第一次跑通 CLI：零凭证、mock provider、默认输出到 ./out/{requestId}
# 推荐修改顺序：先改 provider/agent，再改 taskSpec，再改 verificationPolicy
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对证据不足的部分明确标成 undetermined
  allowed_tools:
    - repo
    - tests
    - benchmarks
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 3
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
    required_checks:
      - name: workspace-boundary
        kind: allowed_paths
        paths:
          - .
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

# 输出目录下会生成 result.json、ledger.jsonl、summary.md 和 artifacts/
output:
  directory: ./out/{requestId}

providers:
  mock:
    type: mock
    behavior: deterministic
    models:
      default:
        provider_model: mock-default

agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
  - id: verifier-a
    provider: mock
    model: default
    role: semantic-verifier
  - id: reporter-a
    provider: mock
    model: default
    role: reporter
  - id: actor-a
    provider: mock
    model: default
    role: actor

roles:
  proposers:
    - proposer-a
  challengers:
    - challenger-a
  arbiter: arbiter-a
  semantic_verifier: verifier-a
  reporter: reporter-a
  actor: actor-a
`

const debateTemplate = `# til-consensus free_debate 配置
# 适合多 CLI 交叉辩论：initial -> debate* -> final_vote -> report -> action
schema_version: 1

defaults:
  mode: free_debate
  success_criteria:
    - 让多个 participant 独立提出主张并交叉辩论
    - 最终通过 final vote 收敛
    - 没有共识时允许 no_consensus
  per_task_timeout: 20m
  task_retry_attempts: 1
  debate_policy:
    min_rounds: 2
    max_rounds: 3
    vote_threshold: 1.0
    enable_early_stop: true
    peer_context_mode: summary+active_claims

output:
  directory: ./out/{requestId}

providers:
  mock:
    type: mock
    behavior: deterministic
    models:
      default:
        provider_model: mock-default

agents:
  - id: debater-a
    provider: mock
    model: default
    role: participant
  - id: debater-b
    provider: mock
    model: default
    role: participant
  - id: reporter-a
    provider: mock
    model: default
    role: reporter
  - id: actor-a
    provider: mock
    model: default
    role: actor

roles:
  participants:
    - debater-a
    - debater-b
  reporter: reporter-a
  actor: actor-a
`

const delphiTemplate = `# til-consensus delphi 配置
# 适合匿名多轮问卷：questionnaire -> summary -> revision，直到收敛或达到轮数上限
schema_version: 1

defaults:
  mode: delphi
  success_criteria:
    - 让参与者匿名给出候选结论和评分
    - 每轮基于匿名汇总修订意见
    - 输出推荐结论、收敛度和异议摘要
  per_task_timeout: 20m
  task_retry_attempts: 1
  delphi_policy:
    min_rounds: 2
    max_rounds: 3
    convergence_threshold: 0.8
    rating_scale_min: 1
    rating_scale_max: 5
    anonymous: true
    facilitator_summary_style: anonymous-aggregate

output:
  directory: ./out/{requestId}

providers:
  mock:
    type: mock
    behavior: deterministic
    models:
      default:
        provider_model: mock-default

agents:
  - id: participant-a
    provider: mock
    model: default
    role: participant
  - id: participant-b
    provider: mock
    model: default
    role: participant
  - id: facilitator-a
    provider: mock
    model: default
    role: facilitator
  - id: reporter-a
    provider: mock
    model: default
    role: reporter

roles:
  participants:
    - participant-a
    - participant-b
  facilitator: facilitator-a
  reporter: reporter-a
`

const genericTemplate = `# til-consensus generic CLI 配置
# 适合接自己的脚本、本地模型代理或公司内部 CLI
# 推荐修改顺序：先改 command/args/env，再改 agent 分工，再改 run.yaml
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对证据不足的部分明确标成 undetermined
  allowed_tools:
    - docs
    - compare
    - summarize
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

output:
  directory: ./out/{requestId}

providers:
  generic-local:
    type: cli
    cli_type: generic
    command: python3
    args:
      - ./scripts/generic_adapter.py
    env:
      ADAPTER_MODEL: "{providerModel}"
      ADAPTER_ROLE: "{role}"
      ADAPTER_REQUEST_ID: "{requestId}"
      ADAPTER_SESSION_ID: "{sessionId}"
    models:
      default:
        provider_model: local-generic
        reasoning: medium

agents:
  - id: proposer-generic
    provider: generic-local
    model: default
    role: proposer
  - id: challenger-generic
    provider: generic-local
    model: default
    role: challenger
  - id: verifier-generic
    provider: generic-local
    model: default
    role: semantic-verifier
  - id: arbiter-generic
    provider: generic-local
    model: default
    role: arbiter
  - id: reporter-generic
    provider: generic-local
    model: default
    role: reporter

roles:
  proposers:
    - proposer-generic
  challengers:
    - challenger-generic
  semantic_verifier: verifier-generic
  arbiter: arbiter-generic
  reporter: reporter-generic
`

const codexTemplate = `# til-consensus Codex CLI 配置
# 适合文档完善、架构选择和 coding review
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对证据不足的部分明确标成 undetermined
  allowed_tools:
    - repo
    - docs
    - tests
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

output:
  directory: ./out/{requestId}

providers:
  codex-cli:
    type: cli
    cli_type: codex
    command: codex
    models:
      default:
        provider_model: gpt-5
        reasoning: medium

agents:
  - id: proposer-codex
    provider: codex-cli
    model: default
    role: proposer
  - id: challenger-codex
    provider: codex-cli
    model: default
    role: challenger
  - id: verifier-codex
    provider: codex-cli
    model: default
    role: semantic-verifier
  - id: arbiter-codex
    provider: codex-cli
    model: default
    role: arbiter
  - id: reporter-codex
    provider: codex-cli
    model: default
    role: reporter

roles:
  proposers: [proposer-codex]
  challengers: [challenger-codex]
  semantic_verifier: verifier-codex
  arbiter: arbiter-codex
  reporter: reporter-codex
`

const claudeTemplate = `# til-consensus Claude CLI 配置
# 适合架构讨论、风险审查和 claim challenge
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 明确列出 caveat 与 unresolved
  allowed_tools:
    - docs
    - compare
    - summarize
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

output:
  directory: ./out/{requestId}

providers:
  claude-cli:
    type: cli
    cli_type: claude
    command: claude
    models:
      default:
        provider_model: claude-sonnet-4
        reasoning: medium

agents:
  - id: proposer-claude
    provider: claude-cli
    model: default
    role: proposer
  - id: challenger-claude
    provider: claude-cli
    model: default
    role: challenger
  - id: verifier-claude
    provider: claude-cli
    model: default
    role: semantic-verifier
  - id: arbiter-claude
    provider: claude-cli
    model: default
    role: arbiter
  - id: reporter-claude
    provider: claude-cli
    model: default
    role: reporter

roles:
  proposers: [proposer-claude]
  challengers: [challenger-claude]
  semantic_verifier: verifier-claude
  arbiter: arbiter-claude
  reporter: reporter-claude
`

const geminiTemplate = `# til-consensus Gemini CLI 配置
# 适合信息搜集、结构化摘要和 semantic verification
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对证据不足的部分明确标成 undetermined
  allowed_tools:
    - docs
    - compare
    - summarize
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 2
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

output:
  directory: ./out/{requestId}

providers:
  gemini-cli:
    type: cli
    cli_type: gemini
    command: gemini
    models:
      default:
        provider_model: gemini-2.5-pro
        reasoning: medium

agents:
  - id: proposer-gemini
    provider: gemini-cli
    model: default
    role: proposer
  - id: challenger-gemini
    provider: gemini-cli
    model: default
    role: challenger
  - id: verifier-gemini
    provider: gemini-cli
    model: default
    role: semantic-verifier
  - id: arbiter-gemini
    provider: gemini-cli
    model: default
    role: arbiter
  - id: reporter-gemini
    provider: gemini-cli
    model: default
    role: reporter

roles:
  proposers: [proposer-gemini]
  challengers: [challenger-gemini]
  semantic_verifier: verifier-gemini
  arbiter: arbiter-gemini
  reporter: reporter-gemini
`

const openaiTemplate = `# til-consensus OpenAI API 配置
# 适合接真实模型：先填 api_key_env / provider_model，再按角色替换 agent
# 推荐修改顺序：先改 provider/agent，再改 taskSpec，再改 verificationPolicy
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对证据不足的部分明确标成 undetermined
  allowed_tools:
    - repo
    - docs
    - tests
  per_task_timeout: 20m
  task_retry_attempts: 1
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 4
    required_checks:
      - name: workspace-boundary
        kind: allowed_paths
        paths:
          - .
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

# 输出目录下会生成 result.json、ledger.jsonl、summary.md 和 artifacts/
output:
  directory: ./out/{requestId}

providers:
  openai:
    type: api
    protocol: openai-compatible
    base_url: https://api.openai.com/v1
    api_key_env: OPENAI_API_KEY
    models:
      default:
        provider_model: your-openai-model
        temperature: 0.2
        reasoning: medium

agents:
  - id: proposer-a
    provider: openai
    model: default
    role: proposer
  - id: challenger-a
    provider: openai
    model: default
    role: challenger
  - id: arbiter-a
    provider: openai
    model: default
    role: arbiter
  - id: verifier-a
    provider: openai
    model: default
    role: semantic-verifier
  - id: reporter-a
    provider: openai
    model: default
    role: reporter
  - id: actor-a
    provider: openai
    model: default
    role: actor

roles:
  proposers:
    - proposer-a
  challengers:
    - challenger-a
  arbiter: arbiter-a
  semantic_verifier: verifier-a
  reporter: reporter-a
  actor: actor-a
`

const codingTemplate = `# til-consensus coding 裁决配置
# 适合代码审查、patch 裁决、benchmark 主张验证
# 推荐修改顺序：先改 provider/agent，再改 taskSpec，再改 verificationPolicy
schema_version: 1

defaults:
  success_criteria:
    - 给出 claim 级裁决
    - 对测试、基准和 diff 的证据做显式引用
    - 证据不足时允许 undetermined
  allowed_tools:
    - repo
    - git
    - tests
    - benchmarks
  per_task_timeout: 20m
  task_retry_attempts: 1
  workspace_snapshot:
    root: .
    revision: HEAD
    paths:
      - cmd
      - internal
      - go.mod
      - go.sum
  task_constraints:
    language: go
    allowed_paths:
      - cmd/
      - internal/
      - go.mod
      - go.sum
    required_commands:
      - go
      - git
    notes:
      - patch 必须限制在允许路径内
      - benchmark 结果需要可复现
  proposal_policy:
    max_passes: 1
    max_claims_per_worker: 4
    dedupe_strategy: normalized-statement
  verification_policy:
    allow_semantic_verifier: true
    max_parallel_checks: 4
    required_checks:
      - name: workspace-snapshot
        kind: workspace_snapshot
      - name: allowed-paths
        kind: allowed_paths
        paths:
          - cmd/
          - internal/
          - go.mod
          - go.sum
      - name: changed-files
        kind: git_diff_paths
        base_revision: origin/main
        paths:
          - cmd/
          - internal/
      - name: unit-tests
        kind: command
        command: go
        args:
          - test
          - ./...
        workdir: .
      - name: benchmark-budget
        kind: benchmark_threshold
        command: go
        args:
          - test
          - ./...
          - -run
          - ^$
          - -bench
          - .
        workdir: .
        pattern: 'Benchmark.*\s+(\d+(?:\.\d+)?) ns/op'
        threshold: 250000
        threshold_mode: max
  arbiter_policy:
    allow_undetermined: true
    blind_review: true

# 输出目录下会生成 result.json、ledger.jsonl、summary.md 和 artifacts/
output:
  directory: ./out/{requestId}

providers:
  mock:
    type: mock
    behavior: deterministic
    models:
      default:
        provider_model: mock-default

agents:
  - id: proposer-a
    provider: mock
    model: default
    role: proposer
  - id: challenger-a
    provider: mock
    model: default
    role: challenger
  - id: arbiter-a
    provider: mock
    model: default
    role: arbiter
  - id: verifier-a
    provider: mock
    model: default
    role: semantic-verifier
  - id: reporter-a
    provider: mock
    model: default
    role: reporter
  - id: actor-a
    provider: mock
    model: default
    role: actor

roles:
  proposers:
    - proposer-a
  challengers:
    - challenger-a
  arbiter: arbiter-a
  semantic_verifier: verifier-a
  reporter: reporter-a
  actor: actor-a
`
