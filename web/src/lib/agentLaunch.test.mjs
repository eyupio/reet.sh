import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const source = compile('./agentLaunch.ts').replace("from './catalogue'", `from '${asURL(compile('./catalogue.ts'))}'`)
const m = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const { cleanModel, validateDir, TOOLS, MAX_PROMPT_BYTES, modelSuggestions, validateModel, validatePrompt, folderChoices, launchSentence, refusalText, titleFor, notStartedText } = m

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

const ch = (...n) => String.fromCodePoint(...n)
const BAD_CHARS = { newline: '\n', tab: '\t', del: ch(0x7f), c1: ch(0x85), bidiOverride: ch(0x202e), isolate: ch(0x2066), zeroWidth: ch(0x200b), feff: ch(0xfeff), softHyphen: ch(0xad), wordJoiner: ch(0x2060), arabicMark: ch(0x61c), ls: ch(0x2028), ps: ch(0x2029) }

test('prompts: separators and the two-word rule', () => {
  const ctl = 'Use a prompt without control characters other than line breaks.'
  assert.equal(validatePrompt('claude', 'a b' + ch(0x2028)), ctl)
  assert.equal(validatePrompt('claude', 'a b' + ch(0x2029)), ctl)
  assert.equal(validatePrompt('gemini', 'a' + ch(0x2028) + 'b'), ctl)
  const two = 'Write the opening prompt as at least two words.'
  assert.equal(validatePrompt('claude', 'a\nb'), null)
  assert.equal(validatePrompt('codex', 'a b'), null)
  assert.equal(validatePrompt('claude', 'a b'), null)
  assert.equal(validatePrompt('codex', 'a\nb'), null)
  assert.equal(validatePrompt('claude', 'word'), two)
  assert.equal(validatePrompt('codex', 'word'), two)
  assert.equal(validatePrompt('opencode', 'word'), null)
  assert.equal(validatePrompt('gemini', 'word'), null)
})

test('cleanModel returns the trimmed value', () => {
  assert.equal(cleanModel('  opus '), 'opus')
  assert.equal(cleanModel(''), '')
  assert.equal(validateModel(' opus '), null)
})

test('folder rule', () => {
  assert.equal(validateDir('/work/app'), null)
  assert.equal(validateDir('/'), null)
  const abs = 'Use a full path that starts with /.'
  assert.equal(validateDir('work/app'), abs)
  assert.equal(validateDir(''), abs)
  assert.equal(validateDir('/' + 'a'.repeat(4095)), null)
  assert.equal(validateDir('/' + 'a'.repeat(4096)), 'That folder path is too long. Use at most 4096 bytes; accented letters and emoji take more than one.')
  assert.equal(validateDir('/' + ch(0x4e2d).repeat(1365)), null)
  assert.equal(validateDir('/' + ch(0x4e2d).repeat(1366)), 'That folder path is too long. Use at most 4096 bytes; accented letters and emoji take more than one.')
  assert.equal(validateDir('/a' + ch(0xd800)), 'Use a folder path that is valid text.')
  for (const [name, c] of Object.entries(BAD_CHARS)) assert.equal(validateDir('/a' + c + 'b'), 'Use a folder path without control or invisible characters.', name)
})

