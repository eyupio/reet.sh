<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { controlGafferGoal, getGafferGoal, listGafferGoals, listGafferOutcomes, stopGafferOutcome, type GafferGoalOverview, type GafferGoalOverviewView, type GafferOutcome as OutcomeRecord } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import GafferGoal from './GafferGoal.svelte'
  import GafferFloorReview from './GafferFloorReview.svelte'
  import GafferOutcome from './GafferOutcome.svelte'
  import { floorOutcomeLine, floorOutcomes } from './gafferOutcome'
  import { sessionHref, terminalSessions, type TerminalSession } from './terminal/api'
  import { floorCandidates, floorChosen, floorRoom } from './gafferFloor'
  import { toolName } from './agentLaunch'
  import GafferProse from './GafferProse.svelte'
  import { formatWhen } from './time'

  let { onclose, onchanged, onselect, onhistory, onerror, userId=0, timeZone='', dateFormat='locale' }: {
    onclose:()=>void; onchanged:()=>void|Promise<void>; onselect:()=>void; onhistory:()=>void; userId?:number;
    onerror:(e:unknown)=>string; timeZone?:string; dateFormat?:string
  } = $props()
  let view=$state<GafferGoalOverviewView|null>(null), loading=$state(true), error=$state('')
  let filter=$state('all'), managing=$state<GafferGoalOverview|null>(null)
  let reading=$state(false)
  // The rest of the floor: running chats nobody has asked Gaffer to look after yet. Reading them starts nothing.
  let sessions=$state<TerminalSession[]>([]), sessionsError=$state(''), ticked=$state<Set<string>>(new Set()), reviewing=$state(false)
  let lookAfterButton=$state<HTMLButtonElement>()
  // An outcome is an objective Gaffer owns across sessions; its sessions' goals name it, which is how they group here.
  let outcomeOpen=$state(false), outcomeFilter=$state(''), outcomeButton=$state<HTMLButtonElement>()
  let records=$state<OutcomeRecord[]>([])
  // One action for the sessions shown: pause or stop every followed one in the current view.
  let bulkBusy=$state(false), bulkNotice=$state(''), confirmStop=$state(false)
  let refreshButton=$state<HTMLButtonElement>()
  let controller:AbortController|undefined, epoch=0, returnFocus:HTMLElement|undefined
  const rows=$derived(view?.goals??[])
  const needsYou=$derived(rows.filter(row=>row.attention).length)
  const following=$derived(rows.filter(row=>row.goal.state==='active'&&!row.attention).length)
  const candidates=$derived(view?floorCandidates(sessions,rows.map(row=>row.goal)):[])
  const chosen=$derived(floorChosen(candidates,ticked))
  const room=$derived(floorRoom(rows.map(row=>row.goal)))
  const showCandidates=$derived(!outcomeFilter&&(filter==='all'||filter==='idle'))
  const outcomes=$derived(floorOutcomes(records,rows))
  const chosenOutcome=$derived(outcomes.find(group=>group.id===outcomeFilter))
  const visible=$derived(rows.filter(row=>!chosenOutcome||chosenOutcome.rows.includes(row)).filter(row=>filter==='all'||filter==='attention'&&row.attention||filter==='active'&&row.goal.state==='active'&&!row.attention||filter==='finished'&&['completed','stopped','limit'].includes(row.goal.state)))
  const label=(row:GafferGoalOverview)=>row.attention?'Needs review':row.goal.state==='active'?'Following':row.goal.state==='completed'?'You marked complete':row.goal.state==='limit'?'Limit reached':row.goal.state==='stopped'?'Stopped':'Paused'
  const deliveryLabel=(state:string)=>state==='sent'?'Handed to worker':state==='queued'?'Queued for next turn':state==='sending'?'Handoff in progress':state==='uncertain'?'Delivery uncertain':state==='blocked'?'Delivery blocked':state==='removed'?'Instruction removed':'Receipt needs review'

  async function load(fresh=false) {
    if(reading&&!fresh)return
    reading=true;controller?.abort();controller=new AbortController()
    const current=controller,mine=++epoch
    try {
      const [next,owned,found]=await Promise.all([
        listGafferGoals(current.signal),
        // Outcome records are an addition: a controller without them, or a failed read, leaves the floor as it was.
        listGafferOutcomes(current.signal).catch(()=>null),
        terminalSessions(current.signal).then(list=>({list,error:''}),e=>({list:null,error:e instanceof Error?e.message:'Running sessions could not be read.'})),
      ])
      if(mine!==epoch)return
      view=next;error=''
      if(found.list)sessions=found.list
      if(owned)records=owned
      sessionsError=current.signal.aborted?'':found.error
    } catch(e) { if(mine===epoch&&!current.signal.aborted)error=onerror(e) }
    finally { if(mine===epoch){reading=false;loading=false} }
  }
  onMount(()=>{
    void load()
    const timer=setInterval(()=>{if(!managing&&!reviewing&&!outcomeOpen&&!document.hidden)void load()},10000)
    return()=>{epoch++;controller?.abort();clearInterval(timer)}
  })
  function openSession(event:MouseEvent,row:GafferGoalOverview) {
    if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey||!row.session)return
    const request=new CustomEvent('reet:show-session',{detail:row.session.id,cancelable:true})
    if(!window.dispatchEvent(request)){event.preventDefault();onselect()}
    else onclose()
  }
  function tick_(id:string,on:boolean){const next=new Set(ticked);if(on)next.add(id);else next.delete(id);ticked=next}
  function tickAll(){ticked=chosen.length===candidates.length?new Set():new Set(candidates.map(item=>item.id))}
  async function closeReview(){reviewing=false;await load(true);await tick();(lookAfterButton?.isConnected?lookAfterButton:refreshButton)?.focus({preventScroll:true})}
  // Opening a running chat from the floor shows it with its own Gaffer conversation. It enables and sends nothing.
  function openCandidate(event:MouseEvent,item:TerminalSession) {
    if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return
    const request=new CustomEvent('reet:show-session',{detail:item.id,cancelable:true})
    if(!window.dispatchEvent(request)){event.preventDefault();onselect()}
    else onclose()
  }
  async function closeOutcome(){outcomeOpen=false;await load(true);await tick();(outcomeButton?.isConnected?outcomeButton:refreshButton)?.focus({preventScroll:true})}
  $effect(()=>{if(outcomeFilter&&!outcomes.some(group=>group.id===outcomeFilter))outcomeFilter=''})
  const pausable=$derived(visible.filter(row=>row.goal.state==='active'))
  const stoppable=$derived(visible.filter(row=>!['completed','stopped'].includes(row.goal.state)))
  async function bulk(action:'pause'|'stop',targets:GafferGoalOverview[]) {
    if(bulkBusy||!targets.length)return
    bulkBusy=true;bulkNotice='';confirmStop=false
    let done=0;const failed:string[]=[]
    // One at a time, each against a fresh read of its goal, exactly as its own card would.
    for(const row of targets){
      try{
        const fresh=await getGafferGoal(row.goal.conversationId)
        if(!fresh.goal)continue
        await controlGafferGoal(row.goal.conversationId,fresh.goal.revision,action,fresh.pins,false)
        done++
      }catch(e){failed.push(`${row.session?.title||'A session'}: ${e instanceof Error?e.message:'not changed'}`)}
    }
    const verb=action==='pause'?'Paused':'Stopped'
    bulkNotice=`${verb} ${done} of ${targets.length}. The working models keep running.${failed.length?` Not changed: ${failed.join('; ')}`:''}`
    bulkBusy=false
    await load(true);await onchanged()
  }
  async function stopOutcome(record:OutcomeRecord) {
    if(bulkBusy)return
    bulkBusy=true;bulkNotice=''
    try{await stopGafferOutcome(record);bulkNotice='Stopped. Sessions not yet started stay unstarted; the ones already started keep running.'}
    catch(e){bulkNotice=e instanceof Error?e.message:'The outcome was not stopped.'}
    bulkBusy=false
    await load(true)
  }
  async function closeGoal(){managing=null;await load(true);await tick();(returnFocus?.isConnected?returnFocus:refreshButton)?.focus({preventScroll:true})}
