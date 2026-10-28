<script lang="ts">
 import {onMount} from 'svelte'
 import {createTerminal,type TerminalSession} from './api'
 let {agent,host,ready,disabled=false,oncreated}:{agent:string;host:string;ready:boolean;disabled?:boolean;oncreated?:(session:TerminalSession)=>void}=$props()
 let busy=$state(false),error=$state(''),operation:{agent:string;key:string}|undefined,alive=true
 const abort=new AbortController()
 export async function start(){
  if(busy||disabled||!ready)return
  if(operation&&operation.agent!==agent){error='Machine changed. Refresh before starting a terminal.';return}
  operation??={agent,key:crypto.randomUUID()};busy=true;error=''
  try{const item=await createTerminal(operation.agent,operation.key,'Shell',100,30,AbortSignal.any([abort.signal,AbortSignal.timeout(15000)]));if(!alive)return;if(oncreated)oncreated(item);else location.assign(`/sessions?agent=${encodeURIComponent(item.agentId)}&session=${encodeURIComponent(item.id)}`)}catch(e){if(alive&&!abort.signal.aborted)error=e instanceof Error?e.message:'Creation is unconfirmed. Retry checks the same operation.'}finally{if(alive)busy=false}
 }
 onMount(()=>()=>{alive=false;abort.abort()})
</script>
<div class="quick-create"><button type="button" aria-label={`${error?'Retry new':'New'} terminal on ${host}`} title={ready?`${error?'Retry new':'Start a new'} terminal on ${host}`:'Machine unavailable or terminal setup required'} disabled={disabled||!ready||busy} onclick={()=>void start()} aria-busy={busy}>{busy?'…':'+'}</button>{#if error}<span class="error" role="alert">{error} Retry + to check the same creation.</span>{/if}</div>
<style>
 .quick-create{position:relative;z-index:2}button{display:grid;place-items:center;width:32px;min-height:32px;border:1px solid var(--line);border-radius:7px;background:var(--card);color:var(--accent-text);font:500 1.25rem var(--sans);cursor:pointer}button:hover:not(:disabled){background:var(--tint);border-color:var(--accent)}button:disabled{opacity:.4;cursor:default}.error{display:block;width:min(220px,65vw);font-size:.65rem;line-height:1.5;color:var(--bad);margin-top:6px;overflow-wrap:anywhere}@media(pointer:coarse){button{width:44px;min-height:44px}}
</style>
