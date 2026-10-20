import { app, BrowserWindow, ipcMain, Menu, dialog, type IpcMainInvokeEvent } from 'electron'
import { resolve, join, basename } from 'node:path'
import { open, writeFile } from 'node:fs/promises'
import { constants } from 'node:fs'
import { isUtf8 } from 'node:buffer'
import { createHash, randomUUID } from 'node:crypto'
import { pathToFileURL } from 'node:url'
import { HostClient } from './host-client'
import metadata from '../../../package.json'
import { PROTOCOL_VERSION, type HostHello, type DesktopEvent, type InputMode, type MaterialInput, type ProviderConfig } from '../../../packages/protocol/src/index'

const appName = metadata.productName
app.setName(appName)
// 使用 TCView 自身的数据目录；不读取或迁移旧品牌数据。
app.setPath('userData', process.env.TCVIEW_USER_DATA_DIR ? resolve(process.env.TCVIEW_USER_DATA_DIR) : join(app.getPath('appData'), appName))
app.setPath('sessionData', app.getPath('userData'))
if (process.platform === 'win32') app.setAppUserModelId('local.tcview.desktop')
app.setAboutPanelOptions({ applicationName: appName, applicationVersion: metadata.version })
const root = resolve(__dirname, '../..')
const rendererPath = join(root, 'dist/renderer/index.html')
const devURL = process.env.TCVIEW_RENDERER_URL
if (devURL && devURL !== 'http://127.0.0.1:5173') throw new Error('不允许的开发页面地址')
const allowedURL = devURL ?? pathToFileURL(rendererPath).href
let window: BrowserWindow | null = null
let host: HostClient | undefined
let closing = false
let reconnecting: Promise<HostHello> | undefined

function senderAllowed(event: IpcMainInvokeEvent) {
  if (!window || event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame) throw new Error('不允许的 IPC 来源')
  const url = event.senderFrame.url
  if (devURL ? new URL(url).origin !== devURL : url.split('#')[0] !== allowedURL) throw new Error('不允许的 IPC 页面')
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('无效的请求参数')
  return value as Record<string, unknown>
}
function text(value: unknown, label: string, max = 128): string {
  if (typeof value !== 'string' || !value.trim() || value.length > max) throw new Error(`${label}无效`)
  return value.trim()
}
function id(value: unknown) {
  const result = text(value, '标识符')
  if (!/^[\w-]+$/.test(result)) throw new Error('标识符含无效字符')
  return result
}
function providerID(value: unknown) {
  const result = id(value)
  if (!['codex', 'claude', 'grok', 'agy'].includes(result)) throw new Error('不支持的 CLI')
  return result
}
function providerConfiguration(value: unknown): ProviderConfig {
  const input = record(value)
  const selector = (value: unknown, label: string) => {
    if (typeof value !== 'string' || value.length > 200 || /[\s\u0000-\u001f\u007f]/u.test(value)) throw new Error(`${label}无效`)
    return value
  }
  const selectedID = providerID(input.id)
  if (input.enabled !== undefined && typeof input.enabled !== 'boolean') throw new Error('CLI 启用状态必须是布尔值')
  if (input.command !== undefined && input.command !== '' && input.command !== selectedID) throw new Error('此页面不允许修改 CLI 可执行文件路径')
  return { id: selectedID, ...(typeof input.enabled === 'boolean' ? {enabled:input.enabled} : {}), model: selector(input.model, '模型 ID'), reasoning: selector(input.reasoning, '推理配置') }
}
function materials(value: unknown): MaterialInput[] {
  if (value === undefined) return []
  if (!Array.isArray(value) || value.length > 8) throw new Error('最多添加 8 份文本材料')
  return value.map(item => {
    const input = record(item)
    if (typeof input.content !== 'string' || !input.content.trim() || input.content.length > 64 * 1024) throw new Error('材料正文为空或超过 64 KiB')
    if (input.source !== 'file' && input.source !== 'pasted') throw new Error('材料来源无效')
    if (input.rawSha256 !== undefined && (typeof input.rawSha256 !== 'string' || !/^[a-f0-9]{64}$/.test(input.rawSha256))) throw new Error('材料原始指纹无效')
    return { id: id(input.id), title: text(input.title, '材料名称', 160), content: input.content, source: input.source, ...(typeof input.rawSha256 === 'string' ? {rawSha256:input.rawSha256} : {}) }
  })
}

