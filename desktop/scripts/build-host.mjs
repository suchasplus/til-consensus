import { mkdirSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { root, hostDirectory, goEnvironment, engineBuildVersion } from './go-env.mjs'

const binaryDirectory = join(root, 'dist/bin')
rmSync(binaryDirectory, { recursive: true, force: true })
mkdirSync(binaryDirectory, { recursive: true })
execFileSync('go', ['build', '-trimpath', '-ldflags', `-X github.com/suchasplus/til-consensus/desktop/services/host/internal/model.EngineVersion=${engineBuildVersion()}`, '-o', join(binaryDirectory, 'tcview-host'), './cmd/tcview-host'], {
  cwd: hostDirectory, env: goEnvironment(), stdio: 'inherit',
})

