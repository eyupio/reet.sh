import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./fleetSetup.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const f = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('items are named and sized in plain words', () => {
  assert.equal(f.setupItemName({ agent: 'claude', kind: 'instructions', name: 'CLAUDE.md' }), 'Claude Code instructions (CLAUDE.md)')
  assert.equal(f.setupItemName({ agent: 'codex', kind: 'settings', name: 'config.toml' }), 'Codex settings (config.toml)')
  assert.equal(f.setupItemName({ agent: 'zed', kind: 'settings', name: 'a.json' }), 'zed settings (a.json)')
  assert.equal(f.sizeLabel(1), '1 byte')
  assert.equal(f.sizeLabel(812), '812 bytes')
  assert.equal(f.sizeLabel(1536), '1.5 KB')
  assert.equal(f.sizeLabel(48 * 1024), '48 KB')
})

test('a file line says what happens to it', () => {
  const settings = { agent: 'claude', kind: 'settings', name: 'settings.json' }
  assert.equal(f.fleetFileLine({ ...settings, state: 'create', fields: ['permissions', 'outputStyle'] }), 'Creates Claude Code settings.json with permissions and outputStyle.')
  assert.equal(f.fleetFileLine({ ...settings, state: 'merge', fields: ['permissions'] }), 'Changes permissions in Claude Code settings.json.')
  assert.equal(f.fleetFileLine({ agent: 'claude', kind: 'instructions', name: 'CLAUDE.md', state: 'merge' }), 'Replaces the text of Claude Code CLAUDE.md.')
  assert.equal(f.fleetFileLine({ agent: 'claude', kind: 'instructions', name: 'CLAUDE.md', state: 'create' }), 'Creates Claude Code CLAUDE.md.')
  assert.equal(f.fleetFileLine({ ...settings, state: 'unchanged' }), 'Claude Code settings.json is already the same.')
  assert.match(f.fleetFileLine({ ...settings, state: 'blocked', reason: 'destination_symlink' }), /left alone: the file there is a symbolic link\./)
  assert.match(f.fleetFileLine({ ...settings, state: 'blocked', reason: 'no_location' }), /no place for it/)
  assert.match(f.fleetFileLine({ ...settings, state: 'blocked', reason: 'something_new' }), /will not touch it/)
})

test('widening names the fields, and counts of changed lines read plainly', () => {
  const files = [{ agent: 'claude', widens: ['permissions'] }, { agent: 'codex', widens: ['approval_policy', 'sandbox_mode'] }, { agent: 'claude' }, { agent: 'claude', widens: ['permissions'] }]
  assert.deepEqual(f.widenedFields(files), ['Claude Code permissions', 'Codex approval_policy', 'Codex sandbox_mode'])
  assert.equal(f.widenWarning('Laptop', files), 'This copy lets agents do more without asking on Laptop: Claude Code permissions, Codex approval_policy and Codex sandbox_mode.')
  assert.equal(f.widenWarning('Laptop', [{ agent: 'claude', widens: ['permissions'] }]), 'This copy lets agents do more without asking on Laptop: Claude Code permissions.')
  assert.equal(f.widenWarning('Laptop', []), 'This copy lets agents do more without asking on Laptop.')
  assert.deepEqual(f.widenedFields([{ agent: 'claude' }]), [])
  assert.equal(f.diffCounts({ added: ['a', 'b'], removed: ['c'] }), '2 added, 1 removed')
  assert.equal(f.diffCounts({ added: ['a'] }), '1 added')
  assert.equal(f.diffCounts({}), '')
  assert.deepEqual(f.changingFiles([{ state: 'create' }, { state: 'merge' }, { state: 'unchanged' }, { state: 'blocked' }]).length, 2)
})

test('every exclusion has a plain reason, and an unknown one keeps the controller\'s own', () => {
  for (const code of ['same_machine', 'not_enrolled', 'unsupported', 'terminal_off', 'locked', 'not_allowed', 'offline', 'busy', 'plan_failed', 'unchanged']) {
    assert.ok(f.machineExcluded({ code, reason: 'controller sentence' }).length > 10, code)
    assert.notEqual(f.machineExcluded({ code, reason: 'controller sentence' }), 'controller sentence', code)
  }
  assert.match(f.machineExcluded({ code: 'not_allowed' }), /allow-apply/)
  assert.equal(f.machineExcluded({ code: 'brand_new', reason: 'A new sentence.' }), 'A new sentence.')
  assert.equal(f.machineStateLabel({ state: 'ready' }), 'Ready')
  assert.equal(f.machineStateLabel({ state: 'unchanged' }), 'Already the same')
  assert.equal(f.machineStateLabel({ state: 'excluded' }), 'Left out')
  assert.deepEqual(f.readyMachines([{ state: 'ready', n: 1 }, { state: 'excluded' }, { state: 'unchanged' }, { state: 'ready', n: 2 }]).map(m => m.n), [1, 2])
})

