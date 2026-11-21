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
}

self.addEventListener('push', event => event.waitUntil(show()))

// A tap opens the session: in a reet window that is already open, or a new one.
self.addEventListener('notificationclick', event => {
  event.notification.close()
  const url = new URL(event.notification.data?.url || '/sessions', self.location.origin).href
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    const open = windows.find(client => client.url === url) || windows[0]
    if (open) {
      await open.focus()
      if (open.url !== url && 'navigate' in open) await open.navigate(url).catch(() => self.clients.openWindow(url))
      return
    }
    await self.clients.openWindow(url)
  })())
})
