import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'
import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type ElectronApplication, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

// 明确使用 Mock CLI，实际运行 Electron、preload 和 Go 后台；不调用真实模型。
const evidenceDirectory = resolve('.tcview/side-panels-validation', new Date().toISOString().replaceAll(':', '-'))
const widthStorageKey = 'tcview.layout.v1'
const sideStorageKey = 'tcview.side-panels.v1'

async function launchMock(directory: string) {
  const env: Record<string, string> = {
    ...Object.fromEntries(Object.entries(process.env).filter((entry): entry is [string, string] => entry[1] !== undefined)),
    TCVIEW_DATA_DIR: join(directory, 'host'), TCVIEW_USER_DATA_DIR: join(directory, 'electron'),
    TCVIEW_PROVIDER_MODE: 'mock', TCVIEW_MOCK_DELAY_MS: '20',
  }
  for (const provider of ['CODEX', 'CLAUDE', 'GROK', 'AGY']) {
    for (const field of ['MODEL', 'REASONING', 'EFFORT', 'ENABLED']) delete env[`TCVIEW_${provider}_${field}`]
  }
  delete env.ELECTRON_RUN_AS_NODE
  const app = await electron.launch({ args: [resolve('.')], env })
  try {
    const page = await app.firstWindow()
    await prepareTestWindows(app)
    // 状态位于可折叠的历史栏中；检查连接文字，允许栏已从持久设置中收起。
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await setViewportAndWait(page, { width: 1440, height: 920 })
    return { app, page }
  } catch (failure) { await app.close(); throw failure }
}

async function screenshot(page: Page, filename: string) {
  await mkdir(evidenceDirectory, { recursive: true })
  await page.screenshot({ path: join(evidenceDirectory, filename), fullPage: true })
}

async function expectPanels(page: Page, left: boolean, right: boolean) {
  await expect(page.getByTestId('toggle-history-panel')).toHaveAttribute('aria-expanded', String(left))
  await expect(page.getByTestId('toggle-artifacts-panel')).toHaveAttribute('aria-expanded', String(right))
  if (left) await expect(page.locator('#history-panel')).toBeVisible()
  else await expect(page.locator('#history-panel')).not.toBeVisible()
  if (right) await expect(page.locator('#artifacts-panel')).toBeVisible()
  else await expect(page.locator('#artifacts-panel')).not.toBeVisible()
}

async function expectSaved(page: Page, leftCollapsed: boolean, rightCollapsed: boolean) {
  await expect.poll(() => page.evaluate(key => JSON.parse(localStorage.getItem(key) ?? '{}'), sideStorageKey))
    .toEqual({ leftCollapsed, rightCollapsed })
}

async function reloadWithStoredSettings(page: Page, entries: Record<string, string | null>) {
  // 模拟旧版落盘数据。当前文档 pagehide 会再次保存 React 状态，因此在下一份文档
  // 启动、应用读取设置之前注入一次，避免把人工改 localStorage 当成实时设置 API。
  await page.addInitScript(() => {
    const seed = sessionStorage.getItem('tcview.e2e.next-settings')
    if (seed === null) return
    sessionStorage.removeItem('tcview.e2e.next-settings')
    for (const [key, value] of Object.entries(JSON.parse(seed) as Record<string, string | null>)) {
      if (value === null) localStorage.removeItem(key)
      else localStorage.setItem(key, value)
    }
  })
  await page.evaluate(value => sessionStorage.setItem('tcview.e2e.next-settings', JSON.stringify(value)), entries)
  await page.reload()
}

