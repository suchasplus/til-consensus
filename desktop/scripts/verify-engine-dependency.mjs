// 显式冷缓存验收：下载真实远端模块，不调用模型。
import { cpSync, existsSync, mkdirSync, readFileSync, symlinkSync, writeFileSync } from 'node:fs'
import { execFileSync, spawnSync } from 'node:child_process'
import { join } from 'node:path'
import { root, engineCommit, engineModuleVersion, engineModule } from './go-env.mjs'

const directory=join(root,'.tcview','dependency-upgrade',new Date().toISOString().replaceAll(':','-'))
const project=join(directory,'project'),home=join(directory,'home'),cache=join(directory,'module-cache')
mkdirSync(project,{recursive:true,mode:0o700})
mkdirSync(home,{recursive:true,mode:0o700})
for(const name of ['scripts','services','apps','packages','package.json','package-lock.json','tsconfig.json','vite.config.ts']){
  if(existsSync(join(root,name)))cpSync(join(root,name),join(project,name),{recursive:true})
}
symlinkSync(join(root,'node_modules'),join(project,'node_modules'),'dir')
const workspace=join(project,'go.work')
writeFileSync(workspace,'This obsolete workspace must never be used.\n')
const environment={...process.env,HOME:home,GOMODCACHE:cache,GOPATH:join(directory,'gopath'),GOCACHE:execFileSync('go',['env','GOCACHE'],{encoding:'utf8'}).trim(),GOPROXY:'https://proxy.golang.org,direct',GOSUMDB:'sum.golang.org',GOWORK:workspace,TCVIEW_RELEASE:'1'}
delete environment.TIL_CONSENSUS_DIR
delete environment.ELECTRON_RUN_AS_NODE
console.log(`独立构建验收目录：${directory}`)
execFileSync('npm',['run','build'],{cwd:project,env:environment,stdio:'inherit'})
const goEnvironment={...environment,GOWORK:'off'}
const hostDirectory=join(project,'services/host')
const metadata=JSON.parse(execFileSync('go',['list','-m','-json',engineModule],{cwd:hostDirectory,env:goEnvironment,encoding:'utf8'}))
if(metadata.Version!==engineModuleVersion||metadata.Replace||!metadata.Dir?.startsWith(cache+'/'))throw new Error('内核未从独立远端模块缓存解析')
execFileSync('go',['mod','verify'],{cwd:hostDirectory,env:goEnvironment,stdio:'inherit'})
execFileSync('go',['build','-o',join(directory,'til-native-audit'),join(project,'scripts/cli-audit/til-native/main.go')],{cwd:hostDirectory,env:goEnvironment,stdio:'inherit'})
const host=spawnSync(join(project,'dist/bin/tcview-host'),['--data-dir',join(directory,'host-data'),'--provider-mode','cli'],{env:goEnvironment,encoding:'utf8',input:JSON.stringify({jsonrpc:'2.0',id:1,method:'hello',params:{}})+'\n',timeout:30_000})
if(host.status!==0)throw new Error(`独立后台启动失败：${host.stderr||host.error?.message}`)
const hello=host.stdout.trim().split('\n').map(line=>JSON.parse(line)).find(frame=>frame.id===1)?.result
if(hello?.engineVersion!==engineCommit||hello?.providerMode!=='cli'||hello?.protocolVersion!=='1.4')throw new Error('独立后台版本或默认模式不符')
const result={passed:true,directory,module:metadata.Path,moduleVersion:metadata.Version,moduleDirectory:metadata.Dir,moduleSum:metadata.Sum,goModSum:metadata.GoModSum,engineCommit,hello,freshModuleCache:true,localCheckoutOverride:false,releaseBuild:true,obsoleteWorkspaceIgnored:readFileSync(workspace,'utf8').startsWith('This obsolete workspace'),modelRequests:0}
writeFileSync(join(directory,'summary.json'),JSON.stringify(result,null,2)+'\n',{mode:0o600})
console.log(JSON.stringify(result,null,2))
