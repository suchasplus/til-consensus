import { prepareTestWindows } from './helpers/windows'
import { setViewportAndWait } from './helpers/viewport'
import { selectDropdown } from './helpers/select'
import { test, expect, _electron as electron, type ElectronApplication, type Page } from '@playwright/test'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { readFile } from 'node:fs/promises'

test('手工模型逐键输入保留空白，并在保存时拒绝且不显示内部 IPC 包装（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-model-input-'))
  const app = await launch(directory)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    const dialog = page.getByRole('dialog')
    await selectDropdown(page, 'provider-model-agy', '__manual__')
    const manual = page.getByTestId('provider-model-manual-agy')
    // 同值重选保留原 ID；用户清空后仍留在手工模式，并可继续逐键输入。
    await manual.fill('')
    await manual.pressSequentially('invalid model', {delay: 10})
    await expect(manual).toHaveValue('invalid model')
    await page.getByTestId('save-provider-settings').click()
    await expect(dialog).toContainText('模型 ID无效')
    await expect(dialog).not.toContainText('Error invoking remote method')
    const settings = await page.evaluate(() => window.tcview.getProviderSettings())
    expect(settings.find(provider => provider.id === 'agy')?.saved).toBeUndefined()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally {
    await app.close()
    await rm(directory, {recursive: true, force: true})
  }
})

async function launch(directory: string, delay = '80', overrides: Record<string, string | undefined> = {}) {
  const env: Record<string, string> = { ...Object.fromEntries(Object.entries(process.env).filter((entry): entry is [string, string] => entry[1] !== undefined)), TCVIEW_DATA_DIR: join(directory, 'host'), TCVIEW_USER_DATA_DIR: join(directory, 'electron'), TCVIEW_MOCK_DELAY_MS: delay, TCVIEW_PROVIDER_MODE:'mock' }
  for (const provider of ['CODEX', 'CLAUDE', 'GROK', 'AGY']) for (const field of ['MODEL', 'REASONING', 'EFFORT', 'ENABLED']) delete env[`TCVIEW_${provider}_${field}`]
  for (const [key, value] of Object.entries(overrides)) { if (value === undefined) delete env[key]; else env[key] = value }
  delete env.ELECTRON_RUN_AS_NODE
  const app = await electron.launch({ args: [resolve('.')], env })
  const page = await app.firstWindow()
  // 保留真实 Electron/CDP 渲染与合成帧，仅隔离原生鼠标和键盘输入。
  await prepareTestWindows(app)
  await setViewportAndWait(page)
  return app
}

async function openDiscussionContext(page: Page) {
  const toggle = page.getByTestId('toggle-discussion-context')
  if (await toggle.getAttribute('aria-expanded') !== 'true') await toggle.click()
}

test('桌面聊天提交、停止继续和重启后保留会话', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-desktop-'))
  let app: ElectronApplication | undefined
  try {
    app = await launch(directory, '350')
    let page = await app.firstWindow()
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('设计一个可恢复的团队知识库桌面应用')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('message-list')).toContainText('Mock')
    await expect(page.getByTestId('stop-button')).toBeEnabled()
    await page.getByTestId('stop-button').click()
    await expect(page.getByTestId('resume-button')).toBeEnabled()
    await page.getByTestId('resume-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成', { timeout: 25_000 })
    await expect(page.getByTestId('message-list')).toContainText('知识库')
    const completed = await page.evaluate(() => window.tcview.listRuns())
    expect(completed).toHaveLength(1)
    expect(completed[0].status).toBe('completed')
    expect(completed[0].goal).toBe('设计一个可恢复的团队知识库桌面应用')
    await page.screenshot({ path: 'test-results/m0-desktop.png', fullPage: true })
    expect(errors).toEqual([])
    await app.close()
    app = await launch(directory)
    page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await expect(page.getByTestId('message-list')).toContainText('知识库')
    const restored = await page.evaluate(() => window.tcview.listRuns())
    expect(restored.map(run => run.id)).toEqual(completed.map(run => run.id))
  } finally {
    await app?.close()
    await rm(directory, { recursive: true, force: true })
  }
})

test('三种模式经桌面桥接真实 Go 内核，Delphi 使用匿名显示', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-modes-'))
  const app = await launch(directory, '15')
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await setViewportAndWait(page, { width: 980, height: 720 })
    for (const mode of ['adjudication', 'delphi']) {
      await page.getByTestId('new-chat-button').click()
      await selectDropdown(page, 'mode-select', mode)
      await page.getByTestId('goal-input').fill(`验证 ${mode} 的 M0 模式映射`)
      await page.getByTestId('send-button').click()
      await expect(page.getByTestId('run-status')).toContainText('已完成')
      const messages = page.getByTestId('message-list')
      await expect(messages).toContainText('Mock')
      if (mode === 'delphi') {
        await expect(messages).not.toContainText('Claude')
        await expect(messages).not.toContainText('Grok')
        await page.getByRole('button', { name: '打开方案面板' }).click()
        await expect(page.getByRole('tabpanel')).toContainText('评分 4.2/5')
        await expect(page.getByRole('tabpanel')).not.toContainText('420%')
      }
    }
  } finally {
    await app.close()
    await rm(directory, { recursive: true, force: true })
  }
})

