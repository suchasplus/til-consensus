import { existsSync, readFileSync, realpathSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'
import { executionSourceFingerprint, sourceAlgorithm } from './source-identity.mjs'

export const root = realpathSync(resolve(import.meta.dirname, '..'))
export const repositoryRoot = realpathSync(resolve(root, '..'))
export const hostDirectory = join(root, 'services/host')
export const engineModule = 'github.com/suchasplus/til-consensus'
export const hostModule = `${engineModule}/desktop/services/host`
export const workspacePath = join(repositoryRoot, 'go.work')

function rejectExternalOverride() {
  if (process.env.TIL_CONSENSUS_DIR?.trim()) {
    throw new Error('合仓构建不接受 TIL_CONSENSUS_DIR 外部源码覆盖；请移除此变量，使用当前仓库的根模块和 go.work。')
  }
}

function moduleName(directory) {
  return /^\s*module\s+(\S+)/m.exec(readFileSync(join(directory, 'go.mod'), 'utf8'))?.[1]
}

function goFlagTokens(value) {
  // Match Go 1.27 cmd/internal/quoted.Split: only ASCII space/tab/CR/LF
  // separate fields; quotes count only at a field's start; no unescaping.
  const tokens = []
  let offset = 0
  while (offset < value.length) {
    while (offset < value.length && /[ \t\r\n]/.test(value[offset])) offset++
    if (offset === value.length) break
    const quote = value[offset]
    if (quote === '"' || quote === "'") {
      const end = value.indexOf(quote, offset + 1)
      if (end === -1) throw new Error('GOFLAGS 含未闭合引号，无法校验执行源码覆盖参数。')
      tokens.push(value.slice(offset + 1, end))
      offset = end + 1
    } else {
      const start = offset
      while (offset < value.length && !/[ \t\r\n]/.test(value[offset])) offset++
      tokens.push(value.slice(start, offset))
    }
  }
  return tokens
}

function buildEnvironment() {
  rejectExternalOverride()
  // Explicit workspace and flags prevent ambient go.work / GOENV overlays from
  // selecting a second checkout or replacing fingerprinted source at build time.
  const flags = process.env.GOFLAGS ?? ''
  // cmd/go also accepts --flag=val in GOFLAGS (base/goflags.go).
  if (goFlagTokens(flags).some(flag => /^--?(?:overlay|modfile)(?:=|$)/.test(flag))) {
    throw new Error('合仓构建不接受 GOFLAGS 中的 -overlay 或 -modfile；它们会覆盖已记录的执行源码。')
  }
  return { ...process.env, GOWORK: workspacePath, GOFLAGS: flags }
}

export function goEnvironment() {
  const environment = buildEnvironment()
  if (!existsSync(workspacePath)) throw new Error('合仓构建缺少仓库根 go.work；请使用完整的 til-consensus checkout。')
  if (moduleName(repositoryRoot) !== engineModule || moduleName(hostDirectory) !== hostModule) {
    throw new Error('合仓 Go 模块路径无效：必须保留根 til-consensus SDK 模块和 desktop/services/host 模块。')
  }
  const workspace = JSON.parse(execFileSync('go', ['work', 'edit', '-json', workspacePath], {
    cwd: repositoryRoot, env: environment, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  }))
  const expected = [repositoryRoot, hostDirectory].sort()
  const modules = (workspace.Use ?? []).map(entry => realpathSync(resolve(repositoryRoot, entry.DiskPath))).sort()
  if (modules.length !== 2 || modules.some((directory, index) => directory !== expected[index]) || workspace.Replace?.length) {
    throw new Error('仓库 go.work 必须仅包含当前根模块与 desktop/services/host，且不能含 replace 覆盖。')
  }
  for (const directory of [repositoryRoot, hostDirectory]) {
    const moduleFile = readFileSync(join(directory, 'go.mod'), 'utf8')
    if (/(?:^|\n)\s*(?:replace\s+)?github\.com\/suchasplus\/til-consensus(?:\/desktop\/services\/host)?(?:\s+\S+)?\s*=>/.test(moduleFile)) {
      throw new Error('同仓 Go 模块不得含 til-consensus 的 replace 覆盖；必须使用根 go.work。')
    }
  }
  for (const [name, directory] of [[engineModule, repositoryRoot], [hostModule, hostDirectory]]) {
    const metadata = JSON.parse(execFileSync('go', ['list', '-m', '-json', name], {
      cwd: hostDirectory, env: environment, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
    }))
    if (metadata.Path !== name || metadata.Main !== true || metadata.Replace || !metadata.Dir || realpathSync(metadata.Dir) !== directory) {
      throw new Error(`Go 模块 ${name} 未解析到当前 checkout：要求 Main=true 且 Dir=${directory}，禁止远端缓存或其他源码覆盖。`)
    }
  }
  return environment
}

function identityManifest() {
  const manifest = JSON.parse(readFileSync(join(root, 'scripts/engine-lock.json'), 'utf8'))
  if (manifest.schemaVersion !== 1 || manifest.module !== engineModule || manifest.algorithm !== sourceAlgorithm || !/^[0-9a-f]{64}$/.test(manifest.baseline?.sourceHash ?? '') || !/^[0-9a-f]{40}$/.test(manifest.baseline?.engineVersion ?? '')) {
    throw new Error('scripts/engine-lock.json 执行源码身份清单无效；该文件记录精确迁移基线，不再选择远端内核。')
  }
  return manifest
}

export function engineSourceIdentity() {
  rejectExternalOverride()
  const manifest = identityManifest()
  const fingerprint = executionSourceFingerprint(repositoryRoot)
  const baselineMatched = fingerprint.sourceHash === manifest.baseline.sourceHash
  return {
    ...fingerprint,
    engineVersion: baselineMatched ? manifest.baseline.engineVersion : `source-sha256:${fingerprint.sourceHash}`,
    baselineMatched,
  }
}

export function engineBuildVersion() {
  const identity = engineSourceIdentity()
  if (process.env.TCVIEW_RELEASE === '1') repositoryBuildInfo(identity)
  return identity.engineVersion
}

export function repositoryBuildInfo(identity = engineSourceIdentity()) {
  rejectExternalOverride()
  const gitEnvironment = { ...process.env }
  for (const name of ['GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR']) delete gitEnvironment[name]
  const git = args => execFileSync('git', ['-C', repositoryRoot, ...args], { env: gitEnvironment, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  let repositoryRevision, dirty
  try {
    if (realpathSync(git(['rev-parse', '--show-toplevel'])) !== repositoryRoot) throw new Error('wrong repository root')
    repositoryRevision = git(['rev-parse', 'HEAD'])
    dirty = git(['status', '--porcelain=v1', '--untracked-files=all']).length > 0
  } catch {
    throw new Error('构建需要可验证的 til-consensus Git checkout，无法确定仓库 revision 和 dirty 状态。')
  }
  if (!/^[0-9a-f]{40}$/.test(repositoryRevision)) throw new Error('仓库 Git revision 无效。')
  if (process.env.TCVIEW_RELEASE === '1' && dirty) {
    throw new Error('TCVIEW_RELEASE=1 要求干净的 Git checkout（包含未跟踪源码）；请提交或清理当前改动后发布。')
  }
  return { repositoryRevision, sourceHash: identity.sourceHash, engineVersion: identity.engineVersion, dirty }
}
