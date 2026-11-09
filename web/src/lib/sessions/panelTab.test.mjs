import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./panelTab.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('the saved panel view reads back and stays sticky', () => {
  assert.equal(p.readPanelTab('overview'), 'overview')
  assert.equal(p.readPanelTab('changes'), 'changes')
  assert.equal(p.readPanelTab('files'), 'files')
  assert.equal(p.readPanelTab('github'), 'github')
  // Anything missing, unknown or from an older build falls back to Overview.
  assert.equal(p.readPanelTab(null), 'overview')
  assert.equal(p.readPanelTab(undefined), 'overview')
  assert.equal(p.readPanelTab(''), 'overview')
  assert.equal(p.readPanelTab('settings'), 'overview')
  assert.equal(p.readPanelTab('GITHUB'), 'overview')
})
