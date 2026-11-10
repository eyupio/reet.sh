<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { controlGafferGoal, getGafferGoal, listGafferGoals, listGafferOutcomes, stopGafferOutcome, type GafferGoalOverview, type GafferGoalOverviewView, type GafferOutcome as OutcomeRecord } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import GafferGoal from './GafferGoal.svelte'
  import GafferFloorReview from './GafferFloorReview.svelte'
  import GafferOutcome from './GafferOutcome.svelte'
  import { floorOutcomeLine, floorOutcomes, outcomeOf, outcomePart } from './gafferOutcome'
  import GafferCompanionAvatar from './GafferCompanionAvatar.svelte'
  import { gafferStanding } from './gafferMotion'
  import { sessionHref, terminalSessions, type TerminalSession } from './terminal/api'
  import { floorCandidates, floorChosen, floorRoom } from './gafferFloor'
  import { toolName } from './agentLaunch'
  import GafferProse from './GafferProse.svelte'
  import { formatWhen } from './time'

  let { onclose, onchanged, onselect, onhistory, onerror, userId=0, timeZone='', dateFormat='locale', startWithOutcome=false, inline=false, hidden=[], hiddenSessions=[] }: {
    startWithOutcome?:boolean;
    /** On the Factory page the floor is part of the page, not a dialog, and the page shows outcomes itself. `hidden` are goals the page already draws as pipelines. */
    inline?:boolean; hidden?:string[]; hiddenSessions?:string[];
    onclose:()=>void; onchanged:()=>void|Promise<void>; onselect:()=>void; onhistory:()=>void; userId?:number;
    onerror:(e:unknown)=>string; timeZone?:string; dateFormat?:string
  } = $props()
  let view=$state<GafferGoalOverviewView|null>(null), loading=$state(true), error=$state('')
  let filter=$state('all'), managing=$state<GafferGoalOverview|null>(null)
  let reading=$state(false)
  // The rest of the floor: running chats nobody has asked Gaffer to look after yet. Reading them starts nothing.
  let sessions=$state<TerminalSession[]>([]), sessionsError=$state(''), ticked=$state<Set<string>>(new Set()), reviewing=$state(false)
  let lookAfterButton=$state<HTMLButtonElement>()
  // With looking-after pre-approved, the button starts at once; "Review first" still shows the goal and limits beforehand.
  let quick=$state(false)
  // An outcome is an objective Gaffer owns across sessions; its sessions' goals name it, which is how they group here.
  let outcomeOpen=$state(false), outcomeFilter=$state(''), outcomeButton=$state<HTMLButtonElement>()
  let records=$state<OutcomeRecord[]>([])
  // One action for the sessions shown: pause or stop every followed one in the current view.
  let bulkBusy=$state(false), bulkNotice=$state(''), confirmStop=$state(false)
  let refreshButton=$state<HTMLButtonElement>()
  let controller:AbortController|undefined, epoch=0, returnFocus:HTMLElement|undefined
  const rows=$derived((view?.goals??[]).filter(row=>!hidden.includes(row.goal.conversationId)))
  const needsYou=$derived(rows.filter(row=>row.attention).length)
  const following=$derived(rows.filter(row=>row.goal.state==='active'&&!row.attention).length)
  // A session is offered only when it has no goal at all and is not a pipeline's own session.
  const candidates=$derived(view?floorCandidates(sessions,view.goals.map(row=>row.goal)).filter(item=>!hiddenSessions.includes(item.id)):[])
  const chosen=$derived(floorChosen(candidates,ticked))
  const room=$derived(floorRoom(rows.map(row=>row.goal)))
  const showCandidates=$derived(!outcomeFilter&&(filter==='all'||filter==='idle'))
  const outcomes=$derived(floorOutcomes(records,rows))
  const chosenOutcome=$derived(outcomes.find(group=>group.id===outcomeFilter))
  const visible=$derived(rows.filter(row=>!chosenOutcome||chosenOutcome.rows.includes(row)).filter(row=>filter==='all'||filter==='attention'&&row.attention||filter==='active'&&row.goal.state==='active'&&!row.attention||filter==='finished'&&['completed','stopped','limit'].includes(row.goal.state)))
  const label=(row:GafferGoalOverview)=>row.attention?'Needs you':row.goal.state==='active'?'Following':row.goal.state==='completed'?'You marked complete':row.goal.state==='limit'?'Limit reached':row.goal.state==='stopped'?'Stopped':'Paused'
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
    // Arriving to give Gaffer an outcome opens that straight away, over the floor it will appear on.
    if(startWithOutcome)outcomeOpen=true
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

