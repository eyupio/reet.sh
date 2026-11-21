import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The Skills page's wording, focus and confirmation helpers.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const install = dataUrl(compile('../install.ts'))
const page = await import(dataUrl(compile('./skillsPage.ts').replace(/(['"])\.\.\/install\1/, JSON.stringify(install))))
const { arrivedOn, pausedSentence, savedSkillNotice, healthLine, toastText, focusAfterRemoval, deleteConfirm, copyPrompt, installCommandFor, syncSentence, matchesSearch, historyWhat, historyLine, versionDay } = page

const h = (state, o = {}) => ({ state, pending: 0, total: 0, hosts: [], machines: [], ...o })
const row = (o = {}) => ({ id: 'x', name: 'demo', description: 'Demo.', yours: false, published: false, hasCopy: true, origin: 'https://github.com/o/r', health: h('ok'), ...o })

test('retryText names the skill and the machines it is tried again on', () => {
  assert.equal(page.retryText('seo-cluster', ['ThinkPad']), 'Trying seo-cluster again on ThinkPad.')
  assert.equal(page.retryText('seo-cluster', ['ThinkPad', 'studio']), 'Trying seo-cluster again on ThinkPad and studio.')
  assert.equal(page.retryText('seo-cluster', ['a', 'b', 'c']), 'Trying seo-cluster again on 3 machines.')
})

test('healthLine words every state', () => {
  const cases = [
    [h('ok', { total: 3 }), 'On all 3 machines', 'ok', false],
    [h('ok', { total: 1 }), 'On 1 machine', 'ok', false],
    [h('needs-update'), 'Update reet on your machines to keep their skills in sync', 'bad', false],
    [h('installing', { pending: 2, total: 3 }), 'Installing on 2 of 3', 'work', false],
    [h('removing', { pending: 3 }), 'Coming off 3 machines', 'work', false],
    [h('removing', { pending: 1, hosts: ['ThinkPad'] }), 'Coming off ThinkPad', 'work', false],
    [h('failed', { hosts: ['ThinkPad'] }), 'Failed on ThinkPad', 'bad', true],
    [h('failed', { hosts: ['ThinkPad', 'studio'] }), 'Failed on 2 machines', 'bad', true],
    [h('offline', { hosts: ['studio'] }), "Removing from studio when it's back online", 'work', false],
    [h('blocked', { hosts: ['ThinkPad'] }), "Can't write skills on ThinkPad", 'bad', false],
    [h('copying', { hosts: ['Elitebook'] }), 'Copying into reet from Elitebook', 'work', false],
    [h('copy-failed', { reason: 'over 8 MiB', hosts: ['Elitebook'] }), "Couldn't copy into reet: over 8 MiB. It stays on Elitebook until you delete it.", 'bad', true],
    [h('getting', { hosts: ['Elitebook'] }), 'Getting a copy from Elitebook', 'work', false],
    [h('getting'), 'Getting the files', 'work', false],
    [h('fetch-failed'), "Couldn't fetch the files", 'bad', true],
    [h('needs-update', { hosts: ['ThinkPad'] }), 'Update reet on ThinkPad to keep its skills in sync', 'bad', false],
    [h('needs-update', { hosts: ['a', 'b'] }), 'Update reet on 2 machines to keep their skills in sync', 'bad', false],
    [h('off'), 'Off every machine', 'off', false],
  ]
  for (const [health, text, tone, retry] of cases) {
    const got = healthLine(health)
    assert.equal(got.text, text)
    assert.equal(got.tone, tone, text)
    assert.equal(got.retry, retry, text)
  }
  for (const s of ['ok', 'installing', 'removing', 'failed', 'offline', 'blocked', 'copying', 'copy-failed', 'getting', 'fetch-failed', 'needs-update', 'off']) {
    assert.notEqual(healthLine(h(s)).text, '', s)
  }
})

test('an offline machine waits to install an Active skill and to remove an Archived one', () => {
  const offline = h('offline', { pending: 1, hosts: ['studio'] })
  assert.deepEqual(healthLine(offline, 'active'), { text: "Installing on studio when it's back online", tone: 'work', retry: false })
  assert.deepEqual(healthLine(offline, 'archived'), { text: "Removing from studio when it's back online", tone: 'work', retry: false })
  assert.equal(healthLine(h('offline', { pending: 2, hosts: ['studio', 'pi'] }), 'active').text, "Installing on 2 machines when they're back online")
  assert.equal(healthLine(h('offline', { pending: 2, hosts: ['studio', 'pi'] }), 'archived').text, "Removing from 2 machines when they're back online")
})

test('toastText', () => {
  assert.equal(toastText('archived', ['seo-bing'], 3), 'Archived seo-bing · coming off 3 machines')
  assert.equal(toastText('archived', ['a', 'b'], 1), 'Archived 2 skills · coming off 1 machine')
  assert.equal(toastText('deleted', ['x'], 3), 'Deleted x · coming off 3 machines')
  assert.equal(toastText('restored', ['x'], 3), 'Restored x · going back on 3 machines')
  assert.equal(toastText('kept', ['paseo-help'], 3), 'Kept paseo-help · going on every machine')
})

test('focusAfterRemoval', () => {
  assert.equal(focusAfterRemoval(['a', 'b', 'c'], ['b']), 'c')
  assert.equal(focusAfterRemoval(['a', 'b', 'c'], ['c']), 'b')
  assert.equal(focusAfterRemoval(['a', 'b'], ['a', 'b']), null)
})

test('deleteConfirm', () => {
  const w = deleteConfirm([row({ name: 'reet-babysit', yours: true })])
  assert.equal(w.title, 'Delete reet-babysit?')
  assert.equal(w.body, 'You wrote this one. Deleting it removes your writing and the skill from your machines; each machine keeps a backup for 30 days.')
  assert.deepEqual([w.writtenConfirmed, w.noCopyConfirmed], [true, false])
  const wp = deleteConfirm([row({ yours: true, published: true })])
  assert.equal(wp.body, 'You wrote this one. Deleting it removes your writing and its public link, and the skill from your machines; each machine keeps a backup for 30 days.')
  // An Archived skill is on no machine: the dialog does not say Delete takes it off them.
  const wa = deleteConfirm([row({ yours: true, status: 'archived', health: h('off') })])
  assert.equal(wa.body, 'You wrote this one. Deleting it removes your writing.')
  const wpa = deleteConfirm([row({ yours: true, published: true, status: 'archived', health: h('off') })])
  assert.equal(wpa.body, 'You wrote this one. Deleting it removes your writing and its public link.')
  const n = deleteConfirm([row({ hasCopy: false })])
  assert.equal(n.body, 'reet has no copy of this skill. Deleting removes it from your machines; each machine keeps a backup for 30 days.')
  assert.deepEqual([n.writtenConfirmed, n.noCopyConfirmed], [false, true])
  assert.equal(deleteConfirm([row()]), null)
  assert.equal(deleteConfirm([]), null)
})

test('deleteConfirm covers both cases together and several rows', () => {
  const both = deleteConfirm([row({ name: 'a', yours: true, hasCopy: false, published: true })])
  assert.equal(both.body, 'You wrote this one. reet has no copy of this skill. Deleting removes your writing and its public link, and the skill from your machines; each machine keeps a backup for 30 days.')
  assert.deepEqual([both.writtenConfirmed, both.noCopyConfirmed], [true, true])
  const many = deleteConfirm([row({ name: 'a', yours: true }), row({ name: 'b', hasCopy: false }), row({ name: 'c' }), row({ name: 'd', yours: true, published: true }), row({ name: 'e', yours: true })])
  assert.equal(many.title, 'Delete 5 skills?')
  assert.equal(many.body, 'You wrote a, d and e. reet has no copy of b. Deleting removes them from your machines, along with your writing and the public links of d; each machine keeps a backup for 30 days.')
  const four = deleteConfirm([row({ name: 'a', yours: true }), row({ name: 'b', yours: true }), row({ name: 'c', yours: true }), row({ name: 'd', yours: true })])
  assert.equal(four.body, 'You wrote a, b, c and 1 more. Deleting removes them from your machines, along with your writing; each machine keeps a backup for 30 days.')
  for (const d of [w1(), both, many, four]) assert.ok(!d.body.includes('nothing can bring it back'))
  function w1() { return deleteConfirm([row({ hasCopy: false })]) }
})

test('deleteConfirm says which of several skills come off the machines, and never that an Archived one does', () => {
  const off = h('off')
  // Active a and Archived b, both written: only a comes off the machines.
  const mixed = deleteConfirm([row({ name: 'a', yours: true }), row({ name: 'b', yours: true, status: 'archived', health: off })])
  assert.equal(mixed.body, 'You wrote a and b. Deleting removes them, along with your writing. a comes off your machines; each machine keeps a backup for 30 days.')
  // An Archived skill still coming off a machine is on it until then.
  const leaving = deleteConfirm([row({ name: 'a', yours: true, status: 'archived', health: h('removing', { pending: 1, hosts: ['studio'] }) }), row({ name: 'b', yours: true, status: 'archived', health: off }), row({ name: 'c', hasCopy: false })])
  assert.equal(leaving.body, 'You wrote a and b. reet has no copy of c. Deleting removes them, along with your writing. a and c come off your machines; each machine keeps a backup for 30 days.')
  // None of them is on a machine: no backup is promised.
  const archived = deleteConfirm([row({ name: 'a', yours: true, published: true, status: 'archived', health: off }), row({ name: 'b', hasCopy: false, status: 'archived', health: off })])
  assert.equal(archived.body, "You wrote a. reet has no copy of b. Deleting removes them, along with your writing and the public links of a. They're already off your machines.")
  // One Archived skill reet has no copy of, off every machine.
  assert.equal(deleteConfirm([row({ hasCopy: false, status: 'archived', health: off })]).body, "reet has no copy of this skill. Deleting removes it. It's already off your machines.")
  // An Archived skill reet could not copy stays on its machine until it is deleted.
  assert.equal(deleteConfirm([row({ hasCopy: false, status: 'archived', health: h('copy-failed', { hosts: ['Elitebook'] }) })]).body, 'reet has no copy of this skill. Deleting removes it from your machines; each machine keeps a backup for 30 days.')
})

test('copyPrompt for a skill that is not published never carries a link', () => {
  const p = copyPrompt({ name: 'seo-audit', description: 'Full SEO audit.' })
  assert.equal(p, 'Use the seo-audit skill: Full SEO audit.')
  const q = copyPrompt({ name: 'x', description: 'See https://github.com/o/r for more' })
  assert.ok(!/https?:|github\.com/.test(copyPrompt({ name: 'x', description: 'Does things.' })))
  assert.ok(!/https?:|github\.com/.test(q))
  assert.equal(q, 'Use the x skill.')
  assert.equal(copyPrompt({ name: 'x', description: 'Does things. See https://github.com/o/r for more. Fast.' }), 'Use the x skill: Does things. Fast.')
})

test('copyPrompt for a published skill fetches it from its reet.sh page', () => {
  const p = copyPrompt({ name: 'ponytail-audit', description: 'Quality audit of a whole repo.', published: true, publicUrl: 'https://reet.sh/paul/ponytail-audit' })
  assert.equal(p, 'Run `npx skills@1.7.4 use "https://reet.sh/paul/ponytail-audit" --skill "ponytail-audit"` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.')
  // Unpublished, or with no page yet, or with an address that could end the quoted command: used by name.
  assert.equal(copyPrompt({ name: 'x', description: 'Does things.', published: false, publicUrl: 'https://reet.sh/paul/x' }), 'Use the x skill: Does things.')
  assert.equal(copyPrompt({ name: 'x', description: 'Does things.', published: true }), 'Use the x skill: Does things.')
  assert.equal(copyPrompt({ name: 'x', published: true, publicUrl: 'https://reet.sh/paul/x"`id`' }), 'Use the x skill.')
})

test('installCommandFor', () => {
  assert.equal(installCommandFor(row()), null)
  const cmd = installCommandFor(row({ published: true, publicUrl: 'https://reet.sh/s/demo' }))
  assert.equal(cmd, 'npx skills@1.7.4 add https://reet.sh/s/demo --skill demo')
})

test('syncSentence says installing, coming off or updating, and failures first', () => {
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0 }), 'Everything is in sync.')
  assert.equal(syncSentence({ failed: 0, installing: 2, removing: 0 }), 'Installing on 2 machines…')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 2 }), 'Coming off 2 machines…')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 1 }), 'Coming off 1 machine…')
  assert.equal(syncSentence({ failed: 0, installing: 2, removing: 1 }), 'Updating 3 machines…')
  // A machine that both installs and removes is one machine being updated.
  assert.equal(syncSentence({ failed: 0, installing: 3, removing: 3, updating: 3 }), 'Updating 3 machines…')
  assert.equal(syncSentence({ failed: 1, installing: 2, removing: 2 }), '1 machine needs a look.')
  assert.equal(syncSentence({ failed: 2, installing: 0, removing: 0 }), '2 machines need a look.')
})

