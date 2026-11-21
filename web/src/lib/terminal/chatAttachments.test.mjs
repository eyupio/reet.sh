import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const chatModule = asURL(compile('../chat.ts'))
const uploadModule = asURL(compile('./upload.ts'))
const a = await import(asURL(compile('./chatAttachments.ts').replace("from '../chat'", `from '${chatModule}'`).replace("from './upload'", `from '${uploadModule}'`)))

const file = (name, size, type = 'image/png') => ({ name, size, type })
const picture = (id, size = 10) => ({ id, file: file(`p${id}.png`, size), preview: `blob:${id}` })
const urls = () => { const made = []; return { made, make: f => { made.push(f.name); return `blob:${f.name}` } } }

test('a message over 2,000 bytes is refused, counting bytes and not letters', () => {
  assert.equal(a.messageProblem('a'.repeat(2000)), '')
  assert.match(a.messageProblem('a'.repeat(2001)), /too long/)
  assert.match(a.messageProblem('é'.repeat(1001)), /too long/)
})

test('images are checked one by one and each refusal says what is wrong', () => {
  const { make, made } = urls()
  const out = a.admitImages([], [file('a.png', 10), file('b.svg', 10, 'image/svg+xml'), file('c.png', 0), file('d.png', 6 << 20), file('e.gif', 5, 'image/gif')], 20, 0, make)
  assert.deepEqual(out.images.map(i => i.file.name), ['a.png', 'e.gif'])
  assert.deepEqual(made, ['a.png', 'e.gif'], 'an object URL is made only for an accepted image')
  assert.deepEqual(out.images.map(i => i.id), [1, 2])
  assert.equal(out.lastId, 2)
  assert.equal(out.refused, 'Each image must be 5 MB or smaller.', 'the last refusal is the one shown')
  assert.equal(a.admitImages([], [file('x.txt', 5, 'text/plain')], 20, 0, make).refused, 'Use PNG, JPEG, GIF or WebP images.')
  assert.equal(a.admitImages([], [file('x.png', 0)], 20, 0, make).refused, 'That image is empty.')
  assert.equal(a.admitImages([], [file('x.png', 5)], 20, 0, make).refused, '')
})

test('the count limit stops the rest and keeps what was taken', () => {
  const { make, made } = urls()
  const out = a.admitImages([picture(1)], [file('a.png', 1), file('b.png', 1), file('c.png', 1)], 3, 1, make)
  assert.deepEqual(out.images.map(i => i.file.name), ['p1.png', 'a.png', 'b.png'])
  assert.equal(out.refused, 'Attach at most 3 images.')
  assert.deepEqual(made, ['a.png', 'b.png'])
})

test('the total size counts what is already attached', () => {
  const { make } = urls()
  const big = 5 << 20
  const held = Array.from({ length: 10 }, (_, i) => picture(i + 1, big))
  const out = a.admitImages(held, [file('late.png', 1)], 20, 10, make)
  assert.equal(out.refused, 'Those images are more than 50 MB together.')
  assert.equal(out.images, held, 'nothing added leaves the same list')
})

test('removing a picture revokes its object URL and only its', () => {
  const revoked = []
  const left = a.removePicture([picture(1), picture(2), picture(3)], 2, url => revoked.push(url))
  assert.deepEqual(left.map(i => i.id), [1, 3])
  assert.deepEqual(revoked, ['blob:2'])
  assert.deepEqual(a.removePicture([picture(1)], 9, url => revoked.push(url)).map(i => i.id), [1])
  assert.deepEqual(revoked, ['blob:2'], 'an id that is not there revokes nothing')
})

test('closing the pane revokes attached, sent and queued pictures, each once, and sent ones only after they were kept', () => {
  const revoked = []
  const hold = a.createPictureHold(url => revoked.push(url))
  hold.keep([picture(10), picture(11)])
  const queued = [{ id: 'queued-1', text: 'x', payload: [picture(20)] }, { id: 'queued-2', text: '', payload: [picture(21), picture(22)] }]
  hold.releaseAll([picture(1), picture(2)], queued)
  assert.deepEqual([...revoked].sort(), ['blob:1', 'blob:10', 'blob:11', 'blob:2', 'blob:20', 'blob:21', 'blob:22'])
  a.revokePictures([picture(30)], url => revoked.push(url))
  assert.equal(revoked.at(-1), 'blob:30')
})

test('uploads go in order and say which one is on its way when there are several', async () => {
  const notes = [], order = []
  const out = await a.uploadPictures([picture(1), picture(2)], async f => { order.push(f.name); return { path: `/up/${f.name}`, reference: `@${f.name}` } }, note => notes.push(note))
  assert.deepEqual(order, ['p1.png', 'p2.png'])
  assert.deepEqual(notes, ['', 'Uploading image 1 of 2…', 'Uploading image 2 of 2…'])
  assert.deepEqual(out.map(u => u.path), ['/up/p1.png', '/up/p2.png'])
  const one = []
  await a.uploadPictures([picture(1)], async f => ({ path: '/p', reference: '@p' }), note => one.push(note))
  assert.deepEqual(one, [''], 'a single picture needs no counter')
})

test('a failed upload stops the rest', async () => {
  const order = []
  await assert.rejects(a.uploadPictures([picture(1), picture(2)], async f => { order.push(f.name); throw new Error('refused') }, () => {}), /refused/)
  assert.deepEqual(order, ['p1.png'])
})

