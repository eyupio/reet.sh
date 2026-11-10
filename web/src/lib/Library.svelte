<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { getLibrary, getLibraryHistory, getLibrarySelection, getSyncState, resolveLibraryDifference, setLibraryStatus, type LibraryDifference, type LibraryItem, type LibraryReceipt } from './api'
  import { describeError } from './contentReview'
  import { STATUSES, filterLibrary, itemNotes, machinesByItem, receiptLine, statusCounts, statusLabel, type StatusFilter } from './libraryView'

  let { canWrite = true, timeZone = '', onerror }: { canWrite?: boolean; timeZone?: string; onerror?: (e: unknown) => void } = $props()

  let items = $state<LibraryItem[]>([])
  let loading = $state(true)
  let error = $state('')
  let query = $state('')
  let filter = $state<StatusFilter>('all')
  // Machines are read after the items: a failure there is said once and never hides the library.
  let machines = $state(new Map<string, string[]>())
  let machinesKnown = $state(false)
  let machinesError = $state(false)
  let busy = $state('')
  // Machines holding a version of an item that reet has never saved, by item id.
  let differences = $state<Record<string, LibraryDifference[]>>({})
  let rowError = $state<Record<string, string>>({})
  let open = $state<Record<string, { state: 'loading' | 'ready' | 'error'; receipts: LibraryReceipt[]; error: string }>>({})
  let disposed = false

  const counts = $derived(statusCounts(items))
  const shown = $derived(filterLibrary(items, query, filter))
  const supported = ['agents', 'claude', 'opencode', 'cursor']

  const filters = $derived<{ id: StatusFilter; label: string }[]>([{ id: 'all', label: 'All' }, ...STATUSES.map(entry => ({ id: entry.id, label: entry.label }))])

  async function loadMachines() {
    machinesError = false
    try {
      const state = await getSyncState()
      const folders = state.environments.filter(env => supported.includes(env.agent) && state.agents?.some(agent => agent.host.toLowerCase() === env.host.toLowerCase()))
      const saved = await Promise.all(folders.map(async env => {
        try { return { host: env.host, itemIds: (await getLibrarySelection(env.id)).itemIds } }
        catch (e) {
          // A folder with no saved selection yet has nothing selected.
          if (e instanceof Error && 'status' in e && e.status === 404) return { host: env.host, itemIds: [] as string[] }
          throw e
        }
      }))
      if (disposed) return
      machines = machinesByItem(saved)
      machinesKnown = true
    } catch { if (!disposed) machinesError = true }
  }

  async function load() {
    loading = true; error = ''
    try {
      const library = await getLibrary()
      if (disposed) return
      items = library.items
      differences = library.differences ?? {}
      loading = false
      void loadMachines()
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

  // The person decides which version reet holds. Taking a machine's version is saved in the background by that machine,
  // so the row says so and the list is read again a little later; keeping reet's copy is immediate.
  async function resolve(item: LibraryItem, difference: LibraryDifference, action: 'take' | 'keep') {
    if (busy) return
    busy = item.id; rowError = { ...rowError, [item.id]: '' }
    try {
      await resolveLibraryDifference(item.id, difference.environmentId, action)
      if (disposed) return
      const library = await getLibrary()
      if (disposed) return
      items = library.items; differences = library.differences ?? {}
      if (action === 'take') {
        setTimeout(() => { if (!disposed) void refresh() }, 15_000)
      }
    } catch (e) {
      if (!disposed) rowError = { ...rowError, [item.id]: describeError(e) }
    } finally { busy = '' }
  }
  async function refresh() {
    try {
      const library = await getLibrary()
      if (!disposed) { items = library.items; differences = library.differences ?? {} }
    } catch { /* the list on screen stays as it was */ }
  }
  const folderName: Record<string, string> = { agents: 'shared skills', claude: 'Claude Code', opencode: 'OpenCode', cursor: 'Cursor' }

  async function toggleHistory(item: LibraryItem) {
    if (open[item.id]) { const { [item.id]: _closed, ...rest } = open; open = rest; return }
    open = { ...open, [item.id]: { state: 'loading', receipts: [], error: '' } }
    try {
      const receipts = await getLibraryHistory(item.id)
      if (!disposed && open[item.id]) open = { ...open, [item.id]: { state: 'ready', receipts, error: '' } }
    } catch (e) {
      if (!disposed && open[item.id]) open = { ...open, [item.id]: { state: 'error', receipts: [], error: describeError(e) } }
    }
  }

  const machineLine = (item: LibraryItem): string => {
    const hosts = machines.get(item.id) ?? []
    if (!machinesKnown) return ''
    if (!hosts.length) return 'Not selected on any machine'
    const names = hosts.join(', ')
    return item.status === 'in_use' ? `Selected on ${names}` : `Selected on ${names}, not installed while ${statusLabel(item.status).toLowerCase()}`
  }

  onMount(() => { if (canWrite) void load(); else loading = false; return () => { disposed = true } })
</script>

<PageHeader title="Library" lede="Every skill you own, one entry each. Choose what is In use here, and pick the machines that keep it on the Machines page." />

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
    {#if machinesError}<p class="quiet-note" role="status">Machine selections could not be read just now, so which machines keep each item is not shown. <button class="btn quiet" type="button" onclick={() => void loadMachines()}>Try again</button></p>{/if}
        <p class="quiet-note" role="status" aria-live="polite">{shown.length === items.length ? `${items.length} ${items.length === 1 ? 'item' : 'items'}` : `${shown.length} of ${items.length} items`}</p>
    {#if shown.length === 0}
      <p class="quiet-note">No items match. Clear the search or choose All to see everything, including items that are not In use.</p>
    {:else}
      <ul class="items">
        {#each shown as item (item.id)}
          {@const notes = itemNotes(item)}
          {@const line = machineLine(item)}
          <li class="item" data-item={item.name}>
            <div class="main">
              <h2>{item.name}{#if item.itemType !== 'skill'} <small class="type">{item.itemType}</small>{/if}</h2>
              {#if notes.length}<p class="notes">{notes.join(' · ')}</p>{/if}
              {#if item.problem || item.contentState !== 'complete'}<p class="wait">Waiting for complete content. reet saves a copy from the machine that has it when that machine next checks in.</p>{/if}
              {#if line}<p class="machines">{line} · <a href="/machines">Choose machines</a></p>{/if}
              {#each differences[item.id] ?? [] as difference (difference.environmentId)}
                <div class="differs" data-difference={difference.host}>
                  {#if difference.taking}
                    <p role="status">Saving {difference.host}’s version as reet’s copy. This finishes by itself while that machine is online.</p>
                  {:else}
                    <p><strong>{difference.host} has a different version</strong> in its {folderName[difference.agent] ?? difference.agent} folder. reet is keeping its own copy until you choose.</p>
                    {#if canWrite}
                      <div class="choose">
                        <button type="button" class="btn" disabled={busy === item.id} onclick={() => void resolve(item, difference, 'take')}>Use {difference.host}’s version</button>
                        <button type="button" class="btn quiet" disabled={busy === item.id} onclick={() => void resolve(item, difference, 'keep')}>Keep reet’s copy</button>
                      </div>
                    {/if}
                  {/if}
                </div>
              {/each}
              {#if rowError[item.id]}<p class="notice bad" role="alert">{rowError[item.id]}</p>{/if}
            </div>
            <div class="side">
              <label class="status">Status<select aria-label={`Status of ${item.name}`} value={item.status} disabled={busy === item.id} onchange={event => void change(item, event.currentTarget.value, event.currentTarget)}>
                {#each STATUSES as option (option.id)}<option value={option.id}>{option.label}</option>{/each}
                {#if !STATUSES.some(option => option.id === item.status)}<option value={item.status}>{statusLabel(item.status)}</option>{/if}
              </select></label>
              <button type="button" class="btn quiet" aria-expanded={!!open[item.id]} aria-controls={`history-${item.id}`} onclick={() => void toggleHistory(item)}>History</button>
            </div>
            {#if open[item.id]}
              <div class="history" id={`history-${item.id}`} role="region" aria-label={`History of ${item.name}`}>
                {#if open[item.id].state === 'loading'}<p role="status">Loading history…</p>
                {:else if open[item.id].state === 'error'}<p class="notice bad" role="alert">{open[item.id].error}</p>
                {:else if open[item.id].receipts.length === 0}<p>No saves are recorded for this item yet. Edits made on a machine or here appear once they are saved.</p>
                {:else}<ol>{#each open[item.id].receipts as receipt (receipt.eventId)}<li>{receiptLine(receipt, timeZone)}</li>{/each}</ol>{/if}
              </div>
            {/if}
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
  .differs { display: grid; gap: 8px; margin-top: 8px; padding: 10px 12px; border: 1px solid var(--warn); border-radius: 8px; }
  .differs p { margin: 0; }
  .differs .choose { display: flex; flex-wrap: wrap; gap: 8px; }
  .differs .btn { min-height: 44px; }
  .type { font-weight: 400; color: var(--muted); text-transform: capitalize; }
  .side { display: flex; gap: 8px; align-items: end; flex-wrap: wrap; justify-content: flex-end; }
  .status { display: grid; gap: 4px; font-size: .8rem; color: var(--muted); }
  .status select { min-height: 44px; padding: 8px; max-width: 100%; color: var(--ink); }
  .side .btn { min-height: 44px; }
  .history { grid-column: 1 / -1; border-top: 1px solid var(--line); padding-top: 10px; font-size: .9rem; }
  .history p { margin: 0; color: var(--muted); }
  .history ol { margin: 0; padding-left: 20px; display: grid; gap: 4px; }
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
