// Notifications on this device: the card on the Notifications page (on, choices, a test, off, other devices, and the
// browsers that cannot be told), and the service worker itself, woken by a real push event through the browser's
// debugging protocol. The push service is never contacted: the page's subscription is a stand-in, and the worker's
// question to the controller is answered here.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const ENDPOINT = 'https://fcm.googleapis.com/fcm/send/this-phone'
const KEY = 'BAEC_-8'

// A browser whose push manager hands out one fixed subscription, and whose permission prompt answers `answer`.
const standIn = ({ endpoint, answer, subscribed }) => {
  let permission = subscribed ? 'granted' : 'default', subscription = null
  const make = key => ({ endpoint, options: { applicationServerKey: key }, unsubscribe: async () => { subscription = null; window.__unsubscribed = true; return true } })
  if (subscribed) subscription = make(Uint8Array.from([4, 1, 2, 255, 239]).buffer)
  const registration = { pushManager: {
    getSubscription: async () => subscription,
    subscribe: async options => { window.__subscribeOptions = { userVisibleOnly: options.userVisibleOnly, key: [...new Uint8Array(options.applicationServerKey.buffer ?? options.applicationServerKey)] }; subscription = make(options.applicationServerKey.buffer ?? options.applicationServerKey); return subscription },
  } }
  let registered = subscribed
  Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: {
    register: async (url, options) => { window.__registered = { url, scope: options?.scope }; registered = true; return registration },
    getRegistration: async () => registered ? registration : undefined,
    get ready() { return Promise.resolve(registration) },
    addEventListener() {}, removeEventListener() {},
  } })
  window.PushManager = window.PushManager || function PushManager() {}
  class FakeNotification { static get permission() { return permission } static async requestPermission() { window.__asked = true; if (answer) permission = answer; return permission } }
  Object.defineProperty(window, 'Notification', { configurable: true, value: FakeNotification })
}

async function open({ width = 390, height = 844, devices = [], current = 0, answer = 'granted', subscribed = false, available = true, init = null, touch = true, remembered = 0, theme = "light" } = {}) {
  const calls = []
  const state = { devices: [...devices], current }
  const view = () => ({ available, publicKey: available ? KEY : undefined, devices: state.devices, ...(state.current ? { current: state.current } : {}) })
  const app = await openWorkspace(browser, { width, height, touch, theme, routes: {
    '/api/v2/notifications': { notifications: [], unread: 0 },
    '/api/v2/notifications/preferences': { preferences: [] },
    '/api/v2/push': request => { calls.push({ method: 'GET', endpoint: request.headers()['x-reet-push-endpoint'] || '' }); return view() },
    '/api/v2/push/subscriptions': request => {
      const body = request.postDataJSON(); calls.push({ method: request.method(), body })
      if (request.method() === 'DELETE') { state.devices = state.devices.filter(d => d.id !== body.id && !(body.endpoint && d.id === state.current)); if (body.endpoint) state.current = 0; return { removed: true } }
      const kept = body.replace ? state.devices.find(d => d.id === body.replace) : null
      const saved = kept ? { ...kept, deadAt: 0 } : { id: 7, label: body.label, waiting: body.waiting, ended: body.ended, createdAt: Date.now(), lastOkAt: 0 }
      state.devices = [...state.devices.filter(d => d.id !== 7), saved]; state.current = 7
      return saved
    },
    '/api/v2/push/test': request => { calls.push({ method: 'TEST' }); return { sent: state.devices.length, devices: state.devices.length } },
  } })
  await app.page.addInitScript(init ?? standIn, { endpoint: ENDPOINT, answer, subscribed })
  if (remembered) await app.page.addInitScript(id => { try { localStorage.setItem('reet.push.device', String(id)) } catch { /* none */ } }, remembered)
  await app.page.goto(`${base}/notifications`)
  const card = app.page.getByRole('region', { name: 'On this device' })
  return { ...app, calls, card, state }
}
const noSidewaysScroll = page => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)
const targets = card => card.evaluate(node => [...node.querySelectorAll('button, label.check')].filter(e => e.getClientRects().length).map(e => { const r = e.getBoundingClientRect(); return { name: e.innerText.trim().slice(0, 30), w: Math.round(r.width), h: Math.round(r.height) } }).filter(t => t.h < 44))