test('方案版本、关联讨论、导出和设置经完整桌面桥接工作', async () => {
  const directory=await mkdtemp(join(tmpdir(),'tcview-product-'))
  const app=await launch(directory,'15')
  try{
    const page=await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('验证方案版本、补充约束和导出')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const parent=(await page.evaluate(()=>window.tcview.listRuns()))[0]
    const first=await page.evaluate(id=>window.tcview.getRun(id),parent.id)
    expect(first.proposalHistory!.length).toBeGreaterThanOrEqual(2)
    await selectDropdown(page, 'proposal-version-select', '0')
    await expect(page.getByTestId('proposal-version-select')).toHaveAttribute('data-value', '0')
    await page.getByTestId('branch-button').click()
    await page.getByTestId('goal-input').fill('增加离线恢复的验收约束')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const runs=await page.evaluate(()=>window.tcview.listRuns())
    expect(runs).toHaveLength(2)
    const child=runs.find(run=>run.id!==parent.id)!
    const second=await page.evaluate(id=>window.tcview.getRun(id),child.id)
    expect(second.parentRunId).toBe(parent.id)
    expect(second.sourceProposalHash).toBe(first.proposal!.hash)
    const exportPath=join(directory,'discussion.json')
    await app.evaluate(({dialog},path)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath:path})},exportPath)
    await page.getByTestId('export-json-button').click()
    await expect.poll(async()=>{try{return JSON.parse(await readFile(exportPath,'utf8')).run.id}catch{return ''}}).toBe(child.id)
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByRole('dialog')).toContainText('Mock')
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).not.toBeVisible()
  }finally{await app.close();await rm(directory,{recursive:true,force:true})}
})

test('自动模式复用一次 Mock 检查，文本材料随讨论和关联讨论冻结', async () => {
  test.setTimeout(60_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-topic-materials-'))
  const filePath = join(directory, '用户反馈.md')
  const fileContent = '用户希望在断网时继续整理资料。\n联网后应能查看同步冲突并手动处理。\n'
  await writeFile(filePath, fileContent, 'utf8')
  const app = await launch(directory, '35')
  try {
    const page = await app.firstWindow()
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await expect(page.getByTestId('mode-select')).toHaveAttribute('data-value', 'auto')
    await expect(page.getByTestId('preparation-provider-select')).toHaveAttribute('data-value', 'agy')
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('团队约束')
    await page.getByTestId('material-content-input').fill('材料必须离线保存在本机，两周内交付知识库 MVP，并提供可恢复的同步队列。')
    await page.getByTestId('add-pasted-material-button').click()
    await app.evaluate(({ dialog }, path) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [path] }) }, filePath)
    await page.getByTestId('import-materials-button').click()
    await expect(page.getByTestId('material-list')).toContainText('团队约束')
    await expect(page.getByTestId('material-list')).toContainText('用户反馈.md')
    await expect(page.getByTestId('material-list')).not.toContainText(directory)
    await page.getByTestId('material-list').getByText('用户反馈.md', { exact: true }).click()
    await expect(page.getByTestId('material-list')).toContainText('SHA-256')
    await page.getByTestId('goal-input').fill('比较离线优先与云端优先的知识库实现，确定两周 MVP 范围和验收标准。')
    await page.getByTestId('analyze-topic-button').click()
    await expect(page.getByTestId('preparation-status')).toHaveText('检查完成')
    await expect(page.getByTestId('preparation-card')).toContainText('Mock')
    await expect(page.getByTestId('preparation-card')).toContainText('分析建议不代表事实核验')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    const preparationId = await page.getByTestId('preparation-card').getAttribute('data-preparation-id')
    expect(preparationId).toBeTruthy()
    const before = await page.evaluate(id => window.tcview.getPreparation(id!), preparationId)
    expect(before.status).toBe('completed')
    expect(before.materials.map(material => material.title)).toEqual(['团队约束', '用户反馈.md'])
    expect(before.materials[1].rawSha256).toMatch(/^[a-f0-9]{64}$/)
    await page.screenshot({ path: 'test-results/topic-preparation-ready.png', fullPage: true })
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成', { timeout: 25_000 })
    const runs = await page.evaluate(() => window.tcview.listRuns())
    expect(runs).toHaveLength(1)
    const run = await page.evaluate(id => window.tcview.getRun(id), runs[0].id)
    expect(run.requestedMode).toBe('auto')
    expect(run.mode).toBe('free_debate')
    expect(run.preparation?.id).toBe(preparationId)
    expect(run.materials?.[1].content).toBe(fileContent)
    await expect(page.getByTestId('message-list')).toContainText('团队约束')
    await page.screenshot({ path: 'test-results/topic-materials-desktop.png', fullPage: true })
    await page.getByTestId('branch-button').click()
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('新增验收')
    await page.getByTestId('material-content-input').fill('新增验收条件：断电重启后不得丢失已经提交的材料。')
    await page.getByTestId('add-pasted-material-button').click()
    await page.getByTestId('goal-input').fill('增加断电恢复验收，继续评审当前方案。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const all = await page.evaluate(() => window.tcview.listRuns())
    expect(all).toHaveLength(2)
    const child = await page.evaluate(id => window.tcview.getRun(id), all.find(item => item.id !== run.id)!.id)
    expect(child.mode).toBe(run.mode)
    expect(child.parentRunId).toBe(run.id)
    expect(child.materials?.map(material => material.title)).toEqual(['团队约束', '用户反馈.md', '新增验收'])
    expect(errors).toEqual([])
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('自动分析取消、修改输入和切换会话后不会由旧结果启动讨论（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-topic-cancel-'))
  const app = await launch(directory, '750')
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('goal-input').fill('比较两种知识库同步方案，明确实现成本和恢复目标。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('cancel-preparation-button')).toBeVisible()
    const cancelledId = await page.getByTestId('preparation-card').getAttribute('data-preparation-id')
    await page.getByTestId('cancel-preparation-button').click()
    await expect(page.getByTestId('preparation-status')).toHaveText('已取消')
    await expect.poll(async () => (await page.evaluate(id => window.tcview.getPreparation(id!), cancelledId)).status).toBe('cancelled')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)

    await page.getByTestId('goal-input').fill('评审新的材料归档方案，比较离线恢复与索引一致性。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('cancel-preparation-button')).toBeVisible()
    const outdatedId = await page.getByTestId('preparation-card').getAttribute('data-preparation-id')
    await page.getByTestId('goal-input').fill('修改后的问题：先明确验收目标与参与角色，不沿用旧检查。')
    await expect(page.getByTestId('preparation-card')).toHaveCount(0)
    await selectDropdown(page, 'preparation-provider-select', 'codex')
    await expect.poll(async () => (await page.evaluate(id => window.tcview.getPreparation(id!), outdatedId)).status).toBe('cancelled')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)

    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('cancel-preparation-button')).toBeVisible()
    const switchedId = await page.getByTestId('preparation-card').getAttribute('data-preparation-id')
    await page.getByTestId('new-chat-button').click()
    await expect(page.getByTestId('mode-select')).toHaveAttribute('data-value', 'auto')
    await expect(page.getByTestId('goal-input')).toBeEmpty()
    await expect(page.getByTestId('preparation-card')).toHaveCount(0)
    await expect.poll(async () => (await page.evaluate(id => window.tcview.getPreparation(id!), switchedId)).status).toBe('cancelled')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    await expect(page.getByTestId('run-status')).toHaveText('准备就绪')
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('粘贴材料按 UTF-8 字节限制并可移除（不调用模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-material-limit-'))
  const app = await launch(directory)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('paste-material-button').click()
    await page.getByTestId('material-title-input').fill('超出限制的中文材料')
    await page.getByTestId('material-content-input').fill('字'.repeat(22_000))
    await page.getByTestId('add-pasted-material-button').click()
    await expect(page.getByTestId('material-error')).toContainText('64 KiB')
    await expect(page.getByTestId('material-list')).toHaveCount(0)
    await page.getByTestId('material-content-input').fill('保留完整的文本原文，不自动读取 /tmp/example.txt。')
    await page.getByTestId('add-pasted-material-button').click()
    await expect(page.getByTestId('material-list')).toContainText('超出限制的中文材料')
    await page.getByRole('button', { name: '移除材料 超出限制的中文材料' }).click()
    await expect(page.getByTestId('material-list')).toHaveCount(0)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

