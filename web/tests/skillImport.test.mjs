import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('../src/lib/skillImport.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { prepareImports, parseFrontmatter, renameSkillContent, uniqueSkillName, validSupportingPath } = await import(
  `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`
)

const bytes = text => Buffer.byteLength(text)
const markdown = (name, options = {}) => {
  const description = options.description ?? 'Does a useful thing'
  const body = options.body ?? '# Use it\n\nDo the thing.'
  return `---\nname: ${name}\ndescription: ${description}\n---\n\n${body}\n`
}
const main = (path, content) => ({ path, size: bytes(content), markdown: content, base64: Buffer.from(content).toString('base64') })
const support = (path, content = 'support') => ({ path, size: bytes(content), base64: Buffer.from(content).toString('base64') })

test('imports multiple skill folders with relative supporting files and a reet prefix', () => {
  const result = prepareImports(
    [
      main('teams/alpha/SKILL.md', markdown('alpha')),
      support('teams/alpha/scripts/run.sh', 'echo run'),
      main('teams/beta/SKILL.md', markdown('beta')),
      support('teams/beta/references/notes.md', 'notes'),
    ],
    [],
    true,
  )

  assert.deepEqual(result.map(skill => skill.name), ['reet-alpha', 'reet-beta'])
  assert.deepEqual(Object.keys(result[0].files), ['scripts/run.sh'])
  assert.deepEqual(Object.keys(result[1].files), ['references/notes.md'])
  assert.match(result[0].content, /^name: reet-alpha$/m)
  assert.equal(result[0].problem, '')
  assert.equal(result[0].selected, true)
  assert.equal(result[0].fileCount, 2)
})

test('skills inside hidden tool folders import and metadata junk is ignored', () => {
  const result = prepareImports(
    [
      main('.claude/skills/alpha/SKILL.md', markdown('alpha')),
      support('.claude/skills/alpha/.DS_Store', 'junk'),
      support('.claude/skills/alpha/.git/config', 'junk'),
      support('.claude/skills/alpha/scripts/run.sh', 'echo run'),
    ],
    [],
    false,
  )

  assert.equal(result.length, 1)
  assert.equal(result[0].name, 'alpha')
  assert.deepEqual(Object.keys(result[0].files), ['scripts/run.sh'])
  assert.equal(result[0].problem, '')
})

test('a parent skill keeps loose support files but not a nested skill folder', () => {
  const result = prepareImports(
    [
      main('SKILL.md', markdown('parent')),
      support('scripts/parent.sh'),
      main('child/SKILL.md', markdown('child')),
      support('child/scripts/child.sh'),
    ],
    [],
    false,
  )

  assert.deepEqual(result.map(skill => skill.name), ['child', 'parent'])
  const parent = result.find(skill => skill.name === 'parent')
  const child = result.find(skill => skill.name === 'child')
  assert.deepEqual(Object.keys(parent.files), ['scripts/parent.sh'])
  assert.deepEqual(Object.keys(child.files), ['scripts/child.sh'])
})

test('imports several flat markdown skill files without doubling an existing prefix', () => {
  const result = prepareImports(
    [main('alpha.md', markdown('alpha')), main('reet-beta.md', markdown('reet-beta'))],
    [],
    true,
  )

  assert.deepEqual(result.map(skill => skill.name), ['reet-alpha', 'reet-beta'])
  assert.match(result[1].content, /^name: reet-beta$/m)
})

test('two flat files both named SKILL.md become separate skills', () => {
  const result = prepareImports([main('SKILL.md', markdown('one')), main('SKILL.md', markdown('two'))], [], true)
  assert.equal(result.length, 2)
  assert.deepEqual(result.map(skill => skill.name), ['reet-one', 'reet-two'])
})

test('name collisions with existing skills and within the batch get stable suffixes', () => {
  const result = prepareImports(
    [main('a/SKILL.md', markdown('alpha')), main('b/SKILL.md', markdown('alpha'))],
    ['reet-alpha'],
    true,
  )
  assert.deepEqual(result.map(skill => skill.name), ['reet-alpha-2', 'reet-alpha-3'])
  assert.equal(uniqueSkillName('reet-alpha', ['reet-alpha', 'reet-alpha-2']), 'reet-alpha-3')
})

test('invalid skills are explained and left unselected', () => {
  const result = prepareImports(
    [
      main('missing/SKILL.md', markdown('missing', { description: '' })),
      main('paths/SKILL.md', markdown('paths')),
      support('paths/../escape.sh'),
      main('large/SKILL.md', markdown('large')),
      support('large/blob.bin', 'x'.repeat(2 * 1024 * 1024)),
    ],
    [],
    false,
  )

  const problems = Object.fromEntries(result.map(skill => [skill.name, skill.problem]))
  assert.match(problems.missing, /description/)
  assert.match(problems.paths, /Invalid supporting file path/)
  assert.match(problems.large, /2 MiB/)
  assert.equal(result.every(skill => skill.selected === false), true)
})

test('missing frontmatter name is derived from the folder and written into the bundle', () => {
  const content = '---\ndescription: A named-by-folder skill\n---\n\nDo the thing.\n'
  const [skill] = prepareImports([main('nice-tools/SKILL.md', content)], [], false)
  assert.equal(skill.name, 'nice-tools')
  assert.match(skill.content, /^name: nice-tools$/m)
  assert.equal(skill.problem, '')
})

test('CRLF and BOM frontmatter is normalised and keeps its instructions', () => {
  const content = '\uFEFF---\r\nname: crlf-skill\r\ndescription: Works across platforms\r\n---\r\n\r\n# Steps\r\n\r\nRun it.\r\n'
  const frontmatter = parseFrontmatter(content)
  assert.ok(frontmatter)
  assert.equal(frontmatter.fields.name, 'crlf-skill')
  const renamed = renameSkillContent(content, 'reet-crlf-skill')
  assert.match(renamed, /^---\nname: reet-crlf-skill\ndescription: Works across platforms\n---\n\n# Steps/)
  assert.doesNotMatch(renamed, /\r/)
})

test('supporting paths follow the hosted skill rules', () => {
  for (const path of ['scripts/run.sh', 'references/guide.md', 'a/b/c.txt']) assert.equal(validSupportingPath(path), true)
  for (const path of ['', 'SKILL.md', '.hidden/x', 'a//b', '/abs', '../escape', 'a/../b', 'a\\b', 'a/b?.txt', 'a/b#.txt', '.well-known/x']) {
    assert.equal(validSupportingPath(path), false, path)
  }
})