test('the typed word is needed only when a chosen machine widens, and the request carries it only then', () => {
  assert.equal(f.anyWidens([{ widens: false }, { widens: false }]), false)
  assert.equal(f.anyWidens([{ widens: false }, { widens: true }]), true)
  assert.equal(f.anyWidens([]), false)
  assert.equal(f.confirmed('', false), true)
  assert.equal(f.confirmed('', true), false)
  assert.equal(f.confirmed('cop', true), false)
  assert.equal(f.confirmed('copy', true), true)
  assert.equal(f.confirmed('  Copy ', true), true)
  assert.equal(f.confirmed('COPY', true), true)
  assert.equal(f.confirmed('copy it', true), false)
  // The word is trimmed and lowercased before it is sent, and exactly `copy`; nothing is sent when none was needed.
  assert.equal(f.confirmToSend('  Copy ', true), 'copy')
  assert.equal(f.confirmToSend('copy', false), undefined)
  assert.equal(f.confirmToSend('', false), undefined)
  assert.equal(f.confirmToSend('nope', true), undefined)
  assert.equal(f.copyButton(1), 'Copy to 1 machine')
  assert.equal(f.copyButton(3), 'Copy to 3 machines')
})

test('members have a label, a note and an undo only when one can be undone', () => {
  assert.equal(f.setupMemberLabel({ state: 'applied' }), 'Copied')
  assert.equal(f.setupMemberLabel({ state: 'applied', undoneAt: 5 }), 'Undone')
  assert.equal(f.setupMemberLabel({ state: 'running' }), 'Copying')
  assert.equal(f.setupMemberLabel({ state: 'failed' }), 'Failed')
  assert.equal(f.setupMemberLabel({ state: 'skipped' }), 'Skipped')
  assert.equal(f.setupMemberLabel({ state: 'excluded' }), 'Left out')
  assert.equal(f.setupMemberLabel({ state: 'pending' }), 'Waiting')
  assert.equal(f.setupMemberCanUndo({ state: 'applied', journal: 'j1' }), true)
  assert.equal(f.setupMemberCanUndo({ state: 'applied', journal: 'j1', undoneAt: 9 }), false)
  assert.equal(f.setupMemberCanUndo({ state: 'applied', journal: '' }), false)
  assert.equal(f.setupMemberCanUndo({ state: 'failed', journal: 'j1' }), false)
  // A skipped machine was never reached; a dropped connection may have applied.
  assert.match(f.setupMemberNote({ state: 'skipped', code: 'offline' }), /nothing was changed/)
  assert.match(f.setupMemberNote({ state: 'failed', code: 'connection_lost' }), /may or may not have finished/)
  assert.match(f.setupMemberNote({ state: 'failed', code: 'changed' }), /read the source again/)
  assert.match(f.setupMemberNote({ state: 'failed', code: 'not_allowed' }), /allow-apply/)
  assert.match(f.setupMemberNote({ state: 'failed', code: 'content_expired' }), /read the source again/)
  // An excluded machine says why in the review's words, not the skipped one's.
  assert.equal(f.setupMemberNote({ state: 'excluded', code: 'offline' }), "That machine isn't connected.")
  assert.equal(f.setupMemberNote({ state: 'excluded', code: 'not_reviewed', reason: 'Not in the review.' }), 'Not in the review.')
  assert.equal(f.setupMemberNote({ state: 'applied' }), '')
  for (const code of ['write_failed', 'timed_out', 'apply_unsupported', 'bad_answer', 'apply_failed', 'busy', 'off', 'locked', 'nothing', 'invalid', 'abandoned', 'expired', 'paused_by_restore']) {
    assert.ok(f.setupMemberNote({ state: 'failed', code }).length > 10, code)
  }
})