for (const fixture of [
  { recommendation: 'needs_clarification', confidence: 0.91, notice: 'clarification-questions', requiredGap: false, expired: false },
  { recommendation: 'not_suitable', confidence: 0.91, notice: 'not-suitable-notice', requiredGap: false, expired: false },
  { recommendation: 'delphi', confidence: 0.42, notice: 'low-confidence-notice', requiredGap: false, expired: false },
  { recommendation: 'free_debate', confidence: 0.91, notice: 'required-material-gaps', requiredGap: true, expired: false },
  { recommendation: 'free_debate', confidence: 0.91, notice: 'expired-preparation-notice', requiredGap: false, expired: true },
] as const) test(`固定 Mock IPC 分类 ${fixture.recommendation}/${fixture.confidence} ${fixture.notice} 不自动启动讨论`, async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-ui-classification-'))
  const app = await launch(directory)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    // 仅替换当前测试 app 的公开 IPC handler，验证前端门禁；不声称这是模型推理。
    await app.evaluate(({ ipcMain }, configured) => {
      ipcMain.removeHandler('tcview:topic-analyze')
      ipcMain.handle('tcview:topic-analyze', (_event, input) => {
        const at = new Date(Date.now() - (configured.expired ? 16 * 60_000 : 0)).toISOString()
        return {
          id: input.id, goal: input.goal, inputHash: 'explicit-mock-fixture', requestHash: 'explicit-mock-fixture', materials: [], status: 'completed', createdAt: at, updatedAt: at,
          classification: {
            recommendation: configured.recommendation, confidence: configured.confidence, summary: '固定 Mock IPC 结果，仅验证界面分支。', why: ['这是明确标记的测试结果', '不会调用真实模型'], missingInformation: configured.recommendation === 'needs_clarification' ? ['请补充可验证的成功标准。'] : [], estimatedModeAfterClarification: configured.recommendation === 'needs_clarification' ? 'delphi' : '', estimatedModeReason: configured.recommendation === 'needs_clarification' ? '补充标准后可做匿名评分。' : '', suggestedTask: '采用后的新任务表述，需要重新检查。',
          },
          materialAnalysis: { summary: '没有附加材料。', materials: [], gaps: configured.requiredGap ? [{ question: '请提供实际验收条件。', importance: 'required', materialIds: [] }] : [], limitations: ['固定 Mock fixture，不代表事实核验。'] },
          source: { providerId: 'agy', cliType: 'mock', cliVersion: 'test-fixture', requestedModel: 'mock-classifier', actualModel: 'mock-classifier', modelSource: 'explicit-test-fixture', elapsedMs: 1, mock: true }, usage: { calls: 1 }, costUsd: null,
        }
      })
    }, fixture)
    const goal = '这是需要用户决定后续处理方式的原始任务。'
    await page.getByTestId('goal-input').fill(goal)
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('preparation-status')).toHaveText('检查完成')
    await expect(page.getByTestId(fixture.notice)).toBeVisible()
    await expect(page.getByTestId('preparation-card')).toContainText('Mock')
    await expect(page.getByTestId('goal-input')).toHaveValue(goal)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    if (fixture.recommendation === 'needs_clarification') {
      await expect(page.getByTestId('clarification-questions')).toContainText('不会据此直接开始讨论')
      await page.getByTestId('adopt-rewrite-button').click()
      await expect(page.getByTestId('goal-input')).toHaveValue('采用后的新任务表述，需要重新检查。')
      await expect(page.getByTestId('preparation-card')).toHaveCount(0)
      expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
    }
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('固定 Mock IPC 请求失败后读取持久准备记录的错误，不重试或启动讨论', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-ui-preparation-error-'))
  const app = await launch(directory)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await app.evaluate(({ ipcMain }) => {
      let record: Record<string, unknown> | undefined
      ipcMain.removeHandler('tcview:topic-analyze')
      ipcMain.removeHandler('tcview:topic-get')
      ipcMain.handle('tcview:topic-analyze', (_event, input) => {
        const at = new Date().toISOString()
        record = { id: input.id, goal: input.goal, materials: [], inputHash: 'mock-failed-record', requestHash: 'mock-failed-record', status: 'failed', createdAt: at, updatedAt: at, error: '持久准备记录：固定 Mock 解析失败，原始材料仍保留。', source: { providerId: 'agy', cliType: 'mock', cliVersion: 'test-fixture', requestedModel: 'mock', modelSource: 'explicit-test-fixture', elapsedMs: 1, mock: true } }
        throw new Error('固定 Mock：请求在返回结果前断开')
      })
      ipcMain.handle('tcview:topic-get', () => record)
    })
    await page.getByTestId('goal-input').fill('固定失败场景，用于验证持久准备记录回显。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('preparation-status')).toHaveText('检查失败')
    await expect(page.getByTestId('preparation-card')).toContainText('持久准备记录：固定 Mock 解析失败')
    await expect(page.getByTestId('send-button')).toBeEnabled()
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})


