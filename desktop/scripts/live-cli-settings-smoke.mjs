// 显式运行才使用真实 CLI 额度：每个指定 provider 最多一个请求，无重试。
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import { createInterface } from 'node:readline'
import { join } from 'node:path'
import { root, engineBuildVersion } from './go-env.mjs'

const allProviders = ['codex', 'claude', 'grok', 'agy']
const option = process.argv.slice(2).find(value => value.startsWith('--providers='))
const selected = option ? option.slice('--providers='.length).split(',') : allProviders
const catalogsOnly = process.argv.includes('--catalogs-only')
assert.ok(selected.length && selected.every(id => allProviders.includes(id)) && new Set(selected).size === selected.length)
const directory = join(root, '.tcview', 'cli-settings-validation', new Date().toISOString().replaceAll(':', '-'))
await mkdir(directory, {recursive: true, mode: 0o700})
const expectedEngineVersion = engineBuildVersion()
const environment = {...process.env}
// 此验收仅覆盖独立数据目录里的保存设置，不继承公共模型覆盖；认证仍由 CLI 管理。
for (const id of allProviders) for (const field of ['MODEL','REASONING']) delete environment[`TCVIEW_${id.toUpperCase()}_${field}`]
let client
function connect() {
  const child = spawn(join(root, 'dist/bin/tcview-host'), ['--data-dir', join(directory, 'host'), '--provider-mode', 'cli'], {env: environment, stdio: ['pipe','pipe','pipe']})
  let sequence = 0, closed = false
  const pending = new Map()
  const exit = new Promise(resolve => child.once('close', resolve))
  child.stdin.on('error', () => {})
  child.stderr.resume()
  createInterface({input: child.stdout}).on('line', line => {
    let value
    try { value = JSON.parse(line) } catch { return }
    const request = pending.get(value.id)
    if (!request) return
    pending.delete(value.id); clearTimeout(request.timer)
    value.error ? request.reject(new Error(value.error.message)) : request.resolve(value.result)
  })
  return {
    call(method, params = {}) {
      const id = ++sequence
      return new Promise((resolve,reject) => {
        const timer = setTimeout(() => {pending.delete(id); reject(new Error(`${method} 超时`))}, method === 'provider.models' ? 90_000 : method === 'provider.check' ? 150_000 : 30_000)
        pending.set(id,{resolve,reject,timer})
        child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n')
      })
    },
    async close() {
      if (closed) return
      closed = true; child.stdin.end()
      const timer = setTimeout(() => child.kill('SIGKILL'), 6000)
      await exit; clearTimeout(timer)
      for (const value of pending.values()) clearTimeout(value.timer)
    },
  }
}
const summary = {directory, providerMode:'cli', selected, catalogsOnly, startedAt:new Date().toISOString(), catalogErrors:[], results:[], errors:[], maxModelRequests:catalogsOnly ? 0 : selected.length}
try {
  client = connect()
  summary.hello = await client.call('hello')
  assert.equal(summary.hello.protocolVersion,'1.4')
  assert.equal(summary.hello.providerMode,'cli')
  assert.equal(summary.hello.engineVersion, expectedEngineVersion)
  summary.defaults = await client.call('provider.settings.get')
  assert.equal(summary.defaults.length,4)
  const catalogs = await Promise.all(selected.map(async providerId => {
    const catalog = await client.call('provider.models',{providerId})
    await writeFile(join(directory,`${providerId}-catalog.json`),JSON.stringify(catalog,null,2),{mode:0o600})
    if (catalog.error) summary.catalogErrors.push({providerId,error:catalog.error})
    return catalog
  }))
  summary.catalogCounts = catalogs.map(catalog => ({providerId:catalog.id,count:catalog.models.length,source:catalog.source,error:catalog.error}))
  if (catalogsOnly) {
    for (const catalog of catalogs) if (catalog.error || !catalog.models.length) summary.errors.push(`${catalog.id}: ${catalog.error || '目录为空'}`)
  } else {
  const configurations = [
    {id:'codex',model:'gpt-6.1-sol',reasoning:selected.includes('codex')?'low':''},
    {id:'claude',model:'claude-opus-5-5',reasoning:selected.includes('claude')?'low':''},
    {id:'grok',model:'grok-4.7',reasoning:'low'},
    {id:'agy',model:selected.includes('agy')?'gemini-3.8-flash-low':'gemini-3.8-flash-medium',reasoning:''},
  ]
  summary.saved = await client.call('provider.settings.save',{configs:configurations})
  for (const config of configurations) {
    const view = summary.saved.find(item => item.id === config.id)
    assert.equal(view.saved.model,config.model)
    assert.equal(view.effective.model,config.model)
    assert.equal(view.effective.reasoning ?? '',config.reasoning)
    assert.equal(view.sources.model.kind,'saved')
  }
  // 保存、后台重启后读取不应发出任何模型请求。
  await client.close(); client = connect()
  summary.reloaded = await client.call('provider.settings.get')
  for (const config of configurations) {
    const view = summary.reloaded.find(item => item.id === config.id)
    assert.equal(view.effective.model,config.model)
    assert.equal(view.effective.reasoning ?? '',config.reasoning)
  }
  const rejected = await client.call('provider.check',{providerId:'agy',config:{id:'agy',model:'gemini-3.8-flash-medium',reasoning:'low'}})
  assert.equal(rejected.requestOK,false)
  assert.equal(rejected.requestAttempted,false)
  assert.ok(rejected.error)
  summary.invalidCombination = rejected
  for (const providerId of selected) {
    const config = configurations.find(value => value.id === providerId)
    const result = await client.call('provider.check',{providerId})
    await writeFile(join(directory,`${providerId}-check.json`),JSON.stringify(result,null,2),{mode:0o600})
    summary.results.push(result)
    if (!result.requestOK) summary.errors.push(`${providerId}: ${result.error || '真实请求未通过'}`)
    else {
      assert.equal(result.requestAttempted,true)
      assert.equal(result.checkedConfiguration,'effective')
      assert.equal(result.requestedModel,config.model)
      assert.equal(result.requestedReasoning ?? '',config.reasoning)
      assert.equal(result.effective.model,config.model,'canonical 不得回写 selector')
      assert.ok(result.reportedModel,'实际模型必须来自原生报告')
      assert.equal(result.modelSource,'cli-report')
      if (providerId === 'codex') assert.equal(result.reportedReasoning,'low')
    }
    console.log(JSON.stringify({directory,providerId,requestOK:result.requestOK,requestedModel:result.requestedModel,reportedModel:result.reportedModel,requestedReasoning:result.requestedReasoning,reportedReasoning:result.reportedReasoning,latencyMs:result.latencyMs,error:result.error}))
  }
  assert.deepEqual(await client.call('run.list'),[],'连接验收不创建整场讨论')
  }
} catch (error) {
  summary.errors.push(error.message)
} finally {
  if (client) await client.close()
  summary.finishedAt = new Date().toISOString()
  summary.success = !summary.errors.length && (catalogsOnly || summary.results.length === selected.length)
  await writeFile(join(directory,'summary.json'),JSON.stringify(summary,null,2),{mode:0o600})
  console.log(JSON.stringify({directory,success:summary.success,completed:summary.results.length,catalogErrors:summary.catalogErrors,errors:summary.errors}))
  if (!summary.success) process.exitCode = 1
}
