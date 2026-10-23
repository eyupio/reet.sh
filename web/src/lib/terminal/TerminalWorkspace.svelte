<script lang="ts">
  import { onMount } from 'svelte'
  import CopyButton from '../CopyButton.svelte'
  import { createTerminal, machineHealth, readiness, terminalMachines, terminalSessions, type HealthResponse, type TerminalMachine, type TerminalSession } from './api'
  import MachineHealthStrip from './MachineHealthStrip.svelte'
  import TerminalPane from './TerminalPane.svelte'

  let machines = $state<TerminalMachine[]>([]), sessions = $state<TerminalSession[]>([])
  let selectedAgent = $state(new URLSearchParams(location.search).get('agent') || '')
  let selectedSession = $state(new URLSearchParams(location.search).get('session') || '')
  let health = $state<HealthResponse>({ serverNow:Date.now(),machines:[] })
  let receivedAt = performance.now(), elapsed = $state(0)
  let loading = $state(true), busy = $state(false), error = $state(''), title = $state('Shell')
  let requestId = $state('')
  let refreshing = false, disposed = false
  const machine = $derived(machines.find(item=>item.agentId===selectedAgent))
  const ownSessions = $derived(sessions.filter(item=>item.agentId===selectedAgent))
  const session = $derived(ownSessions.find(item=>item.id===selectedSession))
  const machineStatus = $derived(health.machines.find(item=>item.agentId===selectedAgent))
  const setup = $derived(readiness(machine))
  const serverNow = $derived(health.serverNow+elapsed)

  function remember() { const url=new URL(location.href);url.searchParams.set('view','terminal');if(selectedAgent)url.searchParams.set('agent',selectedAgent);else url.searchParams.delete('agent');if(selectedSession)url.searchParams.set('session',selectedSession);else url.searchParams.delete('session');history.replaceState(null,'',url) }
  function chooseMachine(id: string) { selectedAgent=id;selectedSession=sessions.find(item=>item.agentId===id&&item.state==='running')?.id||'';requestId='';error='';remember() }
  function chooseSession(id: string) { selectedSession=id;remember() }
  async function refresh(signal?: AbortSignal) {
    if(refreshing||disposed)return
    refreshing=true
    try {
      const [nextMachines,nextSessions,nextHealth]=await Promise.all([terminalMachines(signal),terminalSessions(signal),machineHealth(signal)])
      if(disposed)return
      machines=nextMachines;sessions=nextSessions;health=nextHealth;receivedAt=performance.now();elapsed=0
      if(!selectedAgent||!machines.some(item=>item.agentId===selectedAgent))selectedAgent=machines[0]?.agentId||''
      if(!selectedSession)selectedSession=sessions.find(item=>item.agentId===selectedAgent&&item.state==='running')?.id||''
      remember()
    } catch(e){if(!signal?.aborted&&!disposed)error=e instanceof Error?e.message:'Could not refresh machines.'}
    finally{refreshing=false;loading=false}
  }
  async function create() {
    if(busy||!machine?.ready||!title.trim())return
    busy=true;error=''
    // Keep this key on an uncertain response. Clicking retry inspects the
    // original operation rather than silently creating a replacement shell.
    requestId ||= crypto.randomUUID()
    try {const created=await createTerminal(selectedAgent,requestId,title.trim());sessions=[created,...sessions.filter(item=>item.id!==created.id)];selectedSession=created.id;requestId='';remember();await refresh()}
    catch(e){error=e instanceof Error?e.message:'Creation could not be confirmed. Retry with the same request.'}
    finally{busy=false}
  }
  onMount(()=>{
    const abort=new AbortController();void refresh(abort.signal)
    const poll=setInterval(()=>void refresh(abort.signal),15000)
    const clock=setInterval(()=>{elapsed=performance.now()-receivedAt},1000)
    const wake=()=>{if(document.visibilityState==='visible')void refresh(abort.signal)}
    document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake)
    return()=>{disposed=true;abort.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}
  })
</script>

