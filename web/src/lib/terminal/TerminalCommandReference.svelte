<script lang="ts">
  import {onMount} from 'svelte'
  import CopyButton from '../CopyButton.svelte'
  import {terminalControllerStatus,type TerminalControllerStatus} from './api'
  let status=$state<TerminalControllerStatus|null>(null),failed=$state(false),busy=$state(false),checkedAt=$state(0),now=$state(performance.now())
  let request:AbortController|undefined,disposed=false
  const stale=$derived(checkedAt>0&&now-checkedAt>45000)
  const label=$derived(failed?'Status unavailable':stale?'Status stale':!status?'Checking…':status.available?'Active':status.configured?'Blocked':'Inactive')
  const description=$derived(status?.available?'Controller access is enabled. Each host still needs local terminal consent.':status?.reason==='https_required'?'Started, but the controller has no usable HTTPS public URL. Start/restart reads your existing .env configuration.':status?.reason==='accounts_required'?'Started, but the running controller is not using accounts authentication.':'Remote access is stopped. Existing host shells continue running.')
  async function refresh(){
    if(busy||disposed)return;busy=true;const abort=new AbortController();request=abort;const timeout=setTimeout(()=>abort.abort(),5000)
    try{const next=await terminalControllerStatus(abort.signal);if(!disposed){status=next;failed=false;checkedAt=performance.now();now=checkedAt}}
    catch{if(!disposed)failed=true}finally{clearTimeout(timeout);busy=false}
  }
  const commands=[{action:'status',detail:'Inspect availability'},{action:'start',detail:'Enable remote access'},{action:'stop',detail:'Disconnect access; keep shells'},{action:'restart',detail:'Reconnect with fresh authorization'}]
  onMount(()=>{void refresh();const wake=()=>{if(document.visibilityState==='visible')void refresh()},poll=setInterval(wake,15000),clock=setInterval(()=>now=performance.now(),1000);document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake);return()=>{disposed=true;request?.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}})
</script>

<section class="reference" aria-label="Terminal command reference">
  <header><div><strong>Controller terminal access</strong><span class="badge" class:active={!failed&&!stale&&status?.available} role="status">{label}</span></div><button type="button" class="btn quiet" disabled={busy} onclick={()=>void refresh()} aria-label="Refresh controller terminal status">{busy?'Checking…':'Refresh status'}</button></header>
  {#if failed||stale}<p role="status">Current status could not be confirmed.{#if status} Last reported: {status.available?'active':status.configured?'blocked':'inactive'}.{/if}</p>{:else if status}<p>{description}</p>{/if}
  <details open={status!==null&&!status.available}><summary>Quick command reference</summary><p>Run on the controller hosting reet.sh, as its OS user—not on a connected host. Existing .env settings are reused.</p><ul>{#each commands as item}<li><div><code>reet terminal {item.action}</code><small>{item.detail}</small></div><CopyButton text={`reet terminal ${item.action}`} label="Copy" ariaLabel={`Copy reet terminal ${item.action}`} compact /></li>{/each}</ul><p>Use <code>--data-dir PATH</code> or <code>--env-file PATH</code> when needed. For containers, run inside the controller container. On a connected host, use <code>reet agent terminal enable</code> to opt in locally.</p></details>
  {#if checkedAt>0&&!failed&&!stale}<small class="checked">Checked {Math.max(0,Math.floor((now-checkedAt)/1000))}s ago · updates every 15s</small>{/if}
</section>
<style>
 .reference{border:1px solid var(--line);border-radius:12px;padding:14px 16px;background:var(--card);min-width:0}header,header>div{display:flex;align-items:center;gap:10px;flex-wrap:wrap}header{justify-content:space-between}strong{font-size:.82rem}.badge{font-size:.68rem;font-weight:600;background:var(--tint);color:var(--muted);border:1px solid var(--line);border-radius:999px;padding:4px 9px}.badge.active{color:var(--good);background:color-mix(in srgb,var(--good) 10%,var(--card))}header button{font-size:.72rem;padding:4px 8px;min-height:32px}p{font-size:.75rem;color:var(--muted);line-height:1.6;margin:10px 0}details{border-top:1px solid var(--line);padding-top:10px;margin-top:10px}summary{cursor:pointer;font-size:.76rem;font-weight:600}ul{padding:0;margin:12px 0;list-style:none;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px}li{display:flex;align-items:center;justify-content:space-between;gap:10px;border:1px solid var(--line);border-radius:8px;padding:10px;min-width:0}li>div{min-width:0}code{font:.75rem var(--mono,monospace);overflow-wrap:anywhere;color:var(--ink)}li small{display:block;color:var(--muted);font-size:.68rem;margin-top:5px}.checked{font-size:.65rem;color:var(--muted);display:block;margin-top:10px}@media(max-width:650px){ul{grid-template-columns:minmax(0,1fr)}.reference{padding:12px}header{gap:6px}}
</style>
