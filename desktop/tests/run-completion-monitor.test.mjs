import assert from 'node:assert/strict'
import { test } from 'node:test'
import { build } from 'esbuild'
import { fileURLToPath } from 'node:url'

// 仅在内存中编译被测模块，不改动桌面 dist 或启动 Electron。
const { outputFiles } = await build({
  entryPoints: [fileURLToPath(new URL('../apps/desktop/main/run-completion-monitor.ts', import.meta.url))],
  bundle: true, platform: 'node', format: 'esm', write: false,
})
const { RunCompletionMonitor } = await import(`data:text/javascript;base64,${Buffer.from(outputFiles[0].contents).toString('base64')}`)

function snapshot(overrides = {}) {
  return {
    id: 'run-1', title: '测试讨论', goal: '测试', mode: 'free_debate', status: 'running', phase: 'proposal',
    createdAt: '2026-10-20T00:00:00Z', updatedAt: '2026-10-20T00:00:00Z',
    attemptId: 'attempt-1', hostPhase: 'running', deadlineAt: '2026-10-20T00:15:00Z',
    messages: [], reviews: [], claims: [], lastEventId: 1, ...overrides,
  }
}

function event(type, overrides = {}) {
  return { runId: 'run-1', attemptId: 'attempt-1', eventId: 2, at: '2026-10-20T00:00:01Z', type, payload: {}, ...overrides }
}

function monitor() {
  const notifications = []
  return { notifications, monitor: new RunCompletionMonitor({ notify: value => notifications.push(value) }) }
}

for (const status of ['completed', 'failed', 'stopped', 'interrupted']) {
  test(`实际终态 ${status} 只通知一次，后续导出、删除和清理不重复`, () => {
    const { monitor: subject, notifications } = monitor()
    subject.observe(snapshot())
    const finished = snapshot({ status, lastEventId: 2, error: status === 'failed' ? '额度不足' : undefined })
    subject.observe(finished)
    subject.observe(finished)
    subject.observe({ ...finished, lastEventId: 3 })
    subject.observe({ ...finished, lastEventId: 4, deletedAt: '2026-10-20T00:00:02Z' })
    subject.observe({ ...finished, lastEventId: 5 })
    assert.deepEqual(notifications, [finished])
    assert.deepEqual(subject.watching(), [])
  })
}

test('初次历史列表与快照静默，重新打开和操作历史记录不触发', () => {
  const { monitor: subject, notifications } = monitor()
  const history = snapshot({ status: 'completed', lastEventId: 7 })
  subject.observeSummary(history)
  subject.observe(history)
  subject.observe({ ...history, lastEventId: 8 })
  // 同一个幂等请求返回已经存在的历史终态，也不能重复通知。
  subject.observe({ ...history, lastEventId: 9 }, true)
  assert.equal(notifications.length, 0)
  assert.deepEqual(subject.watching(), [])
})

test('活跃列表与首次 get 之间快速结束仍通知，终态列表不提前解除观察', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observeSummary(snapshot())
  subject.observeSummary(snapshot({ status: 'completed' }))
  assert.deepEqual(subject.watching(), ['run-1'])
  subject.observe(snapshot({ status: 'completed', lastEventId: 3 }))
  assert.equal(notifications.length, 1)
})

test('新建、分支或恢复 RPC 首次直接返回终态也通知', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot({ status: 'completed' }), true)
  subject.observe(snapshot({ id: 'branch-1', status: 'failed' }), true)
  assert.equal(notifications.length, 2)
})

test('停止后恢复使用新 attempt，可再次通知；旧事件和旧快照不能解除新观察', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot())
  subject.observe(snapshot({ status: 'stopped', lastEventId: 3 }))
  assert.equal(subject.handleEvent(event('run.resumed', { attemptId: 'attempt-2', eventId: 4 })), true)
  assert.equal(subject.handleEvent(event('run.stopped', { eventId: 3 })), false)
  // 即使旧 attempt 的事件错误地晚到，也不能取消新 attempt。
  assert.equal(subject.handleEvent(event('run.failed', { eventId: 10 })), false)
  subject.observe(snapshot({ status: 'failed', lastEventId: 10 }))
  assert.deepEqual(subject.watching(), ['run-1'])
  subject.observe(snapshot({ attemptId: 'attempt-2', status: 'completed', lastEventId: 5 }))
  assert.equal(notifications.length, 2)
  assert.deepEqual(notifications.map(value => value.attemptId), ['attempt-1', 'attempt-2'])
})

test('恢复 RPC 可以观察到没有 start 事件的新 attempt，随后正常结束', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot({ status: 'failed' }))
  subject.observe(snapshot({ attemptId: 'attempt-2', lastEventId: 3 }), true)
  subject.observe(snapshot({ attemptId: 'attempt-2', status: 'stopped', lastEventId: 4 }))
  assert.equal(notifications.length, 1)
})

test('活跃列表可观察历史记录的未知新 attempt 并捕获其快速结束', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot({ status: 'failed', lastEventId: 3 }))
  subject.observeSummary(snapshot({ status: 'running' }))
  assert.equal(subject.handleEvent(event('run.stopped', { attemptId: 'attempt-2', eventId: 5 })), true)
  subject.observe(snapshot({ attemptId: 'attempt-2', status: 'stopped', lastEventId: 5 }))
  assert.equal(notifications.length, 1)
})

