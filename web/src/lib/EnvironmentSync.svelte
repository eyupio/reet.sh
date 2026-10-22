<script lang="ts">
  import { onMount, tick } from 'svelte'
  import type { Snippet } from 'svelte'
  import { getSyncState, mergeMachines, forgetMachine, forgetEnvironment, type SyncState, type SyncEnvironment } from './api'
  import CopyButton from './CopyButton.svelte'
  import SyncControl from './SyncControl.svelte'
  import { groupLabel, agentGroups, inventoryMachines } from './machines'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite, token = '', tokenSetup, view = 'machines', startConnect = false }: { canWrite: boolean; token?: string; tokenSetup?: Snippet; view?: 'machines' | 'compare'; startConnect?: boolean } = $props()
  let fleetSince = $state(0)
  function refreshedGroups(machine: import('./api').Machine): number { return fleetSince ? agentGroups(machine).filter(group => group.complete && group.scannedAt >= fleetSince).length : 0 }
  let maintenanceFrom = $state('')
  let maintenanceTo = $state('')
  let machineChange = $state<{ from: string; to?: string } | null>(null)
  function refreshCommand(host: string): string { return `reet scan --server ${quote(site)} --env ${quote(host)} --prune --reconcile` }
  function previewMachine(e: SubmitEvent) {
    e.preventDefault()
    if (busy || !maintenanceFrom || !maintenanceTo.trim()) return
    machineChange = { from: maintenanceFrom, to: maintenanceTo.trim() }
  }
  async function changeMachine() {
    if (!machineChange || busy) return
    const change=machineChange; machineChange=null; busy=true; error=''; notice=''
    try {
      syncState=change.to ? await mergeMachines(change.from,change.to) : await forgetMachine(change.from)
      notice=change.to ? `Reet, reports now belong to ${change.to}. Old names remain aliases; future scans update this machine.` : `Forgot ${change.from}. Installed files and shelf entries are unchanged; a future scan can report it again.`
      maintenanceFrom='';maintenanceTo=''
      await tick(); document.getElementById('machine-list-title')?.focus()
    } catch(e) { error=e instanceof Error ? e.message : String(e) } finally { busy=false }
  }
  let syncState = $state<SyncState>({ environments: [], targets: [] })
  const reportedMachines = $derived(syncState.machines ?? [])
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let host = $state('')
  let connectionSince = $state(Date.now())
  let lastRefresh = $state(0)
  let channel = $state('stable')
  let emptyAgent = $state('agents')
  let allowApply = $state(false)
  let forgetting = $state<SyncEnvironment | null>(null)
  let now = $state(Date.now())
  // svelte-ignore state_referenced_locally
  let connectOpen = $state(startConnect)
  let loadedOnce = false
  let refreshing = false
  let loadGeneration = 0
  const machines = $derived(inventoryMachines(syncState.machines ?? []))
  const serverMachines = $derived((syncState.machines ?? []).filter(machine => agentGroups(machine).length === 0))
  const connectedMachine = $derived(machines.find(machine => machine.host === host.trim() && agentGroups(machine).some(group => group.complete && fresh(group) && group.scannedAt >= connectionSince)))
  const site = location.origin
  function quote(value: string): string { return "'" + value.replaceAll("'", "'\\''") + "'" }
  const install = $derived(`curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh${channel === 'dev' ? ' -s -- --version dev' : ''}\nexport PATH="$HOME/.local/bin:$PATH"\nreet --version`)
  const credentials = $derived(`export REET_URL=${quote(site)}\nexport REET_TOKEN=${quote(token || 'PASTE_YOUR_PERSONAL_TOKEN_HERE')}`)
  const scan = $derived(`reet scan --server ${quote(site)} --env ${quote(host.trim() || 'my-laptop')} --prune`)
  const enrollment = $derived(`reet agent enroll --server ${quote(site)} --env ${quote(host.trim() || 'my-laptop')}${allowApply ? ' --allow-apply' : ''}\nreet agent run`)
  const emptyFolder = $derived(`mkdir -p ${emptyAgent === 'claude' ? '~/.claude/skills' : emptyAgent === 'cursor' ? '~/.cursor/skills' : emptyAgent === 'opencode' ? '~/.config/opencode/skills' : '~/.agents/skills'}\n${scan}`)
  const serverConfig = $derived(`export REET_CONTROLLER_URL=${quote(site)}\nexport REET_CONTROLLER_TOKEN="$REET_TOKEN"\nexport REET_ENV=${quote(host.trim() || 'my-server')}\nreet serve`)
  function fresh(env: SyncEnvironment): boolean { return env.scannedAt > now - (env.agent === 'reet-server' ? 5 * 60000 : 14 * 86400000) }
  function when(at: number): string { return at ? new Date(at).toLocaleString() : 'Not reported yet' }
  function status(env: SyncEnvironment): string {
    if (env.agent === 'reet-server') return fresh(env) ? 'Server reporting' : 'Server not seen recently'
    if (env.agent === 'reet') return fresh(env) ? 'Scan reported' : 'Scan is out of date'
    if (!env.complete) return 'Run a full scan'
    if (!fresh(env)) return 'Scan is out of date'
    const policy = syncState.targets.find((item) => item.target.id === env.id)
    if (!policy) return 'No reference selected'
    return policy.missing.length || policy.extra.length ? 'Changes needed' : 'In sync with snapshot'
  }
  async function load(background = false) {
    if (refreshing || busy) return
    refreshing = true
    const generation = ++loadGeneration
    if (!background) loading = true; error = ''
    try {
      const next = await getSyncState(); if (generation !== loadGeneration) return; syncState = next; lastRefresh = Date.now()
      if (!loadedOnce) {
        connectOpen = inventoryMachines(syncState.machines ?? []).length === 0 || startConnect
        loadedOnce = true
      }
    } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { loading = false; refreshing = false }
  }
  onMount(() => { void load(); const timer = setInterval(() => { now = Date.now(); if (document.visibilityState === 'visible') void load(true) }, 5000); return () => clearInterval(timer) })
  async function forget() {
    if (!forgetting || busy) return
    const id = forgetting.id; forgetting = null; busy = true; error = ''
    try { syncState = await forgetEnvironment(id); notice = 'Report forgotten. A future scan or server heartbeat can register it again.' } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
</script>

<section class="sync" aria-labelledby="sync-title">
  <header><div><p class="eyebrow">Machines & servers</p><h2 id="sync-title">{view === 'machines' ? 'Your machines' : 'Compare & sync'}</h2></div><button class="btn quiet" type="button" onclick={() => load()} disabled={loading || busy}>{loading ? 'Refreshing…' : 'Refresh status'}</button></header>
  <p class="lede">Keep your reet.sh skills and machines in step. Review differences and approve the changes you want.</p>
  {#if lastRefresh}<p class="hint">Status refreshed {when(lastRefresh)} · updates automatically while this page is open.</p>{/if}
  <SyncControl snapshot={syncState} {canWrite} {view} onupdate={next => { ++loadGeneration; syncState = next }} />
  {#if view === 'machines'}<label class="site">Controller site <input value={site} readonly /></label>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if notice}<p class="notice" role="status">{notice}</p>{/if}

  {#if canWrite && view === 'machines'}
    <details id="connect-machine" class="connect" bind:open={connectOpen}><summary>{machines.length ? 'Connect another machine' : 'Connect your first machine'}</summary>
    <p class="hint">Ey up. Three steps to connect this account to your machine.</p>
    <ol class="setup">
      <li><h3>Install the binary on each machine</h3>
        <label class="channel">Release <select aria-label="Release" bind:value={channel}><option value="stable">Stable — recommended</option><option value="dev">Dev — latest main build</option></select></label>
        {#if channel === 'dev'}<p class="hint">Dev updates after each merge to main. It may contain unfinished changes and does not replace the latest stable release.</p>{/if}
        <div class="command"><pre>{install}</pre><CopyButton text={install} label="Copy install" /></div>
      </li>
      <li><h3>Create a token and connect</h3>
        <p class="hint">Give each machine its own token so you can revoke access separately. The token is shown once and is not saved to browser storage.</p>
        {#if tokenSetup}{@render tokenSetup()}{/if}
        {#if token}
          <div class="command"><pre>{credentials}</pre><CopyButton text={credentials} label="Copy connection" /></div>
          <p class="hint">Paste this into the same terminal you will use for the scan. Keep it private: the command contains your token.</p>
        {:else}
          <details class="existing-token"><summary>Already have a token?</summary><p class="hint">Set <code>REET_URL</code> to <code>{site}</code> and <code>REET_TOKEN</code> to your existing token in your terminal, then continue to the scan below. Existing tokens cannot be displayed again.</p></details>
        {/if}
      </li>
      <li><h3>Report what is installed</h3>
        <label>Machine name <input bind:value={host} oninput={() => connectionSince = Date.now()} placeholder="Choose a unique machine name" autocomplete="off" /></label>
        <div class="command"><pre>{scan}</pre><CopyButton text={scan} label="Copy scan" /></div>
        <p class="hint">For ongoing reporting and web controls, enroll once, then run the agent. Add <code>--project /absolute/path</code> for each project. Add <code>--allow-apply</code> only if this machine should execute plans approved here.</p>
        <label class="apply-opt-in"><input type="checkbox" bind:checked={allowApply} /> Allow this machine to apply plans I approve in reet.sh</label>
        <div class="command"><pre>{enrollment}</pre><CopyButton text={enrollment} label="Copy enrollment" /></div>
        <p class="hint">Run this in each project you want to track. <code>--prune</code> updates this machine's inventory; it does not uninstall skills or remove them from your shelf. Keep the name stable between scans.</p>
      </li>
    </ol>
    <div class="connection-status" role="status">
      <strong>{connectedMachine ? `Reet, ${connectedMachine.host} is connected.` : 'Check your connection'}</strong>
      <p class="hint">{connectedMachine ? 'A complete, recent skill scan has been received. Your machine is ready to compare.' : 'After running the scan, check for a complete report from this machine.'}</p>
      <button class="btn" type="button" onclick={() => load()} disabled={loading || busy}>{loading ? 'Checking…' : 'Check for my scan'}</button>
    </div>
    <details class="server"><summary>Install a server, upgrade or diagnose problems</summary>
      <p class="hint">After downloading reet, choose a user service or Docker Compose. Installation and upgrades show a plan and ask for confirmation. Docker Compose needs Docker with the Compose plugin. Run these commands on the host, outside containers.</p>
      <div class="command"><pre>reet install --mode native
# Or: reet install --mode compose
reet upgrade --check
reet upgrade{channel === 'dev' ? ' --version dev' : ''}
reet doctor</pre><CopyButton text={`reet upgrade${channel === 'dev' ? ' --version dev' : ''}\nreet doctor`} label="Copy upgrade and doctor" /></div>
    </details>
    <details class="server"><summary>Connecting an empty machine?</summary><p class="hint">Choose the agent you want to sync, create its empty skills folder, then report it. This does not install the agent itself.</p><label>Agent <select aria-label="Agent" bind:value={emptyAgent}><option value="agents">Codex / shared agents</option><option value="claude">Claude Code</option><option value="cursor">Cursor</option><option value="opencode">OpenCode</option></select></label><div class="command"><pre>{emptyFolder}</pre><CopyButton text={emptyFolder} label="Copy empty-machine setup" /></div></details>
    <details class="server"><summary>Running reet on another server?</summary><p class="hint">After setting the token above, configure the server process to report to this controller every minute. A container reports only its own files; mount the skill directories you want it to see. Without a remote controller configured, the server identity appears in its local owner's account.</p><div class="command"><pre>{serverConfig}</pre><CopyButton text={serverConfig} label="Copy server setup" /></div></details>
    </details>
  {/if}

  {#if view === 'machines'}
  {#if canWrite && reportedMachines.length}
    <details class="maintenance"><summary>Manage machine names & duplicate reports</summary>
      <p class="hint">Hostname and --env names can be reports from one machine. Choose the report to move, then keep an existing name to merge duplicates or enter a new name to rename it. Only combine reports you know are the same machine.</p>
      <form class="machine-form" onsubmit={previewMachine}>
        <label>Report to move<select aria-label="Report to move" bind:value={maintenanceFrom} required><option value="">Choose a machine</option>{#each reportedMachines as machine (machine.host)}<option value={machine.host}>{machine.host}</option>{/each}</select></label>
        <label>Preferred machine name<input aria-label="Preferred machine name" list="machine-names" bind:value={maintenanceTo} maxlength="200" required placeholder="e.g. PROD_Snicket" /></label>
        <datalist id="machine-names">{#each reportedMachines as machine (machine.host)}<option value={machine.host}></option>{/each}</datalist>
        <button class="btn" type="submit" disabled={busy || !maintenanceFrom || !maintenanceTo.trim()}>Review change</button>
      </form>
    </details>
    <details class="maintenance" ontoggle={event => { if (event.currentTarget.open && !fleetSince) fleetSince=Date.now() }}><summary>Full fleet inventory refresh</summary>
      <p class="hint">Run one command per physical machine, using the name you want to keep. If both a hostname and a friendly name appear, choose the friendly-name command only, or merge them above first. It reconciles the current hostname with the preferred name and rescans detected agents. Run again from each tracked project. Set REET_TOKEN to that account's token first. These commands never install or remove skills.</p>
      <button class="btn quiet" onclick={() => (fleetSince=Date.now())}>Start a new refresh check</button>
      {#each machines as machine (machine.host)}<div class="fleet-command"><strong>{machine.host}</strong><p class="hint" role="status">{refreshedGroups(machine)} of {agentGroups(machine).length} agent groups refreshed since this check started.</p><div class="command"><pre>{refreshCommand(machine.host)}</pre><CopyButton text={refreshCommand(machine.host)} label={`Copy refresh for ${machine.host}`} /></div></div>{/each}
      <p class="hint">The controller cannot rescan machines remotely. After each command finishes, use Refresh status to see the reports received.</p>
    </details>
  {/if}
  <div class="sectionhead"><h3 id="machine-list-title" tabindex="-1">Known machines</h3><span>{machines.length} {machines.length === 1 ? 'machine' : 'machines'} · {syncState.environments.filter(e => !e.agent.startsWith('reet')).length} agent groups</span></div>
  {#if !loading && !machines.length}<p class="empty">No machines have reported yet.{canWrite ? ' Run the scan command above, then refresh status.' : ''}</p>{/if}
  <ul class="environments">
    {#each machines as machine (machine.host)}
      <li><details class="machine-inventory"><summary class="envhead"><div><strong>{machine.host}</strong>{#if machine.aliases?.length}<small class="machine-alias">Also known as {machine.aliases.join(', ')}</small>{/if}</div><span class="badge">{agentGroups(machine).every(g => g.complete && fresh(g)) ? 'Inventory current' : 'Scan needed'} · {machine.skillCount} unique {machine.skillCount === 1 ? 'skill' : 'skills'} · {agentGroups(machine).length} agent {agentGroups(machine).length === 1 ? 'group' : 'groups'}</span></summary>
        {#if machine.aliases?.length}<p class="hint">Also known as: {machine.aliases.join(', ')}</p>{/if}
        {#if canWrite}<button class="btn quiet" disabled={busy} onclick={() => (machineChange={from:machine.host})}>Forget machine {machine.host}</button>{/if}
        <p class="hint">One machine environment. Each agent / project below is a subgroup; shared skills count once above.</p>
        {#each machine.groups.filter(g => g.agent.startsWith('reet')) as heartbeat (heartbeat.id)}
          <p class="hint">{groupLabel(heartbeat)}: {status(heartbeat)} · {heartbeat.os || 'OS unknown'}/{heartbeat.arch || '?'} · {heartbeat.version || 'Version unknown'} · {when(heartbeat.scannedAt || heartbeat.lastSeen)}</p>
          {#if canWrite}<button type="button" class="btn quiet" disabled={busy} onclick={() => forgetting = heartbeat}>Forget {groupLabel(heartbeat).toLowerCase()} report</button>{/if}
        {/each}
        <ul class="agent-groups">
          {#each agentGroups(machine) as env (env.id)}
            <li><div class="envhead"><strong>{groupLabel(env)}</strong><span class="badge" class:attention={!env.complete || !fresh(env)}>{status(env)}</span></div>
              {#if env.error}<p class="error" role="alert">{env.error}</p>{/if}
              <p class="hint">{env.skillCount ?? 0} skills · Last full report: {when(env.scannedAt || env.lastSeen)}</p>
              {#if !env.complete || !fresh(env)}<p class="advice">Run <code>reet scan --server {quote(site)} --env {quote(env.host)} --prune</code>{env.project ? ` from ${env.project}` : ''}, then refresh.</p>{/if}
              {#if canWrite}<button type="button" class="btn quiet" disabled={busy} onclick={() => forgetting = env}>Forget agent group</button>{/if}
            </li>
          {:else}<li class="hint">No agent inventories reported yet. Run a scan on this machine to compare its skills.</li>{/each}
        </ul>
      </details></li>
    {/each}
  </ul>
  {#if serverMachines.length}
    <details class="server"><summary>Server-only reports ({serverMachines.length})</summary><p class="hint">These machines have reported a heartbeat but no agent inventory. Container restarts may create new host IDs.</p>
      <ul>{#each serverMachines as machine (machine.host)}{#each machine.groups as heartbeat (heartbeat.id)}<li><strong>{machine.host}</strong><p class="hint">{status(heartbeat)} · {when(heartbeat.scannedAt || heartbeat.lastSeen)}</p>{#if canWrite}<button class="btn quiet" onclick={() => (machineChange={from:machine.host})} disabled={busy}>Forget machine {machine.host}</button>{/if}</li>{/each}{/each}</ul>
    </details>
  {/if}
  {/if}


</section>

{#if machineChange}
  <ConfirmDialog title={machineChange.to ? 'Combine or rename these reports?' : `Forget ${machineChange.from}?`}
    body={machineChange.to ? `Move ${machineChange.from} into ${machineChange.to}. For matching agent/project groups, the most recent report is kept; distinct groups and saved references are preserved. Old names become aliases. Installed files and shelf entries are unchanged. Conflicting saved references block the whole change.` : 'Remove this machine’s reports and target references. Installed files and shelf entries stay. Snapshots used by other machines remain, and a future scan can report this machine again.'}
    confirmLabel={machineChange.to ? 'Save machine identity' : 'Forget machine'} danger={!machineChange.to} onconfirm={changeMachine} oncancel={() => (machineChange=null)} />
{/if}

{#if forgetting}
  <ConfirmDialog title={`Forget ${forgetting.name}?`} body="This removes its reports and target reference, without uninstalling any files. Saved reference snapshots used by other targets remain. An active server or future scan will register it again." confirmLabel="Forget report" onconfirm={forget} oncancel={() => (forgetting = null)} />
{/if}

<style>
 .apply-opt-in { display:flex; align-items:center; gap:10px; margin:12px 0; }
 .machine-alias { display: block; color: var(--muted); font-size: 0.78rem; margin-top: 4px; overflow-wrap: anywhere; }
 .maintenance { margin: 16px 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; }
 .maintenance > summary { cursor: pointer; min-height: 24px; font-weight: 600; }
 .machine-form { display: grid; gap: 12px; margin-top: 16px; }
 .machine-form label { display: grid; gap: 4px; }
 .fleet-command { margin-top: 16px; }
 .machine-inventory > summary { cursor: pointer; min-height: 44px; }
 .sync { margin-top:24px; padding:clamp(18px,3vw,28px); border:1px solid var(--line); border-radius:12px; background:var(--card); }
 header,.envhead,.sectionhead { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; }
 .eyebrow { margin:0 0 4px; color:var(--accent); text-transform:uppercase; letter-spacing:.1em; font-size:.7rem; font-weight:650; }
 h2 { margin:0; font:500 1.7rem/1.15 var(--serif); } h3 { margin:0 0 8px; font-size:1rem; }
 .lede { color:var(--muted); max-width:72ch; } .hint { color:var(--muted); font-size:.83rem; margin:6px 0; overflow-wrap:anywhere; }
 label { display:grid; gap:6px; font-weight:600; font-size:.85rem; } .site { margin:18px 0; } .site input { color:var(--muted); }
 .setup { display:grid; gap:22px; padding-left:22px; margin:24px 0; } .setup>li { padding-left:4px; min-width:0; }
 .command { display:flex; align-items:center; gap:12px; flex-wrap:wrap; margin:10px 0; padding:12px; background:var(--bg); border:1px solid var(--line); border-radius:8px; }
 pre { flex:1 1 260px; min-width:0; margin:0; white-space:pre-wrap; overflow-wrap:anywhere; font: .78rem/1.6 var(--mono); }
 .connect { margin:18px 0; } .connect>summary { padding:12px 0; }
 .existing-token { margin-top:12px; } .connection-status { margin:20px 0; padding:16px; border:1px solid var(--line); border-radius:10px; background:var(--bg); }
 .server { padding:14px; border:1px solid var(--line); border-radius:8px; } summary { cursor:pointer; font-weight:600; } .server .command { margin-bottom:0; }
 .sectionhead { margin-top:28px; } .sectionhead span { color:var(--muted); font-size:.8rem; }
 .environments { list-style:none; padding:0; display:grid; gap:10px; } .environments>li { border:1px solid var(--line); border-radius:10px; padding:14px; min-width:0; }
 .agent-groups { list-style:none; padding:0 0 0 14px; margin-top:14px; border-left:2px solid var(--line); } .agent-groups>li { padding:12px 0; border-top:1px solid var(--line); }

 .envhead strong { overflow-wrap:anywhere; } .badge { font-size:.73rem; color:var(--muted); padding:3px 8px; border:1px solid var(--line); border-radius:999px; } .attention { color:var(--accent); border-color:var(--accent); }
 .advice { font-size:.8rem; overflow-wrap:anywhere; margin:10px 0 0; }

 .error,.notice { padding:12px; border:1px solid var(--bad); border-radius:8px; overflow-wrap:anywhere; } .notice { border-color:var(--good); } .empty { color:var(--muted); padding:20px 0; }

 @media(max-width:560px) { .command { align-items:stretch; } .command :global(button) { width:100%; } .setup { padding-left:16px; } }
</style>
