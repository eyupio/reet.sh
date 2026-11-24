// Notifications on this device (web push): the browser's own APIs. The rules are in pushSupport.ts.
import { getPush, removePushDevice, savePushDevice, type PushDevice, type PushStatus } from './api'

import { deviceLabel, isIOS, keyBytes, pushHealth, sameKey, type PushEnvironment } from './pushSupport'

export function browserEnvironment(): PushEnvironment {
  const standalone = matchMedia('(display-mode: standalone)').matches || (navigator as Navigator & { standalone?: boolean }).standalone === true
  return {
    secure: window.isSecureContext, serviceWorker: 'serviceWorker' in navigator, pushManager: 'PushManager' in window,
    notification: 'Notification' in window, ios: isIOS(navigator.userAgent, navigator.maxTouchPoints), standalone,
  }
}

const WORKER = '/sw.js'

async function registration(): Promise<ServiceWorkerRegistration> {
  await navigator.serviceWorker.register(WORKER, { scope: '/' })
  return navigator.serviceWorker.ready
}

/** This browser's subscription, if it has one. Never asks for permission and never registers the worker. */
export async function currentSubscription(): Promise<PushSubscription | null> {
  const existing = await navigator.serviceWorker.getRegistration('/')
  return existing ? existing.pushManager.getSubscription() : null
}

export class PushRefused extends Error {}

// The id of the device this browser last turned on, so a browser whose subscription vanished can still say which record
// was its own. A convenience: without storage a vanished subscription reads as off.
const REMEMBERED = 'reet.push.device'
function remembered(): number {
  try { return Number(localStorage.getItem(REMEMBERED)) || 0 } catch { return 0 }
}
function remember(id: number) {
  try { if (id) localStorage.setItem(REMEMBERED, String(id)); else localStorage.removeItem(REMEMBERED) } catch { /* storage unavailable */ }
}

/** Turns this device on: asks the browser's permission, subscribes, and registers the subscription with the controller.
 *  With `replace` (Repair) the browser's subscription is made afresh and takes that record's place. */
export async function enablePush(status: PushStatus, choices: { waiting: boolean; ended: boolean }, replace?: PushDevice): Promise<PushDevice> {
  if (!status.publicKey) throw new Error('This instance has no push identity yet. Reload and try again.')
  const permission = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission()
  if (permission !== 'granted') throw new PushRefused(permission === 'denied'
    ? 'Notifications are blocked for this site in the browser. Allow them in the site settings, then turn this on again.'
    : 'The browser did not allow notifications, so nothing was turned on.')
  const worker = await registration(), key = keyBytes(status.publicKey)
  let subscription = await worker.pushManager.getSubscription()
  if (subscription && (replace || !sameKey(subscription.options.applicationServerKey, key))) { await subscription.unsubscribe(); subscription = null }
  subscription ??= await worker.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  const device = await savePushDevice({ endpoint: subscription.endpoint, label: deviceLabel(navigator.userAgent, navigator.maxTouchPoints), ...choices, ...(replace ? { replace: replace.id } : {}) })
  remember(device.id)
  return device
}

/** Turns this device off, here and on the controller. `id` removes this device's record when the browser no longer has
 *  the subscription to name it by. */
export async function disablePush(id = 0): Promise<void> {
  remember(0)
  const subscription = await currentSubscription()
  if (!subscription) { if (id) await removePushDevice({ id }); return }
  await removePushDevice({ endpoint: subscription.endpoint }).catch(() => { /* the browser's side still goes */ })
  await subscription.unsubscribe()
}

/** Sign-out: turns this device off while the session can still delete its record. Never throws, and gives up after a few
 *  seconds so sign-out is not held up. */
export async function disablePushOnSignOut(): Promise<void> {
  if (!('serviceWorker' in navigator)) return
  let timer: ReturnType<typeof setTimeout> | undefined
  await Promise.race([
    disablePush(remembered()).catch(() => { /* sign-out goes ahead */ }),
    new Promise<void>((done) => { timer = setTimeout(done, 3000) }),
  ])
  clearTimeout(timer)
}

/** The controller's devices, with this browser's own record picked out, and whether it still works (see pushHealth). A
 *  browser whose subscription the controller does not know (it was removed from another device's list) reads as off, and
 *  is not registered again. Nothing here subscribes: a stopped device is repaired only when the person asks. */
export async function readPush(): Promise<{ status: PushStatus; device: PushDevice | null; stopped: boolean }> {
  const subscription = 'serviceWorker' in navigator ? await currentSubscription().catch(() => null) : null
  const status = await getPush(subscription?.endpoint ?? '')
  const current = subscription ? status.devices.find(d => d.id === status.current) ?? null : null
  const permission = 'Notification' in window ? Notification.permission : 'denied'
  const health = pushHealth({ permission, current, remembered: remembered(), devices: status.devices })
  remember(health.device?.id ?? 0)
  return { status, device: health.device, stopped: health.state === 'stopped' }
}
