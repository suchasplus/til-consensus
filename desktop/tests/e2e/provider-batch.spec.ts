import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type ElectronApplication } from '@playwright/test'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import type { ProviderConfig, ProviderInfo } from '../../packages/protocol/src/index'

const providerIds = ['codex', 'claude', 'grok', 'agy'] as const
interface FixtureOptions {
  unavailable?: string
  thrown?: string
  disabled?: string
  environmentGrok?: boolean
}
interface CheckInput { providerId: string; config: ProviderConfig }
interface FixtureState {
  calls: CheckInput[]
  effectiveRequests: CheckInput[]
  current: number
  peak: number
  saveCalls: number
  resetCalls: number
  reloadCalls: number
}

async function launchFixture(directory: string, options: FixtureOptions = {}) {
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
    // 真实 Electron 与 preload，所有 CLI 目录和结果仅使用下面明确标注的 Mock IPC。
    await app.evaluate(({ BrowserWindow }) => { for (const window of BrowserWindow.getAllWindows()) window.hide() })
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await app.evaluate(({ ipcMain }, fixtureOptions) => {
      const ids = ['codex', 'claude', 'grok', 'agy']
      const defaults: Record<string, ProviderConfig> = {
        codex: { id: 'codex', enabled: fixtureOptions.disabled !== 'codex', model: 'fixture-codex-a', reasoning: '' },
        claude: { id: 'claude', enabled: fixtureOptions.disabled !== 'claude', model: 'fixture-claude-opus', reasoning: '' },
        grok: { id: 'grok', enabled: fixtureOptions.disabled !== 'grok', model: 'grok-4.7', reasoning: 'low' },
        agy: { id: 'agy', enabled: fixtureOptions.disabled !== 'agy', model: 'gemini-3.8-flash-medium', reasoning: '' },
      }
      const argument = (id: string, reasoningValues = ['low', 'high']) => ({ id, reasoningMode: 'argument' as const, reasoningValues, verified: true })
      const models = {
        codex: [argument('fixture-codex-a'), argument('fixture-codex-b')],
        claude: [argument('fixture-claude-opus', ['low', 'high', 'max'])],
        grok: [argument('grok-4.7'), argument('fixture-grok-editing')],
        agy: ['low', 'medium', 'high'].map(effort => ({ id: `gemini-3.8-flash-${effort}`, family: 'gemini-3.8-flash', effort, reasoningMode: 'encoded' as const, reasoningValues: [], verified: true })),
      }
      const views: ProviderInfo[] = ids.map(id => ({
        id, name: `${id} · 显式 Mock 批量检查 fixture`, cliType: id, enabled: defaults[id].enabled,
        binaryPath: fixtureOptions.unavailable === id ? undefined : `/explicit-mock/${id}`,
        version: fixtureOptions.unavailable === id ? undefined : 'explicit-mock-batch-v1',
        available: fixtureOptions.unavailable !== id, executableFound: fixtureOptions.unavailable !== id,
        versionOK: fixtureOptions.unavailable !== id, requestOK: false, status: fixtureOptions.unavailable === id ? 'unavailable' : 'available',
        defaults: defaults[id], effective: defaults[id],
        sources: {
          enabled: { kind: 'default' }, command: { kind: 'default' },
          model: fixtureOptions.environmentGrok && id === 'grok' ? { kind: 'environment', env: 'TCVIEW_GROK_MODEL' } : { kind: 'default' },
          reasoning: fixtureOptions.environmentGrok && id === 'grok' ? { kind: 'environment', env: 'TCVIEW_GROK_REASONING' } : { kind: 'default' },
        },
        catalog: { id, cliVersion: 'explicit-mock-batch-v1', source: 'explicit-mock-fixture', models: models[id as keyof typeof models], loadedAt: new Date().toISOString(), manualAllowed: true },
      }))
      const state: FixtureState = { calls: [], effectiveRequests: [], current: 0, peak: 0, saveCalls: 0, resetCalls: 0, reloadCalls: 0 }
      let released = false
      const waiters: (() => void)[] = []
      Reflect.set(globalThis, '__tcviewBatchFixtureState', state)
      Reflect.set(globalThis, '__tcviewBatchFixtureRelease', () => { released = true; for (const resolve of waiters.splice(0)) resolve() })
      for (const channel of ['tcview:provider-settings-get', 'tcview:provider-settings-save', 'tcview:provider-settings-reset', 'tcview:provider-models', 'tcview:provider-check']) ipcMain.removeHandler(channel)
      ipcMain.handle('tcview:provider-settings-get', () => views)
      ipcMain.handle('tcview:provider-settings-save', () => { state.saveCalls++; return views })
      ipcMain.handle('tcview:provider-settings-reset', () => { state.resetCalls++; return views })
      ipcMain.handle('tcview:provider-models', (_event, id) => { state.reloadCalls++; return views.find(view => view.id === id)?.catalog })
      ipcMain.handle('tcview:provider-check', async (_event, input: CheckInput) => {
        const captured = structuredClone(input)
        const view = views.find(item => item.id === captured.providerId)!
        const effectiveConfig = { ...captured.config }
        if (view.sources.model.kind === 'environment') effectiveConfig.model = view.effective!.model
        if (view.sources.reasoning.kind === 'environment') effectiveConfig.reasoning = view.effective!.reasoning
        state.calls.push(captured)
        state.effectiveRequests.push({ providerId: captured.providerId, config: effectiveConfig })
        state.current++
        state.peak = Math.max(state.peak, state.current)
        try {
          if (!released) await new Promise<void>(resolve => waiters.push(resolve))
          if (fixtureOptions.thrown === input.providerId) throw new Error('显式 Mock：当前账号未登录，批次必须继续检查其他 CLI。')
          const failed = fixtureOptions.unavailable === input.providerId
          return {
            ...view, status: failed ? 'error' : 'ready', requestOK: !failed, requestAttempted: !failed,
            checkedConfiguration: 'editing', requestedModel: effectiveConfig.model, requestedReasoning: effectiveConfig.reasoning,
            reportedModel: failed ? undefined : input.providerId === 'grok' ? 'grok-4.7-build' : effectiveConfig.model || undefined,
            modelSource: 'explicit-mock-cli-report', lastCheckedAt: new Date().toISOString(), latencyMs: 1,
            error: failed ? '显式 Mock：未找到 CLI 可执行文件，本次未发起模型请求。' : undefined,
          }
        } finally { state.current-- }
      })
    }, options)
    return { app, page }
  } catch (error) { await app.close(); throw error }
}

