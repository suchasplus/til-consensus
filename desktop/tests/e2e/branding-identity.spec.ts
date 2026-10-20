import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'
import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type ElectronApplication } from '@playwright/test'
import { createHash } from 'node:crypto'
import { mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, relative, resolve } from 'node:path'

const evidenceDirectory = resolve('.tcview/branding-validation', new Date().toISOString().replaceAll(':', '-'))

// 显式 Mock 模型，实际经过 Electron、preload、Go 后台和 SQLite。
// 在加载主进程前把 appData 指向临时目录；所有新旧数据均为隔离测试夹具。
async function launchIsolated(launcher: string, overrides: Record<string, string> = {}) {
  const env: Record<string, string> = {
    ...Object.fromEntries(Object.entries(process.env).filter((entry): entry is [string, string] => entry[1] !== undefined)),
  }
  for (const key of Object.keys(env)) {
    if (/^(TCVIEW_|NEWTC_)/.test(key) || key === 'ELECTRON_RUN_AS_NODE') delete env[key]
  }
  Object.assign(env, { TCVIEW_PROVIDER_MODE: 'mock', TCVIEW_MOCK_DELAY_MS: '15' }, overrides)
  const app = await electron.launch({ args: [launcher], env })
  try {
    const page = await app.firstWindow()
    await prepareTestWindows(app)
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await setViewportAndWait(page, { width: 1440, height: 920 })
    return { app, page }
  } catch (failure) { await app.close(); throw failure }
}

async function makeLauncher(directory: string) {
  const appData = join(directory, 'application-support')
  const launcher = join(directory, 'launch-isolated.cjs')
  await mkdir(appData)
  await writeFile(launcher, `const { app } = require('electron');\napp.setPath('appData', ${JSON.stringify(appData)});\nrequire(${JSON.stringify(resolve('dist/main/index.cjs'))});\n`)
  return { appData, launcher }
}

async function directorySnapshot(directory: string): Promise<Record<string, string>> {
  const entries: Record<string, string> = {}
  async function visit(path: string) {
    for (const entry of await readdir(path, { withFileTypes: true })) {
      const file = join(path, entry.name)
      if (entry.isDirectory()) await visit(file)
      else if (entry.isFile()) entries[relative(directory, file)] = createHash('sha256').update(await readFile(file)).digest('hex')
    }
  }
  await visit(directory)
  return Object.fromEntries(Object.entries(entries).sort(([left], [right]) => left.localeCompare(right)))
}

