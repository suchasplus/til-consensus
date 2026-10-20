import { existsSync, mkdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { auditDir, prompt, schema, expected, probe, save, validate } from './common.mjs'

const providerDir = join(auditDir, 'agy')
mkdirSync(providerDir, { recursive: true, mode: 0o700 })
const commonArgs = ['--mode', 'plan', '--sandbox', '--print-timeout', '110s']
async function savedProbe(name, args, options) {
  const directory = join(providerDir, name)
  // Auditing is not a load test: preserve completed real requests across reruns.
  if (existsSync(join(directory, 'meta.json'))) {
    return {
      ...JSON.parse(readFileSync(join(directory, 'meta.json'), 'utf8')),
      directory,
      stdout: readFileSync(join(directory, 'stdout.txt'), 'utf8'),
      stderr: readFileSync(join(directory, 'stderr.txt'), 'utf8'),
      records: JSON.parse(readFileSync(join(directory, 'events.json'), 'utf8')),
    }
  }
  return probe('agy', name, 'agy', args, options)
}
function alignment(result) {
  if (!result.structured_output) return { checked: false, reason: '没有 structured_output' }
  try {
    const response = JSON.parse(result.response)
    const mismatches = Object.keys(schema.properties).filter(key => JSON.stringify(response[key]) !== JSON.stringify(result.structured_output[key]))
    return { checked: true, ok: mismatches.length === 0, mismatches }
  } catch {
    return { checked: false, reason: 'response 不是单个 JSON 对象；须使用原生 structured_output，并保留原始 response' }
  }
}
const summarize = run => ({
  name: run.name, exitCode: run.exitCode, signal: run.signal,
  elapsedMs: run.elapsedMs, firstStdoutMs: run.firstStdoutMs,
  timedOut: run.timedOut, cancellationRequested: run.cancellationRequested,
  cancellationMs: run.cancellationMs, eventCount: run.eventCount,
  eventTypes: [...new Set(run.records.map(record => record.event.event).filter(Boolean))],
  results: run.records.filter(record => record.event.event === 'result').map(record => ({
    ms: record.ms, ...record.event.result,
    validation: record.event.result.structured_output ? validate(record.event.result.structured_output) : undefined,
    structuredResponseAlignment: alignment(record.event.result),
  })),
  toolCalls: run.records.filter(record => record.event.step_update?.step_type === 'tool').map(record => ({
    ms: record.ms, tool: record.event.step_update.tool_name,
    state: record.event.step_update.state,
  })),
  init: run.records.find(record => record.event.event === 'init')?.event.init,
  diagnostics: run.stderr.trim(),
  directory: run.directory,
})

// /model is a CLI-owned read-only command: no model inference or credential file read.
const version = await savedProbe('version', ['--version'], { timeoutMs: 20_000 })
await savedProbe('help', ['--help'], { timeoutMs: 20_000 })
const model = await savedProbe('model', ['-p', '/model', '--output-format', 'json', ...commonArgs], { timeoutMs: 25_000 })
let modelData
try { modelData = JSON.parse(model.stdout) } catch {}

const user = content => JSON.stringify({ event: 'user', message: { content } })
const followup = '根据你上一轮读取的真实 package.json 和 go.mod，再输出完整的同一结构化对象；所有依赖版本、project_name 和 scripts 必须保持正确。risk 改为指出应用多轮 session 累计 usage 和每轮 response 处理上的一个具体风险。禁止调用工具、读取文件、执行命令和联网。只返回符合当前 JSON Schema 的 JSON 对象。'
save(join(providerDir, 'input-envelopes.ndjson'), `${user(prompt)}\n${user(followup)}\n`)

// The real stream input contract uses event:user/message.content, never -p.
// Closing stdin after the queued turns is required for bounded process lifetime.
const multi = await savedProbe('schema-stream-multiturn', [
  '--input-format', 'stream-json', '--output-format', 'stream-json',
  '--json-schema', JSON.stringify(schema), '--disable-slash-commands', ...commonArgs,
], { stdin: `${user(prompt)}\n${user(followup)}\n`, timeoutMs: 120_000 })

// A separate real model turn verifies cancellation using the first response delta.
const cancelPrompt = `请根据下面真实本地项目字段，撰写 100 条彼此不同的桌面多 CLI 共识应用接入风险。每条约 70 字，简体中文。必须直接输出正文，不要调用工具、执行命令、读取文件或联网。\n真实项目字段：${JSON.stringify(expected)}`
const cancel = await savedProbe('stream-cancel', [
  '--input-format', 'stream-json', '--output-format', 'stream-json',
  '--disable-slash-commands', ...commonArgs,
], {
  stdin: `${user(cancelPrompt)}\n`, timeoutMs: 120_000,
  cancelWhen: event => event.event === 'step_update' && Boolean(event.step_update?.text_delta),
})

// The previous runs revealed that --disable-slash-commands disables --mode plan.
// A fourth, single-turn request isolates the supported plan + native schema path.
const single = await savedProbe('schema-stream-plan-single', [
  '--input-format', 'stream-json', '--output-format', 'stream-json',
  '--json-schema', JSON.stringify(schema), ...commonArgs,
], { stdin: `${user(prompt)}\n`, timeoutMs: 120_000 })

const result = {
  provider: 'agy', cliVersion: version.stdout.trim(),
  officialDocs: 'https://www.antigravity.google/docs/cli/headless/',
  resolvedModel: modelData?.command?.data ?? null,
  modelProbe: { exitCode: model.exitCode, status: modelData?.status, response: modelData?.response },
  requestedMode: 'plan', effectivePermissionMode: 'request-review', sandbox: true, bypassPermissions: false,
  modeCaveat: '多轮及取消 probe 带 --disable-slash-commands，CLI 明确警告 --mode plan has no effect；单轮补充 probe 移除此参数。',
  inputContract: { event: 'user', message: { content: '<string or text blocks>' } },
  runs: [summarize(multi), summarize(cancel), summarize(single)],
  findings: [
    '真实 package.json/go.mod 字段在 native structured_output 中均通过核对；这不代表每轮语义新鲜度正确。',
    'schema-stream-multiturn 第二轮 response.risk 已改变，但 structured_output.risk 仍是第一轮值；在多轮 native schema 输出上观察到陈旧结果，不可直接宣称多轮 schema 可靠。',
    '首轮 response 拼接了两个 JSON 对象；不要把 response 当作可直接 JSON.parse 的单对象。',
    '首轮 num_turns=2、次轮 num_turns=3；原生 schema 内部补充轮次会影响计数，不可据此给 UI 的人类轮次编号。usage 和 duration_seconds 为会话累计值。',
    '取消发送进程组 SIGINT，实际终态 status=ERROR/error=interrupted，exit1；应结合本地取消意图映射 canceled，而非依赖文档中的 INTERRUPTED 字面值。',
  ],
  compatibility: {
    oldTransport: 'til-consensus runtime/cli/prompt_file.go 用 --add-dir、--dangerously-skip-permissions 和 -p bootstrap 要求模型读取临时文件；runtime/cli/runner.go 未给 antigravity 添加 native schema/envelope 处理。',
    recommended: '以实际版本能力探测选择 native stream-json stdin，每个 schema 任务单独进程，使用 --json-schema 并读取 result.structured_output；默认 plan/sandbox 且不加 --disable-slash-commands，保留每轮 result、conversation_id、累计 usage。暂不复用带 schema 的多轮 session。',
  },
}
save(join(providerDir, 'summary.json'), result)
console.log(JSON.stringify(result, null, 2))
