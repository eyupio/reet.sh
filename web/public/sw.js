// reet.sh service worker: notifications only. It caches nothing and answers no requests, so the app always loads
// from the controller as before.
//
// A push carries no data. Woken, the worker asks the controller what to show, over the person's own sign-in, so a
// session's name never passes through a push service (see docs/push-notifications.md).
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()))

const FALLBACK = { tag: 'reet-sessions', title: 'reet.sh', body: 'Something changed in your sessions.', url: '/sessions' }

// pending is what to show, or null when the controller says this browser's subscription is not the signed-in
// person's: somebody else turned it on here and signed out. The subscription is then ended, so their sessions stop
// waking this browser.
async function pending() {
  try {
    const subscription = await self.registration.pushManager.getSubscription()
    const response = await fetch('/api/v2/push/pending', {
      credentials: 'same-origin', cache: 'no-store',
      headers: subscription ? { 'X-Reet-Push-Endpoint': subscription.endpoint } : {},
    })
    if (!response.ok) return []
    const body = await response.json()
    if (body.unsubscribe === true) {
      if (subscription) await subscription.unsubscribe().catch(() => {})
      return null
    }
    return Array.isArray(body.notifications) ? body.notifications : []
  } catch {
    return []
  }
}

// Every push shows something: browsers withdraw a subscription that wakes a worker and shows nothing. The one push
// that shows nothing is the last one of a subscription that was just ended.
async function show() {
  const notes = await pending()
  if (notes === null) return
  const list = notes.length ? notes : [FALLBACK]
  await Promise.all(list.map(note => self.registration.showNotification(String(note.title || FALLBACK.title), {
    body: String(note.body || ''), tag: String(note.tag || FALLBACK.tag), renotify: true,
    icon: '/icon-192.png', badge: '/icon-192.png', data: { url: typeof note.url === 'string' && note.url.startsWith('/') ? note.url : '/sessions' },
  })))
  // One reet tab in front is told, so it can play its sound if the person turned it on there. A hidden tab is not: the
  // notification's own sound is enough, and several tabs would play it several times.
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
  const front = windows.find(client => client.focused) || windows.find(client => client.visibilityState === 'visible')
  front?.postMessage({ type: 'reet-notified' })
}

self.addEventListener('push', event => event.waitUntil(show()))

// The browser replaced or withdrew this device's subscription. A device that was on is subscribed again with the same
// options, and the controller is told to move its record to the new address, keeping its name and choices. Without the
// old subscription nothing says this device was on, so nothing is done: the Notifications page then offers Repair.
async function resubscribe(event) {
  const old = event.oldSubscription
  if (!old) return
  try {
    const fresh = event.newSubscription || await self.registration.pushManager.subscribe(old.options)
    await fetch('/api/v2/push/subscriptions', {
      method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ endpoint: fresh.endpoint, previous: old.endpoint }),
    })
  } catch {
    // Signed out, offline or refused: the page notices the mismatch and offers Repair.
  }
}

self.addEventListener('pushsubscriptionchange', event => event.waitUntil(resubscribe(event)))

// Whether an open window already shows what a tap asks for: the same session (whatever else its address carries, as
// the workspace adds its own view to it), or for any other page the same address.
function shows(clientUrl, target) {
  const open = new URL(clientUrl)
  if (open.origin !== target.origin) return false
  const session = target.searchParams.get('session')
  if (session) return open.pathname === target.pathname && open.searchParams.get('session') === session
  return open.pathname === target.pathname && open.search === target.search
}

// A tap opens the session on the view that asked. A reet window already showing it is brought forward as it is, so
// nothing typed there is lost. Otherwise the reet window in front (or any) is brought forward and moved there; a window
// this worker cannot move itself (it does not control it) is asked to move, and with no reet window a new one opens.
self.addEventListener('notificationclick', event => {
  event.notification.close()
  const target = new URL(event.notification.data?.url || '/sessions', self.location.origin)
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    const same = windows.find(client => shows(client.url, target))
    if (same) { await same.focus(); return }
    const open = windows.find(client => client.focused) || windows.find(client => client.visibilityState === 'visible') || windows[0]
    if (!open) { await self.clients.openWindow(target.href); return }
    await open.focus().catch(() => {})
    try {
      if (!('navigate' in open) || !await open.navigate(target.href)) throw new Error('not moved')
    } catch {
      open.postMessage({ type: 'reet-open', url: target.pathname + target.search })
    }
  })())
})
