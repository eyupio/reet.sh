<script lang="ts">
  import { onMount } from 'svelte'
  import CopyButton from '../CopyButton.svelte'
  import { createTerminal, machineHealth, readiness, terminalMachines, terminalSessions, type HealthResponse, type TerminalMachine, type TerminalSession } from './api'
  import MachineHealthStrip from './MachineHealthStrip.svelte'
  import TerminalPane from './TerminalPane.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import HealthHistory from './HealthHistory.svelte'
  import { renameTerminal, endTerminal } from './api'
  import { TerminalError, validTerminalTitle } from './api'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import TerminalCommandReference from './TerminalCommandReference.svelte'

  let machines = $state<TerminalMachine[]>([]), sessions = $state<TerminalSession[]>([])
  let selectedAgent = $state(new URLSearchParams(location.search).get('agent') || '')
  let selectedSession = $state(new URLSearchParams(location.search).get('session') || '')
  let health = $state<HealthResponse>({ serverNow:Date.now(),machines:[] })
  let receivedAt = performance.now(), elapsed = $state(0)
  let loading = $state(true), busy = $state(false), error = $state(''), title = $state('Shell')
  let requestId = $state('')
  let workspace = $state<HTMLElement>(), workspaceHeight = $state(Math.max(120, innerHeight - 78))
  let navOpen = $state(innerWidth >= 960), detailsOpen = $state(false), query = $state('')
  let healthIssue=$state(''),showCreate=$state(false)
  let action = $state<'rename'|'end'|null>(null), actionSession = $state(''), rename = $state(''), actionTitle = $state(''), actionBusy = $state(false)
  const endKeys = new Map<string,string>()
  let refreshing = false, disposed = false
  const machine = $derived(machines.find(item=>item.agentId===selectedAgent))
  const ownSessions = $derived(sessions.filter(item=>item.agentId===selectedAgent))
  const filteredSessions = $derived(ownSessions.filter(item => `${item.title} ${item.state.replaceAll('_',' ')}`.toLowerCase().includes(query.toLowerCase())))
  const activeSessions = $derived(ownSessions.filter(item=>['creating','running','end_requested'].includes(item.state)))
  const accountActive = $derived(sessions.filter(item=>['creating','running','end_requested'].includes(item.state)).length)
  const atQuota = $derived(activeSessions.length>=8 || accountActive>=32)
  const session = $derived(ownSessions.find(item=>item.id===selectedSession))
  const machineStatus = $derived(health.machines.find(item=>item.agentId===selectedAgent))
  const setup = $derived(readiness(machine))
  const serverNow = $derived(health.serverNow+elapsed)

  function remember() { const url=new URL(location.href);url.searchParams.set('view','terminal');if(selectedAgent)url.searchParams.set('agent',selectedAgent);else url.searchParams.delete('agent');if(selectedSession)url.searchParams.set('session',selectedSession);else url.searchParams.delete('session');history.replaceState(null,'',url) }
  function chooseMachine(id: string) { selectedAgent=id;selectedSession=sessions.find(item=>item.agentId===id&&item.state==='running')?.id||'';requestId='';error='';remember() }
  function chooseSession(id: string) { selectedSession=id;remember();if(innerWidth<960)navOpen=false }
  async function refresh(signal?: AbortSignal) {
    if(refreshing||disposed)return
    refreshing=true
    try {
      const [nextMachines,nextSessions,nextHealth]=await Promise.all([terminalMachines(signal),terminalSessions(signal),machineHealth(signal).then(data=>({data,error:''})).catch(()=>({data:null,error:'Resource readings are temporarily unavailable. Existing readings are marked by their observation time.'}))])
      if(disposed)return
      machines=nextMachines;sessions=nextSessions;healthIssue=nextHealth.error
      if(nextHealth.data){health=nextHealth.data;receivedAt=performance.now();elapsed=0}
      if(!selectedAgent){selectedAgent=machines[0]?.agentId||'';selectedSession=''}
      if(!selectedSession)selectedSession=sessions.find(item=>item.agentId===selectedAgent&&item.state==='running')?.id||''
      remember()
    } catch(e){if(!signal?.aborted&&!disposed)error=e instanceof Error?e.message:'Could not refresh machines.'}
    finally{refreshing=false;loading=false}
  }
  async function create() {
    if(busy||!machine?.ready||!validTerminalTitle(title.trim())||atQuota&&!requestId)return
    busy=true;error=''
    // Keep this key on an uncertain response. Clicking retry inspects the
    // original operation rather than silently creating a replacement shell.
    requestId ||= crypto.randomUUID()
    const agent=selectedAgent,key=requestId
    try {const created=await createTerminal(agent,key,title.trim());if(disposed)return;sessions=[created,...sessions.filter(item=>item.id!==created.id)];if(selectedAgent===agent)selectedSession=created.id;if(requestId===key)requestId='';showCreate=false;remember();await refresh()}
    catch(e){if(disposed)return;if(e instanceof TerminalError&&e.status===400&&requestId===key)requestId='';error=e instanceof Error?e.message:'Creation could not be confirmed. Retry with the same request.'}
    finally{busy=false}
  }
  function openAction(kind:'rename'|'end') { if(!session)return;actionSession=session.id;actionTitle=session.title;rename=session.title;action=kind }
  async function performAction() {
    const kind=action,id=actionSession;action=null;if(!kind||actionBusy)return
    actionBusy=true;error=''
    try {
      if(kind==='rename'){if(!validTerminalTitle(rename.trim())){error='Use a session name of 1–80 characters without control characters.';return}await renameTerminal(id,rename.trim())}
      else {let key=endKeys.get(id);if(!key){key=crypto.randomUUID();endKeys.set(id,key)}await endTerminal(id,key)}
      await refresh()
    } catch(e){if(!disposed)error=e instanceof Error?e.message:'Action could not be confirmed. Retry the same action.'}
    finally{actionBusy=false}
  }
  onMount(() => {
    let frame=0
    const measure=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(()=>{if(!workspace)return;const viewport=window.visualViewport;workspaceHeight=Math.max(120,(viewport?.height||innerHeight)-Math.max(0,workspace.getBoundingClientRect().top-(viewport?.offsetTop||0))-8)})}
    const observer=new ResizeObserver(measure),header=document.querySelector('.site-header')
    if(header)observer.observe(header)
    measure();window.addEventListener('resize',measure);window.visualViewport?.addEventListener('resize',measure);window.visualViewport?.addEventListener('scroll',measure)
    return()=>{cancelAnimationFrame(frame);observer.disconnect();window.removeEventListener('resize',measure);window.visualViewport?.removeEventListener('resize',measure);window.visualViewport?.removeEventListener('scroll',measure)}
  })
  onMount(()=>{
    const abort=new AbortController();void refresh(abort.signal)
    const poll=setInterval(()=>{if(document.visibilityState==='visible')void refresh(abort.signal)},15000)
    const clock=setInterval(()=>{elapsed=performance.now()-receivedAt},1000)
    const wake=()=>{if(document.visibilityState==='visible')void refresh(abort.signal)}
    document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake)
    return()=>{disposed=true;abort.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}
  })
