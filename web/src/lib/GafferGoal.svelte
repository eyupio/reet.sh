<script lang="ts">
 import {onMount} from 'svelte'
 import {getGafferGoal,setupGafferGoal,controlGafferGoal,type GafferGoalView} from './api'
 import {formatWhen} from './time'
 import ConfirmDialog from './ConfirmDialog.svelte'
 import GafferProse from './GafferProse.svelte'
 let {id,timeZone='',dateFormat='locale',onchanged}:{id:string;timeZone?:string;dateFormat?:string;onchanged:()=>void|Promise<void>}=$props()
 let view=$state<GafferGoalView|null>(null),text=$state(''),maxReviews=$state(12),hours=$state(24),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),confirming=$state(''),editOpen=$state(false)
 let controller:AbortController|undefined,epoch=0
 const bytes=$derived(new TextEncoder().encode(text).length)
 const canSetup=$derived(view?.setupAllowed??view?.allowed??false)
 const stateLabel=(state:string)=>state==='active'?'Looking after this session':state==='needs_user'?'Needs your attention':state==='limit'?'Limit reached':state==='completed'?'You marked this complete':state==='stopped'?'Stopped':'Paused'
 async function load(reset=false){
  controller?.abort();controller=new AbortController();const current=controller,mine=++epoch
  try{const next=await getGafferGoal(id,current.signal);if(mine!==epoch)return;if(reset){text=next.goal?.text??'';maxReviews=next.goal?.maxReviews??12;hours=next.goal?Math.max(1,Math.round((next.goal.expiresAt-next.goal.createdAt)/3600000)):24}view=next;error=''}catch(e){if(mine===epoch&&!current.signal.aborted)error=e instanceof Error?e.message:'The goal could not be read.'}finally{if(mine===epoch)loading=false}
 }
 onMount(()=>{void load(true);const timer=setInterval(()=>{if(!busy&&view?.goal?.state==='active')void load()},4000);return()=>{epoch++;controller?.abort();clearInterval(timer)}})
 async function save(){if(!view||busy||!text.trim()||bytes>2000)return;busy=true;error='';controller?.abort();controller=new AbortController();const mine=++epoch
  try{const next=await setupGafferGoal(id,{...view.pins,revision:view.goal?.revision??0,conversationRevision:view.conversationRevision??0,confirmed:true,action:'start',text,maxReviews,hours},controller.signal);if(mine!==epoch)return;view=next;editOpen=false;notice='Reet. Access and goal saved together. Gaffer will review new worker progress within this budget.';await onchanged()}catch(e){if(mine===epoch)error=e instanceof Error?e.message:'The goal was not saved. Reload before trying again.'}finally{busy=false}
 }
 async function control(action:string){if(!view?.goal||busy)return;busy=true;error='';controller?.abort();controller=new AbortController();const mine=++epoch
  try{const next=action==='resume'?await setupGafferGoal(id,{...view.pins,revision:view.goal.revision,conversationRevision:view.conversationRevision??0,confirmed:true,action:'resume'},controller.signal):await controlGafferGoal(id,view.goal.revision,action,view.pins,false,controller.signal);if(mine!==epoch)return;view=next;notice=action==='resume'?'Resumed with the remaining budget. Gaffer waits for fresh worker progress; old instructions are not resent.':'Goal updated. The working model keeps running.';await onchanged()}catch(e){if(mine===epoch)error=e instanceof Error?e.message:'The goal was not changed. Reload its current state.'}finally{busy=false}
 }
 function confirm(){const action=confirming;confirming='';if(action==='replace')void save();else void control(action)}
