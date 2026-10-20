// 显式运行时才发起真实四方讨论，不属于默认 CI。
import { _electron as electron } from 'playwright'
import { mkdir, writeFile, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { root } from './go-env.mjs'

const batch=new Date().toISOString().replaceAll(':','-').replaceAll('.','-')
const directory=join(root,'.tcview','live-validation',`discussion-${batch}`)
await mkdir(directory,{recursive:true,mode:0o700})
const environment=Object.fromEntries(Object.entries(process.env).filter(([,v])=>v!==undefined))
delete environment.ELECTRON_RUN_AS_NODE
Object.assign(environment,{TCVIEW_DATA_DIR:join(directory,'host'),TCVIEW_USER_DATA_DIR:join(directory,'electron'),TCVIEW_PROVIDER_MODE:'cli'})
const app=await electron.launch({args:[root],env:environment})
async function selectDropdown(page, testId, value) {
  const control = page.getByTestId(testId)
  await control.click()
  const options = page.locator('.carbon-select-menu').getByRole('option')
  for (const option of await options.all()) {
    if (await option.getAttribute('data-value') === value) {
      await option.click()
      if (await control.getAttribute('data-value') !== value) throw new Error(`下拉框 ${testId} 未选择预期值`)
      return
    }
  }
  throw new Error(`下拉框 ${testId} 中没有选项 ${value}`)
}
function cliChildren(){
  const rows=execFileSync('/bin/ps',['-axo','pid=,ppid=,command='],{encoding:'utf8'}).trim().split('\n').map(line=>{
    const match=line.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/)
    return match?{pid:Number(match[1]),parent:Number(match[2]),command:match[3]}:null
  }).filter(Boolean)
  const host=rows.find(row=>row.command.startsWith(join(root,'dist/bin/tcview-host')+' --data-dir '+join(directory,'host')+' '))
  if(!host)throw new Error('未找到本次验收的独立后台')
  const found=new Set([host.pid])
  let changed=true
  while(changed){changed=false;for(const row of rows)if(found.has(row.parent)&&!found.has(row.pid)){found.add(row.pid);changed=true}}
  return rows.filter(row=>row.pid!==host.pid&&found.has(row.pid)).map(row=>({pid:row.pid,parent:row.parent}))
}
let final
try {
  const page=await app.firstWindow()
  await page.waitForFunction(()=>window.tcview!==undefined)
  const hello=await page.evaluate(()=>window.tcview.hello())
  if(hello.providerMode!=='cli')throw new Error('真实验收不得运行Mock')
  const providers=await page.evaluate(()=>window.tcview.listProviders())
  await writeFile(join(directory,'providers.json'),JSON.stringify(providers,null,2),{mode:0o600})
  if(providers.some(p=>!p.available))throw new Error('有CLI不可用，不能开始四方验收')
  const metadata=JSON.parse(await readFile(join(root,'package.json'),'utf8'))
  const goal=`审议 ${metadata.productName} ${metadata.version} 首版的一条产品规则：缺少任一必需 CLI 时阻止新的四方讨论，指出缺失项并引导用户修复，不自动切换 Mock。真实项目输入：Electron ${metadata.devDependencies.electron}、React ${metadata.dependencies.react}，Go 后台，四方为 Codex/Claude/Grok/agy；默认真实 CLI，Mock 只供显式测试。范围为本地单用户 macOS。请独立评审这条规则的可用性与可信度，允许提出异议，不要求达成共识。每次只输出一条紧凑主张，各 summary 和 rationale 不超过80字；若已有主张覆盖意见则直接评价，不重复新增主张。不得声称执行了测试，不调用工具、不读文件、不改代码。保持所有哈希、引用和任务协议字段完整。`
  await selectDropdown(page, 'mode-select', 'free_debate')
  await page.getByTestId('goal-input').fill(goal)
  await page.getByTestId('send-button').click()
  let last='',selected,stopResume
  const started=Date.now()
  while(Date.now()-started<16*60_000) {
    const runs=await page.evaluate(()=>window.tcview.listRuns())
    selected=runs[0]
    if(selected){
      final=await page.evaluate(id=>window.tcview.getRun(id),selected.id)
      await writeFile(join(directory,'latest-snapshot.json'),JSON.stringify(final,null,2),{mode:0o600})
      const key=`${final.status}/${final.phase}/calls=${final.usage?.calls??0}/messages=${final.messages.length}`
      if(key!==last){last=key;console.log(JSON.stringify({directory,runId:final.id,progress:key,error:final.error}))}
      if(process.env.TCVIEW_LIVE_STOP_RESUME==='1'&&!stopResume&&final.phase==='debate'&&(final.usage?.calls??0)>=7&&final.checkpoint!=='ingest'){
        const before={checkpoint:final.checkpoint,calls:final.usage?.calls,cliChildren:cliChildren(),completeInvocations:final.messages.filter(m=>m.invocationId&&!m.streaming).map(m=>m.invocationId)}
        const stopping=Date.now()
        await page.evaluate(id=>window.tcview.stopRun(id),final.id)
        let stopped
        do{
          await new Promise(resolve=>setTimeout(resolve,100))
          stopped=await page.evaluate(id=>window.tcview.getRun(id),final.id)
        }while(stopped.status==='running'&&Date.now()-stopping<10_000)
        if(stopped.status!=='stopped')throw new Error(`真实讨论未及时停止：${stopped.status}`)
        const stopLatencyMs=Date.now()-stopping,remainingChildren=cliChildren()
        const survivingPids=before.cliChildren.filter(({pid})=>{try{process.kill(pid,0);return true}catch{return false}}).map(({pid})=>pid)
        if(remainingChildren.length||survivingPids.length)throw new Error(`停止后仍存在 CLI 进程：${JSON.stringify({remainingChildren,survivingPids})}`)
        const resumed=await page.evaluate(id=>window.tcview.resumeRun({runId:id,idempotencyKey:`live-resume-${id}`}),final.id)
        stopResume={before,stoppedAt:stopped.updatedAt,stopLatencyMs,remainingChildren,survivingPids,resumedAttemptId:resumed.attemptId}
        await writeFile(join(directory,'stop-resume.json'),JSON.stringify(stopResume,null,2),{mode:0o600})
      }
      if(['completed','failed','stopped','interrupted'].includes(final.status))break
    }
    await new Promise(resolve=>setTimeout(resolve,1200))
  }
  if(!final || !['completed','failed'].includes(final.status))throw new Error('真实讨论未在期限内完成')
  if(process.env.TCVIEW_LIVE_STOP_RESUME==='1'&&!stopResume)throw new Error('未执行真实停止恢复验收')
  await page.screenshot({path:join(directory,'desktop.png'),fullPage:true})
  await app.evaluate(({dialog},path)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath:path})},join(directory,'discussion.md'))
  await page.evaluate(id=>window.tcview.exportArtifact(id,'markdown'),final.id)
  await app.evaluate(({dialog},path)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath:path})},join(directory,'discussion.json'))
  await page.evaluate(id=>window.tcview.exportArtifact(id,'json'),final.id)
  const validationErrors=[]
  const validBallots=(final.ballots??[]).filter(ballot=>ballot.valid&&ballot.proposalHash===final.proposal?.hash&&ballot.manifestHash===final.decision?.manifestHash)
  if(new Set(validBallots.map(ballot=>ballot.agentId)).size!==3)validationErrors.push('未获得当前冻结版本的三张有效票')
  if((final.proposalHistory?.length??0)<2)validationErrors.push('缺少综合方案版本')
  if(stopResume){
    if(!stopResume.before.cliChildren.length)validationErrors.push('停止前未观测到活跃 CLI 子进程')
    for(const id of stopResume.before.completeInvocations)if(final.messages.filter(message=>message.invocationId===id).length!==1)validationErrors.push(`已提交消息未被唯一保留：${id}`)
  }
  const summary={directory,runId:final.id,status:final.status,error:final.error,decision:final.decision?.outcome,coreOutcome:final.decision?.coreOutcome,providerMode:final.providerMode,calls:final.usage?.calls,ballots:final.ballots,models:final.messages.filter(m=>m.model).map(m=>({agent:m.agentId,model:m.model})),messageCount:final.messages.length,proposalVersions:final.proposalHistory?.length,stopResume,validationErrors}
  await writeFile(join(directory,'summary.json'),JSON.stringify(summary,null,2),{mode:0o600})
  console.log(JSON.stringify(summary,null,2))
  if(final.status!=='completed'||validationErrors.length)process.exitCode=1
}finally{await app.close()}
