<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, syncCandidates, saveDesired, startSyncRun, controlSyncAgent, queueSyncJob, type SyncState, type SyncSkill } from './api'
  import { agentOnline, groupLabel, inventoryMachines } from './machines'
  import { fallbackInstallTarget, isFresh, syncableAgent, machineSyncOverview } from './syncOverview'

  let { snapshot, canWrite, onupdate }: { snapshot: SyncState; canWrite: boolean; onupdate: (state: SyncState) => void } = $props()
  let candidates = $state<SyncSkill[]>([]), chosen = $state<string[]>([]), hosts = $state<string[]>([])
  let locations = $state<Record<string, string[]>>({})
  let all = $state(true), query = $state(''), busy = $state(false), loading = $state(true)
  let error = $state(''), result = $state(''), operation = $state(''), toggling = $state('')
  const connectHref = '/machines#add-machine'
  const fleet = $derived(inventoryMachines(snapshot.machines ?? []))
  const selectableHosts = $derived(fleet.filter(machine => snapshot.agents?.some(agent => agent.host.toLowerCase() === machine.host.toLowerCase())).map(machine => machine.host))
  const linked = $derived(candidates.filter(skill => skill.source && skill.sourceType !== 'local'))
  const selected = $derived(all ? linked.map(skill => skill.id) : chosen)
  const filtered = $derived(candidates.filter(skill => `${skill.name} ${skill.source}`.toLowerCase().includes(query.toLowerCase())))
  const chosenMachines = $derived(fleet.filter(machine => hosts.includes(machine.host)))
  const blockedSources = $derived(all && candidates.length !== linked.length)

  function scopes(host: string): string[] {
    if (locations[host]) return locations[host]
    const machine = fleet.find(item => item.host === host)
    if (!machine) return []
    const saved = snapshot.targets.filter(target => target.target.host.toLowerCase() === host.toLowerCase() && syncableAgent(target.target.agent)).map(target => target.target.id)
    if (saved.length) return saved
    const preferred = fallbackInstallTarget(machine, snapshot)
    return preferred ? [preferred.targetId] : []
  }
  function toggleHost(host: string) { hosts = hosts.includes(host) ? hosts.filter(item => item !== host) : [...hosts, host] }
  function toggleScope(host: string, id: string) {
    const ids = scopes(host)
    locations = { ...locations, [host]: ids.includes(id) ? ids.filter(item => item !== id) : [...ids, id] }
  }
  async function machineKeep(id: string, enabled: boolean) {
    if (busy || toggling) return
    toggling = id; error = ''
    try { onupdate(await controlSyncAgent(id, enabled ? 'keep-sync-on' : 'keep-sync-off')) }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { toggling = '' }
  }
  async function rescan(id: string) {
    if (busy) return
    busy = true; error = ''
    try { onupdate(await queueSyncJob(id, '', 'scan')); result = 'Scan requested. This page updates as the machine reports back.' }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
  async function sync() {
    if (busy || !canWrite) return
    busy = true; error = ''; result = ''
    const successes: string[] = []
    const failures: string[] = []
    try {
      let state = await getSyncState()
      for (const machine of chosenMachines) {
        const agent = state.agents?.find(item => item.host.toLowerCase() === machine.host.toLowerCase())
        const ids = scopes(machine.host)
        try {
          if (!agent) throw new Error('Connect the reet agent first.')
          if (agent.paused) throw new Error('Resume this machine first.')
          if (!ids.length) throw new Error('Choose an install location, or request a fresh scan.')
          for (const id of ids) {
            const env = state.environments.find(item => item.id === id)
            if (!isFresh(env)) throw new Error('A fresh complete scan is needed. Use Scan machine below.')
            operation = `Saving skills for ${machine.host}…`
            state = await saveDesired({ targetId: id, sourceId: 'shelf', listId: 0, skillIds: selected, mode: 'additive', revision: state.targets.find(target => target.target.id === id)?.revision ?? '', followSource: all })
            onupdate(state)
          }
          operation = `Starting sync on ${machine.host}…`
          state = await startSyncRun(agent.id, ids)
          onupdate(state)
          successes.push(machine.host)
        } catch (e) { failures.push(`${machine.host}: ${e instanceof Error ? e.message : String(e)}`) }
      }
      if (successes.length) result = `Sync queued for ${successes.join(', ')}. Follow live progress above. Machines with Keep synced on continue after you close this page.`
      if (failures.length) error = failures.join(' ')
    } catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false; operation = '' }
  }
  onMount(() => {
    const params = new URLSearchParams(location.search)
    const requested = params.get('machine')
    hosts = requested === '*' ? selectableHosts : fleet.filter(machine => requested ? machine.host === requested : fleet.length === 1).map(machine => machine.host)
    const requestedSkills = params.getAll('skill')
    if (requestedSkills.length) { all = false; chosen = requestedSkills }
    void syncCandidates().then(response => { candidates = response.skills; chosen = chosen.filter(id => candidates.some(skill => skill.id === id && skill.source && skill.sourceType !== 'local')) }).catch(e => { error = e instanceof Error ? e.message : String(e) }).finally(() => { loading = false })
  })