</script>
<section class="goal" aria-label="Session follow-up">
 <p>Give Gaffer a goal. The session’s working model handles the coding and PR work.</p>
 {#if loading}<p role="status">Finding the job sheet…</p>{:else if view}
  <p class="hint">Gaffer: {view.providerName||'Selected provider'} · {view.pins.model||'Model unavailable'}</p>
  {#if !view.goal||editOpen||['paused','needs_user'].includes(view.goal.state)}<div class="access-review" aria-label="Session access review"><p>Starting lets Gaffer read only this session’s recent conversation, including any code or logs it contains, and queue instructions for its working model. The model keeps its own approvals.</p>{#if view.requiresProviderAccess}<p class="hint">This also enables selected-session data for {view.providerName||'this provider'}. Other sessions need their own delegation.</p>{/if}</div>{/if}
  {#if !canSetup}<p class="warning" role="status">This session needs an enabled provider and a supported structured worker with live message controls. Check the session or Gaffer settings before starting.</p>{/if}
  {#if view.goal}
   <section class="status" aria-label="Current goal"><h3>{stateLabel(view.goal.state)}</h3><p class="saved-goal">{view.goal.text}</p><p>{view.goal.usedReviews} / {view.goal.maxReviews} reviews used · Until {formatWhen(view.goal.expiresAt,timeZone,dateFormat)}</p><p class="hint">{view.goal.note}</p>
    {#if view.goal.summary}<div class="summary"><h4>Latest Gaffer review</h4><p class="hint">Review model: {view.goal.model}</p><GafferProse text={view.goal.summary}/><p class="hint">Observed through worker event #{view.goal.lastSeq} · {formatWhen(view.goal.lastObservedAt??0,timeZone,dateFormat)}. Review finished {formatWhen(view.goal.lastReviewedAt??0,timeZone,dateFormat)}</p></div>{/if}
    <p class="hint">{view.goal.usageReported?`${view.goal.inputTokens} input · ${view.goal.outputTokens} output tokens reported`:'Provider usage is not fully reported.'}</p>
    <div class="actions">
     {#if view.goal.state==='active'}<button class="btn" disabled={busy} onclick={()=>void control('pause')}>Pause follow-up</button>{/if}
     {#if ['paused','needs_user'].includes(view.goal.state)}<button class="btn primary" disabled={busy||!canSetup||view.goal.usedReviews>=view.goal.maxReviews||view.goal.expiresAt<=Date.now()} onclick={()=>confirming='resume'}>Resume after review</button>{/if}
     {#if !['completed','stopped'].includes(view.goal.state)}<button class="btn" disabled={busy} onclick={()=>confirming='complete'}>Mark goal complete</button><button class="btn quiet" disabled={busy} onclick={()=>confirming='stop'}>Stop goal</button>{/if}
    </div>
   </section>
  {/if}
  {#if view.goal}<button class="edit-goal btn quiet" type="button" aria-expanded={editOpen} onclick={()=>editOpen=!editOpen}>{editOpen?'Close goal editor':'Edit goal or start a new budget'}</button>{/if}
  {#if !view.goal||editOpen}<form onsubmit={event=>{event.preventDefault();if(view?.goal)confirming='replace';else void save()}}>
   <label>Agreed goal<textarea aria-label="Agreed goal" bind:value={text} maxlength="2000" rows="3" placeholder="Ensure a PR is raised and its checks and conflicts are addressed. Bring decisions back to me. Do not merge." disabled={busy}></textarea></label>
   <details class="limit-editor"><summary>{maxReviews} reviews · {hours} hours <span>Change limits</span></summary><div class="limits"><label>Review budget<input aria-label="Review budget" type="number" min="1" max="48" step="1" bind:value={maxReviews} required disabled={busy}/></label><label>Follow for (hours)<input aria-label="Follow for (hours)" type="number" min="1" max="168" step="1" bind:value={hours} required disabled={busy}/></label></div></details>
   <p class="hint">Gaffer keeps going with new progress while you’re offline, and pauses for decisions or limits. Reviews may use provider credits.</p>
   <details><summary>How follow-up works</summary><p class="hint">Each review may queue one instruction and use up to two model requests. Closing the browser keeps the goal running. Pause or Stop removes instructions still queued; already claimed work cannot be recalled. A readiness report needs your verification.</p></details>
   {#if bytes>2000}<p class="warning">Keep the goal within 2,000 UTF-8 bytes.</p>{/if}
   <p class="hint">{view.goal?'Starting a new goal renews':'Start follow-up saves'} this session’s access and the displayed goal together. You can pause or stop at any time.</p>
   <button class="btn primary" type="submit" disabled={busy||!canSetup||!text.trim()||bytes>2000}>{view.goal?'Start new goal / budget':'Start follow-up'}</button>
  </form>{/if}
 {/if}
 {#if error}<p class="warning" role="alert">{error}</p><button class="btn" disabled={busy} onclick={()=>load(true)}>Reload goal</button>{/if}
 {#if notice}<p role="status">{notice}</p>{/if}
</section>
{#if confirming}<ConfirmDialog title={confirming==='resume'?'Resume after checking the working conversation?':confirming==='replace'?'Start a new goal and budget?':confirming==='complete'?'Mark this goal complete?':'Stop this goal?'} body={confirming==='resume'?'Confirm that you checked approvals, scope decisions and any uncertain delivery. Gaffer keeps the remaining budget and waits for fresh worker progress; old instructions are not resent.':confirming==='replace'?'This replaces the saved goal, removes its queued instructions and starts the displayed review budget. Already claimed work cannot be recalled.':confirming==='complete'?'This records your decision. It does not merge a PR or stop the working model.':'This ends Gaffer follow-up and removes goal instructions still queued. The working model keeps running.'} confirmLabel={confirming==='resume'?'Resume follow-up':confirming==='replace'?'Start new goal':confirming==='complete'?'Mark complete':'Stop goal'} danger={confirming==='stop'} oncancel={()=>confirming=''} onconfirm={confirm}/>{/if}
<style>
 .goal{display:grid;gap:16px;font-size:14px;line-height:1.5;min-width:0}p{margin:0}h3{margin:0;font:600 22px var(--serif)}h4{margin:0 0 8px;font-size:14px;font-weight:600}.hint{color:var(--muted);font-variant-numeric:tabular-nums}.warning{color:var(--bad)}.status{padding:16px;border:1px solid var(--line);border-radius:12px;display:grid;gap:12px;font-variant-numeric:tabular-nums}.saved-goal{white-space:pre-wrap;overflow-wrap:anywhere}.summary{padding-top:12px;border-top:1px solid var(--line)}.summary .hint{margin-top:10px}form{display:grid;gap:12px}label{display:grid;gap:6px;font-weight:600}textarea,input{font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;max-width:100%;min-height:44px}textarea{resize:vertical;font-weight:400;width:100%;box-sizing:border-box}.limits{display:grid;grid-template-columns:1fr 1fr;gap:12px}.limits input{width:100%;box-sizing:border-box}.actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px} @media(max-width:559px){textarea,input{font-size:16px}.limits{gap:8px}}
</style>
