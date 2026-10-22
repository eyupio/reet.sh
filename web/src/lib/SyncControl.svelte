<script lang="ts">
  import { onMount } from 'svelte'
  import { syncCandidates, listLists, getList, saveDesired, reconcileDesired, adoptSyncSkills, queueSyncJob, controlSyncJob, controlSyncAgent, deleteSyncTarget, type SyncState, type SyncSkill, type SharedList, type SyncTarget, type SyncJob } from './api'
  import { groupLabel } from './machines'
  import { elapsed } from './time'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  let { snapshot, canWrite, onupdate, view = 'compare' }: { snapshot: SyncState; canWrite: boolean; onupdate: (s: SyncState) => void; view?: 'machines' | 'compare' } = $props()
  let source = $state('shelf'), target = $state(''), listId = $state(0), mode = $state('additive')
  let candidates = $state<SyncSkill[]>([]), selected = $state<string[]>([]), adopt = $state<string[]>([]), lists = $state<SharedList[]>([])
  let revision = $state(''), busy = $state(false), loading = $state(false), error = $state(''), notice = $state(''), query = $state('')
  let listEditable = $state(false)
  let conflicts = $state<string[]>([])
  let confirm = $state<{ title: string; body: string; action: () => Promise<void> } | null>(null)
  const groups = $derived(snapshot.environments.filter(e => !e.agent.startsWith('reet')))
  const targetEnv = $derived(groups.find(e => e.id === target))
  const desired = $derived(candidates.filter(s => selected.includes(s.id)))
  const actual = $derived(targetEnv?.skills ?? [])
  const onlySource = $derived(desired.filter(s => !actual.some(a => a.id === s.id)))
  const onlyMachine = $derived(actual.filter(s => !desired.some(d => d.id === s.id)))
  const matching = $derived(desired.filter(s => actual.some(a => a.id === s.id)))
  const sourceConflicts = $derived(desired.filter(s => actual.some(a => a.slug === s.slug && a.id !== s.id)))
  const unknown = $derived(desired.filter(s => !s.source || s.sourceType === 'local'))
  const ready = $derived(!!targetEnv?.complete && (targetEnv?.scannedAt ?? 0) > Date.now() - 14 * 86400000)
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
  function selectTarget() { revision = snapshot.targets.find(t => t.target.id === target)?.revision ?? ''; adopt = []; conflicts = [] }
  async function save() {
    await run(async () => {
      const next = await saveDesired({ targetId: target, sourceId: source, listId: source === 'shelf' ? listId : 0, skillIds: selected, mode, revision })
      onupdate(next); revision = next.targets.find(t => t.target.id === target)?.revision ?? ''
      notice = 'Sorted. Desired skills saved. Preview the plan on the machine before approving changes.'
    })
  }
  function reviewSave() {
    if (mode !== 'additive' && onlyMachine.length) {
      confirm = { title: 'Save a set that removes extra skills?', body: `${onlyMachine.length} installed skills are outside this selection. A later approved apply can remove them from the selected agent scope, keeping local backups. Shelf entries and notes remain. Saving alone does not change installed files.`, action: save }
    } else void save()
  }
  async function edit(item: SyncTarget) {
    target = item.target.id; mode = item.mode || 'mirror'; revision = item.revision
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
  async function adoptSelected() { await run(async () => { await adoptSyncSkills(target, adopt, source === 'shelf' ? listId : 0); await loadSource(); notice = 'Selected skills added to reet.sh. Existing notes and shelf choices were preserved.' }) }
  function approve(job: SyncJob) { confirm = { title: 'Apply this plan on the machine?', body: `${job.plan.actions?.filter(a => a.add).length ?? 0} installs and ${job.plan.actions?.filter(a => !a.add).length ?? 0} removals in ${job.plan.directory || 'the selected scope'}. Removed entries are backed up locally. The agent rechecks this exact plan before applying.`, action: () => run(async () => { onupdate(await controlSyncJob(job.id, 'approve', job.hash)); notice = 'Approved plan queued for the enrolled machine.' }) } }
  onMount(() => { if (view === 'compare') { void loadSource(); void listLists().then(r => lists = r.lists).catch(e => error = String(e)) } })
</script>

{#if error}<p class="feedback bad" role="alert">{error}</p>{/if}
{#if notice}<p class="feedback" role="status">{notice}</p>{/if}
{#if view === 'compare'}
<section id="sync-selection" class="control" aria-labelledby="selection-title">
  <h3 id="selection-title">Reet.sh skills & your machines</h3>
  <p>Choose skills from reet.sh, a list, or another machine. Compare, save the desired set, then preview and approve changes.</p>
  <div class="pickers">
    <label>Compare from<select aria-label="Compare from" bind:value={source} onchange={loadSource} disabled={busy}><option value="shelf">Reet.sh shelf & lists</option><option value="hosted">My published reet.sh skills</option>{#each groups as g}<option value={g.id}>{g.host} · {groupLabel(g)}</option>{/each}</select></label>
    {#if source === 'shelf'}<label>Skill collection<select aria-label="Skill collection" bind:value={listId} onchange={loadSource} disabled={busy}><option value={0}>My shelf</option>{#each lists as list}<option value={list.id}>{list.name}</option>{/each}</select></label>{/if}
    <label>Machine & agent scope<select aria-label="Machine & agent scope" bind:value={target} onchange={selectTarget} disabled={busy}><option value="">Choose a target</option>{#each groups as g}<option value={g.id} disabled={!['agents','claude','cursor','opencode'].includes(g.agent)}>{g.host} · {groupLabel(g)}</option>{/each}</select></label>
    <label>Sync mode<select aria-label="Sync mode" bind:value={mode} disabled={!canWrite || busy}><option value="additive">Additive</option><option value="bidirectional">Bidirectional review</option><option value="mirror">Exact set</option></select></label>
  </div>
  <p class="hint">{mode === 'additive' ? 'Install selected skills and keep local extras.' : mode === 'bidirectional' ? 'Review changes from both sides before applying. Removals need approval.' : 'Match this exact selection. Extra skills require reviewed removal.'}</p>
  <p class="hint">Bookmarks and “In use” are personal shelf choices. They never approve installation or removal. Matching here means skill identity/source, not file contents or versions.</p>
  <label>Find skills<input type="search" bind:value={query} placeholder="Name or source" /></label>
  <div class="actions"><button class="btn quiet" disabled={loading || busy} onclick={() => selected = [...new Set([...selected, ...filtered.map(s => s.id)])]}>Select visible</button><button class="btn quiet" disabled={busy} onclick={() => selected = []}>Clear selection</button><span>{selected.length} selected</span></div>
  <div class="skill-options" aria-label="Skills to compare">
    {#if loading}<p role="status">Loading skills…</p>{:else}{#each filtered as skill (skill.id)}<label class="skill"><input type="checkbox" checked={selected.includes(skill.id)} onchange={() => toggle(skill.id)} disabled={busy} /><span><strong>{skill.name}</strong><small>{skill.source || 'Unknown source — link before syncing'}</small></span></label>{:else}<p>No skills here yet. Add skills to your shelf or report a machine inventory.</p>{/each}{/if}
  </div>
  {#if targetEnv}
    <p class:bad={!ready} role="status">{ready ? `Last complete report: ${ago(targetEnv.scannedAt)}` : 'Scan incomplete or out of date. Rescan before saving or applying.'}{targetEnv.error ? ` — ${targetEnv.error}` : ''}</p>
    <div class="counts"><span>{onlySource.length} only {['shelf','hosted'].includes(source) ? 'on reet.sh (selected)' : 'in reference selection'}</span><span>{onlyMachine.length} only on machine</span><span>{matching.length} matching</span><span>{sourceConflicts.length} source conflicts</span><span>{unknown.length} unknown</span></div>
    {#if sourceConflicts.length}<p class="bad">Source conflicts: {sourceConflicts.map(s => s.name).join(', ')}. Resolve the installed source before applying.</p>{/if}
    {#if conflicts.length}<p class="bad" role="alert">Concurrent changes: {conflicts.join(', ')}. Select the source you want for each conflict, or keep it absent, then <button class="btn quiet" onclick={() => conflicts = []}>Confirm conflict choices</button>.</p>{/if}
    <details><summary>Only on machine ({onlyMachine.length})</summary><p>Select skills to add to {listId && source === 'shelf' ? 'the selected reet.sh list' : 'your reet.sh shelf'}.</p>{#each onlyMachine as skill}<label class="skill"><input type="checkbox" value={skill.id} bind:group={adopt} disabled={!canWrite || busy} /><span>{skill.name}<small>{skill.source || 'Unknown source'}</small></span></label>{/each}{#if canWrite}<button class="btn" disabled={busy || !ready || !adopt.length || (source === 'shelf' && listId > 0 && !listEditable)} onclick={adoptSelected}>Add selected to reet.sh</button>{/if}</details>
    {#if canWrite}<button class="btn primary" disabled={busy || loading || !ready || unknown.length > 0 || conflicts.length > 0} onclick={reviewSave}>{busy ? 'Saving…' : 'Save desired skills'}</button>{/if}
  {/if}
</section>
<section aria-labelledby="saved-sets"><h3 id="saved-sets">Desired skill sets</h3>
{#each snapshot.targets as item (item.target.id)}
  {@const env = groups.find(g => g.id === item.target.id)}
  {@const agent = snapshot.agents?.find(a => a.host.toLowerCase() === item.target.host.toLowerCase())}
  <article class="control"><h4>{item.target.host} · {groupLabel(item.target)}</h4><p>{item.sourceName} · {item.mode || 'mirror'} · {item.desired.length} desired skills</p>
    <p class:bad={!env?.complete || (env?.scannedAt ?? 0) < Date.now()-14*86400000}>{env?.complete && env.scannedAt > Date.now()-14*86400000 ? `${item.missing.length} to install · ${item.extra.length} extra${item.mode === 'additive' ? ' (kept)' : ' (review removals)'}` : 'Current differences unconfirmed — rescan required'}</p>
    <p class="hint">Last report: {env?.scannedAt ? `${ago(env.scannedAt)}` : 'none'} · {item.appliedRevision === item.revision ? 'Desired revision acknowledged' : 'Desired revision not yet acknowledged'}</p>
    {#if canWrite}<div class="actions"><button class="btn" disabled={busy} onclick={() => edit(item)}>Edit selection</button><button class="btn" disabled={busy || !env?.complete} onclick={() => reconcile(item)}>Review local & web changes</button>
    {#if agent}<button class="btn primary" disabled={busy || agent.paused} onclick={() => run(async () => { onupdate(await queueSyncJob(agent.id, item.target.id, 'preview')); notice = 'Preview queued. The machine will report its exact plan.' })}>Preview on machine</button>{:else}<p class="hint">Enroll the machine to use remote controls. Local preview:</p><div class="command"><code>{cli(item)}</code><CopyButton text={cli(item)} label="Copy local preview" /></div>{/if}
    <button class="btn quiet" disabled={busy} onclick={() => confirm = { title: 'Stop tracking this set?', body: 'This removes the desired set. Installed skills, shelf entries and notes remain.', action: () => run(async () => onupdate(await deleteSyncTarget(item.target.id))) }}>Stop tracking</button></div>{/if}
  </article>
{:else}<p class="hint">No desired skill sets yet. Choose reet.sh skills and a target above to get started.</p>{/each}
</section>
{/if}

{#if view === 'machines'}
<section aria-labelledby="enrolled-title"><h3 id="enrolled-title">Enrolled machines</h3><p class="hint">Enrolled agents report each minute and process queued jobs. Apply must also be enabled locally with <code>--allow-apply</code>.</p>
{#each snapshot.agents ?? [] as agent (agent.id)}
  <article class="control"><h4>{agent.host}</h4><p>{agent.paused ? 'Paused' : agent.lastSeen > Date.now()-90000 ? 'Agent connected' : 'Agent offline'} · {agent.version} · seen {ago(agent.lastSeen)}</p>
    {#if canWrite}<div class="actions"><button class="btn" disabled={busy || agent.paused} onclick={() => run(async () => { onupdate(await queueSyncJob(agent.id, '', 'scan')); notice = 'Scan queued.' })}>Scan now</button><button class="btn" disabled={busy} onclick={() => run(async () => onupdate(await controlSyncAgent(agent.id, agent.paused ? 'resume' : 'pause')))}>{agent.paused ? 'Resume' : 'Pause'}</button><button class="btn quiet" disabled={busy} onclick={() => confirm = { title: `Revoke ${agent.host}?`, body: 'The agent cannot claim further jobs. An operation already executing may finish. Installed files and shelf entries remain.', action: () => run(async () => onupdate(await controlSyncAgent(agent.id, 'revoke'))) }}>Revoke enrollment</button></div>{/if}
  </article>
{:else}<p>No machines enrolled yet. Use Connect a machine to enroll the binary.</p>{/each}
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
 @media(max-width:600px){.pickers{grid-template-columns:1fr}.control{padding:12px}.actions .btn{flex:1 1 auto}}
</style>
