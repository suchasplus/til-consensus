import { existsSync, lstatSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { join, relative, sep } from 'node:path'

export const sourceAlgorithm = 'tcview-execution-source-sha256-v1'
const excludedDirectories = new Set(['desktop', 'vendor', 'testdata', 'node_modules', 'dist', 'release', 'references', 'out', 'tmp', 'logs', 'bin'])

// Canonical manifest: sorted repository-relative POSIX paths, NUL, SHA-256 of
// the exact file bytes, LF; prefixed by the algorithm name + LF. Test files,
// UI, documentation and build outputs are intentionally outside this identity.
export function executionSourceFingerprint(repositoryRoot) {
  const files = []
  function visit(directory, moduleRoot) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      if (entry.name.startsWith('.') || entry.name.startsWith('_')) continue
      const absolute = join(directory, entry.name)
      if (entry.isSymbolicLink() && (entry.name.endsWith('.go') || (existsSync(absolute) && statSync(absolute).isDirectory() && !excludedDirectories.has(entry.name)))) {
        throw new Error(`执行源码不允许符号链接：${relative(repositoryRoot, absolute)}`)
      }
      if (entry.isDirectory()) {
        if (excludedDirectories.has(entry.name) || (absolute !== moduleRoot && existsSync(join(absolute, 'go.mod')))) continue
        visit(absolute, moduleRoot)
      } else if (entry.name.endsWith('.go') && !entry.name.endsWith('_test.go')) {
        if (entry.isFile()) files.push(relative(repositoryRoot, absolute).split(sep).join('/'))
      }
    }
  }
  visit(repositoryRoot, repositoryRoot)
  visit(join(repositoryRoot, 'desktop/services/host'), join(repositoryRoot, 'desktop/services/host'))
  for (const file of ['go.mod', 'go.sum', 'go.work', 'desktop/services/host/go.mod', 'desktop/services/host/go.sum']) {
    const absolute = join(repositoryRoot, file)
    if (!existsSync(absolute)) {
      if (file.endsWith('go.sum')) continue
      throw new Error(`执行源码身份缺少 ${file}`)
    }
    if (!lstatSync(absolute).isFile()) throw new Error(`执行源码元数据必须为普通文件：${file}`)
    files.push(file)
  }
  files.sort()
  const hash = createHash('sha256').update(`${sourceAlgorithm}\n`)
  for (const file of files) {
    const contentHash = createHash('sha256').update(readFileSync(join(repositoryRoot, file))).digest('hex')
    hash.update(`${file}\0${contentHash}\n`)
  }
  return { algorithm: sourceAlgorithm, sourceHash: hash.digest('hex'), files }
}
