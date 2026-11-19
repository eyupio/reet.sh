<script lang="ts">
  import { tick } from 'svelte'
  import type { Tab } from './skillsState.svelte'

  // The bar that acts on the selected skills: Archive (or Restore on the Archived tab), Delete, Add tag and Cancel.
  let { count, tab, tagNames, onarchive, onrestore, ondelete, ontag, oncancel }: {
    count: number
    tab: Tab
    tagNames: string[]
    onarchive: () => void
    onrestore: () => void
    ondelete: () => void
    ontag: (name: string) => void
    oncancel: () => void
  } = $props()

  let tagging = $state(false)
  let tagDraft = $state('')
  let bar = $state<HTMLElement>()
  let tagButton = $state<HTMLButtonElement>()

  // A toolbar: one Tab stop, the button last used in the bar (at first its first one that works), and the arrow keys,
  // Home and End to move along it. The Add tag step is a small form instead: its field and buttons keep plain Tab
  // order, and the field keeps its own keys.
  let current = $state<HTMLElement | null>(null)
  const controls = () => (bar ? [...bar.querySelectorAll<HTMLElement>('input, button:not(:disabled)')] : [])
  $effect(() => {
    void [tagging, tagDraft, count, current]
    const list = controls()
    if (tagging) {
      for (const el of list) el.tabIndex = 0
      return
    }
    const stop = current && list.includes(current) ? current : list[0]
    for (const el of list) el.tabIndex = el === stop ? 0 : -1
  })
  function keys(e: KeyboardEvent) {
    if (tagging || !['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(e.key) || !(e.target instanceof HTMLButtonElement)) return
    const list = controls()
    const at = list.indexOf(e.target)
    if (at < 0) return
    const to = e.key === 'Home' ? 0 : e.key === 'End' ? list.length - 1 : (at + (e.key === 'ArrowRight' ? 1 : list.length - 1)) % list.length
    e.preventDefault()
    list[to].focus()
  }
  function focused(e: FocusEvent) {
    if (e.target instanceof HTMLElement && controls().includes(e.target)) current = e.target
  }

  // Cancel (or Escape in the field) closes the step with nothing typed kept, and puts focus back on Add tag.
  async function stopTagging() {
    tagging = false
    tagDraft = ''
    await tick()
    tagButton?.focus()
  }

  function addTag(e: SubmitEvent) {
    e.preventDefault()
    const name = tagDraft.trim()
    if (!name || !count) return
    ontag(name)
  }
</script>

<div class="bulk" role="toolbar" aria-label="Selected skills" tabindex="-1" bind:this={bar} onkeydown={keys} onfocusin={focused}>
  <strong>{count === 1 ? '1 selected' : `${count} selected`}</strong>
  {#if tagging}
    <form onsubmit={addTag}>
      <label class="vh" for="bulk-tag">Tag the selected skills</label>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="bulk-tag" autofocus data-combo-list="bulk-tag-options" bind:value={tagDraft} placeholder="Tag name" autocomplete="off"
        onkeydown={e => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); void stopTagging() } }} />
      <datalist id="bulk-tag-options">{#each tagNames as name (name)}<option value={name}></option>{/each}</datalist>
      <button type="submit" class="btn primary" disabled={!tagDraft.trim() || !count}>Add tag</button>
      <button type="button" class="btn quiet" onclick={stopTagging}>Cancel</button>
    </form>
  {:else}
    {#if tab === 'active'}
      <button type="button" class="btn" disabled={!count} onclick={onarchive}>Archive</button>
    {:else}
      <button type="button" class="btn" disabled={!count} onclick={onrestore}>Restore</button>
    {/if}
    <button type="button" class="btn danger" disabled={!count} onclick={ondelete}>Delete</button>
    <button type="button" class="btn" disabled={!count} bind:this={tagButton} onclick={() => (tagging = true)}>Add tag</button>
    <button type="button" class="btn quiet" onclick={oncancel}>Cancel</button>
  {/if}
</div>

<style>
  .bulk {
    position: fixed; left: 50%; bottom: calc(24px + var(--bottom-chrome, 0px)); transform: translateX(-50%); z-index: 19;
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap; width: max-content; max-width: calc(100vw - 32px);
    padding: 8px 8px 8px 18px; border: 1px solid var(--line-strong); border-radius: 14px; background: var(--card); box-shadow: 0 18px 40px -20px var(--scrim);
  }
  .bulk strong { font-size: 0.9375rem; margin-right: 8px; font-variant-numeric: tabular-nums; }
  .bulk .btn { min-height: 44px; border-radius: 10px; }
  .bulk .danger { color: var(--bad); }
  .bulk form { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .bulk input { min-height: 44px; width: 12rem; }
  @media (max-width: 560px) {
    .bulk { width: calc(100vw - 24px); bottom: calc(12px + var(--bottom-chrome, 0px)); padding: 8px 10px; }
    .bulk .btn { flex: 1; }
    .bulk form, .bulk input { width: 100%; }
  }
</style>
