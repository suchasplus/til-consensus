import { packager } from '@electron/packager'
import { cp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { root, engineBuildVersion } from './go-env.mjs'

if(process.platform!=='darwin')throw new Error('当前安装包仅支持 macOS')
await import('./build.mjs')
const metadata=JSON.parse(await readFile(join(root,'package.json'),'utf8'))
const productName=metadata.productName
const appBundleId='local.tcview.desktop'
if(typeof productName!=='string'||!productName.trim())throw new Error('package.json 缺少桌面产品名称')
const stage=join(root,'.package-stage')
await mkdir(stage,{recursive:true})
await rm(join(stage,'dist'),{recursive:true,force:true})
await cp(join(root,'dist'),join(stage,'dist'),{recursive:true})
await writeFile(join(stage,'package.json'),JSON.stringify({name:metadata.name,productName,version:metadata.version,main:'dist/main/index.cjs',description:metadata.description},null,2))
await writeFile(join(stage,'build-info.json'),JSON.stringify({productName,appBundleId,hostBinary:'tcview-host',version:metadata.version,engineVersion:engineBuildVersion(),platform:process.platform,arch:process.arch,builtAt:new Date().toISOString(),distribution:'local-development'},null,2))
// 桌面应用使用独立的 TCView bundle 身份。
const paths=await packager({dir:stage,out:join(root,'release'),name:productName,appBundleId,appCategoryType:'public.app-category.developer-tools',platform:'darwin',arch:process.arch,electronVersion:metadata.devDependencies.electron,asar:false,prune:false,overwrite:true})
for(const path of paths){
  const app=join(path,`${productName}.app`)
  execFileSync('codesign',['--force','--deep','--sign','-',app],{stdio:'inherit'})
  execFileSync('codesign',['--verify','--deep','--strict',app],{stdio:'inherit'})
  console.log(`本机开发安装包：${app}`)
}
