<script lang="ts">
  import { tick } from 'svelte'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import DeskMenu from './DeskMenu.svelte'
  import type { Tab } from './tree'

  // The desk's tab bar: flat tabs with a colour marker, and an underline in the tab's colour on the current one. Click to
  // switch; double-click or F2 to rename in place; drag to reorder; a menu (right-click, Shift+F10 or ⋮) with Rename,
  // Colour, Duplicate tab and Close tab; + opens New tab. A hidden tab whose shell waits for the person carries a mark.
  let { tabs, active, panelId, waiting, full = false, onselect, onrename, oncolour, onduplicate, onclose, onreorder, onnew }: {
    tabs: Tab[]
    active: string
    /** The id of the element the tabs control. */
    panelId: string
    /** Tabs whose shell is waiting for the person. */
    waiting: ReadonlySet<string>
    /** At the 20-tab limit: + and Duplicate are off. */
    full?: boolean
    onselect: (id: string) => void
    onrename: (id: string, name: string) => void
    oncolour: (id: string, colour: ProjectColour) => void
    onduplicate: (id: string) => void
    onclose: (id: string) => void
    onreorder: (ids: string[]) => void
    onnew: (opener: HTMLElement) => void
  } = $props()

  let bar = $state<HTMLElement>()
  const label = (t: Tab) => t.name || 'Untitled'
  const tabButton = (id: string) => bar?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(id)}"] [role=tab]`)

  // Rename in place.
  let renaming = $state(''), draft = $state(''), input = $state<HTMLInputElement>()
  async function startRename(id: string) {
    const t = tabs.find(x => x.id === id)
    if (!t) return
    renaming = id; draft = t.name
    await tick()
    input?.select()
  }
  async function finishRename(save: boolean) {
    const id = renaming
    if (!id) return
    renaming = ''
    const t = tabs.find(x => x.id === id), next = draft.trim()
    if (save && t && next && next !== t.name && [...next].length <= 80) onrename(id, next)
    await tick()
    tabButton(id)?.focus()
  }

  // The tab menu.
  let menu = $state<{ id: string; x: number; y: number; opener: HTMLElement } | null>(null)
  const menuTab = $derived(menu ? tabs.find(t => t.id === menu?.id) : undefined)
  function openMenu(id: string, opener: HTMLElement, at?: { x: number; y: number }) {
    const box = opener.getBoundingClientRect()
    menu = { id, opener, x: at?.x ?? box.left, y: at?.y ?? box.bottom + 4 }
  }
  /** Move left or right from the menu: the same reorder a drag makes, for a keyboard. */
  function shift(id: string, by: -1 | 1) {
    const ids = tabs.map(t => t.id), at = ids.indexOf(id), to = at + by
    if (at < 0 || to < 0 || to >= ids.length) return
    ids.splice(at, 1); ids.splice(to, 0, id)
    onreorder(ids)
    void tick().then(() => tabButton(id)?.focus())
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }
  function menuThen(run: (id: string) => void) { const id = menu?.id; closeMenu(false); if (id) run(id) }

  function keys(event: KeyboardEvent, id: string) {
    if (event.key === 'F2') { event.preventDefault(); void startRename(id); return }
    if ((event.key === 'F10' && event.shiftKey) || event.key === 'ContextMenu') { event.preventDefault(); openMenu(id, event.currentTarget as HTMLElement); return }
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const at = tabs.findIndex(t => t.id === id)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (at + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length
    const target = tabs[next]
    if (!target) return
    onselect(target.id)
    void tick().then(() => tabButton(target.id)?.focus())
  }

  // Drag to reorder: a press that moves more than a few pixels picks the tab up; the others make room as it passes their
  // middles, and letting go saves the new order. A press that does not move is an ordinary click.
  let drag: { id: string; x: number; pointer: number; moved: boolean } | null = null
  let preview = $state<string[] | null>(null), dragging = $state('')
  const order = $derived((preview ?? tabs.map(t => t.id)).map(id => tabs.find(t => t.id === id)).filter((t): t is Tab => !!t))
  function down(event: PointerEvent, id: string) {
    if (event.button !== 0 || renaming) return
    drag = { id, x: event.clientX, pointer: event.pointerId, moved: false }
  }
  function move(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointer) return
    if (!drag.moved) {
      if (Math.abs(event.clientX - drag.x) < 6) return
      drag.moved = true
      dragging = drag.id
      ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
    }
    const others = [...(bar?.querySelectorAll<HTMLElement>('[data-tab]') ?? [])].filter(el => el.dataset.tab !== drag?.id)
    const before = others.filter(el => { const r = el.getBoundingClientRect(); return r.left + r.width / 2 < event.clientX }).length
    const ids = others.map(el => el.dataset.tab!)
    ids.splice(before, 0, drag.id)
    if (!preview || preview.join() !== ids.join()) preview = ids
  }
  let suppressClick = false
  function up(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointer) return
    const moved = drag.moved, next = preview
    drag = null; dragging = ''; preview = null
    if (!moved) return
    suppressClick = true
    setTimeout(() => (suppressClick = false))
    if (next && next.join() !== tabs.map(t => t.id).join()) onreorder(next)
  }
  function cancelDrag() { drag = null; dragging = ''; preview = null }
</script>

