// What web push needs from a browser, and how a device is named. No browser APIs, so the rules are unit tested.

/** Whether this browser can be told, and if not, why. */
export type PushSupport = 'ok' | 'insecure' | 'install' | 'unsupported'

export type PushEnvironment = {
  secure: boolean; serviceWorker: boolean; pushManager: boolean; notification: boolean
  /** An iPhone or iPad, where web push exists only for a site added to the Home Screen. */
  ios: boolean; standalone: boolean
}

export function pushSupport(env: PushEnvironment): PushSupport {
  if (!env.secure) return 'insecure'
  if (env.serviceWorker && env.pushManager && env.notification) return 'ok'
  return env.ios && !env.standalone ? 'install' : 'unsupported'
}

/** An iPad says it is a Mac; only the touch points tell them apart. */
export function isIOS(userAgent: string, maxTouchPoints: number): boolean {
  return /iPad|iPhone|iPod/.test(userAgent) || (/Macintosh/.test(userAgent) && maxTouchPoints > 1)
}

/** "Chrome on Android": how a device is named in the list, so one can be told from another. */
export function deviceLabel(userAgent: string, maxTouchPoints = 0): string {
  const browser = /Edg\w*\//.test(userAgent) ? 'Edge'
    : /Firefox\/|FxiOS\//.test(userAgent) ? 'Firefox'
    : /SamsungBrowser\//.test(userAgent) ? 'Samsung Internet'
    : /Chrome\/|CriOS\//.test(userAgent) ? 'Chrome'
    : /Safari\//.test(userAgent) ? 'Safari' : 'Browser'
  const system = /iPhone|iPod/.test(userAgent) ? 'iPhone'
    : isIOS(userAgent, maxTouchPoints) ? 'iPad'
    : /Android/.test(userAgent) ? 'Android'
    : /Windows/.test(userAgent) ? 'Windows'
    : /Macintosh|Mac OS X/.test(userAgent) ? 'Mac'
    : /CrOS/.test(userAgent) ? 'ChromeOS'
    : /Linux/.test(userAgent) ? 'Linux' : ''
  return system ? `${browser} on ${system}` : browser
}

/** What the card and the Sessions page call this device in a sentence: a phone says "phone", anything else "device". */
export function deviceNoun(label: string): 'phone' | 'device' {
  return /iPhone|Android/.test(label) ? 'phone' : 'device'
}

/** The controller's public key as the bytes pushManager.subscribe wants. */
export function keyBytes(base64url: string): Uint8Array<ArrayBuffer> {
  const padded = base64url.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(base64url.length / 4) * 4, '=')
  const raw = atob(padded), out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

/** Whether a subscription was made for this controller's key; one made for another key cannot be woken by it. */
export function sameKey(current: ArrayBuffer | null | undefined, wanted: Uint8Array): boolean {
  if (!current) return false
  const have = new Uint8Array(current)
  return have.length === wanted.length && have.every((byte, i) => byte === wanted[i])
}

/** This device as the browser and the controller see it together. `stopped` means reet believes it is on, but nothing
 *  will reach it: the push service dropped its subscription (the controller marked it dead), the browser no longer has
 *  the subscription the controller holds, or notification permission was taken back. `remembered` is the device id this
 *  browser last turned on; a device that is not in the list at all (removed from another device) is off. */
export function pushHealth<D extends { id: number; deadAt?: number }>(input: { permission: string; current: D | null; remembered: number; devices: D[] }): { state: 'on' | 'stopped' | 'off'; device: D | null } {
  const { permission, current, remembered, devices } = input
  if (current) return { state: current.deadAt || permission !== 'granted' ? 'stopped' : 'on', device: current }
  const kept = remembered ? devices.find(d => d.id === remembered) : undefined
  return kept ? { state: 'stopped', device: kept } : { state: 'off', device: null }
}

/** Whether a notification that just arrived should chime in this tab: the person turned the sound on, the tab is in
 *  front (a hidden tab leaves it to the notification's own sound), and it did not chime in the last second. */
export function shouldChime(input: { on: boolean; visible: boolean; now: number; last: number }): boolean {
  return input.on && input.visible && input.now - input.last >= 1000
}
