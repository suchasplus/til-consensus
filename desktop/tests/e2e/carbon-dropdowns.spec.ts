import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'
import { test, expect, _electron as electron, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { selectDropdown } from './helpers/select'

// 仅验证真实 Electron、preload 与自绘组件；目录和讨论请求明确使用 Mock，不消耗模型额度。
const evidenceDirectory = resolve('.tcview/dropdown-validation', new Date().toISOString().replaceAll(':', '-'))
const longModel = 'zulu-explicit-mock-enterprise-catalog-with-a-very-long-model-selector-2026-10-20'

async function launchMock(directory: string) {
  const env: Record<string, string> = {
    ...Object.fromEntries(Object.entries(process.env).filter((entry): entry is [string, string] => entry[1] !== undefined)),
    TCVIEW_DATA_DIR: join(directory, 'host'), TCVIEW_USER_DATA_DIR: join(directory, 'electron'),
    TCVIEW_PROVIDER_MODE: 'mock', TCVIEW_MOCK_DELAY_MS: '15',
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
  } catch (error) { await app.close(); throw error }
}

async function screenshot(page: Page, filename: string) {
  await mkdir(evidenceDirectory, { recursive: true })
  await page.screenshot({ path: join(evidenceDirectory, filename), fullPage: true })
}

async function expectMenuInViewport(page: Page) {
  const menu = page.locator('.carbon-select-menu')
  await expect(menu).toBeVisible()
  const bounds = (await menu.boundingBox())!
  const viewport = page.viewportSize()!
  expect(bounds.x).toBeGreaterThanOrEqual(0)
  expect(bounds.y).toBeGreaterThanOrEqual(0)
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width + 1)
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(viewport.height + 1)
  expect(await menu.evaluate(node => node.scrollWidth - node.clientWidth)).toBeLessThanOrEqual(1)
}

test('模式下拉采用 Carbon 菜单，键盘导航与取消不会提前修改选择（显式 Mock）', async ({}, testInfo) => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-carbon-dropdown-mode-'))
  const { app, page } = await launchMock(directory)
  try {
    await setViewportAndWait(page, { width: 1440, height: 920 })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    const control = page.getByTestId('mode-select')
    const menu = page.locator('.carbon-select-menu')
    // 只观察事件，不等待/重试或改变焦点。偶发窗口失焦时，将原因附到失败 trace。
    await page.evaluate(() => {
      const control = document.querySelector('[data-testid="mode-select"]')!
      const events: Record<string, unknown>[] = []
      Reflect.set(window, '__carbonDropdownInteractionTrace', events)
      const describe = (target: EventTarget | null) => target instanceof Element
        ? { tag: target.tagName, id: target.id, testId: target.getAttribute('data-testid'), role: target.getAttribute('role') }
        : { tag: target === window ? 'window' : target === document ? 'document' : null }
      const note = (kind: string, event?: Event) => {
        events.push({ at: performance.now(), kind, target: describe(event?.target ?? control), active: describe(document.activeElement),
          expanded: control.getAttribute('aria-expanded'), value: control.getAttribute('data-value'),
          documentFocused: document.hasFocus(), visibility: document.visibilityState, trusted: event?.isTrusted })
        if (events.length > 200) events.shift()
      }
      for (const type of ['blur', 'focus']) window.addEventListener(type, event => note(type, event), true)
      for (const type of ['focusin', 'pointerdown']) document.addEventListener(type, event => note(type, event), true)
      new MutationObserver(() => note('combobox-attributes')).observe(control, {
        attributes: true, attributeFilter: ['aria-expanded', 'aria-activedescendant', 'data-value'],
      })
      note('observation-start')
    })
    await expect(page.locator('select')).toHaveCount(0)
    await expect(control).toHaveRole('combobox')
    await expect(control).toHaveAttribute('data-value', 'auto')
    await control.click()
    await expect(menu).toHaveRole('listbox')
    await expect(menu).toHaveCSS('border-radius', '0px')
    await expect(menu).toHaveAttribute('data-placement', 'bottom')
    await expect(menu.getByRole('option')).toHaveText(['自动选择', '自由辩论', '证据裁决', '德尔菲'])
    await expect(menu.locator('[aria-selected="true"]')).toHaveAttribute('data-value', 'auto')
    await screenshot(page, 'carbon-mode-menu.png')
    await page.keyboard.press('ArrowDown')
    await expect(menu.locator('.is-active')).toHaveAttribute('data-value', 'free_debate')
    await expect(control).toHaveAttribute('data-value', 'auto')
    await page.keyboard.press('End')
    await expect(menu.locator('.is-active')).toHaveAttribute('data-value', 'delphi')
    await page.keyboard.press('Home')
    await expect(menu.locator('.is-active')).toHaveAttribute('data-value', 'auto')
    await page.keyboard.press('ArrowDown')
    await page.keyboard.press('Enter')
    await expect(menu).not.toBeVisible()
    await expect(control).toHaveAttribute('data-value', 'free_debate')
    await expect(control).toBeFocused()

    await control.press('ArrowUp')
    await expect(menu).toBeVisible()
    await page.keyboard.press('End')
    await page.keyboard.press('Escape')
    await expect(menu).not.toBeVisible()
    await expect(control).toHaveAttribute('data-value', 'free_debate')
    await expect(control).toBeFocused()
    await control.press('Space')
    await expect(menu).toBeVisible()
    await page.keyboard.press('ArrowDown')
    await page.keyboard.press('Tab')
    await expect(menu).not.toBeVisible()
    await expect(control).toHaveAttribute('data-value', 'free_debate')
    await expect(control).not.toBeFocused()
    await control.click()
    await page.locator('.phase-label').click()
    await expect(menu).not.toBeVisible()
    await expect(control).toHaveAttribute('data-value', 'free_debate')
    await selectDropdown(page, 'mode-select', 'auto')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    expect(errors).toEqual([])
  } finally {
    if (testInfo.status !== testInfo.expectedStatus) {
      const diagnostic = await page.evaluate(() => ({
        events: Reflect.get(window, '__carbonDropdownInteractionTrace'),
        combobox: document.querySelector('[data-testid="mode-select"]')?.outerHTML,
        menu: document.querySelector('.carbon-select-menu')?.outerHTML ?? null,
        documentFocused: document.hasFocus(), visibility: document.visibilityState,
      })).catch(error => ({ unavailable: String(error) }))
      await testInfo.attach('carbon-dropdown-interaction-events', { body: JSON.stringify({ providerMode: 'explicit-mock', diagnostic }, null, 2), contentType: 'application/json' })
    }
    await app.close()
    await rm(directory, { recursive: true, force: true })
  }
})

