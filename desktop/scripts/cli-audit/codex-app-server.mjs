import { join } from 'node:path'
import { auditDir, prompt, probe, save } from './common.mjs'

let threadId, turnId, model, interruptedAt, completed, firstTextAt
const started = performance.now()
const send = (child, value) => child.stdin.write(JSON.stringify(value) + '\n')
const result = await probe('codex', 'app-server-interrupt', 'codex', ['app-server', '--listen', 'stdio://'], {
  keepStdinOpen: true,
  timeoutMs: 120_000,
  stdin: JSON.stringify({ id: 1, method: 'initialize', params: { clientInfo: { name: 'tcview_cli_audit', title: 'TCView 接口实测', version: '0.1.0' } } }) + '\n',
  onEvent(event, child) {
    if (event.id === 1 && event.result) {
      send(child, { method: 'initialized' })
      send(child, { id: 2, method: 'thread/start', params: { cwd: join(auditDir, 'codex/app-server-interrupt'), ephemeral: true, approvalPolicy: 'never', sandbox: 'read-only' } })
    } else if (event.id === 2 && event.result) {
      threadId = event.result.thread.id
      model = event.result.model
      send(child, { id: 3, method: 'turn/start', params: { threadId, input: [{ type: 'text', text: prompt }] } })
    } else if (event.method === 'turn/started') {
      turnId = event.params.turn.id
    } else if (event.method === 'item/agentMessage/delta' && event.params.delta && !interruptedAt) {
      firstTextAt = Math.round(performance.now() - started)
      interruptedAt = performance.now()
      send(child, { id: 4, method: 'turn/interrupt', params: { threadId, turnId } })
    } else if (event.method === 'turn/completed') {
      completed = { status: event.params.turn.status, interruptToCompletionMs: interruptedAt ? Math.round(performance.now() - interruptedAt) : null }
      child.stdin.end()
    } else if (event.error) {
      child.stdin.end()
    }
  },
})
const summary = {
  model, firstPublicTextMs: firstTextAt, terminal: completed,
  exitCode: result.exitCode, signal: result.signal, elapsedMs: result.elapsedMs,
  timedOut: result.timedOut,
  methods: [...new Set(result.records.map(record => record.event.method).filter(Boolean))],
  rpcErrors: result.records.filter(record => record.event.error).map(record => ({ id: record.event.id, error: record.event.error })),
  artifact: result.directory,
}
save(join(auditDir, 'codex', 'app-server-summary.json'), summary)
console.log(JSON.stringify(summary, null, 2))
