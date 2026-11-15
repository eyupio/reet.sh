// Notifications on this device (web push): the browser's own APIs. The rules are in pushSupport.ts.
import { getPush, removePushDevice, savePushDevice, type PushDevice, type PushStatus } from './api'

import { deviceLabel, isIOS, keyBytes, sameKey, type PushEnvironment } from './pushSupport'

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

/** Turns this device on: asks the browser's permission, subscribes, and registers the subscription with the controller. */
export async function enablePush(status: PushStatus, choices: { waiting: boolean; ended: boolean }): Promise<PushDevice> {
  if (!status.publicKey) throw new Error('This instance has no push identity yet. Reload and try again.')
  const permission = Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission()
  if (permission !== 'granted') throw new PushRefused(permission === 'denied'
    ? 'Notifications are blocked for this site in the browser. Allow them in the site settings, then turn this on again.'
    : 'The browser did not allow notifications, so nothing was turned on.')
  const worker = await registration(), key = keyBytes(status.publicKey)
  let subscription = await worker.pushManager.getSubscription()
  if (subscription && !sameKey(subscription.options.applicationServerKey, key)) { await subscription.unsubscribe(); subscription = null }
  subscription ??= await worker.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  return savePushDevice({ endpoint: subscription.endpoint, label: deviceLabel(navigator.userAgent, navigator.maxTouchPoints), ...choices })
}

/** Turns this device off, here and on the controller. */
export async function disablePush(): Promise<void> {
  const subscription = await currentSubscription()
  if (!subscription) return
  await removePushDevice({ endpoint: subscription.endpoint }).catch(() => { /* the browser's side still goes */ })
  await subscription.unsubscribe()
}

/** The controller's devices, with this browser's own record picked out if it is on. A browser whose subscription the
 *  controller does not know (it was removed from another device's list) reads as off, and is not registered again. */
export async function readPush(): Promise<{ status: PushStatus; device: PushDevice | null }> {
  const subscription = 'serviceWorker' in navigator ? await currentSubscription().catch(() => null) : null
  const status = await getPush(subscription?.endpoint ?? '')
  const device = subscription && Notification.permission === 'granted' ? status.devices.find(d => d.id === status.current) ?? null : null
  return { status, device }
}
