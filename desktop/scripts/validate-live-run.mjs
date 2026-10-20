// 对保存的真实验收证据离线检查，不发起模型请求。
import { readFile, readdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { createHash } from 'node:crypto'

if(!process.argv[2])throw new Error('用法：node scripts/validate-live-run.mjs <验收目录>')
const directory=resolve(process.argv[2])
const snapshot=JSON.parse(await readFile(join(directory,'latest-snapshot.json'),'utf8'))
const exported=JSON.parse(await readFile(join(directory,'discussion.json'),'utf8'))
const errors=[]
const check=(condition,message)=>{if(!condition)errors.push(message)}
check(snapshot.providerMode==='cli'&&exported.run.providerMode==='cli','来源必须是真实 CLI')
check(snapshot.status==='completed','完整讨论必须结束')
check(snapshot.id===exported.run.id,'导出和快照讨论必须一致')
check(snapshot.proposal?.hash===snapshot.decision?.proposalHash,'最终票必须绑定当前方案')
for(const proposal of snapshot.proposalHistory??[])check(createHash('sha256').update(proposal.content).digest('hex')===proposal.hash,`方案 P${proposal.version} 哈希不符`)
check((snapshot.proposalHistory?.length??0)>=2,'必须保留初稿和综合版本')
const ballots=(snapshot.ballots??[]).filter(ballot=>ballot.valid&&ballot.proposalHash===snapshot.proposal?.hash&&ballot.manifestHash===snapshot.decision?.manifestHash)
const voters=new Set(ballots.map(ballot=>ballot.agentId))
check(['claude','grok','agy'].every(id=>voters.has(id))&&voters.size===3,'必须有三位评审的当前版本有效票')
const models=['claude','grok','agy'].map(id=>exported.modelBindings?.[id]?.model)
check(models.every(Boolean)&&new Set(models).size===3,'必须确认三个独立实际模型')
const outputs=[]
async function visit(path){
  for(const entry of await readdir(path,{withFileTypes:true})){
    const child=join(path,entry.name)
    if(entry.isDirectory())await visit(child)
    else if(entry.name.endsWith('.output.json'))outputs.push(JSON.parse(await readFile(child,'utf8')))
  }
}
await visit(join(directory,'host','runs',snapshot.id,'artifacts'))
check(outputs.filter(output=>output.taskKind==='propose'&&output.status==='completed').length===1,'已提交 P0 不得重新执行')
check(outputs.filter(output=>output.taskKind==='initial_proposal'&&output.status==='completed').length===3,'已提交独立评审不得重新执行')
let stopResume
try{stopResume=JSON.parse(await readFile(join(directory,'stop-resume.json'),'utf8'))}catch{}
if(stopResume){
  check(stopResume.before.cliChildren.length>0,'停止前必须存在实际 CLI 进程')
  check(stopResume.remainingChildren.length===0&&stopResume.survivingPids.length===0,'停止后不得遗留原有 CLI 进程')
  for(const id of stopResume.before.completeInvocations)check(snapshot.messages.filter(message=>message.invocationId===id).length===1,`已提交 invocation 未被唯一保留：${id}`)
}
const result={runId:snapshot.id,passed:errors.length===0,errors,decision:snapshot.decision?.outcome,validBallots:ballots.length,models,calls:snapshot.usage?.calls,completedInvocations:outputs.filter(output=>output.status==='completed').length,cancelledInvocations:outputs.filter(output=>output.status==='cancelled').length,stopLatencyMs:stopResume?.stopLatencyMs,engineVersion:exported.frozenConfig?.engineVersion}
await writeFile(join(directory,'validation.json'),JSON.stringify(result,null,2),{mode:0o600})
console.log(JSON.stringify(result,null,2))
if(errors.length)process.exitCode=1