test('左右独立折叠后主区铺满，立即保存并在重载、重启后恢复，展开保留首选宽度（不调用模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-side-panels-persistence-'))
  let app: ElectronApplication | undefined
  try {
    let launched = await launchMock(directory)
    app = launched.app
    let page = launched.page
    const errors: string[] = []
    page.on('pageerror', failure => errors.push(failure.message))
    await expectPanels(page, true, true)
    await reloadWithStoredSettings(page, { [widthStorageKey]: JSON.stringify({ left: 282, right: 366 }) })
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '282')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '366')
    const expandedMain = await page.locator('.main-panel').evaluate(node => node.clientWidth)

    await page.getByTestId('collapse-history-panel').click()
    await expectPanels(page, false, true)
    await expect(page.getByTestId('resize-left')).not.toBeVisible()
    await expect(page.getByTestId('resize-left')).toHaveAttribute('tabindex', '-1')
    await expect(page.getByTestId('toggle-history-panel')).toBeInViewport()
    await expectSaved(page, true, false)
    await page.getByTestId('collapse-artifacts-panel').click()
    await expectPanels(page, false, false)
    await expect(page.getByTestId('resize-right')).not.toBeVisible()
    await expect(page.getByTestId('resize-right')).toHaveAttribute('tabindex', '-1')
    await expectSaved(page, true, true)
    const collapsed = await page.locator('.main-panel').evaluate(node => ({
      width: node.clientWidth, left: node.getBoundingClientRect().left,
      right: node.getBoundingClientRect().right, viewport: innerWidth,
      documentWidth: document.documentElement.scrollWidth,
    }))
    expect(collapsed.width).toBeGreaterThanOrEqual(collapsed.viewport - 2)
    expect(collapsed.left).toBeLessThanOrEqual(1)
    expect(collapsed.right).toBeGreaterThanOrEqual(collapsed.viewport - 1)
    expect(collapsed.documentWidth).toBeLessThanOrEqual(collapsed.viewport + 1)
    expect(collapsed.width - expandedMain).toBeGreaterThan(640)
    await screenshot(page, 'mock-both-collapsed-full-main.png')
    await writeFile(join(evidenceDirectory, 'layout-metrics.json'), JSON.stringify({
      providerMode: 'mock', expandedMain, collapsed, additionalMainWidth: collapsed.width - expandedMain,
    }, null, 2) + '\n')
    await page.reload()
    await expectPanels(page, false, false)
    await app.close()
    app = undefined
    launched = await launchMock(directory)
    app = launched.app
    page = launched.page
    await expectPanels(page, false, false)
    await expectSaved(page, true, true)
    const historyToggle = page.getByTestId('toggle-history-panel')
    await historyToggle.focus()
    await historyToggle.press('Enter')
    await expectPanels(page, true, false)
    await expect(historyToggle).toBeFocused()
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '282')
    const artifactsToggle = page.getByTestId('toggle-artifacts-panel')
    await artifactsToggle.focus()
    await artifactsToggle.press('Space')
    await expectPanels(page, true, true)
    await expect(artifactsToggle).toBeFocused()
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '366')
    expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key) ?? '{}'), widthStorageKey)).toEqual({ left: 282, right: 366 })
    await expectSaved(page, false, false)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    expect(errors).toEqual([])
    await screenshot(page, 'mock-panels-restored-widths.png')
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})

