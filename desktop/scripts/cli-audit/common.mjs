import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve, join } from 'node:path'

export const root = resolve(import.meta.dirname, '../..')
const batch = new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Kuala_Lumpur',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}).format(new Date()).replace(' ','T').replaceAll(':','-')
export const auditDir = resolve(process.env.TCVIEW_AUDIT_DIR ?? join(root, '.tcview/cli-audit',batch))
mkdirSync(auditDir, { recursive: true, mode: 0o700 })
export const packageText = readFileSync(join(root, 'package.json'), 'utf8')
export const goText = readFileSync(join(root, 'services/host/go.mod'), 'utf8')
const pkg = JSON.parse(packageText)
export const expected = {
  project_name: pkg.name,
  electron_version: pkg.devDependencies.electron,
  react_version: pkg.dependencies.react,
  go_version: goText.match(/^go\s+(\S+)/m)[1],
  consensus_version: goText.match(/github\.com\/suchasplus\/til-consensus\s+(\S+)/)[1],
  scripts: Object.keys(pkg.scripts).sort(),
}
export const schema = {
  type: 'object', additionalProperties: false,
  required: [...Object.keys(expected), 'risk'],
  properties: {
    project_name: { type: 'string' }, electron_version: { type: 'string' }, react_version: { type: 'string' },
    go_version: { type: 'string' }, consensus_version: { type: 'string' },
    scripts: { type: 'array', items: { type: 'string' } }, risk: { type: 'string' },
  },
}
export const prompt = `检查下面真实本地项目的依赖和脚本。只根据提供的文件摘取结果，risk 用简体中文一句话指出 M0 接入真实 CLI 的一个具体风险。禁止调用工具、联网、执行命令或读取其他文件。返回符合附带 JSON Schema 的 JSON 对象，不要 Markdown。scripts 要包含 package.json 的所有 script 键。\n\nJSON Schema:\n${JSON.stringify(schema)}\n\n<package.json>\n${packageText}\n</package.json>\n\n<services/host/go.mod>\n${goText}\n</services/host/go.mod>`

export function save(path, value) {
  writeFileSync(path, typeof value === 'string' ? value : JSON.stringify(value, null, 2) + '\n', { mode: 0o600 })
}
save(join(auditDir, 'input-manifest.json'), { source: ['package.json', 'services/host/go.mod'], sha256: createHash('sha256').update(packageText + goText).digest('hex'), expected })
save(join(auditDir, 'schema.json'), schema)
save(join(auditDir, 'prompt.txt'), prompt)

export function validate(value) {
  const errors = []
  if (!value || typeof value !== 'object') return { ok: false, errors: ['没有结构化对象'] }
  for (const [key, target] of Object.entries(expected)) {
    if (key === 'scripts') {
      if (JSON.stringify([...(value.scripts ?? [])].sort()) !== JSON.stringify(target)) errors.push('scripts 与真实 package.json 不同')
    } else if (value[key] !== target) errors.push(`${key}: ${JSON.stringify(value[key])} != ${JSON.stringify(target)}`)
  }
  if (typeof value.risk !== 'string' || !value.risk.trim()) errors.push('缺少 risk')
  return { ok: errors.length === 0, errors }
}

function environment() {
  const exact = new Set(['PATH','HOME','USER','LOGNAME','SHELL','TMPDIR','TMP','TEMP','LANG','LC_ALL','LC_CTYPE','TZ','TERM','HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','NO_PROXY','http_proxy','https_proxy','all_proxy','no_proxy','SSL_CERT_FILE','SSL_CERT_DIR','NODE_EXTRA_CA_CERTS','XDG_CONFIG_HOME','XDG_CACHE_HOME','XDG_DATA_HOME','XDG_STATE_HOME','XDG_RUNTIME_DIR','GOOGLE_APPLICATION_CREDENTIALS','GOOGLE_CLOUD_PROJECT','GOOGLE_CLOUD_LOCATION'])
  return Object.fromEntries(Object.entries(process.env).filter(([key, value]) => value !== undefined && (exact.has(key) || /^(OPENAI|CODEX|ANTHROPIC|XAI|GROK|GEMINI|GOOGLE_API)_/.test(key))))
}

export async function probe(provider, name, command, args, options = {}) {
  const directory = join(auditDir, provider, name)
  if (existsSync(join(directory, 'meta.json'))) throw new Error(`已有审计记录，拒绝覆盖或重复调用：${directory}；请使用新的 label 或 TCVIEW_AUDIT_DIR`)
  mkdirSync(directory, { recursive: true, mode: 0o700 })
  const startedAt = new Date().toISOString()
  const started = performance.now()
  const records = []
  let stdout = '', stderr = '', firstStdoutMs = null, partial = ''
  let timedOut = false, cancellationRequested = false, cancellationMs = null
  const child = spawn(command, args, { cwd: directory, env: { ...environment(), ...options.env }, detached: process.platform !== 'win32', stdio: ['pipe','pipe','pipe'] })
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  const signal = type => { try { process.kill(process.platform === 'win32' ? child.pid : -child.pid, type) } catch {} }
  const interrupt = () => {
    if (cancellationRequested) return
    cancellationRequested = true
    cancellationMs = performance.now() - started
    signal('SIGINT')
    escalation.push(setTimeout(() => signal('SIGTERM'), 2500), setTimeout(() => signal('SIGKILL'), 6000))
  }
  const escalation = []
  const timer = setTimeout(() => { timedOut = true; interrupt() }, options.timeoutMs ?? 120_000)
  const cancelTimer = options.cancelAfterMs ? setTimeout(interrupt, options.cancelAfterMs) : undefined
  child.stdout.on('data', bytes => {
    if (firstStdoutMs === null) firstStdoutMs = performance.now() - started
    stdout += bytes.toString('utf8')
    partial += bytes.toString('utf8')
    const lines = partial.split('\n')
    partial = lines.pop() ?? ''
    for (const line of lines) {
      if (!line.trim()) continue
      try {
        const event = JSON.parse(line)
        records.push({ ms: Math.round(performance.now() - started), event })
        options.onEvent?.(event, child, interrupt)
      } catch {}
    }
    if (options.cancelWhen && records.some(record => options.cancelWhen(record.event))) interrupt()
    if (stdout.length > 8 * 1024 * 1024) interrupt()
  })
  child.stderr.on('data', bytes => { stderr += bytes.toString('utf8') })
  child.stdin.on('error', () => {})
  if (options.keepStdinOpen) child.stdin.write(options.stdin ?? '')
  else child.stdin.end(options.stdin ?? '')
  const result = await new Promise(resolve => {
    child.once('error', error => resolve({ exitCode: null, signal: null, spawnError: error.message }))
    child.once('close', (exitCode, signal) => resolve({ exitCode, signal }))
  })
  clearTimeout(timer)
  clearTimeout(cancelTimer)
  escalation.forEach(clearTimeout)
  const meta = { provider, name, command, args, startedAt, elapsedMs: Math.round(performance.now()-started), firstStdoutMs: firstStdoutMs === null ? null : Math.round(firstStdoutMs), timedOut, cancellationRequested, cancellationMs: cancellationMs === null ? null : Math.round(cancellationMs), ...result, stdoutBytes: Buffer.byteLength(stdout), stderrBytes: Buffer.byteLength(stderr), eventCount: records.length }
  save(join(directory, 'stdout.txt'), stdout)
  save(join(directory, 'stderr.txt'), stderr)
  save(join(directory, 'events.json'), records)
  save(join(directory, 'meta.json'), meta)
  return { ...meta, directory, stdout, stderr, records, child }
}
