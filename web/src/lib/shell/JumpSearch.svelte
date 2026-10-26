<script lang="ts">
  import { jumpMatches, type JumpItem } from './tabs'
  // "Jump to a session, project or machine": a combobox over what the workspace already lists (sessions, existing Projects
  // and machines), read when the field is first used and again after 15 seconds. Arrow Down moves into the results,
  // Escape closes them. On a desktop it sits in the top bar; on a tablet or phone it is a row under the bar that the
  // search button opens. It stores nothing.
  let { load, open = false, input = $bindable(), onfocuschange, onclose }: {
    load: () => Promise<JumpItem[]>
    open?: boolean
    input?: HTMLInputElement
    onfocuschange?: (focused: boolean) => void
    onclose?: () => void
  } = $props()

  const listId = $props.id()
  const LABEL = 'Jump to a session, project or machine'
  let query = $state('')
  let items = $state<JumpItem[]>([])
  let loadedAt = 0
  let loading = $state(false)
  let failed = $state(false)
  let showing = $state(false)
  let root = $state<HTMLElement>()
  let list = $state<HTMLElement>()
  const matches = $derived(jumpMatches(items, query))
  const expanded = $derived(showing && query.trim() !== '')

  async function refresh() {
    if (loading || Date.now() - loadedAt < 15000) return
    loading = true
    try { items = await load(); loadedAt = Date.now(); failed = false }
    catch { failed = true }
    finally { loading = false }
  }

  function options(): HTMLElement[] { return [...(list?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])] }
  function onInputKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' && expanded) { event.preventDefault(); options()[0]?.focus() }
    else if (event.key === 'Escape') {
      if (expanded) { event.preventDefault(); event.stopPropagation(); showing = false }
      else if (open) { event.preventDefault(); event.stopPropagation(); onclose?.() }
    }
  }
  function onOptionKey(event: KeyboardEvent) {
    const all = options(), at = all.indexOf(event.currentTarget as HTMLElement)
    if (event.key === 'ArrowDown') { event.preventDefault(); all[Math.min(all.length - 1, at + 1)]?.focus() }
    else if (event.key === 'ArrowUp') { event.preventDefault(); if (at <= 0) input?.focus(); else all[at - 1]?.focus() }
    else if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); showing = false; input?.focus() }
  }
  function focusIn(event: FocusEvent) {
    const from = event.relatedTarget
    if (from instanceof Node && root?.contains(from)) return
    showing = true; void refresh(); onfocuschange?.(true)
  }
  function focusOut(event: FocusEvent) {
    const next = event.relatedTarget
    if (next instanceof Node && root?.contains(next)) return
    showing = false
    onfocuschange?.(false)
  }
</script>

<div class="jump" class:open bind:this={root} role="search" onfocusin={focusIn} onfocusout={focusOut}>
  <svg class="glass" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
  <input class="field" bind:this={input} bind:value={query} type="search" role="combobox" aria-label={LABEL} placeholder={LABEL}
    aria-expanded={expanded} aria-controls={listId} aria-autocomplete="list" autocomplete="off" spellcheck="false"
    oninput={() => (showing = true)} onkeydown={onInputKey} />
  {#if open}<button type="button" class="close" aria-label="Close search" onclick={() => onclose?.()}>
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
  </button>{/if}
  <div class="results" hidden={!expanded}>
    <div role="listbox" id={listId} aria-label="Jump results" bind:this={list}>
      {#each matches as item (item.kind + item.href)}
        <a role="option" aria-selected="false" href={item.href} onkeydown={onOptionKey}>
          <span class="label">{item.label}</span><span class="detail">{item.detail}</span>
        </a>
      {/each}
    </div>
    {#if matches.length === 0}
      <p class="note" role="status">{loading && items.length === 0 ? 'Looking…' : failed && items.length === 0 ? 'Could not read the sessions, Projects and machines. Try again in a moment.' : 'Nothing matches.'}</p>
    {/if}
  </div>
</div>

<style>
  .jump {
    position: relative; display: flex; align-items: center; gap: 8px; width: clamp(200px, 22vw, 300px); height: 36px; padding: 0 10px;
    border: 1px solid var(--line); border-radius: 9px; background: var(--card); color: var(--muted);
  }
  .jump:focus-within { border-color: var(--line-strong); }
  .glass { flex: none; width: 16px; height: 16px; }
  /* Two classes: wins over the page-wide field rules (border, padding, 44 px height) in app.css. */
  .jump input.field {
    flex: 1; min-width: 0; min-height: 0; height: 100%; padding: 0; border: 0; border-radius: 0; outline: none; background: none; box-shadow: none;
    color: var(--ink); font: 400 0.88rem var(--sans);
  }
  input::placeholder { color: var(--muted); opacity: 1; }
  input::-webkit-search-cancel-button { display: none; }
  .jump:has(input:focus-visible) { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .close {
    display: grid; place-items: center; flex: none; width: 44px; height: 44px; margin-right: -8px; padding: 0;
    border: 0; border-radius: 10px; background: none; color: var(--muted); cursor: pointer;
  }
  .close svg { width: 20px; height: 20px; }
  .results {
    position: absolute; left: -1px; right: -1px; top: calc(100% + 6px); z-index: 30; max-height: min(60vh, 420px); overflow: auto;
    padding: 6px; border: 1px solid var(--line); border-radius: 12px; background: var(--card-raised);
    box-shadow: 0 12px 28px color-mix(in srgb, var(--bg) 70%, transparent);
  }
  .results[hidden] { display: none; }
  [role='listbox'] { display: grid; gap: 2px; }
  [role='option'] {
    display: grid; gap: 1px; min-height: 44px; align-content: center; padding: 6px 10px; border-radius: 8px;
    color: var(--ink); text-decoration: none; min-width: 0;
  }
  [role='option']:hover, [role='option']:focus { background: var(--tint); outline: none; }
  [role='option']:focus-visible { box-shadow: inset 0 0 0 2px var(--accent-text); }
  .label { font-weight: 500; overflow-wrap: anywhere; }
  .detail { color: var(--muted); font-size: 0.75rem; overflow-wrap: anywhere; }
  .note { margin: 0; padding: 10px; color: var(--muted); font-size: 0.85rem; }
  /* Tablet and phone: a row under the top bar, opened by the search button; 16 px text so focusing never zooms. */
  @media (max-width: 1099.98px) {
    .jump { display: none; }
    .jump.open {
      display: flex; position: absolute; left: 0; right: 0; top: 100%; z-index: 25; width: auto; height: 60px; padding: 0 16px;
      border: 0; border-bottom: 1px solid var(--line); border-radius: 0; background: var(--bg);
    }
    .jump.open:has(input:focus-visible) { outline: none; box-shadow: inset 0 -2px 0 var(--accent-text); }
    .jump input.field { font-size: 16px; }
    .results { left: 8px; right: 8px; top: calc(100% + 4px); }
  }
</style>
