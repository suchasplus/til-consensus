import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createInterface } from 'node:readline'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { resolve, join } from 'node:path'
import { randomUUID } from 'node:crypto'

const binary = resolve('dist/bin/tcview-host')
function connect(dataDir, delay = 12) {
  const env = { ...process.env }
  for (const key of Object.keys(env)) if (/^TCVIEW_(CODEX|CLAUDE|GROK|AGY)_(MODEL|REASONING|EFFORT|ENABLED)$/.test(key)) delete env[key]
  const child = spawn(binary, ['--data-dir', dataDir, '--mock-delay-ms', String(delay), '--provider-mode', 'mock'], { stdio: ['pipe', 'pipe', 'pipe'], env })
  let sequence = 0
  let diagnostics = ''
  let closed = false
  const requests = new Map()
  const notifications = []
  const exit = new Promise(resolve => child.once('close', resolve))
  child.stdin.on('error', () => {})
  child.stderr.on('data', data => { diagnostics = (diagnostics + data).slice(-8000) })
  createInterface({ input: child.stdout }).on('line', line => {
    const value = JSON.parse(line)
    if (value.method === 'event') notifications.push(value.params)
    const pending = requests.get(value.id)
    if (!pending) return
    clearTimeout(pending.timer)
    requests.delete(value.id)
    if (value.error) pending.reject(new Error(value.error.message))
    else pending.resolve(value.result)
  })
  return {
    child, notifications,
    call(method, params = {}) {
      const id = ++sequence
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => { requests.delete(id); reject(new Error(`RPC ${method} 超时：${diagnostics}`)) }, 12_000)
        requests.set(id, { resolve, reject, timer })
        child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n')
      })
    },
    async close() {
      if (closed) return
      closed = true
      child.stdin.end()
      const timer = setTimeout(() => child.kill('SIGKILL'), 4000)
      await exit
      clearTimeout(timer)
    },
    async crash() {
      if (closed) return
      closed = true
      child.kill('SIGKILL')
      await exit
    },
  }
}

async function until(fn, predicate, timeout = 12_000) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    const value = await fn()
    if (predicate(value)) return value
    await new Promise(resolve => setTimeout(resolve, 25))
  }
  assert.fail('等待后台状态超时')
}

test('真实 stdio RPC 完成三模式、幂等创建和持久化事件', { timeout: 60_000 }, async () => {
  const dataDir = await mkdtemp(join(tmpdir(), 'tcview-rpc-'))
  let client = connect(dataDir)
  try {
    const hello = await client.call('hello')
    assert.equal(hello.protocolVersion, '1.4')
    assert.equal(hello.providerMode, 'mock')
    await assert.rejects(client.call('unknown.method'))
    const ids = []
    for (const mode of ['free_debate', 'adjudication', 'delphi']) {
      const input = { goal: `验证 ${mode} 的桌面 mock 讨论与恢复`, mode, idempotencyKey: randomUUID() }
      const first = await client.call('run.start', input)
      const duplicate = await client.call('run.start', input)
      assert.equal(first.id, duplicate.id)
      ids.push(first.id)
      const done = await until(() => client.call('run.get', { runId: first.id }), value => value.status === 'completed' || value.status === 'failed')
      assert.equal(done.status, 'completed', done.error)
      assert.equal(done.decision.mock, true)
      assert.ok(done.messages.length >= 4)
      assert.ok(done.proposal.hash)
      const events = await client.call('run.events', { runId: done.id, afterEventId: 0 })
      assert.ok(events.length > 5)
      for (let i = 1; i < events.length; i++) assert.ok(events[i].eventId > events[i - 1].eventId)
      const last = events.at(-1).eventId
      assert.deepEqual(await client.call('run.events', { runId: done.id, afterEventId: last }), [])
    }
    const before = await client.call('run.list')
    assert.equal(before.length, 3)
    await client.close()
    client = connect(dataDir)
    const after = await client.call('run.list')
    assert.deepEqual(new Set(after.map(run => run.id)), new Set(ids))
    for (const id of ids) {
      const snapshot = await client.call('run.get', { runId: id })
      assert.equal(snapshot.status, 'completed')
      assert.ok(snapshot.messages.length > 0)
    }
  } finally {
    await client.close()
    await rm(dataDir, { recursive: true, force: true })
  }
})

