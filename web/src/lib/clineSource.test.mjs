import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { KNOWN_SOURCES, sourceLabel, sourceChoices, resolveStartSource, creditGuidance } = await import(asURL(compile('./clineSource.ts')))

test('known sources are labelled and any other id is shown as Cline reports it', () => {
  assert.deepEqual(KNOWN_SOURCES.map(s => s.id), ['cline', 'cline-pass'])
  assert.equal(sourceLabel('cline'), 'Usage-Billing')
  assert.equal(sourceLabel('cline-pass'), 'ClinePass')
  assert.equal(sourceLabel('x-other'), 'x-other')
})

test('a source is only a choice when there is more than one', () => {
  assert.deepEqual(sourceChoices(['cline']), [])
  assert.deepEqual(sourceChoices([]), [])
  assert.deepEqual(sourceChoices(['cline', 'cline-pass', 'x-other']), [
    { id: 'cline', label: 'Usage-Billing' }, { id: 'cline-pass', label: 'ClinePass' }, { id: 'x-other', label: 'x-other' },
  ])
})

test('the start source is the explicit choice, then the saved default, then Cline’s own', () => {
  const offered = ['cline', 'cline-pass']
  assert.deepEqual(resolveStartSource({ explicit: 'cline-pass', saved: 'cline', offered }), { source: 'cline-pass', note: '' })
  assert.deepEqual(resolveStartSource({ saved: 'cline-pass', offered }), { source: 'cline-pass', note: '' })
  assert.deepEqual(resolveStartSource({ offered }), { source: '', note: '' })
  assert.deepEqual(resolveStartSource({ saved: '', offered }), { source: '', note: '' })
  // A saved source that is no longer offered falls back to Cline's own, and says so.
  const gone = resolveStartSource({ saved: 'gone', offered })
  assert.equal(gone.source, '')
  assert.match(gone.note, /gone/)
  assert.match(gone.note, /isn.t available/)
})

test('no saved default leaves Cline’s own choice, without a note or a block', () => {
  const offered = ['cline', 'cline-pass']
  assert.deepEqual(resolveStartSource({ offered, saved: '' }), { source: '', note: '' })
  assert.deepEqual(resolveStartSource({ offered }), { source: '', note: '' })
})

test('a one-off choice overrides the saved default', () => {
  const offered = ['cline', 'cline-pass']
  assert.equal(resolveStartSource({ explicit: 'cline', saved: 'cline-pass', offered }).source, 'cline')
  assert.equal(resolveStartSource({ explicit: 'cline-pass', saved: 'cline', offered }).source, 'cline-pass')
  // Cline's own is a deliberate one-off too; it is not replaced by the saved default.
  assert.equal(resolveStartSource({ explicit: '', saved: 'cline-pass', offered }).source, '')
})

test('both agent flows use one source control and one default resolver', () => {
  for (const file of ['./StartAgentDialog.svelte', './GafferOutcome.svelte']) {
    const text = readFileSync(new URL(file, import.meta.url), 'utf8')
    assert.match(text, /import ClineSourceSelect from '\.\/ClineSourceSelect\.svelte'/, file)
    assert.match(text, /<ClineSourceSelect/, file)
    assert.match(text, /resolveStartSource\(/, file)
    assert.match(text, /defaultClineSource/, file)
  }
})

test('the credit hint names the other sources the session offers', () => {
  const lead = 'This source has no credit. Switch to '
  const tail = ', or add credits with Cline, then send a message to continue.'
  assert.equal(creditGuidance({ acpModel: true, offered: ['cline', 'cline-pass'], current: 'cline' }), lead + 'ClinePass above' + tail)
  assert.equal(creditGuidance({ acpModel: false, offered: ['cline', 'cline-pass', 'x-other'], current: 'cline' }), lead + 'ClinePass or x-other above' + tail)
  assert.equal(creditGuidance({ acpModel: true, offered: ['cline'], current: 'cline' }),
    'This model has reached its usage or credit limit. Choose another model below, or add credits with your provider, then send a message to continue.')
  assert.equal(creditGuidance({ acpModel: false, offered: [], current: '' }),
    'This model has reached its usage or credit limit. Add credits with your provider, then send a message to continue.')
})
