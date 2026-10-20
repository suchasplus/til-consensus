import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { root, hostDirectory, goEnvironment, repositoryBuildInfo } from './go-env.mjs'

const environment = goEnvironment()
const identity = repositoryBuildInfo()
const binaryDirectory = join(root, 'dist/bin')
rmSync(binaryDirectory, { recursive: true, force: true })
mkdirSync(binaryDirectory, { recursive: true })
execFileSync('go', ['build', '-trimpath', '-ldflags', `-X github.com/suchasplus/til-consensus/desktop/services/host/internal/model.EngineVersion=${identity.engineVersion}`, '-o', join(binaryDirectory, 'tcview-host'), './cmd/tcview-host'], {
  cwd: hostDirectory, env: environment, stdio: 'inherit',
})
const completedIdentity = repositoryBuildInfo()
if (completedIdentity.sourceHash !== identity.sourceHash || completedIdentity.repositoryRevision !== identity.repositoryRevision) {
  throw new Error('构建期间执行源码或仓库提交发生变化；请重建以保证二进制身份可追踪。')
}
writeFileSync(join(root, 'dist/host-build-info.json'), JSON.stringify(identity, null, 2) + '\n')

