<script lang="ts">
  import { untrack } from 'svelte'
  import type { PaneActionKind, PaneCard } from './paneState'

  // What a pane says when it has no live terminal to show: a small tag, a heading, a sentence and its actions, the main
  // one first (the mock-up's Pane states). Over a terminal that is still mounted (offline, open elsewhere) it is a band
  // across the top, so the last screen stays in view. A change of state is announced once, politely; the first state a
  // pane shows is not, so opening a desk of nine panes does not read out nine cards.
  let { card, dir = '', busy = false, error = '', overlay = false, onaction }: {
    card: PaneCard
    /** The folder as shown in the card's words, set in code type where it appears. */
    dir?: string
    /** The main action is in flight: it says so, and every action on the card waits for it. */
    busy?: boolean
    /** A refusal the card has no state of its own for, in the route's words. */
    error?: string
    overlay?: boolean
    onaction: (kind: PaneActionKind) => void
  } = $props()

  const headingId = $props.id()
  /** The sentence in pieces, so the folder can be set in code type without changing the words. */
  const parts = $derived.by(() => {
    const at = dir ? card.body.indexOf(dir) : -1
    return at < 0 ? [card.body, '', ''] : [card.body.slice(0, at), dir, card.body.slice(at + dir.length)]
  })
  const busyLabel = (kind: PaneActionKind) => (kind === 'start' ? 'Starting…' : kind === 'continue' ? 'Continuing…' : 'Reopening…')
  const reopens = (kind: PaneActionKind) => kind === 'reopen' || kind === 'reopenHome' || kind === 'start' || kind === 'again' || kind === 'continue'

  let said = $state(''), seen = ''
  $effect(() => {
    const now = `${card.tag}. ${card.heading}`
    untrack(() => {
      if (seen && now !== seen) said = now
      seen = now
    })
  })
</script>

<div class="state" class:overlay role="group" aria-labelledby={headingId} data-card={card.tag}>
  <span class="tag">{card.tag}</span>
  <h2 id={headingId}>{card.heading}</h2>
  {#if card.body}<p>{parts[0]}{#if parts[1]}<code>{parts[1]}</code>{/if}{parts[2]}</p>{/if}
  {#if error}<p class="bad" role="alert">{error}</p>{/if}
  {#if card.actions.length}
    <div class="row">
      {#each card.actions as action, i (action.kind + action.label)}
        {@const main = i === 0}
        <button type="button" class="btn" class:primary={main} disabled={busy} aria-busy={busy && main ? 'true' : undefined}
          onclick={() => onaction(action.kind)}>{busy && main && reopens(action.kind) ? busyLabel(action.kind) : action.label}</button>
      {/each}
    </div>
  {/if}
  <span class="sr-only" role="status">{said}</span>
</div>

<style>
  .state { flex: 1; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 8px; padding: 16px 20px; overflow: auto; }
  .state h2 { margin: 0; font: 600 1rem/1.35 var(--sans); overflow-wrap: anywhere; }
  .state p { margin: 0; color: var(--muted); max-width: 52ch; overflow-wrap: anywhere; }
  .state p.bad { color: var(--bad); }
  code { font-family: var(--mono); color: var(--ink-soft); }
  .tag { font-size: 14px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--faint); }
  .row { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 4px; }
  /* Over a mounted terminal: a band across the top, the screen under it still in view. */
  .overlay { position: absolute; left: 0; right: 0; top: 0; z-index: 5; flex: none; justify-content: flex-start; gap: 6px; padding: 12px 16px; background: var(--tint); border-bottom: 1px solid var(--line); overflow: visible; }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
  @media (max-width: 650px) { .row { width: 100%; } .row .btn { flex: 1 1 auto; } }
</style>