<section class="workspace" aria-label="Remote terminal workspace">
  <div class="workspace-heading"><div><p class="eyebrow">Private workspace</p><h2>Your machine. Your shell.</h2><p>Run commands here. Close the browser and return to the same session.</p></div><span class="privacy"><span aria-hidden="true">◇</span> Private host connection</span></div>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if loading}<div class="empty" role="status">Loading your machines…</div>
  {:else if !machines.length}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h3>Connect your first machine</h3><p>Enroll a machine and start its background agent. You can then enable terminal access locally.</p><a class="btn" href="/machines#connect-machine">Connect a machine</a></div>
  {:else}
    <div class="machine-picker"><label>Machine<select value={selectedAgent} onchange={(e)=>chooseMachine(e.currentTarget.value)}>{#each machines as item (item.agentId)}<option value={item.agentId}>{item.host}{item.ready?' · Ready':''}</option>{/each}</select></label><span class="machine-state" class:ready={machine?.ready}><span aria-hidden="true">●</span> {machine?.ready?'Ready for terminals':machine?.capabilities?.terminalEnabled?'Reconnecting / needs setup':'Monitoring only'}</span><button class="btn quiet refresh" type="button" onclick={()=>void refresh()}>Refresh</button></div>
    <MachineHealthStrip health={machineStatus} {serverNow} />
    {#if setup}<aside class="setup-notice"><div><strong>Terminal setup</strong><p>{setup}</p>{#if machine?.capabilities?.osUser!=='root' && !machine?.capabilities?.terminalEnabled}<p>Run on this machine:</p><pre>reet agent terminal enable
reet agent service install</pre><CopyButton text={'reet agent terminal enable\nreet agent service install'} label="Copy local terminal setup" />{:else if machine?.capabilities?.readiness==='supervisor_unavailable'}<pre>reet agent service install</pre><CopyButton text="reet agent service install" label="Copy service install command" />{/if}</div><span aria-hidden="true">↗</span></aside>{/if}
    <div class="session-bar"><label>Session<select value={selectedSession} onchange={(e)=>chooseSession(e.currentTarget.value)}><option value="">Choose a session</option>{#each ownSessions as item (item.id)}<option value={item.id}>{item.title} · {item.state.replaceAll('_',' ')}</option>{/each}</select></label><form onsubmit={(e)=>{e.preventDefault();void create()}}><label>New session name<input bind:value={title} disabled={busy||!!requestId} maxlength="80" required placeholder="Build, deploy, investigate…" /></label><button class="btn primary" disabled={busy||!machine?.ready||!title.trim()}>{busy?'Creating…':requestId?'Retry creation':'New terminal'}</button></form></div>
    {#if session}{#key session.id}<TerminalPane {session} host={machine?.host||'machine'} />{/key}
    {:else}<div class="empty shell-empty"><span class="empty-icon" aria-hidden="true">›_</span><h3>A shell that stays with your work</h3><p>{machine?.ready?'Create a terminal explicitly to get started. Running sessions stay on the host until you end them or the shell exits.':'Finish local setup or wait for the machine to reconnect. Existing sessions are kept.'}</p></div>{/if}
  {/if}
</section>

<style>
  .workspace{min-width:0;max-width:100%;display:grid;gap:20px}.workspace-heading{display:flex;justify-content:space-between;align-items:center;gap:20px}.eyebrow{color:var(--accent);font-size:.65rem;letter-spacing:.12em;text-transform:uppercase;font-weight:700;margin:0 0 8px}h2{font:500 1.8rem/1.15 var(--serif);letter-spacing:-.025em;margin:0 0 8px}.workspace-heading p:last-child{font-size:.85rem;color:var(--muted);margin:0}.privacy{font-size:.7rem;color:var(--muted);display:flex;gap:6px;white-space:nowrap}.privacy>span{color:var(--good);font-size:1.1rem}.machine-picker,.session-bar{display:flex;gap:14px;align-items:end;flex-wrap:wrap}.machine-picker label{min-width:min(280px,100%)}label{display:grid;gap:6px;font-size:.7rem;font-weight:600;color:var(--muted);min-width:0}select,input{font-weight:400;font-size:.85rem;max-width:100%;min-width:0}select{min-width:200px}.machine-state{font-size:.75rem;color:var(--muted);align-self:center;padding-top:16px}.machine-state.ready{color:var(--good)}.refresh{margin-left:auto;font-size:.75rem}.session-bar{justify-content:space-between}.session-bar>label{flex:1;max-width:300px}.session-bar form{display:flex;align-items:end;gap:10px;min-width:0}.session-bar input{width:220px}.session-bar button{white-space:nowrap;font-size:.8rem}.empty{border:1px dashed var(--line);border-radius:14px;padding:44px 24px;text-align:center;background:var(--card);color:var(--muted)}.empty-icon{display:block;font:500 2rem monospace;color:var(--accent);margin-bottom:16px}.empty h3{font:500 1.3rem var(--serif);color:var(--ink);margin:0 0 10px}.empty p{max-width:60ch;font-size:.85rem;margin:0 auto 18px;line-height:1.6}.shell-empty{min-height:290px;display:grid;place-content:center}.setup-notice{display:flex;justify-content:space-between;gap:16px;border:1px solid var(--line);border-left:3px solid var(--accent);background:var(--card);padding:16px 20px;border-radius:10px;font-size:.8rem}.setup-notice p{margin:8px 0;color:var(--muted);line-height:1.5}.setup-notice pre{font-size:.78rem;white-space:pre-wrap;overflow-wrap:anywhere}.setup-notice>span{color:var(--accent);font-size:1.2rem}
  @media(max-width:700px){.workspace{gap:16px}.workspace-heading{align-items:flex-start}.privacy{display:none}h2{font-size:1.5rem}.session-bar{display:grid;grid-template-columns:minmax(0,1fr)}.session-bar>label{max-width:none}.session-bar form{display:grid;grid-template-columns:minmax(0,1fr) auto}.session-bar input{width:100%}.machine-picker{gap:10px}.machine-picker label{flex:1;min-width:0}.machine-picker select{min-width:0;width:100%}.machine-state{padding-top:18px;font-size:.65rem}.refresh{margin-left:0}.setup-notice{padding:14px}}
</style>
