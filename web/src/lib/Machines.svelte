<script lang="ts">
  import EnvironmentSync from './EnvironmentSync.svelte'
  import PageHeader from './PageHeader.svelte'
  import ProjectsBar from './ProjectsBar.svelte'
  import FleetSoftware from './FleetSoftware.svelte'
  import { createToken } from './api'
  import { describeError } from './contentReview'
  import { onMount, type Component } from 'svelte'
  let { canWrite, accounts, monitoring=false, compare = false, userId=0, onsignedout, ontitle }: { canWrite: boolean; accounts: boolean; monitoring?:boolean; compare?: boolean;userId?:number; onsignedout?: (e: unknown) => void; ontitle?: (title: string) => void } = $props()
  let token = $state(''), name = $state('machine token'), busy = $state(false), error = $state(''), softwareOpen = $state(false)
  // A session address (?session=, ?agent= or view=terminal) opens the workspace. A bare /sessions reaches this page only for
  // someone without Sessions (a viewer, a token or an open instance), who gets the Machines page itself.
  const query = new URLSearchParams(location.search)
  // Without Sessions (not a writer in accounts mode) a session address shows the Machines page with its header too.
  // svelte-ignore state_referenced_locally
  const terminals = (location.pathname === '/sessions' && (query.has('session') || query.has('agent')) || query.get('view') === 'terminal') && accounts && canWrite
  // svelte-ignore state_referenced_locally
  const focus = compare ? '' : (new URLSearchParams(location.search).get('machine') ?? '')
  let TerminalView = $state<Component<{userId?:number; ontitle?: (title: string) => void}>>()
  onMount(()=>{let disposed=false;if(terminals&&accounts&&canWrite)void import('./terminal/TerminalWorkspace.svelte').then(module=>{if(!disposed)TerminalView=module.default}).catch(()=>{if(!disposed)error='Could not load the terminal workspace. Refresh this page to retry.'});return()=>{disposed=true}})
  async function connectToken() {
    if (busy) return
    busy = true; error = ''
    try { token = (await createToken(name.trim() || 'machine token')).token }
    catch (e) { error = describeError(e) }
    finally { busy = false }
  }