const isolatedProviderEnvironment: Record<string, undefined> = Object.fromEntries(['CODEX', 'CLAUDE', 'GROK', 'AGY'].flatMap((provider) => ['MODEL', 'REASONING', 'EFFORT', 'ENABLED'].map((field) => [`TCVIEW_${provider}_${field}`, undefined])))

test('CLI 应用设置保存、重启持久化和恢复默认经真实 Go 桥接（显式 Mock 模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-provider-settings-'))
  let app: ElectronApplication | undefined
  try {
    app = await launch(directory, '35', isolatedProviderEnvironment)
    let page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-model-codex')).toBeVisible()
    await selectDropdown(page, 'provider-model-codex', '__manual__')
    await page.getByTestId('provider-model-manual-codex').fill('fixture-codex-persisted')
    await expect(page.getByTestId('provider-model-verification-codex')).toContainText('尚未验证')
    await expect(page.getByTestId('provider-settings-save-state')).toContainText('未保存')
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    const saved = await page.evaluate(() => window.tcview.getProviderSettings())
    expect(saved.find((provider) => provider.id === 'codex')?.saved?.model).toBe('fixture-codex-persisted')
    expect(saved.find((provider) => provider.id === 'codex')?.effective?.model).toBe('fixture-codex-persisted')
    await expect(page.getByTestId('provider-model-source-codex')).toHaveText('已保存设置')
    await page.screenshot({ path: 'test-results/provider-settings-saved.png', fullPage: true })
    await app.close()
    app = await launch(directory, '35', isolatedProviderEnvironment)
    page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-model-manual-codex')).toHaveValue('fixture-codex-persisted')
    await expect(page.getByTestId('provider-config-values-codex')).toContainText('fixture-codex-persisted')
    // 未保存编辑不能污染持久项；恢复默认即使与保存值相同也要清除草稿。
    await page.getByTestId('provider-model-manual-codex').fill('fixture-unsaved-change')
    await page.keyboard.press('Escape')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-model-manual-codex')).toHaveValue('fixture-codex-persisted')
    await page.getByTestId('reset-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    const reset = await page.evaluate(() => window.tcview.getProviderSettings())
    for (const provider of reset) {
      expect(provider.effective!.model).toBe(provider.defaults!.model)
      expect(provider.effective!.reasoning).toBe(provider.defaults!.reasoning)
      expect(provider.sources.model.kind).toBe('default')
    }
    await expect(page.getByTestId('provider-config-values-codex')).toContainText(reset.find((provider) => provider.id === 'codex')!.defaults!.model)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})

