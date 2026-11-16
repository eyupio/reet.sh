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

const installSource = readFileSync(new URL('./install.ts', import.meta.url), 'utf8')
const installCompiled = ts.transpileModule(installSource, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { useSkillPrompt, installCommand } = await import(`data:text/javascript;base64,${Buffer.from(installCompiled).toString('base64')}`)

const skill = over => ({ id: 'local/unslop', name: 'unslop', slug: 'unslop', source: 'local', sourceType: 'local', url: '', ...over })

test('a skill with no source is used by name, with reet as its source', () => {
  assert.equal(useSkillPrompt(skill()), 'Use the unslop skill.')
  assert.equal(useSkillPrompt(skill({ name: '', slug: 'from-the-slug' })), 'Use the from-the-slug skill.')
})

test('a skill with a description carries it, on one line, so the agent gets its context', () => {
  assert.equal(useSkillPrompt(skill({ description: 'Manage the\n  full PR lifecycle' })), 'Use the unslop skill: Manage the full PR lifecycle.')
  assert.equal(useSkillPrompt(skill({ description: 'Already ends.' })), 'Use the unslop skill: Already ends.')
  assert.ok(useSkillPrompt(skill({ description: 'word '.repeat(100) })).endsWith('…'))
})

test('a skill with an install source keeps the fetch-and-run prompt', () => {
  assert.match(useSkillPrompt(skill({ source: 'demo/toolkit', sourceType: 'github' })), /^Run `npx skills use "https:\/\/github\.com\/demo\/toolkit" --skill "unslop"`/)
  assert.match(useSkillPrompt(skill({ source: '', sourceType: 'hosted', url: 'https://reet.test/demo' })), /^Run `npx skills use "https:\/\/reet\.test\/demo"/)
})

test('a skill with neither a source nor a name cannot be used', () => {
  assert.equal(useSkillPrompt(skill({ name: '', slug: '' })), null)
})

test('a copied install command offers the skill\'s own name when it is not its folder name', () => {
  // The skills CLI matches --skill on the name in SKILL.md, so the folder name alone finds nothing.
  assert.equal(installCommand(skill({ name: 'SVG Logo Designer', slug: 'svg-logo-designer', source: 'rknall/claude-skills', sourceType: 'github' })), "npx skills add https://github.com/rknall/claude-skills --skill svg-logo-designer 'SVG Logo Designer'")
  assert.equal(installCommand(skill({ source: 'demo/toolkit', sourceType: 'github' })), 'npx skills add https://github.com/demo/toolkit --skill unslop')
  assert.equal(installCommand(skill({ name: 'Unslop', source: 'demo/toolkit', sourceType: 'github' })), 'npx skills add https://github.com/demo/toolkit --skill unslop')
  assert.equal(installCommand(skill({ name: "It's --all", slug: 'its', source: 'demo/toolkit', sourceType: 'github' })), "npx skills add https://github.com/demo/toolkit --skill its 'It'\\''s --all'")
  assert.equal(installCommand(skill({ name: '--all', source: 'demo/toolkit', sourceType: 'github' })), 'npx skills add https://github.com/demo/toolkit --skill unslop')
})

test('the use prompt names a skill the way the skills CLI finds it', () => {
  // `skills use` takes one selector and matches the skill's own name; the folder name finds nothing
  // for "SVG Logo Designer" on skills 1.7.4, and nothing for "UI/UX Design Review" on any version tried.
  const run = over => useSkillPrompt(skill({ source: 'rknall/claude-skills', sourceType: 'github', ...over })).split('`')[1]
  assert.equal(run({ name: 'SVG Logo Designer', slug: 'svg-logo-designer' }), 'npx skills use "https://github.com/rknall/claude-skills" --skill "SVG Logo Designer"')
  assert.equal(run({ name: 'UI/UX Design Review', slug: 'uiux-design-review' }), 'npx skills use "https://github.com/rknall/claude-skills" --skill "UI/UX Design Review"')
  assert.equal(run({}), 'npx skills use "https://github.com/rknall/claude-skills" --skill "unslop"')
  assert.equal(run({ name: 'Unslop' }), 'npx skills use "https://github.com/rknall/claude-skills" --skill "unslop"')
  // A name that could end the quotes or the code span, or read as a flag, is not put in a command.
  for (const name of ['Say "hi"', 'Tick `this`', 'Cost $HOME', 'Back\\slash', '--all', '*', '']) {
    assert.equal(run({ name }), 'npx skills use "https://github.com/rknall/claude-skills" --skill "unslop"', name)
  }
})