test('syncSentence asks for reet to be updated on a machine below the skills protocol, after failures', () => {
  assert.equal(syncSentence({ failed: 0, installing: 2, removing: 0, needsUpdate: 1, needsUpdateHost: 'ThinkPad' }), 'Update reet on ThinkPad to keep its skills in sync')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsUpdate: 2 }), 'Update reet on 2 machines to keep their skills in sync')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsUpdate: 1 }), 'Update reet on 1 machine to keep its skills in sync')
  assert.equal(syncSentence({ failed: 1, installing: 0, removing: 0, needsUpdate: 2 }), '1 machine needs a look.')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsUpdate: 0 }), 'Everything is in sync.')
})

test('syncSentence asks for the controller to be updated when a machine runs a newer reet, after machines to update', () => {
  assert.equal(syncSentence({ failed: 0, installing: 2, removing: 0, needsNewerController: 1, needsNewerControllerHost: 'ThinkPad' }), 'ThinkPad runs a newer reet than this controller; update the controller to keep its skills in sync')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsNewerController: 1 }), '1 machine runs a newer reet than this controller; update the controller to keep its skills in sync')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsNewerController: 3 }), '3 machines run a newer reet than this controller; update the controller to keep their skills in sync')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsUpdate: 1, needsUpdateHost: 'studio', needsNewerController: 1, needsNewerControllerHost: 'ThinkPad' }), 'Update reet on studio to keep its skills in sync')
  assert.equal(syncSentence({ failed: 2, installing: 0, removing: 0, needsNewerController: 1 }), '2 machines need a look.')
  assert.equal(syncSentence({ failed: 0, installing: 0, removing: 0, needsNewerController: 0 }), 'Everything is in sync.')
})

