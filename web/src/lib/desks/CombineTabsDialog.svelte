<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'

  // Combine tabs… (the Layout menu): the panes of the tabs chosen here move into the current tab and are laid out. All
  // the tabs that fit start ticked; a tab is taken whole, and one that would take the tab past its limit cannot be ticked.
  type Layout = 'side' | 'stack' | 'grid'
  let { name, room, limit, tabs, oncombine, oncancel }: {
    /** The current tab, which the others join. */
    name: string
    /** How many more panes the current tab can take. */
    room: number
    /** The most panes a tab holds. */
    limit: number
    /** The other tabs that hold panes. */
    tabs: { id: string; name: string; panes: number }[]
    oncombine: (ids: string[], layout: Layout) => void
    oncancel: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
  // svelte-ignore state_referenced_locally
  let picked = $state<string[]>((() => { let left = room; return tabs.filter(t => t.panes <= left && ((left -= t.panes), true)).map(t => t.id) })())
  let layout = $state<Layout>('side')
  const used = $derived(tabs.filter(t => picked.includes(t.id)).reduce((n, t) => n + t.panes, 0))
  const fits = (t: { id: string; panes: number }) => picked.includes(t.id) || used + t.panes <= room
  const allFit = $derived(tabs.reduce((n, t) => n + t.panes, 0) <= room)
  const LAYOUTS: { name: Layout; label: string }[] = [{ name: 'side', label: 'Side by side' }, { name: 'stack', label: 'Stacked' }, { name: 'grid', label: 'Grid' }]

  onMount(() => {
    dialog?.showModal()
    return () => dialog?.close()
  })
  function done(run: () => void) { dialog?.close(); run() }
  function toggle(id: string, on: boolean) { picked = on ? [...picked, id] : picked.filter(x => x !== id) }
  function all(on: boolean) {
    if (!on) { picked = []; return }
    let left = room
    picked = tabs.filter(t => t.panes <= left && ((left -= t.panes), true)).map(t => t.id)
  }
  function submit(event: SubmitEvent) {
    event.preventDefault()
    if (picked.length === 0) return
    // In tab order, whatever order they were ticked in.
    done(() => oncombine(tabs.filter(t => picked.includes(t.id)).map(t => t.id), layout))
  }
</script>

<dialog bind:this={dialog} class="combine" use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={event => { event.preventDefault(); done(oncancel) }}>
  <form class="box" data-shell-card onsubmit={submit}>
    <h2 id={titleId}>Combine tabs into “{name}”</h2>
    <p>The panes of the tabs you choose move into this tab. Nothing is restarted, and a pane's menu can move it back out.</p>
    <fieldset>
      <legend>Tabs</legend>
      <label class="row all"><input type="checkbox" checked={picked.length > 0 && tabs.every(t => picked.includes(t.id) || !fits(t))} indeterminate={picked.length > 0 && tabs.some(t => !picked.includes(t.id) && fits(t))} onchange={event => all(event.currentTarget.checked)} /><span>{allFit ? 'All tabs' : 'All that fit'}</span></label>
      {#each tabs as t (t.id)}
        <label class="row" class:off={!fits(t)}><input type="checkbox" checked={picked.includes(t.id)} disabled={!fits(t)} onchange={event => toggle(t.id, event.currentTarget.checked)} /><span class="name">{t.name}</span><span class="count">{t.panes === 1 ? '1 pane' : `${t.panes} panes`}{fits(t) ? '' : ' · does not fit'}</span></label>
      {/each}
    </fieldset>
    <p class="room" aria-live="polite">{limit - room + used} of {limit} panes in this tab.</p>
    <fieldset>
      <legend>Layout</legend>
      <div class="layouts">
        {#each LAYOUTS as l (l.name)}<label class="row"><input type="radio" name="combine-layout" value={l.name} bind:group={layout} /><span>{l.label}</span></label>{/each}
      </div>
    </fieldset>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" onclick={() => done(oncancel)}>Cancel</button>
      <button type="submit" class="btn primary" disabled={picked.length === 0}>{picked.length === 1 ? 'Combine 1 tab' : `Combine ${picked.length} tabs`}</button>
    </div>
  </form>
</dialog>

<style>
  dialog { max-width: min(460px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); overflow-wrap: anywhere; }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; text-wrap: balance; }
  p { margin: 8px 0 0; color: var(--muted); }
  fieldset { margin: 16px 0 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 6px; font-weight: 600; }
  .row { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 2px 0; cursor: pointer; }
  .row.all { border-bottom: 1px solid var(--line); margin-bottom: 4px; }
  .row.off { color: var(--faint); cursor: default; }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .count { color: var(--muted); font-size: 14px; white-space: nowrap; }
  .room { font-size: 14px; }
  .layouts { display: flex; gap: 4px 18px; flex-wrap: wrap; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; flex-wrap: wrap; }
  @media (pointer: coarse) { .row { min-height: 44px; } }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
</style>