test('环境变量优先于应用设置，编辑检查和界面显示相同有效配置（显式 Mock 模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-provider-env-'))
  const app = await launch(directory, '35', { ...isolatedProviderEnvironment, TCVIEW_GROK_MODEL: 'grok-4.7', TCVIEW_GROK_REASONING: 'low', XAI_API_KEY: 'fixture-secret-must-never-render' })
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-env-notice-grok')).toContainText('TCVIEW_GROK_MODEL')
    await expect(page.getByTestId('provider-model-source-grok')).toContainText('环境变量')
    await selectDropdown(page, 'provider-model-grok', '__manual__')
    await page.getByTestId('provider-model-manual-grok').fill('fixture-grok-saved-selector')
    await expect(page.getByTestId('provider-editing-effective-grok')).toContainText('grok-4.7')
    await expect(page.getByTestId('provider-editing-effective-grok')).not.toContainText('fixture-grok-saved-selector')
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    const provider = (await page.evaluate(() => window.tcview.getProviderSettings())).find((item) => item.id === 'grok')!
    expect(provider.saved?.model).toBe('fixture-grok-saved-selector')
    expect(provider.effective!.model).toBe('grok-4.7')
    expect(provider.effective!.reasoning).toBe('low')
    expect(provider.sources.model.env).toBe('TCVIEW_GROK_MODEL')
    await page.getByTestId('check-provider-grok').click()
    await expect(page.getByTestId('provider-check-result-grok')).toBeVisible()
    await expect(page.getByTestId('provider-requested-model-grok')).toHaveText('grok-4.7')
    await expect(page.getByTestId('provider-check-result-grok')).toContainText('low')
    await expect(page.getByTestId('provider-reported-reasoning-grok')).toHaveText('未报告')
    await expect(page.getByRole('dialog')).not.toContainText('fixture-secret-must-never-render')
    await expect(page.getByRole('dialog')).toContainText('Mock')
    await page.screenshot({ path: 'test-results/provider-settings-environment.png', fullPage: true })
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('固定 Mock CLI 目录限定模型档位，编辑检查保留请求与实际报告并显示错误', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-provider-ui-fixture-'))
  const app = await launch(directory, '35', isolatedProviderEnvironment)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    // 只验证 renderer 与公开 Electron IPC 的配置行为；所有目录和调用结果均明确是 Mock。
    await app.evaluate(({ ipcMain }) => {
      const ids = ['codex', 'claude', 'grok', 'agy']
      const defaults: Record<string, { id: string; model: string; reasoning: string }> = {
        codex: { id: 'codex', model: 'fixture-codex-reasoning', reasoning: '' },
        claude: { id: 'claude', model: 'fixture-claude-opus', reasoning: '' },
        grok: { id: 'grok', model: 'grok-4.7', reasoning: 'low' },
        agy: { id: 'agy', model: 'gemini-3.8-flash-medium', reasoning: '' },
      }
      const argument = (id: string, values: string[]) => ({ id, reasoningMode: 'argument', reasoningValues: values, verified: true })
      const unsupported = (id: string) => ({ id, reasoningMode: 'unsupported', reasoningValues: [], verified: true })
      const models = {
        codex: [argument('fixture-codex-reasoning', ['low', 'high']), unsupported('fixture-codex-no-effort')],
        claude: [argument('fixture-claude-opus', ['low', 'medium', 'high', 'max']), unsupported('fixture-claude-haiku')],
        grok: [argument('grok-4.7', ['low', 'high'])],
        agy: ['low', 'medium', 'high'].map(effort => ({ id: `gemini-3.8-flash-${effort}`, family: 'gemini-3.8-flash', effort, reasoningMode: 'encoded', reasoningValues: [], verified: true })),
      }
      const views = ids.map(id => ({ id, name: `${id} · 固定 Mock fixture`, cliType: id, binaryPath: `/fixture/${id}`, version: 'explicit-mock-v1', available: true, executableFound: true, versionOK: true, requestOK: false, status: 'available', defaults: defaults[id], effective: defaults[id], sources: { command: { kind: 'default' }, model: { kind: 'default' }, reasoning: { kind: 'default' } }, catalog: { id, source: 'explicit-mock-fixture', models: models[id as keyof typeof models], loadedAt: new Date().toISOString(), manualAllowed: true } }))
      const requests: { providerId: string; config: { id: string; model: string; reasoning: string } }[] = []
      Reflect.set(globalThis, '__tcviewProviderUIFixtureRequests', requests)
      for (const channel of ['tcview:provider-settings-get', 'tcview:provider-models', 'tcview:provider-check']) ipcMain.removeHandler(channel)
      ipcMain.handle('tcview:provider-settings-get', () => views)
      ipcMain.handle('tcview:provider-models', (_event, id) => views.find(view => view.id === id)?.catalog)
      ipcMain.handle('tcview:provider-check', async (_event, input) => {
        requests.push(input)
        await new Promise(resolve => setTimeout(resolve, 150))
        const view = views.find(item => item.id === input.providerId)!
        const configurationError = input.config.model === 'fixture-invalid-configuration'
        const failed = input.config.model === 'fixture-invalid-selector' || configurationError
        return { ...view, status: failed ? 'error' : 'ready', requestOK: !failed, requestAttempted: !configurationError, checkedConfiguration: 'editing', requestedModel: input.config.model, requestedReasoning: input.config.reasoning, reportedModel: failed ? undefined : input.providerId === 'grok' ? 'grok-4.7-build' : input.config.model || undefined, modelSource: 'explicit-mock-cli-report', latencyMs: 150, lastCheckedAt: new Date().toISOString(), error: configurationError ? '固定 Mock：此模型不支持所选参数，校验拒绝且没有派发模型请求。' : failed ? '固定 Mock：模型不可用（invalid model selector），未切换模型或降低推理强度。' : undefined }
      })
    })
    await page.getByTestId('provider-settings-button').click()
    await selectDropdown(page, 'provider-reasoning-codex', 'high')
    await page.getByTestId('provider-reasoning-codex').click()
    await expect(page.locator('.carbon-select-menu').getByRole('option')).toHaveText(['使用 CLI 默认（不传覆盖）', 'low', 'high'])
    await page.keyboard.press('Escape')
    await selectDropdown(page, 'provider-model-codex', 'fixture-codex-no-effort')
    await expect(page.getByTestId('provider-reasoning-codex')).toBeDisabled()
    await expect(page.getByTestId('provider-reasoning-codex')).toHaveAttribute('data-value', '')
    await selectDropdown(page, 'provider-reasoning-claude', 'max')
    await selectDropdown(page, 'provider-model-claude', 'fixture-claude-haiku')
    await expect(page.getByTestId('provider-reasoning-claude')).toBeDisabled()
    await expect(page.getByTestId('provider-reasoning-claude')).toHaveAttribute('data-value', '')
    await selectDropdown(page, 'provider-reasoning-agy', 'gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-model-agy')).toHaveAttribute('data-value', 'gemini-3.8-flash-high')
    await expect(page.getByTestId('provider-card-agy')).toContainText('不再额外传入 effort')
    await page.getByTestId('check-provider-agy').click()
    await expect(page.getByTestId('provider-check-result-agy')).toContainText('真实请求通过')
    const agyRequests = await app.evaluate(() => Reflect.get(globalThis, '__tcviewProviderUIFixtureRequests'))
    expect(agyRequests).toEqual([{ providerId: 'agy', config: { id: 'agy', enabled: true, model: 'gemini-3.8-flash-high', reasoning: '' } }])
    await selectDropdown(page, 'provider-reasoning-grok', 'high')
    // 双击只能产生一次 IPC 检查；按钮和同步锁都防重复。
    await page.getByTestId('check-provider-grok').click({ clickCount: 2 })
    await expect(page.getByTestId('provider-requested-model-grok')).toHaveText('grok-4.7')
    await expect(page.getByTestId('provider-reported-model-grok')).toContainText('grok-4.7-build')
    await expect(page.getByTestId('provider-reported-reasoning-grok')).toHaveText('未报告')
    await expect(page.getByTestId('provider-settings-save-state')).toContainText('未保存')
    await expect(page.getByTestId('provider-config-values-grok')).toContainText('尚无保存项')
    const requested = await app.evaluate(() => Reflect.get(globalThis, '__tcviewProviderUIFixtureRequests'))
    expect(requested.filter((input: { providerId: string }) => input.providerId === 'grok')).toEqual([{ providerId: 'grok', config: { id: 'grok', enabled: true, model: 'grok-4.7', reasoning: 'high' } }])
    await selectDropdown(page, 'provider-model-grok', '__manual__')
    await page.getByTestId('provider-model-manual-grok').fill('fixture-invalid-selector')
    await expect(page.getByTestId('provider-model-verification-grok')).toContainText('尚未验证')
    await page.getByTestId('check-provider-grok').click()
    await expect(page.getByTestId('provider-card-grok')).toContainText('模型不可用（invalid model selector）')
    await expect(page.getByTestId('provider-check-result-grok')).toContainText('真实请求失败')
    await expect(page.getByTestId('provider-reported-model-grok')).toContainText('未报告')
    await page.getByTestId('provider-model-manual-grok').fill('fixture-invalid-configuration')
    await page.getByTestId('check-provider-grok').click()
    await expect(page.getByTestId('provider-check-result-grok')).toContainText('未发起模型请求')
    await expect(page.getByTestId('provider-card-grok')).toContainText('配置或探测未通过')
    await selectDropdown(page, 'provider-model-codex', '')
    await page.getByTestId('check-provider-codex').click()
    await expect(page.getByTestId('provider-requested-model-codex')).toHaveText('使用 CLI 默认（不传覆盖）')
    await expect(page.getByTestId('provider-reported-model-codex')).toContainText('未报告')
    await expect(page.getByTestId('provider-reported-reasoning-codex')).toHaveText('未报告')
    await page.screenshot({ path: 'test-results/provider-settings-fixture-report.png', fullPage: true })
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('保存新配置仅影响新讨论，停止恢复继续用冻结模型（显式 Mock 模型）', async () => {
  test.setTimeout(75_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-provider-freeze-'))
  const app = await launch(directory, '250', isolatedProviderEnvironment)
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-model-codex')).toBeVisible()
    await selectDropdown(page, 'provider-model-codex', '__manual__')
    await page.getByTestId('provider-model-manual-codex').fill('fixture-codex-frozen-a')
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await page.keyboard.press('Escape')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：验证已冻结配置不会在恢复时切换。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('stop-button')).toBeEnabled()
    await page.getByTestId('stop-button').click()
    await expect(page.getByTestId('resume-button')).toBeEnabled()
    const original = (await page.evaluate(() => window.tcview.listRuns()))[0]
    const before = await page.evaluate(id => window.tcview.getRun(id), original.id)
    expect(before.providers?.find(provider => provider.id === 'codex')?.model).toBe('fixture-codex-frozen-a')
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-model-manual-codex')).toHaveValue('fixture-codex-frozen-a')
    await page.getByTestId('provider-model-manual-codex').fill('fixture-codex-new-b')
    await page.getByTestId('provider-enabled-grok').uncheck()
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await page.getByTestId('provider-frozen-codex').locator('summary').click()
    await expect(page.getByTestId('provider-frozen-codex')).toContainText('fixture-codex-frozen-a')
    await expect(page.getByTestId('provider-config-values-codex')).toContainText('fixture-codex-new-b')
    await page.keyboard.press('Escape')
    await page.getByTestId('resume-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成', { timeout: 30_000 })
    const resumed = await page.evaluate(id => window.tcview.getRun(id), original.id)
    expect(resumed.providers?.map(provider => provider.id)).toEqual(['codex', 'claude', 'grok', 'agy'])
    expect(resumed.rolePlan?.reviewers).toEqual(['claude', 'grok', 'agy'])
    expect(resumed.providers?.find(provider => provider.id === 'codex')?.model).toBe('fixture-codex-frozen-a')
    expect(resumed.providers?.find(provider => provider.id === 'codex')?.sources?.model?.kind).toBe('saved')
    await page.getByTestId('new-chat-button').click()
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：新讨论应采用刚保存的新模型。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成', { timeout: 30_000 })
    const all = await page.evaluate(() => window.tcview.listRuns())
    const next = await page.evaluate(id => window.tcview.getRun(id), all.find(item => item.id !== original.id)!.id)
    expect(next.providers?.find(provider => provider.id === 'codex')?.model).toBe('fixture-codex-new-b')
    expect(next.providers?.map(provider => provider.id)).toEqual(['codex', 'claude', 'agy'])
    expect(next.rolePlan?.reviewers).toEqual(['claude', 'agy'])
    expect(next.usage?.calls).toBeLessThanOrEqual(30)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('停用 Codex 后冻结动态角色，重启保持选择；不足三家与全部停用不会启动分析（显式 Mock）', async () => {
  test.setTimeout(70_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-enabled-ui-'))
  let app: ElectronApplication | undefined
  try {
    app = await launch(directory, '20')
    let page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    for (const id of ['codex', 'claude', 'grok', 'agy']) await expect(page.getByTestId(`provider-enabled-${id}`)).toBeChecked()
    await page.getByTestId('provider-enabled-codex').uncheck()
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await expect(page.getByTestId('provider-enabled-effective-codex')).toContainText('当前有效：停用')
    await page.keyboard.press('Escape')
    await openDiscussionContext(page)
    await expect(page.getByTestId('participant-codex')).toHaveCount(0)
    await expect(page.getByTestId('participant-claude')).toContainText('起草 · 综合')
    await expect(page.getByTestId('role-plan')).toContainText('2 位独立评审')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：Codex 不可用时，由三家启用的 CLI 完成讨论。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const summary = (await page.evaluate(() => window.tcview.listRuns()))[0]
    const run = await page.evaluate(id => window.tcview.getRun(id), summary.id)
    expect(run.providers?.map(provider => provider.id)).toEqual(['claude', 'grok', 'agy'])
    expect(run.rolePlan).toEqual({ proposer: 'claude', reviewers: ['grok', 'agy'] })
    await page.getByRole('tab', { name: '投票' }).click()
    const matrix = page.locator('.vote-matrix')
    await expect(matrix).toContainText('Grok')
    await expect(matrix).not.toContainText('Claude')
    await page.screenshot({ path: 'test-results/participants-three-cli.png', fullPage: true })
    await page.getByTestId('new-chat-button').click()
    await selectDropdown(page, 'mode-select', 'delphi')
    await page.getByTestId('goal-input').fill('固定 Mock：三家参与时匿名专家角色与汇总角色应保持正确。')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    await expect(page.getByTestId('message-list')).toContainText('专家 A')
    await expect(page.getByTestId('message-list')).toContainText('专家 B')
    await expect(page.getByTestId('message-list')).not.toContainText('Claude')
    await expect(page.getByTestId('message-list')).not.toContainText('Grok')
    await openDiscussionContext(page)
    await expect(page.getByTestId('participant-claude')).toContainText('提案协调器')
    await expect(page.getByTestId('participant-grok')).toContainText('专家 A')
    await expect(page.getByTestId('participant-agy')).toContainText('专家 B')
    await app.close()
    app = await launch(directory, '20')
    page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('new-chat-button').click()
    await page.getByTestId('provider-settings-button').click()
    await expect(page.getByTestId('provider-enabled-codex')).not.toBeChecked()
    await page.getByTestId('provider-enabled-agy').uncheck()
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('preparation-provider-select')).toHaveAttribute('data-value', 'claude')
    await expect(page.getByRole('status').filter({ hasText: '原检查提供方已停用' })).toBeVisible()
    await openDiscussionContext(page)
    await expect(page.getByTestId('participant-gate')).toBeVisible()
    await page.getByTestId('goal-input').fill('人数不足时不应先花额度进行自动分析。')
    await expect(page.getByTestId('send-button')).toBeDisabled()
    await page.getByTestId('goal-input').press('Enter')
    await expect(page.getByTestId('preparation-card')).toHaveCount(0)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(2)
    await page.getByTestId('provider-settings-button').click()
    await page.getByTestId('provider-enabled-claude').uncheck()
    await page.getByTestId('provider-enabled-grok').uncheck()
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('preparation-provider-select')).toBeDisabled()
    await expect(page.getByTestId('analyze-topic-button')).toBeDisabled()
    expect((await page.evaluate(() => window.tcview.getProviderSettings())).every(provider => provider.effective?.enabled === false)).toBe(true)
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})

test('启用状态的环境覆盖明确显示，保存关闭后仍按有效配置参与（显式 Mock）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-enabled-env-'))
  const app = await launch(directory, '20', { TCVIEW_GROK_ENABLED: 'true' })
  try {
    const page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await page.getByTestId('provider-settings-button').click()
    await page.getByTestId('provider-enabled-grok').uncheck()
    await page.getByTestId('save-provider-settings').click()
    await expect(page.getByTestId('provider-settings-save-state')).toHaveText('已保存配置')
    await expect(page.getByTestId('provider-enabled-effective-grok')).toContainText('已保存：停用 · 当前有效：启用')
    await expect(page.getByTestId('provider-env-notice-grok')).toContainText('TCVIEW_GROK_ENABLED')
    await page.keyboard.press('Escape')
    await openDiscussionContext(page)
    await expect(page.getByTestId('participant-grok')).toBeVisible()
    await expect(page.getByTestId('role-plan')).toContainText('3 位独立评审')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app.close(); await rm(directory, { recursive: true, force: true }) }
})

