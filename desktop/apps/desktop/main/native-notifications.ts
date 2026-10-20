import { mkdir, chmod, readFile, open, rename, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import type { NotificationPreferences, NotificationSettings, NotificationTestResult, RunSnapshot } from '../../../packages/protocol/src/index'

interface NativeNotificationHandle {
  once(event: string, listener: (...arguments_: unknown[]) => void): this
  show(): void
  close(): void
}

/** 单测可注入显式替身；生产仅使用 Electron 主进程的系统通知。 */
export interface NativeNotificationAPI {
  isSupported(): boolean
  create(options: { title: string; body: string; silent: boolean }): NativeNotificationHandle
}

function electronNotifications(): NativeNotificationAPI {
  // 主进程打包为 CJS。惰性加载使纯 Node 测试不会初始化 Electron。
  const { Notification } = require('electron') as typeof import('electron')
  return { isSupported: () => Notification.isSupported(), create: options => new Notification(options) }
}

const defaults: NotificationPreferences = { enabled: true, soundEnabled: true }
const terminalStatuses = new Set(['completed', 'failed', 'stopped', 'interrupted'])
const outcomeLabels: Record<string, string> = {
  supported: '方案获支持', supported_with_caveats: '有条件支持', rejected: '不推荐', no_consensus: '尚未达成共识',
  quorum_not_met: '有效票不足', insufficient_evidence: '证据不足', needs_human: '需要人工决定',
  converged: '意见已收敛', accepted: '方案获支持', completed: '流程完成', failed: '判定失败',
}
const deliveryFailure = '系统未能显示通知，请检查 TCView 的系统通知设置。'
const deliveryTimeout = '系统未在 10 秒内报告通知已显示；请检查 TCView 的系统通知设置和专注模式。'

function nativeFailure(error: unknown): string {
  // 仅根据关键词选择固定建议，不把原生错误的路径、正文或可能的凭据转发到界面。
  const description = typeof error === 'string' ? error : error instanceof Error ? error.message : ''
  if (/\b(?:signing|signature|signed|codesign|bundle)\b/iu.test(description)) return '系统未能显示通知，请使用已签名的 TCView 安装包，并确认应用标识有效。'
  if (/\b(?:permission|authorization|denied)\b/iu.test(description)) return '系统拒绝通知，请在系统设置中允许 TCView 发送通知。'
  return deliveryFailure
}

function filesystemFailure(error: unknown, action: string) {
  const code = error && typeof error === 'object' && 'code' in error && typeof error.code === 'string' && /^[A-Z0-9_]{1,32}$/.test(error.code) ? `（${error.code}）` : ''
  // 不把路径、设置内容或异常堆栈传到渲染进程。
  return `${action}${code}。`
}

function notificationContent(snapshot: RunSnapshot) {
  const mock = snapshot.providerMode === 'mock' || snapshot.decision?.mock
  const outcome = snapshot.decision?.outcome
  const failed = snapshot.status === 'failed' || (snapshot.status === 'completed' && outcome === 'failed')
  const status = failed ? '讨论失败' : snapshot.status === 'completed' ? '讨论已结束' : snapshot.status === 'stopped' ? '讨论已停止' : '讨论已中断'
  // 不发送 goal、材料、完整结论、CLI 错误或诊断信息。只显示短标题与静态分类。
  const title = Array.from(snapshot.title.replace(/[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/gu, ' ').replace(/\s+/gu, ' ').trim()).slice(0, 64).join('') || '未命名讨论'
  const conclusion = snapshot.status === 'stopped' || snapshot.status === 'interrupted'
    ? '可打开讨论查看恢复选项'
    : snapshot.status === 'failed' && outcome !== 'failed'
      ? '运行失败，请打开讨论查看详情'
      : outcome && Object.hasOwn(outcomeLabels, outcome)
        ? `${outcomeLabels[outcome]}（${outcome}）`
        : outcome ? '请打开讨论查看判定结果' : '请打开讨论查看详情'
  return { title: `TCView${mock ? ' · Mock' : ''} · ${status}`, body: `${title}\n${conclusion}` }
}

export class NativeNotifications {
  private preferences: NotificationPreferences = { ...defaults }
  private settingsError: string | undefined
  private lastDelivery: NotificationSettings['lastDelivery']
  private initialized = false
  private initialization: Promise<void> | undefined
  private saving: Promise<unknown> = Promise.resolve()
  private testing: Promise<NotificationTestResult> | undefined
  private readonly active = new Set<NativeNotificationHandle>()
  private readonly native: NativeNotificationAPI

  constructor(private readonly directory: string, private readonly onOpen: (runId: string) => void, native?: NativeNotificationAPI) {
    this.native = native ?? electronNotifications()
  }

  initialize(): Promise<void> {
    this.initialization ??= this.readSettings()
    return this.initialization
  }

  private async readSettings() {
    try {
      const value: unknown = JSON.parse(await readFile(join(this.directory, 'notification-settings.json'), 'utf8'))
      if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('invalid settings')
      const input = value as Record<string, unknown>
      if (input.version !== 1 || typeof input.enabled !== 'boolean' || typeof input.soundEnabled !== 'boolean') throw new Error('invalid settings')
      this.preferences = { enabled: input.enabled, soundEnabled: input.soundEnabled }
    } catch (error) {
      if (!(error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT')) {
        this.preferences = { enabled: false, soundEnabled: false }
        this.settingsError = filesystemFailure(error, '通知设置无法读取或格式无效，通知已安全停用；重新保存可修复')
      }
    }
    this.initialized = true
  }

  getSettings(): NotificationSettings {
    let supported = false
    let error = this.settingsError
    try { supported = this.native.isSupported() } catch { error ??= '无法检测系统通知能力。' }
    return { ...this.preferences, supported, ...(this.lastDelivery ? { lastDelivery: { ...this.lastDelivery } } : {}), ...(error ? { error } : {}) }
  }

  save(preferences: NotificationPreferences): Promise<NotificationSettings> {
    if (!preferences || typeof preferences.enabled !== 'boolean' || typeof preferences.soundEnabled !== 'boolean') return Promise.reject(new Error('通知与提示音设置必须是布尔值。'))
    const next = { enabled: preferences.enabled, soundEnabled: preferences.soundEnabled }
    const result = this.saving.then(async () => {
      await this.initialize()
      const temporary = join(this.directory, `.notification-settings-${randomUUID()}.tmp`)
      try {
        await mkdir(this.directory, { recursive: true, mode: 0o700 })
        await chmod(this.directory, 0o700)
        const file = await open(temporary, 'wx', 0o600)
        try {
          await file.writeFile(`${JSON.stringify({ version: 1, ...next })}\n`, 'utf8')
          await file.sync()
        } finally { await file.close() }
        await rename(temporary, join(this.directory, 'notification-settings.json'))
      } catch (error) {
        await rm(temporary, { force: true }).catch(() => undefined)
        throw new Error(filesystemFailure(error, '通知设置保存失败，原设置保持不变'))
      }
      this.preferences = next
      this.settingsError = undefined
      return this.getSettings()
    })
    this.saving = result.catch(() => undefined)
    return result
  }

  test(): Promise<NotificationTestResult> {
    this.testing ??= this.testOnce().finally(() => { this.testing = undefined })
    return this.testing
  }

  private async testOnce(): Promise<NotificationTestResult> {
    await this.initialize()
    const settings = this.getSettings()
    if (!settings.enabled) return { status: 'failed', error: '桌面通知已关闭，请先启用并保存。' }
    if (!settings.supported) return { status: 'unsupported', error: settings.error ?? '当前系统不支持桌面通知。' }
    return this.deliver({ title: 'TCView · 桌面通知测试', body: '这是一条系统通知测试；讨论结束后将通过此处提醒。没有发起模型请求。' })
  }

  showRun(snapshot: RunSnapshot): void {
    if (!this.initialized || !this.preferences.enabled || snapshot.deletedAt || !terminalStatuses.has(snapshot.status)) return
    if (!this.getSettings().supported) return
    void this.deliver(notificationContent(snapshot), snapshot.id)
  }

  private deliver(content: { title: string; body: string }, runId?: string): Promise<NotificationTestResult> {
    return new Promise(resolve => {
      let notification: NativeNotificationHandle | undefined
      let timer: ReturnType<typeof setTimeout> | undefined
      let settled = false
      const finish = (status: NotificationTestResult['status'], error?: string) => {
        const at = new Date().toISOString()
        this.lastDelivery = { status: status === 'shown' ? 'shown' : status === 'timeout' ? 'timeout' : 'failed', at, ...(error ? { error } : {}) }
        // 超时只结束等待；系统稍后仍可能显示通知，需要继续记录真实回执。
        if (settled) return
        settled = true
        if (timer) clearTimeout(timer)
        resolve({ status, ...(error ? { error } : {}) })
      }
      try {
        notification = this.native.create({ ...content, silent: !this.preferences.soundEnabled })
        const shownNotification = notification
        this.active.add(shownNotification)
        notification.once('show', () => { finish('shown') })
        notification.once('failed', (_event, error) => { this.active.delete(shownNotification); finish('failed', nativeFailure(error)) })
        notification.once('close', () => {
          this.active.delete(shownNotification)
          if (!settled) finish('failed', '通知在系统报告显示前已关闭。')
        })
        notification.once('click', () => {
          if (runId && /^[\w-]{1,128}$/u.test(runId)) this.onOpen(runId)
        })
        timer = setTimeout(() => {
          finish('timeout', deliveryTimeout)
          // 不撤销仍在系统队列中的全局提醒，保留引用和点击回调，等待真实生命周期事件。
        }, 10_000)
        // 无窗口且系统不回应时，不因测试计时器阻止进程退出。
        timer.unref()
        notification.show()
      } catch (error) {
        if (notification) this.active.delete(notification)
        finish('failed', nativeFailure(error))
      }
    })
  }
}