async function state(app: ElectronApplication): Promise<FixtureState> {
  return app.evaluate(() => Reflect.get(globalThis, '__tcviewBatchFixtureState'))
}
async function release(app: ElectronApplication) {
  await app.evaluate(() => Reflect.get(globalThis, '__tcviewBatchFixtureRelease')())
}

// 真实请求路径在生产中复用已有 checkProvider；本文件不得调用真实模型或伪称真实验收。
test('一键检查按编辑快照串行覆盖四家，含停用与不可用项；失败继续且不自动保存（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-batch-all-'))
  const { app, page } = await launchFixture(directory, { disabled: 'agy', unavailable: 'claude', environmentGrok: true })
  try {
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-enabled-agy')).not.toBeChecked()
    await selectDropdown(page, 'provider-model-codex', 'fixture-codex-b')
    await selectDropdown(page, 'provider-reasoning-codex', 'high')
    await selectDropdown(page, 'provider-model-grok', 'fixture-grok-editing')
    await selectDropdown(page, 'provider-reasoning-grok', 'high')
    await selectDropdown(page, 'provider-reasoning-agy', 'gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-editing-effective-grok')).toContainText('grok-4.7')
    await page.getByTestId('check-all-providers').click({ clickCount: 2 })
    await expect.poll(async () => (await state(app)).calls.length).toBe(1)
    await expect(page.getByTestId('provider-batch-status')).toContainText('1/4')
    await expect(page.getByTestId('check-all-providers')).toBeDisabled()
    await expect(page.getByTestId('save-provider-settings')).toBeDisabled()
    await expect(page.getByTestId('reset-provider-settings')).toBeDisabled()
    for (const id of providerIds) {
      await expect(page.getByTestId(`provider-model-${id}`)).toBeDisabled()
      await expect(page.getByTestId(`provider-reasoning-${id}`)).toBeDisabled()
      await expect(page.getByTestId(`provider-enabled-${id}`)).toBeDisabled()
      await expect(page.getByTestId(`reload-provider-models-${id}`)).toBeDisabled()
      await expect(page.getByTestId(`check-provider-${id}`)).toBeDisabled()
    }
    await release(app)
    await expect(page.getByTestId('provider-batch-status')).toContainText('检查完成')
    await expect(page.getByTestId('provider-batch-status')).toContainText('通过 3')
    await expect(page.getByTestId('provider-batch-status')).toContainText('失败 1')
    const actual = await state(app)
    expect(actual.calls).toEqual([
      { providerId: 'codex', config: { id: 'codex', enabled: true, model: 'fixture-codex-b', reasoning: 'high' } },
      { providerId: 'claude', config: { id: 'claude', enabled: true, model: 'fixture-claude-opus', reasoning: '' } },
      { providerId: 'grok', config: { id: 'grok', enabled: true, model: 'fixture-grok-editing', reasoning: 'high' } },
      { providerId: 'agy', config: { id: 'agy', enabled: false, model: 'gemini-3.8-flash-high', reasoning: '' } },
    ])
    expect(actual.effectiveRequests[2]).toEqual({ providerId: 'grok', config: { id: 'grok', enabled: true, model: 'grok-4.7', reasoning: 'low' } })
    expect(actual.peak).toBe(1)
    expect(actual.current).toBe(0)
    expect(actual.saveCalls).toBe(0)
    expect(actual.resetCalls).toBe(0)
    expect(actual.reloadCalls).toBe(0)
    await expect(page.getByTestId('provider-check-result-claude')).toContainText('未发起模型请求')
    await expect(page.getByTestId('provider-card-claude')).toContainText('未找到 CLI 可执行文件')
    await expect(page.getByTestId('provider-check-result-agy')).toContainText('真实请求通过')
    await expect(page.getByTestId('provider-requested-model-grok')).toHaveText('grok-4.7')
    await expect(page.getByTestId('provider-reported-model-grok')).toContainText('grok-4.7-build')
    await expect(page.getByTestId('provider-reported-reasoning-grok')).toHaveText('未报告')
    await expect(page.getByTestId('provider-settings-save-state')).toContainText('未保存')
    await expect(page.getByTestId('provider-config-values-codex')).toContainText('尚无保存项')
    await expect(page.getByTestId('check-all-providers')).toBeEnabled()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('批次关闭重开保持进度及编辑快照，抛错后继续且完成后可主动复查（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-batch-reopen-'))
  const { app, page } = await launchFixture(directory, { thrown: 'claude' })
  try {
    await page.getByTestId('provider-settings-button').click()
    await selectDropdown(page, 'provider-model-codex', 'fixture-codex-b')
    await selectDropdown(page, 'provider-reasoning-codex', 'high')
    await page.getByTestId('check-all-providers').click()
    await expect.poll(async () => (await state(app)).calls.length).toBe(1)
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).not.toBeVisible()
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-batch-status')).toContainText('1/4')
    await expect(page.getByTestId('provider-model-codex')).toHaveAttribute('data-value', 'fixture-codex-b')
    await expect(page.getByTestId('provider-reasoning-codex')).toHaveAttribute('data-value', 'high')
    await expect(page.getByTestId('check-all-providers')).toBeDisabled()
    expect((await state(app)).calls).toHaveLength(1)
    await release(app)
    await expect(page.getByTestId('provider-batch-status')).toContainText('检查完成')
    await expect(page.getByTestId('provider-batch-status')).toContainText('通过 3')
    await expect(page.getByTestId('provider-batch-status')).toContainText('失败 1')
    await expect(page.getByTestId('provider-card-claude')).toContainText('当前账号未登录')
    await expect(page.getByTestId('provider-check-result-agy')).toContainText('真实请求通过')
    await expect(page.getByTestId('check-all-providers')).toBeEnabled()
    const first = await state(app)
    expect(first.calls.map(call => call.providerId)).toEqual(providerIds)
    expect(first.peak).toBe(1)
    expect(first.saveCalls).toBe(0)
    await page.getByTestId('check-all-providers').click()
    await expect.poll(async () => (await state(app)).calls.length).toBe(8)
    await expect(page.getByTestId('provider-batch-status')).toContainText('检查完成')
    expect((await state(app)).calls.slice(4).map(call => call.providerId)).toEqual(providerIds)
    expect((await state(app)).peak).toBe(1)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('单家检查占用共享锁，一键检查等待其结束（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-batch-single-lock-'))
  const { app, page } = await launchFixture(directory)
  try {
    await page.getByTestId('provider-settings-button').click()
    await page.getByTestId('check-provider-codex').click({ clickCount: 2 })
    await expect.poll(async () => (await state(app)).calls.length).toBe(1)
    await expect(page.getByTestId('check-all-providers')).toBeDisabled()
    for (const id of providerIds) await expect(page.getByTestId(`check-provider-${id}`)).toBeDisabled()
    await release(app)
    await expect(page.getByTestId('provider-check-result-codex')).toContainText('真实请求通过')
    await expect(page.getByTestId('check-all-providers')).toBeEnabled()
    await page.getByTestId('check-all-providers').click()
    await expect.poll(async () => (await state(app)).calls.length).toBe(5)
    await expect(page.getByTestId('provider-batch-status')).toContainText('检查完成')
    expect((await state(app)).calls.map(call => call.providerId)).toEqual(['codex', ...providerIds])
    expect((await state(app)).peak).toBe(1)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('停止后续检查保留当前结果并明确跳过数量，不派发余下请求（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-batch-stop-'))
  const { app, page } = await launchFixture(directory)
  try {
    await page.getByTestId('provider-settings-button').click()
    await page.getByTestId('check-all-providers').click()
    await expect.poll(async () => (await state(app)).calls.length).toBe(1)
    await page.getByTestId('stop-provider-batch').click()
    await expect(page.getByTestId('check-all-providers')).toBeDisabled()
    expect((await state(app)).current).toBe(1)
    await release(app)
    await expect(page.getByTestId('provider-batch-status')).toContainText('已停止')
    await expect(page.getByTestId('provider-batch-status')).toContainText('通过 1')
    await expect(page.getByTestId('provider-batch-status')).toContainText('跳过 3')
    await expect(page.getByTestId('provider-check-result-codex')).toContainText('真实请求通过')
    await expect(page.getByTestId('check-all-providers')).toBeEnabled()
    expect((await state(app)).calls.map(call => call.providerId)).toEqual(['codex'])
    expect((await state(app)).current).toBe(0)
    await page.getByTestId('check-all-providers').click()
    await expect.poll(async () => (await state(app)).calls.length).toBe(5)
    await expect(page.getByTestId('provider-batch-status')).toContainText('检查完成')
    expect((await state(app)).calls.slice(1).map(call => call.providerId)).toEqual(providerIds)
    expect((await state(app)).peak).toBe(1)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})
