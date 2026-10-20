import { test, expect, _electron as electron, type ElectronApplication, type Page } from '@playwright/test'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'

type CapturedNotification = { title: string; body: string; silent: boolean }
type NativeFixture = { captured: CapturedNotification[]; handles: import('electron').Notification[] }

// Electron、preload、Go 后台与 Mock 讨论真实运行；只有系统通知投递使用显式替身。
// 这些断言不代表操作系统实际显示横幅或播放声音。
async function launchMock(directory: string, delay = '180') {
  const env: Record<string, string> = {
    ...Object.fromEntries(Object.entries(process.env).filter((entry): entry is [string, string] => entry[1] !== undefined)),
    TCVIEW_DATA_DIR: join(directory, 'host'), TCVIEW_USER_DATA_DIR: join(directory, 'electron'),
    TCVIEW_PROVIDER_MODE: 'mock', TCVIEW_MOCK_DELAY_MS: delay,
  }
  for (const provider of ['CODEX', 'CLAUDE', 'GROK', 'AGY']) {
    for (const field of ['MODEL', 'REASONING', 'EFFORT', 'ENABLED']) delete env[`TCVIEW_${provider}_${field}`]
  }
  delete env.ELECTRON_RUN_AS_NODE
  const app = await electron.launch({ args: [resolve('.')], env })
  try {
    const page = await app.firstWindow()
    await app.evaluate(({ Notification }) => {
      const fixture: NativeFixture = { captured: [], handles: [] }
      Reflect.set(globalThis, '__tcviewExplicitNativeNotificationFixture', fixture)
      Notification.isSupported = () => true
      Notification.prototype.show = function () {
        fixture.captured.push({ title: this.title, body: this.body, silent: this.silent })
        fixture.handles.push(this)
        queueMicrotask(() => this.emit('show'))
      }
    })
    await prepareTestWindows(app, { preserveNotificationSettings: true })
    await setViewportAndWait(page)
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    return { app, page }
  } catch (failure) { await app.close(); throw failure }
}

async function captured(app: ElectronApplication): Promise<CapturedNotification[]> {
  return app.evaluate(() => (Reflect.get(globalThis, '__tcviewExplicitNativeNotificationFixture') as NativeFixture).captured)
}

async function enableNotifications(page: Page) {
  await page.evaluate(() => window.tcview.saveNotificationSettings({ enabled: true, soundEnabled: false }))
}

async function startMock(page: Page, goal: string) {
  return page.evaluate(goal => window.tcview.startRun({ goal, mode: 'free_debate', idempotencyKey: crypto.randomUUID() }), goal)
}

test('桌面通知与声音设置保存、重启和测试投递（真实 Electron/Go，原生投递显式替身）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-notification-settings-'))
  let app: ElectronApplication | undefined
  try {
    let launched = await launchMock(directory)
    app = launched.app
    let page = launched.page
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    await page.getByTestId('notification-settings-button').click()
    const enabled = page.getByTestId('notification-enabled')
    const sound = page.getByTestId('notification-sound-enabled')
    const send = page.getByTestId('test-notification')
    const save = page.getByTestId('save-notification-settings')
    await expect(enabled).toBeChecked()
    await expect(sound).toBeChecked()
    await send.click()
    await expect(page.getByTestId('notification-test-result')).toContainText('shown')
    expect(await captured(app)).toEqual([expect.objectContaining({ title: 'TCView · 桌面通知测试', silent: false })])

    await sound.uncheck()
    await expect(page.getByTestId('notification-save-state')).toContainText('有未保存修改')
    await expect(send).toBeDisabled()
    expect(await captured(app)).toHaveLength(1)
    await save.click()
    await expect(send).toBeEnabled()
    await send.click()
    await expect(page.getByTestId('notification-test-result')).toContainText('shown')
    expect(await captured(app)).toEqual([
      expect.objectContaining({ silent: false }), expect.objectContaining({ silent: true }),
    ])
    // 通知测试不创建讨论，也不触发任何 Mock/真实模型工作流。
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)

    await enabled.uncheck()
    await save.click()
    await expect(page.getByTestId('notification-save-state')).toContainText('已保存：系统通知关闭')
    await expect(send).toBeDisabled()
    await expect(sound).toBeDisabled()
    expect(await page.evaluate(() => window.tcview.testNotification())).toMatchObject({ status: 'failed' })
    expect(await captured(app)).toHaveLength(2)
    await app.close()
    app = undefined

    launched = await launchMock(directory)
    app = launched.app
    page = launched.page
    expect(await page.evaluate(() => window.tcview.getNotificationSettings())).toMatchObject({ enabled: false, soundEnabled: false, supported: true })
    expect(await captured(app)).toHaveLength(0)
    await page.getByTestId('notification-settings-button').click()
    await expect(page.getByTestId('notification-enabled')).not.toBeChecked()
    await expect(page.getByTestId('notification-sound-enabled')).not.toBeChecked()
    await expect(page.getByTestId('test-notification')).toBeDisabled()
  } finally {
    await app?.close()
    await rm(directory, { recursive: true, force: true })
  }
})