// 旧标识仅在本文件作为拒绝兼容的负例出现，应用不得读取、迁移或写入这些位置。
test('TCView 使用独立默认身份，不导入旧目录、旧环境变量和浏览器键；新数据跨重启持久化（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-branding-identity-'))
  const { appData, launcher } = await makeLauncher(directory)
  const legacyUserData = join(appData, 'NewTC')
  const currentUserData = join(appData, 'TCView')
  let app: ElectronApplication | undefined
  try {
    // 仅测试夹具准备阶段，通过显式新变量把真实 SQLite 数据写入旧名称目录。
    // 后续正常默认启动必须忽略该目录，不能把它当作隐式兼容来源。
    let launched = await launchIsolated(launcher, { TCVIEW_USER_DATA_DIR: legacyUserData, TCVIEW_DATA_DIR: join(legacyUserData, 'host') })
    app = launched.app
    let page = launched.page
    await page.evaluate(async () => {
      const providers = await window.tcview.getProviderSettings()
      await window.tcview.saveProviderSettings(providers.map(provider => ({
        ...provider.effective!, id: provider.id,
        model: provider.id === 'codex' ? 'fixture-legacy-codex-selector' : provider.effective!.model,
        enabled: provider.id !== 'grok',
      })))
    })
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：旧目录中的讨论不应被 TCView 自动导入。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const legacyRun = (await page.evaluate(() => window.tcview.listRuns()))[0]
    expect((await page.evaluate(id => window.tcview.getRun(id), legacyRun.id)).decision?.mock).toBe(true)
    await page.evaluate(() => {
      localStorage.setItem('newtc.layout.v1', JSON.stringify({ left: 334, right: 448 }))
      localStorage.setItem('newtc.side-panels.v1', JSON.stringify({ leftCollapsed: true, rightCollapsed: true }))
    })
    await app.close()
    app = undefined
    await writeFile(join(legacyUserData, 'do-not-migrate.txt'), '旧数据必须保留原样\n', { mode: 0o600 })
    expect((await stat(join(legacyUserData, 'host/desktop.sqlite'))).size).toBeGreaterThan(0)
    const legacyBefore = await directorySnapshot(legacyUserData)

    const ignoredLegacyEnvironment = {
      NEWTC_USER_DATA_DIR: legacyUserData,
      NEWTC_DATA_DIR: join(legacyUserData, 'host'),
      NEWTC_RENDERER_URL: 'https://old-renderer.invalid/must-not-load',
      NEWTC_CODEX_MODEL: 'fixture-legacy-environment-must-not-apply',
      NEWTC_GROK_REASONING: 'fixture-legacy-unsupported-effort',
      NEWTC_GROK_ENABLED: 'false',
    }
    launched = await launchIsolated(launcher, ignoredLegacyEnvironment)
    app = launched.app
    page = launched.page
    const identity = await app.evaluate(({ app, BrowserWindow, Menu, ipcMain }) => {
      const handlers = Reflect.get(ipcMain, '_invokeHandlers')
      if (!(handlers instanceof Map)) throw new Error('当前 Electron 未提供可检查的 IPC handler 注册表')
      return {
        name: app.getName(), appData: app.getPath('appData'), userData: app.getPath('userData'),
        sessionData: app.getPath('sessionData'),
        titles: BrowserWindow.getAllWindows().map(window => window.getTitle()),
        menuLabels: Menu.getApplicationMenu()?.items.map(item => item.label),
        ipcChannels: Array.from(handlers.keys()) as string[],
      }
    })
    expect(identity.name).toBe('TCView')
    expect(identity.appData).toBe(appData)
    expect(identity.userData).toBe(currentUserData)
    expect(identity.sessionData).toBe(currentUserData)
    expect(identity.titles).toHaveLength(1)
    expect(identity.titles[0]).toMatch(/^TCView/)
    if (process.platform === 'darwin') expect(identity.menuLabels?.[0]).toBe('TCView')
    expect(identity.ipcChannels).toContain('tcview:hello')
    expect(identity.ipcChannels.some(channel => channel.startsWith('newtc:'))).toBe(false)
    expect(await page.evaluate(() => ({ current: typeof window.tcview, legacy: typeof Reflect.get(window, 'newtc') }))).toEqual({ current: 'object', legacy: 'undefined' })
    await expect(page).toHaveTitle(/^TCView/)
    await expect(page.locator('.brand strong')).toHaveText('TCView')
    await expect(page.locator('.brand')).toHaveAttribute('title', 'til-consensus 桌面客户端')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    const defaults = await page.evaluate(() => window.tcview.getProviderSettings())
    expect(defaults.every(provider => provider.saved === undefined)).toBe(true)
    expect(defaults.find(provider => provider.id === 'codex')?.effective?.model).not.toContain('fixture-legacy')
    expect(defaults.find(provider => provider.id === 'codex')?.sources.model.kind).toBe('default')
    expect(defaults.find(provider => provider.id === 'grok')?.effective?.enabled).toBe(true)
    expect(defaults.find(provider => provider.id === 'grok')?.sources.reasoning.kind).toBe('default')
    expect(await page.evaluate(() => localStorage.getItem('newtc.layout.v1'))).toBeNull()
    expect(await page.evaluate(() => localStorage.getItem('newtc.side-panels.v1'))).toBeNull()

    // 同一新浏览器存储中即使出现旧键，也不得将其读作当前设置。
    await page.addInitScript(() => {
      localStorage.setItem('newtc.layout.v1', JSON.stringify({ left: 334, right: 448 }))
      localStorage.setItem('newtc.side-panels.v1', JSON.stringify({ leftCollapsed: true, rightCollapsed: true }))
    })
    await page.reload()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await expect(page.getByTestId('toggle-history-panel')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('toggle-artifacts-panel')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '218')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '310')

    const settings = await page.evaluate(async () => {
      const providers = await window.tcview.getProviderSettings()
      return window.tcview.saveProviderSettings(providers.map(provider => ({
        ...provider.effective!, id: provider.id,
        model: provider.id === 'codex' ? 'fixture-tcview-saved-selector' : provider.effective!.model,
        enabled: provider.id !== 'grok',
      })))
    })
    expect(settings.find(provider => provider.id === 'codex')?.saved?.model).toBe('fixture-tcview-saved-selector')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：TCView 独立保存新的讨论、模型设置和布局偏好。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const before = (await page.evaluate(() => window.tcview.listRuns()))[0]
    const frozenBefore = await page.evaluate(id => window.tcview.getRun(id), before.id)
    expect(frozenBefore.decision?.mock).toBe(true)
    expect(frozenBefore.providers?.map(provider => provider.id)).toEqual(['codex', 'claude', 'agy'])
    expect(frozenBefore.providers?.find(provider => provider.id === 'codex')?.model).toBe('fixture-tcview-saved-selector')
    const widths = { left: 282, right: 366 }
    const collapsed = { leftCollapsed: true, rightCollapsed: true }
    await page.addInitScript(({ widths, collapsed }) => {
      localStorage.setItem('tcview.layout.v1', JSON.stringify(widths))
      localStorage.setItem('tcview.side-panels.v1', JSON.stringify(collapsed))
    }, { widths, collapsed })
    await page.reload()
    await expect(page.getByTestId('toggle-history-panel')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByTestId('toggle-artifacts-panel')).toHaveAttribute('aria-expanded', 'false')
    await app.close()
    app = undefined
    const database = join(currentUserData, 'host/desktop.sqlite')
    const databaseBefore = await stat(database)
    expect(databaseBefore.size).toBeGreaterThan(0)

    launched = await launchIsolated(launcher, ignoredLegacyEnvironment)
    app = launched.app
    page = launched.page
    await expect(page.getByTestId('toggle-history-panel')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByTestId('toggle-artifacts-panel')).toHaveAttribute('aria-expanded', 'false')
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem('tcview.layout.v1') ?? 'null'))).toEqual(widths)
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem('tcview.side-panels.v1') ?? 'null'))).toEqual(collapsed)
    expect((await page.evaluate(() => window.tcview.listRuns())).map(run => run.id)).toEqual([before.id])
    expect(await page.evaluate(id => window.tcview.getRun(id), before.id)).toEqual(frozenBefore)
    const restoredSettings = await page.evaluate(() => window.tcview.getProviderSettings())
    expect(restoredSettings.map(provider => provider.saved)).toEqual(settings.map(provider => provider.saved))
    expect(restoredSettings.find(provider => provider.id === 'codex')?.sources.model.kind).toBe('saved')
    expect(restoredSettings.find(provider => provider.id === 'grok')?.effective?.enabled).toBe(false)
    const databaseAfter = await stat(database)
    expect(databaseAfter.ino).toBe(databaseBefore.ino)
    expect(databaseAfter.dev).toBe(databaseBefore.dev)
    expect(await directorySnapshot(legacyUserData)).toEqual(legacyBefore)
    await page.getByTestId('toggle-history-panel').click()
    await page.getByTestId('toggle-artifacts-panel').click()
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '282')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '366')
    await expect(page.getByTestId(`history-row-${before.id}`)).toBeVisible()
    await expect(page.getByTestId('message-list')).toContainText('独立保存新的讨论')
    await mkdir(evidenceDirectory, { recursive: true })
    await page.screenshot({ path: join(evidenceDirectory, 'mock-tcview-independent-identity.png'), fullPage: true })
    await writeFile(join(evidenceDirectory, 'identity-summary.json'), JSON.stringify({
      providerMode: 'mock', identity, defaultDirectory: 'TCView', legacyDirectoryUntouched: true,
      ignoredLegacyEnvironment: Object.keys(ignoredLegacyEnvironment), legacyRunNotImported: legacyRun.id !== before.id,
      sameCurrentDatabaseFile: databaseBefore.ino === databaseAfter.ino && databaseBefore.dev === databaseAfter.dev,
      retainedRunId: before.id, retainedRunStatus: frozenBefore.status,
      frozenSnapshotUnchanged: true, savedProviderSettingsUnchanged: true,
      widths, collapsed, realModelRequests: 0,
    }, null, 2) + '\n')
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})