async function readSelectedMaterial(path: string): Promise<MaterialInput> {
  const file = await open(path, constants.O_RDONLY | constants.O_NONBLOCK)
  try {
    const info = await file.stat()
    if (!info.isFile() || info.size > 64 * 1024) throw new Error(`${basename(path)} 需要是 64 KiB 以内的文本文件`)
    const buffer = Buffer.alloc(64 * 1024 + 1)
    let length = 0
    while (length < buffer.length) {
      const result = await file.read(buffer, length, buffer.length - length, null)
      if (!result.bytesRead) break
      length += result.bytesRead
    }
    const bytes = buffer.subarray(0, length)
    if (length > 64 * 1024 || !isUtf8(bytes) || bytes.includes(0)) throw new Error(`${basename(path)} 超过大小限制或不是 UTF-8 文本；请先导出为文本`)
    const content = bytes.toString('utf8')
    if (!content.trim()) throw new Error(`${basename(path)} 没有可用正文`)
    return { id: randomUUID(), title: basename(path), content, source:'file', rawSha256:createHash('sha256').update(bytes).digest('hex') }
  } finally { await file.close() }
}
function bind(channel: string, method: string, params: (value: unknown) => unknown = () => ({})) {
  ipcMain.handle(`tcview:${channel}`, (event, value: unknown) => {
    senderAllowed(event)
    if (!host) throw new Error('Go 后台尚未启动')
    return host.request(method, params(value))
  })
}

