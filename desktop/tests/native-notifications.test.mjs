import test from 'node:test'
import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { mkdtemp, mkdir, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'
import { setImmediate as nextTurn } from 'node:timers/promises'
import { build } from 'esbuild'

// 显式通知替身：验证投递事件语义，不声称发送过系统通知或播放过声音。
const compilation = await mkdtemp(join(tmpdir(), 'tcview-native-notification-test-'))
const entry = new URL('../apps/desktop/main/native-notifications.ts', import.meta.url)
await build({ entryPoints: [entry.pathname], outfile: join(compilation, 'notifications.cjs'), bundle: true, platform: 'node', format: 'cjs', external: ['electron'], logLevel: 'silent' })
const { NativeNotifications } = createRequire(import.meta.url)(join(compilation, 'notifications.cjs'))
test.after(() => rm(compilation, { recursive: true, force: true }))

async function fixture(t, { supported = true, behavior = 'manual' } = {}) {
  const directory = await mkdtemp(join(tmpdir(), 'tcview-native-settings-test-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const sent = [], opened = []
  const native = {
    isSupported: () => supported,
    create(options) {
      const notification = new EventEmitter()
      notification.options = options
      notification.show = () => {
        notification.shown = true
        if (behavior === 'show') notification.emit('show')
        if (behavior === 'fail') notification.emit('failed', {}, 'RAW-CREDENTIAL-DO-NOT-EXPOSE')
        if (behavior === 'throw') throw new Error('RAW-CREDENTIAL-DO-NOT-EXPOSE')
      }
      notification.close = () => { notification.closed = true; notification.emit('close') }
      sent.push(notification)
      return notification
    },
  }
  const notifications = new NativeNotifications(directory, id => opened.push(id), native)
  await notifications.initialize()
  return { directory, native, notifications, sent, opened }
}

function run(overrides = {}) {
  return { id: 'run-123', title: '讨论短标题', goal: '完整私密问题不能出现在系统通知', status: 'completed', providerMode: 'cli', error: 'CLI-SECRET-ERROR', decision: { outcome: 'supported', summary: 'LONG-PRIVATE-SUMMARY', mock: false }, ...overrides }
}

test('缺省开启通知和系统声音，设置原子持久化，读取和返回值不泄漏可变状态', async t => {
  const { directory, native, notifications } = await fixture(t)
  assert.deepEqual(notifications.getSettings(), { enabled: true, soundEnabled: true, supported: true })
  const input = { enabled: true, soundEnabled: false }
  const saving = notifications.save(input)
  input.enabled = false
  await saving
  assert.deepEqual(JSON.parse(await readFile(join(directory, 'notification-settings.json'), 'utf8')), { version: 1, enabled: true, soundEnabled: false })
  assert.deepEqual((await readdir(directory)).sort(), ['notification-settings.json'])
  if (process.platform !== 'win32') {
    assert.equal((await stat(directory)).mode & 0o777, 0o700)
    assert.equal((await stat(join(directory, 'notification-settings.json'))).mode & 0o777, 0o600)
  }
  const restarted = new NativeNotifications(directory, () => {}, native)
  await restarted.initialize()
  const settings = restarted.getSettings()
  settings.enabled = false
  assert.equal(restarted.getSettings().enabled, true)
  assert.equal(restarted.getSettings().soundEnabled, false)
})

test('损坏、格式错误和不可读设置安全停用，重新保存可修复', async t => {
  for (const contents of ['{invalid', '[]', '{"version":2,"enabled":true,"soundEnabled":true}', '{"version":1,"enabled":"true","soundEnabled":true}', undefined]) {
    const { directory, native } = await fixture(t)
    const path = join(directory, 'notification-settings.json')
    if (contents === undefined) await mkdir(path)
    else await writeFile(path, contents)
    const notifications = new NativeNotifications(directory, () => {}, native)
    await notifications.initialize()
    assert.equal(notifications.getSettings().enabled, false)
    assert.equal(notifications.getSettings().soundEnabled, false)
    assert.match(notifications.getSettings().error, /安全停用/)
    if (contents === undefined) await rm(path, { recursive: true })
    await notifications.save({ enabled: true, soundEnabled: false })
    assert.equal(notifications.getSettings().enabled, true)
    assert.equal(notifications.getSettings().error, undefined)
  }
})

test('保存串行执行，失败保留原设置且后续保存能够恢复', async t => {
  const { directory, notifications } = await fixture(t)
  const first = notifications.save({ enabled: false, soundEnabled: true })
  const second = notifications.save({ enabled: true, soundEnabled: false })
  assert.equal((await first).enabled, false)
  assert.equal((await second).enabled, true)
  assert.deepEqual(JSON.parse(await readFile(join(directory, 'notification-settings.json'), 'utf8')), { version: 1, enabled: true, soundEnabled: false })
  await rm(join(directory, 'notification-settings.json'))
  await mkdir(join(directory, 'notification-settings.json'))
  await assert.rejects(notifications.save({ enabled: false, soundEnabled: true }), /保存失败/)
  assert.equal(notifications.getSettings().enabled, true)
  assert.equal(notifications.getSettings().soundEnabled, false)
  assert.deepEqual((await readdir(directory)).sort(), ['notification-settings.json'])
  await rm(join(directory, 'notification-settings.json'), { recursive: true })
  await notifications.save({ enabled: false, soundEnabled: false })
  assert.equal(notifications.getSettings().enabled, false)
  await assert.rejects(notifications.save({ enabled: 'true', soundEnabled: true }), /布尔值/)
})

test('能力支持不等于投递成功，测试只有收到 show 才报告 shown，重复点击合并', async t => {
  const { notifications, sent } = await fixture(t)
  assert.equal(notifications.getSettings().supported, true)
  assert.equal(notifications.getSettings().lastDelivery, undefined)
  const first = notifications.test(), second = notifications.test()
  assert.equal(first, second)
  await nextTurn()
  assert.equal(sent.length, 1)
  assert.equal(sent[0].shown, true)
  assert.equal(sent[0].options.silent, false)
  assert.equal('sound' in sent[0].options, false)
  assert.equal(notifications.getSettings().lastDelivery, undefined)
  sent[0].emit('show')
  assert.deepEqual(await first, { status: 'shown' })
  assert.equal(notifications.getSettings().lastDelivery.status, 'shown')
  assert.match(notifications.getSettings().lastDelivery.at, /^\d{4}-/)
  const settings = notifications.getSettings()
  settings.lastDelivery.status = 'failed'
  assert.equal(notifications.getSettings().lastDelivery.status, 'shown')
  sent[0].emit('close')
  assert.equal(notifications.getSettings().lastDelivery.status, 'shown')
})

test('关闭、系统不支持、静音与实际原生失败有明确结果，不伪造成功', async t => {
  const disabled = await fixture(t)
  await disabled.notifications.save({ enabled: false, soundEnabled: false })
  assert.equal((await disabled.notifications.test()).status, 'failed')
  disabled.notifications.showRun(run())
  assert.equal(disabled.sent.length, 0)
  const unsupported = await fixture(t, { supported: false })
  assert.equal((await unsupported.notifications.test()).status, 'unsupported')
  unsupported.notifications.showRun(run())
  assert.equal(unsupported.sent.length, 0)
  const silent = await fixture(t, { behavior: 'show' })
  await silent.notifications.save({ enabled: true, soundEnabled: false })
  assert.equal((await silent.notifications.test()).status, 'shown')
  assert.equal(silent.sent[0].options.silent, true)
  for (const behavior of ['fail', 'throw']) {
    const failed = await fixture(t, { behavior })
    const result = await failed.notifications.test()
    assert.equal(result.status, 'failed')
    assert.doesNotMatch(JSON.stringify(result), /RAW-CREDENTIAL/)
    assert.equal(failed.notifications.getSettings().lastDelivery.status, 'failed')
  }
})

test('超时仅结束等待，不撤销系统通知，延迟显示仍更新真实状态', async t => {
  const { notifications, sent } = await fixture(t)
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const result = notifications.test()
  await nextTurn()
  assert.equal(sent.length, 1)
  t.mock.timers.tick(10_000)
  assert.equal((await result).status, 'timeout')
  assert.equal(notifications.getSettings().lastDelivery.status, 'timeout')
  assert.notEqual(sent[0].closed, true)
  sent[0].emit('show')
  assert.equal(notifications.getSettings().lastDelivery.status, 'shown')
  assert.equal(notifications.getSettings().lastDelivery.error, undefined)
  assert.equal((await result).status, 'timeout')
  sent[0].emit('close')
  assert.equal(notifications.getSettings().lastDelivery.status, 'shown')
})

test('运行提醒超时后仍可点击，系统稍后失败记录固定建议且不泄漏原文', async t => {
  const { notifications, sent, opened } = await fixture(t)
  t.mock.timers.enable({ apis: ['setTimeout'] })
  notifications.showRun(run())
  t.mock.timers.tick(10_000)
  assert.equal(notifications.getSettings().lastDelivery.status, 'timeout')
  assert.notEqual(sent[0].closed, true)
  sent[0].emit('click')
  assert.deepEqual(opened, ['run-123'])
  sent[0].emit('failed', {}, 'Permission denied: RAW-CREDENTIAL-DO-NOT-EXPOSE')
  assert.equal(notifications.getSettings().lastDelivery.status, 'failed')
  assert.match(notifications.getSettings().lastDelivery.error, /系统设置中允许 TCView/)
  assert.doesNotMatch(JSON.stringify(notifications.getSettings()), /RAW-CREDENTIAL/)
})

test('原生签名、标识和授权失败映射固定建议，未知错误保持泛化', async t => {
  const cases = [
    ['Signing validation failed RAW-CREDENTIAL', /已签名的 TCView/],
    ['Invalid bundle identifier /private/RAW-CREDENTIAL', /已签名的 TCView/],
    ['User notification permission denied RAW-CREDENTIAL', /系统设置中允许 TCView/],
    ['Authorization failed RAW-CREDENTIAL', /系统设置中允许 TCView/],
    ['Unrecognized error RAW-CREDENTIAL', /系统通知设置/],
  ]
  for (const [error, expected] of cases) {
    const { notifications, sent } = await fixture(t)
    const result = notifications.test()
    await nextTurn()
    sent[0].emit('failed', {}, error)
    assert.equal((await result).status, 'failed')
    assert.match((await result).error, expected)
    assert.doesNotMatch(JSON.stringify(notifications.getSettings()), /RAW-CREDENTIAL|\/private/)
  }
})

test('仅四种未删除的终态发送通知，判定失败与未达成共识保留真实含义，Mock 清楚标记', async t => {
  const { notifications, sent, opened } = await fixture(t, { behavior: 'show' })
  for (const status of ['queued', 'running']) notifications.showRun(run({ status }))
  notifications.showRun(run({ deletedAt: new Date().toISOString() }))
  assert.equal(sent.length, 0)
  for (const status of ['completed', 'failed', 'stopped', 'interrupted']) notifications.showRun(run({ status }))
  assert.equal(sent.length, 4)
  assert.match(sent[0].options.body, /方案获支持（supported）/)
  assert.match(sent[1].options.title, /讨论失败/)
  assert.doesNotMatch(sent[1].options.body, /方案获支持/)
  assert.match(sent[2].options.title, /已停止/)
  assert.match(sent[2].options.body, /恢复选项/)
  assert.match(sent[3].options.title, /已中断/)
  assert.match(sent[3].options.body, /恢复选项/)
  sent[0].emit('click')
  assert.deepEqual(opened, ['run-123'])
  notifications.showRun(run({ decision: { outcome: 'failed' } }))
  assert.match(sent.at(-1).options.title, /讨论失败/)
  assert.match(sent.at(-1).options.body, /判定失败（failed）/)
  notifications.showRun(run({ providerMode: 'mock', decision: { outcome: 'no_consensus' } }))
  assert.match(sent.at(-1).options.title, /Mock/)
  assert.match(sent.at(-1).options.body, /尚未达成共识（no_consensus）/)
  for (const notification of sent) assert.doesNotMatch(JSON.stringify(notification.options), /完整私密问题|CLI-SECRET-ERROR|LONG-PRIVATE-SUMMARY/)
})

test('通知文本长度受限，不发送未知结果原文、控制字符或任意外部链接回调', async t => {
  const { notifications, sent, opened } = await fixture(t, { behavior: 'show' })
  notifications.showRun(run({ id: 'https://untrusted.invalid', title: `A\n\u202e${'长'.repeat(200)}`, decision: { outcome: 'UNKNOWN-SECRET-OUTCOME', summary: 'SECRET' } }))
  assert.ok(Array.from(sent[0].options.body.split('\n')[0]).length <= 64)
  assert.doesNotMatch(sent[0].options.body, /\u202e|UNKNOWN-SECRET-OUTCOME|SECRET/)
  sent[0].emit('click')
  assert.deepEqual(opened, [])
})