test('CLI 选择、动态角色和可恢复删除跨 RPC 与重启持久化（显式 Mock 模型）', { timeout: 45_000 }, async () => {
  const dataDir = await mkdtemp(join(tmpdir(), 'tcview-selection-history-'))
  let client = connect(dataDir)
  try {
    const settings = await client.call('provider.settings.get')
    assert.equal(settings.length, 4)
    assert.ok(settings.every(item => item.enabled !== false))
    const configs = settings.map(item => ({ id: item.id, model: item.effective.model, reasoning: item.effective.reasoning, enabled: item.id !== 'codex' }))
    const saved = await client.call('provider.settings.save', { configs })
    assert.equal(saved.find(item => item.id === 'codex').enabled, false)
    assert.equal(saved.find(item => item.id === 'codex').sources.enabled.kind, 'saved')
    const input = { goal: '验证停用 Codex 后讨论并恢复删除历史', mode: 'free_debate', idempotencyKey: randomUUID() }
    const started = await client.call('run.start', input)
    assert.deepEqual(started.rolePlan, { proposer: 'claude', reviewers: ['grok', 'agy'] })
    assert.deepEqual(started.providers.map(item => item.id), ['claude', 'grok', 'agy'])
    const done = await until(() => client.call('run.get', { runId: started.id }), item => ['completed', 'failed'].includes(item.status))
    assert.equal(done.status, 'completed', done.error)
    const deleted = await until(async () => {
      try { return await client.call('run.delete', { runId: done.id }) }
      catch (error) { if (/等待 CLI 完全退出/.test(error.message)) return null; throw error }
    }, value => !!value?.deletedAt)
    assert.ok(deleted.deletedAt)
    assert.deepEqual(await client.call('run.list'), [])
    assert.equal((await client.call('run.trash'))[0].id, done.id)
    await assert.rejects(client.call('run.start', input), /恢复|删除/)
    const exported = JSON.parse((await client.call('artifact.export', { runId: done.id, format: 'json' })).content)
    assert.equal(exported.run.proposal.hash, done.proposal.hash)
    await client.close()
    client = connect(dataDir)
    assert.deepEqual(await client.call('run.list'), [])
    assert.equal((await client.call('run.trash'))[0].id, done.id)
    const restored = await client.call('run.restore', { runId: done.id })
    assert.equal(restored.deletedAt, undefined)
    assert.deepEqual(restored.rolePlan, started.rolePlan)
    assert.equal(restored.proposal.hash, done.proposal.hash)
    assert.equal(restored.chainCalls, done.chainCalls)
    assert.equal((await client.call('run.list'))[0].id, done.id)
    assert.deepEqual(await client.call('run.trash'), [])
    assert.equal((await client.call('provider.settings.get')).find(item => item.id === 'codex').enabled, false)
  } finally { await client.close(); await rm(dataDir, { recursive: true, force: true }) }
})

for (const mode of ['free_debate', 'adjudication', 'delphi']) test(`${mode} 后台异常终止后恢复同一讨论（显式 Mock 模型）`, { timeout: 45_000 }, async () => {
  const dataDir = await mkdtemp(join(tmpdir(), 'tcview-crash-'))
  let client = connect(dataDir, 100)
  try {
    const first = await client.call('run.start', { goal: `验证 ${mode} 后台意外退出后的恢复能力`, mode, idempotencyKey: randomUUID() })
    const checkpoints = mode === 'free_debate' ? ['debate_initial_complete', 'debate_round_complete'] : mode === 'delphi' ? ['delphi_round_complete', 'delphi_rounds_complete'] : ['propose', 'challenge', 'verify', 'revise', 'adjudicate']
    const ready = await until(() => client.call('run.get', { runId: first.id }), value => checkpoints.includes(value.checkpoint) && Boolean(value.proposal))
    assert.equal(ready.status, 'running')
    const committedInvocations = ready.messages.filter(message => message.invocationId && !message.streaming).map(message => message.invocationId)
    await client.crash()
    client = connect(dataDir)
    const recovered = await client.call('run.get', { runId: first.id })
    assert.equal(recovered.status, 'interrupted')
    assert.equal(recovered.proposal.hash, ready.proposal.hash)
    const resumeInput = { runId: first.id, idempotencyKey: randomUUID() }
    const resumed = await client.call('run.resume', resumeInput)
    const duplicate = await client.call('run.resume', resumeInput)
    assert.equal(duplicate.attemptId, resumed.attemptId)
    assert.notEqual(resumed.attemptId, ready.attemptId)
    const done = await until(() => client.call('run.get', { runId: first.id }), value => value.status === 'completed' || value.status === 'failed')
    assert.equal(done.status, 'completed', done.error)
    assert.equal(done.sessionId, ready.sessionId)
    assert.equal(done.messages.filter(message => message.kind === 'proposal' && message.phase === 'preparing').length, 1)
    for (const invocationId of committedInvocations) assert.equal(done.messages.filter(message => message.invocationId === invocationId).length, 1)
  } finally {
    await client.close()
    await rm(dataDir, { recursive: true, force: true })
  }
})

