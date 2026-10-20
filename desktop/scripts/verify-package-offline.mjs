// CI 离线安装包烟测：只使用显式 Mock，不依赖本机 CLI 或认证，不发模型请求。
import assert from 'node:assert/strict'
import { _electron as electron } from '@playwright/test'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

if (process.platform !== 'darwin') throw new Error('离线安装包烟测目前仅支持 macOS 开发包')
const root = resolve(import.meta.dirname, '..')
const metadata = JSON.parse(await readFile(join(root, 'package.json'), 'utf8'))
const appPath = join(root, 'release', `${metadata.productName}-darwin-${process.arch}`, `${metadata.productName}.app`)
const binary = join(appPath, 'Contents/MacOS', metadata.productName)
const resources = join(appPath, 'Contents/Resources/app')
const buildInfo = JSON.parse(await readFile(join(resources, 'build-info.json'), 'utf8'))
const bundlePlist = await readFile(join(appPath, 'Contents/Info.plist'), 'utf8')
assert.match(bundlePlist, /<key>CFBundleIdentifier<\/key>\s*<string>local\.tcview\.desktop<\/string>/)
assert.equal(buildInfo.appBundleId, 'local.tcview.desktop')
assert.equal(buildInfo.hostBinary, 'tcview-host')
assert.match(buildInfo.repositoryRevision, /^[0-9a-f]{40}$/)
assert.match(buildInfo.sourceHash, /^[0-9a-f]{64}$/)
assert.equal(typeof buildInfo.dirty, 'boolean')
const hostBuildInfo = JSON.parse(await readFile(join(resources, 'dist/host-build-info.json'), 'utf8'))
for (const field of ['repositoryRevision', 'sourceHash', 'engineVersion', 'dirty']) assert.equal(buildInfo[field], hostBuildInfo[field])
assert.deepEqual(await readdir(join(resources, 'dist/bin')), ['tcview-host'])

const directory = join(root, '.tcview/package-offline-validation', new Date().toISOString().replaceAll(':', '-'))
await mkdir(directory, { recursive: true, mode: 0o700 })
// 不把开发者的 TCVIEW 覆盖、凭据、CLI 路径或宿主嵌套会话传进子进程。
const env = Object.fromEntries(['HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'CI'].filter(key => process.env[key] !== undefined).map(key => [key, process.env[key]]))
Object.assign(env, {
  PATH: '/usr/bin:/bin',
  TCVIEW_DATA_DIR: join(directory, 'host'),
  TCVIEW_USER_DATA_DIR: join(directory, 'electron'),
  TCVIEW_PROVIDER_MODE: 'mock',
  TCVIEW_MOCK_DELAY_MS: '5',
})
const rendererErrors = []
let app
const launch = async () => {
  const instance = await electron.launch({ executablePath: binary, env })
  const page = await instance.firstWindow()
  page.on('pageerror', error => rendererErrors.push(error.message))
  await page.getByTestId('connection-status').filter({ hasText: '已连接' }).waitFor()
  return { instance, page }
}
try {
  const first = await launch()
  app = first.instance
  const page = first.page
  await page.evaluate(() => window.tcview.saveNotificationSettings({ enabled: false, soundEnabled: false }))
  const hello = await page.evaluate(() => window.tcview.hello())
  assert.equal(hello.providerMode, 'mock', 'CI 安装包烟测必须显式使用 Mock')
  assert.equal(hello.protocolVersion, '1.4')
  assert.equal(hello.engineVersion, buildInfo.engineVersion)
  const identity = await app.evaluate(({ app, BrowserWindow }) => ({
    name: app.getName(), userData: app.getPath('userData'), sessionData: app.getPath('sessionData'),
    titles: BrowserWindow.getAllWindows().map(window => window.getTitle()),
  }))
  assert.equal(identity.name, metadata.productName)
  assert.equal(identity.userData, env.TCVIEW_USER_DATA_DIR)
  assert.equal(identity.sessionData, env.TCVIEW_USER_DATA_DIR)
  assert.equal(identity.titles.length, 1)
  assert.ok(identity.titles[0].startsWith(metadata.productName))

  await page.getByTestId('mode-select').click()
  await page.locator('.carbon-select-menu [role="option"][data-value="free_debate"]').click()
  await page.getByTestId('goal-input').fill('显式 Mock：验收同仓 TCView 安装包的聊天、共识内核和持久化。')
  await page.getByTestId('send-button').click()
  await page.getByTestId('message-list').filter({ hasText: 'Mock' }).waitFor()
  const runs = await page.evaluate(() => window.tcview.listRuns())
  assert.equal(runs.length, 1)
  const started = runs[0]
  let completed
  const deadline = Date.now() + 30_000
  while (Date.now() < deadline) {
    const run = await page.evaluate(runId => window.tcview.getRun(runId), started.id)
    if (run.status === 'failed') throw new Error(`Mock 安装包讨论失败：${run.error ?? '未报告原因'}`)
    if (run.status === 'completed') { completed = run; break }
    await new Promise(resolve => setTimeout(resolve, 50))
  }
  assert.ok(completed, 'Mock 安装包讨论未在 30 秒内完成')
  assert.equal(completed.decision?.mock, true)
  assert.ok(completed.proposal?.hash)
  assert.ok(completed.messages.length >= 4)
  const events = await page.evaluate(runId => window.tcview.getEvents(runId, 0), started.id)
  assert.ok(events.length > 5)

  const reconnected = await page.evaluate(() => window.tcview.reconnectHost())
  assert.equal(reconnected.providerMode, 'mock')
  assert.equal(reconnected.engineVersion, hello.engineVersion)
  await page.screenshot({ path: join(directory, 'desktop.png'), fullPage: true })
  await app.close()
  app = undefined

  const second = await launch()
  app = second.instance
  const afterRestart = await second.page.evaluate(runId => window.tcview.getRun(runId), started.id)
  assert.equal(afterRestart.status, 'completed')
  assert.equal(afterRestart.proposal?.hash, completed.proposal.hash)
  assert.equal(afterRestart.decision?.mock, true)
  const restartedHello = await second.page.evaluate(() => window.tcview.hello())
  assert.equal(restartedHello.engineVersion, hello.engineVersion)
  assert.equal(restartedHello.providerMode, 'mock')
  assert.deepEqual(rendererErrors, [])

  const result = {
    passed: true, providerMode: 'mock', realModelRequests: 0, cliRequired: false,
    buildInfo, identity, hello, runId: started.id, mockDiscussionCompleted: true,
    persistedProposalHash: completed.proposal.hash, eventCount: events.length,
    reconnected: true, restarted: true, rendererErrors,
  }
  await writeFile(join(directory, 'summary.json'), `${JSON.stringify(result, null, 2)}\n`, { mode: 0o600 })
  console.log(JSON.stringify({ passed: true, providerMode: 'mock', realModelRequests: 0, directory }, null, 2))
} finally {
  await app?.close()
}
