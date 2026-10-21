<script lang="ts">
  import { onMount } from 'svelte'
  import { listLists, audienceLabel, type SharedList } from './api'
  import PageHeader from './PageHeader.svelte'
  import NewList from './NewList.svelte'
  let { userId, canWrite }: { userId: number; canWrite: boolean } = $props()
  let lists = $state<SharedList[]>([]), scope = $state('mine'), search = $state(''), loading = $state(true), error = $state(''), creating = $state(false), linkEnabled = $state(false)
  let ownerId = $state(0)
  const shown = $derived(lists.filter(l => {
    const match = scope === 'mine' ? l.ownerId === ownerId : scope === 'people' ? l.ownerId !== ownerId && l.role !== 'admin' && ['private', 'link'].includes(l.visibility) : scope === 'team' ? l.visibility === 'team' : l.visibility === 'members'
    return match && `${l.name} ${l.description}`.toLowerCase().includes(search.trim().toLowerCase())
  }))
  async function load() { try { const r = await listLists(); lists = r.lists; ownerId = r.userId ?? userId; linkEnabled = r.linkEnabled } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { loading = false } }
  onMount(() => { void load() })
</script>
<PageHeader eyebrow="Your lists & shared spaces" title="The right skills. The right people." lede="Keep personal lists private, collaborate with selected people, or share with a team. Public discoveries have their own community area." />
<div class="controls"><div role="group" aria-label="List audience filters"><button class="btn" aria-pressed={scope === 'mine'} onclick={() => scope = 'mine'}>My lists</button><button class="btn" aria-pressed={scope === 'people'} onclick={() => scope = 'people'}>Shared with me</button><button class="btn" aria-pressed={scope === 'team'} onclick={() => scope = 'team'}>Teams & organisations</button>{#if lists.some(l => l.visibility === 'members')}<button class="btn" aria-pressed={scope === 'legacy'} onclick={() => scope = 'legacy'}>Legacy audience</button>{/if}<a class="btn" href="/community">Public community ↗</a></div>{#if canWrite}<button class="btn primary" onclick={() => creating = true}>New list</button>{/if}</div>
<label class="search">Search lists <input type="search" bind:value={search} placeholder="Find a list" /></label>
{#if error}<p role="alert">{error}</p>{/if}
{#if loading}<p role="status">Loading lists…</p>{:else}<div class="cards">{#each shown as list (list.id)}<a class="card" href={`/lists/${list.id}`}><span class="badge" class:public={list.visibility === 'public'}>{audienceLabel(list.visibility)}</span><h2>{list.name}</h2><p>{list.description || 'A collection of skills.'}</p><footer><span>{list.skillCount} skills</span><span>{list.ownerId === ownerId ? 'You own this' : list.role === 'editor' ? 'You can edit' : 'You can view'}</span></footer></a>{:else}<div class="empty"><h2>{search ? 'No lists match your search.' : scope === 'people' ? 'Nothing shared with you yet.' : scope === 'team' ? 'Bring a team’s toolkit together.' : 'Start with your own collection.'}</h2><p>{scope === 'team' ? 'Create a team or organisation, add its members, then choose it as a list audience.' : 'Create a personal list, or ask someone to share a list with your account.'}</p>{#if scope === 'team'}<a class="btn" href="/spaces">Manage teams & organisations</a>{/if}</div>{/each}</div>{/if}
{#if creating}<NewList {linkEnabled} oncancel={() => creating = false} oncreated={list => location.assign(`/lists/${list.id}`)} />{/if}
<style>
 .controls { display:flex; flex-wrap:wrap; gap:16px; justify-content:space-between; margin:24px 0 16px; } [role=group] { display:flex; flex-wrap:wrap; gap:6px; } [aria-pressed=true] { background:var(--tint); border-color:var(--accent); } a.btn { text-decoration:none; color:var(--ink); } .search { display:grid; gap:6px; font-size:.8rem; max-width:360px; }
 .cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr)); gap:16px; margin-top:24px; } .card,.empty { color:var(--ink); text-decoration:none; border:1px solid var(--line); border-radius:12px; background:var(--card); padding:22px; min-width:0; } .card:hover { border-color:var(--accent); } .badge { color:var(--muted); font-size:.75rem; } .public { color:var(--accent); } h2 { font:500 1.4rem/1.3 var(--serif); overflow-wrap:anywhere; } p { font-size:.85rem; color:var(--muted); line-height:1.6; overflow-wrap:anywhere; } footer { display:flex; flex-wrap:wrap; gap:10px; justify-content:space-between; font-size:.75rem; color:var(--muted); }
</style>
