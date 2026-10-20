import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { createInterface } from 'node:readline'
import { EventEmitter } from 'node:events'
import type { DesktopEvent } from '../../../packages/protocol/src/index'

interface Pending {
  resolve: (value: unknown) => void
  reject: (error: Error) => void
  timer: ReturnType<typeof setTimeout>
}

export class HostClient extends EventEmitter {
  private child: ChildProcessWithoutNullStreams
  private sequence = 0
  private pending = new Map<number, Pending>()
  private unavailable?: Error
  private exitPromise: Promise<void>

  constructor(binary: string, dataDir: string, delay: string, providerMode = 'cli') {
    super()
    const baseline=new Set(['PATH','HOME','USER','LOGNAME','SHELL','TMPDIR','TMP','TEMP','LANG','LC_ALL','LC_CTYPE','TZ','TERM','HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','NO_PROXY','http_proxy','https_proxy','all_proxy','no_proxy','SSL_CERT_FILE','SSL_CERT_DIR','NODE_EXTRA_CA_CERTS','XDG_CONFIG_HOME','XDG_CACHE_HOME','XDG_DATA_HOME','XDG_STATE_HOME','XDG_RUNTIME_DIR','GOOGLE_APPLICATION_CREDENTIALS','GOOGLE_CLOUD_PROJECT','GOOGLE_CLOUD_LOCATION'])
    this.child = spawn(binary, ['--data-dir', dataDir, '--mock-delay-ms', delay, '--provider-mode', providerMode], {
      shell: false, stdio: ['pipe', 'pipe', 'pipe'],
      env: Object.fromEntries(Object.entries(process.env).filter(([key,value]) => value !== undefined && (baseline.has(key)||/^(TCVIEW|OPENAI|CODEX|ANTHROPIC|XAI|GROK|GEMINI|GOOGLE_API)_/.test(key)))),
    })
    this.exitPromise = new Promise(resolve => this.child.once('close', () => resolve()))
    const lines = createInterface({ input: this.child.stdout, crlfDelay: Infinity })
    lines.on('line', line => {
      try {
        const value = JSON.parse(line) as { id?: number; method?: string; params?: DesktopEvent; result?: unknown; error?: { message: string } }
        if (value.method === 'event' && value.params) this.emit('event', value.params)
        else if (value.id !== undefined) {
          const pending = this.pending.get(value.id)
          if (!pending) return
          clearTimeout(pending.timer)
          this.pending.delete(value.id)
          if (value.error) pending.reject(new Error(value.error.message))
          else pending.resolve(value.result)
        }
      } catch {
        this.fail(new Error('Go 后台返回了无效的协议消息'))
        this.child.kill('SIGTERM')
      }
    })
    this.child.stderr.on('data', data => process.stderr.write(`[tcview-host] ${String(data)}`))
    this.child.once('error', error => this.fail(new Error(`无法启动 Go 后台：${error.message}`)))
    this.child.once('exit', (code, signal) => this.fail(new Error(`Go 后台已退出（${signal ?? code}）`)))
    this.child.stdin.on('error', error => this.fail(error))
  }

  request<T>(method: string, params: unknown = {}): Promise<T> {
    if (this.unavailable) return Promise.reject(this.unavailable)
    const id = ++this.sequence
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`Go 后台请求超时：${method}`))
      }, ['provider.check', 'topic.analyze'].includes(method) ? 150_000 : method === 'provider.models' ? 90_000 : 30_000)
      this.pending.set(id, { resolve: value => resolve(value as T), reject, timer })
      this.child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id, method, params })}\n`, error => {
        if (error) this.fail(error)
      })
    })
  }

  private fail(error: Error) {
    if (this.unavailable) return
    this.unavailable = error
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer)
      pending.reject(error)
    }
    this.pending.clear()
    this.emit('disconnect', error.message)
  }

  async close() {
    this.child.stdin.end()
    const force = setTimeout(() => this.child.kill('SIGKILL'), 4_000)
    await this.exitPromise
    clearTimeout(force)
  }
}

