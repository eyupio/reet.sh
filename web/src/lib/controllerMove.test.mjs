import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./controllerMove.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const {
  NOT_FENCED, addressLines, addressProblem, cleanAddress, copyPasswordProblem, copySource, createFenceWatch, fenceFromRefusal,
  fenceSentence, finalCopy, isLiveSourceRefusal, moveFeatures, setServerCommand,
} = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const fence = (over = {}) => ({ fenced: true, since: 1000, movedTo: 'https://reet.new.example', ...over })
const copy = (over = {}) => ({ id: 'c1', takenAt: 2000, local: true, fenced: true, ...over })

// The page offers a step only when the controller says it holds it, so an older controller is never sent a field or
// a route it would not understand.
test('a controller that lists no features is offered no part of a move', () => {
  const none = { fence: false, copyOrigin: false, publicUrl: false, copyPassword: false }
  assert.deepEqual(moveFeatures(undefined), none)
  assert.deepEqual(moveFeatures(null), none)
  assert.deepEqual(moveFeatures('fence'), none)
  assert.deepEqual(moveFeatures([]), none)
  assert.deepEqual(moveFeatures(['fence', 'copy-password-request']), { fence: true, copyOrigin: false, publicUrl: false, copyPassword: true })
  assert.deepEqual(moveFeatures(['copy-origin', 'fence', 'restore-public-url', 'copy-password-request']), { fence: true, copyOrigin: true, publicUrl: true, copyPassword: true })
})

// A banner that appeared on any 423, or on a body the controller did not write, would tell people the controller had
// moved when it had not.
test('only the controller\'s own refusal says it is fenced', () => {
  const body = { error: 'This controller has moved', code: 'controller_moved', fenced: true, since: 1234, movedTo: 'https://reet.new.example' }
  assert.deepEqual(fenceFromRefusal(423, body), { fenced: true, since: 1234, movedTo: 'https://reet.new.example' })
  assert.deepEqual(fenceFromRefusal(423, { code: 'controller_moved' }), { fenced: true, since: 0, movedTo: '' })
  assert.equal(fenceFromRefusal(409, body), null)
  assert.equal(fenceFromRefusal(423, { error: 'Locked' }), null)
  assert.equal(fenceFromRefusal(423, { code: 'something_else' }), null)
  assert.equal(fenceFromRefusal(423, null), null)
  assert.equal(fenceFromRefusal(423, 'controller_moved'), null)
})

test('the fence watch tells its listeners of a change once, and stops when asked', () => {
  const watch = createFenceWatch()
  const heard = []
  const stop = watch.subscribe(f => heard.push(f))
  assert.deepEqual(watch.get(), NOT_FENCED)
  watch.set(fence())
  watch.set(fence())
  assert.equal(heard.length, 1)
  assert.deepEqual(watch.get(), fence())
  // A lift is the plain unfenced state whatever else the answer carried, and saying so twice is one change.
  watch.set({ fenced: false, since: 99, movedTo: 'https://stale.example' })
  watch.set(NOT_FENCED)
  assert.equal(heard.length, 2)
  assert.deepEqual(watch.get(), NOT_FENCED)
  stop()
  watch.set(fence())
  assert.equal(heard.length, 2)
})

