// 不发起模型调用；验证安装包、隔离 PATH 下的 CLI 探测和后台重连。
import { _electron as electron } from '@playwright/test'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { root } from './go-env.mjs'

const metadata=JSON.parse(await readFile(join(root,'package.json'),'utf8'))
const productName=metadata.productName
const appPath=join(root,'release',`${productName}-darwin-${process.arch}`,`${productName}.app`)
const binary=join(appPath,`Contents/MacOS/${productName}`)
const bundlePlist=await readFile(join(appPath,'Contents/Info.plist'),'utf8')
if(!/<key>CFBundleIdentifier<\/key>\s*<string>local\.tcview\.desktop<\/string>/.test(bundlePlist))throw new Error('安装包 bundle ID 不是 local.tcview.desktop')
const buildInfo=JSON.parse(await readFile(join(appPath,'Contents/Resources/app/build-info.json'),'utf8'))
if(buildInfo.appBundleId!=='local.tcview.desktop'||buildInfo.hostBinary!=='tcview-host')throw new Error('安装包构建身份未统一为 TCView')
const hostBinaries=await readdir(join(appPath,'Contents/Resources/app/dist/bin'))
if(hostBinaries.length!==1||hostBinaries[0]!=='tcview-host')throw new Error('安装包应仅包含 tcview-host 后台二进制')
const directory=join(root,'.tcview','package-validation',new Date().toISOString().replaceAll(':','-'))
await mkdir(directory,{recursive:true,mode:0o700})
const env={...process.env,PATH:'/usr/bin:/bin',TCVIEW_DATA_DIR:join(directory,'host'),TCVIEW_USER_DATA_DIR:join(directory,'electron'),TCVIEW_PROVIDER_MODE:'cli'}
delete env.ELECTRON_RUN_AS_NODE
const app=await electron.launch({executablePath:binary,env})
try {
  const page=await app.firstWindow()
  await page.getByTestId('connection-status').filter({hasText:'已连接'}).waitFor()
  const errors=[]
  page.on('pageerror',error=>errors.push(error.message))
  const hello=await page.evaluate(()=>window.tcview.hello())
  if(hello.providerMode!=='cli'||hello.protocolVersion!=='1.4')throw new Error('安装包后台协议或默认模式错误')
  const identity=await app.evaluate(({app,BrowserWindow})=>({name:app.getName(),userData:app.getPath('userData'),sessionData:app.getPath('sessionData'),titles:BrowserWindow.getAllWindows().map(window=>window.getTitle())}))
  if(identity.name!==metadata.productName||identity.titles.length!==1||!identity.titles[0].startsWith(metadata.productName))throw new Error('安装包应用名称或窗口标题未同步')
  if(identity.userData!==env.TCVIEW_USER_DATA_DIR||identity.sessionData!==env.TCVIEW_USER_DATA_DIR)throw new Error('安装包未使用指定的 TCView 用户数据和浏览器存储目录')
  const providers=await page.evaluate(()=>window.tcview.listProviders())
  if(providers.length!==4||providers.some(provider=>!provider.available))throw new Error(`隔离 PATH 下 CLI 探测失败：${JSON.stringify(providers)}`)
  const reconnected=await page.evaluate(()=>window.tcview.reconnectHost())
  if(reconnected.engineVersion!==hello.engineVersion)throw new Error('重连更换了内核版本')
  await page.getByTestId('provider-settings-button').click()
  await page.getByRole('dialog').waitFor()
  await page.getByRole('dialog').getByText('Grok Build',{exact:true}).waitFor()
  await page.getByTestId('provider-config-values-grok').getByText('low',{exact:true}).first().waitFor()
  await page.screenshot({path:join(directory,'desktop.png'),fullPage:true})
  if(errors.length)throw new Error(errors.join('\n'))
  const result={binary,directory,buildInfo,hostBinaries,identity,hello,providers,reconnected:true,rendererErrors:errors}
  await writeFile(join(directory,'summary.json'),JSON.stringify(result,null,2),{mode:0o600})
  console.log(JSON.stringify(result,null,2))
}finally {await app.close()}
