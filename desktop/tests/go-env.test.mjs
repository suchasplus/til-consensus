import test from 'node:test'
import assert from 'node:assert/strict'
import { cpSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, realpathSync, writeFileSync, rmSync, appendFileSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { execFileSync, spawnSync } from 'node:child_process'
import { pathToFileURL, fileURLToPath } from 'node:url'

const scriptsDirectory = fileURLToPath(new URL('../scripts/', import.meta.url))
const engineModule = 'github.com/suchasplus/til-consensus'
const hostModule = `${engineModule}/desktop/services/host`
const identityAlgorithm = 'tcview-execution-source-sha256-v1'

// These are real, independent Git repositories and real local Go modules.
// No git/go substitute, network access, CLI authentication or model request is
// involved. Source identity must be derived from the current checkout itself.
function fixture(t) {
  const directory = realpathSync(mkdtempSync(join(tmpdir(), 'tcview-monorepo-test-')))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const repository = join(directory, 'til-consensus')
  const desktop = join(repository, 'desktop')
  const host = join(desktop, 'services/host')
  mkdirSync(join(desktop, 'scripts'), { recursive: true })
  mkdirSync(host, { recursive: true })
  for (const file of readdirSync(scriptsDirectory)) {
    if (file.endsWith('.mjs') || file === 'engine-lock.json') cpSync(join(scriptsDirectory, file), join(desktop, 'scripts', file))
  }
  writeFileSync(join(repository, '.gitignore'), '.tcview/\nnode_modules/\ndist/\n')
  writeFileSync(join(repository, 'go.mod'), `module ${engineModule}\n\ngo 1.27.1\n`)
  writeFileSync(join(repository, 'go.sum'), '')
  writeFileSync(join(repository, 'consensus.go'), 'package consensus\nconst Source = "fixture-local-checkout"\n')
  writeFileSync(join(repository, 'consensus_test.go'), 'package consensus\n')
  writeFileSync(join(host, 'go.mod'), `module ${hostModule}\n\ngo 1.27.1\n\nrequire ${engineModule} v0.0.0\n`)
  writeFileSync(join(host, 'go.sum'), '')
  writeFileSync(join(host, 'host.go'), 'package host\n')
  writeFileSync(join(repository, 'go.work'), 'go 1.27.1\n\nuse (\n  .\n  ./desktop/services/host\n)\n')
  mkdirSync(join(desktop, 'apps/desktop/renderer'), { recursive: true })
  writeFileSync(join(desktop, 'apps/desktop/renderer/styles.css'), 'body { color: #161616; }\n')
  writeFileSync(join(repository, 'README.md'), '# Independent build test fixture\n')
  const environment = { ...process.env, GOTOOLCHAIN: 'local', GOPROXY: 'off', GOSUMDB: 'off', GOWORK: '/ambient/workspace/must/not/be/read', TCVIEW_RELEASE: '', GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null' }
  for (const key of ['TIL_CONSENSUS_DIR', 'GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE']) delete environment[key]
  const f = { directory, repository, desktop, host, environment, moduleURL: pathToFileURL(join(desktop, 'scripts/go-env.mjs')).href }
  git(f, 'init', '--quiet')
  commit(f, 'fixture source')
  return f
}

function git(f, ...args) {
  return execFileSync('git', ['-C', f.repository, ...args], { env: f.environment, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
}

function commit(f, message) {
  git(f, 'add', '--all')
  git(f, '-c', 'user.name=TCView build fixture', '-c', 'user.email=tcview-fixture@example.invalid', '-c', 'commit.gpgSign=false', 'commit', '--quiet', '-m', message)
  return git(f, 'rev-parse', 'HEAD')
}

function invoke(f, expression = 'build.engineSourceIdentity()', environment = f.environment) {
  return spawnSync(process.execPath, ['--input-type=module', '-e', `import * as build from ${JSON.stringify(f.moduleURL)}; try { console.log(JSON.stringify(${expression})) } catch (error) { console.error(error.message); process.exitCode = 1 }`], { cwd: f.desktop, env: environment, encoding: 'utf8', timeout: 30_000 })
}

function parsed(result) {
  assert.equal(result.error, undefined)
  assert.equal(result.status, 0, result.stderr)
  return JSON.parse(result.stdout.trim().split('\n').at(-1))
}

function identity(f, environment) {
  const result = parsed(invoke(f, 'build.engineSourceIdentity()', environment))
  assert.equal(result.algorithm, identityAlgorithm)
  assert.match(result.sourceHash, /^[a-f0-9]{64}$/)
  assert.ok(result.files.includes('go.mod'))
  assert.ok(result.files.includes('desktop/services/host/host.go'))
  return result
}

function rejected(result, pattern) {
  assert.equal(result.status, 1, result.stdout)
  assert.match(result.stderr, pattern)
}

test('同仓构建强制根 workspace，忽略 ambient GOWORK 并解析当前 checkout 的两个 Main 模块', t => {
  const f = fixture(t)
  for (const ambient of ['off', '/missing/ambient/go.work', join(f.directory, 'broken.work')]) {
    if (ambient.endsWith('broken.work')) writeFileSync(ambient, 'invalid workspace must never be read\n')
    const environment = { ...f.environment, GOWORK: ambient }
    const checked = parsed(invoke(f, '({ GOWORK: build.goEnvironment().GOWORK, repositoryRoot: build.repositoryRoot, root: build.root })', environment))
    assert.equal(checked.GOWORK, join(f.repository, 'go.work'))
    assert.equal(checked.repositoryRoot, f.repository)
    assert.equal(checked.root, f.desktop)
    const engine = JSON.parse(execFileSync('go', ['list', '-m', '-json', engineModule], { cwd: f.host, env: { ...environment, GOWORK: checked.GOWORK }, encoding: 'utf8' }))
    const host = JSON.parse(execFileSync('go', ['list', '-m', '-json', hostModule], { cwd: f.host, env: { ...environment, GOWORK: checked.GOWORK }, encoding: 'utf8' }))
    assert.equal(engine.Main, true)
    assert.equal(realpathSync(engine.Dir), f.repository)
    assert.equal(engine.Version, undefined)
    assert.equal(host.Main, true)
    assert.equal(realpathSync(host.Dir), f.host)
  }
})

test('旧外部源码覆盖变量被明确拒绝，不能悄悄使用远端或另一份 checkout', t => {
  const f = fixture(t)
  for (const source of [f.repository, join(f.directory, 'missing')]) {
    const environment = { ...f.environment, TIL_CONSENSUS_DIR: source }
    rejected(invoke(f, 'build.goEnvironment().GOWORK', environment), /TIL_CONSENSUS_DIR/)
    rejected(invoke(f, 'build.engineBuildVersion()', environment), /TIL_CONSENSUS_DIR/)
  }
})

test('Go overlay 与 modfile 参数不能绕过当前 checkout 的源码校验', t => {
  const f = fixture(t)
  const externalSource = join(f.directory, 'outside.go')
  writeFileSync(externalSource, 'package consensus\nimport _ "unsafe"\nconst Source = "external-overlay"\n')
  const overlays = [join(f.directory, 'overlay.json'), join(f.directory, 'path with spaces/overlay.json')]
  for (const overlay of overlays) {
    mkdirSync(join(overlay, '..'), { recursive: true })
    writeFileSync(overlay, JSON.stringify({ Replace: { [join(f.repository, 'consensus.go')]: externalSource } }))
  }
  const modfiles = [join(f.directory, 'override.mod'), join(f.directory, 'path with spaces/override.mod')]
  for (const modfile of modfiles) writeFileSync(modfile, 'module fixture/externally-overridden\n\ngo 1.27.1\n')
  // Go's actual quoted-field parser accepts quotes around the complete flag,
  // including embedded spaces and adjacent quoted fields without whitespace.
  // Run real `go list` first to prove these are effective overrides, then check
  // that TCView rejects the same GOFLAGS before resolving its build workspace.
  const acceptedByGo = [
    { flags: `-overlay=${overlays[0]}`, kind: 'overlay' },
    { flags: `'-overlay=${overlays[0]}'`, kind: 'overlay' },
    { flags: `"-overlay=${overlays[0]}"`, kind: 'overlay' },
    { flags: `'--overlay=${overlays[1]}'`, kind: 'overlay' },
    { flags: `"-overlay=${overlays[1]}"`, kind: 'overlay' },
    { flags: `'-trimpath''--overlay=${overlays[1]}'`, kind: 'overlay' },
    { flags: `-trimpath\t"--overlay=${overlays[1]}"\n`, kind: 'overlay' },
    { flags: `-modfile=${modfiles[0]}`, kind: 'modfile' },
    { flags: `'-modfile=${modfiles[0]}'`, kind: 'modfile' },
    { flags: `"-modfile=${modfiles[0]}"`, kind: 'modfile' },
    { flags: `'--modfile=${modfiles[1]}'`, kind: 'modfile' },
    { flags: `"-modfile=${modfiles[1]}"`, kind: 'modfile' },
  ]
  for (const { flags, kind } of acceptedByGo) {
    const goArgs = kind === 'overlay' ? ['list', '-json', '.'] : ['list', '-m', '-json']
    const metadata = JSON.parse(execFileSync('go', goArgs, { cwd: f.repository, env: { ...f.environment, GOFLAGS: flags, GOWORK: kind === 'overlay' ? join(f.repository, 'go.work') : 'off' }, encoding: 'utf8' }))
    if (kind === 'overlay') assert.ok(metadata.Imports.includes('unsafe'), `真实 Go 未使用该 overlay：${flags}`)
    else assert.equal(metadata.Path, 'fixture/externally-overridden', `真实 Go 未使用该 modfile：${flags}`)
    rejected(invoke(f, 'build.goEnvironment().GOWORK', { ...f.environment, GOFLAGS: flags }), /GOFLAGS|overlay|modfile/)
  }
  // Unsupported argument forms must also fail closed instead of being ignored.
  for (const flags of ['-modfile /outside/go.mod', '-trimpath -overlay /outside/overlay.json']) {
    rejected(invoke(f, 'build.goEnvironment().GOWORK', { ...f.environment, GOFLAGS: flags }), /GOFLAGS|overlay|modfile/)
  }
  const benignFlags = '\'-trimpath\' "-buildvcs=false"'
  assert.equal(parsed(invoke(f, 'build.goEnvironment().GOFLAGS', { ...f.environment, GOFLAGS: benignFlags })), benignFlags)
})

test('缺失或篡改同仓 workspace 会失败，不能回退到锁定的远端模块', t => {
  const f = fixture(t)
  const workspace = join(f.repository, 'go.work')
  rmSync(workspace)
  rejected(invoke(f, 'build.goEnvironment().GOWORK'), /go\.work|workspace|工作区/)
  const external = join(f.directory, 'outside-engine')
  mkdirSync(external)
  writeFileSync(join(external, 'go.mod'), `module ${engineModule}\n\ngo 1.27.1\n`)
  writeFileSync(workspace, 'go 1.27.1\nuse (\n  ../outside-engine\n  ./desktop/services/host\n)\n')
  rejected(invoke(f, 'build.goEnvironment().GOWORK'), /同仓|当前|根|workspace|工作区|源码/)
})

test('host 模块不得借 replace 引入外部内核源码', t => {
  const f = fixture(t)
  const external = join(f.directory, 'outside-engine')
  mkdirSync(external)
  writeFileSync(join(external, 'go.mod'), `module ${engineModule}\n\ngo 1.27.1\n`)
  appendFileSync(join(f.host, 'go.mod'), `\nreplace ${engineModule} => ${JSON.stringify(external)}\n`)
  rejected(invoke(f, 'build.goEnvironment().GOWORK'), /replace|覆盖|替换|外部/)
})

test('完整仓库复制到另一绝对路径后，执行源码指纹保持一致', t => {
  const f = fixture(t)
  const original = identity(f)
  const copied = join(f.directory, 'same-checkout-different-path')
  cpSync(f.repository, copied, { recursive: true })
  const copy = { ...f, repository: copied, desktop: join(copied, 'desktop'), host: join(copied, 'desktop/services/host'), moduleURL: pathToFileURL(join(copied, 'desktop/scripts/go-env.mjs')).href }
  const relocated = identity(copy)
  assert.equal(relocated.sourceHash, original.sourceHash)
  assert.equal(relocated.engineVersion, original.engineVersion)
  assert.deepEqual(relocated.files, original.files)
})

test('仅界面、文档、测试变更和提交不改变执行身份，但仓库版本与 dirty 独立记录', t => {
  const f = fixture(t)
  const original = identity(f)
  const infoBefore = parsed(invoke(f, 'build.repositoryBuildInfo()'))
  assert.equal(infoBefore.dirty, false)
  appendFileSync(join(f.desktop, 'apps/desktop/renderer/styles.css'), 'body { background: white; }\n')
  appendFileSync(join(f.repository, 'README.md'), 'UI-only revision.\n')
  appendFileSync(join(f.repository, 'consensus_test.go'), '// Additional regression test fixture.\n')
  writeFileSync(join(f.host, 'identity_test.go'), 'package host\n')
  assert.equal(identity(f).engineVersion, original.engineVersion)
  const dirty = parsed(invoke(f, 'build.repositoryBuildInfo()'))
  assert.equal(dirty.sourceHash, original.sourceHash)
  assert.equal(dirty.dirty, true)
  const revision = commit(f, 'renderer documentation and test only')
  const infoAfter = parsed(invoke(f, 'build.repositoryBuildInfo()'))
  assert.notEqual(infoAfter.repositoryRevision, infoBefore.repositoryRevision)
  assert.equal(infoAfter.repositoryRevision, revision)
  assert.equal(infoAfter.dirty, false)
  assert.equal(infoAfter.engineVersion, original.engineVersion)
  assert.equal(identity(f).sourceHash, original.sourceHash)
})

test('内核与 host 生产 Go 文件的修改、增加和删除均改变执行身份', t => {
  const f = fixture(t)
  const initial = identity(f)
  appendFileSync(join(f.repository, 'consensus.go'), 'const Changed = true\n')
  const engineChange = identity(f)
  assert.notEqual(engineChange.engineVersion, initial.engineVersion)
  appendFileSync(join(f.host, 'host.go'), 'const HostChanged = true\n')
  const hostChange = identity(f)
  assert.notEqual(hostChange.engineVersion, engineChange.engineVersion)
  writeFileSync(join(f.host, 'untracked.go'), 'package host\nconst UntrackedProduction = true\n')
  const added = identity(f)
  assert.notEqual(added.engineVersion, hostChange.engineVersion)
  assert.ok(added.files.includes('desktop/services/host/untracked.go'))
  rmSync(join(f.repository, 'consensus.go'))
  assert.notEqual(identity(f).engineVersion, added.engineVersion)
})

test('生产 Go 文件不能通过符号链接指向未记录的外部源码', t => {
  const f = fixture(t)
  const outside = join(f.directory, 'external.go')
  writeFileSync(outside, 'package consensus\nconst Outside = true\n')
  symlinkSync(outside, join(f.repository, 'external.go'))
  rejected(invoke(f), /符号链接|外部/)
})

test('模块依赖清单与 workspace 的变更均进入执行身份', t => {
  const f = fixture(t)
  let previous = identity(f).sourceHash
  for (const file of ['go.mod', 'go.sum', 'desktop/services/host/go.mod', 'desktop/services/host/go.sum', 'go.work']) {
    appendFileSync(join(f.repository, file), file.endsWith('.sum') ? '\n' : '\n// dependency manifest changed\n')
    const next = identity(f)
    assert.notEqual(next.sourceHash, previous, `${file} 未进入执行身份`)
    assert.ok(next.files.includes(file))
    previous = next.sourceHash
  }
})

test('发布拒绝真实 dirty checkout，提交后允许且不强制旧远端提交', t => {
  const f = fixture(t)
  const release = { ...f.environment, TCVIEW_RELEASE: '1' }
  const before = parsed(invoke(f, 'build.repositoryBuildInfo()', release))
  assert.equal(before.dirty, false)
  assert.equal(before.repositoryRevision, git(f, 'rev-parse', 'HEAD'))
  appendFileSync(join(f.desktop, 'apps/desktop/renderer/styles.css'), '/* release must include every committed UI change */\n')
  rejected(invoke(f, 'build.repositoryBuildInfo()', release), /干净|dirty|未提交/)
  rejected(invoke(f, 'build.engineBuildVersion()', release), /干净|dirty|未提交/)
  commit(f, 'new clean monorepo revision')
  const after = parsed(invoke(f, 'build.repositoryBuildInfo()', release))
  assert.equal(after.dirty, false)
  assert.notEqual(after.repositoryRevision, before.repositoryRevision)
  assert.equal(after.engineVersion, before.engineVersion)
})

test('精确迁移基线保留旧执行身份，任何生产源码改变都会脱离该基线', t => {
  const f = fixture(t)
  const baseline = identity(f)
  const manifestFile = join(f.desktop, 'scripts/engine-lock.json')
  const manifest = JSON.parse(readFileSync(manifestFile, 'utf8'))
  manifest.baseline.sourceHash = baseline.sourceHash
  writeFileSync(manifestFile, JSON.stringify(manifest, null, 2) + '\n')
  const matched = identity(f)
  assert.equal(matched.baselineMatched, true)
  assert.equal(matched.engineVersion, manifest.baseline.engineVersion)
  assert.equal(matched.sourceHash, baseline.sourceHash)
  appendFileSync(join(f.host, 'host.go'), 'const ExecutionChanged = true\n')
  const changed = identity(f)
  assert.equal(changed.baselineMatched, false)
  assert.notEqual(changed.engineVersion, matched.engineVersion)
  assert.equal(changed.engineVersion, `source-sha256:${changed.sourceHash}`)
  manifest.baseline.engineVersion = 'invalid-noncanonical-version'
  writeFileSync(manifestFile, JSON.stringify(manifest))
  rejected(invoke(f), /身份清单无效|基线|manifest/)
})