// The address is used again in `reet agent set-server` and in the saved addresses, which take an origin and nothing
// more, so a path is caught beside the field instead of on a machine three steps later.
test('an address is optional, and when given is a plain http or https origin', () => {
  for (const ok of ['', '   ', 'https://reet.example.com', 'https://reet.example.com/', ' https://reet.example.com:8443 ', 'http://127.0.0.1:8080']) {
    assert.equal(addressProblem(ok), '', ok)
  }
  assert.match(addressProblem('reet.example.com'), /full address/)
  assert.match(addressProblem('ftp://reet.example.com'), /https:\/\/ or http:\/\//)
  assert.match(addressProblem('https://sam:secret@reet.example.com'), /user name and password/)
  assert.match(addressProblem('https://reet.example.com/app'), /nothing after the host name/)
  assert.match(addressProblem('https://reet.example.com?x=1'), /nothing after the host name/)
  assert.match(addressProblem('https://reet.example.com#top'), /nothing after the host name/)
  assert.equal(cleanAddress(' https://reet.example.com// '), 'https://reet.example.com')
})

test('a copy password follows the controller\'s rule of twelve characters', () => {
  assert.match(copyPasswordProblem(''), /at least 12 characters/)
  assert.match(copyPasswordProblem('eleven-char'), /at least 12 characters/)
  assert.match(copyPasswordProblem('           x'), /at least 12 characters/)
  assert.equal(copyPasswordProblem('twelve-chars'), '')
})

test('the command for a machine names the new address, or a placeholder nobody could mistake for one', () => {
  assert.equal(setServerCommand('https://reet.new.example/'), 'reet agent set-server https://reet.new.example')
  assert.equal(setServerCommand(''), 'reet agent set-server https://NEW-ADDRESS')
})

test('the banner sentence says since when and where to, and nothing when there is no fence', () => {
  const when = ms => `day ${ms}`
  assert.equal(fenceSentence(NOT_FENCED, when), '')
  assert.equal(fenceSentence(fence(), when), 'Every change has been refused here since day 1000. It has moved to https://reet.new.example.')
  assert.equal(fenceSentence(fence({ movedTo: '' }), when), 'Every change has been refused here since day 1000.')
  assert.equal(fenceSentence(fence({ since: 0, movedTo: '' }), when), 'Every change has been refused here.')
})

// "Unknown" is a copy from before copies recorded their source. Reading it as live, or as fenced, would both be a guess.
test('a copy that does not say is unknown, not live', () => {
  assert.equal(copySource({ fenced: true }), 'fenced')
  assert.equal(copySource({ fenced: false }), 'live')
  assert.equal(copySource({}), 'unknown')
})

// The copy offered for download is the one a new controller accepts without being told the old one is gone. A copy
// from before this fence, even a fenced one, is older than what the controller did in between.
test('the final copy is the newest fenced one on this machine taken since the fence went up', () => {
  const items = [
    copy({ id: 'before', takenAt: 500 }),
    copy({ id: 'live', takenAt: 3000, fenced: false }),
    copy({ id: 'unknown', takenAt: 3500, fenced: undefined }),
    copy({ id: 'elsewhere', takenAt: 4000, local: false }),
    copy({ id: 'first', takenAt: 1500 }),
    copy({ id: 'second', takenAt: 2500 }),
  ]
  assert.equal(finalCopy(items, fence()).id, 'second')
  assert.equal(finalCopy(items, NOT_FENCED), null)
  assert.equal(finalCopy(items.slice(0, 4), fence()), null)
  assert.equal(finalCopy([], fence()), null)
})

test('the report says what was done about the address, or that nothing was', () => {
  assert.deepEqual(addressLines({}), ['Saved addresses were left as they were.'])
  assert.deepEqual(addressLines({ publicUrl: 'https://reet.new.example', mailBaseRewritten: true, syncOriginsRewritten: 2, mcpGrantsStale: 3 }), [
    'Saved addresses now point to https://reet.new.example.',
    'The mail base URL was changed.',
    'The sync library origin was changed for 2 account(s).',
    '3 MCP grant(s) issued for the old address were removed; each client signs in again here.',
  ])
  assert.deepEqual(addressLines({ publicUrl: 'https://reet.new.example' }).slice(1), [
    'No mail base URL was saved, so none was changed.',
    'No sync library origin was saved, so none was changed.',
    'No MCP grant was issued for the old address.',
  ])
})

// The tick is offered on the refusal's code. Matching the sentence would break the day its wording improved.
test('the live-source tick is asked for on the refusal\'s code, not its wording', () => {
  assert.equal(isLiveSourceRefusal({ code: 'copy_source_live', message: 'anything' }), true)
  assert.equal(isLiveSourceRefusal({ code: 'controller_not_empty' }), false)
  assert.equal(isLiveSourceRefusal(new Error('this copy was taken from a controller that was not stopped')), false)
  assert.equal(isLiveSourceRefusal(null), false)
})
