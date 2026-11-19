<script lang="ts">
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

  function addTag(e: SubmitEvent) {
    e.preventDefault()
    const name = tagDraft.trim()
    if (!name || !count) return
    ontag(name)
  }
</script>

<div class="bulk" role="toolbar" aria-label="Selected skills">
  <strong>{count === 1 ? '1 selected' : `${count} selected`}</strong>
  {#if tagging}
    <form onsubmit={addTag}>
      <label class="vh" for="bulk-tag">Tag the selected skills</label>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="bulk-tag" autofocus data-combo-list="bulk-tag-options" bind:value={tagDraft} placeholder="Tag name" autocomplete="off" />
      <datalist id="bulk-tag-options">{#each tagNames as name (name)}<option value={name}></option>{/each}</datalist>
      <button type="submit" class="btn primary" disabled={!tagDraft.trim() || !count}>Add tag</button>
      <button type="button" class="btn quiet" onclick={() => (tagging = false)}>Cancel</button>
    </form>
  {:else}
    {#if tab === 'active'}
      <button type="button" class="btn" disabled={!count} onclick={onarchive}>Archive</button>
    {:else}
      <button type="button" class="btn" disabled={!count} onclick={onrestore}>Restore</button>
    {/if}
    <button type="button" class="btn danger" disabled={!count} onclick={ondelete}>Delete</button>
    <button type="button" class="btn" disabled={!count} onclick={() => (tagging = true)}>Add tag</button>
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
