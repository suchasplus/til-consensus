import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { auditDir, prompt, probe, validate, save } from './common.mjs'

const capture = join(auditDir, 'codex-final.json')
const args = ['exec', '--sandbox', 'read-only', '-c', 'approval_policy="never"', '--json', '--color', 'never', '--skip-git-repo-check', '--ephemeral']
const result = await probe('codex', 'schema', 'codex', [...args, '--output-schema', join(auditDir, 'schema.json'), '--output-last-message', capture, '-'], { stdin: prompt, timeoutMs: 120_000 })
let output, parseError
try { output = JSON.parse(readFileSync(capture, 'utf8')) } catch (error) { parseError = error.message }
const events = result.records.map(record => record.event)
const summary = {
  version: '0.163.0',
  schema: { exitCode: result.exitCode, signal: result.signal, elapsedMs: result.elapsedMs, firstStdoutMs: result.firstStdoutMs, validation: validate(output), output, parseError, eventTypes: [...new Set(events.map(event => event.type))], terminal: events.filter(event => ['turn.completed','turn.failed','error'].includes(event.type)), artifact: result.directory },
}
save(join(auditDir, 'codex', 'summary.json'), summary)
console.log(JSON.stringify(summary, null, 2))