{#snippet sessionsPart()}
    <section class="sessions" aria-label="Sessions">
      <div class="section-head"><h3 class:sr-only={inline}>Sessions</h3>
        {#if pausable.length>1||stoppable.length>1}<div class="bulk" role="group" aria-label="Actions for the sessions shown">
          {#if pausable.length>1}<button class="btn quiet" type="button" disabled={bulkBusy} onclick={()=>void bulk('pause',pausable)}>Pause all {pausable.length} following</button>{/if}
          {#if stoppable.length>1}<button class="btn quiet" type="button" disabled={bulkBusy} onclick={()=>confirmStop=true}>Stop all {stoppable.length} goals</button>{/if}
        </div>{/if}
        <button class="btn quiet refresh" bind:this={refreshButton} disabled={reading} onclick={()=>void load()}>Refresh</button>
      </div>
      <nav class="filters" aria-label="Filter the floor">
        {#each [['all','All',-1],['attention','Needs you',needsYou],['active','Following',following],['idle','Not looked after',candidates.length],['finished','Finished',-1]] as [value,name,count] (value)}<button class="filter" type="button" aria-pressed={filter===value} onclick={()=>filter=String(value)}>{name}{#if Number(count)>0}{' '}<strong>{count}</strong>{/if}</button>{/each}
      </nav>
    {#if bulkNotice}<p role="status">{bulkNotice}</p>{/if}
    {#if error}<p role="alert" class="warning">{error}</p>{#if view}<p class="muted">Showing the last successful read. Refresh to check for changes.</p>{/if}{/if}
    {#if loading}<p role="status">Reading the job sheet…</p>
    {:else if !rows.length&&!candidates.length&&!error}<div class="empty"><h3>Nowt on the job sheet yet.</h3><p>Give Gaffer an outcome above, or start an agent in Chat and it appears here, ready to be looked after. Opening this view starts nothing.</p>{#if sessionsError}<p class="warning">{sessionsError}</p>{/if}</div>
    {:else if !visible.length&&!(showCandidates&&candidates.length)&&!error}<p>{filter==='idle'?'Every running chat is already on the job sheet.':'No sessions here that Gaffer looks after.'}</p>
    {:else if visible.length}<ul class="goals" aria-label="Sessions Gaffer looks after">
      {#each visible as row (row.goal.conversationId)}{@const g=row.goal}{@const owned=outcomeOf(g.text)}{@const part=outcomePart(g.text)}{@const title=row.session?.title||`Session · ${g.sessionId.slice(-8)}`}<li class:attention={row.attention}>
        <div class="card-head"><h3>{title}</h3><span class="state">{label(row)}</span></div>
        {#if owned}<p class="belongs"><span class="eyebrow-inline">Outcome</span> {owned}{#if part&&part!==title}<span class="part"> · {part}</span>{/if}</p>
        {:else}<p class="goal-text clamp">{g.text}</p>{/if}
        {#if row.reason}<p class="callout">{row.reason}{#if g.note}{' '}{g.note}{/if}</p>{/if}
        {#if g.summary}<div class="review"><p class="eyebrow">Gaffer reports</p><p class="report clamp">{g.summary.length>280?g.summary.slice(0,280)+'…':g.summary}</p></div>{:else}<p class="muted">{g.note||'No completed Gaffer review yet.'}</p>{/if}
        {#if g.activeTurnId}<p class="muted" role="status">Gaffer review in progress.</p>{/if}
        {#if row.delivery}<p class="delivery">{deliveryLabel(row.delivery.state)}</p>{:else if g.pendingId}<p class="warning">Instruction delivery is unavailable in this view.</p>{/if}
        <p class="evidence">{g.usedReviews} of {g.maxReviews} reviews used · until {formatWhen(g.expiresAt,timeZone,dateFormat)}</p>
        <details><summary>Review &amp; evidence</summary><div class="detail-body">
          <h4>Agreed goal</h4><p class="goal-text">{g.text}</p>
          {#if g.note&&g.summary&&!row.reason}<p class="muted">{g.note}</p>{/if}
          {#if g.summary}<h4>Latest Gaffer review</h4><GafferProse text={g.summary}/>{/if}
          {#if g.lastObservedAt}<p class="evidence">Worker evidence observed {formatWhen(g.lastObservedAt,timeZone,dateFormat)} · Event #{g.lastSeq}</p>{/if}
          {#if g.lastReviewedAt}<p class="evidence">Review finished {formatWhen(g.lastReviewedAt,timeZone,dateFormat)}</p>{/if}
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
    </section>
    {#if !loading&&showCandidates&&(candidates.length||sessionsError)}<section class="idle" aria-label="Not looked after">
      <div class="section-head"><h3>Not looked after</h3>{#if candidates.length>1}<button class="btn quiet" type="button" onclick={tickAll}>{chosen.length===candidates.length?'Clear selection':'Select all'}</button>{/if}</div>
      {#if sessionsError}<p class="warning">{sessionsError}</p>{/if}
      {#if candidates.length}
        <p class="muted">Tick the running chats Gaffer should look after. Ticking shares nothing and starts nothing; one review follows.</p>
        <div class="picks" role="group" aria-label="Running chats not followed">
          {#each candidates as item (item.id)}<div class="pick-row"><label class="pick"><input type="checkbox" checked={ticked.has(item.id)} onchange={event=>tick_(item.id,event.currentTarget.checked)}/><span class="pick-text"><strong>{item.title}</strong><span>{toolName(item.tool??'')||'Agent'} · {item.dir||'Directory not reported'}</span></span></label><a class="pick-open" href={`${sessionHref(item)}&gaffer=1`} aria-label={`Open ${item.title}`} onclick={event=>openCandidate(event,item)}>Open</a></div>{/each}
        </div>
        {#if chosen.length>room}<p class="warning" role="status">{room?`Gaffer has room for ${room} more active ${room===1?'goal':'goals'}. Untick ${chosen.length-room}.`:'Gaffer is already following ten sessions. Pause or stop one first.'}</p>{/if}
        <div class="actions"><button class="btn primary" type="button" bind:this={lookAfterButton} disabled={!chosen.length||chosen.length>room} onclick={()=>{quick=$gafferStanding;reviewing=true}}>{chosen.length?`Look after ${chosen.length} selected`:'Look after selected'}</button>{#if $gafferStanding}<button class="btn quiet" type="button" disabled={!chosen.length||chosen.length>room} onclick={()=>{quick=false;reviewing=true}}>Review first</button>{/if}</div>
        {#if $gafferStanding}<p class="muted">Starts at once with your usual goal and limits. You pre-approved this in Gaffer settings.</p>{/if}
      {/if}
    </section>{/if}
    {#if view&&!inline}<p class="evidence foot">Read {formatWhen(view.readAt,timeZone,dateFormat)}. Reading the floor makes no model request and starts nothing.</p>{/if}
{/snippet}
{#if inline}
  <div class="overview" data-floor>{@render sessionsPart()}</div>
{:else}
<WorkspaceDialog title="Factory floor" wide {onclose}>
  <div class="overview" data-floor>
    <div class="own">
      <GafferCompanionAvatar size={48}/>
      <div class="own-copy"><p class="own-title">Hand Gaffer the outcome, not the chores.</p><p class="muted">Gaffer plans the sessions, you approve, and it follows each one to the finish.</p></div>
      <button class="btn primary" type="button" bind:this={outcomeButton} onclick={()=>outcomeOpen=true}>Give Gaffer an outcome</button>
    </div>
    {#if outcomes.length}<section class="outcomes" aria-label="Outcomes">
      <h3>Outcomes</h3>
      <ul class="outcome-list">{#each outcomes as group (group.id)}{@const open=outcomeFilter===group.id}<li class:open>
        <button class="outcome" type="button" aria-pressed={open} onclick={()=>outcomeFilter=open?'':group.id}><strong>{group.objective}</strong><span>{floorOutcomeLine(group)}</span></button>
        {#if open}<div class="outcome-detail">
          {#if group.record}<ul class="plan-state" aria-label="Planned sessions of this outcome">{#each group.record.sessions as item (item.key)}<li class:bad={['failed','cancelled','unfollowed'].includes(item.state)}><strong>{item.title}</strong><span>{item.note}</span></li>{/each}</ul>{/if}
          <p class="muted">Showing this outcome’s sessions below. <button class="clear" type="button" onclick={()=>outcomeFilter=''}>Show everything</button>{#if group.record?.state==='starting'}{@const record=group.record}{' '}<button class="clear" type="button" disabled={bulkBusy} onclick={()=>{const live=records.find(item=>item.id===record.id);if(live)void stopOutcome(live)}}>Stop starting sessions</button>{/if}</p>
        </div>{/if}
      </li>{/each}</ul>
    </section>{/if}
{@render sessionsPart()}
  </div>
</WorkspaceDialog>
{/if}
{#if confirmStop}<ConfirmDialog title={`Stop ${stoppable.length} goals?`} body="This ends Gaffer looking after for every session shown and removes goal instructions still queued. The working models keep running. Resuming is done one session at a time, after you have checked it." confirmLabel="Stop goals" danger oncancel={()=>confirmStop=false} onconfirm={()=>void bulk('stop',stoppable)}/>{/if}
{#if outcomeOpen}<GafferOutcome {room} {userId} onstarted={onchanged} onclose={()=>void closeOutcome()}/>{/if}
{#if reviewing}<GafferFloorReview auto={quick} sessions={chosen} {room} {userId} onstarted={onchanged} onclose={()=>void closeReview()}/>{/if}
{#if managing}<WorkspaceDialog title="Look after this session" onclose={()=>void closeGoal()}><GafferGoal id={managing.goal.conversationId} {userId} {timeZone} {dateFormat} onchanged={async()=>{await load(true);await onchanged()}}/></WorkspaceDialog>{/if}

<style>
  .overview{display:grid;gap:16px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.muted,.evidence{color:var(--muted)}.card-head,.actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.filter strong{font-variant-numeric:tabular-nums}.filters{display:flex;gap:6px;flex-wrap:wrap}.filter{font:inherit;color:var(--ink);border:1px solid var(--line);background:var(--bg);border-radius:8px;padding:7px 12px;min-height:44px;cursor:pointer}.filter[aria-pressed=true]{background:var(--tint);border-color:var(--accent)}.goals{list-style:none;padding:0;margin:0;display:grid;gap:16px}.goals>li{display:grid;gap:10px;min-width:0;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--bg)}.goals>li.attention{border-color:var(--accent)}h3{font:600 21px var(--serif);margin:0;overflow-wrap:anywhere;min-width:0}h4{font-size:14px;margin:0}.card-head{align-items:flex-start;flex-wrap:wrap}.state{font-size:0.875rem;border:1px solid var(--line);border-radius:6px;padding:2px 8px;white-space:nowrap}.attention .state{color:var(--accent)}.goal-text,.instruction{white-space:pre-wrap;overflow-wrap:anywhere}.review{border-left:2px solid var(--line-strong);padding-left:12px}.eyebrow{font-size:0.875rem;color:var(--muted);margin-bottom:4px}.report{white-space:pre-wrap}.evidence{font-size:0.875rem;font-variant-numeric:tabular-nums}.warning{color:var(--bad)}.delivery{font-weight:600}details{border-top:1px solid var(--line);padding-top:6px}summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;color:var(--muted)}.detail-body{display:grid;gap:10px;padding:8px 0}.actions{justify-content:flex-start;flex-wrap:wrap}.actions a{text-decoration:none;box-sizing:border-box}.btn{min-height:44px}button:focus-visible,a:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:3px}.own{display:grid;gap:6px;justify-items:start}.outcomes{display:grid;gap:10px;min-width:0}.outcome-list{list-style:none;margin:0;padding:0;display:grid;gap:8px}.outcome{display:grid;gap:2px;width:100%;text-align:left;font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:12px;padding:10px 14px;min-height:44px;cursor:pointer;overflow-wrap:anywhere}.outcome[aria-pressed=true]{background:var(--tint);border-color:var(--accent)}.outcome span{font-size:0.875rem;color:var(--muted);font-variant-numeric:tabular-nums}.clear{border:0;background:none;padding:0;min-height:44px;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer;text-decoration:underline;text-underline-offset:3px}.plan-state{list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:12px}.plan-state li{display:grid;gap:2px;padding:8px 14px;overflow-wrap:anywhere}.plan-state li+li{border-top:1px solid var(--line)}.plan-state span{font-size:0.875rem;color:var(--muted)}.bulk{display:flex;flex-wrap:wrap;gap:8px}.clear:disabled{opacity:.5;cursor:default}.idle{display:grid;gap:12px;min-width:0;border-top:1px solid var(--line);padding-top:16px}.picks{display:grid;border:1px solid var(--line);border-radius:12px}.pick{display:flex;align-items:flex-start;gap:12px;min-height:44px;padding:12px 14px;cursor:pointer;min-width:0}.pick{flex:1 1 auto}.pick-row{display:flex;align-items:center;gap:8px;min-width:0;padding-right:10px}.pick-row+.pick-row{border-top:1px solid var(--line)}.pick-open{flex:none;min-height:44px;min-width:44px;display:inline-flex;align-items:center;justify-content:center;color:var(--accent-text);font-weight:600;text-underline-offset:3px}.pick input{width:20px;height:20px;margin:2px 0 0;flex:none;accent-color:var(--accent)}.pick-text{display:grid;gap:2px;min-width:0;overflow-wrap:anywhere}.pick-text span{font-size:0.875rem;color:var(--muted)}.pick:has(input:focus-visible){outline:2px solid var(--accent);outline-offset:-2px;border-radius:12px}.empty{padding:16px 0}.empty p{margin-top:10px;color:var(--muted)}@media(max-width:559px){.goals>li{padding:12px}.filter{padding:7px 10px}}
  /* The floor: one hero action, then outcomes, sessions and what is not followed, each under its own heading. */
  .own{display:grid;grid-template-columns:auto 1fr auto;align-items:center;gap:14px;justify-items:stretch;padding:14px 16px;border:1px solid var(--line);border-radius:14px;background:var(--tint)}
  .own-copy{display:grid;gap:2px;min-width:0}.own-title{font:600 1.05rem var(--serif);color:var(--ink)}.own .btn{min-height:44px;white-space:nowrap}
  .section-head{display:flex;align-items:center;justify-content:space-between;gap:8px 12px;flex-wrap:wrap;min-width:0}
  .sessions{display:grid;gap:12px;min-width:0}.section-head h3{margin-right:auto}.bulk{justify-content:flex-end}
  .filters{flex-wrap:nowrap;overflow-x:auto;scrollbar-width:thin;padding-bottom:2px}.filter{flex:none;white-space:nowrap}
  .outcome-list>li{border:1px solid var(--line);border-radius:12px;background:var(--bg);overflow:hidden}.outcome-list>li.open{border-color:var(--accent)}
  .outcome{border:0;border-radius:0}.outcome-detail{display:grid;gap:10px;padding:0 14px 12px}.plan-state{background:var(--card)}.plan-state li.bad span{color:var(--bad)}
  .belongs{color:var(--ink)}.eyebrow-inline{font-size:0.8125rem;color:var(--muted);border:1px solid var(--line);border-radius:6px;padding:1px 6px;margin-right:4px}.part{color:var(--muted)}
  .clamp{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:3;line-clamp:3;overflow:hidden}
  .callout{padding:8px 12px;border-radius:8px;background:var(--tint);border:1px solid var(--accent);color:var(--ink)}
  .goals>li{gap:8px}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}.foot{border-top:1px solid var(--line);padding-top:12px}
  @media(max-width:559px){.own{grid-template-columns:auto 1fr}.own .btn{grid-column:1 / -1;justify-self:stretch}.refresh{order:2}.bulk{order:3;flex-basis:100%;justify-content:flex-start}}
</style>
