# Go Library 嵌入

`til-consensus` 可以作为 CLI 使用，也可以作为 Go library 嵌入到其它项目。

## 可导入包

稳定面向嵌入方的包位于模块根目录：

- `github.com/suchasplus/til-consensus/runner`：推荐入口，负责加载配置、解析 run plan、创建 engine、执行 run/resume/replay/action。
- `github.com/suchasplus/til-consensus/consensus`：核心 engine、workflow 类型、任务和结果结构。
- `github.com/suchasplus/til-consensus/config`：YAML 配置类型、include/profile 加载、run plan 解析。
- `github.com/suchasplus/til-consensus/runtime`：provider delegate、schema decode、repair 和 normalize。
- `github.com/suchasplus/til-consensus/runtime/api`：API provider runner。
- `github.com/suchasplus/til-consensus/runtime/cli`：CLI provider runner。
- `github.com/suchasplus/til-consensus/runtime/mock`：测试和本地 mock provider。
- `github.com/suchasplus/til-consensus/runtime/sdk`：SDK provider adapter。
- `github.com/suchasplus/til-consensus/store/file`：文件型 session store。
- `github.com/suchasplus/til-consensus/store/memory`：内存型 session store。
- `github.com/suchasplus/til-consensus/observer`：JSONL ledger/event observer。

`internal/*` 仍然只给 CLI 使用，例如 `internal/app`、`internal/artifact`、`internal/viewer`、`internal/preflight`。

## 最小嵌入流程

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
	"github.com/suchasplus/til-consensus/runner"
)

func main() {
	ctx := context.Background()

	loaded, err := runner.LoadConfig("til-consensus.yaml", "")
	if err != nil {
		panic(err)
	}
	executor := runner.NewExecutor(loaded)

	result, err := executor.Run(
		ctx,
		config.RunInput{
			Mode: consensus.WorkflowModeAdjudication,
			TaskSpec: config.TaskSpecInput{
				Goal: "判断这个 patch 是否真正修复了竞态问题",
			},
		},
		config.RunOverrides{},
		time.Now().UTC(),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Output.RequestID, result.Output.Mode)
}
```

如果要自定义 session store、observer、ledger 或测试时使用内存 store，可以设置 `Executor` 字段：

```go
executor := runner.NewExecutor(loaded)
executor.SessionStore = memorystore.New()
executor.Observer = myObserver
executor.Ledger = myLedger
```

## 后续 action

`runner.Executor.Act` 可基于已有 `RunResult` 执行后续 action：

```go
action, err := executor.Act(ctx, runner.ActionInput{
	Result:       *result.Output,
	Prompt:       "根据裁决结果创建修复 patch",
	ActorID:      "actor-a",
	ArtifactsDir: result.Plan.ArtifactsDir,
	Timeout:      10 * time.Minute,
})
if err != nil {
	panic(err)
}
fmt.Println(action.Output.Summary)
```

## 不使用 YAML

嵌入方也可以直接构造 `consensus.StartRequest`，绕过 `config` 包。

这种方式适合已经有自己的配置系统或数据库 schema 的服务：

```go
request := consensus.StartRequest{
	Mode:      consensus.WorkflowModeAdjudication,
	RequestID: consensus.NewRequestID(time.Now().UTC()),
	TaskSpec: consensus.TaskSpec{
		Goal: "判断这个 patch 是否真正修复了竞态问题",
	},
	Roles: consensus.RoleAssignments{
		Proposers:   []string{"proposer-a"},
		Challengers: []string{"challenger-a"},
		Arbiter:     "arbiter-a",
	},
	WaitingPolicy: consensus.WaitingPolicy{
		PerTaskTimeout: 10 * time.Minute,
		RetryAttempts:  1,
	},
}
```

如果直接构造 `StartRequest`，你仍然需要提供一个实现 `consensus.TaskDelegate` 的执行层。最简单的方式是继续使用 `runtime.NewDelegate`，并提供包含 providers/agents 的 `config.Config`。

## 嵌入建议

- Web 服务推荐从 `runner.Executor` 起步，只在需要替换持久化、事件或 provider 网关时下沉到 `consensus.Engine`。
- 本地开发可以用 `store/memory` 起步，生产环境按需改成 `store/file` 或自定义 `consensus.SessionStore`。
- 如果已有事件系统，实现 `consensus.Observer` 即可接收 phase/task/ledger/debug 事件。
- 如果已有 provider 网关，可以直接实现 `consensus.TaskDelegate`，不必使用 `runtime.NewDelegate`。
- CLI 产物写入、summary/view/web viewer 属于 `internal/*`，library 嵌入方应自己决定如何展示结果。
