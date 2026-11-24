// The service worker's own logic (web/public/sw.js), run in node against stand-ins for the browser: what it does when the
// browser replaces a subscription. The real worker in a browser is covered by tests/device-push.e2e.mjs.
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'

const source = readFileSync(new URL('../public/sw.js', import.meta.url), 'utf8')

// A worker with its listeners, a push manager that hands out `fresh`, and fetch recorded.
function worker({ fresh = 'https://fcm.googleapis.com/fcm/send/fresh', windows = [], pending = { notifications: [] } } = {}) {
  const on = {}, fetched = [], subscribed = [], opened = []
  const self = {
    location: { origin: 'https://reet.example' },
    addEventListener: (name, fn) => { on[name] = fn },
    skipWaiting() {},
    registration: {
      pushManager: { getSubscription: async () => null, subscribe: async options => { subscribed.push(options); return { endpoint: fresh } } },
      showNotification: async () => {},
    },
    clients: { claim: async () => {}, matchAll: async () => windows, openWindow: async url => { opened.push(url) } },
  }
  const fetch = async (url, init = {}) => { fetched.push({ url, ...init }); return { ok: true, json: async () => pending } }
  new Function('self', 'fetch', source)(self, fetch)
  const fire = async (name, event) => { let work; on[name]({ ...event, waitUntil: p => { work = p } }); await work }
  return { fire, fetched, subscribed, opened }
}

test('a replaced subscription is subscribed again with the same options and the controller is told', async () => {
  const w = worker()
  const options = { userVisibleOnly: true, applicationServerKey: new Uint8Array([4, 1]).buffer }
  await w.fire('pushsubscriptionchange', { oldSubscription: { endpoint: 'https://fcm.googleapis.com/fcm/send/old', options } })
  assert.deepEqual(w.subscribed, [options])
  assert.equal(w.fetched.length, 1)
  assert.equal(w.fetched[0].url, '/api/v2/push/subscriptions')
  assert.equal(w.fetched[0].method, 'POST')
  assert.deepEqual(JSON.parse(w.fetched[0].body), { endpoint: 'https://fcm.googleapis.com/fcm/send/fresh', previous: 'https://fcm.googleapis.com/fcm/send/old' })
})

test('a subscription the browser already replaced is used as it is', async () => {
  const w = worker()
  await w.fire('pushsubscriptionchange', { oldSubscription: { endpoint: 'old', options: {} }, newSubscription: { endpoint: 'given' } })
  assert.deepEqual(w.subscribed, [])
  assert.deepEqual(JSON.parse(w.fetched[0].body), { endpoint: 'given', previous: 'old' })
})

test('without the old subscription nothing says this device was on, so nothing is subscribed', async () => {
  const w = worker()
  await w.fire('pushsubscriptionchange', { oldSubscription: null })
  assert.deepEqual(w.subscribed, [])
  assert.deepEqual(w.fetched, [])
})

// A reet window as clients.matchAll gives it, recording what is done to it.
function win(url, { focused = false, visibilityState = 'visible', controlled = true } = {}) {
  const done = []
  const client = { url, focused, visibilityState, done, focus: async () => { done.push('focus'); return client }, postMessage: message => done.push(message) }
  if (controlled) client.navigate = async to => { done.push(`navigate ${to}`); return client }
  else client.navigate = async () => { throw new TypeError('not controlled') }
  return client
}
const click = url => ({ notification: { close() {}, data: { url } } })
const SESSION = '/sessions?agent=a1&session=s1&from=push'

test('a tap brings forward a window already on that session as it is, whatever else its address says', async () => {
  const other = win('https://reet.example/', { focused: true }), there = win('https://reet.example/sessions?view=terminal&agent=a1&session=s1', { visibilityState: 'hidden' })
  const w = worker({ windows: [other, there] })
  await w.fire('notificationclick', click(SESSION))
  assert.deepEqual(there.done, ['focus'])
  assert.deepEqual(other.done, [])
  assert.deepEqual(w.opened, [])
})

test('a tap moves the reet window in front to the session, not just any window, and never stays on another page', async () => {
  const back = win('https://reet.example/machines', { visibilityState: 'hidden' }), front = win('https://reet.example/sessions?session=s2', { focused: true })
  const w = worker({ windows: [back, front] })
  await w.fire('notificationclick', click(SESSION))
  assert.deepEqual(front.done, ['focus', 'navigate https://reet.example/sessions?agent=a1&session=s1&from=push'])
  assert.deepEqual(back.done, [])
})

test('a window the worker cannot move is asked to move itself; with no reet window a new one opens', async () => {
  const page = win('https://reet.example/notifications', { controlled: false })
  const w = worker({ windows: [page] })
  await w.fire('notificationclick', click(SESSION))
  assert.deepEqual(page.done, ['focus', { type: 'reet-open', url: SESSION }])
  assert.deepEqual(w.opened, [])
  const none = worker()
  await none.fire('notificationclick', click(SESSION))
  assert.deepEqual(none.opened, ['https://reet.example/sessions?agent=a1&session=s1&from=push'])
})

test('a push that shows something tells one reet tab in front, so it can play its sound once', async () => {
  const hidden = win('https://reet.example/', { visibilityState: 'hidden' }), front = win('https://reet.example/sessions', { focused: true }), also = win('https://reet.example/machines')
  const w = worker({ windows: [hidden, also, front], pending: { notifications: [{ tag: 'reet-session-s1', title: 'A', body: 'Needs you.', url: '/sessions' }] } })
  await w.fire('push', {})
  assert.deepEqual(front.done, [{ type: 'reet-notified' }])
  assert.deepEqual([hidden.done, also.done], [[], []])
  // Every tab hidden: the notification's own sound is enough.
  const away = win('https://reet.example/', { visibilityState: 'hidden' })
  await worker({ windows: [away] }).fire('push', {})
  assert.deepEqual(away.done, [])
  // A push that shows nothing (the subscription was just ended) tells nobody.
  const quiet = win('https://reet.example/', { focused: true })
  await worker({ windows: [quiet], pending: { notifications: [], unsubscribe: true } }).fire('push', {})
  assert.deepEqual(quiet.done, [])
})
