import { useEffect, useId, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import type {
  DesktopAPI,
  NotificationPreferences,
  NotificationSettings as SavedNotificationSettings,
  NotificationTestResult,
} from '../../../packages/protocol/src/index'
import './notification-settings.css'

function failureMessage(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}

function deliveryTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

export function NotificationSettings({ api, onClose }: { api: DesktopAPI; onClose: () => void }) {
  const headingId = useId()
  const introId = useId()
  const dialog = useRef<HTMLDivElement>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const mounted = useRef(false)
  const requestInFlight = useRef(false)
  const [saved, setSaved] = useState<SavedNotificationSettings | null>(null)
  const [draft, setDraft] = useState<NotificationPreferences>({ enabled: true, soundEnabled: true })
  const [busy, setBusy] = useState<'loading' | 'saving' | 'testing' | null>('loading')
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<NotificationTestResult | null>(null)

  const loadSettings = async () => {
    if (requestInFlight.current) return
    requestInFlight.current = true
    setBusy('loading')
    setError(null)
    try {
      const settings = await api.getNotificationSettings()
      if (!mounted.current) return
      setSaved(settings)
      setDraft({ enabled: settings.enabled, soundEnabled: settings.soundEnabled })
    } catch (failure) {
      if (mounted.current) setError(`读取通知设置失败：${failureMessage(failure)}`)
    } finally {
      requestInFlight.current = false
      if (mounted.current) setBusy(null)
    }
  }

  useEffect(() => {
    mounted.current = true
    closeButton.current?.focus()
    void loadSettings()
    return () => { mounted.current = false }
  }, [])

  const dirty = saved !== null && (draft.enabled !== saved.enabled || draft.soundEnabled !== saved.soundEnabled)

  const saveSettings = async () => {
    if (!saved || (!dirty && !saved.error) || requestInFlight.current) return
    requestInFlight.current = true
    setBusy('saving')
    setError(null)
    setNotice(null)
    setTestResult(null)
    try {
      const settings = await api.saveNotificationSettings(draft)
      if (!mounted.current) return
      setSaved(settings)
      setDraft({ enabled: settings.enabled, soundEnabled: settings.soundEnabled })
      setNotice('通知设置已保存，之后结束的讨论将使用此设置。')
    } catch (failure) {
      if (mounted.current) setError(`保存通知设置失败：${failureMessage(failure)}`)
    } finally {
      requestInFlight.current = false
      if (mounted.current) setBusy(null)
    }
  }

  const sendTest = async () => {
    if (!saved?.enabled || !saved.supported || dirty || requestInFlight.current) return
    requestInFlight.current = true
    setBusy('testing')
    setError(null)
    setNotice(null)
    setTestResult(null)
    try {
      const result = await api.testNotification()
      if (!mounted.current) return
      setTestResult(result)
      // 投递结果读取失败不应掩盖已得到的测试结果。
      try {
        const settings = await api.getNotificationSettings()
        if (mounted.current) setSaved(settings)
      } catch { /* 下一次打开设置时重新读取最近投递状态。 */ }
    } catch (failure) {
      if (mounted.current) setError(`测试通知失败：${failureMessage(failure)}`)
    } finally {
      requestInFlight.current = false
      if (mounted.current) setBusy(null)
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClose()
      return
    }
    if (event.key !== 'Tab') return
    const controls = [...(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), [tabindex="0"]') ?? [])]
    const first = controls[0]
    const last = controls.at(-1)
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }

  const resultText = testResult?.status === 'shown'
    ? '系统已接收测试通知；是否显示横幅或播放声音仍由系统设置决定。'
    : testResult?.status === 'unsupported'
      ? '当前系统通知接口不可用，未发送测试通知。'
      : testResult?.status === 'timeout'
        ? '等待系统投递结果超时，无法确认通知是否显示。'
        : '测试通知投递失败。'

  return <div className="notification-settings-overlay">
    <button className="notification-settings-backdrop" tabIndex={-1} aria-label="关闭桌面通知设置背景" onClick={onClose} />
    <div ref={dialog} className="notification-settings-dialog" role="dialog" aria-modal="true" aria-labelledby={headingId} aria-describedby={introId} onKeyDown={handleKeyDown} data-testid="notification-settings">
      <header className="notification-settings-heading">
        <div><span className="notification-settings-eyebrow">SYSTEM NOTIFICATIONS</span><h2 id={headingId}>桌面通知</h2></div>
        <button ref={closeButton} className="notification-settings-close" aria-label="关闭桌面通知设置" onClick={onClose}><svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="m5 5 10 10M15 5 5 15" /></svg></button>
      </header>
      <div className="notification-settings-content">
        <p id={introId} className="notification-settings-intro">讨论成功、失败、停止或中断时，在桌面发送系统通知。即使切换到其他应用，也能收到运行结束提示。</p>
        {busy === 'loading' && <p role="status" className="notification-settings-loading">正在读取通知设置…</p>}
        {saved && <>
          <div className="notification-settings-capability" data-supported={saved.supported}>
            <strong>{saved.supported ? '系统通知接口可用' : '系统通知接口不可用'}</strong>
            <span>{saved.supported ? '接口可用仅表示可以请求发送通知，不代表系统已经授权或一定会显示。' : '偏好仍可保存；当前环境无法发送系统通知。'}</span>
          </div>
          {saved.error && <div className="notification-settings-error" role="status" data-testid="notification-settings-warning"><strong>通知设置或系统能力检测异常</strong><p>{saved.error}</p><p>{saved.enabled ? '当前系统通知能力无法确认。' : '当前通知已安全停用。'}可直接点击“保存设置”重新保存当前值，也可调整开关后保存；系统能力是否可用仍取决于当前环境。</p></div>}
          <div className="notification-settings-options">
            <label className="notification-settings-option">
              <span><strong>讨论结束时发送系统通知</strong><small>保存后立即生效，也适用于正在运行的讨论结束时。</small></span>
              <span className="notification-settings-switch"><input type="checkbox" role="switch" checked={draft.enabled} disabled={busy !== null} onChange={(event) => { setDraft((current) => ({ ...current, enabled: event.target.checked })); setNotice(null) }} data-testid="notification-enabled" /><span aria-hidden="true" /></span>
            </label>
            <label className="notification-settings-option" data-disabled={!draft.enabled}>
              <span><strong>播放提示音</strong><small>{draft.enabled ? '随系统通知请求播放提示音。' : '开启系统通知后可调整；当前声音选择会保留。'}</small></span>
              <span className="notification-settings-switch"><input type="checkbox" role="switch" checked={draft.soundEnabled} disabled={!draft.enabled || busy !== null} onChange={(event) => { setDraft((current) => ({ ...current, soundEnabled: event.target.checked })); setNotice(null) }} data-testid="notification-sound-enabled" /><span aria-hidden="true" /></span>
            </label>
          </div>
          <p className="notification-settings-save-state" data-testid="notification-save-state">{dirty ? '有未保存修改。关闭此页面会丢弃修改；请先保存，再发送测试通知。' : saved.error ? '设置存在异常，可直接重新保存当前值；关闭此页面不会自动保存。' : `已保存：系统通知${saved.enabled ? '开启' : '关闭'}，提示音${saved.soundEnabled ? '开启' : '关闭'}。`}</p>
          <section className="notification-settings-test" aria-label="通知测试">
            <div><h3>测试系统通知</h3><p>不调用模型，不消耗服务额度。将按已保存设置发送一条系统通知{saved.soundEnabled ? '并请求播放提示音' : '，不请求播放提示音'}。{!saved.enabled ? '请先开启并保存系统通知。' : ''}</p></div>
            <button className="notification-settings-secondary" disabled={busy !== null || dirty || !saved.enabled || !saved.supported} onClick={() => void sendTest()} data-testid="test-notification" aria-busy={busy === 'testing'}>{busy === 'testing' ? '正在测试…' : '发送测试通知'}</button>
          </section>
        </>}
        <div className="notification-settings-feedback" aria-live="polite" aria-atomic="true">
          {error && <p className="notification-settings-error" role="status">{error}</p>}
          {notice && <p className="notification-settings-notice" role="status">{notice}</p>}
          {testResult && <div className={`notification-settings-result ${testResult.status === 'shown' ? 'is-received' : 'has-warning'}`} role="status" data-testid="notification-test-result"><strong>测试结果：<code>{testResult.status}</code></strong><p>{resultText}</p>{testResult.error && <p className="notification-settings-result-detail">{testResult.error}</p>}</div>}
        </div>
        {saved?.lastDelivery && <div className="notification-settings-last-delivery" data-testid="notification-last-delivery"><strong>最近投递：<code>{saved.lastDelivery.status}</code></strong><span>{deliveryTime(saved.lastDelivery.at)}</span><p>{saved.lastDelivery.status === 'shown' ? '系统已接收通知请求，实际显示与声音由系统控制。' : saved.lastDelivery.status === 'timeout' ? '未收到系统回执，投递结果尚未确认。' : '通知未能完成投递。'}{saved.lastDelivery.error ? ` ${saved.lastDelivery.error}` : ''}</p></div>}
        <p className="notification-settings-system-note">macOS 通知横幅位于屏幕右上角，位置和样式由系统控制。通知权限、专注模式及静音设置可能影响显示与声音；TCView 不会修改这些系统设置。</p>
      </div>
      <footer className="notification-settings-footer"><span>仅保存到 TCView 应用数据目录</span><div>{!saved && busy === null && <button className="notification-settings-secondary" onClick={() => void loadSettings()}>重新读取</button>}<button className="notification-settings-secondary" onClick={onClose}>关闭</button><button className="notification-settings-primary" disabled={!saved || (!dirty && !saved.error) || busy !== null} onClick={() => void saveSettings()} data-testid="save-notification-settings" aria-busy={busy === 'saving'}>{busy === 'saving' ? '正在保存…' : '保存设置'}</button></div></footer>
    </div>
  </div>
}
