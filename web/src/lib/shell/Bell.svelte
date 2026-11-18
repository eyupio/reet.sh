<script lang="ts">
  import { tick } from 'svelte'
  import type { WaitingEntry } from '../home/homeWaiting'
  import { short } from '../gafferOutcome'
  import { needStatus } from '../sessionStatus'
  import { keepFocusIn } from './keepFocus'
  import Approval from './Approval.svelte'

  // The top bar's Needs you bell (spec: Information architecture and navigation). It lists everything that waits for the
  // person, by the same rule as Home's Waiting for you, and its badge is the number listed, so the two never disagree.
  // A session is answered here as on Home; a Factory pipeline links to its detail on the Factory page (and an approval
  // its agent holds is answered here too). When nothing needs the person it carries no badge and stays muted.
  let { entries = [], onseen, onanswered }: {
    entries?: WaitingEntry[]
    onseen?: (sessionId: string) => void | Promise<void>
    onanswered?: () => void | Promise<void>
  } = $props()
  const count = $derived(entries.length)
  // A Factory item's one sentence: Gaffer's recommendation and why when it advised, else what is waiting.
  const sentence = (entry: Extract<WaitingEntry, { source: 'factory' }>) => {
    const advised = entry.item.need?.recommend?.by === 'gaffer' ? entry.item.need.recommend : undefined
    return advised ? `Gaffer recommends: ${advised.label.replace(/…$/, '')}. ${advised.why}` : entry.item.why
  }

  let open = $state(false)
  let root = $state<HTMLDivElement>()
  let button = $state<HTMLButtonElement>()
  const panelId = $props.id()
  const badge = $derived(count > 99 ? '99+' : String(count))
  let list = $state<HTMLUListElement>()

  // An answer or Mark seen that takes its entry away leaves the focus on the next entry (else the previous one, else the
  // bell), never on the page's body.
  async function settled(index: number, action: (() => void | Promise<void>) | undefined) {
    try { await action?.() } finally { await tick(); keepFocusIn(list, index, button) }
  }

  function close(restoreFocus = false) {
    if (!open) return
    open = false
    if (restoreFocus) button?.focus()
  }

  function pointerDown(event: PointerEvent) {
    if (open && event.target instanceof Node && !root?.contains(event.target)) close()
  }

  function keydown(event: KeyboardEvent) {
    if (open && event.key === 'Escape') {
      event.preventDefault()
      close(true)
    }
  }
</script>

<svelte:window onpointerdown={pointerDown} onkeydown={keydown} />

