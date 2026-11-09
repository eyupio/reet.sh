<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { getLibrary, getLibrarySelection, getSyncState, saveLibrarySelection, type LibraryItem, type SyncEnvironment, type SyncState } from './api'
  import { describeError } from './contentReview'
  import { agentOnline } from './machines'
  import { itemMachineState } from './machineItems'
  let { host = '', snapshot, onsaved }: { host?: string; snapshot?: SyncState; onsaved?: () => void } = $props()
  const headingId = $props.id()
  let items = $state<LibraryItem[]>([])
  let target = $state('')
  let selected = $state<string[]>([])
  let version = $state('')
  // What the controller has for this folder, kept so each row can say what is really happening.
  let savedSelection = $state<string[]>([])
  let loadedState = $state<SyncState>()
  let loadedSelection = $state(false)
  let loading = $state(true)
  // True once the library has been read; a failed read keeps its message and offers a retry instead of moving on.
  let libraryReady = $state(false)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let query = $state('')
  let generation = 0
  let disposed = false
  // The newest snapshot wins, so a machine card that refreshes updates these rows too.
  const current = $derived(snapshot ?? loadedState)
  // Folders follow the live snapshot: a machine that has only just connected reports its folders a moment later, and they
  // appear here when they arrive rather than only if they were there when this opened.
  const targets = $derived<SyncEnvironment[]>(current
    ? current.environments.filter(env => ['agents','claude','opencode','cursor'].includes(env.agent)
      && (!host || env.host.toLowerCase()===host.toLowerCase())
      && current.agents?.some(agent=>agent.host.toLowerCase()===env.host.toLowerCase()))
    : [])
  // On a machine's own dialog a single folder needs no choice. Tried once per folder, so a failed read is retried by hand.
  let autoChosen = ''
  $effect(() => {
    const only = targets.length === 1 ? targets[0].id : ''
    if (!host || loading || !libraryReady || target || !only || autoChosen === only) return
    autoChosen = only
    untrack(() => { void choose(only) })
  })
  const destination = $derived(targets.find(option => option.id === target))
  const online = $derived(agentOnline(current?.agents?.find(agent => agent.host.toLowerCase() === (destination?.host ?? host).toLowerCase())))
  const stateOf = (item: LibraryItem) => itemMachineState({ item, selected: savedSelection.includes(item.id), target: current?.targets.find(option => option.target.id === target), jobs: current?.jobs ?? [], online })
  const eligible = (item: LibraryItem) => item.itemType === 'skill' && item.status === 'in_use'
  const visible = $derived(items.filter(item => item.itemType === 'skill' && item.name.toLowerCase().includes(query.trim().toLowerCase())))
  const folderLabel = (destination: SyncEnvironment) => `${host ? '' : `${destination.host} · `}${({ agents: 'Shared skills', claude: 'Claude Code', opencode: 'OpenCode', cursor: 'Cursor' } as Record<string, string>)[destination.agent] ?? destination.agent}${destination.project ? ` · ${destination.project}` : ' · All projects'}`
  async function choose(id: string) {
    const request = ++generation
    target=id; loading=true; loadedSelection=false; error=''; notice=''; selected=[]; savedSelection=[]; version=''
    if (!id) { loading=false; return }
    try {
      const saved = await getLibrarySelection(id)
      if (request !== generation) return
      selected=saved.itemIds; savedSelection=saved.itemIds; version=saved.version; loadedSelection=true
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
      selected=saved.itemIds; savedSelection=saved.itemIds; version=saved.version
      notice='Selection saved. Available updates sync in the background when this machine is connected.'
      onsaved?.()
    } catch(e) { if (!disposed && request === generation) error=describeError(e) }
    finally { busy=false }
  }
  async function load() {
    loading=true; error=''; libraryReady=false
    try {
      const [library,state] = await Promise.all([getLibrary(),snapshot ? Promise.resolve(snapshot) : getSyncState()])
      if(disposed)return
      items=library.items
      loadedState=state
      libraryReady=true
      loading=false
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
      <fieldset disabled={busy}><legend>Selected skills · {selected.length}</legend>{#each visible as item (item.id)}{@const itemState = host && savedSelection.includes(item.id) ? stateOf(item) : undefined}<div class="row"><label class="skill"><input type="checkbox" checked={selected.includes(item.id)} disabled={!eligible(item) && !selected.includes(item.id)} aria-describedby={itemState ? `state-${item.id}` : undefined} onchange={event=>toggle(item.id,event.currentTarget.checked)} /><span>{item.name}{#if item.status!=='in_use'} <small>({item.status.replaceAll('_',' ')})</small>{/if}{#if item.problem || item.contentState!=='complete'}<small> · Waiting for complete content</small>{/if}</span></label>{#if itemState}<p class="state {itemState.tone}" id={`state-${item.id}`} data-item-state={itemState.key}><strong>{itemState.label}</strong>{#if itemState.detail} <span>{itemState.detail}</span>{/if}</p>{/if}</div>{:else}<p>{query.trim() ? 'No skills match your search.' : 'Your library has no skills yet.'}</p>{/each}</fieldset>
      <button class="btn" disabled={busy} aria-busy={busy}>{busy ? 'Saving…' : 'Save selection'}</button>
    </form>
  {:else if loading}<p role="status">Loading selections…</p>{:else if !targets.length && !error}<p>{host ? 'No supported skill folders have been reported for this machine yet. They will appear after it connects and reports them.' : 'Connect a machine to choose its skills.'}</p>{/if}
  {#if notice}<p role="status">{notice}</p>{/if}
</section>
<style>
.selection.embedded{border:0;padding:0;margin:0}.search{margin-top:16px}.search input{min-width:0;width:100%;box-sizing:border-box;padding:10px}.selection :is(button,select,input[type=search]){min-height:44px}
.selection{min-width:0;border:1px solid var(--line);border-radius:12px;padding:20px;margin-bottom:24px}h2{margin:0 0 8px;font-size:1.1rem}p,small{color:var(--muted)}label{display:grid;gap:8px}select{max-width:100%;padding:8px}fieldset{min-width:0;border:0;padding:0;margin:12px 0;max-height:320px;overflow:auto}legend{font-weight:600;padding:0 0 8px}.row{border-bottom:1px solid color-mix(in srgb,var(--line) 60%,transparent)}.row:last-child{border-bottom:0}.skill{display:flex;align-items:center;gap:10px;padding:8px 0}.state{margin:0 0 8px 28px;font-size:.82rem;line-height:1.4;overflow-wrap:anywhere}.state strong{font-weight:600}.state.ok strong{color:var(--good)}.state.work strong{color:var(--info)}.state.wait strong{color:var(--warn)}.state.bad strong{color:var(--bad)}.state.muted strong{color:var(--muted)}.state span{color:var(--muted)}.state strong+span::before{content:' · ';white-space:pre}.skill input{flex:0 0 auto}.skill span{min-width:0;overflow-wrap:anywhere}.selection-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}small{font-size:.8rem}
</style>
