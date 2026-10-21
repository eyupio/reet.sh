<script lang="ts">
  import { onMount } from 'svelte'
  import type { Snippet } from 'svelte'
  import { getSyncState, saveSyncTarget, deleteSyncTarget, forgetEnvironment, type SyncState, type SyncEnvironment, type SyncTarget } from './api'
  import CopyButton from './CopyButton.svelte'
  import MachineComparison from './MachineComparison.svelte'
  import { groupLabel, agentGroups, inventoryMachines } from './machines'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite, token = '', tokenSetup, view = 'machines', startConnect = false }: { canWrite: boolean; token?: string; tokenSetup?: Snippet; view?: 'machines' | 'compare'; startConnect?: boolean } = $props()
  let syncState = $state<SyncState>({ environments: [], targets: [] })
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let host = $state('my-laptop')
  let channel = $state('stable')
  let emptyAgent = $state('agents')
  let source = $state('')
  let target = $state('')
  let forgetting = $state<SyncEnvironment | null>(null)
  let now = $state(Date.now())
  // svelte-ignore state_referenced_locally
  let connectOpen = $state(startConnect)
  let loadedOnce = false
  const machines = $derived(inventoryMachines(syncState.machines ?? []))
  const serverMachines = $derived((syncState.machines ?? []).filter(machine => agentGroups(machine).length === 0))
  const connectedMachine = $derived(machines.find(machine => machine.host === host.trim() && agentGroups(machine).some(group => group.complete && fresh(group))))
  const site = location.origin
  function quote(value: string): string { return "'" + value.replaceAll("'", "'\\''") + "'" }
  const install = $derived(`curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh${channel === 'dev' ? ' -s -- --version dev' : ''}`)
  const credentials = $derived(`export REET_URL=${quote(site)}\nexport REET_TOKEN=${quote(token || 'PASTE_YOUR_PERSONAL_TOKEN_HERE')}`)
  const scan = $derived(`reet scan --server ${quote(site)} --env ${quote(host.trim() || 'my-laptop')} --prune`)
  const emptyFolder = $derived(`mkdir -p ${emptyAgent === 'claude' ? '~/.claude/skills' : emptyAgent === 'cursor' ? '~/.cursor/skills' : emptyAgent === 'opencode' ? '~/.config/opencode/skills' : '~/.agents/skills'}\n${scan}`)
  const serverConfig = $derived(`export REET_CONTROLLER_URL=${quote(site)}\nexport REET_CONTROLLER_TOKEN="$REET_TOKEN"\nexport REET_ENV=${quote(host.trim() || 'my-server')}\nreet serve`)
  const references = $derived(syncState.environments.filter((env) => env.complete && fresh(env) && !env.agent.startsWith('reet')))
  const targets = $derived(references.filter((env) => ['claude', 'agents', 'cursor', 'opencode'].includes(env.agent) && (!env.project || env.project.startsWith('/'))))
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
  function syncCommand(item: SyncTarget, apply = false): string {
    return `reet sync --server ${quote(site)} --env ${quote(item.target.host)} --target ${quote(item.target.id)}${apply ? ' --apply' : ''}`
  }
  async function load() {
    loading = true; error = ''
    try {
      syncState = await getSyncState()
      if (!loadedOnce) {
        connectOpen = inventoryMachines(syncState.machines ?? []).length === 0 || startConnect
        loadedOnce = true
      }
    } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { loading = false }
  }
  onMount(() => { void load(); const timer = setInterval(() => now = Date.now(), 30000); return () => clearInterval(timer) })
  async function save(e: SubmitEvent) {
    e.preventDefault(); if (busy || !source || !target || source === target) return
    busy = true; error = ''; notice = ''
    try { syncState = await saveSyncTarget(source, target); notice = 'Reference snapshot saved. Preview on the target machine before applying.' } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
  async function remove(id: string) {
    if (busy) return; busy = true; error = ''
    try { syncState = await deleteSyncTarget(id); notice = 'Sync reference removed. No machine was changed.' } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
  async function forget() {
    if (!forgetting || busy) return
    const id = forgetting.id; forgetting = null; busy = true; error = ''
    try { syncState = await forgetEnvironment(id); if (source === id) source = ''; if (target === id) target = ''; notice = 'Report forgotten. A future scan or server heartbeat can register it again.' } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
</script>

<section class="sync" aria-labelledby="sync-title">
  <header><div><p class="eyebrow">Machines & servers</p><h2 id="sync-title">{view === 'machines' ? 'Your machines' : 'Compare & sync'}</h2></div><button class="btn quiet" type="button" onclick={load} disabled={loading || busy}>{loading ? 'Refreshing…' : 'Refresh status'}</button></header>
  <p class="lede">Use this reet instance as your controller. It remembers each scan and your chosen skill sets; changes run locally, after you confirm.</p>
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
        <label>Machine name <input bind:value={host} placeholder="my-laptop" autocomplete="off" /></label>
        <div class="command"><pre>{scan}</pre><CopyButton text={scan} label="Copy scan" /></div>
        <p class="hint">Run this in each project you want to track. <code>--prune</code> updates this machine's inventory; it does not uninstall skills or remove them from your shelf. Keep the name stable between scans.</p>
      </li>
    </ol>
    <div class="connection-status" role="status">
      <strong>{connectedMachine ? `Reet, ${connectedMachine.host} is connected.` : 'Check your connection'}</strong>
      <p class="hint">{connectedMachine ? 'A complete, recent skill scan has been received. Your machine is ready to compare.' : 'After running the scan, check for a complete report from this machine.'}</p>
      <button class="btn" type="button" onclick={load} disabled={loading || busy}>{loading ? 'Checking…' : 'Check for my scan'}</button>
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

  {#if view === 'compare'}
  <MachineComparison {machines} {canWrite} onreference={(from, to) => { source = from; target = to; notice = 'Groups selected. Review the reference and target below, then save the snapshot.'; document.getElementById('reference-title')?.focus() }} />
  {/if}
  {#if view === 'machines'}
  <div class="sectionhead"><h3>Known machines</h3><span>{machines.length} machines · {syncState.environments.filter(e => !e.agent.startsWith('reet')).length} agent groups</span></div>
  {#if !loading && !machines.length}<p class="empty">No machines have reported yet.{canWrite ? ' Run the scan command above, then refresh status.' : ''}</p>{/if}
  <ul class="environments">
    {#each machines as machine (machine.host)}
      <li><details class="machine-inventory"><summary class="envhead"><strong>{machine.host}</strong><span class="badge">{machine.skillCount} unique skills · {agentGroups(machine).length} agent groups</span></summary>
        <p class="hint">One machine environment. Each agent / project below is a subgroup; shared skills count once above.</p>
        {#each machine.groups.filter(g => g.agent.startsWith('reet')) as heartbeat (heartbeat.id)}
          <p class="hint">{groupLabel(heartbeat)}: {status(heartbeat)} · {heartbeat.os || 'OS unknown'}/{heartbeat.arch || '?'} · {heartbeat.version || 'Version unknown'} · {when(heartbeat.scannedAt || heartbeat.lastSeen)}</p>
          {#if canWrite}<button type="button" class="btn quiet" disabled={busy} onclick={() => forgetting = heartbeat}>Forget {groupLabel(heartbeat).toLowerCase()} report</button>{/if}
        {/each}
        <ul class="agent-groups">
          {#each agentGroups(machine) as env (env.id)}
            <li><div class="envhead"><strong>{groupLabel(env)}</strong><span class="badge" class:attention={!env.complete || !fresh(env)}>{status(env)}</span></div>
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
      <ul>{#each serverMachines as machine (machine.host)}{#each machine.groups as heartbeat (heartbeat.id)}<li><strong>{machine.host}</strong><p class="hint">{status(heartbeat)} · {when(heartbeat.scannedAt || heartbeat.lastSeen)}</p>{#if canWrite}<button class="btn quiet" onclick={() => forgetting = heartbeat} disabled={busy}>Forget report for {machine.host}</button>{/if}</li>{/each}{/each}</ul>
    </details>
  {/if}
  {/if}

  {#if view === 'compare'}
  {#if canWrite}
    <h3 id="reference-title" class="mirror-title" tabindex="-1">Choose a reference skill set</h3>
    <p class="hint">Choose a machine and agent group on each side. Capture a recently scanned group as the desired skill set for the target group. This saves a snapshot on the controller; nothing is installed yet. Save again when you want to adopt newer reference changes. Local-only skills need a linked source first.</p>
    <form class="reference" onsubmit={save}>
      <label>Reference <select aria-label="Reference" bind:value={source} disabled={busy}><option value="">Choose reference</option>{#each machines as m}<optgroup label={m.host}>{#each references.filter(e => e.host === m.host) as env}<option value={env.id} disabled={env.id === target}>{groupLabel(env)}</option>{/each}</optgroup>{/each}</select></label>
      <label>Target <select aria-label="Target" bind:value={target} disabled={busy}><option value="">Choose target</option>{#each machines as m}<optgroup label={m.host}>{#each targets.filter(e => e.host === m.host) as env}<option value={env.id} disabled={env.id === source}>{groupLabel(env)}</option>{/each}</optgroup>{/each}</select></label>
      <button class="btn primary" type="submit" disabled={busy || !source || !target || source === target}>{busy ? 'Saving…' : 'Save reference snapshot'}</button>
    </form>
    {#if !references.length}<p class="hint">Run a full scan first. Only complete scans from the last 14 days can be used to create a reference.</p>{/if}
  {/if}
  <ul class="policies">
    {#each syncState.targets as item (item.target.id)}
      <li><div class="envhead"><strong>{item.target.name}</strong><span class="badge">{item.missing.length} to install · {item.extra.length} extra</span></div>
        <p class="hint">Reference: {item.sourceName} · snapshot {when(item.updatedAt)}</p>
        {#if item.missing.length || item.extra.length}<details><summary>Review differences</summary><ul>{#each item.missing as skill}<li>Install {skill.name} <code>{skill.source}</code></li>{/each}{#each item.extra as skill}<li>Remove {skill.name}</li>{/each}</ul></details>{/if}
        <p class="hint">Run on <strong>{item.target.host}</strong>{item.target.project ? ` from project ${item.target.project}` : ''}. The CLI checks current files again and stops on unknown sources, name conflicts, or shared-folder removals that would break another detected agent. Removed entries are backed up locally.</p>
        {#if canWrite}<div class="command"><pre>{syncCommand(item)}</pre><CopyButton text={syncCommand(item)} label="Copy preview" /></div>{/if}
        {#if canWrite}<div class="policy-actions"><CopyButton text={syncCommand(item, true)} label="Copy apply command" /><button type="button" class="btn quiet" disabled={busy} onclick={() => remove(item.target.id)}>Stop tracking this reference</button></div><p class="hint">Apply lists installs and removals, then asks you to type the target name. The web page never executes commands on your machines.</p>{/if}
      </li>
    {/each}
  </ul>
  {/if}
</section>

{#if forgetting}
  <ConfirmDialog title={`Forget ${forgetting.name}?`} body="This removes its reports and target reference, without uninstalling any files. Saved reference snapshots used by other targets remain. An active server or future scan will register it again." confirmLabel="Forget report" onconfirm={forget} oncancel={() => (forgetting = null)} />
{/if}

<style>
 .machine-inventory > summary { cursor: pointer; min-height: 44px; }
 .sync { margin-top:24px; padding:clamp(18px,3vw,28px); border:1px solid var(--line); border-radius:12px; background:var(--card); }
 header,.envhead,.sectionhead,.policy-actions { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; }
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
 .environments,.policies { list-style:none; padding:0; display:grid; gap:10px; } .environments>li,.policies>li { border:1px solid var(--line); border-radius:10px; padding:14px; min-width:0; }
 .agent-groups { list-style:none; padding:0 0 0 14px; margin-top:14px; border-left:2px solid var(--line); } .agent-groups>li { padding:12px 0; border-top:1px solid var(--line); }

 .envhead strong { overflow-wrap:anywhere; } .badge { font-size:.73rem; color:var(--muted); padding:3px 8px; border:1px solid var(--line); border-radius:999px; } .attention { color:var(--accent); border-color:var(--accent); }
 .advice { font-size:.8rem; overflow-wrap:anywhere; margin:10px 0 0; } .mirror-title { margin-top:28px; }
 .reference { display:grid; grid-template-columns:1fr 1fr; gap:12px; margin:16px 0; } .reference select { width:100%; } .reference .btn { grid-column:1/-1; justify-self:start; }
 .error,.notice { padding:12px; border:1px solid var(--bad); border-radius:8px; overflow-wrap:anywhere; } .notice { border-color:var(--good); } .empty { color:var(--muted); padding:20px 0; }
 .policies details { margin:12px 0; font-size:.85rem; } .policies details li { padding:4px 0; } .policies details code { overflow-wrap:anywhere; }
 @media(max-width:560px) { .reference { grid-template-columns:1fr; } .command { align-items:stretch; } .command :global(button) { width:100%; } .setup { padding-left:16px; } }
</style>
