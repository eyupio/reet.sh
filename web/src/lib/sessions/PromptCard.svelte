<script lang="ts">
  import { tick } from 'svelte'
  import type { ChatState } from '../chat'
  import { sessionPrompt, TerminalError, type TerminalSession } from '../terminal/api'
  import { promptView } from './promptSource'
  import { createPromptLoader, type PromptOutcome } from './promptLoader'
  import PromptDialog from './PromptDialog.svelte'

  let { session, chat, now }: { session: TerminalSession; chat?: ChatState; now: number } = $props()

  const NOT_KEPT = 'This session started before reet kept opening prompts.'
  const view = $derived(promptView(session, chat))
  // promptView calls a chat that has not loaded yet 'old'; saying "not kept" while it is still connecting would be wrong.
  const waiting = $derived(view.kind === 'none' && view.why === 'old' && !!session.structured && (!chat || chat.connection === 'connecting'))
  const quote = $derived(view.kind === 'stored' ? view.excerpt : view.kind === 'chat' ? view.text : '')

  let open = $state(false)
  let outcome = $state<PromptOutcome>({ kind: 'idle' })
  let trigger = $state<HTMLButtonElement>()
  // The races (a late answer, a second read, an abort) live in the loader, where a node test holds them.
  const loader = createPromptLoader(sessionPrompt, () => session.id, next => { outcome = next }, error => error instanceof TerminalError && error.status === 404)

  $effect(() => {
    void session.id
    return () => { loader.reset(); open = false }
  })

  const full = $derived(outcome.kind === 'ready' ? outcome.text : null)
  const dialogState = $derived.by(() => {
    if (view.kind !== 'stored' || !view.more || full !== null) return 'ready' as const
    return outcome.kind === 'failed' ? 'failed' as const : outcome.kind === 'none' ? 'none' as const : 'loading' as const
  })
  const dialogText = $derived(full ?? quote)

  function load() {
    if (view.kind === 'stored' && view.more && outcome.kind !== 'ready' && outcome.kind !== 'none') void loader.load()
  }

  function show() { open = true; load() }
  async function close() {
    loader.abort()
    open = false
    await tick()
    trigger?.focus()
  }
</script>

<div class="prompt" role="group" aria-labelledby={`${session.id}-asked`}>
  <h4 id={`${session.id}-asked`}>You asked</h4>
  <div class="slot">
    {#if quote}<blockquote><p title={quote}>{quote}</p></blockquote>
    {:else if waiting}<p class="line" role="status">Loading the opening prompt…</p>
    {:else}<p class="line">{view.kind === 'none' && view.why === 'unloaded' ? 'Earlier messages are not loaded.' : NOT_KEPT}</p>{/if}
  </div>
  <!-- The button row keeps its height when there is nothing to open, so the blocks below never move. -->
  <div class="action">{#if quote}<button type="button" class="open" bind:this={trigger} onclick={show}>Read full prompt</button>{/if}</div>
</div>
{#if open}
  <PromptDialog {session} text={dialogText} {now} phase={dialogState} notKept={NOT_KEPT} onretry={load} onclose={() => void close()} />
{/if}

<style>
  h4 { margin: 0 0 10px; font: 600 11px var(--sans); letter-spacing: 0.07em; text-transform: uppercase; color: var(--muted); }
  .slot { height: 40px; }
  blockquote { margin: 0; padding-left: 10px; border-left: 2px solid var(--line); }
  blockquote p, .line { margin: 0; font-size: 12px; line-height: 20px; overflow-wrap: anywhere; }
  blockquote p { height: 40px; overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; color: var(--ink); }
  .line { color: var(--muted); max-height: 40px; overflow: hidden; }
  .action { height: 34px; display: flex; align-items: center; }
  .open { font: inherit; font-size: 11px; cursor: pointer; display: inline-flex; align-items: center; border: 0; background: none; padding: 5px 0; min-height: 30px; color: var(--ink); text-decoration: underline; }
  .open:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .action { height: 44px; } .open { min-height: 44px; min-width: 44px; } }
</style>
