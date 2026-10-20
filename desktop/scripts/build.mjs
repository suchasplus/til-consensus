import './build-host.mjs'
await import('./build-electron.mjs')
const { build } = await import('vite')
await build()

