import { execFileSync } from 'node:child_process'
import { hostDirectory, goEnvironment } from './go-env.mjs'
execFileSync('go', ['test', '-race', './...'], { cwd: hostDirectory, env: goEnvironment(), stdio: 'inherit' })

