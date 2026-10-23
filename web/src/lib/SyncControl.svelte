<script lang="ts">
  import { onMount } from 'svelte'
  import { syncCandidates, listLists, getList, saveDesired, reconcileDesired, adoptSyncSkills, queueSyncJob, controlSyncJob, deleteSyncTarget, type SyncState, type SyncSkill, type SharedList, type SyncTarget, type SyncJob } from './api'
  import { agentGroups, groupLabel, inventory, inventoryMachines } from './machines'
  import { elapsed } from './time'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  let { snapshot, canWrite, onupdate, view = 'compare' }: { snapshot: SyncState; canWrite: boolean; onupdate: (s: SyncState) => void; view?: 'machines' | 'compare' } = $props()
  let source = $state('shelf'), selectedMachine = $state(''), selectedAgents = $state<string[]>([]), listId = $state(0), mode = $state('additive')
  let candidates = $state<SyncSkill[]>([]), selected = $state<string[]>([]), adopt = $state<string[]>([]), lists = $state<SharedList[]>([])
  let revision = $state(''), busy = $state(false), loading = $state(false), error = $state(''), notice = $state(''), query = $state('')
  let listEditable = $state(false)
  let conflicts = $state<string[]>([])
  let confirm = $state<{ title: string; body: string; action: () => Promise<void> } | null>(null)
  const groups = $derived(snapshot.environments.filter(e => !e.agent.startsWith('reet')))
  const fleet = $derived(inventoryMachines(snapshot.machines ?? []))
  const machine = $derived(fleet.find(m => m.host === selectedMachine) ?? null)
  const machineGroups = $derived(machine ? agentGroups(machine) : [])
  const selectableGroups = $derived(machineGroups.filter(g => ['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)))
  const scanOnlyGroups = $derived(machineGroups.filter(g => !['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)))
  const targetEnvs = $derived(selectableGroups.filter(g => selectedAgents.includes(g.id)))
  const actual = $derived(inventory(targetEnvs))
  const desired = $derived(candidates.filter(s => selected.includes(s.id)))
  const onlySource = $derived(desired.filter(s => !actual.some(a => a.id === s.id)))
  const onlyMachine = $derived(actual.filter(s => !desired.some(d => d.id === s.id)))
  const matching = $derived(desired.filter(s => actual.some(a => a.id === s.id)))
  const sourceConflicts = $derived(desired.filter(s => actual.some(a => a.slug === s.slug && a.id !== s.id)))
  const unknown = $derived(desired.filter(s => !s.source || s.sourceType === 'local'))
  const ready = $derived(targetEnvs.length > 0 && targetEnvs.every(e => e.complete && e.scannedAt > Date.now() - 14 * 86400000))
  const oldestScan = $derived(targetEnvs.length ? Math.min(...targetEnvs.map(e => e.scannedAt)) : 0)
  const targetGroups = $derived.by(() => {
    const byHost = new Map<string, { host: string; items: SyncTarget[] }>()
    for (const item of snapshot.targets) {
      const entry = byHost.get(item.target.host) ?? { host: item.target.host, items: [] }
      entry.items.push(item)
      byHost.set(item.target.host, entry)
    }
    return [...byHost.values()]
  })
  const filtered = $derived(candidates.filter(s => `${s.name} ${s.source}`.toLowerCase().includes(query.toLowerCase())))
  function ago(at: number) { const value = elapsed(at); return value === 'just now' ? value : `${value} ago` }
  function quote(v: string) { return "'" + v.replaceAll("'", "'\\''") + "'" }
  function cli(item: SyncTarget) { return `reet sync --server ${quote(location.origin)} --env ${quote(item.target.host)} --target ${quote(item.target.id)}` }
  function toggle(id: string) { selected = selected.includes(id) ? selected.filter(v => v !== id) : [...selected, id]; conflicts = conflicts.filter(name => candidates.find(s => s.id === id)?.slug !== name) }
  async function run(fn: () => Promise<void>) { if (busy) return; busy = true; error = ''; notice = ''; try { await fn() } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false } }
  let request = 0
  async function loadSource() {
    const generation = ++request; loading = true; error = ''; selected = []; adopt = []; conflicts = []; query = ''
    try {
      let next: SyncSkill[]
      if (source === 'shelf' || source === 'hosted') {
        next = (await syncCandidates(source === 'hosted' ? 0 : listId, source)).skills
        const detail = source === 'shelf' && listId ? await getList(listId) : null
        if (generation !== request) return
        listEditable = !detail || detail.canEdit
      } else { next = groups.find(e => e.id === source)?.skills ?? []; listEditable = false }
      if (generation === request) candidates = next
    } catch (e) { if (generation === request) error = String(e) } finally { if (generation === request) loading = false }
  }
  function selectMachine(host: string) {
    selectedMachine = host
    const selected = fleet.find(m => m.host === host)
    selectedAgents = selected ? agentGroups(selected).filter(g => ['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)).map(g => g.id) : []
    revision = ''
    adopt = []
    conflicts = []
  }
  function toggleAgent(id: string) {
    selectedAgents = selectedAgents.includes(id) ? selectedAgents.filter(value => value !== id) : [...selectedAgents, id]
    adopt = []
    conflicts = []
  }
  async function save() {
    await run(async () => {
      if (!targetEnvs.length) throw new Error('Choose at least one agent scope on this machine.')
      let next = snapshot
      for (const env of targetEnvs) {
        const current = snapshot.targets.find(t => t.target.id === env.id)?.revision ?? ''
        next = await saveDesired({ targetId: env.id, sourceId: source, listId: source === 'shelf' ? listId : 0, skillIds: selected, mode, revision: current })
      }
      onupdate(next)
      revision = next.targets.find(t => t.target.id === targetEnvs[0].id)?.revision ?? ''
      notice = `Sorted. Desired skills saved for ${targetEnvs.length} agent ${targetEnvs.length === 1 ? 'scope' : 'scopes'} on ${selectedMachine}. Preview before approving changes.`
    })
  }
  function reviewSave() {
    if (mode !== 'additive' && onlyMachine.length) {
      confirm = { title: 'Save a set that removes extra skills?', body: `${onlyMachine.length} installed skills are outside this selection. A later approved apply can remove them from the selected agent scopes, keeping local backups. Shelf entries and notes remain. Saving alone does not change installed files.`, action: save }
    } else void save()
  }
  async function edit(item: SyncTarget) {
    selectedMachine = item.target.host
    selectedAgents = [item.target.id]
    mode = item.mode || 'mirror'; revision = item.revision
    source = item.sourceId === 'hosted' || groups.some(g => g.id === item.sourceId) ? item.sourceId : 'shelf'
    listId = item.sourceId.startsWith('list:') ? Number(item.sourceId.slice(5)) : 0
    await loadSource(); selected = item.desired.filter(s => candidates.some(c => c.id === s.id)).map(s => s.id)
    if (selected.length !== item.desired.length) notice = 'Some previous skills are no longer in this source. Review the selection and any removals.'
    document.getElementById('sync-selection')?.scrollIntoView({ behavior: 'instant', block: 'start' })
  }
  async function reconcile(item: SyncTarget) {
    await edit(item)
    await run(async () => {
      const result = await reconcileDesired(item.target.id)
      if (result.revision !== revision) throw new Error('Desired skills changed. Refresh and review again.')
      // All reported skills are already imported into the personal shelf. Offer
      // an explicit reviewed selection rather than changing shared lists silently.
      source = 'shelf'; listId = 0; await loadSource()
      selected = result.skills.map(s => s.id); conflicts = result.conflicts
      notice = 'Local and web changes combined for review. Resolve conflicts, then save the selected set. No files changed.'
    })
  }
  async function adoptSelected() { await run(async () => { const env = targetEnvs[0]; if (!env) throw new Error('Choose an agent scope first.'); await adoptSyncSkills(env.id, adopt, source === 'shelf' ? listId : 0); await loadSource(); notice = 'Selected skills added to reet.sh. Existing notes and shelf choices were preserved.' }) }
  async function previewGroup(group: { host: string; items: SyncTarget[] }) {
    const agent = snapshot.agents?.find(a => a.host.toLowerCase() === group.host.toLowerCase())
    if (!agent) return
    await run(async () => {
      let next = snapshot
      for (const item of group.items) next = await queueSyncJob(agent.id, item.target.id, 'preview')
      onupdate(next)
      notice = `Preview queued for ${group.items.length} agent ${group.items.length === 1 ? 'scope' : 'scopes'} on ${group.host}.`
    })
  }
  function approve(job: SyncJob) { confirm = { title: 'Apply this plan on the machine?', body: `${job.plan.actions?.filter(a => a.add).length ?? 0} installs and ${job.plan.actions?.filter(a => !a.add).length ?? 0} removals in ${job.plan.directory || 'the selected scope'}. Removed entries are backed up locally. The agent rechecks this exact plan before applying.`, action: () => run(async () => { onupdate(await controlSyncJob(job.id, 'approve', job.hash)); notice = 'Approved plan queued for the enrolled machine.' }) } }
  onMount(() => {
    if (view !== 'compare') return
    void (async () => {
      try {
        lists = (await listLists()).lists
        const params = new URLSearchParams(location.search)
        const requested = Number(params.get('list'))
        if (Number.isSafeInteger(requested) && requested > 0 && lists.some(list => list.id === requested)) listId = requested
        await loadSource()
        const requestedMachine = params.get('machine') ?? ''
        const match = fleet.find(m => m.host === requestedMachine)
        if (match) selectMachine(match.host)
        else if (fleet.length === 1) selectMachine(fleet[0].host)
        else {
          const withTarget = fleet.find(m => agentGroups(m).some(g => snapshot.targets.some(t => t.target.id === g.id)))
          if (withTarget) selectMachine(withTarget.host)
        }
      } catch (e) { error = e instanceof Error ? e.message : String(e) }
    })()
  })
</script>

{#if error}<p class="feedback bad" role="alert">{error}</p>{/if}
{#if notice}<p class="feedback" role="status">{notice}</p>{/if}
{#if view === 'compare'}
<section id="sync-selection" class="control" aria-labelledby="selection-title">
  <h3 id="selection-title">Reet.sh skills & your machines</h3>
  <p>Choose skills from reet.sh, a list, or another machine. Compare, save the desired set, then preview and approve changes.</p>
  <div class="pickers">
    <label>Compare from<select aria-label="Compare from" bind:value={source} onchange={loadSource} disabled={busy}><option value="shelf">Reet.sh shelf &amp; lists</option><option value="hosted">My published reet.sh skills</option>{#each fleet as m (m.host)}<optgroup label={m.host}>{#each agentGroups(m) as g (g.id)}<option value={g.id}>{groupLabel(g)}</option>{/each}</optgroup>{/each}</select></label>
    {#if source === 'shelf'}<label>Skill collection<select aria-label="Skill collection" bind:value={listId} onchange={loadSource} disabled={busy}><option value={0}>My shelf</option>{#each lists as list}<option value={list.id}>{list.name}</option>{/each}</select></label>{/if}
    <label>Machine<select aria-label="Machine" bind:value={selectedMachine} onchange={() => selectMachine(selectedMachine)} disabled={busy}><option value="">Choose a machine</option>{#each fleet as m (m.host)}<option value={m.host}>{m.host}</option>{/each}</select></label>
    <label>Sync mode<select aria-label="Sync mode" bind:value={mode} disabled={!canWrite || busy}><option value="additive">Additive</option><option value="bidirectional">Bidirectional review</option><option value="mirror">Exact set</option></select></label>
  </div>
  {#if machine}
    <div class="agent-scopes">
      <div class="scope-head"><strong>Agent scopes on {machine.host}</strong><div><button type="button" class="link" onclick={() => (selectedAgents = selectableGroups.map(g => g.id))}>All</button><button type="button" class="link" onclick={() => (selectedAgents = [])}>None</button></div></div>
      <div class="scope-grid">
        {#each selectableGroups as env (env.id)}
          <label class="scope-option" class:selected={selectedAgents.includes(env.id)}>
            <input type="checkbox" checked={selectedAgents.includes(env.id)} onchange={() => toggleAgent(env.id)} disabled={busy} />
            <span><strong>{groupLabel(env)}</strong><small>{env.skillCount ?? 0} skills · {env.complete ? 'Current' : 'Incomplete'} scan</small></span>
          </label>
        {/each}
      </div>
      {#if scanOnlyGroups.length}<p class="hint">Scan-only: {scanOnlyGroups.map(g => groupLabel(g)).join(', ')}. These scopes cannot receive sync jobs.</p>{/if}
      <p class="hint">Every selected scope receives the same desired set. Each keeps its own install target and review.</p>
    </div>
  {/if}
  <p class="hint">{mode === 'additive' ? 'Install selected skills and keep local extras.' : mode === 'bidirectional' ? 'Review changes from both sides before applying. Removals need approval.' : 'Match this exact selection. Extra skills require reviewed removal.'}</p>
  <p class="hint">Bookmarks and “In use” are personal shelf choices. They never approve installation or removal. Matching here means skill identity/source, not file contents or versions.</p>
  <label>Find skills<input type="search" bind:value={query} placeholder="Name or source" /></label>
  <div class="actions"><button class="btn quiet" disabled={loading || busy} onclick={() => selected = [...new Set([...selected, ...filtered.map(s => s.id)])]}>Select visible</button><button class="btn quiet" disabled={busy} onclick={() => selected = []}>Clear selection</button><span>{selected.length} selected</span></div>
  <div class="skill-options" aria-label="Skills to compare">
    {#if loading}<p role="status">Loading skills…</p>{:else}{#each filtered as skill (skill.id)}<label class="skill"><input type="checkbox" checked={selected.includes(skill.id)} onchange={() => toggle(skill.id)} disabled={busy} /><span><strong>{skill.name}</strong><small>{skill.source || 'Unknown source — link before syncing'}</small></span></label>{:else}<p>No skills here yet. Add skills to your shelf or report a machine inventory.</p>{/each}{/if}
  </div>
  {#if targetEnvs.length}
    <p class:bad={!ready} role="status">{ready ? `Selected scopes last reported ${ago(oldestScan)}.` : 'One or more selected scopes need a fresh complete scan. Rescan before saving or applying.'}</p>
    {#each targetEnvs.filter(e => e.error) as env (env.id)}<p class="bad">{groupLabel(env)}: {env.error}</p>{/each}
    <div class="counts"><span>{targetEnvs.length} agent {targetEnvs.length === 1 ? 'scope' : 'scopes'} selected</span><span>{onlySource.length} only {['shelf','hosted'].includes(source) ? 'on reet.sh (selected)' : 'in reference selection'}</span><span>{onlyMachine.length} only on selected scopes</span><span>{matching.length} matching</span><span>{sourceConflicts.length} source conflicts</span><span>{unknown.length} unknown</span></div>
    {#if sourceConflicts.length}<p class="bad">Source conflicts: {sourceConflicts.map(s => s.name).join(', ')}. Resolve the installed source before applying.</p>{/if}
    {#if conflicts.length}<p class="bad" role="alert">Concurrent changes: {conflicts.join(', ')}. Select the source you want for each conflict, or keep it absent, then <button class="btn quiet" onclick={() => conflicts = []}>Confirm conflict choices</button>.</p>{/if}
    <details><summary>Only on selected scopes ({onlyMachine.length})</summary><p>Select skills to add to {listId && source === 'shelf' ? 'the selected reet.sh list' : 'your reet.sh shelf'}.</p>{#each onlyMachine as skill}<label class="skill"><input type="checkbox" value={skill.id} bind:group={adopt} disabled={!canWrite || busy} /><span>{skill.name}<small>{skill.source || 'Unknown source'}</small></span></label>{/each}{#if canWrite}<button class="btn" disabled={busy || !ready || !adopt.length || (source === 'shelf' && listId > 0 && !listEditable)} onclick={adoptSelected}>Add selected to reet.sh</button>{/if}</details>
    {#if canWrite}<button class="btn primary" disabled={busy || loading || !ready || unknown.length > 0 || conflicts.length > 0} onclick={reviewSave}>{busy ? 'Saving…' : `Save to ${targetEnvs.length} agent ${targetEnvs.length === 1 ? 'scope' : 'scopes'}`}</button>{/if}
  {:else if machine}
    <p class="hint">Choose at least one agent scope on {machine.host} to compare.</p>
  {/if}
</section>
<section aria-labelledby="saved-sets">
  <h3 id="saved-sets">Synced machines</h3>
  {#each targetGroups as group (group.host)}
    {@const agent = snapshot.agents?.find(a => a.host.toLowerCase() === group.host.toLowerCase())}
    <article class="control machine-set">
      <header class="set-head">
        <div><h4>{group.host}</h4><p>{group.items.length} agent {group.items.length === 1 ? 'scope' : 'scopes'} configured</p></div>
        {#if agent}<button class="btn primary" type="button" disabled={busy || agent.paused} onclick={() => previewGroup(group)}>Preview all on machine</button>{:else}<span class="hint">Local preview commands below</span>{/if}
      </header>
      <div class="scope-sets">
        {#each group.items as item (item.target.id)}
          {@const env = groups.find(g => g.id === item.target.id)}
          <div class="scope-set">
            <div class="scope-title"><strong>{groupLabel(item.target)}</strong><span class="badge">{item.mode || 'mirror'} · {item.desired.length} desired</span></div>
            <p>{item.sourceName}</p>
            <p class:bad={!env?.complete || (env?.scannedAt ?? 0) < Date.now() - 14 * 86400000}>{env?.complete && env.scannedAt > Date.now() - 14 * 86400000 ? `${item.missing.length} to install · ${item.extra.length} extra${item.mode === 'additive' ? ' (kept)' : ' (review removals)'}` : 'Current differences unconfirmed — rescan required'}</p>
            <p class="hint">Last report: {env?.scannedAt ? ago(env.scannedAt) : 'none'} · {item.appliedRevision === item.revision ? 'Desired revision acknowledged' : 'Desired revision not yet acknowledged'}</p>
            {#if canWrite}<div class="actions"><button class="btn" disabled={busy} onclick={() => edit(item)}>Edit selection</button><button class="btn" disabled={busy || !env?.complete} onclick={() => reconcile(item)}>Review local &amp; web changes</button>{#if agent}<button class="btn" disabled={busy || agent.paused} onclick={() => run(async () => { onupdate(await queueSyncJob(agent.id, item.target.id, 'preview')); notice = 'Preview queued. The machine will report its exact plan.' })}>Preview</button>{/if}<button class="btn quiet" disabled={busy} onclick={() => confirm = { title: 'Stop tracking this set?', body: 'This removes the desired set. Installed skills, shelf entries and notes remain.', action: () => run(async () => onupdate(await deleteSyncTarget(item.target.id))) }}>Stop tracking</button></div>{/if}
            {#if !agent}<details class="local-preview"><summary>Local preview command</summary><div class="command"><code>{cli(item)}</code><CopyButton text={cli(item)} label="Copy local preview" /></div></details>{/if}
          </div>
        {/each}
      </div>
    </article>
  {:else}<p class="hint">No desired skill sets yet. Choose a machine and at least one agent scope above to get started.</p>{/each}
</section>
{/if}


<section aria-labelledby="sync-jobs-title"><details class="history" open={(snapshot.jobs ?? []).some(j => ['queued','running','awaiting-approval'].includes(j.status))}><summary id="sync-jobs-title">Sync activity ({snapshot.jobs?.length ?? 0}){snapshot.jobs?.[0] ? ` · latest: ${snapshot.jobs[0].status.replaceAll('-', ' ')}` : ''}</summary>
{#each snapshot.jobs ?? [] as job (job.id)}
 <article class="control"><h4>{snapshot.agents?.find(a => a.id === job.agentId)?.host || 'Machine'} · {job.kind}</h4><p role="status">{job.status.replaceAll('-', ' ')} · {ago(job.updatedAt)}</p>
 {#if job.message}<p class="bad">{job.message}</p>{/if}
 {#if job.plan.backups?.length}<details><summary>Restore removed skills</summary><p>Run on the enrolled machine. Restore asks for confirmation and never overwrites an existing skill.</p>{#each job.plan.backups as backup}<div class="command"><code>{`reet agent restore --target ${quote(job.targetId)} --backup ${quote(backup)}`}</code><CopyButton text={`reet agent restore --target ${quote(job.targetId)} --backup ${quote(backup)}`} label="Copy restore command" /></div>{/each}</details>{/if}
 {#if job.plan.actions}<details open={job.status === 'awaiting-approval'}><summary>Plan: {job.plan.actions.length} {job.plan.actions.length === 1 ? 'change' : 'changes'}</summary><p><code>{job.plan.directory}</code></p><ul>{#each job.plan.actions as action}<li>{action.add ? 'Install' : 'Remove'} {action.skill.name} <small>{action.skill.source}</small></li>{:else}<li>Already matches the desired set.</li>{/each}</ul></details>{/if}
 {#if canWrite}<div class="actions">{#if job.status === 'awaiting-approval'}<button class="btn primary" disabled={busy} onclick={() => approve(job)}>Review & approve</button>{/if}{#if ['queued','awaiting-approval'].includes(job.status)}<button class="btn quiet" disabled={busy} onclick={() => run(async () => onupdate(await controlSyncJob(job.id, 'cancel')))}>Cancel</button>{/if}{#if ['failed','cancelled'].includes(job.status)}<button class="btn" disabled={busy} onclick={() => run(async () => onupdate(await queueSyncJob(job.agentId, job.targetId, job.kind === 'scan' ? 'scan' : 'preview')))}>Retry with fresh {job.kind === 'scan' ? 'scan' : 'preview'}</button>{/if}</div>{/if}
 </article>
{:else}<p class="hint">No jobs yet. Preview a desired set or request a scan from Machines.</p>{/each}
</details></section>

{#if confirm}<ConfirmDialog title={confirm.title} body={confirm.body} confirmLabel="Confirm" oncancel={() => confirm = null} onconfirm={() => { const action = confirm?.action; confirm = null; void action?.() }} />{/if}
<style>
 .control { border:1px solid var(--line); border-radius:10px; padding:18px; margin:16px 0; min-width:0; }
 h3,h4 { margin:0 0 10px; } p,.hint { color:var(--muted); line-height:1.6; font-size:.85rem; } .hint {font-size:.8rem}
 .pickers {display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px} label{display:grid;gap:6px;font-size:.85rem;font-weight:600} select,input[type=search]{width:100%;min-width:0}
 .skill-options{max-height:320px;overflow:auto;border:1px solid var(--line);border-radius:8px;margin:12px 0;padding:8px}
 .skill{display:flex;align-items:center;gap:10px;padding:10px;font-weight:400;border-bottom:1px solid var(--line)} .skill input{flex-shrink:0} small{display:block;font-size:.75rem;color:var(--muted);overflow-wrap:anywhere}
 .actions,.counts,.command{display:flex;gap:10px;flex-wrap:wrap;align-items:center;margin:14px 0}.counts span{border:1px solid var(--line);border-radius:16px;padding:5px 10px;font-size:.8rem}
 .feedback{padding:12px;border:1px solid var(--good);border-radius:8px}.bad{color:var(--bad);border-color:var(--bad)}details{margin:16px 0}summary{cursor:pointer;font-weight:600}code,li,h4{overflow-wrap:anywhere}section{margin:20px 0} .command code{font-size:.78rem}
 .agent-scopes{margin:16px 0;padding:14px;border:1px solid var(--line);border-radius:10px;background:var(--bg)}
 .scope-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;font-size:.85rem}
 .scope-head > div{display:flex;gap:10px}
 .link{padding:0;border:0;background:none;color:var(--accent);font:inherit;font-size:.76rem;font-weight:600;text-decoration:underline;text-underline-offset:3px;cursor:pointer}
 .scope-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:8px;margin:10px 0 8px}
 .scope-option{display:flex;align-items:flex-start;gap:8px;padding:9px 10px;border:1px solid var(--line);border-radius:8px;background:var(--card);font-weight:400;cursor:pointer}
 .scope-option.selected{border-color:color-mix(in srgb,var(--accent) 45%,var(--line));background:var(--tint)}
 .scope-option input{flex:none;margin-top:2px}
 .scope-option span{display:grid;gap:2px;min-width:0}
 .scope-option strong{font-size:.8rem;overflow-wrap:anywhere}
 .scope-option small{font-size:.71rem}
 .machine-set{padding:0;overflow:clip}
 .set-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;padding:16px 18px;border-bottom:1px solid var(--line);background:var(--bg)}
 .set-head h4{margin:0}
 .set-head p{margin:3px 0 0;font-size:.78rem}
 .scope-sets{display:grid}
 .scope-set{padding:14px 18px;border-top:1px solid var(--line)}
 .scope-set:first-child{border-top:0}
 .scope-title{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
 .scope-title strong{font-size:.85rem}
 .badge{padding:3px 8px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:.7rem}
 .scope-set > p{margin:5px 0}
 .local-preview{margin:8px 0 0}
 .local-preview summary{font-size:.78rem}
 @media(max-width:600px){.pickers{grid-template-columns:1fr}.control{padding:12px}.actions .btn{flex:1 1 auto}.scope-grid{grid-template-columns:1fr}.set-head{align-items:flex-start}.set-head .btn{width:100%}}
</style>