</script>
{#if !terminals}<PageHeader eyebrow={compare || !focus ? 'Machines' : 'Machine'} title={compare ? "Sync" : focus || "Your machines, connected."} lede={compare ? "Choose skills and machines, then sync. Turn on Keep synced to let reet maintain them automatically." : focus ? "Inspect this machine's agent scopes, health and saved skill sets, or continue in its terminal." : "Connect a machine, inspect its skills and agents, and open a terminal to put them to work."} />{/if}
{#if !terminals}<nav aria-label="Machines"><a class="btn quiet" href="/machines" aria-current={!compare&&!terminals ? 'page' : undefined}>Inventory &amp; connection</a><a class="btn quiet" href="/sync" aria-current={compare ? 'page' : undefined}>Sync</a>{#if accounts && canWrite}<button class="btn quiet" type="button" onclick={() => (softwareOpen = true)}>Software</button>{/if}</nav>{/if}
{#if terminals && accounts && canWrite}{#if TerminalView}<TerminalView {userId} {ontitle} />{:else if error}<p class="notice bad" role="alert">{error}</p>{:else}
  <div class="workspace-boot" role="status" aria-busy="true" aria-label="Loading the terminal workspace">
    <span class="vh">Loading the terminal workspace…</span>
    <div class="workspace-boot-bar" aria-hidden="true"><span class="mark"></span><span class="bar"></span><span class="tools"><i></i><i></i><i></i></span></div>
    <div class="workspace-boot-body" aria-hidden="true">
      <div class="rail"><i></i><i></i><i></i><i></i></div>
      <div class="screen"><p><b>›_</b> connecting to your machines<span class="cursor"></span></p></div>
    </div>
  </div>
  {/if}{:else}
<EnvironmentSync {canWrite} {accounts} monitorAccess={monitoring} terminalAccess={accounts&&canWrite} {token} {onsignedout} view={compare ? 'compare' : 'machines'} startConnect={location.hash === '#connect-machine'}>
  {#snippet tokenSetup()}
    {#if accounts && canWrite}
      {#if !token}
        <form onsubmit={event => { event.preventDefault(); void connectToken() }}><label>Token name <input bind:value={name} maxlength="100" required autocomplete="off" /></label><button class="btn" disabled={busy}>{busy ? 'Creating…' : 'Create machine token'}</button></form>
      {:else}<p role="status">Token created. Copy the one-time token below before leaving this page. Revoke it under Account → API tokens.</p>{/if}
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    {:else}<p>Use the token configured for this instance when the CLI asks. Instances without authentication do not require one.</p>{/if}
  {/snippet}
</EnvironmentSync>
{/if}
{#if softwareOpen}<FleetSoftware onclose={() => (softwareOpen = false)} />{/if}
{#if !terminals && !compare && accounts && canWrite}
  <details class="projects-drawer">
    <summary>Projects</summary>
    <ProjectsBar {canWrite} {accounts} {onsignedout} />
  </details>
{/if}
<style>
 nav,form {display:flex;flex-wrap:wrap;gap:10px;align-items:end;margin-bottom:20px} a {text-decoration:none} a[aria-current=page] {background:var(--tint);border-color:var(--accent)} label {display:grid;gap:6px;font-size:.85rem} p {font-size:.85rem;color:var(--muted)}
 .projects-drawer {margin:26px 0 0;border-top:1px solid var(--line);padding-top:14px}
 .projects-drawer > summary {cursor:pointer;color:var(--muted);font-size:.82rem;font-weight:650;min-height:32px}
 @media (max-width:560px) { nav {display:grid;grid-template-columns:repeat(2,minmax(0,1fr));align-items:stretch} nav a {justify-content:center} }
 /* The terminal workspace is its own chunk, so opening a session waits for it. This holds the workspace's shape so the
    page never shows a bare sentence on an empty background, then gives way to the real thing. */
 .workspace-boot { display:flex; flex-direction:column; height:calc(100dvh - 78px); min-height:220px; border:1px solid var(--line); border-radius:12px; background:var(--card); overflow:hidden; }
 .workspace-boot-bar { display:flex; align-items:center; gap:10px; padding:8px 12px; border-bottom:1px solid var(--line); flex-shrink:0; }
 .workspace-boot-bar .mark { width:26px; height:22px; border-radius:6px; }
 .workspace-boot-bar .bar { width:110px; height:12px; border-radius:6px; }
 .workspace-boot-bar .tools { display:flex; gap:6px; margin-left:auto; }
 .workspace-boot-bar .tools i { width:74px; height:30px; border-radius:8px; }
 .workspace-boot-bar i, .workspace-boot-bar .mark, .workspace-boot-bar .bar, .rail i { display:block; background:var(--line); }
 .workspace-boot-body { display:grid; grid-template-columns:220px minmax(0,1fr); flex:1; min-height:0; }
 .rail { padding:14px; display:grid; align-content:start; gap:10px; border-right:1px solid var(--line); background:var(--bg); }
 .rail i { height:38px; border-radius:8px; }
 .rail i:nth-child(2n) { width:82%; }
 .screen { display:flex; align-items:flex-end; padding:18px; background:var(--bg-sunk); }
 .screen p { margin:0; font:12px/1.7 var(--mono); color:var(--muted); display:flex; align-items:center; gap:8px; }
 .screen b { color:var(--accent-text); font-weight:400; }
 .screen .cursor { display:inline-block; width:7px; height:13px; background:var(--accent-text); }
 @media (prefers-reduced-motion: no-preference) { .workspace-boot-bar i, .workspace-boot-bar .mark, .workspace-boot-bar .bar, .rail i { animation:boot-pulse 1.7s ease-in-out infinite; } .screen .cursor { animation:boot-pulse 1s steps(2,end) infinite; } }
 @keyframes boot-pulse { 0%,100% { opacity:1 } 50% { opacity:.55 } }
 @media (max-width:650px) { .workspace-boot { height:calc(100dvh - 70px); border-radius:8px; } .workspace-boot-body { grid-template-columns:1fr; } .rail { display:none; } .workspace-boot-bar .tools i { width:40px; } }
</style>