test('folder choices: home first, project folders, deduplicated, screened', () => {
  const g = (project, host = 'studio') => ({ id: project, host, agent: 'claude', project, skills: [] })
  const snapshot = [
    { host: 'studio', groups: [g(''), g('/work/app'), g('/work/app'), g('relative/dir'), g('/work/api/'), g('/work/api'), g('/a/./b'), g('//x'), g('/a/../b'), g('/n\nl'), g('/bidi' + ch(0x202e)), g('/' + 'a'.repeat(4096)), g('/lone' + ch(0xd800))], skillCount: 0 },
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

test('launch sentence follows the spec', () => {
  const nothing = 'Nothing runs until you press Start.'
  const kept = 'The prompt is kept with the session record and is visible to other users of that machine in its process list.'
  assert.equal(launchSentence('claude', '', 'studio', false), 'Starts Claude Code in the home folder on studio. ' + nothing)
  assert.equal(launchSentence('codex', '/work/app', 'studio', false), 'Starts Codex in /work/app on studio. ' + nothing)
  assert.equal(launchSentence('codex', '/work/app', 'studio', true), 'Starts Codex in /work/app on studio. ' + nothing + ' ' + kept)
})

test('titles', () => {
  assert.equal(titleFor('claude', ''), 'Claude Code')
  assert.equal(titleFor('gemini', '/work/app'), 'Gemini CLI \u00b7 app'.replace('\\u00b7', ch(0xb7)))
  assert.equal(titleFor('gemini', '/work/app/'), 'Gemini CLI ' + ch(0xb7) + ' app')
  assert.equal(titleFor('gemini', '/'), 'Gemini CLI')
  assert.ok(Array.from(titleFor('claude', '/x/' + 'y'.repeat(200))).length <= 80)
  for (const [name, c] of Object.entries(BAD_CHARS)) assert.equal(titleFor('claude', '/x/a' + c + 'b'), 'Claude Code', name)
})

test('refusal sentences', () => {
  assert.equal(refusalText(409, 'launch_unsupported'), 'Update reet on that machine to start agents.')
  assert.equal(refusalText(422, 'tool_not_installed'), "That agent isn't installed on that machine.")
  assert.equal(refusalText(422, 'folder_unavailable'), "That folder isn't available on that machine.")
  assert.equal(refusalText(422, 'prompt_unsupported'), 'Write the opening prompt as at least two words.')
  assert.equal(refusalText(409, undefined, 'terminal session limit reached'), 'The terminal session limit has been reached. End a session and try again.')
  assert.equal(refusalText(409, '', 'terminal operation conflicts with current state'), 'That request changed. Start it again.')
  assert.equal(refusalText(409, undefined, undefined), 'That request changed. Start it again.')
  assert.equal(refusalText(503, '', 'terminal machine is reconnecting; retry after it reconnects'), 'That machine is reconnecting. Try again in a moment.')
  assert.equal(refusalText(403, '', 'remote terminals are disabled by the server; monitoring and local shells remain available'), 'Browser terminals are switched off on this server.')
  assert.equal(refusalText(403, '', 'terminal access denied'), 'Something changed. Check again.')
  assert.equal(refusalText(403, 'terminal_disabled'), 'Something changed. Check again.')
  assert.equal(refusalText(409, 'capacity'), 'Something changed. Check again.')
  assert.equal(refusalText(500, ''), 'Something changed. Check again.')
  assert.equal(refusalText(0, 'weird'), 'Something changed. Check again.')
  assert.equal(refusalText(422, 'other'), 'Something changed. Check again.')
})

test('no banned words in any sentence', () => {
  const all = [refusalText(409, 'launch_unsupported'), refusalText(503), refusalText(500), launchSentence('claude', '', 'm', true), validateModel('-'), validateDir('x')]
  for (const s of all) assert.doesNotMatch(s, /verified|healthy/i)
})

test('toolName gives the display name, and the id itself for anything unknown', () => {
  assert.equal(m.toolName('claude'), 'Claude Code')
  assert.equal(m.toolName('gemini'), 'Gemini CLI')
  assert.equal(m.toolName('some-future-agent'), 'some-future-agent')
  assert.equal(m.toolName(''), 'an agent')
  assert.equal(m.toolName(undefined), 'an agent')
})

test('toolChoices offers what the machine reports, and lists the rest as not installed', () => {
  const caps = { launch: true, tools: ['codex', 'claude', 'pi'] }
  assert.deepEqual(m.toolChoices(caps), [
    { id: 'claude', name: 'Claude Code', available: true, reason: '' },
    { id: 'codex', name: 'Codex', available: true, reason: '' },
    { id: 'opencode', name: 'OpenCode', available: false, reason: 'Not installed on this machine.' },
    { id: 'gemini', name: 'Gemini CLI', available: false, reason: 'Not installed on this machine.' },
  ])
})

test('toolChoices offers nothing on a machine that cannot take launch requests', () => {
  for (const caps of [undefined, {}, { launch: false, tools: ['claude'] }, { tools: ['claude'] }]) {
    const out = m.toolChoices(caps)
    assert.equal(out.length, 4)
    assert.ok(out.every(t => !t.available && t.reason === 'Update reet on this machine to start agents.'))
  }
  assert.ok(m.toolChoices({ launch: true }).every(t => !t.available && t.reason === 'Not installed on this machine.'))
  assert.ok(m.toolChoices({ launch: true, tools: 'claude' }).every(t => !t.available))
})

test('every surface names a tool from the one catalogue, so Cline reads Cline', () => {
  assert.equal(m.toolName('cline'), 'Cline')
  assert.equal(m.toolName('pi'), 'Pi')
  assert.equal(m.launchActivity({ tool: 'cline', host: 'studio', dir: '/w' }), 'Started Cline in /w on studio')
})

test('launchActivity reads an audit detail as one plain line and never needs the prompt', () => {
  assert.equal(m.launchActivity({ tool: 'claude', host: 'studio', dir: '/home/me/work/site' }), 'Started Claude Code in /home/me/work/site on studio')
  assert.equal(m.launchActivity({ tool: 'codex', host: 'studio', dir: '', model: 'gpt-5-codex' }), 'Started Codex with gpt-5-codex in the home folder on studio')
  assert.equal(m.launchActivity({ tool: 'gemini', machine: 'agent-1' }), 'Started Gemini CLI in the home folder on a machine')
  assert.equal(m.launchActivity({}), 'Started an agent in the home folder on a machine')
  assert.equal(m.launchActivity({ tool: 'claude', host: 'studio', prompt: 'secret words' }), 'Started Claude Code in the home folder on studio')
})

test('preselect keeps the machine you came from when it is ready', () => {
  const ms = [{ agentId: 'a', host: 'studio', ready: true }, { agentId: 'b', host: 'annexe', ready: true }]
  assert.deepEqual(m.preselect(ms, 'b'), { agentId: 'b', note: '' })
  assert.deepEqual(m.preselect(ms, ''), { agentId: 'a', note: '' })
  assert.deepEqual(m.preselect(ms, 'gone'), { agentId: 'a', note: '' })
})

test('preselect says so when the machine you came from is not ready', () => {
  const ms = [{ agentId: 'a', host: 'studio', ready: false }, { agentId: 'b', host: 'annexe', ready: true }]
  assert.deepEqual(m.preselect(ms, 'a'), { agentId: 'b', note: "studio isn't ready, so annexe is selected." })
})

test('preselect keeps an unready machine when none is ready, so its readiness sentence shows', () => {
  const ms = [{ agentId: 'a', host: 'studio', ready: false }, { agentId: 'b', host: 'annexe', ready: false }]
  assert.deepEqual(m.preselect(ms, 'b'), { agentId: 'b', note: '' })
  assert.deepEqual(m.preselect(ms, ''), { agentId: '', note: '' })
  assert.deepEqual(m.preselect([], 'b'), { agentId: '', note: '' })
})

test('rekey resets the model and folder only when the machine or agent changes', () => {
  const start = { key: '', model: '', folder: '' }
  const s1 = m.rekey(start, 'a\0claude')
  assert.deepEqual(s1, { key: 'a\0claude', model: '', folder: '' })
  const chosen = { ...s1, model: 'opus', folder: '/home/a/site' }
  assert.equal(m.rekey(chosen, 'a\0claude'), chosen)
  assert.deepEqual(m.rekey(chosen, 'a\0codex'), { key: 'a\0codex', model: '', folder: '' })
  assert.deepEqual(m.rekey(chosen, 'b\0claude'), { key: 'b\0claude', model: '', folder: '' })
  // Going back does not bring the old choice back.
  assert.deepEqual(m.rekey(m.rekey(chosen, 'b\0claude'), 'a\0claude'), { key: 'a\0claude', model: '', folder: '' })
})

test('refusalField names the field a refusal is about', () => {
  assert.equal(m.refusalField(422, 'folder_unavailable'), 'folder')
  assert.equal(m.refusalField(422, 'tool_not_installed'), 'tool')
  assert.equal(m.refusalField(422, 'prompt_unsupported'), 'prompt')
  assert.equal(m.refusalField(409, 'launch_unsupported'), 'machine')
  assert.equal(m.refusalField(409, undefined), '')
  assert.equal(m.refusalField(400, 'folder_unavailable'), '')
})

test('a 401 says the person was signed out', () => {
  assert.equal(refusalText(401), "You've been signed out. Sign in again to continue.")
  assert.equal(refusalText(401, undefined, 'unauthorized'), "You've been signed out. Sign in again to continue.")
})

test('a 200 for a session the machine did not start is said plainly; a started one says nothing', () => {
  const said = "That machine didn't start the agent. Check its terminal settings on that machine and try again."
  assert.equal(notStartedText('lost'), said)
  assert.equal(notStartedText('ended'), said)
  for (const state of ['creating', 'running', 'end_requested', 'exited']) assert.equal(notStartedText(state), '')
})