test('停止及后台重启后继续同一运行，保留 P0 与 attempt 历史', { timeout: 60_000 }, async () => {
  const dataDir = await mkdtemp(join(tmpdir(), 'tcview-resume-'))
  let client = connect(dataDir, 120)
  try {
    const first = await client.call('run.start', { goal: '验证长讨论的停止和检查点恢复', mode: 'free_debate', idempotencyKey: randomUUID() })
    const ready = await until(() => client.call('run.get', { runId: first.id }), value => Boolean(value.proposal) && Boolean(value.checkpoint))
    await client.call('run.stop', { runId: first.id })
    const stopped = await until(() => client.call('run.get', { runId: first.id }), value => ['stopped', 'interrupted'].includes(value.status))
    assert.ok(['stopped', 'interrupted'].includes(stopped.status))
    assert.ok(stopped.checkpoint)
    const oldAttempt = stopped.attemptId
    await client.close()
    client = connect(dataDir, 12)
    const saved = await client.call('run.get', { runId: first.id })
    assert.equal(saved.proposal.hash, ready.proposal.hash)
    const resumed = await client.call('run.resume', { runId: first.id, idempotencyKey: randomUUID() })
    assert.notEqual(resumed.attemptId, oldAttempt)
    const done = await until(() => client.call('run.get', { runId: first.id }), value => value.status === 'completed' || value.status === 'failed')
    assert.equal(done.status, 'completed', done.error)
    assert.equal(done.id, first.id)
    const p0 = ready.messages.find(message => message.kind === 'proposal' && message.phase === 'preparing')
    assert.ok(p0)
    assert.equal(done.messages.filter(message => message.id === p0.id).length, 1)
    assert.equal(done.messages.filter(message => message.kind === 'proposal' && message.phase === 'preparing').length, 1)
    assert.equal(done.sessionId, stopped.sessionId)
  } finally {
    await client.close()
    await rm(dataDir, { recursive: true, force: true })
  }
})

test('自动模式复用准备记录并冻结材料，拒绝变更输入及重复消费（显式 Mock 模型）', { timeout: 60_000 }, async () => {
  const dataDir = await mkdtemp(join(tmpdir(), 'tcview-preparation-'))
  let client = connect(dataDir, 15)
  try {
    const material = { id:'source-one',title:'验收要求.md',source:'pasted',content:'范围：单用户 macOS。\r\n必须保存全部版本，并为新版本重新投票。\n' }
    const input = { id:randomUUID(),goal:'比较适合单用户桌面讨论工具的存储方案，按恢复与部署成本取舍',providerId:'agy',materials:[material] }
    const prepared = await client.call('topic.analyze',input)
    assert.equal(prepared.status,'completed')
    assert.equal(prepared.classification.recommendation,'free_debate')
    assert.equal(prepared.source.mock,true)
    assert.equal(prepared.materials[0].content,material.content)
    assert.match(prepared.materials[0].sha256,/^[a-f0-9]{64}$/)
    assert.equal(prepared.materials[0].normalization,'none-utf8')
    assert.equal((await client.call('topic.analyze',input)).id,prepared.id)
    await assert.rejects(client.call('topic.analyze',{...input,goal:'另一份目标，不能借用旧 id'}))
    await assert.rejects(client.call('run.start',{goal:input.goal+' 改变约束',mode:'auto',preparationId:prepared.id,materials:[material],idempotencyKey:randomUUID()}))
    const startInput={goal:input.goal,mode:'auto',preparationId:prepared.id,materials:[material],idempotencyKey:randomUUID()}
    const started=await client.call('run.start',startInput)
    assert.equal((await client.call('run.start',startInput)).id,started.id)
    const done=await until(()=>client.call('run.get',{runId:started.id}),value=>['completed','failed'].includes(value.status))
    assert.equal(done.status,'completed',done.error)
    assert.equal(done.mode,'free_debate')
    assert.equal(done.requestedMode,'auto')
    assert.equal(done.materials[0].content,material.content)
    assert.equal(done.preparation.id,prepared.id)
    assert.ok(done.usage.calls>1&&done.usage.calls<=30)
    assert.ok(new Date(done.deadlineAt).getTime()<=new Date(prepared.createdAt).getTime()+15*60_000+1)
    await assert.rejects(client.call('run.start',{...startInput,idempotencyKey:randomUUID()}))
    const artifact=JSON.parse((await client.call('artifact.export',{runId:done.id,format:'json'})).content)
    assert.equal(artifact.run.materials[0].sha256,prepared.materials[0].sha256)
    assert.ok(artifact.frozenConfig.materialsHash)
    assert.ok(artifact.frozenConfig.preparationHash)
    await client.close()
    client=connect(dataDir)
    const saved=await client.call('topic.get',{id:prepared.id})
    assert.equal(saved.usedByRunId,done.id)
    assert.equal(saved.materials[0].content,material.content)
  }finally{await client.close();await rm(dataDir,{recursive:true,force:true})}
})

test('取消准备分析不阻塞其他 RPC，也不自动创建讨论（显式 Mock 模型）', { timeout: 20_000 }, async () => {
  const dataDir=await mkdtemp(join(tmpdir(),'tcview-preparation-cancel-'))
  const client=connect(dataDir,1000)
  try{
    const id=randomUUID()
    const pending=client.call('topic.analyze',{id,goal:'验证可取消的自动模式检查',providerId:'agy',materials:[]}).then(value=>({value}),error=>({error}))
    await until(async()=>{try{return await client.call('topic.get',{id})}catch{return {status:'unregistered'}}},value=>value.status==='running')
    assert.deepEqual(await client.call('run.list'),[])
    await client.call('topic.cancel',{id})
    await pending
    assert.equal((await client.call('topic.get',{id})).status,'cancelled')
    assert.deepEqual(await client.call('run.list'),[])
  }finally{await client.close();await rm(dataDir,{recursive:true,force:true})}
})

