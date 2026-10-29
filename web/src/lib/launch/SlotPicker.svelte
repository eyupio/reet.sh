<script lang="ts" module>
  /** One choice in a slot's list. A disabled one stays in the list (it can be focused and read) with its reason beside it. */
  export type SlotOption = { value: string; label: string; detail?: string; group?: string; disabled?: boolean; reason?: string; starred?: boolean; starLabel?: string; onstar?: () => void }
</script>

<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { shell } from '../dialogShell'

  // One slot of the launch sentence: a listbox in a popover above its button with a mouse, or in a bottom sheet on touch.
  // Arrow keys, Home and End move through the options, Enter or Space chooses, Escape (or Close) closes and gives focus back
  // to the slot's button; Tab from the popover closes it and moves on from the slot. A filter, when there is one, narrows the
  // list as it is typed (or asks the page to, for a list read from a machine). `other` adds a typed value (the model's Other…).
  let { label, options, value, sheet, opener, filter, onfilter, other, status = '', statusAction, onpick, onclose }: {
    label: string
    options: SlotOption[]
    value: string
    /** A bottom sheet (touch) instead of a popover (mouse). */
    sheet: boolean
    opener: HTMLElement | null
    /** The filter field's label; no filter when absent. */
    filter?: string
    /** Filters by asking the page (the list is read again); without it the options are filtered here by their label. */
    onfilter?: (text: string) => void
    other?: { label: string; field: string; validate: (text: string) => string | null }
    /** A line under the list: reading, or why the list is short. */
    status?: string
    /** An action beside the status line, such as Refresh. */
    statusAction?: { label: string; onclick: () => void; disabled?: boolean }
    onpick: (value: string) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  // The button that opened this, kept as it was: the page may drop the slot it came from while this closes.
  const from = untrack(() => opener)
  const OTHER = '\u0000other'
  let text = $state(''), typed = $state(''), typing = $state(false), tried = $state(false), announcement = $state('')
  let dialog = $state<HTMLDialogElement>(), pop = $state<HTMLDivElement>(), list = $state<HTMLDivElement>(), filterInput = $state<HTMLInputElement>(), otherInput = $state<HTMLInputElement>()
  let place = $state({ bottom: 0, left: 0, width: 0, max: 320 })

  const shown = $derived(onfilter || !text.trim() ? options : options.filter(o => o.label.toLowerCase().includes(text.trim().toLowerCase())))
  const typedOther = $derived(other && text.trim() && !shown.some(o => o.label.toLowerCase() === text.trim().toLowerCase()) ? `Use ‘${text.trim()}’` : '')
  const all = $derived(other ? [...shown, { value: OTHER, label: typedOther || other.label }] : shown)
  // Groups in the order they first appear; options without a group are drawn without a heading.
  const groups = $derived(all.reduce<{ name: string; items: SlotOption[] }[]>((out, o) => {
    const name = o.group ?? ''
    const g = out.find(x => x.name === name)
    if (g) g.items.push(o); else out.push({ name, items: [o] })
    return out
  }, []))
  const otherError = $derived(other && (tried || typed !== '') ? (typed.trim() === '' ? (tried ? `Type a ${other.field.toLowerCase()}.` : '') : other.validate(typed) ?? '') : '')

  function close(returnFocus = true) {
    // A modal sheet makes everything else inert: it closes before focus goes back.
    if (sheet && dialog?.open) dialog.close()
    if (returnFocus && from?.isConnected) from.focus()
    onclose()
  }
  function choose(o: SlotOption) {
    if (o.disabled) return
    if (o.value === OTHER) {
      if (text.trim()) typed = text.trim()
      typing = true
      void tick().then(() => otherInput?.focus())
      return
    }
    onpick(o.value)
    close()
  }
  function useTyped() {
    tried = true
    if (!other || typed.trim() === '' || other.validate(typed)) { otherInput?.focus(); return }
    onpick(typed.trim())
    close()
  }
  const items = () => [...(list?.querySelectorAll<HTMLElement>('[role=option]') ?? [])]
  async function star(o: SlotOption) {
    const value = o.value
    o.onstar?.()
    announcement = o.starred ? `Unstarred ${o.label}` : `Starred ${o.label}`
    await tick()
    const row = [...(list?.querySelectorAll<HTMLElement>('.option-row') ?? [])].find(item => item.querySelector<HTMLElement>('[role=option]')?.dataset.value === value)
    const next = row?.querySelector<HTMLElement>('[role=option]') ?? list
    next?.focus()
  }
  function keys(event: KeyboardEvent) {
    if ((event.target as HTMLElement)?.closest('.star')) {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
      return
    }
    const all = items(), at = all.indexOf(document.activeElement as HTMLElement)
    const move = (i: number) => { event.preventDefault(); all[(i + all.length) % all.length]?.focus() }
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
    else if (event.key === 'ArrowDown') move(at + 1)
    else if (event.key === 'ArrowUp') at <= 0 && filterInput ? (event.preventDefault(), filterInput.focus()) : move(at - 1)
    else if (event.key === 'Home' && at >= 0) move(0)
    else if (event.key === 'End' && at >= 0) move(all.length - 1)
    else if ((event.key === 'Enter' || event.key === ' ') && at >= 0) { event.preventDefault(); all[at].click() }
    else if ((event.key.toLowerCase() === 's' || event.key === 'S') && at >= 0) {
      const star = all[at].parentElement?.querySelector<HTMLButtonElement>('button.star')
      if (star) { event.preventDefault(); star.click() }
    }
    else if (event.key === 'Tab' && !sheet) tabOut()
  }
  // Tab from the popover (drawn at the end of the page) gives focus back to the slot first, so the browser's own Tab move
  // goes on from there: to the next slot, or the one before with Shift.
  function tabOut() { close(true) }
  function filterKeys(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
    else if (event.key === 'Tab' && !sheet) tabOut()
    else if (event.key === 'ArrowDown') { event.preventDefault(); items()[0]?.focus() }
    else if (event.key === 'Enter') { event.preventDefault(); const first = all.find(o => !o.disabled); if (first) choose(first) }
  }
  let timer: ReturnType<typeof setTimeout> | undefined
  function filtered() {
    if (!onfilter) return
    clearTimeout(timer)
    const ask = onfilter
    timer = setTimeout(() => ask(text.trim()), 250)
  }

  // The popover opens above its button (the bar is at the foot of the page), as wide as it needs within the window.
  function measure() {
    if (sheet || !from) return
    const r = from.getBoundingClientRect()
    const width = Math.min(360, innerWidth - 16)
    place = { bottom: innerHeight - r.top + 6, left: Math.max(8, Math.min(r.left, innerWidth - width - 8)), width, max: Math.max(160, r.top - 16) }
  }
  function outside(event: PointerEvent) {
    if (sheet) return
    const target = event.target as Node
    if (pop?.contains(target) || from?.contains(target)) return
    close(false)
  }

  onMount(() => {
    measure()
    if (sheet) dialog?.showModal()
    void tick().then(() => {
      const selected = items().find(el => el.getAttribute('aria-selected') === 'true') ?? items().find(el => el.getAttribute('aria-disabled') !== 'true')
      ;(filterInput ?? selected ?? list)?.focus()
    })
    addEventListener('resize', measure)
    return () => { clearTimeout(timer); removeEventListener('resize', measure); if (sheet) dialog?.close() }
  })
</script>

<svelte:document onpointerdown={outside} />

{#snippet row(o: SlotOption)}
  <div class="option-row">
    <div class="option" role="option" tabindex="-1" data-value={o.value} aria-selected={o.value === value && o.value !== OTHER}
      aria-disabled={o.disabled ? 'true' : undefined} onclick={() => choose(o)} onkeydown={() => {}}>
      <span class="name">{o.label}</span>
      {#if o.detail}<span class="detail">{o.detail}</span>{/if}
      {#if o.reason}<span class="reason">{o.reason}</span>{/if}
    </div>
    {#if o.onstar}
      <button type="button" class="star" aria-pressed={o.starred ? 'true' : 'false'}
        aria-label={o.starLabel ?? (o.starred ? `Unstar ${o.label}` : `Star ${o.label}`)}
        title={o.starLabel ?? (o.starred ? `Unstar ${o.label}` : `Star ${o.label}`)}
        onclick={(event) => { event.stopPropagation(); void star(o) }}>
        <span aria-hidden="true">{o.starred ? '★' : '☆'}</span>
      </button>
    {/if}
  </div>
{/snippet}

{#snippet body()}
  {#if filter}
    <input class="filter" type="search" bind:this={filterInput} bind:value={text} aria-label={filter} placeholder={filter} autocomplete="off" spellcheck="false"
      aria-controls={`${id}-list`} oninput={filtered} onkeydown={filterKeys} />
  {/if}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="list" id={`${id}-list`} role="listbox" aria-label={label} tabindex="-1" bind:this={list} onkeydown={keys}>
    {#each groups as g, gi (g.name)}
      {#if g.name}
        <div class="group" role="group" aria-labelledby={`${id}-g-${gi}`}>
          <div class="heading" id={`${id}-g-${gi}`} role="presentation">{g.name}</div>
          {#each g.items as o (o.value)}{@render row(o)}{/each}
        </div>
      {:else}
        {#each g.items as o (o.value)}{@render row(o)}{/each}
      {/if}
    {/each}
    {#if all.length === 0}<p class="empty">Nothing matches.</p>{/if}
  </div>
  {#if typing && other}
    <form class="other" onsubmit={(e) => { e.preventDefault(); useTyped() }}>
      <label for={`${id}-other`}>{other.field}</label>
      <span class="other-row">
        <input id={`${id}-other`} bind:this={otherInput} bind:value={typed} autocomplete="off" spellcheck="false" maxlength="100"
          aria-invalid={otherError ? 'true' : undefined} aria-describedby={otherError ? `${id}-other-error` : undefined}
          onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close() } }} />
        <button type="submit" class="use">Use</button>
      </span>
      {#if otherError}<p class="error" id={`${id}-other-error`}>{otherError}</p>{/if}
    </form>
  {/if}
  <p class="sr-only" aria-live="polite">{announcement}</p>
  {#if status || statusAction}
    <p class="status" role="status" aria-live="polite">
      {#if status}{status}{/if}
      {#if statusAction}<button type="button" class="status-action" disabled={statusAction.disabled} onclick={statusAction.onclick}>{statusAction.label}</button>{/if}
    </p>
  {/if}
{/snippet}

{#if sheet}
  <dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`}
    oncancel={(event) => { event.preventDefault(); close() }} onclick={(event) => { if (event.target === dialog) close() }}>
    <div class="sheet" data-shell-card>
      <header data-shell-head>
        <h2 id={`${id}-title`}>{label}</h2>
        <button type="button" class="close" data-shell-close aria-label="Close" onclick={() => close()}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
      </header>
      <div class="body" data-shell-body>{@render body()}</div>
    </div>
  </dialog>
{:else}
  <div class="pop" bind:this={pop} style:bottom={`${place.bottom}px`} style:left={`${place.left}px`} style:width={`${place.width}px`} style:max-height={`${place.max}px`}>
    {@render body()}
  </div>
{/if}

<style>
  .pop {
    position: fixed; z-index: 30; display: flex; flex-direction: column; gap: 6px; padding: 6px; overflow: auto;
    border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card-raised); box-shadow: 0 12px 30px var(--scrim);
    color: var(--ink); font-size: 14px; line-height: 1.3;
  }
  .list { display: flex; flex-direction: column; gap: 2px; min-height: 0; overflow: auto; outline: none; }
  .group { display: flex; flex-direction: column; gap: 2px; }
  .group + .group { margin-top: 6px; }
  .heading { padding: 4px 10px 2px; font-size: 11px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
  .option-row { display: flex; align-items: stretch; gap: 4px; scroll-margin-top: 36px; }
  .option-row + .option-row { margin-top: 2px; }
  .option-row > .option { flex: 1; min-width: 0; }
  .star {
    flex-shrink: 0; display: grid; place-items: center; min-width: 34px; min-height: 34px; padding: 4px;
    border: 1px solid transparent; border-radius: 8px; background: transparent; color: var(--muted); font-size: 15px; cursor: pointer;
  }
  .star[aria-pressed=true] { color: var(--accent-text); border-color: var(--line); }
  .star:hover { background: var(--tint); border-color: var(--line); }
  .status-action {
    margin-left: 8px; padding: 2px 8px; border: 1px solid var(--line); border-radius: 6px;
    background: var(--card); color: var(--accent-text); font: inherit; font-size: .7rem; cursor: pointer;
  }
  .status-action:disabled { opacity: .45; cursor: default; }
  .heading { position: sticky; top: 0; z-index: 1; background: var(--card-raised); pointer-events: none; }
  .option {
    display: flex; flex-direction: column; gap: 1px; min-height: 34px; justify-content: center; padding: 6px 10px; border-radius: 8px;
    color: var(--ink); cursor: pointer;
  }
  .option:hover, .option:focus-visible { background: var(--line-soft); }
  .option:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .option[aria-selected='true'] { box-shadow: inset 3px 0 0 var(--accent-text); font-weight: 600; }
  .option[aria-disabled='true'] { cursor: default; }
  .option[aria-disabled='true'] .name { color: var(--muted); }
  .detail { font: 12px/1.3 var(--mono); color: var(--muted); overflow-wrap: anywhere; }
  .reason { font-size: 12px; color: var(--ink-soft); font-weight: 400; overflow-wrap: anywhere; }
  .name { overflow-wrap: anywhere; }
  .filter, .other input {
    width: 100%; min-width: 0; min-height: 36px; padding: 6px 10px; font: inherit; font-size: 14px; color: var(--ink);
    background: var(--bg); border: 1px solid var(--line); border-radius: 8px;
  }
  .filter { position: sticky; top: 0; z-index: 2; background: var(--card-raised); }
  .heading { top: 36px; }
  .other { display: grid; gap: 4px; padding: 4px; }
  .other label { font-size: 12px; font-weight: 600; color: var(--muted); }
  .other-row { display: flex; gap: 6px; }
  .use { flex: none; min-height: 36px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 8px; background: transparent; color: var(--ink); font: inherit; font-weight: 600; cursor: pointer; }
  .error { margin: 0; font-size: 12px; color: var(--bad); }
  [aria-invalid='true'] { border-color: var(--bad); }
  .status, .empty { margin: 0; padding: 4px 10px; font-size: 12px; color: var(--muted); }

  dialog { width: min(420px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; overflow: hidden; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 32px); overflow: hidden; padding: 8px 8px 12px; border: 1px solid var(--line); border-radius: 16px; background: var(--card-raised); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 4px 4px 12px; }
  h2 { margin: 0; font: 600 1.15rem/1.3 var(--serif); }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--muted); cursor: pointer; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 20px; height: 20px; }
  .body { display: flex; flex-direction: column; gap: 8px; min-height: 0; flex: 1; overflow: auto; padding: 4px; }
  /* On touch every row is a thumb-sized target, and fields are 16 px so focusing never zooms. */
  /* At phone width typed fields are 16 px whatever the pointer, so focusing never zooms. */
  @media (max-width: 650px) { .filter, .other input { font-size: 16px; } }
  @media (pointer: coarse) {
    .option { min-height: 48px; padding: 8px 12px; font-size: 15px; }
    .filter, .other input { min-height: 44px; font-size: 16px; }
    .use, .star, .status-action { min-width: 44px; min-height: 44px; }
    .heading { top: 44px; }
  }
</style>