test('长模型目录可滚动与前缀定位，AGY 档位保留实际 ID，Escape 先关闭菜单（固定 Mock IPC）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-carbon-dropdown-settings-'))
  const { app, page } = await launchMock(directory)
  try {
    await app.evaluate(({ ipcMain }, finalModel) => {
      const ids = ['codex', 'claude', 'grok', 'agy']
      const argument = (id: string) => ({ id, reasoningMode: 'argument', reasoningValues: ['low', 'high'], verified: true })
      const defaults: Record<string, { id: string; enabled: boolean; model: string; reasoning: string }> = {
        codex: { id: 'codex', enabled: true, model: 'alpha-explicit-mock-model', reasoning: '' },
        claude: { id: 'claude', enabled: true, model: 'claude-explicit-mock-model', reasoning: '' },
        grok: { id: 'grok', enabled: true, model: 'grok-explicit-mock-selector', reasoning: 'low' },
        agy: { id: 'agy', enabled: true, model: 'gemini-3.8-flash-medium', reasoning: '' },
      }
      const models = {
        codex: [argument(defaults.codex.model), ...Array.from({ length: 36 }, (_, index) => argument(`catalog-explicit-mock-model-${String(index).padStart(2, '0')}`)), argument(finalModel), { id: 'unsupported-explicit-mock-model', reasoningMode: 'unsupported', reasoningValues: [], verified: true }],
        claude: [argument(defaults.claude.model)], grok: [argument(defaults.grok.model)],
        agy: ['low', 'medium', 'high'].map(effort => ({ id: `gemini-3.8-flash-${effort}`, family: 'gemini-3.8-flash', effort, reasoningMode: 'encoded', reasoningValues: [], verified: true })),
      }
      const views = ids.map(id => ({ id, name: `${id} · 显式 Mock 自绘下拉 fixture`, cliType: id, enabled: true, binaryPath: `/explicit-mock/${id}`, version: 'explicit-mock-dropdown-v1', available: true, executableFound: true, versionOK: true, requestOK: false, status: 'available', defaults: defaults[id], effective: defaults[id], sources: { enabled: { kind: 'default' }, command: { kind: 'default' }, model: { kind: 'default' }, reasoning: { kind: 'default' } }, catalog: { id, source: 'explicit-mock-fixture', loadedAt: new Date().toISOString(), models: models[id as keyof typeof models], manualAllowed: true } }))
      for (const channel of ['tcview:provider-settings-get', 'tcview:provider-models', 'tcview:provider-check']) ipcMain.removeHandler(channel)
      ipcMain.handle('tcview:provider-settings-get', () => views)
      ipcMain.handle('tcview:provider-models', (_event, id) => views.find(view => view.id === id)?.catalog)
      ipcMain.handle('tcview:provider-check', () => { throw new Error('此下拉样式测试禁止派发模型请求') })
    }, longModel)
    await setViewportAndWait(page, { width: 820, height: 760 })
    await page.getByTestId('provider-settings-button').click()
    const settings = page.getByRole('dialog', { name: 'CLI 与连接检查' })
    const control = page.getByTestId('provider-model-codex')
    const menu = page.locator('.carbon-select-menu')
    await expect(page.locator('select')).toHaveCount(0)
    await control.click()
    await expectMenuInViewport(page)
    expect(await menu.evaluate(node => node.scrollHeight - node.clientHeight)).toBeGreaterThan(100)
    // 真正的键盘前缀查找改变 active 项，Enter 前仍是原模型。
    await page.keyboard.type('zulu')
    await expect(menu.locator('.is-active')).toHaveAttribute('data-value', longModel)
    await expect(control).toHaveAttribute('data-value', 'alpha-explicit-mock-model')
    await screenshot(page, 'carbon-long-model-menu.png')
    await page.keyboard.press('Enter')
    await expect(control).toHaveAttribute('data-value', longModel)
    await expect(page.getByTestId('provider-editing-effective-codex')).toContainText(longModel)
    await control.click()
    const finalOption = menu.getByRole('option').filter({ hasText: longModel })
    await finalOption.scrollIntoViewIfNeeded()
    await expect(finalOption).toBeInViewport()
    expect(await menu.evaluate(node => node.scrollTop)).toBeGreaterThan(0)
    await page.keyboard.press('Escape')
    await expect(menu).not.toBeVisible()
    await expect(settings).toBeVisible()
    await expect(control).toBeFocused()

    await selectDropdown(page, 'provider-reasoning-codex', 'high')
    await selectDropdown(page, 'provider-model-codex', 'unsupported-explicit-mock-model')
    await expect(page.getByTestId('provider-reasoning-codex')).toBeDisabled()
    await expect(page.getByTestId('provider-reasoning-codex')).toHaveAttribute('data-value', '')
    await selectDropdown(page, 'provider-reasoning-agy', 'gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-model-agy')).toHaveAttribute('data-value', 'gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-editing-effective-agy')).toContainText('gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-card-agy')).toContainText('不再额外传入 effort')
    await page.getByTestId('provider-reasoning-agy').click()
    await screenshot(page, 'carbon-agy-effort-menu.png')
    await page.keyboard.press('Escape')
    await expect(settings).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(settings).not.toBeVisible()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('底部下拉自动向上展开，窄窗菜单保持在窗口范围内且不修改草稿（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-carbon-dropdown-bounds-'))
  const { app, page } = await launchMock(directory)
  try {
    await setViewportAndWait(page, { width: 820, height: 760 })
    await page.getByTestId('toggle-history-panel').click()
    const draft = '下拉位置验证保留尚未发送的草稿。'
    await page.getByTestId('goal-input').fill(draft)
    const control = page.getByTestId('preparation-provider-select')
    const menu = page.locator('.carbon-select-menu')
    await control.click()
    await expect(menu).toHaveAttribute('data-placement', 'top')
    await expectMenuInViewport(page)
    const triggerBounds = (await control.boundingBox())!
    const menuBounds = (await menu.boundingBox())!
    expect(menuBounds.y + menuBounds.height).toBeLessThanOrEqual(triggerBounds.y + 1)
    await screenshot(page, 'carbon-narrow-upward-menu.png')
    await page.keyboard.press('Escape')
    await expect(control).toHaveAttribute('data-value', 'agy')
    await expect(page.getByTestId('goal-input')).toHaveValue(draft)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})
