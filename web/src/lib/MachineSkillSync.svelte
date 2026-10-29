<script lang="ts">
 import { onMount } from 'svelte'
 import { shell } from './dialogShell'
 import { getSyncState,getImports,getImport,syncFromMachine,queueSyncJob,type Skill,type ImportRequest,type SyncState } from './api'
 import { installSkillOnMachine } from './syncActions'
 import MachineSkillImports from './MachineSkillImports.svelte'
 import SyncProblem from './SyncProblem.svelte'
 let {skill,destination,initialRequestId='',onclose}:{skill:Pick<Skill,'id'|'slug'|'name'>;destination:string;initialRequestId?:string;onclose:()=>void}=$props()
 let dialog=$state<HTMLDialogElement>(),request=$state<ImportRequest|null>(null),snapshot=$state<SyncState|null>(null)
 let error=$state(''),busy=$state(false),review=$state(false),sourceId=$state(''),retryRunId=$state('')
 let alive=true,polling=false
 let requestKey=crypto.randomUUID()
 const sources=$derived((snapshot?.environments??[]).filter(env=>env.skills.some(item=>item.id===skill.id)).sort((a,b)=>Number(b.agent==='agents')-Number(a.agent==='agents')||b.scannedAt-a.scannedAt))
 const run=$derived(snapshot?.runs?.find(item=>item.id===(retryRunId||request?.syncRunId)))
 const job=$derived((snapshot?.jobs??[]).filter(item=>item.targetId===request?.destinationId).sort((a,b)=>b.createdAt-a.createdAt)[0])
 const complete=$derived(!!request?.syncRunId&&(run?.status==='completed'||!!snapshot?.environments.find(env=>env.id===request?.destinationId)?.skills.some(item=>item.id===skill.id)))
 const failed=$derived(!complete&&(!!request?.syncError||['failed','expired','cancelled'].includes(request?.state??'')||run?.status==='failed'))
 const step=$derived(request?.state==='completed'?(request.syncRunId?2:1):['approved','uploading'].includes(request?.state??'')?1:0)
 const headline=$derived(complete?`${skill.name} is on ${destination}`:failed?'Sync needs attention':request?.state==='reviewing'?'Some files need your review':step===2?`Installing on ${destination}`:step===1?'Saving a private copy in reet.sh':`Reading ${skill.name} from ${request?.host??sources.find(item=>item.id===sourceId)?.host??'your machine'}`)
 async function refresh(){
  if(!alive||polling)return
  polling=true
  try{const [state,current]=await Promise.all([getSyncState(),request?getImport(request.id):Promise.resolve(null)]);if(!alive)return;snapshot=state;if(current)request=current}
  catch(e){if(alive)error=e instanceof Error?e.message:String(e)}finally{polling=false}
 }
 async function start(){
  if(busy)return;busy=true;error=''
  try{request=await syncFromMachine({skillId:skill.id,destination,sourceId,requestKey})}
  catch(e){error=e instanceof Error?e.message:String(e)}finally{busy=false}
 }
 async function retry(){
  if(busy)return;busy=true;error=''
  try{
   if(request?.syncError){
    const target=snapshot?.environments.find(env=>env.id===request?.destinationId)
    const agent=snapshot?.agents?.find(item=>item.host.toLowerCase()===target?.host.toLowerCase())
    if(!agent)throw new Error('Connect the destination machine first.')
    snapshot=await queueSyncJob(agent.id,'','scan')
   }else if(request?.syncRunId){
    const result=await installSkillOnMachine(skill,destination)
    snapshot=result.state;retryRunId=result.state.runs?.find(item=>item.targets.includes(request!.destinationId!))?.id??''
   }else{
    requestKey=crypto.randomUUID()
    request=await syncFromMachine({skillId:skill.id,destination,sourceId,requestKey})
   }
  }catch(e){error=e instanceof Error?e.message:String(e)}finally{busy=false}
 }
 onMount(()=>{dialog?.showModal();void(async()=>{await refresh();if(!alive)return;sourceId=sources.find(item=>item.host.toLowerCase()!==destination.toLowerCase())?.id??sources[0]?.id??'';try{const imports=await getImports();request=imports.requests.find(item=>initialRequestId?item.id===initialRequestId:item.syncSkillId===skill.id&&snapshot?.environments.find(env=>env.id===item.destinationId)?.host.toLowerCase()===destination.toLowerCase()&&['queued','previewing','reviewing','approved','uploading','completed'].includes(item.state))??null}catch{}if(alive&&!request&&sources.length===1)await start()})()
  const timer=setInterval(()=>{if(document.visibilityState==='visible')void refresh()},2000)
  return()=>{alive=false;clearInterval(timer)}
 })