test('终态事件只要求读取快照；正在停止仍是运行中', () => {
  const { monitor: subject, notifications } = monitor()
  assert.equal(subject.handleEvent(event('run.started', { eventId: 1 })), true)
  subject.observe(snapshot({ phase: 'stopping', lastEventId: 2 }))
  assert.equal(subject.handleEvent(event('run.stopped', { eventId: 3, payload: { error: '此内容不作最终结果' } })), true)
  assert.equal(notifications.length, 0)
  assert.deepEqual(subject.watching(), ['run-1'])
  subject.observe(snapshot({ status: 'stopped', lastEventId: 3 }))
  assert.equal(notifications.length, 1)
})

test('完成事件为 decision.finalized，重复或过时 eventId 不倒退状态', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot({ lastEventId: 5 }))
  assert.equal(subject.handleEvent(event('run.started', { eventId: 1 })), false)
  assert.equal(subject.handleEvent(event('decision.finalized', { eventId: 6 })), true)
  assert.equal(subject.handleEvent(event('decision.finalized', { eventId: 6 })), false)
  subject.observe(snapshot({ status: 'completed', lastEventId: 4 }))
  assert.equal(notifications.length, 0)
  subject.observe(snapshot({ status: 'completed', lastEventId: 6 }))
  assert.equal(notifications.length, 1)
})

test('后台断开不伪造失败，重连快照确认中断后只通知一次', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot())
  assert.equal(subject.handleEvent(event('host.disconnected', { eventId: 0, runId: '', attemptId: '' })), false)
  assert.equal(notifications.length, 0)
  assert.equal(subject.handleEvent(event('state.reconciled', { eventId: 3 })), true)
  subject.observe(snapshot({ status: 'interrupted', lastEventId: 3 }))
  subject.observe(snapshot({ status: 'interrupted', lastEventId: 4 }))
  assert.equal(notifications.length, 1)
})

test('丢失全部终态事件时轮询仍能发现结束，且不依赖选中的讨论', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observeSummary(snapshot())
  subject.observeSummary(snapshot({ id: 'run-2' }))
  assert.deepEqual(subject.watching(), ['run-1', 'run-2'])
  subject.observe(snapshot({ status: 'completed', lastEventId: 20 }))
  assert.deepEqual(subject.watching(), ['run-2'])
  assert.equal(notifications.length, 1)
})

test('deleted 快照不通知，恢复被删除的历史记录也不通知', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot())
  subject.observe(snapshot({ status: 'failed', lastEventId: 3, deletedAt: '2026-10-20T00:00:02Z' }))
  subject.observe(snapshot({ status: 'failed', lastEventId: 4 }))
  subject.observeSummary(snapshot({ status: 'running', deletedAt: '2026-10-20T00:00:02Z' }))
  assert.equal(notifications.length, 0)
  assert.deepEqual(subject.watching(), [])
})

test('没有 attemptId 和 eventId 的旧快照可读；恢复到新格式后仍正常通知', () => {
  const { monitor: subject, notifications } = monitor()
  const legacy = snapshot({ attemptId: undefined, lastEventId: undefined, status: 'interrupted' })
  subject.observe(legacy)
  subject.observe(legacy)
  assert.equal(notifications.length, 0)
  subject.observe(snapshot({ attemptId: 'attempt-2', lastEventId: 1 }), true)
  subject.observe(legacy)
  assert.deepEqual(subject.watching(), ['run-1'])
  subject.observe(snapshot({ attemptId: 'attempt-2', lastEventId: 2, status: 'completed' }))
  assert.equal(notifications.length, 1)
})

test('旧格式活跃记录没有 attemptId，后续补齐身份的终态仍通知', () => {
  const { monitor: subject, notifications } = monitor()
  subject.observe(snapshot({ attemptId: undefined, lastEventId: undefined }))
  subject.observe(snapshot({ status: 'completed', lastEventId: 2 }))
  assert.equal(notifications.length, 1)
})

test('预算、到期及 completed 判定失败保留真实快照，不改写成功或错误', () => {
  const { monitor: subject, notifications } = monitor()
  const results = [
    snapshot({ id: 'budget', status: 'failed', error: '已达到本讨论 30 次 CLI 请求上限' }),
    snapshot({ id: 'expired', status: 'failed', error: '已达到原讨论期限，请新建讨论' }),
    snapshot({ id: 'decision-failed', status: 'completed', decision: { outcome: 'failed', coreOutcome: 'failed', summary: '判定错误', mock: true, votes: [] } }),
    snapshot({ id: 'no-consensus', status: 'completed', decision: { outcome: 'no_consensus', coreOutcome: 'unresolved_conflict', summary: '意见未收敛', mock: true, votes: [] } }),
  ]
  for (const result of results) subject.observe(result, true)
  assert.deepEqual(notifications, results)
})

test('普通进度不会产生拉取风暴或终态通知，未知历史终态事件不自行 arm', () => {
  const { monitor: subject, notifications } = monitor()
  assert.equal(subject.handleEvent(event('decision.finalized')), false)
  subject.observe(snapshot({ status: 'completed', lastEventId: 2 }))
  subject.observe(snapshot({ id: 'live', lastEventId: 3 }))
  assert.equal(subject.handleEvent(event('message.delta', { runId: 'live', eventId: 4 })), false)
  assert.equal(notifications.length, 0)
  assert.deepEqual(subject.watching(), ['live'])
})

test('summary 的最小协议结构足以建立初始基线', () => {
  const { monitor: subject } = monitor()
  const summary = { id: 'summary', title: '', goal: '', mode: 'free_debate', status: 'queued', phase: '', createdAt: '', updatedAt: '' }
  subject.observeSummary(summary)
  assert.deepEqual(subject.watching(), ['summary'])
})
