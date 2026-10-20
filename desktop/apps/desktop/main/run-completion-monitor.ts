import type { DesktopEvent, RunSnapshot, RunSummary } from '../../../packages/protocol/src/index'

const activeStatuses = new Set(['queued', 'running'])
const terminalStatuses = new Set(['completed', 'failed', 'stopped', 'interrupted'])
const startedEvents = new Set(['run.started', 'run.resumed'])
const terminalEvents = new Set(['decision.finalized', 'run.failed', 'run.stopped', 'run.interrupted'])

interface ObservedRun {
  lastEventId: number
  snapshotEventId: number
  attemptId: string
  armed: boolean
  // 列表没有 attemptId；在下一份快照到达前保留这次活跃观察。
  unknownAttempt: boolean
  retiredAttempts: Set<string>
}

const eventId = (value: number) => Number.isSafeInteger(value) && value >= 0 ? value : 0
const attemptId = (value: string) => typeof value === 'string' ? value : ''

/** 只跟踪本次进程实际观察到的运行，不把打开历史记录当作运行结束。 */
export class RunCompletionMonitor {
  private runs = new Map<string, ObservedRun>()
  private seen = new Set<string>()
  private notify: (snapshot: RunSnapshot) => void

  constructor({ notify }: { notify: (snapshot: RunSnapshot) => void }) {
    this.notify = notify
  }

  private get(id: string): ObservedRun {
    let run = this.runs.get(id)
    if (!run) {
      run = { lastEventId: -1, snapshotEventId: -1, attemptId: '', armed: false, unknownAttempt: false, retiredAttempts: new Set() }
      this.runs.set(id, run)
    }
    return run
  }

  observeSummary(summary: RunSummary): void {
    if (!summary.id) return
    const run = this.get(summary.id)
    if (summary.deletedAt) {
      run.armed = false
      run.unknownAttempt = false
    } else if (activeStatuses.has(summary.status)) {
      if (!run.armed) run.unknownAttempt = true
      run.armed = true
    }
    // 终态列表不能覆盖仍在等待确认的运行；通知需要完整快照。
  }

  observe(snapshot: RunSnapshot, initiated = false): void {
    if (!snapshot.id) return
    const run = this.get(snapshot.id)
    const nextEventId = eventId(snapshot.lastEventId)
    const nextAttempt = attemptId(snapshot.attemptId)
    // 事件已见过不等于快照已应用。同 cursor 的事件后快照仍必须接受。
    if (nextEventId < run.snapshotEventId || nextEventId < run.lastEventId || (nextAttempt && run.retiredAttempts.has(nextAttempt))) return
    // 缺少 attempt 的旧数据不能覆盖已经观察到的新 attempt。
    if (!nextAttempt && run.attemptId) return

    if (nextAttempt !== run.attemptId) {
      if (run.attemptId) run.retiredAttempts.add(run.attemptId)
      run.attemptId = nextAttempt
      run.armed = run.unknownAttempt
    }
    run.lastEventId = nextEventId
    run.snapshotEventId = nextEventId
    run.unknownAttempt = false

    if (snapshot.deletedAt) {
      run.armed = false
      if (terminalStatuses.has(snapshot.status)) this.seen.add(this.key(snapshot.id, nextAttempt))
      return
    }
    if (activeStatuses.has(snapshot.status)) {
      run.armed = true
      run.unknownAttempt = !nextAttempt
      return
    }
    if (!terminalStatuses.has(snapshot.status)) return

    const key = this.key(snapshot.id, nextAttempt)
    const shouldNotify = (run.armed || initiated) && !this.seen.has(key)
    // 在调用外部 Notification 前记下终态，防止重复事件与同步重入。
    this.seen.add(key)
    run.armed = false
    if (shouldNotify) this.notify(snapshot)
  }

  /** 返回 true 时调用方应获取真实快照；事件 payload 不足以确认最终结果。 */
  handleEvent(event: DesktopEvent): boolean {
    if (!event.runId || event.type === 'host.disconnected') return false
    const run = this.get(event.runId)
    const nextEventId = eventId(event.eventId)
    const nextAttempt = attemptId(event.attemptId)
    if (nextEventId <= run.lastEventId || (nextAttempt && run.retiredAttempts.has(nextAttempt))) return false

    if (startedEvents.has(event.type)) {
      if (nextAttempt && nextAttempt !== run.attemptId) {
        if (run.attemptId) run.retiredAttempts.add(run.attemptId)
        run.attemptId = nextAttempt
      }
      run.lastEventId = nextEventId
      run.unknownAttempt = !run.attemptId
      run.armed = true
      return true
    }

    // 晚到的旧 attempt 终态不能影响正在运行的新 attempt。
    if (nextAttempt && run.attemptId && nextAttempt !== run.attemptId && !run.unknownAttempt) return false
    run.lastEventId = nextEventId
    return run.armed && (terminalEvents.has(event.type) || event.type === 'state.reconciled' || event.type === 'state.legacy')
  }

  watching(): string[] {
    return [...this.runs].filter(([, run]) => run.armed).map(([id]) => id)
  }

  private key(id: string, attempt: string): string {
    return JSON.stringify([id, attempt])
  }
}