</script>
<dialog bind:this={dialog} use:shell aria-labelledby="machine-copy-title" oncancel={event=>{event.preventDefault();onclose()}}>
 <header data-shell-head><h2 id="machine-copy-title">Sync {skill.name}</h2><button class="btn quiet" data-shell-close onclick={onclose}>Close</button></header>
 <div class="body" data-shell-body>
  <p>Reet uses the installed skill’s files when an upstream source is unavailable. The copy stays private in your account.</p>
  {#if !request}
   {#if sources.length>1}<label>Copy from<select bind:value={sourceId}>{#each sources as env (env.id)}<option value={env.id}>{env.host} · {env.agent}{env.project?` · ${env.project}`:''}</option>{/each}</select></label>{/if}
   <p>Destination: <strong>{destination}</strong></p>
   <button class="btn primary" disabled={busy||!sourceId} onclick={start}>{busy?'Requesting copy…':`Sync to ${destination}`}</button>
  {:else}
   <h3 aria-live="polite">{headline}</h3>
   <ol aria-label="Transfer progress">{#each ['Read source machine','Save private copy','Install on destination'] as label,i}<li class:done={complete||i<step} class:active={!complete&&i===step}>{label}</li>{/each}</ol>
   <p>{request.host} → reet.sh → {destination}</p>
   {#if request.state==='reviewing'}<button class="btn primary" onclick={()=>review=true}>Review files</button>{/if}
   {#if request.syncError}<p class="error">{request.syncError}</p>{/if}
   {#if request.syncRunId&&job?.message&&job.status==='failed'}<SyncProblem message={job.message} host={destination} onretry={()=>void retry()} retrying={busy} />{:else if request.error}<p class="error">{request.error.replaceAll('_',' ')}. Refresh the source machine, then request a new copy.</p>{/if}
   {#if step===2&&!complete&&!failed}<p role="status">{job?.stages?.find(stage=>stage.status==='running')?.detail??job?.message??'Waiting for the destination machine to report.'}</p>{/if}
   {#if run?.status==='review'}<a class="btn primary" href={`/sync?machine=${encodeURIComponent(destination)}#sync-jobs`}>Review changes</a>{/if}
   {#if failed&&!request.syncRunId}<button class="btn primary" disabled={busy} onclick={retry}>{busy?'Trying…':'Fix and try again'}</button>{/if}
   {#if failed}<a class="btn" href={`/sync?machine=${encodeURIComponent(destination)}`}>Fix it in Sync</a>{/if}
  {/if}
  {#if error}<p class="error" role="alert">{error}</p><a href="/machines">Check machine connection and reet updates</a>{/if}
 </div>
 <footer data-shell-foot><p>Closing keeps the requested sync running. Nothing is published publicly.</p><button class="btn" onclick={onclose}>{complete?'Done':'Close'}</button></footer>
</dialog>
{#if review&&snapshot&&request}<MachineSkillImports {snapshot} source={{environmentId:request.environmentId,name:request.name}} initialRequestId={request.id} onclose={()=>{review=false;void refresh()}} />{/if}
<style>
 dialog{width:min(620px,calc(100vw - 28px));max-height:calc(100dvh - 28px);padding:0;border:1px solid var(--line);border-radius:14px;background:var(--card);color:var(--ink)}dialog::backdrop{background:var(--scrim)}header,footer{padding:18px;display:flex;gap:12px;align-items:center;justify-content:space-between}header{border-bottom:1px solid var(--line)}footer{border-top:1px solid var(--line);flex-wrap:wrap}h2{font:500 1.5rem var(--serif);margin:0}h3{font-size:1rem}.body{padding:18px;overflow:auto}p{color:var(--muted);font-size:.85rem;line-height:1.6}label{display:grid;gap:6px}ol{list-style:none;padding:0;display:grid;gap:8px}li{padding:10px;border:1px solid var(--line);border-radius:8px;color:var(--muted);font-size:.85rem}li.active{border-color:var(--accent-text);color:var(--accent-text)}li.done{border-color:var(--good);color:var(--good)}.error{color:var(--bad)}a{color:var(--accent-text)}
</style>
