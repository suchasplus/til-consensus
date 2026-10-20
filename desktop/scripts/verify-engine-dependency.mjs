// 不调用模型：以独立 Git checkout、冷 Go 缓存验证当前同仓源码构建。
// npm ci 的已安装 Node 依赖明确复用；这不是 NPM 冷缓存验收。
import { cpSync, existsSync, lstatSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { execFileSync, spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { root, repositoryRoot, engineModule, hostModule, repositoryBuildInfo } from './go-env.mjs'

const sourceIdentity = repositoryBuildInfo()
const releaseBuild = process.env.TCVIEW_RELEASE === '1'
const directory = join(root, '.tcview', 'dependency-upgrade', new Date().toISOString().replaceAll(':', '-'))
const project = join(directory, 'project')
const desktop = join(project, 'desktop')
const home = join(directory, 'home')
const moduleCache = join(directory, 'module-cache')
const buildCache = join(directory, 'build-cache')
mkdirSync(directory, { recursive: true, mode: 0o700 })
mkdirSync(home, { recursive: true, mode: 0o700 })
const gitEnvironment = { ...process.env }
for (const name of ['GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR']) delete gitEnvironment[name]
const git = (cwd, args, options = {}) => execFileSync('git', ['-C', cwd, ...args], {
  env: gitEnvironment, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], ...options,
})

// A normal clone copies the actual commit history. --no-hardlinks ensures that
// Git objects do not depend on the source checkout remaining at its old path.
git(repositoryRoot, ['clone', '--no-hardlinks', '--no-checkout', repositoryRoot, project])
git(project, ['checkout', '--detach', sourceIdentity.repositoryRevision])
if (existsSync(join(project, '.git/objects/info/alternates'))) throw new Error('隔离 checkout 不得通过 Git alternates 依赖外部对象目录')
// Include HEAD paths as well: staged deletions and renames no longer appear in
// ls-files, but must still remove the corresponding file from the clone.
const files = [...new Set([
  ...git(repositoryRoot, ['ls-files', '--cached', '--others', '--exclude-standard', '-z']).split('\0'),
  ...git(repositoryRoot, ['ls-tree', '-r', '--name-only', '-z', sourceIdentity.repositoryRevision]).split('\0'),
].filter(Boolean))]
for (const file of files) {
  const source = join(repositoryRoot, file)
  const target = join(project, file)
  if (!existsSync(source)) {
    rmSync(target, { recursive: true, force: true })
    continue
  }
  if (!lstatSync(source).isFile()) throw new Error(`隔离源码镜像仅接受普通文件：${file}`)
  mkdirSync(dirname(target), { recursive: true })
  cpSync(source, target)
}
if (!existsSync(join(root, 'node_modules'))) throw new Error('请先在 desktop 执行 npm ci，再进行隔离依赖验收')
symlinkSync(join(root, 'node_modules'), join(desktop, 'node_modules'), 'dir')
const obsoleteWorkspace = join(directory, 'unrelated.go.work')
writeFileSync(obsoleteWorkspace, 'This unrelated ambient workspace must never be used.\n')
const environment = {
  ...process.env,
  HOME: home,
  GOMODCACHE: moduleCache,
  GOPATH: join(directory, 'gopath'),
  GOCACHE: buildCache,
  GOPROXY: 'https://proxy.golang.org,direct',
  GOSUMDB: 'sum.golang.org',
  GOWORK: obsoleteWorkspace,
  TCVIEW_RELEASE: releaseBuild ? '1' : '',
}
for (const name of ['TIL_CONSENSUS_DIR', 'ELECTRON_RUN_AS_NODE', 'GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR']) delete environment[name]
console.log(`独立同仓构建验收目录：${directory}（Go 模块缓存和构建缓存均为空；Node 依赖复用）`)
execFileSync('npm', ['run', 'build'], { cwd: desktop, env: environment, stdio: 'inherit' })
const workspaceEnvironment = { ...environment, GOWORK: join(project, 'go.work') }
const metadata = JSON.parse(execFileSync('go', ['list', '-m', '-json', engineModule], { cwd: join(desktop, 'services/host'), env: workspaceEnvironment, encoding: 'utf8' }))
const hostMetadata = JSON.parse(execFileSync('go', ['list', '-m', '-json', hostModule], { cwd: join(desktop, 'services/host'), env: workspaceEnvironment, encoding: 'utf8' }))
if (metadata.Main !== true || metadata.Replace || metadata.Dir !== project || hostMetadata.Main !== true || hostMetadata.Replace || hostMetadata.Dir !== join(desktop, 'services/host')) {
  throw new Error('内核与 host 未从隔离 checkout 的两个 Main 模块解析')
}
const buildInfo = JSON.parse(readFileSync(join(desktop, 'dist/host-build-info.json'), 'utf8'))
if (buildInfo.sourceHash !== sourceIdentity.sourceHash || buildInfo.engineVersion !== sourceIdentity.engineVersion || buildInfo.repositoryRevision !== sourceIdentity.repositoryRevision || (releaseBuild && buildInfo.dirty)) {
  throw new Error('隔离构建身份与原始 checkout 不一致')
}
execFileSync('go', ['mod', 'verify'], { cwd: join(desktop, 'services/host'), env: workspaceEnvironment, stdio: 'inherit' })
execFileSync('go', ['build', '-o', join(directory, 'til-native-audit'), join(desktop, 'scripts/cli-audit/til-native/main.go')], { cwd: join(desktop, 'services/host'), env: workspaceEnvironment, stdio: 'inherit' })

// Public SDK stays a standalone module: no workspace is needed to resolve its
// dependencies, and desktop SQLite must not leak into its module graph.
const sdkEnvironment = { ...environment, GOWORK: 'off', GOFLAGS: '' }
execFileSync('go', ['mod', 'download'], { cwd: project, env: sdkEnvironment, stdio: 'inherit' })
execFileSync('go', ['mod', 'verify'], { cwd: project, env: sdkEnvironment, stdio: 'inherit' })
const sdkModules = execFileSync('go', ['list', '-m', 'all'], { cwd: project, env: sdkEnvironment, encoding: 'utf8' }).trim().split('\n')
if (sdkModules.some(line => line.startsWith(hostModule) || line.startsWith('modernc.org/sqlite '))) throw new Error('桌面模块或 SQLite 依赖泄漏到公共 SDK 模块')
const host = spawnSync(join(desktop, 'dist/bin/tcview-host'), ['--data-dir', join(directory, 'host-data'), '--provider-mode', 'cli'], {
  env: workspaceEnvironment, encoding: 'utf8', input: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'hello', params: {} }) + '\n', timeout: 30_000,
})
if (host.status !== 0) throw new Error(`独立后台启动失败：${host.stderr || host.error?.message}`)
const hello = host.stdout.trim().split('\n').map(line => JSON.parse(line)).find(frame => frame.id === 1)?.result
if (hello?.engineVersion !== sourceIdentity.engineVersion || hello?.providerMode !== 'cli' || hello?.protocolVersion !== '1.4') throw new Error('独立后台源码身份或默认模式不符')
const result = {
  passed: true, directory,
  source: sourceIdentity,
  buildInfo,
  module: { path: metadata.Path, main: metadata.Main, directory: metadata.Dir },
  hostModule: { path: hostMetadata.Path, main: hostMetadata.Main, directory: hostMetadata.Dir },
  hello,
  freshGoModuleCache: true,
  freshGoBuildCache: true,
  reusedNodeDependencies: true,
  gitObjectsIndependent: true,
  currentCheckoutSourceMirrored: true,
  externalSourceOverride: false,
  releaseBuild,
  ambientWorkspaceIgnored: readFileSync(obsoleteWorkspace, 'utf8').startsWith('This unrelated ambient workspace'),
  sdkStandalone: { workspace: 'off', dependenciesDownloaded: true, desktopDependenciesAbsent: true },
  modelRequests: 0,
}
writeFileSync(join(directory, 'summary.json'), JSON.stringify(result, null, 2) + '\n', { mode: 0o600 })
console.log(JSON.stringify(result, null, 2))
