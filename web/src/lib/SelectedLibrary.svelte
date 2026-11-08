<script lang="ts">
  import { onMount } from 'svelte'
  import { getLibrary, getLibrarySelection, getSyncState, saveLibrarySelection, type LibraryItem, type LibrarySelection, type SyncEnvironment } from './api'
  import { describeError } from './contentReview'
  let items = $state<LibraryItem[]>([])
  let targets = $state<SyncEnvironment[]>([])
  let target = $state('')
  let selected = $state<string[]>([])
  let version = $state('')
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let generation = 0
  const eligible = (item: LibraryItem) => item.itemType === 'skill' && item.status === 'in_use'
  async function choose(id: string) {
    const request = ++generation
    target=id; loading=true; error=''; notice=''; selected=[]; version=''
    if (!id) { loading=false; return }
    try {
      const saved = await getLibrarySelection(id)
      if (request !== generation) return
      selected=saved.itemIds; version=saved.version
    } catch (e) {
      if (request !== generation) return
      // An unconfigured destination starts with no implicit selection.
      if (!(e instanceof Error && 'status' in e && e.status === 404)) error=describeError(e)
    } finally { if (request === generation) loading=false }
  }
  function toggle(id: string, checked: boolean) { selected=checked ? [...selected.filter(value=>value!==id),id] : selected.filter(value=>value!==id); notice='' }
  async function save() {
    busy=true; error=''; notice=''
    try {
      const saved: LibrarySelection = await saveLibrarySelection({ targetId:target,itemIds:selected,version })
      selected=saved.itemIds; version=saved.version
      notice='Selection saved. This machine will catch up when connected. Unselected items stop receiving updates.'
    } catch(e) { error=describeError(e) }
    finally { busy=false }
  }
  onMount(()=>{ let disposed=false; void Promise.all([getLibrary(),getSyncState()]).then(([library,state])=>{
    if(disposed)return
    items=library.items
    targets=state.environments.filter(env=>['agents','claude','opencode','cursor'].includes(env.agent) && state.agents?.some(agent=>agent.host.toLowerCase()===env.host.toLowerCase()))
    loading=false
  }).catch(e=>{if(!disposed){error=describeError(e);loading=false}});return()=>{disposed=true;generation++} })
</script>
<section aria-labelledby="selected-library-heading" class="selection">
  <h2 id="selected-library-heading">Skills to sync</h2>
  <p>Choose a machine folder and the skills to keep up to date. Only skills marked In use are sent.</p>
  <label>Machine folder <select value={target} disabled={busy} onchange={event=>void choose(event.currentTarget.value)}><option value="">Choose a folder…</option>{#each targets as destination}<option value={destination.id}>{destination.host} · {destination.agent}{destination.project ? ` · ${destination.project}` : ''}</option>{/each}</select></label>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if target && !loading && !error}
    <form onsubmit={event=>{event.preventDefault();void save()}}>
      <div class="selection-actions"><button class="btn quiet" type="button" disabled={busy} onclick={()=>{selected=[...new Set([...selected,...items.filter(eligible).map(item=>item.id)])];notice=''}}>Select all In use</button><button class="btn quiet" type="button" disabled={busy} onclick={()=>{selected=[];notice=''}}>Clear selection</button></div>
      <fieldset disabled={busy}><legend>Selected skills</legend>{#each items.filter(item=>item.itemType==='skill') as item}<label class="skill"><input type="checkbox" checked={selected.includes(item.id)} disabled={!eligible(item) && !selected.includes(item.id)} onchange={event=>toggle(item.id,event.currentTarget.checked)} /><span>{item.name}{#if item.status!=='in_use'} <small>({item.status.replaceAll('_',' ')})</small>{/if}{#if item.problem || item.contentState!=='complete'}<small> · Waiting for complete content</small>{/if}</span></label>{:else}<p>Your library has no skills yet.</p>{/each}</fieldset>
      <button class="btn" disabled={busy} aria-busy={busy}>{busy ? 'Saving…' : 'Save selection'}</button>
    </form>
  {:else if loading}<p role="status">Loading selections…</p>{:else if !targets.length}<p>Connect a machine to choose its skills.</p>{/if}
  {#if notice}<p role="status">{notice}</p>{/if}
</section>
<style>
.selection{border:1px solid var(--line);border-radius:12px;padding:20px;margin-bottom:24px}h2{margin:0 0 8px;font-size:1.1rem}p,small{color:var(--muted)}label{display:grid;gap:8px}select{max-width:100%;padding:8px}fieldset{border:0;padding:0;margin:12px 0;max-height:320px;overflow:auto}legend{font-weight:600;padding:0 0 8px}.skill{display:flex;align-items:center;gap:10px;padding:8px 0}.skill input{flex:0 0 auto}.selection-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}small{font-size:.8rem}
</style>
