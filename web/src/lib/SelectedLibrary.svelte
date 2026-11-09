<script lang="ts">
  import { onMount } from 'svelte'
  import { getLibrary, getLibrarySelection, getSyncState, saveLibrarySelection, type LibraryItem, type SyncEnvironment, type SyncState } from './api'
  import { describeError } from './contentReview'
  let { host = '', snapshot, onsaved }: { host?: string; snapshot?: SyncState; onsaved?: () => void } = $props()
  const headingId = $props.id()
  let items = $state<LibraryItem[]>([])
  let targets = $state<SyncEnvironment[]>([])
  let target = $state('')
  let selected = $state<string[]>([])
  let version = $state('')
  let loadedSelection = $state(false)
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let query = $state('')
  let generation = 0
  let disposed = false
  const eligible = (item: LibraryItem) => item.itemType === 'skill' && item.status === 'in_use'
  const visible = $derived(items.filter(item => item.itemType === 'skill' && item.name.toLowerCase().includes(query.trim().toLowerCase())))
  const folderLabel = (destination: SyncEnvironment) => `${host ? '' : `${destination.host} · `}${({ agents: 'Shared skills', claude: 'Claude Code', opencode: 'OpenCode', cursor: 'Cursor' } as Record<string, string>)[destination.agent] ?? destination.agent}${destination.project ? ` · ${destination.project}` : ' · All projects'}`
  async function choose(id: string) {
    const request = ++generation
    target=id; loading=true; loadedSelection=false; error=''; notice=''; selected=[]; version=''
    if (!id) { loading=false; return }
    try {
      const saved = await getLibrarySelection(id)
      if (request !== generation) return
      selected=saved.itemIds; version=saved.version; loadedSelection=true
    } catch (e) {
      if (request !== generation) return
      // An unconfigured destination starts with no implicit selection.
      if (e instanceof Error && 'status' in e && e.status === 404) loadedSelection=true
      else error=describeError(e)
    } finally { if (request === generation) loading=false }
  }
  function toggle(id: string, checked: boolean) { selected=checked ? [...selected.filter(value=>value!==id),id] : selected.filter(value=>value!==id); notice='' }
  async function save() {
    if (busy || loading || !loadedSelection || !target) return
    const request = generation
    busy=true; error=''; notice=''
    try {
      const saved = await saveLibrarySelection({ targetId:target,itemIds:selected,version })
      if (disposed || request !== generation) return
      selected=saved.itemIds; version=saved.version
      notice='Selection saved. Available updates sync in the background when this machine is connected.'
      onsaved?.()
    } catch(e) { if (!disposed && request === generation) error=describeError(e) }
    finally { busy=false }
  }
  async function load() {
    loading=true; error=''
    try {
      const [library,state] = await Promise.all([getLibrary(),snapshot ? Promise.resolve(snapshot) : getSyncState()])
      if(disposed)return
      items=library.items
      targets=state.environments.filter(env=>['agents','claude','opencode','cursor'].includes(env.agent)
        && (!host || env.host.toLowerCase()===host.toLowerCase())
        && state.agents?.some(agent=>agent.host.toLowerCase()===env.host.toLowerCase()))
      loading=false
      if (host && targets.length === 1) await choose(targets[0].id)
    } catch(e) { if(!disposed){error=describeError(e);loading=false} }
  }
  onMount(()=>{void load();return()=>{disposed=true;generation++}})
</script>
<section aria-labelledby={headingId} class="selection" class:embedded={!!host}>
  <h2 id={headingId} class:vh={!!host}>Skills to sync</h2>
  <p>{host ? 'Choose which library skills belong on this machine. You can save changes while it is offline.' : 'Choose a machine folder and the skills to keep up to date.'} Only skills marked In use are sent.</p>
  {#if targets.length > 0}
  <label>Machine folder <select value={target} disabled={busy || loading} onchange={event=>void choose(event.currentTarget.value)}><option value="">Choose a folder…</option>{#each targets as destination}<option value={destination.id}>{folderLabel(destination)}</option>{/each}</select></label>
  {/if}
  {#if error}<p class="notice bad" role="alert">{error}</p><button class="btn quiet" type="button" disabled={busy || loading} onclick={()=>void (target ? choose(target) : load())}>{target ? 'Reload selection' : 'Try again'}</button>{/if}
  {#if target && !loading && loadedSelection}
    <form onsubmit={event=>{event.preventDefault();void save()}}>
      <label class="search">Find a skill <input type="search" bind:value={query} placeholder="Search your library" /></label>
      <div class="selection-actions"><button class="btn quiet" type="button" disabled={busy} onclick={()=>{selected=[...new Set([...selected,...items.filter(eligible).map(item=>item.id)])];notice=''}}>Select all In use</button><button class="btn quiet" type="button" disabled={busy} onclick={()=>{selected=[];notice=''}}>Clear selection</button></div>
      <fieldset disabled={busy}><legend>Selected skills · {selected.length}</legend>{#each visible as item}<label class="skill"><input type="checkbox" checked={selected.includes(item.id)} disabled={!eligible(item) && !selected.includes(item.id)} onchange={event=>toggle(item.id,event.currentTarget.checked)} /><span>{item.name}{#if item.status!=='in_use'} <small>({item.status.replaceAll('_',' ')})</small>{/if}{#if item.problem || item.contentState!=='complete'}<small> · Waiting for complete content</small>{/if}</span></label>{:else}<p>{query.trim() ? 'No skills match your search.' : 'Your library has no skills yet.'}</p>{/each}</fieldset>
      <button class="btn" disabled={busy} aria-busy={busy}>{busy ? 'Saving…' : 'Save selection'}</button>
    </form>
  {:else if loading}<p role="status">Loading selections…</p>{:else if !targets.length && !error}<p>{host ? 'No supported skill folders have been reported for this machine yet. They will appear after it connects and reports them.' : 'Connect a machine to choose its skills.'}</p>{/if}
  {#if notice}<p role="status">{notice}</p>{/if}
</section>
<style>
.selection.embedded{border:0;padding:0;margin:0}.search{margin-top:16px}.search input{min-width:0;width:100%;box-sizing:border-box;padding:10px}.selection :is(button,select,input[type=search]){min-height:44px}
.selection{min-width:0;border:1px solid var(--line);border-radius:12px;padding:20px;margin-bottom:24px}h2{margin:0 0 8px;font-size:1.1rem}p,small{color:var(--muted)}label{display:grid;gap:8px}select{max-width:100%;padding:8px}fieldset{min-width:0;border:0;padding:0;margin:12px 0;max-height:320px;overflow:auto}legend{font-weight:600;padding:0 0 8px}.skill{display:flex;align-items:center;gap:10px;padding:8px 0}.skill input{flex:0 0 auto}.skill span{min-width:0;overflow-wrap:anywhere}.selection-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}small{font-size:.8rem}
</style>
