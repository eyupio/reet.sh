import { mount } from 'svelte'
import '../../src/app.css'
import Harness from './QuestionCardHarness.svelte'

// ?theme=light|dark picks the palette (dark, the designed one, by default).
document.documentElement.dataset.theme = new URLSearchParams(location.search).get('theme') === 'light' ? 'light' : 'dark'

// The real chat pane, with the network answered here: a stream the page's buttons feed, and answers that are only recorded.
// Nothing leaves the page. A question is never sent by a real agent yet, so this is how a person tries the card by hand.
type Listener = (event: { data: string }) => void
const listeners: Record<string, Listener[]> = {}
class ScriptedStream {
  url: string
  onopen: ((event: unknown) => void) | null = null
  constructor(url: string) { this.url = url; setTimeout(() => this.onopen?.({}), 0) }
  addEventListener(type: string, listener: Listener) { (listeners[type] ??= []).push(listener) }
  close() { /* the page keeps its stream for good */ }
}
;(window as unknown as { EventSource: unknown }).EventSource = ScriptedStream
;(window as unknown as { __emit: (type: string, data: unknown) => void }).__emit = (type, data) => { for (const l of listeners[type] ?? []) l({ data: JSON.stringify(data) }) }

const answers: { id: string; body: unknown }[] = []
;(window as unknown as { __answers: typeof answers }).__answers = answers
const real = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  const question = /\/chat\/questions\/([^/?]+)$/.exec(url)
  if (question) {
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    answers.push({ id: decodeURIComponent(question[1]), body })
    document.dispatchEvent(new CustomEvent('fixture-answer', { detail: { id: decodeURIComponent(question[1]), body } }))
    return json({ answered: true })
  }
  if (/\/models/.test(url)) return json({ tool: 'opencode', state: 'unavailable', models: [], more: false, source: '', version: '', fetchedAt: 0, checkedAt: 0, reason: '', sentence: '' })
  if (/model-favourites/.test(url)) return json({ version: 1, favourites: [] })
  if (/^\/api\//.test(url)) return json({})
  return real(input, init)
}

mount(Harness, { target: document.getElementById('app')! })