test('最近讨论可搜索、删除与重启后恢复，运行中的讨论要求先停止（显式 Mock）', async () => {
  test.setTimeout(70_000)
  const directory = await mkdtemp(join(tmpdir(), 'tcview-history-ui-'))
  let app: ElectronApplication | undefined
  try {
    app = await launch(directory, '30')
    let page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：保留的第一份历史讨论')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    const retained = (await page.evaluate(() => window.tcview.listRuns()))[0]
    await page.getByTestId('new-chat-button').click()
    await selectDropdown(page, 'mode-select', 'free_debate')
    await page.getByTestId('goal-input').fill('固定 Mock：要移动到最近删除的第二份讨论')
    await page.getByTestId('send-button').click()
    await expect(page.getByTestId('stop-button')).toBeEnabled()
    const deleting = (await page.evaluate(() => window.tcview.listRuns())).find(run => run.id !== retained.id)!
    await page.getByTestId(`delete-run-${deleting.id}`).click()
    await expect(page.getByRole('alert')).toContainText('请先停止讨论')
    expect(await page.evaluate(() => window.tcview.listDeletedRuns())).toHaveLength(0)
    await expect(page.getByTestId('run-status')).toContainText('已完成')
    await page.getByTestId(`delete-run-${deleting.id}`).click()
    await expect(page.getByTestId(`history-row-${deleting.id}`)).toHaveCount(0)
    await expect(page.getByTestId('undo-delete-run')).toBeVisible()
    await expect(page.getByTestId(`history-row-${retained.id}`)).toBeVisible()
    await page.getByTestId('history-search').fill('不存在的搜索结果')
    await expect(page.getByText('没有匹配的讨论。', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: '清除讨论搜索' }).click()
    await page.getByTestId('deleted-history-tab').click()
    await expect(page.getByTestId(`history-row-${deleting.id}`)).toContainText('删除于')
    await page.screenshot({ path: 'test-results/history-recently-deleted.png', fullPage: true })
    await app.close()
    app = await launch(directory, '20')
    page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(1)
    await page.getByTestId('deleted-history-tab').click()
    await page.getByTestId(`restore-run-${deleting.id}`).click()
    await expect(page.getByTestId('recent-history-tab')).toHaveClass('selected')
    await expect(page.getByTestId(`history-row-${deleting.id}`)).toBeVisible()
    expect(await page.evaluate(() => window.tcview.listDeletedRuns())).toHaveLength(0)
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(2)
    const restored = await page.evaluate(id => window.tcview.getRun(id), deleting.id)
    expect(restored.deletedAt).toBeFalsy()
    expect(restored.usage?.calls).toBeGreaterThan(0)
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})

test('三栏拖动与方向键宽度自动保存，窄窗不覆盖首选，重启保留并可复位（不调用模型）', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-panel-layout-'))
  let app: ElectronApplication | undefined
  try {
    app = await launch(directory)
    let page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await setViewportAndWait(page, { width: 1400, height: 900 })
    const left = page.getByTestId('resize-left')
    const right = page.getByTestId('resize-right')
    await expect(left).toBeVisible()
    await expect(left).toHaveAttribute('aria-valuenow', '218')
    await expect(right).toBeVisible()
    await expect(right).toHaveAttribute('aria-valuenow', '310')
    await left.focus()
    await expect(left).toBeFocused()
    await left.press('ArrowRight')
    await expect(left).toHaveAttribute('aria-valuenow', '226')
    await right.focus()
    await expect(right).toBeFocused()
    await right.press('Shift+ArrowLeft')
    await expect(right).toHaveAttribute('aria-valuenow', '342')
    const handle = (await left.boundingBox())!
    await page.mouse.move(handle.x + 3, handle.y + 300)
    await page.mouse.down()
    await expect(left).toHaveClass(/\bdragging\b/)
    await page.mouse.move(handle.x + 67, handle.y + 300, { steps: 8 })
    await expect(left).toHaveAttribute('aria-valuenow', '290')
    await page.mouse.up()
    await expect(left).not.toHaveClass(/\bdragging\b/)
    await expect(left).toHaveAttribute('aria-valuenow', '290')
    await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('tcview.layout.v1') ?? '{}'))).toEqual({ left: 290, right: 342 })
    await setViewportAndWait(page, { width: 980, height: 760 })
    await expect(right).not.toBeVisible()
    await setViewportAndWait(page, { width: 1400, height: 900 })
    await expect(left).toHaveAttribute('aria-valuenow', '290')
    await expect(right).toHaveAttribute('aria-valuenow', '342')
    expect(await page.locator('.main-panel').evaluate(node => node.clientWidth)).toBeGreaterThanOrEqual(420)
    await page.screenshot({ path: 'test-results/resizable-desktop-panels.png', fullPage: true })
    await app.close()
    app = await launch(directory)
    page = await app.firstWindow()
    await expect(page.getByTestId('connection-status')).toContainText('已连接')
    await setViewportAndWait(page, { width: 1400, height: 900 })
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '290')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '342')
    await page.getByTestId('resize-left').press('Home')
    await page.getByTestId('resize-right').dblclick()
    await expect(page.getByTestId('resize-left')).toHaveAttribute('aria-valuenow', '218')
    await expect(page.getByTestId('resize-right')).toHaveAttribute('aria-valuenow', '310')
    expect(await page.evaluate(() => window.tcview.listRuns())).toHaveLength(0)
  } finally { await app?.close(); await rm(directory, { recursive: true, force: true }) }
})
