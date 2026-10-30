<script lang="ts">
  import type { NeedCard } from '../home'

  // The top bar's Needs you bell (spec: Information architecture and navigation).
  // When nothing needs the person it carries no badge and stays in the muted chrome colour.
  let { count, cards = [], onseen }: {
    count: number
    cards?: NeedCard[]
    onseen?: (sessionId: string) => void | Promise<void>
  } = $props()

  let open = $state(false)
  let root = $state<HTMLDivElement>()
  let button = $state<HTMLButtonElement>()
  const panelId = $props.id()
  const badge = $derived(count > 99 ? '99+' : String(count))

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
    {#if cards.length === 0}
      <p class="empty">Nothing needs you.</p>
    {:else}
      <ul class="list">
        {#each cards as card (card.sessionId)}
          <li class="item">
            <span class="kind {card.tone}">{card.label} · {card.age}</span>
            <span class="title">{card.title}</span>
            {#if card.where}<span class="where">{card.where}</span>{/if}
            <div class="actions">
              <a class="btn open" href={card.href} onclick={() => { close(); void onseen?.(card.sessionId) }}>Open</a>
              {#if card.markSeen}
                <button type="button" class="btn seen" onclick={() => void onseen?.(card.sessionId)}>Mark seen</button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .bell { position: relative; flex: none; }
  .bell-button {
    position: relative; display: grid; place-items: center; width: 40px; height: 40px; padding: 0;
    border: 1px solid var(--line); border-radius: 9px; background: none; color: var(--muted); cursor: pointer;
  }
  /* The warning colour only when something waits; a quiet bell at 0. */
  .bell-button.waiting { color: var(--warn); }
  .bell-button:hover, .bell-button[aria-expanded='true'] { border-color: var(--line-strong); background: var(--card); }
  svg { width: 18px; height: 18px; }
  .badge {
    position: absolute; top: -6px; right: -6px; min-width: 18px; height: 18px; padding: 0 5px; display: grid; place-items: center;
    border-radius: 9px; background: var(--accent); color: var(--accent-ink); font-size: 0.68rem; font-weight: 600; line-height: 1;
  }
  .panel {
    position: absolute; right: 0; top: calc(100% + 8px); z-index: 30; width: min(320px, calc(100vw - 32px));
    max-height: min(50dvh, 360px); overflow-y: auto;
    padding: 12px 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--card-raised);
    box-shadow: 0 12px 28px color-mix(in srgb, var(--bg) 70%, transparent);
  }
  .panel[hidden] { display: none; }
  .heading { margin: 0 0 6px; font: 600 1rem/1.3 var(--serif); color: var(--ink); }
  .empty { margin: 0; color: var(--muted); font-size: 0.85rem; }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .item { display: flex; flex-direction: column; gap: 4px; padding-bottom: 10px; border-bottom: 1px solid var(--line-soft); }
  .item:last-child { padding-bottom: 0; border-bottom: 0; }
  .kind { font-size: 11px; font-weight: 600; color: var(--muted); }
  .kind.warn { color: var(--warn); }
  .kind.bad { color: var(--bad); }
  .kind.good { color: var(--good); }
  .title { font-size: 13px; font-weight: 500; color: var(--ink); overflow-wrap: anywhere; }
  .where { font-size: 12px; color: var(--muted); overflow-wrap: anywhere; }
  .actions { display: flex; align-items: center; gap: 8px; margin-top: 2px; }
  .btn {
    display: inline-flex; align-items: center; justify-content: center; min-height: 32px; padding: 0 10px;
    border-radius: 7px; font: 500 12px/1 var(--sans); text-decoration: none; cursor: pointer;
  }
  .btn.open { border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); }
  .btn.seen { border: 1px solid var(--line); background: transparent; color: var(--ink); }
  @media (max-width: 1099.98px), (pointer: coarse) {
    .bell-button { width: 44px; height: 44px; border-radius: 10px; }
    .btn { min-height: 44px; flex: 1 1 0; }
  }
  /* A phone: the panel spans the screen under the slim bar instead of hanging off the bell (it ran off the left edge). */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .panel { position: fixed; top: 64px; left: 16px; right: 16px; width: auto; }
    .bell-button { border: 0; }
    .bell-button svg { width: 22px; height: 22px; }
    .badge { top: 4px; right: 4px; }
  }
</style>
