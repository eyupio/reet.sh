<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import { listLists, audienceLabel, type SharedList } from './api'
  import {
    LIST_SCOPES, listAccessLabel, listCountLabel, listScopeCounts, matchesListScope, matchesListSearch,
    type ListScope,
  } from './lists'
  import { elapsed } from './time'
  import CollectionPanel from './CollectionPanel.svelte'
  import PageHeader from './PageHeader.svelte'
  import NewList from './NewList.svelte'
  import { t } from './copy'

  let { userId, canWrite }: { userId: number; canWrite: boolean } = $props()

  let lists = $state<SharedList[]>([])
  let scope = $state<ListScope>('all')
  let search = $state('')
  let loading = $state(true)
  let error = $state('')
  let creating = $state(false)
  let linkEnabled = $state(false)
  let ownerId = $state(0)
  let layout = $state<'rows' | 'cards'>('rows')

  // The search narrows what the chips count, so a chip never promises a record that pressing it hides.
  const searched = $derived(lists.filter((list) => matchesListSearch(list, search)))
  const counts = $derived(listScopeCounts(searched, ownerId))
  const shown = $derived(searched.filter((list) => matchesListScope(list, scope, ownerId)))
  const scopes = $derived(LIST_SCOPES.filter((item) => item.id !== 'legacy' || counts.legacy > 0 || scope === 'legacy'))
  const countLabel = $derived(listCountLabel(shown.length, lists.length))
  const countText = $derived(loading ? 'Reading your lists…' : error ? 'Lists unavailable' : countLabel)

  async function load() {
    loading = true
    error = ''
    try {
      const r = await listLists()
      lists = r.lists
      ownerId = r.userId ?? userId
      linkEnabled = r.linkEnabled
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  function chooseLayout(value: 'rows' | 'cards') {
    layout = value
    try { localStorage.setItem('reet-lists-layout', value) } catch { /* choice lasts for this visit */ }
  }

  function clearFilters() {
    search = ''
    scope = 'all'
  }

  function updated(ts: number): string {
    const age = elapsed(ts)
    return age === 'just now' ? 'Updated just now' : `Updated ${age} ago`
  }

  function emptyTitle(): string {
    if (search) return t('empty.search.title')
    if (scope === 'people') return 'No lists shared with you yet.'
    if (scope === 'team') return 'No team lists yet.'
    if (scope === 'mine') return 'No lists of your own yet.'
    if (scope === 'legacy') return 'No lists with the legacy audience.'
    return 'Make room for a good toolkit.'
  }

  function emptyBody(): string {
    if (search) return t('empty.search.body')
    if (scope === 'people') return 'Ask someone to share a collection with you, or create your own.'
    if (scope === 'team') return 'Share a list with a team or organisation to see it here.'
    if (scope === 'mine') return 'Create a list to collect the skills you want to keep together.'
    if (scope === 'legacy') return 'Lists created before audiences changed keep their earlier access.'
    return 'Create a list, explore community, or ask someone to share a collection with you.'
  }

  onMount(() => {
    try { layout = localStorage.getItem('reet-lists-layout') === 'cards' ? 'cards' : 'rows' } catch { /* compact default */ }
    void load()
  })
</script>

<PageHeader title="Lists" lede="Curate a toolkit, share it with the right people, and keep personal collections private.">
  {#snippet actions()}
    {#if canWrite}<button class="btn primary" type="button" onclick={() => (creating = true)}>Create a list</button>{/if}
    <a class="btn" href="/community">Explore community</a>
  {/snippet}
</PageHeader>

<CollectionPanel label="Your lists" count={countText}>
  {#snippet actions()}
    <div class="layout" role="group" aria-label="List layout">
      <button type="button" aria-pressed={layout === 'rows'} onclick={() => chooseLayout('rows')}>Rows</button>
      <button type="button" aria-pressed={layout === 'cards'} onclick={() => chooseLayout('cards')}>Cards</button>
    </div>
  {/snippet}

  {#snippet toolbar()}
    <div class="list-toolbar">
      <label class="search">Search lists
        <input type="search" bind:value={search} placeholder="Name, description or owner" />
      </label>
      <div class="scopes" role="group" aria-label="Show lists">
        {#each scopes as item (item.id)}
          <button type="button" class="scope" aria-pressed={scope === item.id} onclick={() => (scope = item.id)}>
            {item.label}<small>{counts[item.id]}</small>
          </button>
        {/each}
      </div>
    </div>
  {/snippet}

  {#if loading}
    <Skeleton label="Reading your lists…" shape="rows" count={5} />
  {:else if error}
    <div class="state error" role="alert">
      <p>{error}</p>
      <button type="button" class="btn" onclick={load}>Try again</button>
    </div>
  {:else if shown.length === 0}
    <div class="state">
      <h2>{emptyTitle()}</h2>
      <p>{emptyBody()}</p>
      <div class="state-actions">
        {#if search || scope !== 'all'}<button type="button" class="btn" onclick={clearFilters}>Clear filters</button>{/if}
        {#if !lists.length && canWrite}<button type="button" class="btn primary" onclick={() => (creating = true)}>Create a list</button>{/if}
        {#if scope === 'team'}<a class="btn quiet" href="/spaces">Manage teams</a>{/if}
        {#if !lists.length && !canWrite}<a class="btn" href="/community">Explore community</a>{/if}
      </div>
    </div>
  {:else}
    <div class="lists" class:cards={layout === 'cards'} class:rows={layout === 'rows'}>
      {#each shown as list (list.id)}
        <a class="list" href={`/lists/${list.id}`}>
          <div class="description">
            <h2>{list.name}</h2>
            <p>{list.description || 'A collection of skills.'}</p>
          </div>
          <div class="metadata">
            <div class="meta-row">
              <span class="badge">{audienceLabel(list.visibility)}</span>
              <span>{list.skillCount} {list.skillCount === 1 ? 'skill' : 'skills'}</span>
            </div>
            <div class="meta-row">
              <span>{listAccessLabel(list, ownerId)}</span>
              <time datetime={new Date(list.updatedAt).toISOString()} title={new Date(list.updatedAt).toLocaleString()}>{updated(list.updatedAt)}</time>
            </div>
          </div>
          <span class="go" aria-hidden="true"></span>
        </a>
      {/each}
    </div>
  {/if}
</CollectionPanel>

{#if creating}<NewList {linkEnabled} oncancel={() => (creating = false)} oncreated={list => location.assign(`/lists/${list.id}`)} />{/if}

<style>
  .layout { display: inline-flex; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .layout button { min-height: 34px; padding: 4px 10px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .layout button:hover { color: var(--ink); }
  .layout button[aria-pressed='true'] { background: var(--tint); color: var(--ink); font-weight: 600; }

  .list-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 260px; display: grid; gap: 6px; color: var(--muted); font-size: 0.875rem; }
  .search input { width: 100%; }
  .scopes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
  .scope { min-height: 36px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .scope:hover { border-color: var(--muted); color: var(--ink); }
  .scope[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 600; }
  .scope small { margin-left: 5px; font-size: 0.875rem; font-variant-numeric: tabular-nums; }

  .lists { display: grid; }
  .list { color: var(--ink); text-decoration: none; min-width: 0; }
  .rows .list { display: grid; grid-template-columns: minmax(0, 1fr) clamp(220px, 28vw, 320px) 8px; align-items: center; gap: 16px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .rows .list:first-child { border-top: 0; }
  .rows .list:hover h2 { text-decoration: underline; text-underline-offset: 2px; }
  .description { min-width: 0; }
  h2 { margin: 0 0 4px; font: 500 1.2rem/1.3 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .description p { margin: 0; color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .metadata { display: grid; width: 100%; justify-items: end; gap: 5px; }
  .meta-row { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 5px 14px; color: var(--muted); font-size: 0.875rem; text-align: right; font-variant-numeric: tabular-nums; }
  .meta-row > * { flex: none; }
  .badge { border: 1px solid var(--line); border-radius: 6px; padding: 3px 8px; color: var(--ink); font-size: 0.875rem; white-space: nowrap; }
  .go { width: 7px; height: 7px; border-top: 2px solid var(--muted); border-right: 2px solid var(--muted); transform: rotate(45deg); }

  .cards { grid-template-columns: repeat(auto-fit, minmax(min(100%, 280px), 1fr)); gap: 14px; padding-top: 14px; }
  .cards .list { display: flex; flex-direction: column; align-items: flex-start; gap: 12px; padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  .cards .list:hover { border-color: var(--muted); }
  .cards .metadata { justify-items: start; max-width: none; }
  .cards .meta-row { justify-content: flex-start; text-align: left; }
  .cards .go { display: none; }

  .state { padding: 26px 0 18px; max-width: 60ch; }
  .state h2 { font-size: 1.3rem; }
  .state p { margin: 6px 0 0; color: var(--muted); }
  .state-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
  .error p { margin: 0 0 12px; color: var(--ink); }

  @media (prefers-reduced-motion: no-preference) {
    .layout button, .scope { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
  }
  @media (max-width: 650px) {
    .list-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .scopes { width: 100%; }
    .rows .list { grid-template-columns: minmax(0, 1fr); gap: 10px; padding: 14px 0; }
    .metadata { justify-items: start; max-width: none; }
    .meta-row { justify-content: flex-start; text-align: left; }
    .go { display: none; }
    .layout { margin-left: auto; }
  }
  @media (pointer: coarse) { .layout button, .scope { min-height: 44px; } }
</style>