test('仅 TCVIEW 环境变量覆盖应用目录、模型和推理设置，旧变量不参与优先级（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-branding-overrides-'))
  const { appData, launcher } = await makeLauncher(directory)
  const currentUserData = join(directory, 'custom-electron')
  const currentHostData = join(directory, 'custom-host')
  const legacyUserData = join(appData, 'NewTC')
  let app: ElectronApplication | undefined
  try {
    await mkdir(legacyUserData)
    const sentinel = join(legacyUserData, 'do-not-read-or-change.txt')
    await writeFile(sentinel, '旧变量指定的目录不参与设置解析\n', { mode: 0o600 })
    const legacyBefore = await directorySnapshot(legacyUserData)
    const overrides = {
      TCVIEW_USER_DATA_DIR: currentUserData, TCVIEW_DATA_DIR: currentHostData,
      TCVIEW_CODEX_MODEL: 'fixture-tcview-environment-selector',
      TCVIEW_GROK_REASONING: 'low', TCVIEW_GROK_ENABLED: 'false',
      NEWTC_USER_DATA_DIR: legacyUserData, NEWTC_DATA_DIR: join(legacyUserData, 'host'),
      NEWTC_CODEX_MODEL: 'fixture-legacy-environment-must-not-apply',
      NEWTC_GROK_REASONING: 'high', NEWTC_GROK_ENABLED: 'true',
    }
    let launched = await launchIsolated(launcher, overrides)
    app = launched.app
    let page = launched.page
    expect(await app.evaluate(({ app }) => ({ userData: app.getPath('userData'), sessionData: app.getPath('sessionData') }))).toEqual({ userData: currentUserData, sessionData: currentUserData })
    const views = await page.evaluate(() => window.tcview.getProviderSettings())
    expect(views.find(provider => provider.id === 'codex')?.effective?.model).toBe('fixture-tcview-environment-selector')
    expect(views.find(provider => provider.id === 'codex')?.sources.model).toEqual({ kind: 'environment', env: 'TCVIEW_CODEX_MODEL' })
    expect(views.find(provider => provider.id === 'grok')?.effective?.reasoning).toBe('low')
    expect(views.find(provider => provider.id === 'grok')?.effective?.enabled).toBe(false)
    expect(views.find(provider => provider.id === 'grok')?.sources.reasoning.env).toBe('TCVIEW_GROK_REASONING')
    const saved = await page.evaluate(async () => {
      const providers = await window.tcview.getProviderSettings()
      return window.tcview.saveProviderSettings(providers.map(provider => ({
        ...provider.effective!, id: provider.id,
        model: provider.id === 'codex' ? 'fixture-tcview-saved-under-environment' : provider.effective!.model,
      })))
    })
    expect(saved.find(provider => provider.id === 'codex')?.saved?.model).toBe('fixture-tcview-saved-under-environment')
    expect(saved.find(provider => provider.id === 'codex')?.effective?.model).toBe('fixture-tcview-environment-selector')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-env-notice-codex')).toContainText('TCVIEW_CODEX_MODEL')
    await expect(page.getByRole('dialog')).not.toContainText('NEWTC_')
    await app.close()
    app = undefined
    expect((await stat(join(currentHostData, 'desktop.sqlite'))).size).toBeGreaterThan(0)
    launched = await launchIsolated(launcher, { TCVIEW_USER_DATA_DIR: currentUserData, TCVIEW_DATA_DIR: currentHostData, NEWTC_CODEX_MODEL: overrides.NEWTC_CODEX_MODEL })
    app = launched.app
    page = launched.page
    const restored = (await page.evaluate(() => window.tcview.getProviderSettings())).find(provider => provider.id === 'codex')!
    expect(restored.effective?.model).toBe('fixture-tcview-saved-under-environment')
    expect(restored.sources.model.kind).toBe('saved')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    expect(await directorySnapshot(legacyUserData)).toEqual(legacyBefore)
    expect(await readdir(appData)).toEqual(['NewTC'])
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})