</script>

{#snippet createForm()}
  <form class="create-form" onsubmit={event => { event.preventDefault(); void create() }}>
    <label>New session name<input bind:value={title} disabled={busy || !!requestId} maxlength="80" required placeholder="Build, deploy, investigate…" /></label>
    <button class="btn primary" disabled={busy || !machine?.ready || !validTerminalTitle(title.trim()) || atQuota && !requestId}>{busy ? 'Creating…' : requestId ? 'Retry creation' : 'New terminal'}</button>
    {#if !machine?.ready}<p class="create-reason" role="status">{setup || 'Choose an available machine before creating a shell.'}</p>{/if}
    <p>{activeSessions.length} / 8 active on this machine · {accountActive} / 32 across your account{atQuota ? ' · Session limit reached' : ''}. Existing shells keep running when you switch or disconnect.</p>
  </form>
{/snippet}

<section class="workspace" bind:this={workspace} style={`--workspace-height:${workspaceHeight}px`} aria-label="Remote terminal workspace">
  <header class="workspace-toolbar">
    <div class="workspace-title"><a href="/#active-sessions" aria-label="Back to active sessions">←</a><h1>Terminals</h1></div>
    <label class="machine-picker"><span class="vh">Machine</span><select aria-label="Machine" value={selectedAgent} onchange={event => chooseMachine(event.currentTarget.value)}><option value="" disabled>Choose a machine</option>{#each machines as item (item.agentId)}<option value={item.agentId}>{item.host}{item.ready ? ' · Ready' : ''}</option>{/each}</select></label>
    <div class="workspace-actions"><button class="btn" aria-expanded={navOpen} aria-controls="terminal-sessions" onclick={() => navOpen = !navOpen}>Sessions <span class="count">{activeSessions.length}</span></button><button class="btn primary new-button" disabled={!machines.length} onclick={() => { if(session)showCreate = true;else workspace?.querySelector<HTMLInputElement>('.create-form input')?.focus() }} aria-label="New session"><span class="wide-label">New session</span><span class="mobile-label">New</span></button><button class="btn" aria-label="Details & help" onclick={() => detailsOpen = true}><span class="wide-label">Details &amp; help</span><span class="mobile-label">Help</span></button></div>
  </header>
  {#if error && !showCreate}<p class="workspace-notice bad" role="alert">{error} <button onclick={() => void refresh()}>Retry status</button></p>{/if}
  <div class="workspace-body" class:with-sidebar={navOpen}>
    {#if navOpen}<aside class="session-sidebar" id="terminal-sessions" aria-label="Session navigation">
      <div class="sidebar-heading"><strong>Your sessions</strong><button class="close-sidebar" onclick={() => navOpen = false} aria-label="Close session navigation">×</button></div>
      <label><span class="vh">Find sessions</span><input type="search" bind:value={query} placeholder="Find a session…" aria-label="Find sessions" /></label>
      <p class="machine-user">{machine?.capabilities?.osUser ? `Shell user ${machine.capabilities.osUser}` : 'Private machine shells'}</p>
      <nav aria-label="Sessions on selected machine">{#each filteredSessions as item (item.id)}<button class="session-row" class:selected={selectedSession === item.id} aria-current={selectedSession === item.id ? 'true' : undefined} onclick={() => chooseSession(item.id)}><span class="session-dot" class:running={item.state === 'running'} aria-hidden="true"></span><span><strong>{item.title}</strong><small>{item.state.replaceAll('_',' ')}</small></span></button>{:else}<p class="sidebar-empty">{query ? 'No sessions match.' : 'No sessions on this machine yet.'}</p>{/each}</nav>
      <div class="sidebar-footer"><button class="btn quiet" onclick={() => void refresh()}>Refresh status</button><a href="/machines">Manage machines →</a></div>
    </aside>{/if}
    <div class="terminal-stage">
      {#if loading}<div class="empty" role="status"><span class="empty-icon" aria-hidden="true">›_</span><h2>Finding your shells…</h2><p>Connecting to your existing workspace.</p></div>
      {:else if !machines.length}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>Connect your first machine</h2><p>Enroll a machine and enable terminal access locally to open a shell here.</p><a class="btn primary" href="/machines#connect-machine">Connect a machine</a></div>
      {:else if session}
        {#if setup || session.state === 'end_requested'}<div class="workspace-notice" role="status">{session.state === 'end_requested' ? 'End requested. Waiting for this machine to confirm; the shell still counts towards its limit.' : setup}<button onclick={() => detailsOpen = true}>View details</button></div>{/if}
        {#key session.id}<TerminalPane {session} host={machine?.host || 'Unavailable machine'} onrename={() => openAction('rename')} onend={() => openAction('end')} actionDisabled={actionBusy || machine?.serverEnabled === false} />{/key}
      {:else}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>{selectedSession ? 'This session is unavailable' : selectedAgent && !machine ? 'This machine is unavailable' : 'Your next shell starts here.'}</h2><p>{selectedSession ? 'Choose an existing session or create one explicitly. No replacement shell was created.' : setup || 'Create a terminal, then return to the same session whenever you need it.'}</p>{@render createForm()}{#if setup}<button class="btn" onclick={() => detailsOpen = true}>View setup &amp; connection details</button>{/if}</div>{/if}
    </div>
  </div>
</section>

{#if showCreate}<WorkspaceDialog title="New terminal session" onclose={() => showCreate = false}><p class="dialog-copy">Create a shell on <strong>{machine?.host || 'the selected machine'}</strong>. Changing machines does not stop existing sessions.</p>{#if error}<p class="dialog-copy" role="alert">{error}</p>{/if}{@render createForm()}</WorkspaceDialog>{/if}
{#if detailsOpen}<WorkspaceDialog title="Machine & terminal details" onclose={() => detailsOpen = false}>
  <p class="dialog-copy">{machine?.host || 'No machine selected'}{machine?.capabilities?.osUser ? ` · Shell user ${machine.capabilities.osUser}` : ''}</p>
  {#if setup}<section class="setup-notice"><h3>Terminal setup</h3><p>{setup}</p>{#if machine?.serverEnabled !== false && !machine?.capabilities?.terminalEnabled}<pre>reet agent stop
reet agent terminal enable
reet agent service install</pre><CopyButton text={'reet agent stop\nreet agent terminal enable\nreet agent service install'} label="Copy local terminal setup" />{:else if machine?.capabilities?.readiness === 'supervisor_unavailable'}<pre>reet agent stop
reet agent service install</pre><CopyButton text={'reet agent stop\nreet agent service install'} label="Copy service install command" />{/if}</section>{/if}
  {#if healthIssue}<p class="dialog-copy" role="status">{healthIssue}</p>{/if}
  <section aria-label="Machine diagnostics"><h3>Machine health</h3><MachineHealthStrip health={machineStatus} {serverNow} />{#key selectedAgent}<HealthHistory agent={selectedAgent} />{/key}</section>
  <TerminalCommandReference />
  <section class="shortcut-help"><h3>Keyboard &amp; clipboard</h3><p>Ctrl+C interrupts the shell. On Windows/Linux, Ctrl+C copies selected terminal output; use Interrupt to send Ctrl+C when text is selected. On Mac, Cmd+C copies.</p><p>Ctrl+V / Cmd+V pastes. Ctrl+Shift+C/V are alternatives. Right-click the terminal for Copy/Paste; Shift+right-click keeps the browser menu. Ctrl+X stays a shell/editor key; cut works normally in editable app fields.</p><p>Multiple lines, control characters and large pastes need review. Disconnect keeps the shell running. End session stops its running work.</p></section>
</WorkspaceDialog>{/if}
{#if action === 'rename'}<ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={() => void performAction()} oncancel={() => action = null}><label>Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>{:else if action === 'end'}<ConfirmDialog title={`End ${actionTitle}?`} body="This stops the shell and its running jobs. Disconnecting keeps them running. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={() => void performAction()} oncancel={() => action = null} />{/if}

<style>
  .workspace{height:var(--workspace-height);min-height:0;display:flex;flex-direction:column;border:1px solid var(--line);border-radius:12px;background:var(--card);overflow:hidden}.workspace-toolbar{display:flex;align-items:center;gap:14px;padding:8px 12px;border-bottom:1px solid var(--line);flex-shrink:0}.workspace-title{display:flex;align-items:center;gap:8px}.workspace-title>a{font-size:1.2rem;text-decoration:none;padding:4px}.workspace-title h1{font:500 1.2rem var(--serif);margin:0}.machine-picker{min-width:0;max-width:280px;flex:1}.machine-picker select{width:100%;min-width:0;padding:6px 8px;min-height:34px;font-size:.78rem}.workspace-actions{display:flex;gap:6px;margin-left:auto;flex-shrink:0}.workspace-actions .btn{font-size:.75rem;padding:6px 10px;min-height:34px}.workspace-actions .count{font-size:.65rem;border-radius:20px;background:var(--tint);padding:1px 5px}.workspace-body{display:grid;grid-template-columns:minmax(0,1fr);flex:1;min-height:0;position:relative}.workspace-body.with-sidebar{grid-template-columns:220px minmax(0,1fr)}.session-sidebar{display:flex;flex-direction:column;gap:12px;background:var(--bg);border-right:1px solid var(--line);padding:14px;min-height:0;overflow:auto}.sidebar-heading{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:.8rem}.close-sidebar{background:none;border:0;font-size:1.4rem;cursor:pointer;line-height:1}.session-sidebar input{width:100%;min-height:34px;padding:6px 8px;font-size:.76rem}.machine-user{font-size:.65rem;color:var(--muted);margin:-4px 0 0;overflow-wrap:anywhere}.session-sidebar nav{display:grid;gap:6px;align-content:start}.session-row{display:flex;gap:8px;align-items:flex-start;text-align:left;border:1px solid transparent;border-radius:8px;background:transparent;padding:10px 8px;cursor:pointer;min-width:0;color:var(--ink)}.session-row.selected{background:var(--card);border-color:var(--line)}.session-row:hover{border-color:var(--accent)}.session-row>span:last-child{display:grid;gap:3px;min-width:0}.session-row strong{font-size:.76rem;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.session-row small{font-size:.65rem;color:var(--muted);text-transform:capitalize}.session-dot{height:6px;width:6px;flex:none;background:var(--muted);border-radius:50%;margin-top:6px}.session-dot.running{background:var(--good)}.sidebar-empty{color:var(--muted);font-size:.75rem}.sidebar-footer{margin-top:auto;display:grid;gap:8px;padding-top:14px}.sidebar-footer>a{font-size:.7rem;text-decoration:none}.sidebar-footer .btn{justify-content:flex-start;padding:0;font-size:.72rem;min-height:28px}.terminal-stage{min-height:0;min-width:0;display:flex;flex-direction:column;overflow:hidden}.terminal-stage :global(.pane:not(.maximised)){flex:1;min-height:0;border:0;border-radius:0;box-shadow:none}.empty{display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:24px;flex:1;min-height:0;overflow:auto;color:var(--muted)}.empty-icon{font:500 2.2rem var(--mono);color:var(--accent)}.empty h2{font:500 1.6rem var(--serif);color:var(--ink);margin:12px 0}.empty>p{font-size:.85rem;max-width:55ch;margin:0 0 20px}.create-form{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;max-width:480px;width:100%;text-align:left;align-items:end}.create-form label{display:grid;gap:6px;font-size:.75rem}.create-form input{width:100%;min-width:0}.create-form p{grid-column:1/-1;font-size:.72rem;color:var(--muted);line-height:1.5;margin:0 0 16px}.workspace-notice{margin:0;padding:7px 12px;background:var(--tint);font-size:.74rem;flex-shrink:0;display:flex;align-items:center;gap:10px;justify-content:space-between}.workspace-notice button{border:0;background:none;text-decoration:underline;cursor:pointer;white-space:nowrap;color:inherit;font:inherit}.bad{color:var(--bad)}.dialog-copy,.setup-notice p,.shortcut-help p{font-size:.8rem;color:var(--muted);line-height:1.6;margin:0}.setup-notice{border:1px solid var(--line);border-left:3px solid var(--accent);padding:14px;border-radius:8px}.setup-notice pre{font-size:.75rem;white-space:pre-wrap;overflow-wrap:anywhere}h3{font-size:.85rem;margin:0 0 10px}.shortcut-help p+p{margin-top:8px}
  .mobile-label{display:none}
  @media(max-width:960px){.workspace-body.with-sidebar{grid-template-columns:minmax(0,1fr)}.session-sidebar{position:absolute;inset:0 auto 0 0;width:min(290px,85%);z-index:8;box-shadow:12px 0 30px rgb(0 0 0 / .12)}}
  @media(max-width:650px){.workspace{border-radius:8px}.workspace-toolbar{gap:6px;padding:5px 6px}.workspace-title h1{display:none}.workspace-title>a{padding:3px}.machine-picker select{font-size:.72rem;padding:4px;min-height:32px}.workspace-actions{gap:4px}.workspace-actions .btn{padding:5px 7px;min-height:32px;font-size:.67rem}.workspace-actions .count,.workspace-actions .wide-label{display:none}.workspace-actions .mobile-label{display:inline}.empty{padding:20px 14px}.empty h2{font-size:1.35rem}.create-form{grid-template-columns:minmax(0,1fr)}.create-form p{grid-column:1}.workspace-notice{font-size:.65rem;padding:5px 8px}}
  @media(pointer:coarse){.workspace-actions .btn,.machine-picker select{min-height:40px}}
</style>
