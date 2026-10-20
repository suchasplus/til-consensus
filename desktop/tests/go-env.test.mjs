import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, readFileSync, realpathSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, delimiter } from 'node:path'
import { spawnSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'
import { engineCommit, engineModuleVersion } from '../scripts/go-env.mjs'

const scriptSource = readFileSync(new URL('../scripts/go-env.mjs', import.meta.url), 'utf8')

function fixture(t, upgraded = true) {
  const directory = realpathSync(mkdtempSync(join(tmpdir(), 'tcview-go-env-test-')))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const source = join(directory, 'til-consensus')
  const bin = join(directory, 'bin')
  const project = join(directory, 'tcview')
  mkdirSync(join(project, 'scripts'), { recursive: true })
  mkdirSync(join(project, 'services/host'), { recursive: true })
  writeFileSync(join(project, 'scripts/go-env.mjs'), scriptSource)
  writeFileSync(join(project, 'scripts/engine-lock.json'), JSON.stringify({ module: 'github.com/suchasplus/til-consensus', moduleVersion: engineModuleVersion, commit: engineCommit }))
  writeFileSync(join(project, 'services/host/go.mod'), `module fixture/tcview\n\ngo 1.27.1\n\nrequire github.com/suchasplus/til-consensus ${engineModuleVersion}\n`)
  mkdirSync(source, { recursive: true })
  mkdirSync(bin)
  writeFileSync(join(source, 'go.mod'), 'module github.com/suchasplus/til-consensus\n\ngo 1.27.1\n')
  if (upgraded) {
    mkdirSync(join(source, 'runtime/cli'), { recursive: true })
    mkdirSync(join(source, 'config'))
    writeFileSync(join(source, 'runtime/cli/runner.go'), 'package cli\ntype RunTaskResult struct { RawOutput string }\nfunc NewRunnerWithOptions() {}\n')
    writeFileSync(join(source, 'runtime/cli/progress.go'), 'package cli\ntype RunnerOptions struct {}\ntype CLIEvent struct {}\n')
    writeFileSync(join(source, 'config/types.go'), 'package config\ntype ProviderConfig struct { Workdir string; PermissionMode string; OutputFormat string }\n')
    writeFileSync(join(source, 'other.go'), 'package consensus\n')
  }
  // Git metadata is stubbed only inside this subprocess fixture; no repository
  // checkout or model process is mutated by these build-resolution tests.
  writeFileSync(join(bin, 'git'), `#!/bin/sh
case "$3" in
  rev-parse) printf '%s\\n' "\${TCVIEW_TEST_GIT_REVISION:-${engineCommit}}" ;;
  status) printf '%s' "$TCVIEW_TEST_GIT_STATUS" ;;
  ls-files) printf '%s\\000' go.mod runtime/cli/runner.go runtime/cli/progress.go config/types.go other.go ;;
  *) exit 2 ;;
esac
`, { mode: 0o700 })
  return { directory, source, project, moduleURL: pathToFileURL(join(project, 'scripts/go-env.mjs')).href, environment: { ...process.env, TIL_CONSENSUS_DIR: source, PATH: `${bin}${delimiter}${process.env.PATH}`, TCVIEW_RELEASE: '', TCVIEW_TEST_GIT_STATUS: ' M runtime/cli/runner.go', TCVIEW_TEST_GIT_REVISION: engineCommit } }
}

function invoke(fixture, fn, environment = fixture.environment) {
  const expression = fn === 'goEnvironment' ? '{ GOWORK: goEnvironment().GOWORK }' : 'engineBuildVersion()'
  return spawnSync(process.execPath, ['--input-type=module', '-e', `import { ${fn} } from ${JSON.stringify(fixture.moduleURL)}; try { console.log(JSON.stringify(${expression})) } catch (error) { console.error(error.message); process.exitCode = 1 }`], { env: environment, encoding: 'utf8' })
}

function parsed(result) {
  assert.equal(result.status, 0, result.stderr)
  return JSON.parse(result.stdout.trim().split('\n').at(-1))
}

test('默认不依赖本地 checkout，并忽略环境和自动发现的 go.work', t => {
  const f = fixture(t)
  const environment = { ...f.environment, HOME: f.directory, GOWORK: join(f.project, 'go.work') }
  delete environment.TIL_CONSENSUS_DIR
  writeFileSync(environment.GOWORK, 'invalid workspace that must never be read')
  // A pre-existing ~/agentic/til-consensus must not become an implicit override.
  mkdirSync(join(f.directory, 'agentic/til-consensus'), { recursive: true })
  writeFileSync(join(f.directory, 'agentic/til-consensus/go.mod'), 'module wrong/checkout\n')
  assert.deepEqual(parsed(invoke(f, 'goEnvironment', environment)), { GOWORK: 'off' })
  assert.equal(parsed(invoke(f, 'engineBuildVersion', environment)), engineCommit)
})

test('显式本地路径错误不会静默切回远端', t => {
  const f = fixture(t)
  for (const fn of ['goEnvironment', 'engineBuildVersion']) {
    const result = invoke(f, fn, { ...f.environment, TIL_CONSENSUS_DIR: join(f.directory, 'missing') })
    assert.equal(result.status, 1)
    assert.match(result.stderr, /不存在或缺少 go.mod/)
    assert.match(result.stderr, /TIL_CONSENSUS_DIR/)
    assert.doesNotMatch(result.stdout, /"off"/)
  }
})

test('版本锁、go.mod 和伪版本提交必须一致', t => {
  const f = fixture(t)
  const environment = { ...f.environment }
  delete environment.TIL_CONSENSUS_DIR
  writeFileSync(join(f.project, 'services/host/go.mod'), 'module fixture/tcview\nrequire github.com/suchasplus/til-consensus v0.0.0-20000101000000-000000000000\n')
  const result = invoke(f, 'goEnvironment', environment)
  assert.equal(result.status, 1)
  assert.match(result.stderr, /内核版本锁不一致/)
  writeFileSync(join(f.project, 'scripts/engine-lock.json'), JSON.stringify({ module: 'github.com/suchasplus/til-consensus', moduleVersion: 'v0.0.0-20000101000000-111111111111', commit: '2'.repeat(40) }))
  const mismatchedCommit = invoke(f, 'engineBuildVersion', environment)
  assert.equal(mismatchedCommit.status, 1)
  assert.match(mismatchedCommit.stderr, /提交后缀与 commit 不符/)
})

test('显式本地 checkout 仍要求所需新接口', t => {
  const f = fixture(t, false)
  const result = invoke(f, 'engineBuildVersion', { ...f.environment, TCVIEW_TEST_GIT_STATUS: '' })
  assert.equal(result.status, 1)
  assert.match(result.stderr, /尚未包含所需升级接口/)
  assert.match(result.stderr, /NewRunnerWithOptions/)
})

test('默认发布使用远端固定模块，本地发布拒绝 dirty 或错误提交', t => {
  const f = fixture(t)
  const remoteEnvironment = { ...f.environment, TCVIEW_RELEASE: '1', GOWORK: '/must/be/ignored' }
  delete remoteEnvironment.TIL_CONSENSUS_DIR
  assert.equal(parsed(invoke(f, 'engineBuildVersion', remoteEnvironment)), engineCommit)
  assert.deepEqual(parsed(invoke(f, 'goEnvironment', remoteEnvironment)), { GOWORK: 'off' })
  const dirty = invoke(f, 'engineBuildVersion', { ...f.environment, TCVIEW_RELEASE: '1' })
  assert.equal(dirty.status, 1)
  assert.match(dirty.stderr, /干净的固定提交/)
  const wrongPin = invoke(f, 'goEnvironment', { ...f.environment, TCVIEW_TEST_GIT_REVISION: '0'.repeat(40) })
  assert.equal(wrongPin.status, 1)
  assert.match(wrongPin.stderr, /基线提交不符/)
  assert.equal(parsed(invoke(f, 'engineBuildVersion', { ...f.environment, TCVIEW_RELEASE: '1', TCVIEW_TEST_GIT_STATUS: '' })), engineCommit)
})

test('显式覆盖使用独立 workspace，不覆盖项目原有 go.work', t => {
  const f = fixture(t)
  const oldWorkspace = join(f.project, 'go.work')
  writeFileSync(oldWorkspace, 'existing project workspace')
  const result = parsed(invoke(f, 'goEnvironment', { ...f.environment, GOWORK: oldWorkspace }))
  assert.equal(result.GOWORK, join(f.project, '.tcview/local-engine/go.work'))
  const generated = readFileSync(result.GOWORK, 'utf8')
  assert.ok(generated.includes(JSON.stringify(f.source)))
  assert.ok(generated.includes(JSON.stringify(join(f.project, 'services/host'))))
  assert.equal(readFileSync(oldWorkspace, 'utf8'), 'existing project workspace')
})

test('本地开发指纹稳定且涵盖源码修改和删除', t => {
  const f = fixture(t)
  const version = () => {
    const result = invoke(f, 'engineBuildVersion')
    assert.equal(result.status, 0, result.stderr)
    return JSON.parse(result.stdout.trim().split('\n').at(-1))
  }
  const original = version()
  assert.match(original, new RegExp(`^${engineCommit}\\+local\\.[a-f0-9]{16}$`))
  assert.equal(version(), original)
  writeFileSync(join(f.source, 'other.go'), 'package consensus\nconst Changed = true\n')
  const changed = version()
  assert.notEqual(changed, original)
  rmSync(join(f.source, 'other.go'))
  assert.notEqual(version(), changed)
})
