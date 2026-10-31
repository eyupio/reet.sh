import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./skillPrompt.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { insertSkillPrompt } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const RUN = 'Run `npx skills use "https://example.test/skill" --skill "demo"` and follow it.'

test('a skill prompt fills an empty prompt and puts the caret after it', () => {
  const out = insertSkillPrompt('', RUN, 0, 0)
  assert.equal(out.text, RUN)
  assert.equal(out.caret, RUN.length)
})

test('a skill prompt after existing text starts on its own paragraph', () => {
  const out = insertSkillPrompt('Make the page calmer', RUN)
  assert.equal(out.text, `Make the page calmer\n\n${RUN}`)
  assert.equal(out.caret, out.text.length)
})

test('an existing paragraph break is not doubled', () => {
  assert.equal(insertSkillPrompt('First\n\n', RUN, 7, 7).text, `First\n\n${RUN}`)
  assert.equal(insertSkillPrompt('First\n', RUN, 6, 6).text, `First\n\n${RUN}`)
  assert.equal(insertSkillPrompt(`First\n\n${RUN}\n\n`, 'More', 7, 7).text, `First\n\nMore\n\n${RUN}\n\n`)
})

test('inserting at a caret keeps the text after it and separates the paragraphs', () => {
  const out = insertSkillPrompt('Before after', RUN, 6, 6)
  assert.equal(out.text, `Before\n\n${RUN}\n\n after`)
  assert.equal(out.caret, `Before\n\n${RUN}`.length)
})

test('a selection is replaced without losing either side', () => {
  const out = insertSkillPrompt('Keep this and that', RUN, 5, 14)
  assert.equal(out.text, `Keep \n\n${RUN}\n\nthat`)
})

test('a missing caret falls back to the end of the prompt, and out-of-range values clamp', () => {
  const missing = insertSkillPrompt('Prompt', RUN, Number.NaN, Number.NaN)
  assert.equal(missing.text, `Prompt\n\n${RUN}`)
  assert.equal(missing.caret, missing.text.length)
  assert.equal(insertSkillPrompt('Prompt', RUN, -4, 0).text, `${RUN}\n\nPrompt`)
  assert.equal(insertSkillPrompt('Prompt', RUN, 500, 900).text, `Prompt\n\n${RUN}`)
})
