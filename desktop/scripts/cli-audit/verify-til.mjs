import { execFileSync } from 'node:child_process'
import { join } from 'node:path'
import { goEnvironment, hostDirectory } from '../go-env.mjs'
import { root, auditDir, probe, validate, save } from './common.mjs'

const binary = join(auditDir, 'til-native')
execFileSync('go', ['build', '-o', binary, join(root, 'scripts/cli-audit/til-native/main.go')], { cwd: hostDirectory, env: goEnvironment(), stdio: 'inherit' })
const option = key => { const index=process.argv.indexOf(key); return index<0 ? undefined : process.argv[index+1] }
const providers = (option('--providers') ?? 'codex,claude,grok,antigravity').split(',')
if (providers.some(value=>!['codex','claude','grok','antigravity'].includes(value))) throw new Error('无效 provider')
const label = option('--label') ?? 'live'
if (!/^[\w-]+$/.test(label)) throw new Error('无效 label')
const results = await Promise.all(providers.map(async provider => {
  const name=`til-runner-${label}`
  const directory = join(auditDir, provider, name)
  const run = await probe(provider, name, binary, ['--provider',provider,'--prompt',join(auditDir,'prompt.txt'),'--schema',join(auditDir,'schema.json'),'--output',directory], { timeoutMs: 150_000 })
  let result
  try { result = JSON.parse(run.stdout) } catch {}
  return { provider, cliVersion:result?.cliVersion, exitCode: run.exitCode, elapsedMs: run.elapsedMs, ok: result?.ok ?? false, validation: validate(result?.output), eventCount: result?.eventCount, error: result?.error, artifact: directory }
}))
save(join(auditDir,`til-runner-${label}-summary.json`),results)
console.log(JSON.stringify(results,null,2))
if (results.some(result => !result.ok || !result.validation.ok)) process.exitCode = 1
