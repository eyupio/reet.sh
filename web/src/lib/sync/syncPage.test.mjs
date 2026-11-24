import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The Sync page's wording: its one sentence, each machine's line and the recent activity.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const install = dataUrl(compile('../install.ts'))
const skills = dataUrl(compile('../skills/skillsPage.ts').replace(/(['"])\.\.\/install\1/, JSON.stringify(install)))
const page = await import(dataUrl(compile('./syncPage.ts').replace(/(['"])\.\.\/skills\/skillsPage\1/, JSON.stringify(skills))))
const { ago, groupActivity, activityText, machineLine, pageSentence, ledeText } = page

const now = Date.UTC(2026, 10, 19, 12, 0, 0)
const min = 60_000, hour = 60 * min, day = 24 * hour
const m = (state, o = {}) => ({ agentId: 'a1', host: 'ThinkPad', paused: false, online: true, protocol: 2, state, failed: [], waiting: [], lastSeen: now - 2 * min, ...o })
const work = (o = {}) => ({ failed: 0, installing: 0, removing: 0, updating: 0, ...o })

test('ago is short and plain', () => {
  assert.equal(ago(now - 10_000, now), 'Now')
  assert.equal(ago(now - 2 * min, now), '2 min')
  assert.equal(ago(now - hour, now), '1 h')
  assert.equal(ago(now - 23 * hour, now), '23 h')
  assert.equal(ago(now - 30 * hour, now), 'Yesterday')
  assert.equal(ago(now - 3 * day, now), '3 days')
  assert.equal(ago(now - 20 * day, now), new Date(now - 20 * day).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' }))
})

test('the page says one sentence', () => {
  assert.equal(pageSentence(work(), 3), 'Everything is in sync.')
  assert.equal(pageSentence(work({ failed: 1 }), 3), '1 machine needs a look.')
  assert.equal(pageSentence(work({ installing: 2, updating: 2 }), 3), 'Installing on 2 machines…')
  assert.equal(pageSentence(work({ needsUpdate: 1, needsUpdateHost: 'old-mac' }), 3), 'Update reet on old-mac to keep its skills in sync.')
  assert.equal(pageSentence(work({ needsNewerController: 1, needsNewerControllerHost: 'future' }), 3), 'future runs a newer reet than this controller; update the controller to keep its skills in sync.')
  assert.equal(pageSentence(work(), 0), 'No machines yet.')
})

test('the lede counts the Active skills and names a paused machine', () => {
  assert.equal(ledeText(118, work()), "Your 118 Active skills go on every machine. Archived ones come off. That's the only rule.")
  assert.equal(ledeText(1, work({ paused: 1, pausedHost: 'build-box' })), "Your 1 Active skill goes on every machine. Archived ones come off. That's the only rule. Sync is paused on build-box, so reet leaves its skills as they are.")
})

test('each machine has a line for its state', () => {
  const cases = [
    [m('ok'), 'In sync · 118 skills · connector seen 2 min ago', 'ok'],
    [m('ok', { lastSeen: now - 5_000 }), 'In sync · 118 skills · connector seen just now', 'ok'],
    [m('installing', { waiting: ['a', 'b'] }), 'Installing 2 skills…', 'work'],
    [m('removing', { waiting: ['a'] }), 'Taking 1 skill off…', 'work'],
    [m('updating', { waiting: ['a', 'b', 'c'] }), 'Updating 3 skills…', 'work'],
    [m('failed', { failed: [{ name: 'seo-cluster', detail: 'no write access' }] }), '1 skill failed: seo-cluster', 'bad'],
    [m('failed', { failed: [{ name: 'a', detail: '' }, { name: 'b', detail: '' }] }), '2 skills failed: a and b', 'bad'],
    [m('blocked'), "Can't write skills here", 'bad'],
    [m('offline', { online: false, lastSeen: now - 30 * hour, waiting: ['a'] }), "Offline · last seen yesterday · catches up when it's back", 'off'],
    [m('offline', { online: false, lastSeen: now - 3 * hour, waiting: ['a', 'b'] }), "Offline · last seen 3 h ago · catches up when it's back", 'off'],
    // Nothing waits for it, so there is nothing to catch up on.
    [m('offline', { online: false, lastSeen: now - 3 * hour }), 'Offline · last seen 3 h ago', 'off'],
    [m('paused', { paused: true }), 'Paused by you · left alone', 'off'],
    [m('needs-update', { host: 'old-mac', protocol: 1 }), 'Update reet on old-mac to keep its skills in sync', 'bad'],
    [m('needs-newer-controller', { host: 'future', protocol: 3 }), 'future runs a newer reet than this controller; update the controller to keep its skills in sync', 'bad'],
  ]
  for (const [machine, text, tone] of cases) assert.deepEqual(machineLine(machine, 118, now), { text, tone }, text)
  // A locked machine is waiting for its owner, not failing: muted, never "Can't write skills here".
  assert.deepEqual(machineLine({ ...cases.find(c => c[0].state === 'blocked')[0] }, 118, now, true), { text: 'Locked, waiting', tone: 'off' })
  assert.deepEqual(machineLine({ ...cases.find(c => c[0].state === 'ok')[0] }, 118, now, true).tone, 'ok')
})

test('activity names the skill and every machine that did the same', () => {
  const at = now - 2 * min
  const groups = groupActivity([
    { at, host: 'Elitebook', name: 'seo-bing', action: 'removed', detail: '' },
    { at: at - 1000, host: 'ThinkPad', name: 'seo-bing', action: 'removed', detail: '' },
    { at: at - 2000, host: 'ThinkPad', name: 'seo-cluster', action: 'failed', detail: 'no write access' },
    { at: at - 3000, host: 'studio', name: 'seo-cluster', action: 'failed', detail: 'disk full' },
  ])
  assert.deepEqual(groups.map(g => g.hosts), [['Elitebook', 'ThinkPad'], ['ThinkPad'], ['studio']])
  assert.deepEqual(activityText(groups[0]), { lead: 'Removed ', name: 'seo-bing', rest: ' from Elitebook and ThinkPad.' })
  assert.deepEqual(activityText(groups[1]), { lead: '', name: 'seo-cluster', rest: ' failed on ThinkPad: no write access.' })
  const one = (action, detail = '') => activityText({ at, name: 'askme', action, detail, hosts: ['a', 'b', 'c'] })
  assert.deepEqual(one('installed'), { lead: 'Installed ', name: 'askme', rest: ' on 3 machines.' })
  assert.deepEqual(one('updated'), { lead: 'Updated ', name: 'askme', rest: ' on 3 machines.' })
  assert.deepEqual(one('removed again'), { lead: 'Removed ', name: 'askme', rest: ' from 3 machines again: it came back.' })
  assert.deepEqual(one('edited'), { lead: '', name: 'askme', rest: " was edited on 3 machines; reet's copy put back." })
  assert.deepEqual(one('found'), { lead: 'Found ', name: 'askme', rest: ' on 3 machines. Waiting for you on Skills.' })
  assert.deepEqual(one('retrying'), { lead: 'Asked 3 machines to try ', name: 'askme', rest: ' again.' })
  assert.deepEqual(one('failed', 'Busy.'), { lead: '', name: 'askme', rest: ' failed on 3 machines: Busy.' })
})
