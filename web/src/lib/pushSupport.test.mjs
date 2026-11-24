import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { pushSupport, isIOS, deviceLabel, keyBytes, sameKey } = await import(`data:text/javascript;base64,${Buffer.from(compile('./pushSupport.ts')).toString('base64')}`)

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1'
const IPAD = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15'
const ANDROID = 'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36'
const all = { secure: true, serviceWorker: true, pushManager: true, notification: true, ios: false, standalone: false }

test('a browser with everything can be told', () => {
  assert.equal(pushSupport(all), 'ok')
  assert.equal(pushSupport({ ...all, ios: true, standalone: true }), 'ok')
})

test('a page not served over https says so first, whatever else is missing', () => {
  assert.equal(pushSupport({ ...all, secure: false }), 'insecure')
  assert.equal(pushSupport({ ...all, secure: false, pushManager: false, ios: true }), 'insecure')
})

test('an iPhone or iPad in a browser tab is asked to add reet to the Home Screen; anything else is unsupported', () => {
  assert.equal(pushSupport({ ...all, pushManager: false, ios: true, standalone: false }), 'install')
  assert.equal(pushSupport({ ...all, pushManager: false, ios: true, standalone: true }), 'unsupported')
  assert.equal(pushSupport({ ...all, pushManager: false }), 'unsupported')
  assert.equal(pushSupport({ ...all, notification: false }), 'unsupported')
  assert.equal(pushSupport({ ...all, serviceWorker: false }), 'unsupported')
})

test('an iPad that says it is a Mac is told apart by touch', () => {
  assert.equal(isIOS(IPHONE, 5), true)
  assert.equal(isIOS(IPAD, 5), true)
  assert.equal(isIOS(IPAD, 0), false)
  assert.equal(isIOS(ANDROID, 5), false)
})

test('a device is named by its browser and system', () => {
  assert.equal(deviceLabel(IPHONE, 5), 'Safari on iPhone')
  assert.equal(deviceLabel(IPAD, 5), 'Safari on iPad')
  assert.equal(deviceLabel(IPAD, 0), 'Safari on Mac')
  assert.equal(deviceLabel(ANDROID, 5), 'Chrome on Android')
  assert.equal(deviceLabel('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0'), 'Edge on Windows')
  assert.equal(deviceLabel('Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0'), 'Firefox on Linux')
  assert.equal(deviceLabel('Mozilla/5.0 (Linux; Android 14; SM-S921B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36'), 'Samsung Internet on Android')
  assert.equal(deviceLabel('something else'), 'Browser')
})

test("the controller's key is read as bytes, and a subscription for another key is noticed", () => {
  const bytes = keyBytes('BAEC_-8')
  assert.deepEqual([...bytes], [4, 1, 2, 255, 239])
  assert.equal(sameKey(bytes.buffer, keyBytes('BAEC_-8')), true)
  assert.equal(sameKey(bytes.buffer, keyBytes('BAEC_-4')), false)
  assert.equal(sameKey(bytes.buffer, keyBytes('BAEC')), false)
  assert.equal(sameKey(null, bytes), false)
})

test('a device is stopped when reet thinks it is on but nothing can reach it', async () => {
  const { pushHealth } = await import(`data:text/javascript;base64,${Buffer.from(compile('./pushSupport.ts')).toString('base64')}`)
  const phone = { id: 7 }, dead = { id: 7, deadAt: 1 }, other = { id: 3 }
  assert.deepEqual(pushHealth({ permission: 'granted', current: phone, remembered: 7, devices: [phone] }), { state: 'on', device: phone })
  // The push service said it is gone.
  assert.deepEqual(pushHealth({ permission: 'granted', current: dead, remembered: 7, devices: [dead] }), { state: 'stopped', device: dead })
  // Permission was taken back while the subscription stayed.
  assert.deepEqual(pushHealth({ permission: 'denied', current: phone, remembered: 7, devices: [phone] }), { state: 'stopped', device: phone })
  // The browser dropped or changed its subscription: the controller no longer knows this browser by it.
  assert.deepEqual(pushHealth({ permission: 'granted', current: null, remembered: 7, devices: [other, phone] }), { state: 'stopped', device: phone })
  // Removed from another device's list, or never turned on here: off, not stopped.
  assert.deepEqual(pushHealth({ permission: 'granted', current: null, remembered: 7, devices: [other] }), { state: 'off', device: null })
  assert.deepEqual(pushHealth({ permission: 'default', current: null, remembered: 0, devices: [other] }), { state: 'off', device: null })
})
