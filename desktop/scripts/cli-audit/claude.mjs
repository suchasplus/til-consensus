import { join } from 'node:path'
import { mkdirSync } from 'node:fs'
import { auditDir, prompt, schema, expected, save, probe, validate } from './common.mjs'

const providerDir = join(auditDir, 'claude')
mkdirSync(providerDir, { recursive: true, mode: 0o700 })
const base = ['--safe-mode', '--print', '--tools', '', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}', '--no-session-persistence', '--permission-mode', 'dontAsk', '--permission-prompts', 'none', '--disable-slash-commands', '--no-chrome']
const versionRun = await probe('claude', 'version', 'claude', ['--version'], { timeoutMs: 10000 })
const summary = { provider: 'claude', version: versionRun.stdout.trim(), testedAt: new Date().toISOString(), source: ['https://code.claude.com/docs/en/headless', 'https://code.claude.com/docs/en/cli-reference'], expected, probes: [] }

function safeJSON(text) { try { return JSON.parse(text) } catch { return null } }
function summarize(run, mode) {
  const result = mode === 'json' ? safeJSON(run.stdout) : run.records.findLast(record => record.event.type === 'result')?.event
  const init = run.records.find(record => record.event.type === 'system' && record.event.subtype === 'init')?.event
  const events = [...new Set(run.records.map(record => record.event.type + (record.event.subtype ? '/' + record.event.subtype : record.event.type === 'stream_event' ? '/' + record.event.event?.type + (record.event.event?.delta?.type ? '/' + record.event.event.delta.type : '') : '')))]
  let extracted = result?.structured_output
  if (!extracted && typeof result?.result === 'string') extracted = safeJSON(result.result.replace(/^```(?:json)?\s*|\s*```$/g, ''))
  const terminal = result ? Object.fromEntries(['type', 'subtype', 'is_error', 'terminal_reason', 'stop_reason', 'errors'].filter(key => key in result).map(key => [key, result[key]])) : null
  const usage = result?.usage ? Object.fromEntries(['input_tokens', 'cache_creation_input_tokens', 'cache_read_input_tokens', 'output_tokens'].map(key => [key, result.usage[key]])) : null
  return { name: run.name, exitCode: run.exitCode, signal: run.signal, elapsedMs: run.elapsedMs, firstStdoutMs: run.firstStdoutMs, timedOut: run.timedOut, cancellationRequested: run.cancellationRequested, cancellationMs: run.cancellationMs, stderrBytes: run.stderrBytes, eventCount: run.eventCount, eventTypes: events, terminal, total_cost_usd: result?.total_cost_usd ?? null, usage, tools: init?.tools ?? null, mcp_servers: init?.mcp_servers ?? null, resolvedModel: init?.model ?? Object.keys(result?.modelUsage ?? {})[0] ?? null, validation: validate(extracted), artifacts: run.directory }
}

const schemaRun = await probe('claude', 'native-schema', 'claude', [...base, '--output-format', 'json', '--json-schema', JSON.stringify(schema)], { stdin: prompt, timeoutMs: 120000 })
summary.probes.push(summarize(schemaRun, 'json'))
save(join(providerDir, 'summary.json'), summary)
console.log(JSON.stringify({probe:'native-schema',...summary.probes.at(-1),result:undefined},null,2))

const streamRun = await probe('claude', 'stream-complete', 'claude', [...base, '--output-format', 'stream-json', '--verbose', '--include-partial-messages'], { stdin: prompt, timeoutMs: 120000 })
summary.probes.push(summarize(streamRun, 'stream'))
save(join(providerDir, 'summary.json'), summary)
console.log(JSON.stringify({probe:'stream-complete',...summary.probes.at(-1),result:undefined},null,2))

const cancelRun = await probe('claude', 'stream-cancel', 'claude', [...base, '--output-format', 'stream-json', '--verbose', '--include-partial-messages'], { stdin: prompt + '\n额外要求：输出 risk 约 200 字，便于验证流式取消。', timeoutMs: 120000, cancelWhen: event => event.type === 'stream_event' && event.event?.type === 'content_block_delta' && event.event?.delta?.type === 'text_delta' && Boolean(event.event.delta.text) })
summary.probes.push(summarize(cancelRun, 'stream'))
save(join(providerDir, 'summary.json'), summary)
console.log(JSON.stringify({probe:'stream-cancel',...summary.probes.at(-1),result:undefined},null,2))
