import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type Page } from '@playwright/test'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

// 主题回归明确使用 Mock CLI；真实 Electron、preload、Go 后台与讨论状态机保留。
// 截图放在持久目录，后续 Playwright 运行不会清空视觉验收记录。
const evidenceDirectory = resolve('.tcview/theme-validation', new Date().toISOString().replaceAll(':', '-'))

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
    await app.evaluate(({ BrowserWindow }) => { for (const window of BrowserWindow.getAllWindows()) window.hide() })
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    return { app, page }
  } catch (error) { await app.close(); throw error }
}

async function expectNoHorizontalOverflow(page: Page) {
  const widths = await page.evaluate(() => ({ viewport: innerWidth, content: document.documentElement.scrollWidth }))
  expect(widths.content).toBeLessThanOrEqual(widths.viewport + 1)
}

async function screenshot(page: Page, filename: string) {
  await mkdir(evidenceDirectory, { recursive: true })
  await page.screenshot({ path: join(evidenceDirectory, filename), fullPage: true })
}

test('Carbon 商务主题使用本地 Plex 字体，欢迎与 CLI 配置清晰可用且无横向溢出（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-carbon-theme-'))
  const { app, page } = await launchMock(directory)
  try {
    const errors: string[] = []
    const externalRequests: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    // 重载后禁用全部外网资源，字体必须由安装包本地文件提供。
    await page.route(/^https?:\/\//, async route => {
      externalRequests.push(route.request().url())
      await route.abort()
    })
    await page.reload()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.setViewportSize({ width: 1440, height: 920 })
    await expect(page.locator('.app-shell')).toHaveAttribute('data-theme', 'carbon')
    await expect(page.locator('.welcome')).toBeVisible()
    await expect(page.getByTestId('new-chat-button')).toHaveCSS('background-color', 'rgb(15, 98, 254)')
    await expect(page.locator('.sidebar')).toHaveCSS('background-color', 'rgb(22, 22, 22)')
    await expect(page.getByTestId('new-chat-button')).toHaveCSS('border-radius', '0px')
    // 深色导航计数曾被通用的 last-child 规则覆盖；检验可读性而非固定文字颜色。
    const historyCountContrast = await page.getByTestId('recent-history-tab').locator('span').evaluate(node => {
      const rgb = (value: string) => (value.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number)
      const luminance = (color: number[]) => color.map(value => {
        const component = value / 255
        return component <= .04045 ? component / 12.92 : ((component + .055) / 1.055) ** 2.4
      }).reduce((sum, value, index) => sum + value * [.2126, .7152, .0722][index], 0)
      const style = getComputedStyle(node)
      const foreground = luminance(rgb(style.color))
      const background = luminance(rgb(style.backgroundColor))
      return (Math.max(foreground, background) + .05) / (Math.min(foreground, background) + .05)
    })
    expect(historyCountContrast).toBeGreaterThanOrEqual(4.5)
    const fonts = await page.evaluate(async () => {
      await Promise.all([
        document.fonts.load('400 14px "IBM Plex Sans"', 'TCView Business'),
        document.fonts.load('400 12px "IBM Plex Mono"', 'gpt-6.1-sol'),
      ])
      await document.fonts.ready
      return {
        bodyFamily: getComputedStyle(document.body).fontFamily,
        faces: [...document.fonts].map(face => ({ family: face.family.replaceAll('"', ''), status: face.status })),
        remoteFonts: performance.getEntriesByType('resource').map(entry => entry.name).filter(name => /^https?:/.test(name) && /\.(woff2?|ttf|otf)(?:\?|$)/.test(name)),
      }
    })
    expect(fonts.bodyFamily).toContain('IBM Plex Sans')
    expect(fonts.faces).toEqual(expect.arrayContaining([
      expect.objectContaining({ family: 'IBM Plex Sans', status: 'loaded' }),
      expect.objectContaining({ family: 'IBM Plex Mono', status: 'loaded' }),
    ]))
    expect(fonts.remoteFonts).toEqual([])
    await expect(page.getByTestId('goal-input')).toBeInViewport()
    await expect(page.getByTestId('send-button')).toBeInViewport()
    await expectNoHorizontalOverflow(page)
    await screenshot(page, 'carbon-welcome.png')

    await page.getByTestId('provider-settings-button').click()
    const settings = page.getByRole('dialog', { name: 'CLI 与连接检查' })
    await expect(settings).toBeVisible()
    await expect(page.getByTestId('check-all-providers')).toBeVisible()
    await expect(page.getByTestId('check-all-providers')).toBeEnabled()
    await expect(page.getByTestId('check-all-providers')).toContainText('使用额度')
    await expect(page.getByTestId('check-all-providers')).toBeInViewport()
    await expect(page.getByTestId('provider-card-codex')).toBeVisible()
    await expectNoHorizontalOverflow(page)
    await screenshot(page, 'carbon-cli-settings.png')

    await page.setViewportSize({ width: 820, height: 760 })
    await expect(page.getByTestId('check-all-providers')).toBeInViewport()
    await page.getByTestId('provider-enabled-agy').scrollIntoViewIfNeeded()
    await expect(page.getByTestId('provider-enabled-agy')).toBeVisible()
    const bounds = (await settings.boundingBox())!
    expect(bounds.x).toBeGreaterThanOrEqual(0)
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(821)
    expect(await page.locator('.settings-content').evaluate(node => node.scrollWidth - node.clientWidth)).toBeLessThanOrEqual(1)
    await expectNoHorizontalOverflow(page)
    await page.keyboard.press('Escape')
    await expect(settings).not.toBeVisible()
    await expect(page.getByTestId('goal-input')).toBeInViewport()
    await expect(page.getByTestId('send-button')).toBeInViewport()
    expect(externalRequests).toEqual([])
    expect(errors).toEqual([])
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('Carbon 三栏拖动与键盘焦点保持可见，完成的 Mock 讨论与窄窗详情可阅读', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-carbon-layout-'))
  const { app, page } = await launchMock(directory)
  try {
    await page.setViewportSize({ width: 1440, height: 920 })
    const separator = page.getByTestId('resize-left')
    await separator.focus()
    await separator.press('ArrowRight')
    await expect(separator).toBeFocused()
    const focus = await separator.evaluate(node => {
      const style = getComputedStyle(node)
      return { outline: style.outlineStyle, width: Number.parseFloat(style.outlineWidth) }
    })
    expect(focus.outline).not.toBe('none')
    expect(focus.width).toBeGreaterThanOrEqual(2)
    const previousWidth = Number(await separator.getAttribute('aria-valuenow'))
    const handle = (await separator.boundingBox())!
    await page.mouse.move(handle.x + handle.width / 2, handle.y + 200)
    await page.mouse.down()
    await page.mouse.move(handle.x + handle.width / 2 + 40, handle.y + 200, { steps: 6 })
    await page.mouse.up()
    await expect(separator).toHaveAttribute('aria-valuenow', String(previousWidth + 40))
    expect(await page.locator('.main-panel').evaluate(node => node.clientWidth)).toBeGreaterThanOrEqual(420)
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('为企业知识库确定可恢复的离线同步方案，明确 MVP 范围与验收标准。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    await expect(page.getByTestId('message-list')).toContainText('Mock')
    await expect(page.getByRole('tabpanel')).toContainText('方案')
    await expectNoHorizontalOverflow(page)
    await screenshot(page, 'carbon-discussion.png')

    await page.setViewportSize({ width: 820, height: 760 })
    await expect(page.getByTestId('resize-right')).not.toBeVisible()
    const toggle = page.getByRole('button', { name: '打开方案面板' })
    await toggle.focus()
    await toggle.press('Enter')
    const inspector = page.getByRole('complementary', { name: '讨论详情' })
    await expect(inspector).toBeVisible()
    const close = inspector.getByRole('button', { name: '关闭方案面板' })
    await close.focus()
    await expect(close).toBeFocused()
    expect(await close.evaluate(node => getComputedStyle(node).outlineStyle)).not.toBe('none')
    await expect(inspector.getByRole('tabpanel')).toContainText('方案')
    await expectNoHorizontalOverflow(page)
    await close.press('Enter')
    await expect(inspector).not.toBeVisible()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(1)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('三栏细分隔线消除浅槽，保留双侧鼠标热区、右侧拖动和窄窗抽屉边界（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-panel-seam-'))
  const { app, page } = await launchMock(directory)
  const seamEvidenceDirectory = resolve('.tcview/panel-seam-validation', new Date().toISOString().replaceAll(':', '-'))
  try {
    await page.setViewportSize({ width: 1400, height: 900 })
    const right = page.getByTestId('resize-right')
    const inspector = page.getByRole('complementary', { name: '讨论详情' })
    // 隐藏的 Electron 窗口可能异步投递指针事件；保留时间线以区分状态提交和额外输入。
    await page.evaluate(() => {
      const separator = document.querySelector('[data-testid="resize-right"]')!
      const records: Record<string, unknown>[] = []
      Reflect.set(window, '__tcviewSeamPointerDiagnostics', records)
      const record = (value: Record<string, unknown>) => {
        records.push({ at: performance.now(), width: separator.getAttribute('aria-valuenow'), className: separator.className, ...value })
        if (records.length > 200) records.shift()
      }
      record({ type: 'initial', visibility: document.visibilityState, focused: document.hasFocus(), innerWidth: window.innerWidth })
      for (const type of ['pointerdown', 'pointermove', 'pointerup', 'pointercancel', 'gotpointercapture', 'lostpointercapture']) {
        document.addEventListener(type, event => {
          const pointer = event as PointerEvent
          record({
            type, timeStamp: pointer.timeStamp, trusted: pointer.isTrusted, pointerId: pointer.pointerId,
            buttons: pointer.buttons, clientX: pointer.clientX, clientY: pointer.clientY,
            target: pointer.target instanceof Element ? pointer.target.closest('[data-testid]')?.getAttribute('data-testid') ?? pointer.target.tagName : null,
          })
        }, true)
      }
      new MutationObserver(() => record({ type: 'separator-state' })).observe(separator, { attributes: true, attributeFilter: ['class', 'aria-valuenow'] })
    })
    const geometry = await page.evaluate(() => {
      const main = document.querySelector('.main-panel')!.getBoundingClientRect()
      const detail = document.querySelector('.inspector')!.getBoundingClientRect()
      const separator = document.querySelector('[data-testid="resize-right"]')!.getBoundingClientRect()
      const center = separator.x + separator.width / 2
      return {
        gap: detail.left - main.right,
        handleWidth: separator.width,
        centerOffset: center - (main.right + detail.left) / 2,
        hitTargets: [-2, 2].map(offset => document.elementFromPoint(center + offset, 200)?.getAttribute('data-testid')),
      }
    })
    expect(geometry.gap).toBeCloseTo(1, 2)
    expect(geometry.handleWidth).toBe(7)
    expect(geometry.centerOffset).toBeCloseTo(0, 2)
    expect(geometry.hitTargets).toEqual(['resize-right', 'resize-right'])
    await expect(inspector).toHaveCSS('border-left-width', '0px')

    // 从线右侧的透明热区拖动，实际改变详情栏宽度；不靠视觉线的单个像素命中。
    const originalWidth = Number(await right.getAttribute('aria-valuenow'))
    const handle = (await right.boundingBox())!
    const startX = handle.x + handle.width / 2 + 2
    await page.mouse.move(startX, 200)
    await page.mouse.down()
    await expect(right).toHaveClass(/\bdragging\b/)
    await page.mouse.move(startX - 40, 200, { steps: 6 })
    await expect(right).toHaveAttribute('aria-valuenow', String(originalWidth + 40))
    await page.mouse.up()
    await expect(right).not.toHaveClass(/\bdragging\b/)
    await expect(right).toHaveAttribute('aria-valuenow', String(originalWidth + 40))
    await right.press('ArrowRight')
    await expect(right).toHaveAttribute('aria-valuenow', String(originalWidth + 32))
    expect(await page.locator('.main-panel').evaluate(node => node.clientWidth)).toBeGreaterThanOrEqual(420)
    await page.mouse.move(400, 200)
    await right.evaluate(node => (node as HTMLElement).blur())
    await expectNoHorizontalOverflow(page)
    await mkdir(seamEvidenceDirectory, { recursive: true })
    await page.screenshot({ path: join(seamEvidenceDirectory, 'desktop-thin-divider.png'), fullPage: true })

    await page.setViewportSize({ width: 980, height: 760 })
    await expect(right).not.toBeVisible()
    await page.getByRole('button', { name: '打开方案面板' }).click()
    await expect(inspector).toBeVisible()
    await expect(inspector).toHaveCSS('border-left-width', '1px')
    await expectNoHorizontalOverflow(page)
    await page.screenshot({ path: join(seamEvidenceDirectory, 'narrow-drawer-boundary.png'), fullPage: true })
    await inspector.getByRole('button', { name: '关闭方案面板' }).click()
    await expect(inspector).not.toBeVisible()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } catch (failure) {
    // 不掩盖原始断言失败；诊断仅包含本测试的指针和布局信息。
    try {
      const diagnostics = await page.evaluate(() => ({
        records: Reflect.get(window, '__tcviewSeamPointerDiagnostics'),
        visibility: document.visibilityState, focused: document.hasFocus(),
        innerWidth: window.innerWidth, outerWidth: window.outerWidth,
        width: document.querySelector('[data-testid="resize-right"]')?.getAttribute('aria-valuenow'),
      }))
      await mkdir(seamEvidenceDirectory, { recursive: true })
      await writeFile(join(seamEvidenceDirectory, 'pointer-failure.json'), JSON.stringify({ failure: String(failure), ...diagnostics }, null, 2) + '\n')
    } catch { /* 诊断写入失败不改变测试失败原因。 */ }
    throw failure
  } finally {
    await page.mouse.up().catch(() => {})
    await app.close()
    await rm(directory, { recursive: true, force: true })
  }
})
