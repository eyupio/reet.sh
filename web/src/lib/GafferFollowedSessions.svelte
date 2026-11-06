<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { listGafferGoals, type GafferGoalOverview, type GafferGoalOverviewView } from './api'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  import GafferGoal from './GafferGoal.svelte'
  import GafferProse from './GafferProse.svelte'
  import { formatWhen } from './time'

  let { onclose, onchanged, onselect, onhistory, onerror, timeZone='', dateFormat='locale' }: {
    onclose:()=>void; onchanged:()=>void|Promise<void>; onselect:()=>void; onhistory:()=>void;
    onerror:(e:unknown)=>string; timeZone?:string; dateFormat?:string
  } = $props()
  let view=$state<GafferGoalOverviewView|null>(null), loading=$state(true), error=$state('')
  let filter=$state('all'), managing=$state<GafferGoalOverview|null>(null)
  let reading=$state(false)
  let refreshButton=$state<HTMLButtonElement>()
  let controller:AbortController|undefined, epoch=0, returnFocus:HTMLElement|undefined
  const rows=$derived(view?.goals??[])
  const needsYou=$derived(rows.filter(row=>row.attention).length)
  const following=$derived(rows.filter(row=>row.goal.state==='active'&&!row.attention).length)
  const visible=$derived(rows.filter(row=>filter==='all'||filter==='attention'&&row.attention||filter==='active'&&row.goal.state==='active'&&!row.attention||filter==='finished'&&['completed','stopped','limit'].includes(row.goal.state)))
  const label=(row:GafferGoalOverview)=>row.attention?'Needs you':row.goal.state==='active'?'Following':row.goal.state==='completed'?'You marked complete':row.goal.state==='limit'?'Limit reached':row.goal.state==='stopped'?'Stopped':'Paused'
  const deliveryLabel=(state:string)=>state==='sent'?'Handed to worker':state==='queued'?'Queued for next turn':state==='sending'?'Handoff in progress':state==='uncertain'?'Delivery uncertain':state==='blocked'?'Delivery blocked':state==='removed'?'Instruction removed':'Receipt needs review'

  async function load(fresh=false) {
    if(reading&&!fresh)return
    reading=true;controller?.abort();controller=new AbortController()
    const current=controller,mine=++epoch
    try {
      const next=await listGafferGoals(current.signal)
      if(mine!==epoch)return
      view=next;error=''
    } catch(e) { if(mine===epoch&&!current.signal.aborted)error=onerror(e) }
    finally { if(mine===epoch){reading=false;loading=false} }
  }
  onMount(()=>{
    void load()
    const timer=setInterval(()=>{if(!managing&&!document.hidden)void load()},10000)
    return()=>{epoch++;controller?.abort();clearInterval(timer)}
  })
  function openSession(event:MouseEvent,row:GafferGoalOverview) {
    if(event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey||!row.session)return
    const request=new CustomEvent('reet:show-session',{detail:row.session.id,cancelable:true})
    if(!window.dispatchEvent(request)){event.preventDefault();onselect()}
    else onclose()
  }
  async function closeGoal(){managing=null;await load(true);await tick();(returnFocus?.isConnected?returnFocus:refreshButton)?.focus({preventScroll:true})}
</script>

<WorkspaceDialog title="Followed sessions" {onclose}>
  <div class="overview">
    <p class="intro">The job sheet, with a brew on standby. These are the sessions you explicitly asked Gaffer to look after.</p>
    <div class="toolbar"><p class="counts">Needs you <strong>{needsYou}</strong> <span aria-hidden="true">·</span> Following <strong>{following}</strong></p><button class="btn quiet" bind:this={refreshButton} disabled={reading} onclick={()=>void load()}>Refresh</button></div>
    <nav class="filters" aria-label="Filter followed sessions">
      {#each [['all','All'],['attention','Needs you'],['active','Following'],['finished','Finished']] as [value,name]}<button class="filter" type="button" aria-pressed={filter===value} onclick={()=>filter=value}>{name}</button>{/each}
    </nav>
    {#if error}<p role="alert" class="warning">{error}</p>{#if view}<p class="muted">Showing the last successful read. Refresh to check for changes.</p>{/if}{/if}
    {#if loading}<p role="status">Reading the job sheet…</p>
    {:else if !rows.length&&!error}<div class="empty"><h3>Nowt on the job sheet yet.</h3><p>In a supported session, choose Chat options → Session follow-up, review its permissions and save a bounded goal. Opening this overview starts no follow-up.</p></div>
    {:else if !visible.length&&!error}<p>No followed sessions in this view.</p>
    {:else}<ul class="goals" aria-label="Followed session goals">
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
    {#if view}<p class="evidence">Overview read {formatWhen(view.readAt,timeZone,dateFormat)}. Evidence times above show what each review actually saw. Refreshing makes no model request.</p>{/if}
  </div>
</WorkspaceDialog>
{#if managing}<WorkspaceDialog title="Look after this session" onclose={()=>void closeGoal()}><GafferGoal id={managing.goal.conversationId} {timeZone} {dateFormat} onchanged={async()=>{await load(true);await onchanged()}}/></WorkspaceDialog>{/if}

<style>
  .overview{display:grid;gap:16px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.intro,.muted,.evidence{color:var(--muted)}.toolbar,.card-head,.actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.counts{font-size:15px}.counts strong{color:var(--ink)}.filters{display:flex;gap:6px;flex-wrap:wrap}.filter{font:inherit;color:var(--ink);border:1px solid var(--line);background:var(--bg);border-radius:20px;padding:7px 12px;min-height:44px;cursor:pointer}.filter[aria-pressed=true]{background:var(--tint);border-color:var(--accent)}.goals{list-style:none;padding:0;margin:0;display:grid;gap:16px}.goals>li{display:grid;gap:10px;min-width:0;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--bg)}.goals>li.attention{border-color:var(--accent)}h3{font:600 21px var(--serif);margin:0;overflow-wrap:anywhere;min-width:0}h4{font-size:14px;margin:0}.card-head{align-items:flex-start;flex-wrap:wrap}.state{font-size:12px;border:1px solid var(--line);border-radius:12px;padding:2px 8px;white-space:nowrap}.attention .state{color:var(--accent)}.goal-text,.instruction{white-space:pre-wrap;overflow-wrap:anywhere}.review{border-left:2px solid var(--line-strong);padding-left:12px}.eyebrow{font-size:12px;color:var(--muted);margin-bottom:4px}.report{white-space:pre-wrap}.evidence{font-size:12px}.warning{color:var(--bad)}.delivery{font-weight:600}details{border-top:1px solid var(--line);padding-top:6px}summary{min-height:44px;display:list-item;align-content:center;cursor:pointer;color:var(--muted)}.detail-body{display:grid;gap:10px;padding:8px 0}.actions{justify-content:flex-start;flex-wrap:wrap}.actions a{text-decoration:none;box-sizing:border-box}.btn{min-height:44px}button:focus-visible,a:focus-visible,summary:focus-visible{outline:2px solid var(--accent);outline-offset:3px}.empty{padding:18px 0}.empty p{margin-top:10px;color:var(--muted)}@media(max-width:559px){.goals>li{padding:12px}.toolbar{gap:8px}.filter{padding:7px 10px}}
</style>
