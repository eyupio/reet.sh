import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./agentLaunch.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const m = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const { TOOLS, MAX_PROMPT_BYTES, modelSuggestions, validateModel, validatePrompt, folderChoices, launchSentence, refusalText, titleFor } = m

test('tools are the four built-in ids', () => {
  assert.deepEqual(TOOLS, [{ id: 'claude', name: 'Claude Code' }, { id: 'codex', name: 'Codex' }, { id: 'opencode', name: 'OpenCode' }, { id: 'gemini', name: 'Gemini CLI' }])
})

test('model suggestions are short hints', () => {
  assert.deepEqual(modelSuggestions('claude'), ['sonnet', 'opus', 'haiku'])
  assert.deepEqual(modelSuggestions('codex'), ['gpt-5-codex'])
  assert.deepEqual(modelSuggestions('opencode'), [])
  assert.deepEqual(modelSuggestions('gemini'), ['gemini-2.5-pro', 'gemini-2.5-flash'])
  assert.deepEqual(modelSuggestions('nope'), [])
  for (const t of TOOLS) for (const s of modelSuggestions(t.id)) assert.equal(validateModel(s), null)
})

test('model rule', () => {
  assert.equal(validateModel(''), null)
  assert.equal(validateModel('  '), null)
  assert.equal(validateModel('claude-opus-4.1'), null)
  assert.equal(validateModel('a/b:c@d+e_f-g'), null)
  assert.equal(validateModel('a'.repeat(100)), null)
  const bad = 'That model name isn\'t one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit.'
  assert.equal(validateModel('a'.repeat(101)), bad)
  assert.equal(validateModel('-x'), bad)
  assert.equal(validateModel('a b'), bad)
  assert.equal(validateModel('é'), bad)
  assert.equal(validateModel('a\nb'), bad)
})

test('prompt rule', () => {
  assert.equal(validatePrompt('claude', ''), null)
  assert.equal(validatePrompt('claude', 'fix the build'), null)
  assert.equal(validatePrompt('claude', 'fix\nthe build'), null)
  assert.equal(validatePrompt('claude', 'fix it;'), null)
  assert.equal(validatePrompt('opencode', 'purge'), null)
  assert.equal(validatePrompt('gemini', 'purge'), null)
  const two = 'Write the opening prompt as at least two words.'
  assert.equal(validatePrompt('claude', 'purge'), two)
  assert.equal(validatePrompt('codex', 'update'), two)
  assert.equal(MAX_PROMPT_BYTES, 2000)
  const long = 'That prompt is too long. Use at most 2000 bytes; accented letters and emoji take more than one.'
  assert.equal(validatePrompt('claude', 'a '.repeat(1000)), null)
  assert.equal(validatePrompt('claude', 'a '.repeat(1000) + 'b'), long)
  // 3-byte characters at the boundary: 666*3 = 1998, plus a space = 1999, plus "a" = 2000, plus one more = 2001
  const han = '中'.repeat(666)
  assert.equal(validatePrompt('claude', han + ' a'), null)
  assert.equal(validatePrompt('claude', han + ' ab'), long)
  assert.equal(validatePrompt('claude', han + '中' + ' '), long)
  assert.equal(validatePrompt('claude', '-a b'), 'Start the prompt with a word, not a dash.')
  assert.equal(validatePrompt('gemini', '-x'), 'Start the prompt with a word, not a dash.')
  const ctl = 'Use a prompt without control characters other than line breaks.'
  assert.equal(validatePrompt('claude', 'a\tb'), ctl)
  assert.equal(validatePrompt('claude', 'a b\u0000'), ctl)
  assert.equal(validatePrompt('claude', 'a b\u007f'), ctl)
  assert.equal(validatePrompt('claude', 'a b\u0085'), ctl)
  assert.equal(validatePrompt('claude', 'a b\r'), ctl)
  assert.equal(validatePrompt('claude', 'a\u0000'), ctl)
  assert.equal(validatePrompt('claude', 'a b\ud800'), 'Use a prompt that is valid text.')
})

