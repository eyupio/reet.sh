<script lang="ts">
  import { onMount } from 'svelte'
  import { listTags, mergeTags, renameTag, type Tag } from './api'

  let { onclose, onchanged, onerror }: { onclose: () => void; onchanged: () => Promise<void>; onerror?: (e: unknown) => void } = $props()

  let dialog = $state<HTMLDialogElement>()
  let tags = $state<Tag[]>([])
  let error = $state('')
  let busy = $state(false)
  let editID = $state<number | null>(null)
  let editName = $state('')
  let renameInput = $state<HTMLInputElement>()
  let mergeFrom = $state<number | null>(null)
  let mergeInto = $state<number | null>(null)

  async function load() {
    try {
      const r = await listTags()
      tags = r.tags
      error = ''
      if (!tags.some((t) => t.id === mergeFrom)) mergeFrom = tags[0]?.id ?? null
      if (!tags.some((t) => t.id === mergeInto) || mergeInto === mergeFrom) {
        mergeInto = tags.find((t) => t.id !== mergeFrom)?.id ?? null
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    }
  }

  onMount(() => {
    dialog?.showModal()
    void load()
    return () => dialog?.close()
  })

  function close() {
    dialog?.close()
    onclose()
  }

  $effect(() => {
    if (editID !== null) renameInput?.focus()
  })

  function startRename(tag: Tag) {
    editID = tag.id
    editName = tag.name
    error = ''
  }

  async function saveRename() {
    if (editID === null || !editName.trim()) return
    busy = true
    try {
      await renameTag(editID, editName.trim())
      editID = null
      await load()
      await onchanged()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = false
    }
  }

  async function merge() {
    if (mergeFrom === null || mergeInto === null || mergeFrom === mergeInto) return
    busy = true
    try {
      await mergeTags(mergeFrom, mergeInto)
      await load()
      await onchanged()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} aria-labelledby="tags-title" oncancel={(e) => { e.preventDefault(); close() }}>
  <div class="box">
    <header>
      <h2 id="tags-title">Manage tags</h2>
      <button type="button" class="btn quiet close" onclick={close} aria-label="Close">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
          <path d="m7 7 10 10M17 7 7 17"></path>
        </svg>
      </button>
    </header>
    <p class="lede">Rename a tag everywhere, or merge one tag into another.</p>
    {#if error}<p class="error" role="alert">{error}</p>{/if}

    {#if tags.length === 0}
      <p class="empty">No tags yet. Add one from a skill card.</p>
    {:else}
      <ul class="taglist">
        {#each tags as tag}
          <li>
            {#if editID === tag.id}
              <input
                bind:this={renameInput}
                bind:value={editName}
                aria-label="New name for {tag.name}"
                onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void saveRename() } }}
              />
              <button type="button" class="btn primary small" onclick={saveRename} disabled={busy || !editName.trim()}>Save</button>
              <button type="button" class="btn quiet small" onclick={() => (editID = null)}>Cancel</button>
            {:else}
              <span class="name">{tag.name}</span>
              <span class="count">{tag.count ?? 0}</span>
              <button type="button" class="btn quiet small" onclick={() => startRename(tag)}>Rename</button>
            {/if}
          </li>
        {/each}
      </ul>

      {#if tags.length > 1}
        <div class="merge">
          <p>Merge a tag</p>
          <div class="merge-row">
            <label>
              <span class="vh">Tag to merge from</span>
              <select bind:value={mergeFrom} aria-label="Tag to merge from">
                {#each tags as tag}<option value={tag.id}>{tag.name}</option>{/each}
              </select>
            </label>
            <span aria-hidden="true">into</span>
            <label>
              <span class="vh">Tag to merge into</span>
              <select bind:value={mergeInto} aria-label="Tag to merge into">
                {#each tags as tag}<option value={tag.id}>{tag.name}</option>{/each}
              </select>
            </label>
            <button type="button" class="btn" onclick={merge} disabled={busy || mergeFrom === null || mergeInto === null || mergeFrom === mergeInto}>Merge</button>
          </div>
        </div>
      {/if}
    {/if}
  </div>
</dialog>

<style>
  dialog {
    max-width: min(560px, calc(100vw - 32px)); padding: 0; border: 0;
    background: transparent; color: var(--ink);
  }
  dialog::backdrop { background: rgb(20 18 15 / 0.45); backdrop-filter: blur(2px); }
  .box {
    padding: 22px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius);
    box-shadow: 0 28px 80px -24px rgb(0 0 0 / 0.45);
  }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .close { display: grid; place-items: center; width: 40px; height: 40px; padding: 0; flex: none; }
  .close svg { width: 17px; height: 17px; }
  .lede { margin: 6px 0 0; color: var(--muted); font-size: 0.9rem; }
  .error { margin: 10px 0 0; color: var(--bad); font-size: 0.9rem; }
  .empty { color: var(--muted); padding: 20px 0 6px; }
  .taglist { list-style: none; margin: 16px 0 0; padding: 0; max-height: 46vh; overflow-y: auto; }
  .taglist li {
    display: flex; align-items: center; gap: 8px; min-height: 44px;
    border-bottom: 1px solid var(--line);
  }
  .taglist li:last-child { border-bottom: 0; }
  .name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .count { color: var(--muted); font-size: 0.8rem; font-variant-numeric: tabular-nums; }
  .small { min-height: 32px; padding: 4px 10px; font-size: 0.85rem; }
  .taglist input { flex: 1; min-width: 0; padding: 7px 10px; }
  .merge { margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--line); }
  .merge p { margin: 0 0 8px; color: var(--muted); font-size: 0.9rem; }
  .merge-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .merge-row select { max-width: 160px; }
  @media (max-width: 560px) {
    .box { padding: 18px; border-radius: 14px; }
    .merge-row { align-items: stretch; }
    .merge-row label, .merge-row select, .merge-row button { flex: 1 1 100%; max-width: none; }
  }
  @media (pointer: coarse) { .small, .close { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: scale(0.97); }
      to { opacity: 1; transform: scale(1); }
    }
  }
</style>
