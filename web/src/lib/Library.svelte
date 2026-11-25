<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { getLibrary, setLibraryStatus, type LibraryItem } from './api'
  import { describeError } from './contentReview'
  import { STATUSES, filterLibrary, itemNotes, statusCounts, statusLabel, type StatusFilter } from './libraryView'

  let { canWrite = true, onerror }: { canWrite?: boolean; onerror?: (e: unknown) => void } = $props()

  let items = $state<LibraryItem[]>([])
  let loading = $state(true)
  let error = $state('')
  let query = $state('')
  let filter = $state<StatusFilter>('all')
  let busy = $state('')
  let rowError = $state<Record<string, string>>({})
  let disposed = false

  const counts = $derived(statusCounts(items))
  const shown = $derived(filterLibrary(items, query, filter))

  const filters = $derived<{ id: StatusFilter; label: string }[]>([{ id: 'all', label: 'All' }, ...STATUSES.map(entry => ({ id: entry.id, label: entry.label }))])

  async function load() {
    loading = true; error = ''
    try {
      const library = await getLibrary()
      if (disposed) return
      items = library.items
      loading = false
    } catch (e) {
      if (disposed) return
      error = describeError(e); loading = false
      onerror?.(e)
    }
  }

  async function change(item: LibraryItem, status: string, select: HTMLSelectElement) {
    if (busy || status === item.status) return
    busy = item.id; rowError = { ...rowError, [item.id]: '' }
    try {
      const saved = await setLibraryStatus(item.id, status, item.version ?? 0)
      if (disposed) return
      items = items.map(entry => (entry.id === item.id ? { ...entry, ...saved } : entry))
    } catch (e) {
      if (disposed) return
      select.value = item.status
      const conflict = e instanceof Error && 'status' in e && e.status === 409
      rowError = { ...rowError, [item.id]: conflict ? 'This item changed somewhere else, so its status was not changed. The list has been refreshed.' : describeError(e) }
      if (conflict) { try { items = (await getLibrary()).items } catch { /* the row message already says what happened */ } }
    } finally { busy = '' }
  }

  onMount(() => { if (canWrite) void load(); else loading = false; return () => { disposed = true } })
</script>

<PageHeader title="Library" lede="Every skill you own, one entry each. Choose what is Active here: every machine keeps your Active skills." />

<section class="library" aria-label="Your library">
  {#if !canWrite}
    <!-- The library is private skill content, which the server only gives to an account that can write; asking would only
         produce a refusal. Say so plainly instead. -->
    <div class="empty">
      <h2>The library is for accounts that can write</h2>
      <p>This account is read-only, so it has no library of its own to show. Ask the owner of this instance for write access if you need one.</p>
    </div>
  {:else if loading}
    <p role="status" class="quiet-note">Loading your library…</p>
  {:else if error}
    <p class="notice bad" role="alert">{error}</p>
    <button class="btn" type="button" onclick={() => void load()}>Try again</button>
  {:else if items.length === 0}
    <div class="empty">
      <h2>Your library is empty</h2>
      <p>Connect a machine to bring its skills in, or write one here. Everything you add stays in your library, whichever machines keep it.</p>
      <p class="actions"><a class="btn primary" href="/machines">Connect a machine</a><a class="btn" href="/hosting">Write a skill</a></p>
    </div>
  {:else}
    <div class="controls">
      <label class="search">Find an item <input type="search" bind:value={query} placeholder="Search your library" /></label>
      <div class="filters" role="group" aria-label="Filter by status">
        {#each filters as option (option.id)}
          <button type="button" class="chip" aria-pressed={filter === option.id} onclick={() => (filter = option.id)}>{option.label} <span class="count">{counts[option.id]}</span></button>
        {/each}
      </div>
    </div>
    <p class="quiet-note" role="status" aria-live="polite">{shown.length === items.length ? `${items.length} ${items.length === 1 ? 'item' : 'items'}` : `${shown.length} of ${items.length} items`}</p>
    {#if shown.length === 0}
      <p class="quiet-note">No items match. Clear the search or choose All to see everything, including Archived items.</p>
    {:else}
      <ul class="items">
        {#each shown as item (item.id)}
          {@const notes = itemNotes(item)}
          <li class="item" data-item={item.name}>
            <div class="main">
              <h2>{item.name}{#if item.itemType !== 'skill'} <small class="type">{item.itemType}</small>{/if}</h2>
              {#if notes.length}<p class="notes">{notes.join(' · ')}</p>{/if}
              {#if item.problem || item.contentState !== 'complete'}<p class="wait">Waiting for complete content. reet saves a copy from the machine that has it when that machine next checks in.</p>{/if}
              {#if rowError[item.id]}<p class="notice bad" role="alert">{rowError[item.id]}</p>{/if}
            </div>
            <div class="side">
              <label class="status">Status<select aria-label={`Status of ${item.name}`} value={item.status} disabled={!!busy} onchange={event => void change(item, event.currentTarget.value, event.currentTarget)}>
                {#each STATUSES as option (option.id)}<option value={option.id}>{option.label}</option>{/each}
                {#if !STATUSES.some(option => option.id === item.status)}<option value={item.status}>{statusLabel(item.status)}</option>{/if}
              </select></label>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</section>

<style>
  .library { display: grid; gap: 12px; min-width: 0; }
  .controls { display: grid; gap: 12px; }
  .search { display: grid; gap: 6px; font-weight: 600; }
  .search input { min-width: 0; width: 100%; box-sizing: border-box; padding: 10px; min-height: 44px; font-weight: 400; }
  .filters { display: flex; flex-wrap: wrap; gap: 8px; }
  .chip { min-height: 44px; padding: 0 14px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--ink); cursor: pointer; font: inherit; }
  .chip[aria-pressed='true'] { background: var(--ink); color: var(--card); border-color: var(--ink); }
  .count { color: var(--muted); margin-left: 4px; font-variant-numeric: tabular-nums; }
  .chip[aria-pressed='true'] .count { color: inherit; opacity: .8; }
  .quiet-note { margin: 0; color: var(--muted); font-size: .9rem; }
  .items { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
  .item { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 12px; padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); min-width: 0; }
  .main { min-width: 0; display: grid; gap: 4px; }
  .main h2 { margin: 0; font-size: 1.05rem; overflow-wrap: anywhere; }
  .main p { margin: 0; font-size: .88rem; color: var(--muted); overflow-wrap: anywhere; }
  .main .wait { color: var(--warn); }
  .type { font-weight: 400; color: var(--muted); text-transform: capitalize; }
  .side { display: flex; gap: 8px; align-items: end; flex-wrap: wrap; justify-content: flex-end; }
  .status { display: grid; gap: 4px; font-size: .875rem; color: var(--muted); }
  .status select { min-height: 44px; padding: 8px; max-width: 100%; color: var(--ink); }
  .empty { border: 1px dashed var(--line); border-radius: 12px; padding: 24px; }
  .empty h2 { margin: 0 0 8px; font-size: 1.1rem; }
  .empty p { color: var(--muted); }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  @media (max-width: 560px) {
    .item { grid-template-columns: minmax(0, 1fr); }
    .side { justify-content: stretch; }
    .status { flex: 1 1 100%; }
    .status select { width: 100%; }
  }
</style>