test('折叠保留当前讨论、历史搜索、方案版本、详情标签以及未发送的草稿和材料（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-side-panels-state-'))
  const { app, page } = await launchMock(directory)
  try {
    const goal = '固定 Mock：侧栏折叠时保留讨论上下文。'
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill(goal)
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const run = (await page.evaluate(() => window.tcview.listRuns()))[0]
    await expect(page.getByTestId(`history-row-${run.id}`)).toBeVisible()
    await page.getByTestId('history-search').fill('保留讨论上下文')
    await selectDropdown(page, 'proposal-version-select', '0')
    await page.getByRole('tab', { name: '投票' }).click()
    await page.getByTestId('toggle-composer').click()
    const draft = '尚未发送：请补充可恢复性与边界条件。'
    await page.getByTestId('goal-input').fill(draft)
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('折叠前的验收材料')
    await page.getByTestId('material-content-input').fill('侧栏展开后仍应保留完整材料。')
    await page.getByTestId('add-pasted-material-button').click()
    await page.getByTestId('toggle-history-panel').click()
    await page.getByTestId('toggle-artifacts-panel').click()
    await expectPanels(page, false, false)
    await expect(page.getByTestId('message-list')).toContainText('Mock')
    await expect(page.getByTestId('goal-input')).toHaveValue(draft)
    await expect(page.getByTestId('material-list')).toContainText('折叠前的验收材料')
    await screenshot(page, 'mock-current-discussion-full-main.png')
    await page.getByTestId('toggle-history-panel').click()
    await page.getByTestId('toggle-artifacts-panel').click()
    await expectPanels(page, true, true)
    await expect(page.getByTestId('history-search')).toHaveValue('保留讨论上下文')
    await expect(page.getByTestId(`history-row-${run.id}`).getByRole('button').first()).toHaveAttribute('aria-current', 'page')
    await expect(page.getByRole('tab', { name: '投票' })).toHaveAttribute('aria-selected', 'true')
    await page.getByRole('tab', { name: '方案', exact: true }).click()
    await expect(page.getByTestId('proposal-version-select')).toHaveAttribute('data-value', '0')
    await expect(page.getByTestId('goal-input')).toHaveValue(draft)
    await expect(page.getByTestId('material-list')).toContainText('折叠前的验收材料')
    expect((await page.evaluate(() => window.tcview.listRuns())).map(item => item.id)).toEqual([run.id])
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('旧宽度设置与损坏的折叠设置兼容，缺失和无效字段使用默认展开（不调用模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-side-panels-migration-'))
  const { app, page } = await launchMock(directory)
  try {
    await reloadWithStoredSettings(page, {
      [widthStorageKey]: JSON.stringify({ left: 290, right: 342 }),
      [sideStorageKey]: '{broken JSON',
    })
    await expectPanels(page, true, true)
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '290')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '342')
    await expectSaved(page, false, false)
    await reloadWithStoredSettings(page, { [sideStorageKey]: JSON.stringify({ leftCollapsed: 'true', rightCollapsed: null }) })
    await expectPanels(page, true, true)
    await reloadWithStoredSettings(page, { [sideStorageKey]: null })
    await expectPanels(page, true, true)
    expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key) ?? '{}'), widthStorageKey)).toEqual({ left: 290, right: 342 })
    await page.getByTestId('toggle-history-panel').click()
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.keyboard.press('Escape')
    await expectPanels(page, false, true)
    await expectSaved(page, true, false)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('窄窗自动隐藏右侧不改偏好，抽屉支持键盘开关并记住明确的关闭选择（不调用模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-side-panels-narrow-'))
  let app: ElectronApplication | undefined
  try {
    let launched = await launchMock(directory)
    app = launched.app
    let page = launched.page
    await expectPanels(page, true, true)
    await expectSaved(page, false, false)
    await setViewportAndWait(page, { width: 820, height: 760 })
    await expectPanels(page, true, false)
    await expect(page.getByTestId('resize-right')).not.toBeVisible()
    await expectSaved(page, false, false)
    const rightToggle = page.getByTestId('toggle-artifacts-panel')
    await expect(rightToggle).toHaveAccessibleName('打开方案面板')
    await rightToggle.focus()
    await rightToggle.press('Enter')
    await expectPanels(page, true, true)
    await expectSaved(page, false, false)
    await page.keyboard.press('Escape')
    await expectPanels(page, true, false)
    await expect(rightToggle).toBeFocused()
    await expectSaved(page, false, true)
    await setViewportAndWait(page, { width: 1440, height: 920 })
    await expectPanels(page, true, false)
    await rightToggle.focus()
    await rightToggle.press('Space')
    await expectPanels(page, true, true)
    await expectSaved(page, false, false)
    await setViewportAndWait(page, { width: 820, height: 760 })
    await expectPanels(page, true, false)
    await expectSaved(page, false, false)
    await page.getByTestId('toggle-history-panel').click()
    await expectPanels(page, false, false)
    const viewport = await page.locator('.main-panel').evaluate(node => ({ mainWidth: node.clientWidth, windowWidth: innerWidth, documentWidth: document.documentElement.scrollWidth }))
    expect(viewport.mainWidth).toBeGreaterThanOrEqual(viewport.windowWidth - 2)
    expect(viewport.documentWidth).toBeLessThanOrEqual(viewport.windowWidth + 1)
    await screenshot(page, 'mock-narrow-both-collapsed.png')
    await app.close()
    app = undefined
    launched = await launchMock(directory)
    app = launched.app
    page = launched.page
    await setViewportAndWait(page, { width: 820, height: 760 })
    await expectPanels(page, false, false)
    await expectSaved(page, true, false)
    await setViewportAndWait(page, { width: 1440, height: 920 })
    await expectPanels(page, false, true)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})