test('the batch reads as one sentence', () => {
  const m = (position, host, state, extra = {}) => ({ position, agentId: `a-${host}`, host, state, widens: false, files: [], ...extra })
  assert.deepEqual(f.setupBatchSummary({ state: 'running', members: [m(0, 'Laptop', 'applied'), m(1, 'Build', 'running'), m(2, 'Mini', 'pending'), m(3, 'Gone', 'excluded')] }),
    { state: 'working', headline: 'Copying to Build…', detail: '1 of 3 done.' })
  const stopped = f.setupBatchSummary({ state: 'stopped', members: [m(0, 'Build', 'failed', { code: 'write_failed' })] })
  assert.equal(stopped.headline, 'Stopped on Build.')
  assert.equal(stopped.detail, 'Retry it, skip it and carry on, or abandon the copy.')
  assert.deepEqual(f.setupBatchSummary({ state: 'done', members: [m(0, 'A', 'applied'), m(1, 'B', 'applied'), m(2, 'C', 'skipped')] }),
    { state: 'finished', headline: 'Copied to 2 machines.', detail: 'C was skipped.' })
  assert.equal(f.setupBatchSummary({ state: 'done', members: [m(0, 'A', 'applied', { undoneAt: 1 }), m(1, 'B', 'applied')] }).headline, 'Copied to 1 machine.')
  assert.equal(f.setupBatchSummary({ state: 'done', members: [m(0, 'A', 'applied', { undoneAt: 1 }), m(1, 'B', 'applied')] }).detail, '1 undone.')
  assert.equal(f.setupBatchSummary({ state: 'done', members: [m(0, 'A', 'applied', { undoneAt: 1 })] }).headline, 'Every copy was undone.')
  assert.equal(f.setupBatchSummary({ state: 'done', members: [m(0, 'A', 'skipped')] }).headline, 'Nothing was copied.')
  assert.equal(f.setupBatchSummary({ state: 'abandoned', members: [] }).headline, 'The copy was abandoned.')
  assert.equal(f.setupBatchCanAbandon({ state: 'stopped', members: [m(0, 'A', 'failed')] }), true)
  assert.equal(f.setupBatchCanAbandon({ state: 'running', members: [m(0, 'A', 'running')] }), false)
  assert.equal(f.setupBatchCanAbandon({ state: 'done', members: [] }), false)
  assert.equal(f.setupFailedMember({ members: [m(0, 'A', 'applied'), m(1, 'B', 'failed')] }).host, 'B')
  assert.equal(f.setupBatchStateLabel('stopped'), 'Stopped on a failure')
})

test('every error code in the reference becomes a plain sentence', () => {
  const codes = ['machine_not_found', 'setup_unsupported', 'setup_offline', 'setup_terminal_off', 'setup_locked', 'setup_busy', 'setup_nothing', 'setup_too_large', 'setup_credential', 'setup_read_invalid', 'setup_failed', 'setup_timed_out',
    'setup_request_invalid', 'setup_batch_empty', 'setup_batch_duplicate', 'setup_batch_source_only', 'setup_batch_in_progress', 'setup_read_expired', 'request_id_required', 'setup_confirm_required',
    'setup_batch_not_found', 'setup_batch_member_not_failed', 'setup_batch_finished', 'setup_batch_running', 'software_batch_position_invalid',
    'setup_undo_done', 'setup_undo_unavailable', 'setup_changed', 'setup_no_journal', 'setup_not_allowed']
  for (const code of codes) {
    const sentence = f.setupErrorMessage({ code, message: 'raw server text' }, 'fallback')
    assert.notEqual(sentence, 'raw server text', code)
    assert.notEqual(sentence, 'fallback', code)
    assert.match(sentence, /^[A-Z].*[.]$/, code)
    assert.doesNotMatch(sentence, /—/, code)
  }
  assert.match(f.setupErrorMessage({ code: 'setup_read_expired' }, ''), /Read the source machine again/)
  assert.match(f.setupErrorMessage({ code: 'setup_changed' }, ''), /Nothing was changed/)
  // A credential refusal keeps the controller's sentence, which names the file and agent, and never shows text.
  assert.equal(f.setupErrorMessage({ code: 'setup_credential', message: "reet won't copy CLAUDE.md (claude): it looks like it holds a credential. Remove it on that machine and read it again." }, ''), "reet won't copy CLAUDE.md (claude): it looks like it holds a credential. Remove it on that machine and read it again.")
  assert.match(f.setupErrorMessage({ code: 'setup_credential', message: 'odd' }, ''), /credential/)
  assert.equal(f.setupErrorMessage({ message: 'The terminal connection is unavailable.' }, 'x'), 'The terminal connection is unavailable.')
  assert.equal(f.setupErrorMessage(undefined, 'fallback'), 'fallback')
})
