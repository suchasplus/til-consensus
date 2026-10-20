import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve, join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'

export const root = resolve(import.meta.dirname, '..')
export const hostDirectory = join(root, 'services/host')
const lock = Object.freeze(JSON.parse(readFileSync(join(root, 'scripts/engine-lock.json'), 'utf8')))
export const engineCommit = lock.commit
export const engineModuleVersion = lock.moduleVersion
export const engineModule = lock.module

// The normal build uses the published module pinned by go.mod with GOWORK=off.
// A local checkout participates only when TIL_CONSENSUS_DIR is explicitly set.
// engine-lock.json is the single build-identity pin; keep its moduleVersion in
// sync with services/host/go.mod when upgrading the dependency.
export function validateEngineLock(candidate, goMod) {
  if (candidate.module !== 'github.com/suchasplus/til-consensus' || !/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(candidate.moduleVersion ?? '') || !/^[0-9a-f]{40}$/.test(candidate.commit ?? '')) {
    throw new Error('scripts/engine-lock.json 的模块名、版本或完整提交 SHA 无效')
  }
  const pseudoCommit = /[.-]\d{14}-([0-9a-f]{12})$/.exec(candidate.moduleVersion)?.[1]
  if (pseudoCommit && !candidate.commit.startsWith(pseudoCommit)) {
    throw new Error('内核版本锁不一致：moduleVersion 的提交后缀与 commit 不符')
  }
  const selected = /^\s*(?:require\s+)?github\.com\/suchasplus\/til-consensus\s+(\S+)/m.exec(goMod)?.[1]
  if (selected !== candidate.moduleVersion) {
    throw new Error(`内核版本锁不一致：engine-lock.json 固定 ${candidate.moduleVersion}，services/host/go.mod 为 ${selected ?? '未固定'}；请同时更新版本锁和 go.mod`)
  }
  if (/^\s*(?:replace\s+)?github\.com\/suchasplus\/til-consensus(?:\s+\S+)?\s*=>/m.test(goMod)) {
    throw new Error('services/host/go.mod 不得覆盖已锁定的 til-consensus；本地开发请显式设置 TIL_CONSENSUS_DIR')
  }
  return candidate
}

function checkedLock() {
  return validateEngineLock(lock, readFileSync(join(hostDirectory, 'go.mod'), 'utf8'))
}

function localEngine(configured) {
  const source = resolve(configured)
  const overrideHint = `请修正 TIL_CONSENSUS_DIR，或移除此变量以使用固定远端模块 ${engineModuleVersion}。`
  if (!existsSync(join(source, 'go.mod'))) {
    throw new Error(`TIL_CONSENSUS_DIR 不存在或缺少 go.mod：${source}。${overrideHint}`)
  }
  const module = readFileSync(join(source, 'go.mod'), 'utf8')
  if (!/^\s*module\s+github\.com\/suchasplus\/til-consensus(?:\s|$)/m.test(module)) {
    throw new Error(`TIL_CONSENSUS_DIR 不是 til-consensus 模块：${source}`)
  }
  let revision, status
  try {
    revision = execFileSync('git', ['-C', source, 'rev-parse', 'HEAD'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
    status = execFileSync('git', ['-C', source, 'status', '--porcelain'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  } catch {
    throw new Error(`本地 til-consensus 必须是可校验固定提交的 Git checkout：${source}。${overrideHint}`)
  }
  if (revision !== engineCommit) {
    throw new Error(`til-consensus 基线提交不符：需要 ${engineCommit}，当前 ${revision}。${overrideHint}`)
  }
  if (process.env.TCVIEW_RELEASE === '1' && status) {
    throw new Error('发布构建要求显式本地 til-consensus 覆盖为干净的固定提交；请清理本地补丁，或移除 TIL_CONSENSUS_DIR 以使用固定远端模块。')
  }
  const requiredInterfaces = [
    ['runtime/cli/runner.go', /func\s+NewRunnerWithOptions\s*\(/, 'cli.NewRunnerWithOptions'],
    ['runtime/cli/runner.go', /RawOutput\s+string/, 'cli.RunTaskResult.RawOutput'],
    ['runtime/cli/progress.go', /type\s+RunnerOptions\s+struct/, 'cli.RunnerOptions'],
    ['runtime/cli/progress.go', /type\s+CLIEvent\s+struct/, 'cli.CLIEvent'],
    ['config/types.go', /Workdir\s+string/, 'config.ProviderConfig.Workdir'],
    ['config/types.go', /PermissionMode\s+string/, 'config.ProviderConfig.PermissionMode'],
    ['config/types.go', /OutputFormat\s+string/, 'config.ProviderConfig.OutputFormat'],
  ]
  const missing = requiredInterfaces.filter(([file, pattern]) => !existsSync(join(source, file)) || !pattern.test(readFileSync(join(source, file), 'utf8'))).map(([, , name]) => name)
  if (missing.length) {
    throw new Error(`本地 til-consensus 尚未包含所需升级接口：${missing.join('、')}。${overrideHint}`)
  }
  return { source, revision, status }
}

export function goEnvironment() {
  checkedLock()
  const configured = process.env.TIL_CONSENSUS_DIR?.trim()
  if (!configured) return { ...process.env, GOWORK: 'off' }
  const { source } = localEngine(configured)
  const workspaceDirectory = join(root, '.tcview/local-engine')
  const workspace = join(workspaceDirectory, 'go.work')
  mkdirSync(workspaceDirectory, { recursive: true })
  writeFileSync(workspace, `go 1.27.1\n\nuse (\n  ${JSON.stringify(hostDirectory)}\n  ${JSON.stringify(source)}\n)\n`)
  return { ...process.env, GOWORK: workspace }
}

export function engineBuildVersion() {
  checkedLock()
  const configured = process.env.TIL_CONSENSUS_DIR?.trim()
  if (!configured) return engineCommit
  const { source, revision, status } = localEngine(configured)
  if (!status) return revision
  const files = execFileSync('git', ['-C', source, 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(file => file.endsWith('.go') || file === 'go.mod' || file === 'go.sum').sort()
  const hash = createHash('sha256')
  for (const file of files) {
    hash.update(file + '\0')
    // Deleted tracked files also change the build identity.
    if (existsSync(join(source, file))) hash.update(readFileSync(join(source, file)))
    else hash.update('\0deleted\0')
  }
  const version = `${engineCommit}+local.${hash.digest('hex').slice(0, 16)}`
  console.info(`使用本地内核开发版本：${version}`)
  return version
}

