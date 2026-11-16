<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte'
  import { PROJECT_COLOURS, projectColour, type ProjectColour } from '../projectColour'

  // One small menu for the Desks screens (a desk card, the desk switcher and menu, a tab, a pane). It opens at a point,
  // takes focus on its first item, moves with the arrows and Home/End, and closes on Escape, a press outside or Tab,
  // giving focus back to what opened it. The colour row is project colour's palette and swatches.
  let { x, y, label, opener, onclose, colour, oncolour, children }: {
    x: number
    y: number
    label: string
    opener?: HTMLElement | null
    onclose: (focus: boolean) => void
    /** With oncolour, draws the palette as a row of swatches with this one ticked. */
    colour?: ProjectColour
    oncolour?: (colour: ProjectColour) => void
    children: Snippet
  } = $props()

  let menu = $state<HTMLElement>()
  const items = () => [...(menu?.querySelectorAll<HTMLElement>('[role^=menuitem]:not(:disabled)') ?? [])]
  onMount(() => { void tick().then(() => items()[0]?.focus()) })

  function keys(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onclose(true); return }
    if (event.key === 'Tab') { event.preventDefault(); onclose(true); return }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End', 'ArrowLeft', 'ArrowRight'].includes(event.key)) return
    const list = items(), at = list.indexOf(document.activeElement as HTMLElement)
    const inRow = (document.activeElement as HTMLElement | null)?.closest('.colours')
    if ((event.key === 'ArrowLeft' || event.key === 'ArrowRight') && !inRow) return
    event.preventDefault()
    const step = event.key === 'ArrowDown' || event.key === 'ArrowRight' ? 1 : -1
    list[event.key === 'Home' ? 0 : event.key === 'End' ? list.length - 1 : (at + step + list.length) % list.length]?.focus()
  }
  // Kept inside the window, whatever the point.
  const left = $derived(Math.min(Math.max(8, x), Math.max(8, innerWidth - 248)))
  const top = $derived(Math.min(Math.max(8, y), Math.max(8, innerHeight - 320)))
</script>

<svelte:window onpointerdown={event => { if (menu && !menu.contains(event.target as Node) && !opener?.contains(event.target as Node)) onclose(false) }} />

<div class="desk-menu" role="menu" aria-label={label} tabindex="-1" bind:this={menu} style={`left:${left}px;top:${top}px`} onkeydown={keys}>
  {@render children()}
  {#if oncolour}
    <span class="label">Colour</span>
    <div class="colours">{#each PROJECT_COLOURS as choice (choice.id)}<button type="button" role="menuitemradio" aria-checked={projectColour(colour) === choice.id} aria-label={choice.name} title={choice.name} style={`--swatch:${choice.hex}`} onclick={() => oncolour?.(choice.id)}>{choice.id === '' ? '×' : ''}</button>{/each}</div>
  {/if}
</div>

<style>
  .desk-menu { position: fixed; z-index: 60; min-width: 210px; max-width: 280px; display: flex; flex-direction: column; padding: 4px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised); box-shadow: 0 14px 34px var(--scrim); font-size: 14px; }
  .desk-menu :global(:is(button, a)[role^='menuitem']:not(.colours button)) { min-height: 34px; padding: 0 10px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; text-align: left; text-decoration: none; display: flex; align-items: center; gap: 8px; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .desk-menu :global(:is(button, a)[role^='menuitem']:not(.colours button):hover:not(:disabled)) { background: var(--line-soft); }
  .desk-menu :global(:is(button, a)[role^='menuitem']:not(.colours button):focus-visible) { background: var(--line-soft); outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .desk-menu :global(button[role^='menuitem']:disabled) { color: var(--faint); cursor: default; }
  .desk-menu :global(.danger) { color: var(--bad); }
  .desk-menu :global(hr) { width: 100%; margin: 4px 0; border: 0; border-top: 1px solid var(--line-soft); }
  .desk-menu :global(.swatch) { width: 10px; height: 10px; border-radius: 3px; flex: none; background: var(--swatch, transparent); border: 1px solid var(--line-strong); }
  .label { padding: 5px 10px 1px; color: var(--muted); font-size: 14px; }
  .colours { display: flex; gap: 7px; padding: 4px 10px 6px; }
  .colours button { min-height: 0; width: 20px; height: 20px; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--swatch); color: var(--ink); font-size: 14px; line-height: 1; cursor: pointer; }
  .colours button[aria-checked='true'] { box-shadow: 0 0 0 2px var(--accent); }
  .colours button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  @media (pointer: coarse) {
    .desk-menu :global(:is(button, a)[role^='menuitem']:not(.colours button)) { min-height: 44px; }
    .colours { gap: 4px; }
    .colours { flex-wrap: wrap; padding: 0 2px; }
    .colours button { width: 44px; height: 44px; padding: 11px; border: 0; background: var(--swatch) content-box; outline: 1px solid var(--line-strong); outline-offset: -11px; }
    .colours button[aria-checked='true'] { box-shadow: none; outline: 2px solid var(--accent); outline-offset: -9px; }
  }
</style>