<div class="bar" bind:this={bar} data-keeps-focus>
  <div class="tabs" role="tablist" aria-label="Tabs">
    {#each order as t (t.id)}
      {@const on = t.id === active}
      <div class="tab" class:on class:dragging={dragging === t.id} data-tab={t.id} style={`--tc:${t.colour ? projectColourHex(t.colour) : 'transparent'}`} data-colour={t.colour || undefined}>
        {#if renaming === t.id}
          <input class="rename" aria-label="Tab name" maxlength="80" bind:this={input} bind:value={draft}
            onkeydown={event => { if (event.key === 'Enter') { event.preventDefault(); void finishRename(true) } else if (event.key === 'Escape') { event.preventDefault(); void finishRename(false) } }}
            onblur={() => void finishRename(true)} />
        {:else}
          <button type="button" role="tab" id={`desk-tab-${t.id}`} aria-selected={on} aria-controls={panelId} tabindex={on ? 0 : -1}
            onclick={() => { if (!suppressClick) onselect(t.id) }} ondblclick={() => void startRename(t.id)} onkeydown={event => keys(event, t.id)}
            oncontextmenu={event => { event.preventDefault(); openMenu(t.id, event.currentTarget as HTMLElement, { x: event.clientX, y: event.clientY }) }}
            onpointerdown={event => down(event, t.id)} onpointermove={move} onpointerup={up} onpointercancel={cancelDrag}>
            <span class="marker" aria-hidden="true"></span>
            <span class="name">{label(t)}</span>
            {#if waiting.has(t.id) && !on}<span class="wait" title="Waiting for you"><span class="vh">, waiting for you</span></span>{/if}
          </button>
          <button type="button" class="more" aria-label={`Actions for tab ${label(t)}`} aria-haspopup="menu" aria-expanded={menu?.id === t.id} tabindex={on ? 0 : -1}
            onclick={event => openMenu(t.id, event.currentTarget as HTMLElement)}>⋮</button>
        {/if}
      </div>
    {/each}
  </div>
  <button type="button" class="add" aria-label="New tab" title={full ? 'A desk holds at most 20 tabs.' : 'New tab'} disabled={full} onclick={event => onnew(event.currentTarget as HTMLElement)}>+</button>
</div>

{#if menu && menuTab}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for tab ${label(menuTab)}`} opener={menu.opener} onclose={closeMenu} colour={menuTab.colour}
    oncolour={c => { const id = menu?.id; closeMenu(true); if (id) oncolour(id, c) }}>
    <button type="button" role="menuitem" onclick={() => menuThen(id => void startRename(id))}>Rename</button>
    <button type="button" role="menuitem" disabled={tabs[0]?.id === menuTab.id} onclick={() => menuThen(id => shift(id, -1))}>Move left</button>
    <button type="button" role="menuitem" disabled={tabs.at(-1)?.id === menuTab.id} onclick={() => menuThen(id => shift(id, 1))}>Move right</button>
    <button type="button" role="menuitem" disabled={full} onclick={() => menuThen(onduplicate)}>Duplicate tab</button>
    <button type="button" role="menuitem" class="danger" onclick={() => menuThen(onclose)}>Close tab</button>
  </DeskMenu>
{/if}

<style>
  .bar { flex: none; display: flex; align-items: flex-end; gap: 2px; padding: 0 10px; border-bottom: 1px solid var(--line); min-height: 40px; }
  .tabs { display: flex; align-items: flex-end; gap: 2px; min-width: 0; overflow-x: auto; scrollbar-width: none; }
  .tabs::-webkit-scrollbar { display: none; }
  .tab { position: relative; display: flex; align-items: center; flex: none; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .tab.on { border-bottom-color: var(--tc); }
  .tab.on:not([data-colour]) { border-bottom-color: var(--accent); }
  .tab.dragging { opacity: 0.7; }
  .tab [role='tab'] { display: flex; align-items: center; gap: 8px; min-height: 38px; padding: 6px 4px 6px 12px; border: 0; background: none; color: var(--muted); font-size: 14px; white-space: nowrap; cursor: pointer; touch-action: pan-y; user-select: none; }
  .tab.on [role='tab'] { color: var(--ink); font-weight: 600; }
  .tab [role='tab']:hover { color: var(--ink); }
  .tab [role='tab']:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; border-radius: 6px; }
  .marker { width: 3px; height: 14px; border-radius: 2px; background: var(--tc); flex: none; }
  .name { max-width: 22ch; overflow: hidden; text-overflow: ellipsis; }
  .wait { width: 7px; height: 7px; border-radius: 1px; transform: rotate(45deg); background: var(--status-waiting); flex: none; }
  .more { width: 28px; height: 28px; border: 0; border-radius: 6px; background: transparent; color: var(--faint); cursor: pointer; opacity: 0; }
  .tab:hover .more, .tab.on .more, .more:focus-visible, .more[aria-expanded='true'] { opacity: 1; }
  .more:hover { color: var(--ink); background: var(--line-soft); }
  .rename { min-height: 30px; margin: 4px 6px; padding: 2px 8px; width: 18ch; font-weight: 600; }
  .add { flex: none; min-width: 36px; min-height: 36px; margin-bottom: 1px; border: 0; border-radius: 6px; background: none; color: var(--muted); font-size: 20px; line-height: 1; cursor: pointer; }
  .add:hover:not(:disabled) { color: var(--ink); background: var(--line-soft); }
  .add:disabled { color: var(--faint); cursor: default; }
  @media (pointer: coarse) {
    .tab [role='tab'] { min-height: 44px; }
    .more { width: 44px; height: 44px; opacity: 1; }
    .add { min-width: 44px; min-height: 44px; }
  }
</style>
