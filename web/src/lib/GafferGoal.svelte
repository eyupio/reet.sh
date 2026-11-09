<script lang="ts">
 import {onMount,tick} from 'svelte'
 import {getGafferGoal,setupGafferGoal,controlGafferGoal,type GafferGoalView} from './api'
 import {formatWhen} from './time'
 import ConfirmDialog from './ConfirmDialog.svelte'
 import GafferProse from './GafferProse.svelte'
 import SlotPicker,{type SlotOption} from './launch/SlotPicker.svelte'
 import {BUILT_IN_GOAL_PROMPTS,MAX_GOAL_PROMPT_BYTES,addGoalPrompt,goalPromptTitle,readGoalPrompts,removeGoalPrompt,writeGoalPrompts,type GoalPrompt} from './gafferPrompts'
 import {insertSkillPrompt} from './skillPrompt'
 let {id,userId=0,timeZone='',dateFormat='locale',draft='',onchanged}:{id:string;userId?:number;timeZone?:string;dateFormat?:string;draft?:string;onchanged:()=>void|Promise<void>}=$props()
 // A goal written for the person by the session overview: it is put in the editor once, on the first read, and never started for them.
 let draftUsed=false
 const goalId=$props.id()
 let view=$state<GafferGoalView|null>(null),text=$state(''),maxReviews=$state(12),hours=$state(24),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),confirming=$state(''),editOpen=$state(false)
 let goalInput=$state<HTMLTextAreaElement>(),promptButton=$state<HTMLButtonElement>(),promptOpen=$state(false),touch=$state(false)
 let userPrompts=$state<GoalPrompt[]>([])
 let controller:AbortController|undefined,epoch=0
 const bytes=$derived(new TextEncoder().encode(text).length)
 const canSetup=$derived(view?.setupAllowed??view?.allowed??false)
 const promptOptions=$derived<SlotOption[]>([
  ...BUILT_IN_GOAL_PROMPTS.map(prompt=>({value:`builtin:${prompt.id}`,label:prompt.title,description:prompt.text,group:'reet prompts'})),
  ...userPrompts.map(prompt=>({value:`user:${prompt.id}`,label:goalPromptTitle(prompt.text),description:prompt.text,group:'Your prompts',removeLabel:`Remove “${goalPromptTitle(prompt.text)}”`,onremove:()=>removePrompt(prompt.id)})),
 ])
 const promptStatus=$derived(userPrompts.length?`reet prompts and ${userPrompts.length} of your own, kept in this browser.`:'reet prompts. Save your own with “Save as prompt”.')
 const stateLabel=(state:string)=>state==='active'?'Looking after this session':state==='needs_user'?'Needs your attention':state==='limit'?'Limit reached':state==='completed'?'You marked this complete':state==='stopped'?'Stopped':'Paused'
 async function load(reset=false){
  controller?.abort();controller=new AbortController();const current=controller,mine=++epoch
  try{const next=await getGafferGoal(id,current.signal);if(mine!==epoch)return;if(reset){text=next.goal?.text??'';maxReviews=next.goal?.maxReviews??12;hours=next.goal?Math.max(1,Math.round((next.goal.expiresAt-next.goal.createdAt)/3600000)):24;if(draft&&!draftUsed){draftUsed=true;text=draft;editOpen=true;void tick().then(()=>document.getElementById(`${goalId}-goal`)?.focus())}}view=next;error=''}catch(e){if(mine===epoch&&!current.signal.aborted)error=e instanceof Error?e.message:'The goal could not be read.'}finally{if(mine===epoch)loading=false}
 }
 onMount(()=>{touch=matchMedia('(pointer: coarse)').matches;userPrompts=readGoalPrompts(userId);void load(true);const timer=setInterval(()=>{if(!busy&&view?.goal?.state==='active')void load()},4000);return()=>{epoch++;controller?.abort();clearInterval(timer)}})
 function usePrompt(value:string){
  const prompt=BUILT_IN_GOAL_PROMPTS.find(item=>`builtin:${item.id}`===value)
  const saved=userPrompts.find(item=>`user:${item.id}`===value)
  const chosen=prompt?.text??saved?.text
  if(!chosen)return
  const inserted=insertSkillPrompt(text,chosen,text.length,text.length)
  text=inserted.text
  notice=''
  void tick().then(()=>{if(!goalInput)return;goalInput.focus();goalInput.setSelectionRange(inserted.caret,inserted.caret)})
 }
 function savePrompt(){
  if(!text.trim()||bytes>MAX_GOAL_PROMPT_BYTES)return
  const next=addGoalPrompt(userPrompts,text,promptId(),Date.now())
  if(next===userPrompts){notice='That goal is already in your prompts.';return}
  userPrompts=next
  writeGoalPrompts(userId,userPrompts)
  notice='Saved to your prompts.'
 }
 function removePrompt(promptId:string){
  userPrompts=removeGoalPrompt(userPrompts,promptId)
  writeGoalPrompts(userId,userPrompts)
  notice='Prompt removed.'
 }
 const promptId=()=>globalThis.crypto?.randomUUID?.()??`${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
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
   <div class="goal-field"><div class="goal-head"><label for={`${goalId}-goal`}>Agreed goal</label><div class="goal-tools"><button type="button" class="prompt-add" bind:this={promptButton} aria-haspopup="listbox" aria-expanded={promptOpen} disabled={busy} onclick={()=>promptOpen=!promptOpen}>Use a prompt</button><button type="button" class="prompt-add" disabled={busy||!text.trim()||bytes>MAX_GOAL_PROMPT_BYTES} onclick={savePrompt}>Save as prompt</button></div></div><textarea id={`${goalId}-goal`} bind:this={goalInput} aria-label="Agreed goal" bind:value={text} maxlength="2000" rows="3" placeholder="Ensure a PR is raised and its checks and conflicts are addressed. Bring decisions back to me. Do not merge." disabled={busy}></textarea></div>
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
{#if promptOpen}<SlotPicker label="Goal prompt" options={promptOptions} value="" sheet={touch} opener={promptButton??null} filter="Search prompts" status={promptStatus} onpick={usePrompt} onclose={()=>promptOpen=false}/>{/if}
<style>
 .goal{display:grid;gap:16px;font-size:14px;line-height:1.5;min-width:0}p{margin:0}h3{margin:0;font:600 22px var(--serif)}h4{margin:0 0 8px;font-size:14px;font-weight:600}.hint{color:var(--muted);font-variant-numeric:tabular-nums}.warning{color:var(--bad)}.status{padding:16px;border:1px solid var(--line);border-radius:12px;display:grid;gap:12px;font-variant-numeric:tabular-nums}.saved-goal{white-space:pre-wrap;overflow-wrap:anywhere}.summary{padding-top:12px;border-top:1px solid var(--line)}.summary .hint{margin-top:10px}form{display:grid;gap:12px}label{display:grid;gap:6px;font-weight:600}textarea,input{font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;max-width:100%;min-height:44px}textarea{resize:vertical;font-weight:400;width:100%;box-sizing:border-box}.limits{display:grid;grid-template-columns:1fr 1fr;gap:12px}.limits input{width:100%;box-sizing:border-box}.actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}.goal-field{display:grid;gap:6px}.goal-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.goal-head label{display:block;margin:0}.goal-tools{display:flex;align-items:center;gap:16px;flex-wrap:wrap}.prompt-add{border:0;background:none;padding:4px 0;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer}.prompt-add:hover{text-decoration:underline}.prompt-add:focus-visible{outline:2px solid var(--accent);outline-offset:3px}.prompt-add:disabled{color:var(--muted);cursor:default;text-decoration:none} @media(max-width:559px){textarea,input{font-size:16px}.limits{gap:8px}.goal-tools{gap:12px}.prompt-add{min-height:44px}}
</style>