test('folder choices: home first, project folders, deduplicated, absolute only', () => {
  const g = (project, host = 'studio') => ({ id: project, host, agent: 'claude', project, skills: [] })
  const snapshot = [
    { host: 'studio', groups: [g(''), g('/work/app'), g('/work/app'), g('relative/dir'), g('/work/api/'), g('/work/api')], skillCount: 0 },
    { host: 'Studio', groups: [g('/work/web')], skillCount: 0 },
    { host: 'other', groups: [g('/elsewhere')], skillCount: 0 },
  ]
  assert.deepEqual(folderChoices({ host: 'studio' }, snapshot), [
    { path: '', label: 'Home folder' },
    { path: '/work/app', label: '/work/app' },
    { path: '/work/api', label: '/work/api' },
    { path: '/work/web', label: '/work/web' },
  ])
  assert.deepEqual(folderChoices({ host: 'studio' }, undefined), [{ path: '', label: 'Home folder' }])
  assert.deepEqual(folderChoices(undefined, snapshot), [{ path: '', label: 'Home folder' }])
})

test('launch sentence and title', () => {
  assert.equal(launchSentence('claude', '', 'studio', false), 'Start Claude Code in the home folder on studio.')
  assert.equal(launchSentence('codex', '/work/app', 'studio', true), 'Start Codex in /work/app on studio with your opening prompt.')
  assert.equal(titleFor('claude', ''), 'Claude Code')
  assert.equal(titleFor('gemini', '/work/app'), 'Gemini CLI · app')
  assert.equal(titleFor('gemini', '/work/app/'), 'Gemini CLI · app')
  assert.equal(titleFor('gemini', '/'), 'Gemini CLI')
  assert.ok(Array.from(titleFor('claude', '/x/' + 'y'.repeat(200))).length <= 80)
  assert.equal(titleFor('claude', '/x/a\nb'), 'Claude Code')
})

test('refusal sentences', () => {
  assert.equal(refusalText(409, 'launch_unsupported'), 'Update reet on that machine to start agents.')
  assert.equal(refusalText(422, 'tool_not_installed'), "That agent isn't installed on that machine.")
  assert.equal(refusalText(422, 'folder_unavailable'), "That folder isn't available on that machine.")
  assert.equal(refusalText(422, 'prompt_unsupported'), 'Write the opening prompt as at least two words.')
  assert.equal(refusalText(409, ''), 'That request changed. Start it again.')
  assert.equal(refusalText(409, undefined), 'That request changed. Start it again.')
  assert.equal(refusalText(409, '', 'terminal session limit reached'), 'That machine already has the most sessions it allows.')
  assert.equal(refusalText(409, 'capacity'), 'That machine already has the most sessions it allows.')
  assert.equal(refusalText(503, ''), 'That machine is reconnecting. Try again in a moment.')
  assert.equal(refusalText(403, '', 'remote terminals are disabled by the server; monitoring and local shells remain available'), 'Browser terminals are switched off on that machine.')
  assert.equal(refusalText(403, 'terminal_disabled'), 'Browser terminals are switched off on that machine.')
  assert.equal(refusalText(403, '', 'terminal access denied'), 'Something changed. Check again.')
  assert.equal(refusalText(500, ''), 'Something changed. Check again.')
  assert.equal(refusalText(0, 'weird'), 'Something changed. Check again.')
  assert.equal(refusalText(422, 'other'), 'Something changed. Check again.')
})

test('no banned words in any sentence', () => {
  const all = [refusalText(409, 'launch_unsupported'), refusalText(503), refusalText(500), launchSentence('claude', '', 'm', true), validateModel('-')]
  for (const s of all) assert.doesNotMatch(s, /verified|healthy/i)
})