test('each upload name leads to the picture held for it', () => {
  const map = a.previewsFor([{ path: '/up/a', reference: '@a' }, { path: '', reference: '@b' }], [picture(1), picture(2)])
  assert.deepEqual(map, { '/up/a': { url: 'blob:1', name: 'p1.png' }, '@a': { url: 'blob:1', name: 'p1.png' }, '@b': { url: 'blob:2', name: 'p2.png' } })
})

test('the echo of a message with pictures is matched to them once, however it arrives', () => {
  const echoes = a.createEchoes()
  const body = { text: 'look at this', images: ['/up/a'] }
  const echo = a.echoOf(body, [{ path: '/up/a', reference: '@a' }])
  assert.deepEqual(echo, { text: 'look at this', refs: ['/up/a'] })
  assert.equal(a.echoOf({ text: 'plain', images: [] }, []), null)
  assert.deepEqual(a.echoOf({ text: '  ', images: ['/p'] }, [{ path: '', reference: '@p' }]), { text: a.IMAGE_ONLY_ECHO, refs: ['@p'] })
  echoes.add(echo)
  assert.equal(echoes.claim('k0', 'something else'), undefined)
  assert.deepEqual(echoes.claim('k1', '  look at this '), ['/up/a'])
  assert.equal(echoes.claim('k2', 'look at this'), undefined, 'one echo, one message')
  assert.equal(echoes.claim('k1', 'look at this'), undefined, 'a message already seen is not claimed again')
})

test('an echo that was withdrawn is never matched', () => {
  const echoes = a.createEchoes()
  const echo = { text: 'x', refs: ['/p'] }
  echoes.add(echo); echoes.remove(echo)
  assert.equal(echoes.claim('k', 'x'), undefined)
})

test('a sent message shows its bound pictures, with the broker placeholder hidden, else those its text names', () => {
  assert.deepEqual(a.shownAttachments({ text: a.IMAGE_ONLY_ECHO }, ['/p']), { text: '', refs: ['/p'] })
  assert.deepEqual(a.shownAttachments({ text: 'hi' }, ['/p']), { text: 'hi', refs: ['/p'] })
  assert.deepEqual(a.shownAttachments({ text: 'hi' }, null), { text: 'hi', refs: [] })
})

test('the refusal for pictures depends on whether uploads are off', () => {
  assert.equal(a.imagesRefusal(true), 'Image uploads are off on that machine.')
  assert.equal(a.imagesRefusal(false), 'That machine does not accept images.')
  assert.equal(a.sentNote(2), 'Sent one queued message. The next waits for this turn.')
  assert.equal(a.sentNote(0), 'Queued message sent.')
})

test('an expiry fires once after its time, and can be re-armed or cleared first', async () => {
  let fired = 0
  const expiry = a.createExpiry(20, () => { fired++ })
  expiry.arm(); expiry.arm()
  await new Promise(resolve => setTimeout(resolve, 60))
  assert.equal(fired, 1, 'arming again replaces the timer')
  expiry.arm(); expiry.clear()
  await new Promise(resolve => setTimeout(resolve, 60))
  assert.equal(fired, 1)
})

function drain(queue, deliver) {
  const state = { queue, busy: false, halted: [], sent: [], followed: 0, busyWrites: [] }
  const dispatch = a.createQueueDrain({
    queue: () => state.queue,
    busy: () => state.busy,
    setBusy: value => { state.busy = value; state.busyWrites.push(value) },
    deliver,
    halted: note => state.halted.push(note),
    sent: (rest, note, pictures) => { state.queue = rest; state.sent.push({ rest: rest.map(i => i.id), note, pictures }) },
    follow: async () => { state.followed++ },
  })
  return { state, dispatch }
}
const entry = (id, ...pics) => ({ id, text: id, payload: pics })

test('the queue sends its first message, drops it and says how many wait', async () => {
  const sentTexts = []
  const { state, dispatch } = drain([entry('q1', picture(1)), entry('q2')], async text => { sentTexts.push(text); return true })
  await dispatch()
  assert.deepEqual(sentTexts, ['q1'])
  assert.deepEqual(state.sent, [{ rest: ['q2'], note: 'Sent one queued message. The next waits for this turn.', pictures: [picture(1)] }])
  assert.equal(state.followed, 1)
  assert.deepEqual(state.busyWrites, [true, false])
  await dispatch()
  assert.equal(state.sent[1].note, 'Queued message sent.')
  assert.deepEqual(state.queue, [])
})

test('a message that was not sent halts the queue and keeps it', async () => {
  const { state, dispatch } = drain([entry('q1')], async () => false)
  await dispatch()
  assert.deepEqual(state.halted, ['The queued message was not sent. Retry, edit or remove it.'])
  assert.deepEqual(state.queue.map(i => i.id), ['q1'])
  assert.equal(state.sent.length, 0)
  assert.equal(state.busy, false)
})

test('nothing is sent while a send is in flight or the queue is empty, and a throw frees the queue', async () => {
  let calls = 0
  const first = drain([entry('q1')], async () => { calls++; return true })
  first.state.busy = true
  await first.dispatch()
  assert.equal(calls, 0)
  const empty = drain([], async () => { calls++; return true })
  await empty.dispatch()
  assert.equal(calls, 0)
  const broken = drain([entry('q1')], async () => { throw new Error('boom') })
  await assert.rejects(broken.dispatch(), /boom/)
  assert.equal(broken.state.busy, false)
})

test('two dispatches started together send one message', async () => {
  let calls = 0
  const { state, dispatch } = drain([entry('q1'), entry('q2')], async () => { calls++; await new Promise(resolve => setTimeout(resolve, 10)); return true })
  await Promise.all([dispatch(), dispatch()])
  assert.equal(calls, 1)
  assert.deepEqual(state.queue.map(i => i.id), ['q2'])
})
