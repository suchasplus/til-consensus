import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'
import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

// 只用明确标记的 Mock CLI。Electron、preload、Go 后台和讨论状态机实际运行。
const evidenceDirectory = resolve('.tcview/discussion-panels-validation', new Date().toISOString().replaceAll(':', '-'))

async function launchMock(directory: string, delay = '30') {
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
    await prepareTestWindows(app)
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    return { app, page }
  } catch (failure) { await app.close(); throw failure }
}

async function screenshot(page: Page, filename: string) {
  await mkdir(evidenceDirectory, { recursive: true })
  await page.screenshot({ path: join(evidenceDirectory, filename), fullPage: true })
}

async function expectCollapsed(page: Page) {
  await expect(page.getByTestId('toggle-discussion-context')).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByTestId('discussion-context-body')).not.toBeVisible()
  await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByTestId('composer-dock-body')).not.toBeVisible()
}

test('默认收起讨论介绍，发起后上下自动收起并为消息腾出空间，停止恢复仍可用（显式 Mock）', async () => {
  test.setTimeout(70_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-discussion-panels-run-'))
  const { app, page } = await launchMock(directory, '350')
  try {
    await setViewportAndWait(page, { width: 1440, height: 920 })
    const errors: string[] = []
    page.on('pageerror', failure => errors.push(failure.message))
    await expect(page.getByTestId('toggle-discussion-context')).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByTestId('discussion-context-body')).not.toBeVisible()
    await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('goal-input')).toBeInViewport()
    await expect(page.getByTestId('mode-select')).toBeInViewport()
    await expect(page.getByTestId('run-status')).toBeInViewport()
    await page.getByTestId('toggle-discussion-context').click()
    await expect(page.getByTestId('participant-codex')).toBeVisible()
    const expandedChatHeight = await page.locator('.chat-area').evaluate(node => node.clientHeight)
    await screenshot(page, 'mock-expanded-before-discussion.png')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：折叠上下信息，保留充足的讨论阅读空间。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('message-list')).toContainText('Mock')
    await expectCollapsed(page)
    const compactChatHeight = await page.locator('.chat-area').evaluate(node => node.clientHeight)
    expect(compactChatHeight - expandedChatHeight).toBeGreaterThan(180)
    await writeFile(join(evidenceDirectory, 'layout-metrics.json'), JSON.stringify({
      providerMode: 'mock', viewport: { width: 1440, height: 920 },
      expandedChatHeight, compactChatHeight, additionalChatHeight: compactChatHeight - expandedChatHeight,
    }, null, 2) + '\n')
    await expect(page.getByTestId('stop-button')).toBeInViewport()
    await screenshot(page, 'mock-running-panels-collapsed.png')
    await page.getByTestId('stop-button').click()
    await expect(page.getByTestId('resume-button')).toBeEnabled()
    await expectCollapsed(page)
    await expect(page.getByTestId('resume-button')).toBeInViewport()
    const before = (await page.evaluate(() => window.tcview.listRuns()))[0]
    const frozenBefore = await page.evaluate(id => window.tcview.getRun(id), before.id)
    await page.getByTestId('resume-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成', { timeout: 35_000 })
    const frozenAfter = await page.evaluate(id => window.tcview.getRun(id), before.id)
    expect(frozenAfter.rolePlan).toEqual(frozenBefore.rolePlan)
    expect(frozenAfter.providers).toEqual(frozenBefore.providers)
    await expectCollapsed(page)
    await expect(page.getByTestId('branch-button')).toBeInViewport()
    await screenshot(page, 'mock-completed-panels-collapsed.png')
    await page.getByTestId('branch-button').click()
    await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('goal-input')).toBeEditable()
    expect(errors).toEqual([])
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('折叠不丢草稿、已添加材料与未提交粘贴内容，分析失败后输入保持展开（固定 Mock IPC）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-discussion-panels-draft-'))
  const { app, page } = await launchMock(directory)
  try {
    await app.evaluate(({ ipcMain }) => {
      ipcMain.removeHandler('tcview:topic-analyze')
      ipcMain.handle('tcview:topic-analyze', () => { throw new Error('固定 Mock：材料分析失败，请保留草稿后重试。') })
    })
    const goal = '比较离线恢复方案，保留尚未发送的验收要求。'
    await page.getByTestId('goal-input').fill(goal)
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('已添加的验收约束')
    await page.getByTestId('material-content-input').fill('重启后保留消息和 CLI 冻结配置。')
    await page.getByTestId('add-pasted-material-button').click()
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('尚未添加的补充')
    await page.getByTestId('material-content-input').fill('这段粘贴文本不能因折叠而消失。')
    await page.getByTestId('toggle-composer').click()
    await expect(page.getByTestId('composer-dock-body')).not.toBeVisible()
    await expect(page.locator('.composer-dock-summary')).toContainText('草稿')
    await page.getByTestId('toggle-discussion-context').click()
    await page.getByTestId('toggle-discussion-context').click()
    await page.getByTestId('toggle-composer').click()
    await expect(page.getByTestId('goal-input')).toHaveValue(goal)
    await expect(page.getByTestId('material-list')).toContainText('已添加的验收约束')
    await expect(page.getByTestId('material-title-input')).toHaveValue('尚未添加的补充')
    await expect(page.getByTestId('material-content-input')).toHaveValue('这段粘贴文本不能因折叠而消失。')
    await page.getByTestId('analyze-topic-button').click()
    await expect(page.locator('.error-banner')).toContainText('固定 Mock：材料分析失败')
    await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('goal-input')).toHaveValue(goal)
    await expect(page.getByTestId('material-list')).toContainText('已添加的验收约束')
    await expect(page.getByTestId('material-content-input')).toHaveValue('这段粘贴文本不能因折叠而消失。')
    await page.getByTestId('toggle-composer').click()
    await page.getByTestId('toggle-composer').click()
    await expect(page.locator('.error-banner')).toContainText('固定 Mock：材料分析失败')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('历史讨论默认收起，新讨论重新展开输入，窄窗可用键盘折叠且详情切换保留草稿（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-discussion-panels-history-'))
  const { app, page } = await launchMock(directory)
  try {
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：窄窗与历史讨论折叠验收。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const run = (await page.evaluate(() => window.tcview.listRuns()))[0]
    await page.getByTestId('new-chat-button').click()
    await expect(page.getByTestId('goal-input')).toBeVisible()
    await expect(page.getByTestId('goal-input')).toBeEmpty()
    await page.getByTestId(`history-row-${run.id}`).getByRole('button').first().click()
    await expectCollapsed(page)
    await setViewportAndWait(page, { width: 820, height: 760 })
    const contextToggle = page.getByTestId('toggle-discussion-context')
    await contextToggle.focus()
    await contextToggle.press('Enter')
    await expect(contextToggle).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('participant-codex')).toBeVisible()
    await contextToggle.press('Space')
    await expect(contextToggle).toHaveAttribute('aria-expanded', 'false')
    const composerToggle = page.getByTestId('toggle-composer')
    await composerToggle.focus()
    await composerToggle.press('Enter')
    await expect(page.getByTestId('goal-input')).toBeInViewport()
    await page.getByTestId('goal-input').fill('切换方案详情后仍保留未发送的问题。')
    await page.getByRole('button', { name: '打开方案面板' }).click()
    const inspector = page.getByRole('complementary', { name: '讨论详情' })
    await expect(inspector).toBeVisible()
    await inspector.getByRole('button', { name: '关闭方案面板' }).click()
    await expect(page.getByTestId('goal-input')).toHaveValue('切换方案详情后仍保留未发送的问题。')
    await composerToggle.focus()
    await composerToggle.press('Space')
    await expect(composerToggle).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByTestId('run-status')).toBeInViewport()
    await expect(page.getByTestId('mode-select')).toBeInViewport()
    const widths = await page.evaluate(() => ({ viewport: innerWidth, content: document.documentElement.scrollWidth }))
    expect(widths.content).toBeLessThanOrEqual(widths.viewport + 1)
    await screenshot(page, 'mock-narrow-history-panels-collapsed.png')
    await page.getByTestId('new-chat-button').click()
    await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('goal-input')).toBeEditable()
    await expect(page.getByTestId('goal-input')).toBeEmpty()
    await page.getByTestId('toggle-composer').click()
    await page.getByRole('button', { name: /做一个产品决策/ }).click()
    await expect(page.getByTestId('toggle-composer')).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByTestId('goal-input')).toHaveValue(/任务管理工具/)
    await expect(page.getByTestId('goal-input')).toBeFocused()
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})
