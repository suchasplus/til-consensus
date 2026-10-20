import { spawn } from 'node:child_process'
import electron from 'electron'
import { root } from './go-env.mjs'
await import('./build.mjs')
const env = { ...process.env }
delete env.ELECTRON_RUN_AS_NODE
const child = spawn(electron, ['.'], { cwd: root, env, stdio: 'inherit' })
process.on('SIGINT', () => child.kill('SIGINT'))
process.on('SIGTERM', () => child.kill('SIGTERM'))
child.on('exit', code => process.exit(code ?? 1))

