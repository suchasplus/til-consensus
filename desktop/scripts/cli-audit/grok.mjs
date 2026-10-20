import { join } from 'node:path'
import { mkdirSync } from 'node:fs'
import { root, auditDir, prompt, schema, expected, probe, validate, save } from './common.mjs'

const directory = join(auditDir, 'grok')
mkdirSync(directory, { recursive: true, mode: 0o700 })
const base = ['--permission-mode', 'plan', '--no-subagents', '--disable-web-search', '--tools', '', '--deny', 'MCPTool(*)', '--max-turns', '2']
const env = { GROK_MEMORY: '0' }
const summaries = []
const loggedIn = process.argv.includes('--logged-in')
const suffix = loggedIn ? '-logged-in' : ''

function inspect(result) {
  let envelope = null
  try { envelope = JSON.parse(result.stdout.trim()) } catch {}
  const terminal = result.records.findLast(({ event }) => ['end', 'result', 'error'].includes(event.type))?.event
  envelope ??= terminal ?? null
  let value = envelope?.structuredOutput ?? null
  const text = envelope?.text ?? envelope?.result ?? result.records.filter(({ event }) => event.type === 'text').map(({ event }) => event.data ?? '').join('')
  if (value === null) { try { value = JSON.parse(text) } catch {} }
  const error = result.records.find(({ event }) => event.type === 'error')?.event
  const summary = {
    name: result.name, exitCode: result.exitCode, signal: result.signal,
    elapsedMs: result.elapsedMs, firstStdoutMs: result.firstStdoutMs,
    timedOut: result.timedOut, cancellationRequested: result.cancellationRequested,
    cancellationMs: result.cancellationMs, eventCount: result.eventCount,
    envelopeKeys: envelope && typeof envelope === 'object' ? Object.keys(envelope) : [],
    stopReason: envelope?.stopReason, sessionId: envelope?.sessionId,
    requestId: envelope?.requestId, usage: envelope?.usage,
    modelUsage: envelope?.modelUsage,
    costUSD: envelope?.total_cost_usd,
    totalCostUSDTicks: envelope?.total_cost_usd_ticks,
    models: Object.keys(envelope?.modelUsage ?? {}),
    errorType: error?.type,
    errorMessage: error?.message?.replace(/https?:\/\/[^\s\"<>]+/g, '[URL]'),
    validation: validate(value),
    value,
    firstTextMs: result.records.find(({ event }) => event.type === 'text')?.ms ?? null,
    textEventCount: result.records.filter(({ event }) => event.type === 'text').length,
    terminalEvent: terminal?.type ?? null,
    eventShapes: [...new Set(result.records.map(({event}) => JSON.stringify({ keys: Object.keys(event), type: event.type, method: event.method, sessionUpdate: event.update?.sessionUpdate ?? event.params?.update?.sessionUpdate })))].map(x => JSON.parse(x)),
    artifacts: result.directory,
  }
  save(join(result.directory, 'summary.json'), summary)
  summaries.push(summary)
  return summary
}

const version = await probe('grok', 'version' + suffix, 'grok', ['--version'], { timeoutMs: 10_000 })
const help = await probe('grok', 'help' + suffix, 'grok', ['--help'], { timeoutMs: 10_000 })
const json = await probe('grok', 'native-schema-json' + suffix, 'grok', [...base, '--json-schema', JSON.stringify(schema), '-p', prompt], { env, timeoutMs: 120_000 })
const jsonSummary = inspect(json)
console.log(JSON.stringify(jsonSummary, null, 2))
const authenticationBlocked = json.exitCode !== 0 && /Not signed in/i.test(jsonSummary.errorMessage ?? '')
if (authenticationBlocked) {
  const fixtures = join(root, 'fixtures/cli/grok')
  mkdirSync(fixtures, { recursive: true })
  save(join(fixtures, 'auth-error.json'), JSON.parse(json.stdout.trim()))
  save(join(fixtures, 'auth-error.stderr.txt'), json.stderr)
  save(join(fixtures, 'auth-error.meta.json'), {
    source: `${version.stdout.trim()} native headless invocation`, exitCode: json.exitCode,
    elapsedMs: json.elapsedMs, authenticated: false, inferenceCompleted: false,
  })
} else {
  if (loggedIn) {
    const stream = await probe('grok', 'native-stream-complete' + suffix, 'grok', [...base, '--output-format', 'streaming-json', '-p', prompt], { env, timeoutMs: 120_000 })
    console.log(JSON.stringify(inspect(stream), null, 2))
  }
  const stream = await probe('grok', 'native-stream-cancel' + suffix, 'grok', [...base, '--output-format', 'streaming-json', '-p', prompt], {
    env, timeoutMs: 120_000,
    cancelWhen: event => event.type === 'text' && Boolean(event.data),
  })
  console.log(JSON.stringify(inspect(stream), null, 2))
}
const initialize = await probe('grok', 'acp-initialize' + suffix, 'grok', ['agent', '--no-leader', 'stdio'], {
  env, timeoutMs: 20_000, keepStdinOpen: true,
  stdin: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: 1, clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false } } }) + '\n',
  cancelWhen: event => event.id === 1 && Boolean(event.result || event.error),
})
const init = initialize.records.find(({ event }) => event.id === 1)?.event
const acpSummary = {
  name: initialize.name, exitCode: initialize.exitCode, signal: initialize.signal,
  elapsedMs: initialize.elapsedMs, firstStdoutMs: initialize.firstStdoutMs,
  timedOut: initialize.timedOut, cancellationRequested: initialize.cancellationRequested,
  eventCount: initialize.eventCount,
  protocolVersion: init?.result?.protocolVersion,
  resultKeys: Object.keys(init?.result ?? {}),
  agentInfo: init?.result?.agentInfo,
  capabilities: init?.result?.agentCapabilities,
  authMethods: init?.result?.authMethods?.map(method => ({ id: method.id, name: method.name })),
  error: init?.error,
  artifacts: initialize.directory,
}
save(join(initialize.directory, 'summary.json'), acpSummary)
console.log(JSON.stringify(acpSummary, null, 2))

save(join(directory, loggedIn ? 'summary.logged-in.json' : 'summary.json'), {
  provider: 'grok', version: version.stdout.trim(),
  installedHelpArtifact: join(help.directory, 'stdout.txt'),
  expected, probes: summaries, acp: acpSummary,
  status: authenticationBlocked ? 'blocked_authentication' : 'probed',
  inferenceCompleted: !authenticationBlocked && jsonSummary.validation.ok,
  cancellationDuringContentVerified: summaries.some(item => item.name.startsWith('native-stream-cancel') && item.cancellationRequested && !item.timedOut),
  defaultModel: jsonSummary.models[0] ?? null,
  notes: authenticationBlocked ? ['真实调用返回未登录；未验证模型推理、schema约束、正常终态、usage及内容生成中的取消。ACP initialize 成功仅证明协议握手。'] : [],
  sources: [
    'https://docs.x.ai/build/cli/headless-scripting',
    'https://docs.x.ai/build/cli/reference',
    'https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/14-headless-mode.md',
  ],
})
