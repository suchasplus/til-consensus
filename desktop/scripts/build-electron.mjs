import { build } from 'esbuild'
import { root } from './go-env.mjs'
import { join } from 'node:path'

await build({
  entryPoints: { 'main/index': join(root, 'apps/desktop/main/index.ts'), 'preload/index': join(root, 'apps/desktop/preload/index.ts') },
  outdir: join(root, 'dist'),
  outExtension: { '.js': '.cjs' },
  bundle: true, platform: 'node', format: 'cjs', target: 'node24',
  external: ['electron'], sourcemap: true,
})