</script>

<WorkspaceDialog title="Factory floor" {onclose}>
  <div class="overview">
    <p class="intro">The whole floor on one job sheet, with a brew on standby: the sessions you asked Gaffer to look after, and the running chats you have not.</p>
    <div class="own"><button class="btn primary" type="button" bind:this={outcomeButton} onclick={()=>outcomeOpen=true}>Give Gaffer an outcome</button><p class="muted">Gaffer plans the sessions, you approve, and it follows each one to the finish.</p></div>
    {#if outcomes.length}<section class="outcomes" aria-label="Outcomes">
      <h3>Outcomes</h3>
      <ul class="outcome-list">{#each outcomes as group (group.id)}<li><button class="outcome" type="button" aria-pressed={outcomeFilter===group.id} onclick={()=>outcomeFilter=outcomeFilter===group.id?'':group.id}><strong>{group.objective}</strong><span>{floorOutcomeLine(group)}</span></button></li>{/each}</ul>
      {#if chosenOutcome}
        {#if chosenOutcome.record}<ul class="plan-state" aria-label="Planned sessions of this outcome">{#each chosenOutcome.record.sessions as item (item.key)}<li><strong>{item.title}</strong><span>{item.note}</span></li>{/each}</ul>{/if}
        <p class="muted">Showing this outcome’s sessions. <button class="clear" type="button" onclick={()=>outcomeFilter=''}>Show everything</button>{#if chosenOutcome.record?.state==='starting'}{@const record=chosenOutcome.record}{' '}<button class="clear" type="button" disabled={bulkBusy} onclick={()=>{const live=records.find(item=>item.id===record.id);if(live)void stopOutcome(live)}}>Stop starting sessions</button>{/if}</p>
      {/if}
    </section>{/if}
    <div class="toolbar"><nav class="filters" aria-label="Filter the floor">
      {#each [['all','All',-1],['attention','Needs review',needsYou],['active','Following',following],['idle','Not followed',candidates.length],['finished','Finished',-1]] as [value,name,count] (value)}<button class="filter" type="button" aria-pressed={filter===value} onclick={()=>filter=String(value)}>{name}{#if Number(count)>0}{' '}<strong>{count}</strong>{/if}</button>{/each}
    </nav><button class="btn quiet" bind:this={refreshButton} disabled={reading} onclick={()=>void load()}>Refresh</button></div>
    {#if pausable.length>1||stoppable.length>1}<div class="bulk" role="group" aria-label="Actions for the sessions shown">
      {#if pausable.length>1}<button class="btn quiet" type="button" disabled={bulkBusy} onclick={()=>void bulk('pause',pausable)}>Pause all {pausable.length} following</button>{/if}
      {#if stoppable.length>1}<button class="btn quiet" type="button" disabled={bulkBusy} onclick={()=>confirmStop=true}>Stop all {stoppable.length} goals</button>{/if}
    </div>{/if}
    {#if bulkNotice}<p role="status">{bulkNotice}</p>{/if}
    {#if error}<p role="alert" class="warning">{error}</p>{#if view}<p class="muted">Showing the last successful read. Refresh to check for changes.</p>{/if}{/if}
    {#if loading}<p role="status">Reading the job sheet…</p>
    {:else if !rows.length&&!candidates.length&&!error}<div class="empty"><h3>Nowt on the job sheet yet.</h3><p>Start an agent in Chat and it appears here, ready to be looked after. Opening this view starts no follow-up.</p>{#if sessionsError}<p class="warning">{sessionsError}</p>{/if}</div>
    {:else if !visible.length&&!(showCandidates&&candidates.length)&&!error}<p>{filter==='idle'?'Every running chat is already on the job sheet.':'No followed sessions in this view.'}</p>
    {:else if visible.length}<ul class="goals" aria-label="Followed session goals">
      {#each visible as row (row.goal.conversationId)}{@const g=row.goal}<li class:attention={row.attention}>
        <div class="card-head"><h3>{row.session?.title||`Session · ${g.sessionId.slice(-8)}`}</h3><span class="state">{label(row)}</span></div>
        <p class="goal-text">{g.text}</p>
        {#if row.reason}<p class="warning">{row.reason}</p>{/if}
        {#if g.note}<p class="muted">{g.note}</p>{/if}
        {#if g.summary}<div class="review"><p class="eyebrow">Gaffer reports</p><p class="report">{g.summary.length>280?g.summary.slice(0,280)+'…':g.summary}</p></div>{:else}<p class="muted">No completed Gaffer review yet.</p>{/if}
        <p class="evidence">{g.usedReviews} / {g.maxReviews} reviews used · Until {formatWhen(g.expiresAt,timeZone,dateFormat)}</p>
        {#if g.lastObservedAt}<p class="evidence">Worker evidence observed {formatWhen(g.lastObservedAt,timeZone,dateFormat)} · Event #{g.lastSeq}</p>{/if}
        {#if g.lastReviewedAt}<p class="evidence">Review finished {formatWhen(g.lastReviewedAt,timeZone,dateFormat)}</p>{/if}
        {#if g.activeTurnId}<p class="muted" role="status">Gaffer review in progress.</p>{/if}
        {#if row.delivery}<p class="delivery">{deliveryLabel(row.delivery.state)}</p>{:else if g.pendingId}<p class="warning">Instruction delivery is unavailable in this view.</p>{/if}
        <details><summary>Review &amp; evidence</summary><div class="detail-body">
          {#if g.summary}<h4>Latest Gaffer review</h4><GafferProse text={g.summary}/>{/if}
          <p class="muted">Review model: {g.model}. Gaffer reports progress and remaining work from retained session evidence. PRs, checks and completion need verification in the working conversation.</p>
          {#if row.delivery}<h4>Last goal instruction</h4><p class="instruction">{row.delivery.text}</p><p>{deliveryLabel(row.delivery.state)}{row.delivery.note?` · ${row.delivery.note}`:''}</p><p class="muted">Handoff records delivery to the worker, not completion of the work.</p>{/if}
          <p class="muted">{g.usageReported?`${g.inputTokens} input · ${g.outputTokens} output tokens reported`:'Provider usage is not fully reported.'}</p>
          {#if row.session}<p class="muted">Retained session state: {row.session.state}. Worker availability is checked when dispatching.</p>{/if}
        </div></details>
        <div class="actions">
          {#if row.available&&row.session}<a class="btn" href={`/sessions?agent=${encodeURIComponent(row.session.agentId)}&session=${encodeURIComponent(row.session.id)}&gaffer=1`} onclick={event=>openSession(event,row)}>Open session</a><button class="btn quiet" onclick={event=>{returnFocus=event.currentTarget;managing=row}}>Review goal</button>
          {:else}<button class="btn quiet" onclick={onhistory}>Conversation history</button>{/if}
        </div>
      </li>{/each}
    </ul>{/if}
    {#if !loading&&showCandidates&&(candidates.length||sessionsError)}<section class="idle" aria-label="Not followed">
      <div class="idle-head"><h3>Not followed</h3>{#if candidates.length>1}<button class="btn quiet" type="button" onclick={tickAll}>{chosen.length===candidates.length?'Clear selection':'Select all'}</button>{/if}</div>
      {#if sessionsError}<p class="warning">{sessionsError}</p>{/if}
      {#if candidates.length}
        <p class="muted">Tick the running chats Gaffer should look after. Ticking shares nothing and starts nothing; one review follows.</p>
        <div class="picks" role="group" aria-label="Running chats not followed">
          {#each candidates as item (item.id)}<div class="pick-row"><label class="pick"><input type="checkbox" checked={ticked.has(item.id)} onchange={event=>tick_(item.id,event.currentTarget.checked)}/><span class="pick-text"><strong>{item.title}</strong><span>{toolName(item.tool??'')||'Agent'} · {item.dir||'Directory not reported'}</span></span></label><a class="pick-open" href={`${sessionHref(item)}&gaffer=1`} aria-label={`Open ${item.title}`} onclick={event=>openCandidate(event,item)}>Open</a></div>{/each}
        </div>
        {#if chosen.length>room}<p class="warning" role="status">{room?`Gaffer has room for ${room} more active ${room===1?'goal':'goals'}. Untick ${chosen.length-room}.`:'Gaffer is already following ten sessions. Pause or stop one first.'}</p>{/if}
        <div class="actions"><button class="btn primary" type="button" bind:this={lookAfterButton} disabled={!chosen.length||chosen.length>room} onclick={()=>reviewing=true}>{chosen.length?`Look after ${chosen.length} selected`:'Look after selected'}</button></div>
      {/if}
    </section>{/if}
    {#if view}<p class="evidence">Overview read {formatWhen(view.readAt,timeZone,dateFormat)}. Evidence times above show what each review actually saw. Refreshing makes no model request.</p>{/if}
  </div>
</WorkspaceDialog>
{#if confirmStop}<ConfirmDialog title={`Stop ${stoppable.length} goals?`} body="This ends Gaffer follow-up for every session shown and removes goal instructions still queued. The working models keep running. Resuming is done one session at a time, after you have checked it." confirmLabel="Stop goals" danger oncancel={()=>confirmStop=false} onconfirm={()=>void bulk('stop',stoppable)}/>{/if}
{#if outcomeOpen}<GafferOutcome {room} {userId} onstarted={onchanged} onclose={()=>void closeOutcome()}/>{/if}
{#if reviewing}<GafferFloorReview sessions={chosen} {room} {userId} onstarted={onchanged} onclose={()=>void closeReview()}/>{/if}
{#if managing}<WorkspaceDialog title="Look after this session" onclose={()=>void closeGoal()}><GafferGoal id={managing.goal.conversationId} {userId} {timeZone} {dateFormat} onchanged={async()=>{await load(true);await onchanged()}}/></WorkspaceDialog>{/if}

<style>
  .overview{display:grid;gap:16px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.intro,.muted,.evidence{color:var(--muted)}.toolbar,.card-head,.actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.toolbar{align-items:flex-start}.filter strong{font-variant-numeric:tabular-nums}.filters{display:flex;gap:6px;flex-wrap:wrap}.filter{font:inherit;color:var(--ink);border:1px solid var(--line);background:var(--bg);border-radius:8px;padding:7px 12px;min-height:44px;cursor:pointer}.filter[aria-pressed=true]{background:var(--tint);border-color:var(--accent)}.goals{list-style:none;padding:0;margin:0;display:grid;gap:16px}.goals>li{display:grid;gap:10px;min-width:0;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--bg)}.goals>li.attention{border-color:var(--accent)}h3{font:600 21px var(--serif);margin:0;overflow-wrap:anywhere;min-width:0}h4{font-size:14px;margin:0}.card-head{align-items:flex-start;flex-wrap:wrap}.state{font-size:0.875rem;border:1px solid var(--line);border-radius:6px;padding:2px 8px;white-space:nowrap}.attention .state{color:var(--accent)}.goal-text,.instruction{white-space:pre-wrap;overflow-wrap:anywhere}.review{border-left:2px solid var(--line-strong);padding-left:12px}.eyebrow{font-size:0.875rem;color:var(--muted);margin-bottom:4px}.report{white-space:pre-wrap}.evidence{font-size:0.875rem;font-variant-numeric:tabular-nums}.warning{color:var(--bad)}.delivery{font-weight:600}details{border-top:1px solid var(--line);padding-top:6px}summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;color:var(--muted)}.detail-body{display:grid;gap:10px;padding:8px 0}.actions{justify-content:flex-start;flex-wrap:wrap}.actions a{text-decoration:none;box-sizing:border-box}.btn{min-height:44px}button:focus-visible,a:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:3px}.own{display:grid;gap:6px;justify-items:start}.outcomes{display:grid;gap:10px;min-width:0}.outcome-list{list-style:none;margin:0;padding:0;display:grid;gap:8px}.outcome{display:grid;gap:2px;width:100%;text-align:left;font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:12px;padding:10px 14px;min-height:44px;cursor:pointer;overflow-wrap:anywhere}.outcome[aria-pressed=true]{background:var(--tint);border-color:var(--accent)}.outcome span{font-size:0.875rem;color:var(--muted);font-variant-numeric:tabular-nums}.clear{border:0;background:none;padding:0;min-height:44px;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer;text-decoration:underline;text-underline-offset:3px}.plan-state{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.plan-state li{display:grid;gap:2px;padding:8px 14px;overflow-wrap:anywhere}.plan-state li+li{border-top:1px solid var(--line)}.plan-state span{font-size:0.875rem;color:var(--muted)}.bulk{display:flex;flex-wrap:wrap;gap:8px}.clear:disabled{opacity:.5;cursor:default}.idle{display:grid;gap:12px;min-width:0;border-top:1px solid var(--line);padding-top:16px}.idle-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.picks{display:grid;border:1px solid var(--line);border-radius:12px}.pick{display:flex;align-items:flex-start;gap:12px;min-height:44px;padding:12px 14px;cursor:pointer;min-width:0}.pick{flex:1 1 auto}.pick-row{display:flex;align-items:center;gap:8px;min-width:0;padding-right:10px}.pick-row+.pick-row{border-top:1px solid var(--line)}.pick-open{flex:none;min-height:44px;min-width:44px;display:inline-flex;align-items:center;justify-content:center;color:var(--accent-text);font-weight:600;text-underline-offset:3px}.pick input{width:20px;height:20px;margin:2px 0 0;flex:none;accent-color:var(--accent)}.pick-text{display:grid;gap:2px;min-width:0;overflow-wrap:anywhere}.pick-text span{font-size:0.875rem;color:var(--muted)}.pick:has(input:focus-visible){outline:2px solid var(--accent);outline-offset:-2px;border-radius:12px}.empty{padding:16px 0}.empty p{margin-top:10px;color:var(--muted)}@media(max-width:559px){.goals>li{padding:12px}.toolbar{gap:8px}.filter{padding:7px 10px}}
</style>
