import { mount } from 'svelte'
import '../../src/app.css'
import Harness from './GafferProgressHarness.svelte'

document.documentElement.dataset.theme = new URLSearchParams(location.search).get('theme') === 'light' ? 'light' : 'dark'
type Listener = (event: { data: string }) => void
const listeners: Record<string, Listener[]> = {}
let current: ScriptedStream
class ScriptedStream {
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor() { current = this; setTimeout(() => this.onopen?.(), 0) }
  addEventListener(type: string, listener: Listener) { (listeners[type] ??= []).push(listener) }
  close() { for (const key of Object.keys(listeners)) delete listeners[key] }
}
const emit = (type: string, data: unknown) => { for (const listener of listeners[type] ?? []) listener({ data: JSON.stringify(data) }) }
Object.assign(window, {
  EventSource: ScriptedStream,
  __gaffer: { emit: (data: unknown) => emit('chat', data), reset: () => emit('reset', {}), disconnect: () => current.onerror?.(), reconnect: () => current.onopen?.() },
})
// This fixture never contacts a provider or changes a project. All app requests are answered in this page.
const realFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
  if (/model-favourites/.test(url)) return json({ version: 1, favourites: [] })
  if (/\/models/.test(url)) return json({ models: [], state: 'unavailable' })
  const action = /\/chat\/(permissions\/([^/?]+)|questions\/([^/?]+)|prompt|cancel)$/.exec(url)
  if (action) {
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    const kind = action[2] ? 'permission' : action[3] ? 'question' : action[1]
    setTimeout(() => document.dispatchEvent(new CustomEvent('gaffer-fixture-action', { detail: { kind, body, id: action[2] || action[3] } })), 50)
    return json({})
  }
  if (/\/api\//.test(url)) return json({})
  return realFetch(input, init)
}
mount(Harness, { target: document.getElementById('app')! })
