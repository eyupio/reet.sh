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