function registerBridge() {
  bind('hello', 'hello')
  bind('list', 'run.list')
  bind('trash', 'run.trash')
  bind('delete', 'run.delete', value => ({ runId: id(value) }))
  bind('restore', 'run.restore', value => ({ runId: id(value) }))
  bind('get', 'run.get', value => ({ runId: id(value) }))
  bind('stop', 'run.stop', value => ({ runId: id(value) }))
  bind('start', 'run.start', value => {
    const input = record(value)
    const mode = text(input.mode, '模式') as InputMode
    if (!['auto', 'free_debate', 'adjudication', 'delphi'].includes(mode)) throw new Error('不支持的讨论模式')
    return { goal: text(input.goal, '目标', 6_000), mode, idempotencyKey: id(input.idempotencyKey), materials:materials(input.materials), ...(input.preparationId ? {preparationId:id(input.preparationId)} : {}) }
  })
  bind('resume', 'run.resume', value => {
    const input = record(value)
    return { runId: id(input.runId), idempotencyKey: id(input.idempotencyKey) }
  })
  bind('events', 'run.events', value => {
    const input = record(value)
    if (!Number.isSafeInteger(input.afterEventId) || (input.afterEventId as number) < 0) throw new Error('无效的事件游标')
    return { runId: id(input.runId), afterEventId: input.afterEventId }
  })
  bind('providers', 'provider.list')
  bind('provider-check', 'provider.check', value => {
    if (typeof value === 'string') return { providerId: providerID(value) }
    const input = record(value), selectedID = providerID(input.providerId)
    const config = input.config === undefined ? undefined : providerConfiguration(input.config)
    if (config && config.id !== selectedID) throw new Error('检查配置的 CLI ID 不匹配')
    return {providerId: selectedID, ...(config ? {config} : {})}
  })
  bind('provider-settings-get', 'provider.settings.get')
  bind('provider-settings-save', 'provider.settings.save', value => {
    if (!Array.isArray(value) || value.length !== 4) throw new Error('需要同时保存四家 CLI 的配置')
    const configs = value.map(providerConfiguration)
    if (new Set(configs.map(config => config.id)).size !== 4) throw new Error('CLI 配置不能重复')
    return {configs}
  })
  bind('provider-settings-reset', 'provider.settings.reset')
  bind('provider-models', 'provider.models', value => ({providerId: providerID(value)}))
  bind('branch', 'run.branch', value => {
    const input=record(value)
    return { runId:id(input.runId), goal:text(input.goal,'补充要求',6000), idempotencyKey:id(input.idempotencyKey), materials:materials(input.materials) }
  })
  bind('topic-analyze','topic.analyze',value => {
    const input=record(value)
    const providerId=input.providerId === undefined ? undefined : providerID(input.providerId)
    return {id:id(input.id),goal:text(input.goal,'目标',6000),materials:materials(input.materials),...(providerId ? {providerId} : {})}
  })
  bind('topic-get','topic.get',value=>({id:id(value)}))
  bind('topic-cancel','topic.cancel',value=>({id:id(value)}))
  ipcMain.handle('tcview:materials-import',async event=>{
    senderAllowed(event)
    if(!window)throw new Error('窗口尚未就绪')
    const selected=await dialog.showOpenDialog(window,{title:'选择 UTF-8 文本材料',properties:['openFile','multiSelections'],filters:[{name:'文本与代码材料',extensions:['txt','md','markdown','json','yaml','yml','csv','log','rst','go','py','js','jsx','ts','tsx','sql','toml','ini','xml','html','css','sh']}]})
    if(selected.canceled)return []
    if(selected.filePaths.length>8)throw new Error('最多添加 8 份文本材料')
    const result:MaterialInput[]=[]
    let total=0
    for(const path of selected.filePaths){
      const material=await readSelectedMaterial(path)
      total+=Buffer.byteLength(material.content,'utf8')
      if(total>128*1024)throw new Error('材料合计超过 128 KiB，请减少正文后重新选择')
      result.push(material)
    }
    return result
  })
  ipcMain.handle('tcview:reconnect', event => {
    senderAllowed(event)
    reconnecting ??= replaceHost().finally(() => { reconnecting=undefined })
    return reconnecting
  })
  ipcMain.handle('tcview:export', async (event,value:unknown) => {
    senderAllowed(event)
    const input=record(value),runId=id(input.runId)
    const format=input.format
    if(format!=='markdown' && format!=='json') throw new Error('无效的导出格式')
    if(!host||!window) throw new Error('后台尚未连接')
    const artifact=await host.request<{suggestedName:string;content:string}>('artifact.export',{runId,format})
    const extension=format==='json'?'json':'md'
    const name=basename(artifact.suggestedName).replace(/[<>:"/\\|?*\u0000-\u001f]/g,'_') || `${appName}-${runId}.${extension}`
    const selected=await dialog.showSaveDialog(window,{title:'导出讨论结果',defaultPath:join(app.getPath('documents'),name),filters:[{name:format==='json'?'JSON':'Markdown',extensions:[extension]}]})
    if(selected.canceled||!selected.filePath) return {saved:false}
    await writeFile(selected.filePath,artifact.content,{encoding:'utf8',mode:0o600})
    return {saved:true,path:selected.filePath}
  })
  ipcMain.handle('tcview:diagnostics-clear', async (event,value:unknown) => {
    senderAllowed(event)
    const runId=id(value)
    if(!host||!window) throw new Error('后台尚未连接')
    const answer=await dialog.showMessageBox(window,{type:'question',title:'清理诊断日志',message:'清理这次讨论的原始输入输出日志？',detail:'将保留方案、投票、最终结果与审计账本。运行中的讨论无法清理。',buttons:['取消','清理诊断日志'],defaultId:0,cancelId:0})
    if(answer.response!==1) return {removedFiles:0,retained:['已取消']}
    return host.request('diagnostics.clear',{runId})
  })
}

function publish(event:DesktopEvent) {
  if(window&&!window.isDestroyed()&&!window.webContents.isDestroyed()) window.webContents.send('tcview:event',event)
}

async function replaceHost():Promise<HostHello> {
  const previous=host
  host=undefined
  if(previous) await previous.close()
  const instance=new HostClient(join(root,'dist/bin/tcview-host'),process.env.TCVIEW_DATA_DIR??join(app.getPath('userData'),'host'),process.env.TCVIEW_MOCK_DELAY_MS??'500',process.env.TCVIEW_PROVIDER_MODE??'cli')
  host=instance
  instance.on('event',(event:DesktopEvent)=>{if(host===instance)publish(event)})
  instance.on('disconnect',(message:string)=>{if(host===instance)publish({eventId:0,runId:'',attemptId:'',type:'host.disconnected',at:new Date().toISOString(),payload:{message}})})
  const hello=await instance.request<HostHello>('hello')
  if(hello.protocolVersion!==PROTOCOL_VERSION)throw new Error(`后台协议版本不兼容：${hello.protocolVersion}`)
  return hello
}

async function createWindow() {
  window = new BrowserWindow({
    title: `${appName} · 共识工作室`, width: 1440, height: 920, minWidth: 980, minHeight: 680,
    backgroundColor: '#161616', show: false,
    ...(process.platform === 'darwin' ? { titleBarStyle: 'hiddenInset' as const, trafficLightPosition: { x: 20, y: 21 } } : {}),
    webPreferences: {
      preload: join(root, 'dist/preload/index.cjs'), contextIsolation: true, nodeIntegration: false,
      sandbox: true, webSecurity: true,
    },
  })
  window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  window.webContents.on('will-navigate', (event, url) => { if (url !== allowedURL && url !== `${allowedURL}/`) event.preventDefault() })
  window.webContents.session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
  window.once('ready-to-show', () => window?.show())
  window.on('closed', () => { window = null })
  if (devURL) await window.loadURL(devURL)
  else await window.loadFile(rendererPath)
}

if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', () => { window?.show(); window?.focus() })
  void app.whenReady().then(async () => {
    Menu.setApplicationMenu(Menu.buildFromTemplate([
      ...(process.platform === 'darwin' ? [{ label: appName, submenu: [{ role: 'about' as const }, { type: 'separator' as const }, { role: 'hide' as const }, { role: 'quit' as const }] }] : []),
      { label: '编辑', submenu: [{ role: 'undo' }, { role: 'redo' }, { type: 'separator' }, { role: 'cut' }, { role: 'copy' }, { role: 'paste' }, { role: 'selectAll' }] },
      { label: '视图', submenu: [{ role: 'reload' }, { role: 'toggleDevTools' }, { role: 'resetZoom' }, { role: 'zoomIn' }, { role: 'zoomOut' }] },
    ]))
    registerBridge()
    await replaceHost()
    await createWindow()
  }).catch(error => { console.error(error); app.quit() })
  app.on('activate', () => { if (!window && host) void createWindow() })
  app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit() })
  app.on('before-quit', event => {
    if (!host || closing) return
    event.preventDefault()
    closing = true
    void host.close().finally(() => app.quit())
  })
  process.on('SIGTERM', () => app.quit())
  process.on('SIGINT', () => app.quit())
}
