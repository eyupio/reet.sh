// Runs every *.test.mjs under src/ and tests/, one at a time with plain node, and stops at the first that fails.
// Not `node --test`: under it some of these files behave differently (timers, files left waiting).
import { spawnSync } from 'node:child_process'
import { readdirSync } from 'node:fs'

const files = ['src', 'tests']
  .flatMap(dir => readdirSync(dir, { recursive: true }).map(name => `${dir}/${String(name).replaceAll('\\', '/')}`))
  .filter(name => name.endsWith('.test.mjs'))
  .sort()
if (!files.length) { console.error('No test files found. Run this from web/.'); process.exit(1) }
for (const file of files) {
  const { status } = spawnSync(process.execPath, [file], { stdio: 'inherit' })
  if (status !== 0) { console.error(`\n${file} failed.`); process.exit(status ?? 1) }
}
console.log(`\n${files.length} test files passed.`)
