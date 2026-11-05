// Fails when the built page's gzipped JS + CSS exceeds the budget.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'

const BUDGET = 150 * 1024
const dist = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'web', 'dist')

function walk(dir) {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : [p]
  })
}

let total = 0
for (const f of walk(dist).filter((f) => /\.(js|css)$/.test(f))) {
  const n = gzipSync(readFileSync(f), { level: 9 }).length
  total += n
  console.log(`${(n / 1024).toFixed(1).padStart(7)} KB  ${f.slice(dist.length + 1)}`)
}
console.log(`${(total / 1024).toFixed(1).padStart(7)} KB  total (budget ${BUDGET / 1024} KB)`)
if (total > BUDGET) {
  console.error('over budget')
  process.exit(1)
}
