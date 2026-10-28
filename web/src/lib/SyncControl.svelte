<script lang="ts">
  import { onMount } from 'svelte'
  import { syncCandidates, listLists, getList, saveDesired, reconcileDesired, adoptSyncSkills, queueSyncJob, controlSyncJob, deleteSyncTarget, updateSyncAgent, type SyncState, type SyncSkill, type SharedList, type SyncTarget, type SyncJob } from './api'
  import { agentGroups, agentOnline, groupLabel, inventory, inventoryMachines } from './machines'
  import { elapsed } from './time'
  import ContentObservation from './ContentObservation.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import SyncProblem from './SyncProblem.svelte'
  import FleetActions from './FleetActions.svelte'
  import { destinationScopes } from './fleet'
  import { dedupeSkills, linked as isLinked, localDuplicates as findLocalDuplicates, sourceConflicts as findSourceConflicts } from './syncSelection'
  import { syncActivityOverview, type SyncActivityEntry, type SyncHostActivity } from './syncActivity'
  import { jobPlanCounts, syncJobStages } from './syncOverview'
  import { startSyncRun } from './api'
  let { snapshot, canWrite, onupdate, view = 'compare' }: { snapshot: SyncState; canWrite: boolean; onupdate: (s: SyncState) => void; view?: 'machines' | 'compare' } = $props()
  let source = $state('shelf'), selectedMachine = $state(''), selectedAgents = $state<string[]>([]), listId = $state(0), mode = $state('additive')
  let candidates = $state<SyncSkill[]>([]), selected = $state<string[]>([]), adopt = $state<string[]>([]), lists = $state<SharedList[]>([])
  let revision = $state(''), busy = $state(false), loading = $state(false), error = $state(''), notice = $state(''), query = $state('')
  let listEditable = $state(false), sourceLoaded = $state(false)
  let conflicts = $state<string[]>([])
  let confirm = $state<{ title: string; body: string; action: () => Promise<void> } | null>(null)
  const groups = $derived(snapshot.environments.filter(e => !e.agent.startsWith('reet')))
  const fleet = $derived(inventoryMachines(snapshot.machines ?? []))
  const machine = $derived(fleet.find(m => m.host === selectedMachine) ?? null)
  const machineGroups = $derived(destinationScopes(fleet, selectedMachine))
  const selectableGroups = $derived(machineGroups.filter(g => ['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)))
  const scanOnlyGroups = $derived(machineGroups.filter(g => !['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)))
  const targetEnvs = $derived(selectableGroups.filter(g => selectedAgents.includes(g.id)))
  const actual = $derived(inventory(targetEnvs))
  const desired = $derived(dedupeSkills(candidates.filter(s => selected.includes(s.id))))
  const onlySource = $derived(desired.filter(s => !actual.some(a => a.id === s.id)))
  const onlyMachine = $derived(actual.filter(s => !desired.some(d => d.id === s.id || d.slug === s.slug)))
  const matching = $derived(desired.filter(s => actual.some(a => a.id === s.id)))
  const scopeDifferences = $derived(targetEnvs.map(env => ({ env, missing: desired.filter(s => !env.skills.some(a => a.id === s.id)).length, extra: env.skills.filter(s => !desired.some(d => d.id === s.id || d.slug === s.slug)).length })))
  const sourceConflicts = $derived(findSourceConflicts(desired, actual))
  const localDuplicates = $derived(findLocalDuplicates(desired, actual))
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
  const activity = $derived(syncActivityOverview(snapshot.jobs ?? [], snapshot))
  const activeHosts = $derived(activity.hosts.filter(host => host.entries.some(entry => !entry.resolved)))
  const finished = $derived(activity.hosts.flatMap(host => host.entries.filter(entry => entry.resolved)).sort((a, b) => b.job.updatedAt - a.job.updatedAt))
  const latestFinished = $derived(finished[0])
  const olderFinished = $derived(finished.slice(1))
  const firstActionKey = $derived(activity.hosts.flatMap(host => host.entries).find(entry => entry.failed || entry.review)?.key ?? '')
  const history = $derived(activeHosts.length ? finished : olderFinished)
  const failedHosts = $derived(activity.hosts.filter(host => host.failed))
  const activityHeadline = $derived.by(() => {
    if (activity.active) return `${activity.active} sync ${activity.active === 1 ? 'job' : 'jobs'} in progress`
    if (activity.failed) return failedHosts.length === 1 ? `Sync needs attention on ${failedHosts[0].host}` : `Sync needs attention on ${failedHosts.length} machines`
    if (activity.review) return `${activity.review} ${activity.review === 1 ? 'plan needs' : 'plans need'} review`
    if (activity.cancelled) return `${activity.cancelled} ${activity.cancelled === 1 ? 'job was' : 'jobs were'} cancelled`
    if (activity.done) return 'Sync is up to date'
    return 'No sync activity yet'
  })
  const activityDetail = $derived.by(() => {
    if (activity.active) {
      const waiting = []
      if (activity.failed) waiting.push(`${activity.failed} failed`)
      if (activity.review) waiting.push(`${activity.review} to review`)
      return waiting.length ? `${waiting.join(' · ')}. Open a job below to see the detail.` : 'Machines report each step here as it completes.'
    }
    if (activity.failed) return failedHosts.length === 1
      ? `${activity.failed} ${activity.failed === 1 ? 'job' : 'jobs'} failed on ${failedHosts[0].host}. Open one to see the exact fix.`
      : `${activity.failed} jobs failed across ${failedHosts.length} machines. Open one to see the exact fix.`
    if (activity.review) return 'Nothing changes until you approve a plan.'
    if (activity.cancelled) return 'A cancelled job changed no files.'
    if (latestFinished) return `Last job finished ${ago(latestFinished.job.updatedAt)}.`
    return 'Preview a saved set or request a scan to start.'
  })
  function ago(at: number) { const value = elapsed(at); return value === 'just now' ? value : `${value} ago` }
  function quote(v: string) { return "'" + v.replaceAll("'", "'\\''") + "'" }
  function cli(item: SyncTarget) { return `reet sync --server ${quote(location.origin)} --env ${quote(item.target.host)} --target ${quote(item.target.id)}` }
  function preferred(id: string): string {
    const skill = candidates.find(s => s.id === id)
    if (!skill || isLinked(skill)) return id
    return candidates.find(s => s.slug === skill.slug && isLinked(s))?.id ?? id
  }
  function toggle(id: string) {
    const choice = preferred(id)
    selected = selected.includes(choice) ? selected.filter(v => v !== choice) : [...selected, choice]
    conflicts = conflicts.filter(name => candidates.find(s => s.id === choice)?.slug !== name)
  }
  async function run(fn: () => Promise<void>) { if (busy) return; busy = true; error = ''; notice = ''; try { await fn() } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false } }
  let request = 0
  async function loadSource() {
    const generation = ++request; loading = true; sourceLoaded = false; error = ''; candidates = []; selected = []; adopt = []; conflicts = []; query = ''
    try {
      let next: SyncSkill[]
      if (source === 'shelf' || source === 'hosted') {
        next = (await syncCandidates(source === 'hosted' ? 0 : listId, source)).skills
        const detail = source === 'shelf' && listId ? await getList(listId) : null
        if (generation !== request) return
        listEditable = !detail || detail.canEdit
      } else { next = groups.find(e => e.id === source)?.skills ?? []; listEditable = false }
      if (generation === request) { candidates = next; sourceLoaded = true }
    } catch (e) { if (generation === request) error = String(e) } finally { if (generation === request) loading = false }
  }
  function selectMachine(host: string) {
    selectedMachine = host
    selectedAgents = destinationScopes(fleet, host).filter(g => ['agents', 'claude', 'cursor', 'opencode'].includes(g.agent)).map(g => g.id)
    revision = ''
    adopt = []
    conflicts = []
  }
  function toggleAgent(id: string) {
    revision = ''
    selectedAgents = selectedAgents.includes(id) ? selectedAgents.filter(value => value !== id) : [...selectedAgents, id]
    adopt = []
    conflicts = []
  }
  async function save(revisions: Map<string, string>) {
    await run(async () => {
      if (!sourceLoaded) throw new Error('Load and review the source before saving.')
      if (!targetEnvs.length) throw new Error('Choose at least one destination scope.')
      const destinations = [...targetEnvs]
      let saved = 0
      for (const env of destinations) {
        const current = revisions.get(env.id) ?? ''
        try {
          const next = await saveDesired({ targetId: env.id, sourceId: source, listId: source === 'shelf' ? listId : 0, skillIds: selected, mode, revision: current })
          onupdate(next)
          if (destinations.length === 1) revision = next.targets.find(t => t.target.id === env.id)?.revision ?? ''
          saved++
        } catch (error) {
          throw new Error(`${saved} of ${destinations.length} scopes saved. Could not save ${env.host} · ${groupLabel(env)}: ${error instanceof Error ? error.message : String(error)}. Review the saved sets before retrying.`)
        }
      }
      notice = `Sorted. Desired skills saved for ${saved} scopes across ${new Set(destinations.map(env => env.host)).size} machines. Preview before approving changes.`
    })
  }
  function reviewSave() {
    const revisions = new Map(targetEnvs.map(env => [env.id, targetEnvs.length === 1 && revision ? revision : snapshot.targets.find(t => t.target.id === env.id)?.revision ?? '']))
    const commit = () => save(revisions)
    if (mode !== 'additive' && onlyMachine.length) {
      confirm = { title: 'Save a set that removes extra skills?', body: `${onlyMachine.length} installed skills are outside this selection. A later approved apply can remove them from the selected agent scopes, keeping local backups. Shelf entries and notes remain. Saving alone does not change installed files.`, action: commit }
    } else void commit()
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
  async function adoptSelected() {
    await run(async () => {
      const remaining = new Set(adopt)
      let adopted = 0
      for (const env of targetEnvs) {
        const ids = env.skills.filter(skill => remaining.has(skill.id)).map(skill => skill.id)
        if (!ids.length) continue
        try { await adoptSyncSkills(env.id, ids, source === 'shelf' ? listId : 0) }
        catch (error) { throw new Error(`${adopted} skills added before ${env.host} failed: ${error instanceof Error ? error.message : String(error)}. Review the library before retrying.`) }
        ids.forEach(id => remaining.delete(id)); adopted += ids.length
      }
      if (remaining.size) throw new Error('Some selected skills are no longer reported. Refresh inventories and review again.')
      await loadSource()
      notice = `${adopted} selected skills added to reet.sh. Existing notes and shelf choices were preserved.`
    })
  }
  async function syncGroup(group: { host: string; items: SyncTarget[] }) {
    const agent = snapshot.agents?.find(a => a.host.toLowerCase() === group.host.toLowerCase())
    if (!agent) return
    await run(async () => {
      onupdate(await startSyncRun(agent.id, group.items.map(item => item.target.id)))
      notice = `Automatic sync started for ${group.host}. Additive plans apply on their own; removals wait for review.`
    })
  }
  function online(job: SyncJob): boolean {
    return agentOnline(snapshot.agents?.find(a => a.id === job.agentId))
  }
  let retryResults = $state<Record<string, { text: string; bad: boolean }>>({})
  let retryingEntry = $state('')
  function retryResult(key: string, text: string, bad = false) {
    retryResults = { ...retryResults, [key]: { text, bad } }
  }
  function retried(key: string): boolean {
    const result = retryResults[key]
    return !!result && !result.bad
  }
  function kindName(kind: string): string {
    return kind === 'apply' ? 'Apply' : kind === 'scan' ? 'Scan' : kind === 'preview' ? 'Preview' : kind === 'update' ? 'Update' : 'Sync'
  }
  function entryTitle(entry: SyncActivityEntry): string {
    const kind = kindName(entry.job.kind)
    if (entry.active) return entry.job.status === 'queued' ? 'Waiting for machine' : `${kind} in progress`
    if (entry.failed) return `${kind} failed`
    if (entry.review) return 'Plan ready to review'
    if (entry.cancelled) return `${kind} cancelled`
    return entry.job.kind === 'apply' ? 'Changes applied' : `${kind} complete`
  }
  function statusWord(entry: SyncActivityEntry): string {
    if (entry.active) return entry.job.status === 'queued' ? 'Queued' : 'Running'
    if (entry.review) return 'Review'
    if (entry.failed) return 'Failed'
    if (entry.cancelled) return 'Cancelled'
    return 'Done'
  }
  function entryTone(entry: SyncActivityEntry): string {
    if (entry.active) return 'work'
    if (entry.review) return 'warn'
    if (entry.failed) return 'bad'
    if (entry.cancelled) return 'quiet'
    return 'ok'
  }
  function scopeText(entry: SyncActivityEntry): string {
    if (!entry.scopes.length) return entry.job.kind === 'update' ? '' : 'Machine'
    if (entry.scopes.length <= 2) return entry.scopes.join(', ')
    return `${entry.scopes[0]} +${entry.scopes.length - 1} more`
  }
  function entryMeta(entry: SyncActivityEntry): string {
    const parts = []
    const scopes = scopeText(entry)
    if (scopes) parts.push(scopes)
    if (entry.jobs.length > 1) parts.push(entry.scopes.length === 1 ? `${entry.jobs.length} attempts` : `${entry.jobs.length} jobs`)
    parts.push(ago(entry.job.updatedAt))
    return parts.join(' · ')
  }
  function hostSummary(host: SyncHostActivity): string {
    const parts = []
    if (host.active) parts.push(`${host.active} running`)
    if (host.failed) parts.push(`${host.failed} failed`)
    if (host.review) parts.push(`${host.review} to review`)
    return parts.join(' · ')
  }
  function retryJobs(entry: SyncActivityEntry): SyncJob[] {
    const seen = new Set<string>()
    return entry.jobs.filter(job => {
      const key = job.kind === 'update' ? `agent:${job.agentId}` : `target:${job.targetId}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
  }
  // A grouped failure retries each scope once. Two attempts against the same
  // scope are one retry, so a repeated preview cannot pile up duplicates.
  async function retryEntry(entry: SyncActivityEntry) {
    if (busy || retryingEntry || !canWrite) return
    busy = true
    retryingEntry = entry.key
    const pending = { ...retryResults }
    delete pending[entry.key]
    retryResults = pending
    try {
      const jobs = retryJobs(entry)
      let next = snapshot
      let kind = 'preview'
      for (const job of jobs) {
        if (job.kind === 'update') {
          kind = 'update'
          next = await updateSyncAgent(job.agentId, job.revision || 'latest')
        } else {
          kind = job.kind === 'scan' ? 'scan' : 'preview'
          next = await queueSyncJob(job.agentId, job.targetId, kind)
        }
        onupdate(next)
      }
      retryResult(entry.key, jobs.length > 1
        ? `Fresh ${kind} queued for ${jobs.length} jobs on ${entry.host}. They appear at the top of Sync activity.`
        : `Fresh ${kind} queued for ${entry.host}. It appears at the top of Sync activity.`)
    } catch (e) {
      retryResult(entry.key, e instanceof Error ? e.message : String(e), true)
    } finally {
      retryingEntry = ''
      busy = false
    }
  }
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
  function cancelJob(job: SyncJob) { void run(async () => onupdate(await controlSyncJob(job.id, 'cancel'))) }
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
        if (requestedMachine === '*') selectMachine('*')
        else if (match) selectMachine(match.host)
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

{#snippet jobRow(entry: SyncActivityEntry, startOpen: boolean)}
  {@const counts = jobPlanCounts(entry.job)}
  <details class="activity-job" class:is-active={entry.active} open={startOpen}>
    <summary>
      <span class="job-mark {entryTone(entry)}" aria-hidden="true">
        {#if entry.failed}!{:else if entry.review}?{:else if entry.active}<span class="pulse"></span>{:else if entry.cancelled}–{:else}✓{/if}
      </span>
      <span class="job-copy"><strong>{entryTitle(entry)}</strong><small>{entryMeta(entry)}</small></span>
      <span class="job-end">
        <span class="job-state {entryTone(entry)}">{statusWord(entry)}</span>
        <svg class="job-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="m6 9 6 6 6-6"></path></svg>
      </span>
    </summary>
    <div class="job-body">
      {#if entry.active || entry.review || entry.failed}
        <ol class="stages" aria-label="Job steps">
          {#each syncJobStages(entry.job) as stage (stage.key)}
            <li class={stage.state}>{stage.label}</li>
          {/each}
        </ol>
      {/if}
      {#if counts.total && entry.jobs.length === 1}
        <p class="job-facts">{counts.installs} install{counts.installs === 1 ? '' : 's'}{counts.removals ? ` · ${counts.removals} removal${counts.removals === 1 ? '' : 's'}` : ''}</p>
      {/if}
      {#if entry.failed && entry.job.message}
        <SyncProblem message={entry.job.message} host={entry.host} site={location.origin} attempts={entry.jobs.length} online={online(entry.job)}
          retrying={retryingEntry === entry.key} onretry={canWrite && !retried(entry.key) ? () => void retryEntry(entry) : undefined} />
      {:else if entry.job.message}
        <p class="job-message">{entry.job.message}</p>
      {/if}
      {#if retryResults[entry.key]}
        <p class="retry-result" class:bad={retryResults[entry.key].bad} role={retryResults[entry.key].bad ? 'alert' : 'status'}>{retryResults[entry.key].text}</p>
      {/if}
      {#if entry.failed && !entry.job.message && canWrite}
        <div class="actions"><button class="btn" disabled={busy} onclick={() => void retryEntry(entry)}>{entry.job.kind === 'update' ? 'Try update again' : `Retry with fresh ${entry.job.kind === 'scan' ? 'scan' : 'preview'}`}</button></div>
      {/if}
      {#if entry.cancelled && canWrite}
        <div class="actions"><button class="btn" disabled={busy} onclick={() => void retryEntry(entry)}>{entry.job.kind === 'update' ? 'Try update again' : `Retry with fresh ${entry.job.kind === 'scan' ? 'scan' : 'preview'}`}</button></div>
      {/if}
      {#if entry.review && canWrite}
        <div class="actions"><button class="btn primary" disabled={busy} onclick={() => approve(entry.job)}>Review &amp; approve</button><button class="btn quiet" disabled={busy} onclick={() => cancelJob(entry.job)}>Cancel</button></div>
      {:else if entry.active && entry.job.status === 'queued' && canWrite}
        <div class="actions"><button class="btn quiet" disabled={busy} onclick={() => cancelJob(entry.job)}>Cancel</button></div>
      {/if}
      {#if entry.jobs.length === 1 && entry.job.plan?.actions}
        <details class="plan">
          <summary>Plan · {entry.job.plan.actions.length} {entry.job.plan.actions.length === 1 ? 'change' : 'changes'}</summary>
          {#if entry.job.plan.directory}<p class="plan-dir"><code>{entry.job.plan.directory}</code></p>{/if}
          <ul>
            {#each entry.job.plan.actions as action}
              <li>{action.add ? 'Install' : 'Remove'} <strong>{action.skill.name}</strong> <small>{action.skill.source}</small></li>
            {:else}<li>Already matches the desired set.</li>{/each}
          </ul>
        </details>
      {/if}
      {#if entry.jobs.length === 1 && entry.job.plan?.backups?.length}
        <details class="plan">
          <summary>Restore removed skills</summary>
          <p class="hint">Run on the enrolled machine. Restore asks for confirmation and never overwrites an existing skill.</p>
          {#each entry.job.plan.backups as backup}
            <div class="command"><code>{`reet agent restore --target ${quote(entry.job.targetId)} --backup ${quote(backup)}`}</code><CopyButton text={`reet agent restore --target ${quote(entry.job.targetId)} --backup ${quote(backup)}`} label="Copy restore command" /></div>
          {/each}
        </details>
      {/if}
    </div>
  </details>
{/snippet}

{#if activity.total || location.hash === '#sync-jobs'}
<section id="sync-jobs" class="activity" aria-labelledby="sync-jobs-title">
  <header class="activity-head">
    <div>
      <p class="eyebrow">Sync activity</p>
      <h3 id="sync-jobs-title">{activityHeadline}</h3>
      <p>{activityDetail}</p>
    </div>
    {#if activity.total}<span class="badge">{activity.total} {activity.total === 1 ? 'job' : 'jobs'}</span>{/if}
  </header>
  {#if activeHosts.length}
    <div class="activity-hosts">
      {#each activeHosts as host (host.host)}
        <section class="activity-host" aria-label={`Sync activity on ${host.host}`}>
          <header class="host-head"><h4>{host.host}</h4><p>{hostSummary(host)}</p></header>
          {#each host.entries.filter(entry => !entry.resolved) as entry (entry.key)}
            {@render jobRow(entry, entry.active || entry.key === firstActionKey || !!retryResults[entry.key])}
          {/each}
        </section>
      {/each}
    </div>
  {:else if latestFinished}
    <section class="activity-host calm" aria-label={`Latest sync activity on ${latestFinished.host}`}>
      <header class="host-head"><h4>{latestFinished.host}</h4><p>Nothing needs you.</p></header>
      {@render jobRow(latestFinished, false)}
    </section>
  {/if}
  {#if history.length}
    <details class="activity-history">
      <summary>Earlier activity ({history.length})</summary>
      <div class="history-list">
        {#each history as entry (entry.key)}
          {@render jobRow(entry, false)}
        {/each}
      </div>
    </details>
  {/if}
</section>
{/if}

{#if view === 'compare'}
<FleetActions {snapshot} {canWrite} {onupdate} />
<section id="sync-selection" class="control" aria-labelledby="selection-title">
  <h3 id="selection-title">Choose skills. Choose where they go.</h3>
  <p>Choose skills from reet.sh, a list, or another machine. Compare, save the desired set, then preview and approve changes.</p>
  <div class="pickers">
    <label>Compare from<select aria-label="Compare from" bind:value={source} onchange={loadSource} disabled={busy}><option value="shelf">Reet.sh shelf &amp; lists</option><option value="hosted">My published reet.sh skills</option>{#each fleet as m (m.host)}<optgroup label={m.host}>{#each agentGroups(m) as g (g.id)}<option value={g.id}>{groupLabel(g)}</option>{/each}</optgroup>{/each}</select></label>
    {#if source === 'shelf'}<label>Skill collection<select aria-label="Skill collection" bind:value={listId} onchange={loadSource} disabled={busy}><option value={0}>My shelf</option>{#each lists as list}<option value={list.id}>{list.name}</option>{/each}</select></label>{/if}
    <label>Destination<select aria-label="Destination" bind:value={selectedMachine} onchange={() => selectMachine(selectedMachine)} disabled={busy}><option value="">Choose a machine</option><option value="*">All machines</option>{#each fleet as m (m.host)}<option value={m.host}>{m.host}</option>{/each}</select></label>
    <label>Sync mode<select aria-label="Sync mode" bind:value={mode} disabled={!canWrite || busy}><option value="additive">Additive</option><option value="bidirectional">Bidirectional review</option><option value="mirror">Exact set</option></select></label>
  </div>
  {#if machine || selectedMachine === '*'}
    <div class="agent-scopes">
      <div class="scope-head"><strong>Agent scopes · {selectedMachine === '*' ? 'all machines' : machine?.host}</strong><div><button type="button" class="link" onclick={() => (selectedAgents = selectableGroups.map(g => g.id))}>All</button><button type="button" class="link" onclick={() => (selectedAgents = [])}>None</button></div></div>
      <div class="scope-grid">
        {#each selectableGroups as env (env.id)}
          <div>
          <label class="scope-option" class:selected={selectedAgents.includes(env.id)}>
            <input type="checkbox" checked={selectedAgents.includes(env.id)} onchange={() => toggleAgent(env.id)} disabled={busy} />
            <span><strong>{selectedMachine === '*' ? `${env.host} · ` : ''}{groupLabel(env)}</strong><small>{env.skillCount ?? 0} skills · {!env.complete ? 'Incomplete' : env.scannedAt <= Date.now() - 14 * 86400000 ? 'Out of date' : 'Recent complete'} scan</small></span>
          </label>
          <ContentObservation content={env.content} />
          </div>
        {/each}
      </div>
      {#if scanOnlyGroups.length}<p class="hint">Scan-only: {scanOnlyGroups.map(g => groupLabel(g)).join(', ')}. These scopes cannot receive sync jobs.</p>{/if}
      <p class="hint">Every selected scope receives the same desired set. Each keeps its own install target and review. All machines selects reported inventory scopes, including machines that are currently offline.</p>
    </div>
  {/if}
  <p class="hint">{mode === 'additive' ? 'Install selected skills and keep local extras.' : mode === 'bidirectional' ? 'Review changes from both sides before applying. Removals need approval.' : 'Match this exact selection. Extra skills require reviewed removal.'}</p>
  <p class="hint">Bookmarks and “In use” are personal shelf choices. They never approve installation or removal. Matching here means skill identity/source, not file contents or versions.</p>
  <label>Find skills<input type="search" bind:value={query} placeholder="Name or source" /></label>
  <div class="actions"><button class="btn quiet" disabled={loading || busy || !sourceLoaded} onclick={() => selected = [...new Set([...selected, ...filtered.map(s => preferred(s.id))])]}>Select visible</button><button class="btn quiet" disabled={busy} onclick={() => selected = []}>Clear selection</button><span>{selected.length} selected</span></div>
  <div class="skill-options" aria-label="Skills to compare">
    {#if loading}<p role="status">Loading skills…</p>{:else}{#each filtered as skill (skill.id)}<label class="skill"><input type="checkbox" checked={selected.includes(skill.id)} onchange={() => toggle(skill.id)} disabled={busy} /><span><strong>{skill.name}</strong><small>{skill.source || 'Unknown source — link before syncing'}</small></span></label>{:else}<p>No skills here yet. Add skills to your shelf or report a machine inventory.</p>{/each}{/if}
  </div>
  {#if targetEnvs.length}
    <p class:bad={!ready} role="status">{ready ? `Selected scopes last reported ${ago(oldestScan)}.` : 'One or more selected scopes need a fresh complete scan. Rescan before saving or applying.'}</p>
    {#each targetEnvs.filter(e => e.error) as env (env.id)}<p class="bad">{groupLabel(env)}: {env.error}</p>{/each}
    <div class="counts"><span>{targetEnvs.length} agent {targetEnvs.length === 1 ? 'scope' : 'scopes'} selected</span><span>{onlySource.length} only {['shelf','hosted'].includes(source) ? 'on reet.sh (selected)' : 'in reference selection'}</span><span>{onlyMachine.length} only on selected scopes</span><span>{matching.length} found in at least one selected scope</span><span>{sourceConflicts.length} source conflicts</span><span>{localDuplicates.length} local duplicates</span><span>{unknown.length} unknown</span></div>
    <details><summary>Differences by destination</summary><ul>{#each scopeDifferences as difference (difference.env.id)}<li><strong>{difference.env.host} · {groupLabel(difference.env)}</strong>: {difference.missing} to install · {difference.extra} extra {mode === 'additive' ? '(kept)' : '(review removals)'}</li>{/each}</ul><p class="hint">The counts above combine unique skills across selected scopes. Each scope has its own changes below.</p></details>
    {#if localDuplicates.length}<p class="hint">Local copies of {localDuplicates.map(s => s.name).join(', ')} will be backed up and replaced automatically with the selected source.</p>{/if}
    {#if sourceConflicts.length}<p class="bad">Source conflicts: {sourceConflicts.map(s => s.name).join(', ')}. Resolve the installed source before applying.</p>{/if}
    {#if conflicts.length}<p class="bad" role="alert">Concurrent changes: {conflicts.join(', ')}. Select the source you want for each conflict, or keep it absent, then <button class="btn quiet" onclick={() => conflicts = []}>Confirm conflict choices</button>.</p>{/if}
    <details><summary>Only on selected scopes ({onlyMachine.length})</summary><p>Select skills to add to {listId && source === 'shelf' ? 'the selected reet.sh list' : 'your reet.sh shelf'}.</p>{#each onlyMachine as skill}<label class="skill"><input type="checkbox" value={skill.id} bind:group={adopt} disabled={!canWrite || busy} /><span>{skill.name}<small>{skill.source || 'Unknown source'}</small></span></label>{/each}{#if canWrite}<button class="btn" disabled={busy || loading || !sourceLoaded || !ready || !adopt.length || (source === 'shelf' && listId > 0 && !listEditable)} onclick={adoptSelected}>Add selected to reet.sh</button>{/if}</details>
    {#if canWrite}<button class="btn primary" disabled={busy || loading || !sourceLoaded || !ready || unknown.length > 0 || sourceConflicts.length > 0 || conflicts.length > 0} onclick={reviewSave}>{busy ? 'Saving…' : `Save to ${targetEnvs.length} agent ${targetEnvs.length === 1 ? 'scope' : 'scopes'}`}</button>{/if}
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
        {#if agent}<div class="actions"><button class="btn primary" type="button" disabled={busy || agent.paused} onclick={() => syncGroup(group)}>{busy ? 'Starting…' : 'Sync all now'}</button><button class="btn" type="button" disabled={busy || agent.paused} onclick={() => previewGroup(group)}>Preview all on machine</button></div>{:else}<span class="hint">Local preview commands below</span>{/if}
      </header>
      <div class="scope-sets">
        {#each group.items as item (item.target.id)}
          {@const env = groups.find(g => g.id === item.target.id)}
          <div class="scope-set">
            <div class="scope-title"><strong>{groupLabel(item.target)}</strong><span class="badge">{item.mode || 'mirror'} · {item.desired.length} desired</span></div>
            <p>{item.sourceName}</p>
            <p class:bad={!env?.complete || (env?.scannedAt ?? 0) < Date.now() - 14 * 86400000}>{env?.complete && env.scannedAt > Date.now() - 14 * 86400000 ? `${item.missing.length} to install · ${item.extra.length} extra${item.mode === 'additive' ? ' (kept)' : ' (review removals)'}` : 'Current differences unconfirmed — rescan required'}</p>
            <p class="hint">Last report: {env?.scannedAt ? ago(env.scannedAt) : 'none'} · {item.appliedRevision === item.revision ? 'Desired revision acknowledged' : 'Desired revision not yet acknowledged'}</p>
            {#if canWrite}<div class="actions"><button class="btn" disabled={busy} onclick={() => edit(item)}>Edit selection</button><button class="btn" disabled={busy || !env?.complete} onclick={() => reconcile(item)}>Review local &amp; web changes</button>{#if agent}<button class="btn primary" disabled={busy || agent.paused} onclick={() => run(async () => { onupdate(await startSyncRun(agent.id, [item.target.id])); notice = 'Automatic sync started. Additive plans apply on their own; removals wait for review.' })}>Sync now</button><button class="btn" disabled={busy || agent.paused} onclick={() => run(async () => { onupdate(await queueSyncJob(agent.id, item.target.id, 'preview')); notice = 'Preview queued. The machine will report its exact plan.' })}>Preview</button>{/if}<button class="btn quiet" disabled={busy} onclick={() => confirm = { title: 'Stop tracking this set?', body: 'This removes the desired set. Installed skills, shelf entries and notes remain.', action: () => run(async () => onupdate(await deleteSyncTarget(item.target.id))) }}>Stop tracking</button></div>{/if}
            {#if !agent}<details class="local-preview"><summary>Local preview command</summary><div class="command"><code>{cli(item)}</code><CopyButton text={cli(item)} label="Copy local preview" /></div></details>{/if}
          </div>
        {/each}
      </div>
    </article>
  {:else}<p class="hint">No desired skill sets yet. Choose a machine and at least one agent scope above to get started.</p>{/each}
</section>
{/if}{#if confirm}<ConfirmDialog title={confirm.title} body={confirm.body} confirmLabel="Confirm" oncancel={() => confirm = null} onconfirm={() => { const action = confirm?.action; confirm = null; void action?.() }} />{/if}
<style>
 .control { border:1px solid var(--line); border-radius:10px; padding:18px; margin:16px 0; min-width:0; }
 h3,h4 { margin:0 0 10px; } p,.hint { color:var(--muted); line-height:1.6; font-size:.85rem; } .hint {font-size:.8rem}
 .pickers {display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px} label{display:grid;gap:6px;font-size:.85rem;font-weight:600} select,input[type=search]{width:100%;min-width:0}
 .skill-options{max-height:320px;overflow:auto;border:1px solid var(--line);border-radius:8px;margin:12px 0;padding:8px}
 .skill{display:flex;align-items:center;gap:10px;padding:10px;font-weight:400;border-bottom:1px solid var(--line)} .skill input{flex-shrink:0} small{display:block;font-size:.75rem;color:var(--muted);overflow-wrap:anywhere}
 .actions,.counts,.command{display:flex;gap:10px;flex-wrap:wrap;align-items:center;margin:14px 0}.counts span{border:1px solid var(--line);border-radius:16px;padding:5px 10px;font-size:.8rem}
 .feedback{padding:12px;border:1px solid var(--good);border-radius:8px}.bad{color:var(--bad);border-color:var(--bad)}details{margin:16px 0}summary{cursor:pointer;font-weight:600}code,li,h4{overflow-wrap:anywhere}section{margin:20px 0} .command code{font-size:.78rem}
 .retry-result{margin:8px 0 0;padding:8px 10px;border:1px solid var(--line);border-radius:8px;font-size:.8rem;line-height:1.5;overflow-wrap:anywhere}
 .retry-result.bad{border-color:var(--bad);color:var(--bad)}
 .agent-scopes{margin:16px 0;padding:14px;border:1px solid var(--line);border-radius:10px;background:var(--bg)}
 .scope-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;font-size:.85rem}
 .scope-head > div{display:flex;gap:10px}
 .link{padding:0;border:0;background:none;color:var(--accent-text);font:inherit;font-size:.76rem;font-weight:600;text-decoration:underline;text-underline-offset:3px;cursor:pointer}
 .scope-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:8px;margin:10px 0 8px}
 .scope-option{display:flex;align-items:flex-start;gap:8px;padding:9px 10px;border:1px solid var(--line);border-radius:8px;background:var(--card);font-weight:400;cursor:pointer}
 .scope-option.selected{border-color:color-mix(in srgb,var(--accent) 45%,var(--line));background:var(--tint)}
 .scope-option input{flex:none;margin-top:2px}
 .scope-option span{display:grid;gap:2px;min-width:0}
 .scope-option strong{font-size:.8rem;overflow-wrap:anywhere}
 .scope-option small{font-size:.71rem}
 /* No clip: a card holding actions must never cut them off. The header rounds its own top corners instead. */
 .machine-set{padding:0}
 .set-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;padding:16px 18px;border-bottom:1px solid var(--line);border-radius:9px 9px 0 0;background:var(--bg)}
 .set-head > div{min-width:0}
 .set-head h4{margin:0}
 .set-head p{margin:3px 0 0;font-size:.78rem}
 .scope-sets{display:grid;grid-template-columns:minmax(0,1fr)}
 .scope-set{padding:14px 18px;border-top:1px solid var(--line)}
 .scope-set:first-child{border-top:0}
 .scope-title{display:flex;align-items:center;justify-content:space-between;gap:10px;flex-wrap:wrap}
 .scope-title strong{min-width:0;font-size:.85rem;overflow-wrap:anywhere}
 .badge{padding:3px 8px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:.7rem}
 .scope-set > p{margin:5px 0;overflow-wrap:anywhere}
 .local-preview{margin:8px 0 0}
 .local-preview summary{font-size:.78rem}
 .activity{margin:24px 0;padding:18px;border:1px solid var(--line);border-radius:14px;background:var(--card);scroll-margin-top:16px}
 .activity-head{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:14px}
 .activity-head > div{min-width:0;max-width:70ch}
 .eyebrow{margin:0 0 4px;color:var(--accent-text);text-transform:uppercase;letter-spacing:.1em;font-size:.68rem;font-weight:650}
 .activity-head h3{margin:0;font:500 1.25rem/1.25 var(--serif);letter-spacing:-.02em}
 .activity-head p{margin:5px 0 0;font-size:.8rem;line-height:1.55}
 .activity-head .badge{flex:none}
 .activity-hosts{display:grid;gap:12px}
 .activity-host{border:1px solid var(--line);border-radius:12px;background:var(--bg);min-width:0}
 .activity-host.calm{border-style:dashed}
 .host-head{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:11px 14px;border-bottom:1px solid var(--line);background:var(--card);border-radius:11px 11px 0 0;flex-wrap:wrap}
 .activity-host.calm .host-head{border-bottom:0;border-radius:11px}
 .host-head h4{margin:0;font-size:.86rem;overflow-wrap:anywhere}
 .host-head p{margin:0;color:var(--muted);font-size:.72rem}
 .activity-job{margin:0;border-top:1px solid var(--line);min-width:0}
 .activity-job:first-of-type{border-top:0}
 .activity-job > summary{display:grid;grid-template-columns:24px minmax(0,1fr) auto;align-items:center;gap:10px;min-height:48px;padding:10px 14px;list-style:none;font-weight:400}
 .activity-job > summary::-webkit-details-marker{display:none}
 .activity-job > summary::marker{content:''}
 .activity-job[open] > summary{background:color-mix(in srgb,var(--bg) 55%,transparent)}
 .job-mark{display:grid;place-items:center;width:24px;height:24px;flex:none;border:1px solid var(--line);border-radius:50%;font-size:.72rem;font-weight:700}
 .job-mark.bad{color:var(--bad);border-color:color-mix(in srgb,var(--bad) 45%,var(--line))}
 .job-mark.warn{color:var(--warn);border-color:color-mix(in srgb,var(--warn) 45%,var(--line))}
 .job-mark.work{color:var(--accent-text);border-color:color-mix(in srgb,var(--accent) 45%,var(--line))}
 .job-mark.ok{color:var(--good);border-color:color-mix(in srgb,var(--good) 45%,var(--line))}
 .job-mark.quiet{color:var(--muted)}
 .pulse{width:8px;height:8px;border-radius:50%;background:currentColor;display:block}
 .is-active .pulse{animation:pulse 1.4s ease-in-out infinite}
 @keyframes pulse{50%{opacity:.35}}
 .job-copy{display:grid;gap:1px;min-width:0}
 .job-copy strong{font-size:.84rem;line-height:1.35;overflow-wrap:anywhere}
 .job-copy small{font-size:.71rem;color:var(--muted);line-height:1.4;overflow-wrap:anywhere}
 .job-end{display:flex;align-items:center;gap:6px;justify-self:end;flex:none}
 .job-chevron{width:16px;height:16px;flex:none;color:var(--muted);transition:transform .2s var(--ease-out)}
 .activity-job[open] .job-chevron{transform:rotate(180deg)}
 .job-state{flex:none;padding:3px 9px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:.66rem;font-weight:650;white-space:nowrap}
 .job-state.bad{color:var(--bad);border-color:color-mix(in srgb,var(--bad) 40%,var(--line))}
 .job-state.warn{color:var(--warn);border-color:color-mix(in srgb,var(--warn) 40%,var(--line))}
 .job-state.work{color:var(--accent-text);border-color:color-mix(in srgb,var(--accent) 40%,var(--line))}
 .job-state.ok{color:var(--good);border-color:color-mix(in srgb,var(--good) 40%,var(--line))}
 .job-body{display:grid;gap:10px;padding:0 14px 14px;min-width:0}
 .stages{list-style:none;display:flex;flex-wrap:wrap;gap:6px;margin:0;padding:0}
 .stages li{padding:3px 8px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:.67rem}
 .stages li.active{color:var(--accent-text);border-color:color-mix(in srgb,var(--accent) 40%,var(--line));font-weight:600}
 .stages li.done{color:var(--good);border-color:color-mix(in srgb,var(--good) 35%,var(--line))}
 .stages li.failed{color:var(--bad);border-color:color-mix(in srgb,var(--bad) 40%,var(--line))}
 .stages li.review{color:var(--warn);border-color:color-mix(in srgb,var(--warn) 40%,var(--line))}
 .job-facts,.job-message{margin:0;color:var(--muted);font-size:.76rem;line-height:1.55;overflow-wrap:anywhere}
 .job-facts{color:var(--ink-soft)}
 .plan{margin:0;border-top:1px solid var(--line);padding-top:8px}
 .plan > summary{font-size:.75rem}
 .plan p,.plan ul{font-size:.76rem}
 .plan ul{padding-left:18px}
 .plan li{overflow-wrap:anywhere}
 .plan-dir code{font-size:.72rem}
 .activity-history{margin-top:12px;border-top:1px solid var(--line);padding-top:10px}
 .activity-history > summary{font-size:.78rem}
 .history-list{display:grid;gap:0;margin-top:10px;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:var(--bg)}
 .history-list .activity-job{border-top:1px solid var(--line)}
 .history-list .activity-job:first-child{border-top:0}
 @media (prefers-reduced-motion: reduce){.is-active .pulse{animation:none}.job-chevron{transition:none}}
 @media(max-width:600px){
  .pickers{grid-template-columns:1fr}.control{padding:12px}.actions .btn{flex:1 1 auto}.scope-grid{grid-template-columns:1fr}.set-head{align-items:flex-start}.set-head .btn{width:100%}
  .activity{padding:12px;border-radius:12px}.activity-head h3{font-size:1.12rem}
  .activity-job > summary{grid-template-columns:24px minmax(0,1fr);padding:10px 12px}
  .job-end{grid-column:2;justify-self:start;margin-top:3px}
  .host-head{padding:10px 12px}
  .job-body{padding:0 12px 12px}
 }
</style>
