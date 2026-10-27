<script lang="ts">
  // The bell: a button named for the Needs you count, with a badge only when something waits. It opens a small panel that
  // says what waits; today nothing signals that a session needs the person, so the count is 0 (PR G brings the cards).
  let { count }: { count: number } = $props()
  let open = $state(false)
  let root = $state<HTMLElement>()
  let button = $state<HTMLButtonElement>()
  const panelId = $props.id()
  const badge = $derived(count > 99 ? '99+' : String(count))

  function close(focus: boolean) {
    open = false
    if (focus) button?.focus()
  }
</script>

<svelte:window
  onkeydown={(event) => { if (event.key === 'Escape' && open) { event.preventDefault(); close(true) } }}
  onclick={(event) => { if (open && event.target instanceof Node && !root?.contains(event.target)) close(false) }}
/>

<div class="bell" bind:this={root}>
  <button type="button" class="bell-button" class:waiting={count > 0} bind:this={button} aria-label={`Needs you: ${count}`} aria-expanded={open} aria-controls={panelId} onclick={() => (open = !open)}>
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 17V11a6 6 0 0112 0v6l1.5 2h-15z" /><path d="M10 21h4" /></svg>
    {#if count > 0}<span class="badge" aria-hidden="true">{badge}</span>{/if}
  </button>
  <div class="panel" id={panelId} hidden={!open}>
    <p class="title">Needs you</p>
    {#if count === 0}
      <p class="none">Nothing needs you.</p>
    {:else}
      <p>{count === 1 ? '1 session needs you.' : `${count} sessions need you.`}</p>
      <a href="/">Open Home</a>
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
  .bell-button:hover { border-color: var(--line-strong); background: var(--card); }
  .bell-button[aria-expanded='true'] { border-color: var(--line-strong); background: var(--card); }
  svg { width: 18px; height: 18px; }
  .badge {
    position: absolute; top: -6px; right: -6px; min-width: 18px; height: 18px; padding: 0 5px; display: grid; place-items: center;
    border-radius: 9px; background: var(--accent); color: var(--accent-ink); font-size: 0.68rem; font-weight: 600; line-height: 1;
  }
  .panel {
    position: absolute; right: 0; top: calc(100% + 8px); z-index: 30; width: min(280px, calc(100vw - 32px));
    padding: 12px 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--card-raised);
    box-shadow: 0 12px 28px color-mix(in srgb, var(--bg) 70%, transparent);
  }
  .panel[hidden] { display: none; }
  .panel p { margin: 0; color: var(--ink); font-size: 0.85rem; }
  .panel .title { margin-bottom: 6px; font: 600 1rem/1.3 var(--serif); }
  .panel .none { color: var(--muted); }
  .panel a { display: inline-flex; align-items: center; min-height: 40px; font-size: 0.85rem; }
  @media (max-width: 1099.98px), (pointer: coarse) {
    .bell-button { width: 44px; height: 44px; border-radius: 10px; }
  }
  /* A phone: the panel spans the screen under the slim bar instead of hanging off the bell (it ran off the left edge). */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .panel { position: fixed; top: 64px; left: 16px; right: 16px; width: auto; }
  }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .bell-button { border: 0; }
    .bell-button svg { width: 22px; height: 22px; }
    .badge { top: 4px; right: 4px; }
  }
</style>