</script>

<section class="setup" aria-labelledby="simple-sync-title" aria-busy={busy}>
  <header><p class="eyebrow">Your skills, on your machines</p><h2 id="simple-sync-title">Choose once. Keep in sync.</h2><p>Pick the skills you want and where you want them. Reet handles installation on each machine.</p></header>
  <fieldset disabled={busy || !canWrite}>
    <legend><span class="step">1</span> What do you want to sync?</legend>
    <div class="choices">
      <label class:chosen={all}><input type="radio" name="skill-choice" checked={all} onchange={() => all = true} /><span><strong>All my skills</strong><small>Your whole shelf. With Keep synced on, new skills follow automatically.</small></span></label>
      <label class:chosen={!all}><input type="radio" name="skill-choice" checked={!all} onchange={() => all = false} /><span><strong>Choose individual skills</strong><small>Only the skills you select.</small></span></label>
    </div>
    {#if loading}<p role="status">Loading your skills…</p>
    {:else if !candidates.length}<p>Add your first skill on <a href="/skills">Skills</a>, then come back to sync it.</p>
    {:else if !all}
      <label class="search-label">Find a skill<input type="search" bind:value={query} placeholder="Search your skills" /></label>
      <div class="skills" aria-label="Choose skills">
        {#each filtered as skill (skill.id)}
          {@const installable = skill.source && skill.sourceType !== 'local'}
          <label class="skill"><input type="checkbox" value={skill.id} bind:group={chosen} disabled={!installable} /><span><strong>{skill.name}</strong><small>{installable ? skill.source : 'Link a source on Skills to sync this skill'}</small></span>{#if !installable}<a href={`/skills?search=${encodeURIComponent(skill.name)}`}>Link source</a>{/if}</label>
        {:else}<p>No skills match this search.</p>{/each}
      </div>
    {/if}
    {#if blockedSources}<p class="warning">{candidates.length - linked.length} skills need a source before they can be synced. <a href="/skills">Link their sources</a>, or choose individual skills.</p>{/if}
  </fieldset>
  <fieldset disabled={busy || !canWrite}>
    <legend><span class="step">2</span> Which machines?</legend>
    <div class="machine-tools"><button type="button" class="btn quiet" onclick={() => hosts = [...selectableHosts]}>Select all machines</button><a href={connectHref}>Connect a machine</a></div>
    <div class="machines">
      {#each fleet as machine (machine.host)}
        {@const agent = snapshot.agents?.find(item => item.host.toLowerCase() === machine.host.toLowerCase())}
        {@const overview = machineSyncOverview(machine, snapshot)}
        <article class:selected={hosts.includes(machine.host)}>
          <label class="machine-choice"><input type="checkbox" checked={hosts.includes(machine.host)} onchange={() => toggleHost(machine.host)} disabled={!agent} /><span><strong>{machine.host}</strong><small>{overview.label} · {agentOnline(agent) ? 'Online' : 'Offline'}</small></span></label>
          {#if agent}
            <label class="keep-machine"><span>Keep synced</span><input type="checkbox" role="switch" aria-label={`Keep ${machine.host} synced`} checked={!!agent.keepSynced} disabled={!!toggling} onchange={event => void machineKeep(agent.id, event.currentTarget.checked)} /></label>
            <p class="machine-note">{agent.keepSynced ? snapshot.targets.some(target => target.target.host === machine.host) ? 'Saved skills stay synced, even when this page is closed.' : 'Keep synced is on. Choose skills and sync to finish setup.' : 'Automatic sync is off.'}</p>
            {#if machine.groups.some(group => syncableAgent(group.agent) && !isFresh(group))}<button type="button" class="btn quiet" disabled={!agentOnline(agent)} onclick={() => void rescan(agent.id)}>Fix it · refresh scan</button>{/if}
            {#each snapshot.targets.filter(target => target.target.host === machine.host && target.problem) as target (target.target.id)}<p class="warning">{target.problem} <a href={target.problem?.includes('scan') ? `/machines?machine=${encodeURIComponent(machine.host)}` : '/skills'}>Fix it</a></p>{/each}
            {#if agent.paused}<button type="button" class="btn quiet" onclick={async () => { try { onupdate(await controlSyncAgent(agent.id, 'resume')) } catch(e) { error = String(e) } }}>Resume machine</button>{/if}
            {#if !agentOnline(agent)}<p class="machine-note">Sync waits until reet reconnects. <a href={`/machines?machine=${encodeURIComponent(machine.host)}`}>Connection help</a></p>{/if}
            {#if hosts.includes(machine.host)}
              <details><summary>Install locations · {scopes(machine.host).length} selected</summary><p>Reet uses the shared skills folder where possible. Change locations here if needed.</p>
                {#each machine.groups.filter(group => syncableAgent(group.agent)) as group (group.id)}<label class="location"><input type="checkbox" checked={scopes(machine.host).includes(group.id)} onchange={() => toggleScope(machine.host, group.id)} /><span>{groupLabel(group)}<small>{isFresh(group) ? 'Ready' : 'Needs a fresh scan'}</small></span></label>{/each}
                <button type="button" class="btn quiet" onclick={() => void rescan(agent.id)}>Scan machine</button>
              </details>
            {/if}
          {:else}<a href={`/machines?machine=${encodeURIComponent(machine.host)}`}>Connect reet to enable sync</a>{/if}
        </article>
      {:else}<p>No machines connected yet. <a href={connectHref}>Connect your first machine</a>.</p>{/each}
    </div>
  </fieldset>
  <div class="finish">
    <p>Keep synced runs automatically on the machines where you turn it on. Other installed skills are kept.</p>
    <div class="submit"><p>{selected.length} skills · {hosts.length} {hosts.length === 1 ? 'machine' : 'machines'}<small>Reet checks each change before applying it.</small></p><button type="button" class="btn primary" disabled={busy || loading || !canWrite || !selected.length || !hosts.length || blockedSources || !!toggling} onclick={sync}>{busy ? operation || 'Starting…' : 'Sync now'}</button></div>
  </div>
  {#if result}<p class="result" role="status">{result}</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
</section>

<style>
  .setup{margin:16px 0 28px;min-width:0}header{margin-bottom:28px}h2{font:500 clamp(1.6rem,3vw,2.2rem)/1.15 var(--serif);letter-spacing:-.03em;margin:6px 0 12px}p{color:var(--muted);font-size:.86rem;line-height:1.6}.eyebrow{font-size:.72rem;letter-spacing:.1em;text-transform:uppercase;color:var(--accent-text)}fieldset{border:0;border-top:1px solid var(--line);padding:22px 0;margin:0;min-width:0}legend{font-size:1rem;font-weight:650;padding:0 12px 0 0}.step{display:inline-grid;place-items:center;width:26px;height:26px;border:1px solid var(--line);border-radius:50%;margin-right:8px;color:var(--accent-text);font-size:.8rem}.choices,.machines{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.choices>label{display:flex;gap:12px;padding:18px;border:1px solid var(--line);border-radius:12px;cursor:pointer;background:var(--bg)}.choices>label.chosen,article.selected{border-color:var(--accent-text);background:color-mix(in srgb,var(--accent-text) 5%,var(--card))}strong{font-size:.9rem}small{display:block;color:var(--muted);font-size:.78rem;line-height:1.5;margin-top:4px}.search-label{display:grid;gap:6px;font-size:.8rem;margin-top:18px}.skills{max-height:280px;overflow:auto;margin-top:10px;border:1px solid var(--line);border-radius:10px}.skill{display:flex;align-items:center;gap:12px;padding:12px;border-bottom:1px solid var(--line)}.skill>span{flex:1;min-width:0}.skill a{font-size:.78rem}.machine-tools{display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;font-size:.8rem;gap:8px}.machines article{border:1px solid var(--line);border-radius:12px;padding:16px;min-width:0}.machine-choice{display:flex;gap:12px;align-items:center}.machine-choice strong{overflow-wrap:anywhere}.keep-machine{display:flex;align-items:center;justify-content:space-between;gap:12px;padding-top:16px;margin-top:14px;border-top:1px solid var(--line);font-size:.83rem;font-weight:600}.keep-machine input{appearance:none;position:relative;width:36px;height:22px;border:1px solid var(--line);border-radius:20px;background:var(--line);cursor:pointer}.keep-machine input:checked{background:var(--accent-text);border-color:var(--accent-text)}.keep-machine input:after{content:"";position:absolute;top:3px;left:3px;width:14px;height:14px;border-radius:50%;background:var(--card)}.keep-machine input:checked:after{left:17px}.machine-note{font-size:.75rem;margin:7px 0 0}details{margin-top:14px;font-size:.8rem}summary{cursor:pointer;padding:8px 0;min-height:30px}details p{font-size:.78rem}.location{display:flex;align-items:center;gap:10px;padding:8px}.finish{padding:18px;border:1px solid var(--line);border-radius:12px;background:var(--bg)}.submit{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-top:12px;border-top:1px solid var(--line);padding-top:14px}.submit p{margin:0}.warning,.error,.result{padding:12px;border:1px solid var(--warn);border-radius:8px;font-size:.83rem}.error{border-color:var(--bad);color:var(--bad)}.result{border-color:var(--good)}input[type=checkbox],input[type=radio]{flex-shrink:0;accent-color:var(--accent-text)}a{color:var(--accent-text)}@media(max-width:600px){.choices,.machines{grid-template-columns:1fr}.submit{align-items:stretch;flex-direction:column}.submit .btn{width:100%}.choices>label{padding:14px}}
</style>
