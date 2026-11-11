<script lang="ts">
 import {onMount} from 'svelte'
 import {getWorkerSettings,saveWorkerSettings,interruptWorkerMessage,type WorkerSettingsResponse,type WorkerMessage} from './api'
 import {formatWhen} from './time'
 import ConfirmDialog from './ConfirmDialog.svelte'
 let {path,gaffer=false,onchanged,onrequest,timeZone='',dateFormat='locale'}:{path:string;gaffer?:boolean;onchanged?:()=>void|Promise<void>;onrequest?:()=>void;timeZone?:string;dateFormat?:string}=$props()
 let data=$state<WorkerSettingsResponse|null>(null),selected=$state(''),enabled=$state(false),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),stopping=$state<WorkerMessage|null>(null)
 let request:AbortController|undefined,epoch=0
 const url=$derived(gaffer?path:`${path}?sessionId=${encodeURIComponent(selected)}`)
 const failure=(e:unknown)=>e instanceof Error?e.message:'Worker messaging is unavailable.'
 async function load(reset=false){
  request?.abort();request=new AbortController();const controller=request,mine=++epoch
  if(reset)loading=true
  try{const next=await getWorkerSettings(url,controller.signal);if(mine!==epoch)return
   if(reset||!data||data.worker.grant.revision!==next.worker.grant.revision||data.worker.providerRevision!==next.worker.providerRevision||data.worker.providerId!==next.worker.providerId||data.worker.model!==next.worker.model)enabled=next.worker.grant.enabled
   data=next;error=''
  }catch(e){if(mine===epoch&&!controller.signal.aborted)error=failure(e)}finally{if(mine===epoch)loading=false}
 }
 onMount(()=>{void load(true);const timer=setInterval(()=>{if(!busy&&!loading&&data?.messages.some(m=>m.state==='queued'||m.state==='sending'))void load()},3000);return()=>{epoch++;request?.abort();clearInterval(timer)}})
 async function save(){
  if(!data)return;busy=true;error='';const mine=epoch
  try{await saveWorkerSettings(url,data.worker,enabled,request?.signal);if(mine!==epoch)return;await load(true);notice=enabled?'Worker permission saved. No instruction has been sent.':'Worker messaging stopped. Already claimed or delivered work cannot be recalled.';await onchanged?.()}catch(e){if(mine===epoch)error=failure(e)}finally{busy=false}
 }
 async function interrupt(){
  const message=stopping;if(!message)return;stopping=null;busy=true;error=''
  try{await interruptWorkerMessage(url,message,request?.signal);await load();notice='Stop and send requested. Read the receipt for the outcome.'}catch(e){error=failure(e)}finally{busy=false}
 }
</script>
<section class="worker-messaging" aria-label="Worker messaging">
 <p>{gaffer?'Gaffer relays your instructions; the session’s existing model does the work.':'Grant this client permission to queue instructions for one selected session. Library and context permissions stay separate.'}</p>
 {#if !gaffer}<label>Session<select aria-label="Worker session" value={selected} disabled={busy||loading} onchange={event=>{selected=event.currentTarget.value;notice='';void load(true)}}><option value="">Choose a structured session</option>{#each data?.sessions??[] as session}<option value={session.id}>{session.title||session.id}</option>{/each}{#each data?.scopes?.filter(g=>!data?.sessions?.some(s=>s.id===g.sessionId))??[] as grant}<option value={grant.sessionId}>Session {grant.sessionId}</option>{/each}</select></label>{/if}
 {#if loading}<p role="status">Reading worker permission…</p>{:else if data}
  {#if data.worker.providerId}<p class="hint">Gaffer model: {data.worker.model||'Unavailable'}. A provider/model or policy change needs review.</p>{/if}
  {#if data.worker.reason}<p class="hint" role="status">{data.worker.reason}</p>{/if}
  <label class="grant"><input type="checkbox" aria-label="Allow worker messages" checked={enabled} disabled={busy||(!data.worker.allowed&&!enabled)} onchange={event=>enabled=event.currentTarget.checked}/><span>Allow {gaffer?'Gaffer':'this client'} to queue worker messages for this session</span></label>
  <p class="hint">Messages wait for the next turn. Existing worker approvals stay in charge. No automatic interrupt, shell/GitHub action or offline Gaffer supervision is enabled. Stop permission to remove instructions still queued; claimed or sent work cannot be recalled.</p>
  <div class="actions"><button class="btn" type="button" disabled={busy||loading||(!data.worker.grant.sessionId&&!selected)} onclick={save}>Save worker permission</button>{#if gaffer&&data.worker.grant.enabled&&data.worker.allowed&&!data.worker.needsReview}<button class="btn primary" type="button" disabled={busy||enabled!==data.worker.grant.enabled} onclick={onrequest}>Enable one worker message for next request</button>{/if}</div>
  {#if data.messages.length}<h3>Instruction receipts</h3><p class="hint">Sent means worker acceptance, not finished work. Delivery uncertainty must be reviewed in the working conversation.</p><ol>{#each data.messages as message (message.id)}<li><div class="receipt-head"><strong>{message.state==='sent'?'Accepted by worker':message.state==='removed'?'Removed':message.state==='uncertain'?'Delivery uncertain':message.state==='blocked'?'Blocked':message.state==='sending'?'Sending':'Queued for next turn'}</strong><span>{formatWhen(message.createdAt,timeZone,dateFormat)}</span></div><p class="instruction">{message.text}</p>{#if message.note}<p class="hint">{message.note}</p>{/if}{#if message.state==='uncertain'}<p class="warning">Do not resend automatically. Check the working conversation first.</p>{/if}{#if message.state==='queued'&&data.worker.grant.enabled&&!data.worker.needsReview}<button class="btn" type="button" disabled={busy||!data.worker.allowed} onclick={()=>stopping=message}>Stop worker and send</button>{/if}<details><summary>Receipt IDs</summary><p class="hint">Request {message.id}<br/>Queue {message.queueId}</p></details></li>{/each}</ol>{/if}
 {/if}
 {#if error}<p class="error" role="alert">{error}</p><button class="btn" type="button" disabled={busy} onclick={()=>load(true)}>Reload worker messages</button>{/if}
 {#if notice}<p role="status">{notice}</p>{/if}
</section>
{#if stopping}<ConfirmDialog title="Stop the worker and send this instruction?" body="This interrupts the current worker turn, then sends this already queued instruction. It does not answer approval cards or prove the current work is saved. Check any uncertain delivery first." confirmLabel="Stop worker and send" danger oncancel={()=>stopping=null} onconfirm={()=>void interrupt()}/>{/if}
<style>
 .worker-messaging{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0}.hint{color:var(--muted)}label{display:grid;gap:8px}.grant{display:flex;align-items:flex-start;gap:10px}.grant input{width:20px;height:20px;flex:none;margin-top:2px;accent-color:var(--accent)}select{font:inherit;min-height:44px;min-width:0;max-width:100%;border:1px solid var(--line);border-radius:8px;padding:8px;background:var(--bg);color:var(--ink)}.actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}h3{font:600 20px var(--serif);margin:6px 0 0}ol{list-style:none;padding:0;margin:0;display:grid;gap:12px}li{display:grid;gap:10px;padding:12px;border:1px solid var(--line);border-radius:10px;min-width:0}.receipt-head{display:flex;justify-content:space-between;gap:8px;flex-wrap:wrap}.receipt-head span{color:var(--muted)}.instruction{white-space:pre-wrap;overflow-wrap:anywhere}.error,.warning{color:var(--bad)}details{overflow-wrap:anywhere}summary{cursor:pointer;min-height:32px;align-content:center}
</style>
