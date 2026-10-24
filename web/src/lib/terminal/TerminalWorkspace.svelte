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
  import { fresh, value } from './health'
  import TerminalCommandReference from './TerminalCommandReference.svelte'

  let machines = $state<TerminalMachine[]>([]), sessions = $state<TerminalSession[]>([])
  let selectedAgent = $state(new URLSearchParams(location.search).get('agent') || '')
  let selectedSession = $state(new URLSearchParams(location.search).get('session') || '')
  let health = $state<HealthResponse>({ serverNow:Date.now(),machines:[] })
  let receivedAt = performance.now(), elapsed = $state(0)
  let loading = $state(true), busy = $state(false), error = $state(''), title = $state('Shell')
  let requestId = $state('')
  let healthIssue=$state(''),showCreate=$state(false)
  let action = $state<'rename'|'end'|null>(null), actionSession = $state(''), rename = $state(''), actionBusy = $state(false)
  const endKeys = new Map<string,string>()
  let refreshing = false, disposed = false
  const machine = $derived(machines.find(item=>item.agentId===selectedAgent))
  const ownSessions = $derived(sessions.filter(item=>item.agentId===selectedAgent))
  const activeSessions = $derived(ownSessions.filter(item=>['creating','running','end_requested'].includes(item.state)))
  const atQuota = $derived(activeSessions.length>=8 || sessions.filter(item=>['creating','running','end_requested'].includes(item.state)).length>=32)
  const session = $derived(ownSessions.find(item=>item.id===selectedSession))
  const machineStatus = $derived(health.machines.find(item=>item.agentId===selectedAgent))
  const setup = $derived(readiness(machine))
  const serverNow = $derived(health.serverNow+elapsed)
  const cpu=$derived(value(machineStatus?.connection?.report?.cpuPercent)),healthFresh=$derived(fresh(machineStatus,serverNow))

  function remember() { const url=new URL(location.href);url.searchParams.set('view','terminal');if(selectedAgent)url.searchParams.set('agent',selectedAgent);else url.searchParams.delete('agent');if(selectedSession)url.searchParams.set('session',selectedSession);else url.searchParams.delete('session');history.replaceState(null,'',url) }
  function chooseMachine(id: string) { selectedAgent=id;selectedSession=sessions.find(item=>item.agentId===id&&item.state==='running')?.id||'';requestId='';error='';remember() }
  function chooseSession(id: string) { selectedSession=id;remember() }
  async function refresh(signal?: AbortSignal) {
    if(refreshing||disposed)return
    refreshing=true
    try {
      const [nextMachines,nextSessions,nextHealth]=await Promise.all([terminalMachines(signal),terminalSessions(signal),machineHealth(signal).then(data=>({data,error:''})).catch(()=>({data:null,error:'Resource readings are temporarily unavailable. Existing readings are marked by their observation time.'}))])
      if(disposed)return
      machines=nextMachines;sessions=nextSessions;healthIssue=nextHealth.error
      if(nextHealth.data){health=nextHealth.data;receivedAt=performance.now();elapsed=0}
      if(!selectedAgent||!machines.some(item=>item.agentId===selectedAgent)){selectedAgent=machines[0]?.agentId||'';selectedSession=''}
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
  function openAction(kind:'rename'|'end') { if(!session)return;actionSession=session.id;rename=session.title;action=kind }
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
  onMount(()=>{
    const abort=new AbortController();void refresh(abort.signal)
    const poll=setInterval(()=>{if(document.visibilityState==='visible')void refresh(abort.signal)},15000)
    const clock=setInterval(()=>{elapsed=performance.now()-receivedAt},1000)
    const wake=()=>{if(document.visibilityState==='visible')void refresh(abort.signal)}
    document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake)
    return()=>{disposed=true;abort.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}
  })
</script>

<section class="workspace" aria-label="Remote terminal workspace">
  <div class="workspace-heading"><div><p class="eyebrow">Private workspace</p><h2>Your machine. Your shell.</h2><p>Run commands here. Close the browser and return to the same session.</p></div><span class="privacy"><span aria-hidden="true">◇</span> Private host connection</span></div>
  <TerminalCommandReference />
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if loading}<div class="empty" role="status">Loading your machines…</div>
  {:else if !machines.length}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h3>Connect your first machine</h3><p>Enroll a machine and start its background agent. You can then enable terminal access locally.</p><a class="btn" href="/machines#connect-machine">Connect a machine</a></div>
  {:else}
    <div class="machine-picker"><label>Machine<select aria-label="Machine" value={selectedAgent} onchange={(e)=>chooseMachine(e.currentTarget.value)}>{#each machines as item (item.agentId)}<option value={item.agentId}>{item.host}{item.ready?' · Ready':''}</option>{/each}</select></label><span class="machine-state" class:ready={machine?.ready}><span aria-hidden="true">●</span> {machine?.serverEnabled===false?'Server terminals disabled':machine?.ready?'Ready for terminals':machine?.capabilities?.terminalEnabled?'Reconnecting / needs setup':'Monitoring only'}</span><button class="btn quiet refresh" type="button" onclick={()=>void refresh()}>Refresh</button></div>
    {#if machine?.capabilities?.osUser}<div class="shell-user">Shell user <strong>{machine.capabilities.osUser}</strong><span>· {machine?.host}</span></div>{/if}
    {#if healthIssue}<p class="notice" role="status">{healthIssue}</p>{/if}
    <details class="health-panel" open={!session}><summary>Machine health <span>{machineStatus?.connection?.connected?'Connected':'Awaiting connection'} · RTT {!machineStatus?.connection?.connected||machineStatus.connection.rttMillis==null?'—':`${Math.round(machineStatus.connection.rttMillis)} ms`} · CPU {cpu===null?'—':`${cpu.toFixed(1)}%`}{cpu!==null&&!healthFresh?' · Last known':''}</span></summary><MachineHealthStrip health={machineStatus} {serverNow} />{#key selectedAgent}<HealthHistory agent={selectedAgent} />{/key}</details>
    {#if setup}<aside class="setup-notice"><div><strong>Terminal setup</strong><p>{setup}</p>{#if machine?.serverEnabled!==false&&!machine?.capabilities?.terminalEnabled}<p>Run on this machine:</p><pre>reet agent stop
reet agent terminal enable
reet agent service install</pre><CopyButton text={'reet agent stop\nreet agent terminal enable\nreet agent service install'} label="Copy local terminal setup" />{:else if machine?.capabilities?.readiness==='supervisor_unavailable'}<pre>reet agent stop
reet agent service install</pre><CopyButton text={'reet agent stop\nreet agent service install'} label="Copy service install command" />{/if}</div><span aria-hidden="true">↗</span></aside>{/if}
    <div class="session-bar"><label>Session<select aria-label="Session" value={selectedSession} onchange={(e)=>chooseSession(e.currentTarget.value)}><option value="">Choose a session</option>{#each ownSessions as item (item.id)}<option value={item.id}>{item.title} · {item.state.replaceAll('_',' ')}</option>{/each}</select></label><details class="new-session" class:initial={!session} open={!session||showCreate||!!requestId||machine?.serverEnabled===false} ontoggle={e=>showCreate=e.currentTarget.open}><summary>New session</summary><form onsubmit={(e)=>{e.preventDefault();void create()}}><label>New session name<input bind:value={title} disabled={busy||!!requestId} maxlength="80" required placeholder="Build, deploy, investigate…" /></label><button class="btn primary" disabled={busy||!machine?.ready||!validTerminalTitle(title.trim())||atQuota&&!requestId}>{busy?'Creating…':requestId?'Retry creation':'New terminal'}</button></form></details></div>
    <div class="session-actions"><span>{activeSessions.length} / 8 active on this machine{atQuota?' · Session limit reached':''}</span>{#if session}<button class="btn quiet" disabled={actionBusy||machine?.serverEnabled===false} onclick={()=>openAction('rename')}>Rename</button><button class="btn quiet" disabled={actionBusy||machine?.serverEnabled===false||!['creating','running','end_requested'].includes(session.state)} onclick={()=>openAction('end')}>{session.state==='end_requested'?'Retry end request':'End session'}</button>{/if}</div>
    {#if session?.state==='end_requested'}<p class="notice" role="status">End requested. The shell will stop when this machine reconnects. This session continues to count towards the limit until confirmed.</p>{/if}
    {#if session}{#key session.id}<TerminalPane {session} host={machine?.host||'machine'} />{/key}
    {:else}<div class="empty shell-empty"><span class="empty-icon" aria-hidden="true">›_</span><h3>A shell that stays with your work</h3><p>{machine?.ready?'Create a terminal explicitly to get started. Running sessions stay on the host until you end them or the shell exits.':'Finish local setup or wait for the machine to reconnect. Existing sessions are kept.'}</p></div>{/if}
  {/if}
</section>
{#if action==='rename'}<ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={()=>void performAction()} oncancel={()=>action=null}><label>Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>{:else if action==='end'}<ConfirmDialog title="End this session?" body="This stops the shell and its running jobs. Closing a tab or detaching keeps them running instead. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={()=>void performAction()} oncancel={()=>action=null} />{/if}

<style>
  .shell-user{display:flex;align-items:center;gap:8px;font-size:.72rem;color:var(--muted);margin-top:-10px}.shell-user strong{font-family:var(--mono,monospace);color:var(--ink);background:var(--tint);border-radius:5px;padding:3px 7px}.health-panel{border:1px solid var(--line);border-radius:12px;overflow:hidden;background:var(--card)}.health-panel>summary{padding:12px 16px;font-size:.78rem;cursor:pointer}.health-panel>summary span{float:right;color:var(--muted);font-size:.72rem}.health-panel :global(.health){border:0;border-top:1px solid var(--line);border-radius:0}.health-panel :global(.history){border:0;border-top:1px solid var(--line);border-radius:0}.new-session{min-width:0}.new-session>summary{padding:10px 14px;border:1px solid var(--line);border-radius:8px;cursor:pointer;font-size:.8rem}.new-session[open]>summary{margin-bottom:10px}.new-session.initial>summary{display:none}@media(max-width:700px){.health-panel>summary span{float:none;display:block;margin-top:5px}.new-session{width:100%}}
  .session-actions{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-top:-10px;font-size:.72rem;color:var(--muted)}.session-actions span{margin-right:auto}.session-actions button{font-size:.72rem;padding:6px 10px;min-height:34px}
  .workspace{min-width:0;max-width:100%;display:grid;gap:20px}.workspace-heading{display:flex;justify-content:space-between;align-items:center;gap:20px}.eyebrow{color:var(--accent);font-size:.65rem;letter-spacing:.12em;text-transform:uppercase;font-weight:700;margin:0 0 8px}h2{font:500 1.8rem/1.15 var(--serif);letter-spacing:-.025em;margin:0 0 8px}.workspace-heading p:last-child{font-size:.85rem;color:var(--muted);margin:0}.privacy{font-size:.7rem;color:var(--muted);display:flex;gap:6px;white-space:nowrap}.privacy>span{color:var(--good);font-size:1.1rem}.machine-picker,.session-bar{display:flex;gap:14px;align-items:end;flex-wrap:wrap}.machine-picker label{min-width:min(280px,100%)}label{display:grid;gap:6px;font-size:.7rem;font-weight:600;color:var(--muted);min-width:0}select,input{font-weight:400;font-size:.85rem;max-width:100%;min-width:0}select{min-width:200px}.machine-state{font-size:.75rem;color:var(--muted);align-self:center;padding-top:16px}.machine-state.ready{color:var(--good)}.refresh{margin-left:auto;font-size:.75rem}.session-bar{justify-content:space-between}.session-bar>label{flex:1;max-width:300px}.session-bar form{display:flex;align-items:end;gap:10px;min-width:0}.session-bar input{width:220px}.session-bar button{white-space:nowrap;font-size:.8rem}.empty{border:1px dashed var(--line);border-radius:14px;padding:44px 24px;text-align:center;background:var(--card);color:var(--muted)}.empty-icon{display:block;font:500 2rem monospace;color:var(--accent);margin-bottom:16px}.empty h3{font:500 1.3rem var(--serif);color:var(--ink);margin:0 0 10px}.empty p{max-width:60ch;font-size:.85rem;margin:0 auto 18px;line-height:1.6}.shell-empty{min-height:290px;display:grid;place-content:center}.setup-notice{display:flex;justify-content:space-between;gap:16px;border:1px solid var(--line);border-left:3px solid var(--accent);background:var(--card);padding:16px 20px;border-radius:10px;font-size:.8rem}.setup-notice p{margin:8px 0;color:var(--muted);line-height:1.5}.setup-notice pre{font-size:.78rem;white-space:pre-wrap;overflow-wrap:anywhere}.setup-notice>span{color:var(--accent);font-size:1.2rem}
  @media(max-width:700px){.workspace{gap:16px}.workspace-heading{align-items:flex-start}.privacy{display:none}h2{font-size:1.5rem}.session-bar{display:grid;grid-template-columns:minmax(0,1fr)}.session-bar>label{max-width:none}.session-bar form{display:grid;grid-template-columns:minmax(0,1fr) auto}.session-bar input{width:100%}.machine-picker{gap:10px}.machine-picker label{flex:1;min-width:0}.machine-picker select{min-width:0;width:100%}.machine-state{padding-top:18px;font-size:.65rem}.refresh{margin-left:0}.setup-notice{padding:14px}}
</style>