test('隐藏窗口及非当前讨论结束只通知一次，停止恢复按新 attempt 再通知（显式 Mock/原生替身）', async () => {
  test.setTimeout(90_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-notification-runs-'))
  const { app, page } = await launchMock(directory)
  try {
    await enableNotifications(page)
    // 直接走真实 preload/RPC 发起，renderer 保持新建讨论页，验证非当前 run。
    const background = await startMock(page, '显式 Mock：切换到其他应用后仍能收到讨论结束通知。')
    await expect(page.locator('.breadcrumb strong')).toHaveText('新建讨论')
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].hide())
    expect(await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].isVisible())).toBe(false)
    await expect.poll(() => captured(app), { timeout: 40_000 }).toHaveLength(1)
    expect((await captured(app))[0]).toMatchObject({ title: expect.stringContaining('Mock'), body: expect.stringContaining(background.title), silent: true })
    expect(await page.evaluate(id => window.tcview.getRun(id), background.id)).toMatchObject({ status: 'completed' })
    await page.evaluate(async id => {
      await window.tcview.getRun(id)
      await window.tcview.getEvents(id, 0)
      await window.tcview.reconnectHost()
      await window.tcview.getRun(id)
    }, background.id)
    expect(await captured(app)).toHaveLength(1)

    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].show())
    const stoppable = await startMock(page, '显式 Mock：停止后从检查点恢复，并再次报告最终状态。')
    await expect.poll(() => page.evaluate(async id => (await window.tcview.getRun(id)).messages.some(message => message.kind === 'proposal'), stoppable.id)).toBe(true)
    await page.evaluate(id => window.tcview.stopRun(id), stoppable.id)
    await expect.poll(() => page.evaluate(async id => (await window.tcview.getRun(id)).status, stoppable.id)).toBe('stopped')
    await expect.poll(() => captured(app)).toHaveLength(2)
    expect((await captured(app))[1].title).toContain('已停止')
    const resumed = await page.evaluate(id => window.tcview.resumeRun({ runId: id, idempotencyKey: crypto.randomUUID() }), stoppable.id)
    expect(resumed.attemptId).not.toBe(stoppable.attemptId)
    await expect.poll(() => captured(app), { timeout: 40_000 }).toHaveLength(3)
    expect((await captured(app))[2].title).toContain('已结束')
    await page.evaluate(id => window.tcview.getRun(id), stoppable.id)
    expect(await captured(app)).toHaveLength(3)
  } finally {
    await app.close()
    await rm(directory, { recursive: true, force: true })
  }
})

test('macOS 关闭窗口后继续运行，通知点击重建窗口并打开指定讨论（显式 Mock/原生替身）', async () => {
  test.skip(process.platform !== 'darwin', '关闭全部窗口仍保持应用运行属于 macOS 生命周期')
  test.setTimeout(90_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-notification-click-'))
  const { app, page } = await launchMock(directory)
  try {
    await enableNotifications(page)
    const target = await startMock(page, '显式 Mock：通知点击目标讨论。')
    await expect.poll(() => captured(app), { timeout: 40_000 }).toHaveLength(1)
    // 留下更晚的讨论，避免“默认打开最近记录”冒充正确的通知导航。
    const latest = await startMock(page, '显式 Mock：最新讨论不应覆盖通知点击目标。')
    expect(latest.id).not.toBe(target.id)
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close())
    await expect.poll(() => app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows().length)).toBe(0)
    await expect.poll(() => captured(app), { timeout: 40_000 }).toHaveLength(2)
    expect((await captured(app))[1].body).toContain(latest.title)

    const opened = app.waitForEvent('window')
    await app.evaluate(() => {
      const fixture = Reflect.get(globalThis, '__tcviewExplicitNativeNotificationFixture') as NativeFixture
      fixture.handles[0].emit('click')
    })
    const reopened = await opened
    await prepareTestWindows(app, { preserveNotificationSettings: true })
    await setViewportAndWait(reopened)
    await expect(reopened.getByTestId('connection-status')).toContainText('已连接')
    await expect(reopened.locator('.breadcrumb strong')).toHaveText(target.title)
    await expect(reopened.getByTestId('message-list')).toContainText(target.goal)
    await expect(reopened.getByTestId('run-status')).toContainText('已完成')
    expect(await captured(app)).toHaveLength(2)
  } finally {
    await app.close()
    await rm(directory, { recursive: true, force: true })
  }
})
