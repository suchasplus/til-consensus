import { spawn } from 'node:child_process'
import electron from 'electron'
import { root } from './go-env.mjs'
import './build-host.mjs'
await import('./build-electron.mjs')
const { createServer } = await import('vite')
const server = await createServer()
await server.listen()
const env = { ...process.env, TCVIEW_RENDERER_URL: 'http://127.0.0.1:5173' }
delete env.ELECTRON_RUN_AS_NODE
const child = spawn(electron, ['.'], { cwd: root, env, stdio: 'inherit' })
let closing = false
async function stop(code = 0) {
  if (closing) return
  closing = true
  child.kill('SIGTERM')
  await server.close()
  process.exitCode = code
}
process.on('SIGINT', () => void stop())
process.on('SIGTERM', () => void stop())
child.on('exit', code => void stop(code ?? 0))

