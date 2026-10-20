// 显式运行才使用真实额度：仅点击一次批量按钮，四家各至多一次模型请求，无重试。
// 使用已打包桌面、独立应用数据目录和原生 CLI 登录；不创建讨论或修改全局 CLI 设置。
import assert from 'node:assert/strict'
import { _electron as electron } from '@playwright/test'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { root, engineBuildVersion } from './go-env.mjs'

const ids = ['codex', 'claude', 'grok', 'agy']
const metadata = JSON.parse(await readFile(join(root, 'package.json'), 'utf8'))
const productName = metadata.productName
const binary = join(root, 'release', `${productName}-darwin-${process.arch}`, `${productName}.app/Contents/MacOS/${productName}`)
const directory = join(root, '.tcview', 'provider-batch-validation', new Date().toISOString().replaceAll(':', '-'))
await mkdir(directory, {recursive: true, mode: 0o700})
const expectedEngineVersion = engineBuildVersion()
const env = {...process.env, TCVIEW_DATA_DIR: join(directory, 'host'), TCVIEW_USER_DATA_DIR: join(directory, 'electron'), TCVIEW_PROVIDER_MODE: 'cli'}
// 只清理本验收子进程的应用配置覆盖，认证继续由各家 CLI 管理。
for (const id of ids) for (const field of ['MODEL', 'REASONING', 'EFFORT', 'ENABLED']) delete env[`TCVIEW_${id.toUpperCase()}_${field}`]
delete env.ELECTRON_RUN_AS_NODE
const configurations = [
  {id: 'codex', enabled: true, model: 'gpt-6.1-sol', reasoning: 'low'},
  {id: 'claude', enabled: true, model: 'claude-opus-5-5', reasoning: 'low'},
  {id: 'grok', enabled: true, model: 'grok-4.7', reasoning: 'low'},
  // 停用仍应参加明确点击的全部连接检查；effort 编码在真实完整模型 ID 中。
  {id: 'agy', enabled: false, model: 'gemini-3.8-flash-low', reasoning: ''},
]
const summary = {
  directory, binary, startedAt: new Date().toISOString(), providerMode: 'cli', maxModelRequests: 4,
  configurations, batchButtonClicks: 0, batchCompleted: false, results: [], rendererErrors: [], errors: [], progress: [],
}
const deadline = Date.now() + 650_000
let app
let page
let previous = ''
async function persist() {
  await writeFile(join(directory, 'summary.json'), JSON.stringify(summary, null, 2), {mode: 0o600})
}
async function publicResults() {
  const results = []
  for (const id of ids) {
    const card = page.getByTestId(`provider-card-${id}`)
    const result = card.getByTestId(`provider-check-result-${id}`)
    const checked = await result.count() > 0
    const status = checked ? await result.locator('strong').first().innerText() : '尚无检查结果'
    results.push({
      id, enabled: configurations.find(item => item.id === id).enabled,
      cliVersion: summary.saved?.find(item => item.id === id)?.version,
      status, requestOK: status === '真实请求通过',
      requestedModel: checked ? await card.getByTestId(`provider-requested-model-${id}`).innerText() : undefined,
      reportedModel: checked ? await card.getByTestId(`provider-reported-model-${id}`).innerText() : undefined,
      requestedReasoning: checked ? await result.getByText('请求推理配置', {exact: true}).locator('..').locator('dd').innerText() : undefined,
      reportedReasoning: checked ? await card.getByTestId(`provider-reported-reasoning-${id}`).innerText() : undefined,
      checkedAtAndLatency: checked ? await result.locator(':scope > div > span').innerText() : undefined,
      resultText: checked ? await result.innerText() : undefined,
      error: await card.locator('.provider-error-message').count() ? await card.locator('.provider-error-message').innerText() : undefined,
    })
  }
  return results
}
try {
  console.log(JSON.stringify({directory, phase: '启动独立真实验收', maxModelRequests: 4}))
  await persist()
  app = await electron.launch({executablePath: binary, env, timeout: 40_000})
  page = await app.firstWindow()
  page.on('pageerror', error => summary.rendererErrors.push(error.message))
  if (!process.argv.includes('--show-window')) await app.evaluate(({BrowserWindow}) => { for (const window of BrowserWindow.getAllWindows()) window.hide() })
  await page.getByTestId('connection-status').filter({hasText: '已连接'}).waitFor({timeout: 40_000})
  summary.hello = await page.evaluate(() => window.tcview.hello())
  assert.equal(summary.hello.providerMode, 'cli')
  assert.equal(summary.hello.protocolVersion, '1.4')
  assert.equal(summary.hello.engineVersion, expectedEngineVersion)
  // Codex 显式 effort 的保存校验需要 app-server 实际能力目录；此步骤不发模型消息。
  const codexCatalog = await page.evaluate(() => window.tcview.reloadProviderModels('codex'))
  const model = codexCatalog.models.find(item => item.id === 'gpt-6.1-sol')
  assert.ok(!codexCatalog.error && model?.reasoningMode === 'argument' && model.reasoningValues.includes('low'), '真实 Codex 目录未确认 gpt-6.1-sol/low；不会猜参数或派发模型请求')
  await writeFile(join(directory, 'codex-catalog.json'), JSON.stringify(codexCatalog, null, 2), {mode: 0o600})
  const saved = await page.evaluate(configs => window.tcview.saveProviderSettings(configs), configurations)
  summary.saved = saved.map(item => ({id: item.id, enabled: item.enabled, version: item.version, available: item.available, saved: item.saved, effective: item.effective, sources: item.sources}))
  for (const config of configurations) {
    const view = saved.find(item => item.id === config.id)
    assert.equal(view.saved.model, config.model)
    assert.equal(view.effective.model, config.model)
    assert.equal(view.effective.reasoning, config.reasoning)
    assert.equal(view.effective.enabled, config.enabled)
  }
  await page.getByTestId('provider-settings-button').click()
  await page.getByTestId('check-all-providers').waitFor({state: 'visible'})
  assert.equal(await page.getByTestId('provider-enabled-agy').isChecked(), false)
  await writeFile(join(directory, 'before.txt'), await page.getByRole('dialog').innerText(), {mode: 0o600})
  await page.screenshot({path: join(directory, 'desktop-before.png'), fullPage: true})
  summary.batchButtonClicks++
  await page.getByTestId('check-all-providers').click({clickCount: 1})
  const status = page.getByTestId('provider-batch-status')
  await status.waitFor({state: 'visible'})
  while (Date.now() < deadline) {
    const progress = await status.innerText()
    if (progress !== previous) {
      previous = progress
      const update = {at: new Date().toISOString(), progress}
      summary.progress.push(update)
      summary.results = await publicResults()
      console.log(JSON.stringify({directory, ...update, checked: summary.results.filter(item => item.status !== '尚无检查结果').map(({id, status, error}) => ({id, status, error}))}))
      await persist()
    }
    if (progress.includes('全部检查完成')) { summary.batchCompleted = true; break }
    if (progress.includes('已停止后续检查')) throw new Error('真实批次意外停止，禁止自动重试')
    await page.waitForTimeout(750)
  }
  if (!summary.batchCompleted) throw new Error('真实批次在总计 650 秒期限内未完成，禁止自动重试')
  summary.finalStatus = await status.innerText()
  summary.results = await publicResults()
  assert.ok(summary.finalStatus.includes('已完成 4/4'))
  assert.equal(summary.results.length, 4)
  assert.equal(summary.results.every(item => item.status !== '尚无检查结果'), true)
  assert.deepEqual(await page.evaluate(() => window.tcview.listRuns()), [], '连接检查不得发起整场讨论')
  assert.equal(summary.rendererErrors.length, 0)
  await writeFile(join(directory, 'desktop-final.txt'), await page.getByRole('dialog').innerText(), {mode: 0o600})
  await page.screenshot({path: join(directory, 'desktop-final.png'), fullPage: true})
} catch (error) {
  summary.errors.push(error.message)
  if (page) {
    try { summary.results = await publicResults() } catch {}
    try { await page.screenshot({path: join(directory, 'desktop-error.png'), fullPage: true}) } catch {}
  }
} finally {
  if (app) await app.close()
  summary.finishedAt = new Date().toISOString()
  summary.success = summary.batchCompleted && !summary.errors.length && !summary.rendererErrors.length
  summary.realRequestsPassed = summary.results.filter(item => item.requestOK).length
  summary.realRequestsFailed = summary.results.filter(item => item.status !== '尚无检查结果' && !item.requestOK).length
  // success 只表示批量 UI 完整执行；逐家真实调用通过数独立记录，失败不会改写成成功。
  summary.allRealRequestsPassed = summary.results.length === 4 && summary.realRequestsPassed === 4
  await persist()
  console.log(JSON.stringify({directory, success: summary.success, batchCompleted: summary.batchCompleted, realRequestsPassed: summary.realRequestsPassed, realRequestsFailed: summary.realRequestsFailed, allRealRequestsPassed: summary.allRealRequestsPassed, rendererErrors: summary.rendererErrors, errors: summary.errors}))
  if (!summary.success) process.exitCode = 1
}
