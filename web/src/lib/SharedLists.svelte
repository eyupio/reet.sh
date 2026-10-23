<script lang="ts">
  import { onMount } from 'svelte'
  import { listLists, audienceLabel, type SharedList } from './api'
  import { matchesListScope } from './lists'
  import PageHeader from './PageHeader.svelte'
  import NewList from './NewList.svelte'
  let { userId, canWrite }: { userId: number; canWrite: boolean } = $props()
  let lists = $state<SharedList[]>([]), scope = $state('all'), search = $state(''), loading = $state(true), error = $state(''), creating = $state(false), linkEnabled = $state(false)
  let ownerId = $state(0), layout = $state('rows')
  const shown = $derived(lists.filter(list => matchesListScope(list, scope, ownerId) && `${list.name} ${list.description} ${list.ownerName ?? ''}`.toLowerCase().includes(search.trim().toLowerCase())))
  async function load() {
    loading = true; error = ''
    try { const r = await listLists(); lists = r.lists; ownerId = r.userId ?? userId; linkEnabled = r.linkEnabled }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { loading = false }
  }
  function chooseLayout(value: string) { layout = value; try { localStorage.setItem('reet-lists-layout', value) } catch { /* choice lasts for this visit */ } }
  onMount(() => { try { layout = localStorage.getItem('reet-lists-layout') === 'cards' ? 'cards' : 'rows' } catch { /* compact default */ }; void load() })
</script>
<PageHeader eyebrow="Good finds, kept together" title="Lists." lede="Curate a toolkit, share it with the right people, and keep personal collections private.">
  {#snippet actions()}{#if canWrite}<button class="btn primary" onclick={() => creating = true}>Create a list</button>{/if}{/snippet}
</PageHeader>
<div class="controls">
  <label>Search lists<input type="search" bind:value={search} placeholder="Name, description or owner" /></label>
  <label>Show<select bind:value={scope}><option value="all">All accessible lists</option><option value="mine">My lists</option><option value="people">Shared with me</option><option value="team">Teams & organisations</option>{#if lists.some(list => list.visibility === 'members')}<option value="legacy">Legacy audience</option>{/if}</select></label>
  <div role="group" aria-label="List layout"><button class="btn quiet" aria-pressed={layout === 'rows'} onclick={() => chooseLayout('rows')}>Rows</button><button class="btn quiet" aria-pressed={layout === 'cards'} onclick={() => chooseLayout('cards')}>Cards</button></div>
</div>
{#if error}<p class="notice bad" role="alert">{error} <button class="btn quiet" onclick={load}>Try again</button></p>{/if}
{#if loading}<p role="status">Loading lists…</p>
{:else if !error}
  <p class="count" role="status">{shown.length} of {lists.length} lists · <a href="/community">Explore community</a></p>
  <div class:cards={layout === 'cards'} class:rows={layout === 'rows'}>
    {#each shown as list (list.id)}
      <a class="list" href={`/lists/${list.id}`}>
        <div class="description"><h2>{list.name}</h2><p>{list.description || 'A collection of skills.'}</p></div>
        <div class="metadata"><span class="badge">{audienceLabel(list.visibility)}</span><span>{list.skillCount} skills</span><span>{list.ownerId === ownerId ? 'Owned by you' : `By ${list.ownerName || 'another member'}`}</span><span>{list.role === 'admin' ? 'Admin access' : list.role === 'editor' ? 'Can edit' : list.ownerId === ownerId ? 'Owner' : 'Can view'}</span><time datetime={new Date(list.updatedAt).toISOString()}>Updated {new Date(list.updatedAt).toLocaleDateString()}</time></div>
      </a>
    {:else}
      <section class="empty"><h2>{search ? 'Nowt matches just yet.' : scope === 'people' ? 'No lists shared with you yet.' : 'Make room for a good toolkit.'}</h2><p>{search ? 'Try another search or show all accessible lists.' : 'Create a list, explore community, or ask someone to share a collection with you.'}</p>{#if search || scope !== 'all'}<button class="btn quiet" onclick={() => { search = ''; scope = 'all' }}>Clear filters</button>{/if}{#if scope === 'team'}<a class="btn quiet" href="/spaces">Manage teams</a>{/if}</section>
    {/each}
  </div>
{/if}
{#if creating}<NewList {linkEnabled} oncancel={() => creating = false} oncreated={list => location.assign(`/lists/${list.id}`)} />{/if}
<style>
 .controls {display:flex;gap:12px;flex-wrap:wrap;align-items:end} label {display:grid;gap:6px;font-size:.8rem} label:first-child {flex:1;min-width:min(240px,100%)} [role=group] {display:flex;gap:4px} [aria-pressed=true] {background:var(--tint);border-color:var(--accent)} .count {font-size:.8rem;color:var(--muted);margin:18px 0 10px}
 .rows {display:grid;border-top:1px solid var(--line)} .list {color:var(--ink);text-decoration:none;min-width:0} .rows .list {display:flex;justify-content:space-between;gap:20px;align-items:center;padding:18px 8px;border-bottom:1px solid var(--line)} .list:hover {background:var(--tint)} .description {min-width:0} h2 {font:500 1.2rem/1.3 var(--serif);overflow-wrap:anywhere;margin:0 0 5px} .description p {margin:0;font-size:.85rem;color:var(--muted);overflow-wrap:anywhere} .metadata {display:flex;gap:8px 16px;flex-wrap:wrap;align-items:center;font-size:.75rem;color:var(--muted)} .rows .metadata {max-width:48%;justify-content:flex-end} .badge {border:1px solid var(--line);border-radius:99px;padding:3px 8px;color:var(--ink)} .cards {display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr));gap:16px} .cards .list,.empty {border:1px solid var(--line);border-radius:12px;padding:20px;background:var(--card)} .cards .metadata {margin-top:20px} .empty p {color:var(--muted)}
 @media(max-width:650px) {.rows .list {align-items:start;flex-direction:column;gap:12px;padding:16px 0}.rows .metadata {max-width:none;justify-content:start}.controls label {flex:1;min-width:150px}.controls [role=group] {margin-left:auto}}
</style>