try {
  // Off, turned on: the browser is asked, the worker registered for the whole site, and the subscription sent with a name.
  {
    const { page, card, calls, errors, context } = await open()
    await expect(card).toBeVisible()
    await expect(card.getByText('Off', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => window.__asked)).toBeFalsy()
    await card.getByRole('button', { name: 'Turn on notifications' }).click()
    await expect(card.getByText('On', { exact: true })).toBeVisible()
    await expect(card.getByRole('status')).toHaveText('On. This device will hear when a session needs you.')
    expect(await page.evaluate(() => [window.__asked, window.__registered, window.__subscribeOptions])).toEqual([true, { url: '/sw.js', scope: '/' }, { userVisibleOnly: true, key: [4, 1, 2, 255, 239] }])
    const saved = calls.find(call => call.method === 'POST')
    expect(saved.body).toEqual({ endpoint: ENDPOINT, label: 'Chrome on Linux', waiting: true, ended: true })
    // The device says which it is when it reads the list, so its own row is the card and not one of "other devices".
    expect(calls.filter(call => call.method === 'GET').at(-1).endpoint).toBe(ENDPOINT)
    await expect(card.getByRole('heading', { name: 'Your other devices' })).toHaveCount(0)

    // A choice is saved as it is made.
    await card.getByRole('checkbox', { name: /A session has finished/ }).uncheck()
    await expect.poll(() => calls.filter(call => call.method === 'POST').at(-1).body).toEqual({ endpoint: ENDPOINT, label: 'Chrome on Linux', waiting: true, ended: false })
    await expect(card.getByRole('checkbox', { name: /A session needs you/ })).toBeChecked()

    await card.getByRole('button', { name: 'Send a test' }).click()
    await expect(card.getByRole('status')).toHaveText('Sent to your device. It should arrive within a few seconds.')

    expect(await noSidewaysScroll(page)).toBe(true)
    expect(await targets(card)).toEqual([])

    await card.getByRole('button', { name: 'Turn off' }).click()
    await expect(card.getByRole('button', { name: 'Turn on notifications' })).toBeVisible()
    expect(calls.find(call => call.method === 'DELETE').body).toEqual({ endpoint: ENDPOINT })
    expect(await page.evaluate(() => window.__unsubscribed)).toBe(true)
    expect(errors).toEqual([])
    await context.close()
  }

  // The browser says no: nothing is registered with the controller, and the card says what to do.
  {
    const { card, calls, context } = await open({ answer: 'denied' })
    await card.getByRole('button', { name: 'Turn on notifications' }).click()
    await expect(card.getByRole('status')).toContainText('Notifications are blocked for this site in the browser.')
    expect(calls.some(call => call.method === 'POST')).toBe(false)
    await expect(card.getByRole('button', { name: 'Turn on notifications' })).toBeEnabled()
    await context.close()
  }

  // Already on, with two other devices: this one is the card, the others are listed and can be removed.
  {
    const mine = { id: 7, label: 'Chrome on Android', waiting: true, ended: false, createdAt: Date.now() - 3 * 86_400_000, lastOkAt: 0 }
    const others = [{ id: 3, label: 'Safari on iPhone', waiting: true, ended: true, createdAt: Date.now() - 9 * 86_400_000, lastOkAt: 0 }, { id: 4, label: 'Firefox on Linux', waiting: true, ended: true, createdAt: Date.now() - 60_000 * 90, lastOkAt: 0 }]
    const { page, card, calls, context } = await open({ width: 820, height: 1180, devices: [mine, ...others], current: 7, subscribed: true })
    await expect(card.getByText('On', { exact: true })).toBeVisible()
    await expect(card.getByRole('checkbox', { name: /A session has finished/ })).not.toBeChecked()
    const list = card.getByRole('list')
    await expect(list.getByRole('listitem')).toHaveText([/Safari on iPhone\s*Turned on 9 days ago/, /Firefox on Linux\s*Turned on 1 hour ago/])
    await list.getByRole('button', { name: 'Remove Safari on iPhone' }).click()
    await expect(list.getByRole('listitem')).toHaveCount(1)
    expect(calls.find(call => call.method === 'DELETE').body).toEqual({ id: 3 })
    expect(await noSidewaysScroll(page)).toBe(true)
    expect(await targets(card)).toEqual([])
    await context.close()
  }

  // The push service dropped this phone's subscription (the controller marked it dead): the card says so and offers one
  // button. Nothing is subscribed until it is pressed; Repair subscribes afresh and replaces the record, choices kept.
  {
    const mine = { id: 7, label: 'Chrome on Android', waiting: true, ended: false, createdAt: Date.now() - 3 * 86_400_000, lastOkAt: 0, deadAt: Date.now() - 60_000 }
    const { page, card, calls, errors, context } = await open({ devices: [mine], current: 7, subscribed: true })
    await expect(card.getByText('Stopped', { exact: true })).toBeVisible()
    await expect(card.getByText('Notifications stopped reaching this phone.')).toBeVisible()
    await expect(card.getByRole('checkbox')).toHaveCount(0)
    expect(await page.evaluate(() => [window.__subscribeOptions, window.__unsubscribed])).toEqual([undefined, undefined])
    expect(calls.some(call => call.method === 'POST')).toBe(false)
    expect(await noSidewaysScroll(page)).toBe(true)
    expect(await targets(card)).toEqual([])
    await card.getByRole('button', { name: 'Repair' }).click()
    await expect(card.getByText('On', { exact: true })).toBeVisible()
    await expect(card.getByRole('status')).toHaveText('Repaired. This device will hear when a session needs you.')
    expect(await page.evaluate(() => [window.__unsubscribed, !!window.__subscribeOptions])).toEqual([true, true])
    expect(calls.find(call => call.method === 'POST').body).toEqual({ endpoint: ENDPOINT, label: 'Chrome on Linux', waiting: true, ended: false, replace: 7 })
    await expect(card.getByRole('checkbox', { name: /A session has finished/ })).not.toBeChecked()
    expect(await page.evaluate(() => localStorage.getItem('reet.push.device'))).toBe('7')
    expect(errors).toEqual([])
    await context.close()
  }

  // The browser lost its subscription (or never told the controller of a new one) while the controller still holds this
  // device: stopped, found by the id this browser remembers. Turn off removes that record by its id.
  {
    const mine = { id: 7, label: 'Firefox on Linux', waiting: true, ended: true, createdAt: Date.now() - 86_400_000, lastOkAt: 0 }
    const { page, card, calls, context } = await open({ width: 1280, height: 900, touch: false, devices: [mine], remembered: 7 })
    await expect(card.getByText('Notifications stopped reaching this device.')).toBeVisible()
    expect(await page.evaluate(() => [window.__asked, window.__subscribeOptions])).toEqual([undefined, undefined])
    await card.getByRole('button', { name: 'Turn off' }).click()
    await expect(card.getByRole('button', { name: 'Turn on notifications' })).toBeVisible()
    expect(calls.find(call => call.method === 'DELETE').body).toEqual({ id: 7 })
    expect(await page.evaluate(() => localStorage.getItem('reet.push.device'))).toBe(null)
    await context.close()
  }

  // A device that works when the card opens, and dies while reet is in the background, is noticed when reet comes back.
  {
    const mine = { id: 7, label: 'Chrome on Android', waiting: true, ended: true, createdAt: Date.now() - 86_400_000, lastOkAt: 0 }
    const { page, card, state, context } = await open({ devices: [mine], current: 7, subscribed: true })
    await expect(card.getByText('On', { exact: true })).toBeVisible()
    state.devices = [{ ...mine, deadAt: Date.now() }]
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
    await expect(card.getByText('Notifications stopped reaching this phone.')).toBeVisible()
    await context.close()
  }

  // An iPhone in a browser tab has no push manager until reet is on the Home Screen: the card says how, and offers no button.
  {
    const iphone = () => {
      Object.defineProperty(navigator, 'userAgent', { configurable: true, get: () => 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1' })
      delete window.PushManager
    }
    const { card, context, page } = await open({ init: iphone })
    if (await page.evaluate(() => 'PushManager' in window)) throw new Error('the stand-in iPhone still has a push manager')
    await expect(card.getByText('add reet to your Home Screen first')).toBeVisible()
    await expect(card.getByRole('button')).toHaveCount(0)
    await context.close()
  }

  // An instance without accounts has nobody for a device to belong to: no card.
  {
    const { page, card, context } = await open({ available: false })
    await expect(page.getByRole('heading', { name: 'What you hear about' })).toBeVisible()
    await expect(card).toHaveCount(0)
    await context.close()
  }

  // The worker itself. A push with no data makes it ask the controller, saying which device it is, and show what
  // comes back; a tap opens that session. With nothing to show (signed out, or already seen) it still shows one line.
  {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 } })
    await context.grantPermissions(['notifications'], { origin: base })
    const page = await context.newPage()
    const asked = []
    let answer = { status: 200, json: { notifications: [{ tag: 'reet-session-s1', title: 'Refactor the store layer', body: 'Needs you.', url: '/sessions?agent=a1&session=s1' }, { tag: 'reet-session-s2', title: 'Fix the failing build', body: 'Finished.', url: 'https://elsewhere.example/steal' }] } }
    await context.route('**/api/v2/push/pending', async route => { asked.push(route.request().headers()['x-reet-push-endpoint'] ?? null); await route.fulfill(answer) })
    await context.route('**/api/**', async route => { if (route.request().url().includes('/push/pending')) return route.fallback(); await route.fulfill({ status: 401, json: { error: 'sign in required' } }) })
    await page.goto(`${base}/sw.js`)
    const registrationId = await page.evaluate(async () => { await navigator.serviceWorker.register('/sw.js', { scope: '/' }); await navigator.serviceWorker.ready; return true })
    expect(registrationId).toBe(true)
    const cdp = await context.newCDPSession(page)
    await cdp.send('ServiceWorker.enable')
    const registrations = await new Promise(resolve => { cdp.on('ServiceWorker.workerRegistrationUpdated', event => { if (event.registrations.length) resolve(event.registrations) }); void cdp.send('ServiceWorker.enable') })
    const id = registrations.find(item => item.scopeURL.startsWith(base)).registrationId
    const shown = () => page.evaluate(async () => (await (await navigator.serviceWorker.ready).getNotifications()).map(n => ({ title: n.title, body: n.body, tag: n.tag, url: n.data?.url })).sort((a, b) => a.tag.localeCompare(b.tag)))
    await cdp.send('ServiceWorker.deliverPushMessage', { origin: base, registrationId: id, data: '' })
    await expect.poll(shown).toEqual([
      { title: 'Refactor the store layer', body: 'Needs you.', tag: 'reet-session-s1', url: '/sessions?agent=a1&session=s1' },
      // An address that is not on this controller is never what a tap opens.
      { title: 'Fix the failing build', body: 'Finished.', tag: 'reet-session-s2', url: '/sessions' },
    ])
    expect(asked.length).toBe(1)
    await page.evaluate(async () => { for (const n of await (await navigator.serviceWorker.ready).getNotifications()) n.close() })
    answer = { status: 401, json: { error: 'sign in required' } }
    await cdp.send('ServiceWorker.deliverPushMessage', { origin: base, registrationId: id, data: '' })
    await expect.poll(shown).toEqual([{ title: 'reet.sh', body: 'Something changed in your sessions.', tag: 'reet-sessions', url: '/sessions' }])
    await context.close()
  }
  console.log('device-push: ok')
} finally {
  await browser.close()
}
