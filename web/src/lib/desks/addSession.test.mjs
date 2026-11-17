import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const url = (src) => `data:text/javascript;base64,${Buffer.from(src).toString('base64')}`
const transpile = (name, rewrite = s => s) => ts.transpileModule(rewrite(readFileSync(new URL(name, import.meta.url), 'utf8')), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const terminalApi = url(transpile('../terminal/api.ts'))
const deskApi = url(transpile('./api.ts', s => s.replace("'../terminal/api'", `'${terminalApi}'`)))
const tree = url(transpile('./tree.ts'))
const autosave = url(transpile('./autosave.ts', s => s
  .replace("'../projectColour'", `'${url(transpile('../projectColour.ts'))}'`)
  .replace("'../terminal/api'", `'${terminalApi}'`)
  .replace("'./api'", `'${deskApi}'`)
  .replace("'./tree'", `'${tree}'`)))
const a = await import(url(transpile('./addSession.ts', s => s
  .replace("'../terminal/api'", `'${terminalApi}'`)
  .replace("'./api'", `'${deskApi}'`)
  .replace("'./autosave'", `'${autosave}'`)
  .replace("'./tree'", `'${tree}'`))))

const pane = (id, session) => ({ id, name: id, colour: '', host: 'm1', session, kind: 'shell', dir: '', dirAt: 0, start: '' })
const desk = () => ({ id: 'd', name: 'd', colour: '', template: false, position: 0, version: 1, schema: 1, doc: { active: 't1', tabs: [
  { id: 't1', name: 'one', colour: '', zoomed: '', active: 'p2', root: { split: 'right', ratio: 0.5, a: { pane: 'p1' }, b: { pane: 'p2' } }, panes: { p1: pane('p1', 's1'), p2: pane('p2', 's2') } },
  { id: 't2', name: 'two', colour: '', zoomed: '', active: 'p3', root: { pane: 'p3' }, panes: { p3: pane('p3', 's3') } },
] } })

test('a moved session closes its old pane and the neighbour takes the space', () => {
  const out = a.withoutSession(desk(), 's2')
  const t = out.doc.tabs[0]
  assert.deepEqual(Object.keys(t.panes), ['p1'])
  assert.deepEqual(t.root, { pane: 'p1' })
  assert.equal(t.active, 'p1')
})

test("a tab's only pane stays, empty, and its tab is kept", () => {
  const out = a.withoutSession(desk(), 's3')
  assert.equal(out.doc.tabs.length, 2)
  assert.equal(out.doc.tabs[1].panes.p3.session, '')
})

test('a session on no pane leaves the desk as it is', () => {
  const d = desk()
  assert.equal(a.withoutSession(d, 'nope'), d)
})