// The Skills page searches as Markdown and CSV export do (store.go splitQuery and ListFor), so an export holds the
// rows the page shows: words find the name, the shelf id, the description or the notes; tag:x keeps skills tagged x.
test('matchesSearch finds what export finds', () => {
  const r = row({ name: 'seo-bing', shelfId: 'demo/toolkit/seo-bing', description: 'Bing Webmaster Tools checks.', notes: 'Use for client sites', tags: [{ id: 1, name: 'SEO' }, { id: 2, name: 'research work' }] })
  const yes = q => assert.equal(matchesSearch(r, q), true, q)
  const no = q => assert.equal(matchesSearch(r, q), false, q)
  for (const q of ['', '  ', 'bing', 'BING', 'toolkit/seo', 'webmaster tools', 'client sites', 'tag:seo', 'tag:SEO bing', 'tag:"research work"', 'tag:seo tag:"research work"']) yes(q)
  // A tag's name is found with tag:, not as a word; words are matched as one phrase, in order.
  for (const q of ['research', 'tools webmaster', 'tag:missing', 'tag:seo nothing-like-it', 'tag:research']) no(q)
})

test('a published skill is in the library as Active, not To test', () => {
  assert.equal(savedSkillNotice(false, true), "Skill published. It's in your library as Active and goes on every machine.")
  assert.match(savedSkillNotice(true, true), /^Renamed and published\..* It's in your library as Active and goes on every machine\.$/)
  assert.equal(savedSkillNotice(false, false), 'Draft saved. Only you can see it.')
  for (const renamed of [false, true]) for (const published of [false, true]) assert.doesNotMatch(savedSkillNotice(renamed, published), /To test/)
})

test('pausedSentence names a paused machine, or counts several, and says nothing when none is paused', () => {
  assert.equal(pausedSentence({ failed: 0, installing: 0, removing: 0, paused: 1, pausedHost: 'ThinkPad' }), 'Sync is paused on ThinkPad.')
  assert.equal(pausedSentence({ failed: 0, installing: 0, removing: 0, paused: 1 }), 'Sync is paused on 1 machine.')
  assert.equal(pausedSentence({ failed: 0, installing: 0, removing: 0, paused: 2 }), 'Sync is paused on 2 machines.')
  assert.equal(pausedSentence({ failed: 0, installing: 0, removing: 0, paused: 0 }), '')
  assert.equal(pausedSentence({ failed: 0, installing: 0, removing: 0 }), '')
})

test('arrivedOn: day and month this year, with the year otherwise, and nothing without a date', () => {
  const now = Date.UTC(2026, 10, 19, 12)
  assert.equal(arrivedOn(Date.UTC(2026, 10, 12, 12), now), '12 Nov')
  assert.equal(arrivedOn(Date.UTC(2025, 2, 3, 12), now), '3 Mar 2025')
  assert.equal(arrivedOn(0, now), '')
  assert.equal(arrivedOn(undefined, now), '')
})

test('historyLine: how each version was saved, in plain words, with its day', () => {
  const now = Date.UTC(2026, 10, 20, 12)
  const at = day => Date.UTC(2026, 10, day, 12)
  assert.equal(historyLine({ kind: 'edit', savedAt: at(12) }, false, now), 'Edited 12 Nov')
  assert.equal(historyLine({ kind: 'original', savedAt: at(3) }, false, now), 'Updated from original 3 Nov')
  assert.equal(historyLine({ kind: 'original', savedAt: at(1) }, true, now), 'Imported 1 Nov')
  assert.equal(historyLine({ kind: 'capture', machine: 'Elitebook', savedAt: at(1) }, true, now), 'Kept from Elitebook 1 Nov')
  assert.equal(historyLine({ kind: 'import', savedAt: at(1) }, true, now), 'Kept from a machine 1 Nov')
  assert.equal(historyLine({ kind: 'restore', savedAt: at(14) }, false, now), 'Put back 14 Nov')
  assert.equal(historyLine({ kind: 'use-version', machine: 'ThinkPad', savedAt: at(9) }, false, now), 'Took the version on ThinkPad 9 Nov')
  assert.equal(historyLine({ kind: 'user', savedAt: at(2) }, true, now), 'Written in reet 2 Nov')
  assert.equal(historyLine({ kind: 'fork', savedAt: at(2) }, true, now), 'Copied into reet 2 Nov')
  assert.equal(historyLine({ kind: 'edit', savedAt: Date.UTC(2025, 0, 5, 12) }, false, now), 'Edited 5 Jan 2025')
  assert.equal(historyWhat({ kind: 'something-new' }, false), 'Saved')
})

test('versionDay: the day, with the time when another version shares it', () => {
  const now = Date.UTC(2026, 10, 20, 12)
  const at = (d, h, m) => new Date(2026, 10, d, h, m).getTime()
  const all = [{ savedAt: at(12, 10, 0) }, { savedAt: at(3, 14, 5) }, { savedAt: at(3, 9, 5) }]
  assert.equal(versionDay(all[0], all, now), '12 Nov')
  assert.equal(versionDay(all[1], all, now), '3 Nov, 14:05')
  assert.equal(versionDay(all[2], all, now), '3 Nov, 09:05')
  assert.equal(historyLine({ kind: 'original', savedAt: all[2].savedAt }, true, now, versionDay(all[2], all, now)), 'Imported 3 Nov, 09:05')
})