<div class="bell" bind:this={root}>
  <button
    type="button"
    class="bell-button"
    class:waiting={count > 0}
    bind:this={button}
    aria-label={`Needs you: ${count}`}
    aria-expanded={open}
    aria-controls={panelId}
    onclick={() => (open = !open)}
  >
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 17V11a6 6 0 0112 0v6l1.5 2h-15z" /><path d="M10 21h4" /></svg>
    {#if count > 0}<span class="badge" aria-hidden="true">{badge}</span>{/if}
  </button>
  <div class="panel" id={panelId} role="region" aria-label="Needs you" hidden={!open}>
    <p class="heading">Needs you</p>
    {#if entries.length === 0}
      <p class="empty">Nothing needs you.</p>
    {:else}
      <ul class="list" bind:this={list}>
        {#each entries as entry, index (entry.id)}
          {#if entry.source === 'factory'}
          <li class="item" data-status="waiting">
            <span class="kind">{entry.item.chip}</span>
            <span class="title">{entry.item.title}</span>
            <span class="where" title={entry.item.objective}>Factory · {short(entry.item.objective)}</span>
            {#if sentence(entry)}<span class="ask">{sentence(entry)}</span>{/if}
            {#if entry.approval && entry.item.sessionId}<Approval sessionId={entry.item.sessionId} approval={entry.approval} onanswered={() => settled(index, onanswered)} compact />{/if}
            <div class="actions">
              <a class="btn open" href={`/factory?pipeline=${encodeURIComponent(entry.item.id)}`} aria-label={`Open ${entry.item.title} in the Factory`} onclick={() => close()}>Open in the Factory</a>
            </div>
          </li>
          {:else if entry.source === 'gaffer'}
          <li class="item" data-status="waiting">
            <span class="kind">Gaffer</span>
            <span class="title">{entry.goal.title}</span>
            <span class="where">Session{entry.goal.where ? ` · ${entry.goal.where}` : ''}{entry.desk ? ` · Desk ${entry.desk.name}` : ''}</span>
            <span class="ask">Gaffer’s goal needs you.{#if entry.goal.reason}{' '}{entry.goal.reason}{/if}</span>
            <div class="actions"><a class="btn open" href={entry.goal.href} aria-label={`Open ${entry.goal.title}`} onclick={() => close()}>Open</a></div>
          </li>
          {:else}
          {@const card = entry.card}
          {@const answers = !!card.question && !card.approval}
          {@const where = [card.where, entry.desk ? `Desk ${entry.desk.name}` : ''].filter(Boolean).join(' · ')}
          <li class="item" data-status={needStatus(card.kind)}>
            <span class="kind">{card.label} · {card.age}</span>
            <span class="title">{card.title}</span>
            {#if where}<span class="where">{where}</span>{/if}
            {#if answers && card.question}
              <span class="ask">{card.question.message}{#if card.question.queued > 0}<span class="more"> · {card.question.queued} more waiting</span>{/if}</span>
            {/if}
            {#if card.approval}<Approval sessionId={card.sessionId} approval={card.approval} onanswered={() => settled(index, onanswered)} compact />{/if}
            <div class="actions">
              <a class="btn open" href={card.href} aria-label={answers ? `Answer in chat: ${card.title}` : `Open ${card.title}`} onclick={() => { close(); void onseen?.(card.sessionId) }}>{answers ? 'Answer in chat' : 'Open'}</a>
              {#if card.markSeen}
                <button type="button" class="btn seen" aria-label={`Mark ${card.title} as seen`} onclick={() => void settled(index, () => onseen?.(card.sessionId))}>Mark seen</button>
              {/if}
            </div>
          </li>
          {/if}
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .bell { position: relative; flex: none; }
  .bell-button {
    position: relative; display: grid; place-items: center; width: 40px; height: 40px; padding: 0;
    border: 1px solid var(--line); border-radius: 8px; background: none; color: var(--muted); cursor: pointer;
  }
  /* The warning colour only when something waits; a quiet bell at 0. */
  .bell-button.waiting { color: var(--warn); }
  .bell-button:hover, .bell-button[aria-expanded='true'] { border-color: var(--line-strong); background: var(--card); }
  svg { width: 18px; height: 18px; }
  .badge {
    position: absolute; top: -6px; right: -6px; min-width: 18px; height: 18px; padding: 0 4px; display: grid; place-items: center;
    border-radius: 6px; background: var(--accent); color: var(--accent-ink); font-size: 0.875rem; font-weight: 600; line-height: 1; font-variant-numeric: tabular-nums;
  }
  .panel {
    position: absolute; right: 0; top: calc(100% + 8px); z-index: 30; width: min(320px, calc(100vw - 32px));
    max-height: min(50dvh, 360px); overflow-y: auto;
    padding: 12px 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card-raised);
    box-shadow: 0 12px 28px color-mix(in srgb, var(--bg) 70%, transparent);
  }
  .panel[hidden] { display: none; }
  .heading { margin: 0 0 8px; font: 600 1rem/1.3 var(--serif); color: var(--ink); }
  .empty { margin: 0; color: var(--muted); font-size: 0.875rem; }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
  .item { display: flex; flex-direction: column; gap: 4px; padding-bottom: 12px; border-bottom: 1px solid var(--line-soft); }
  .item:last-child { padding-bottom: 0; border-bottom: 0; }
  .kind { align-self: flex-start; font-size: 14px; font-weight: 600; color: var(--status); border: 1px solid var(--status-edge); background: var(--status-wash); border-radius: 6px; padding: 2px 8px; font-variant-numeric: tabular-nums; }
  .ask { font-size: 14px; color: var(--ink); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .ask .more { color: var(--muted); }
  .title { font-size: 14px; font-weight: 500; color: var(--ink); overflow-wrap: anywhere; }
  .where { font-size: 14px; color: var(--muted); overflow-wrap: anywhere; }
  .actions { display: flex; align-items: center; gap: 8px; margin-top: 4px; }
  .btn {
    display: inline-flex; align-items: center; justify-content: center; min-height: 32px; padding: 0 12px;
    border-radius: 6px; font: 500 14px/1 var(--sans); text-decoration: none; cursor: pointer;
  }
  .btn.open { border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); }
  .btn.seen { border: 1px solid var(--line); background: transparent; color: var(--ink); }
  @media (max-width: 1099.98px), (pointer: coarse) {
    .bell-button { width: 44px; height: 44px; border-radius: 8px; }
    .btn { min-height: 44px; flex: 1 1 0; }
  }
  /* A phone: the panel spans the screen under the slim bar instead of hanging off the bell (it ran off the left edge). */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .panel { position: fixed; top: calc(64px + env(safe-area-inset-top, 0px)); left: calc(16px + env(safe-area-inset-left, 0px)); right: calc(16px + env(safe-area-inset-right, 0px)); width: auto; }
    .bell-button { border: 0; }
    .bell-button svg { width: 22px; height: 22px; }
    .badge { top: 4px; right: 4px; }
  }
</style>
