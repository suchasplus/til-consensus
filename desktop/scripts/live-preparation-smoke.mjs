// 显式运行才发出真实分析请求；不启动完整多方讨论，不属于默认 CI。
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { createHash, randomUUID } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createInterface } from 'node:readline'
import { basename, join } from 'node:path'
import { root } from './go-env.mjs'

const directory = join(root, '.tcview', 'preparation-validation', new Date().toISOString().replaceAll(':', '-'))
await mkdir(directory, { recursive: true, mode: 0o700 })
const hash = value => createHash('sha256').update(value).digest('hex')
async function material(path, id) {
  const bytes = await readFile(join(root, path))
  return { id, title: basename(path), content: bytes.toString('utf8'), source: 'file', rawSha256: hash(bytes) }
}
const project = await material('package.json', 'package')
const module = await material('services/host/go.mod', 'go-module')
const lock = await material('scripts/engine-lock.json', 'engine-lock')
const cases = [
  {
    name: '开放取舍与真实项目材料', expected: 'free_debate', materials: [project],
    goal: '为附件中的本地单用户 macOS 桌面应用比较两种会话持久化设计：SQLite 与一个追加写 JSON 日志。目标是在断电恢复、开发复杂度和部署成本之间取舍；不需要代码实施。硬约束为离线可读、完整保留方案版本与投票、恢复不重复已提交阶段，不引入外部服务。请让多个参与者提出不同立场并投票选择方案。附件是实际项目清单，仅代表当前依赖，未证明任何方案性能。',
  },
  {
    name: '可核查主张与真实模块材料', expected: 'adjudication', materials: [module, lock],
    goal: '请对这个具体主张做逐项裁决：“所附 go.mod 的 til-consensus require 版本与 engine-lock.json 的 moduleVersion 一致，因此默认依赖声明固定同一模块版本。”双方应从附件原文提出支持与反驳，由裁决者判断该主张是否成立。判断范围只限这两份文件的声明一致性；不能据此声称代码已编译、远端可下载或运行正确，不需要讨论其他存储方案，也不执行命令。成功标准是明确裁决、逐字证据和边界。',
  },
  {
    name: '匿名评分与优先级', expected: 'delphi', materials: [project],
    goal: '为所附依赖清单中的本地 macOS 多模型讨论工具做专家匿名多轮优先级评分。候选项固定为 A 导出可审计结果、B 断线恢复、C 支持 PDF；本期只选一项，工期两周、单人开发，不部署服务器。每位专家独立按可靠性提升50%、实现成本30%、日常使用价值20%打1至5分，多轮看匿名评分分布后再评，并保留低一致性的分歧。目标是降低权威偏见和从众效应，不做事实真伪裁决，也不要求所有专家支持第一名。',
  },
  { name: '缺少关键上下文', expected: 'needs_clarification', materials: [], goal: '帮我设计一个系统，选出最好的方案。' },
  { name: '简单任务无需讨论', expected: 'not_suitable', materials: [], goal: '计算 2 加 2 的结果；只需这个整数，不做方案设计、优先级排序或多方讨论。' },
]
const option = process.argv.slice(2).find(value => value.startsWith('--cases='))
const selected = option ? option.slice('--cases='.length).split(',').map(Number) : cases.map((_, index) => index + 1)
if (!selected.length || selected.some(value => !Number.isInteger(value) || value < 1 || value > cases.length) || new Set(selected).size !== selected.length) throw new Error('--cases 需要为不重复的案例序号，如 --cases=2,3,4,5')
const child = spawn(join(root, 'dist/bin/tcview-host'), ['--data-dir', join(directory, 'host'), '--provider-mode', 'cli'], { stdio: ['pipe', 'pipe', 'pipe'] })
let sequence = 0, diagnostics = ''
const pending = new Map()
const exit = new Promise(resolve => child.once('close', resolve))
child.stderr.on('data', data => { diagnostics = (diagnostics + data).slice(-4000) })
child.stdin.on('error', () => {})
createInterface({ input: child.stdout }).on('line', line => {
  let value
  try { value = JSON.parse(line) } catch { return }
  const request = pending.get(value.id)
  if (!request) return
  clearTimeout(request.timer)
  pending.delete(value.id)
  value.error ? request.reject(new Error(value.error.message)) : request.resolve(value.result)
})
function call(method, params = {}) {
  const id = ++sequence
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`${method} 超时：${diagnostics}`)) }, 150_000)
    pending.set(id, { resolve, reject, timer })
    child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n')
  })
}
const summary = { directory, providerMode: 'cli', providerId: 'agy', cases: selected, startedAt: new Date().toISOString(), results: [], errors: [] }
try {
  summary.hello = await call('hello')
  assert.equal(summary.hello.providerMode, 'cli')
  assert.equal(summary.hello.protocolVersion, '1.4')
  for (const [index, sample] of cases.entries()) {
    if (!selected.includes(index + 1)) continue
    const request = { id: randomUUID(), goal: sample.goal, materials: sample.materials, providerId: 'agy' }
    await writeFile(join(directory, `${index + 1}-request.json`), JSON.stringify(request, null, 2), { mode: 0o600 })
    let record
    try { record = await call('topic.analyze', request) } catch (error) {
      record = await call('topic.get', { id: request.id }).catch(() => null)
      await writeFile(join(directory, `${index + 1}-failure.json`), JSON.stringify({ error: error.message, record }, null, 2), { mode: 0o600 })
      throw error
    }
    await writeFile(join(directory, `${index + 1}-result.json`), JSON.stringify(record, null, 2), { mode: 0o600 })
    assert.equal(record.status, 'completed')
    assert.ok(!record.source.mock)
    assert.equal(record.source.providerId, 'agy')
    assert.equal(record.source.modelSource, 'cli-report')
    assert.ok(record.source.actualModel)
    assert.equal(record.usage.calls, 1)
    assert.equal(record.materials.length, sample.materials.length)
    for (const original of sample.materials) {
      const frozen = record.materials.find(value => value.id === original.id)
      assert.equal(frozen.content, original.content)
      assert.equal(frozen.sha256, original.rawSha256)
      assert.equal(frozen.rawSha256, original.rawSha256)
      const analysis = record.materialAnalysis.materials.find(value => value.materialId === original.id)
      assert.ok(analysis)
      for (const point of analysis.keyPoints) assert.ok(original.content.includes(point.quote))
    }
    let gateError
    if (['needs_clarification', 'not_suitable'].includes(record.classification.recommendation) || record.classification.confidence < 0.6 || record.materialAnalysis.gaps.some(gap => gap.importance === 'required')) {
      try { await call('run.start', { goal: sample.goal, mode: 'auto', materials: sample.materials, preparationId: record.id, idempotencyKey: randomUUID() }) } catch (error) { gateError = error.message }
      assert.ok(gateError, '不可直接运行的分析必须阻止自动讨论')
    }
    assert.deepEqual(await call('run.list'), [], '此脚本只验收准备分析，不启动多方讨论')
    const result = { name: sample.name, expected: sample.expected, recommendation: record.classification.recommendation, confidence: record.classification.confidence, matchesExpected: record.classification.recommendation === sample.expected, preparationId: record.id, materialCount: record.materials.length, source: record.source, usage: record.usage, gateError }
    summary.results.push(result)
    if (!result.matchesExpected) summary.errors.push(`${sample.name}：模型实际选择 ${result.recommendation}，预期 ${sample.expected}`)
    console.log(JSON.stringify({ directory, ...result }))
  }
} catch (error) {
  summary.errors.push(error.message)
} finally {
  child.stdin.end()
  const timer = setTimeout(() => child.kill('SIGKILL'), 7000)
  await exit
  clearTimeout(timer)
  for (const item of pending.values()) clearTimeout(item.timer)
  summary.finishedAt = new Date().toISOString()
  summary.success = summary.errors.length === 0 && summary.results.length === selected.length
  await writeFile(join(directory, 'summary.json'), JSON.stringify(summary, null, 2), { mode: 0o600 })
  console.log(JSON.stringify({ directory, success: summary.success, completed: summary.results.length, errors: summary.errors }))
  if (!summary.success) process.exitCode = 1
}
