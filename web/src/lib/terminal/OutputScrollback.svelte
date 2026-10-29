<script lang="ts">
 import {onMount,tick} from 'svelte'
 import {terminalHistory,TerminalError} from './api'
 import {copyText} from '../clipboard'
 let {session,onclose}:{session:string;onclose:()=>void}=$props()
 let text=$state(''),loading=$state(false),error=$state(''),loaded=$state(false),query=$state(''),match=$state(0),viewport=$state<HTMLElement>(),status=$state('')
 let alive=true;const abort=new AbortController()
 const parts=$derived(query?text.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')})`,'ig')):[text])
 const count=$derived(query?Math.floor(parts.length/2):0)
 async function refresh(){
  if(loading)return;loading=true;error=''
  try{const next=await terminalHistory(session,abort.signal);if(!alive)return;text=next;loaded=true;match=0;await tick();if(viewport)viewport.scrollTop=viewport.scrollHeight;viewport?.focus({preventScroll:true})}
  catch(e){if(alive&&!abort.signal.aborted){error=e instanceof Error?e.message:'Earlier output is unavailable.';if(e instanceof TerminalError&&[401,403].includes(e.status)){text='';loaded=false}}}
  finally{if(alive)loading=false}
 }
 async function find(delta:number){if(!count)return;match=(match+delta+count)%count;await tick();const item=viewport?.querySelectorAll('mark')[match];if(item&&viewport){viewport.scrollTop=(item as HTMLElement).offsetTop-viewport.clientHeight/2}}
 async function copyAll(){status=text?await copyText(text)?'Copied all earlier output.':'Clipboard access was denied. Use your browser’s copy command.':'There is nothing to copy yet.'}
 onMount(()=>{void refresh();return()=>{alive=false;abort.abort()}})
</script>
<section class="scrollback" aria-label="Earlier terminal output">
 <header><div class="heading"><strong>Earlier output</strong><span>Read-only · last 2,000 lines / 256 KiB</span></div><div class="actions"><button class="btn" type="button" onclick={()=>void copyAll()}>Copy all</button><button class="btn" onclick={onclose}>Back to live ↓</button></div></header>
 {#if status}<p class="notice" role="status">{status}</p>{/if}
 <form onsubmit={event=>{event.preventDefault();void find(1)}}><input aria-label="Find in earlier output" type="search" bind:value={query} maxlength="200" placeholder="Find in output…" oninput={()=>{match=0;void tick().then(()=>find(0))}}/><span aria-live="polite">{query?(count?`${match+1} / ${count}`:'No matches'):''}</span><button class="btn" type="button" aria-label="Previous output match" disabled={!count} onclick={()=>void find(-1)}>↑</button><button class="btn" aria-label="Next output match" disabled={!count}>↓</button><button class="btn" type="button" disabled={loading} onclick={()=>void refresh()}>Refresh</button></form>
 {#if error}<p class="notice" role="alert">{error}{loaded?' Showing the previous snapshot.':''}</p>{/if}
 {#if loading}<p class="notice" role="status">Reading saved output from this machine…</p>{/if}
 <!-- Native scroll region needs keyboard focus for Page Up/Down without sending shell input. -->
 <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
 <div class="output" role="region" aria-label="Scrollable terminal output" tabindex="0" bind:this={viewport}><pre>{#each parts as part,i}{#if query&&i%2}<mark class:current={(i-1)/2===match}>{part}</mark>{:else}{part}{/if}{/each}</pre>{#if loaded&&!text}<p>No saved output is available yet.</p>{/if}</div>
 <footer>Scroll, drag the bar, or use Page Up / Page Down. Your live shell keeps running; this snapshot does not update until you refresh.</footer>
</section>
<style>
 .scrollback{position:absolute;inset:0;display:flex;flex-direction:column;min-height:0;background:var(--terminal-bg);color:var(--terminal-fg);z-index:2}header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:10px 14px;border-bottom:1px solid color-mix(in srgb,var(--terminal-fg) 20%,transparent)}header div{display:grid;gap:3px}header .heading{min-width:0}header .heading strong{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}header .actions{display:flex;align-items:center;gap:6px;flex-shrink:0}strong{font-size:.8rem}header span,footer,.notice{font-size:.68rem;line-height:1.5;opacity:.85}form{display:flex;align-items:center;gap:6px;padding:8px 14px;flex-shrink:0}input{flex:1;min-width:0;width:100%;font-size:.74rem}form>span{font-size:.65rem;white-space:nowrap}.btn{font-size:.7rem;padding:5px 8px;min-height:30px;white-space:nowrap}.output{position:relative;flex:1;min-height:0;overflow:auto;scrollbar-gutter:stable;scrollbar-width:auto;scrollbar-color:color-mix(in srgb,var(--terminal-fg) 60%,transparent) color-mix(in srgb,var(--terminal-fg) 10%,transparent);overscroll-behavior:contain;padding:12px 14px;outline-offset:-3px;touch-action:pan-x pan-y}.output::-webkit-scrollbar{width:14px;height:14px}.output::-webkit-scrollbar-track{background:color-mix(in srgb,var(--terminal-fg) 10%,transparent)}.output::-webkit-scrollbar-thumb{background:color-mix(in srgb,var(--terminal-fg) 60%,transparent);border:3px solid var(--terminal-bg);border-radius:9px}pre{font:var(--terminal-font-size,14px)/1.45 "DejaVu Sans Mono","SFMono-Regular",Consolas,monospace;margin:0;white-space:pre;tab-size:8;min-height:100%;width:max-content;min-width:100%;user-select:text}mark{background:#e7c66e;color:#211b17}mark.current{background:#fc9c54;color:#211b17;outline:1px solid currentColor}footer{padding:7px 14px;border-top:1px solid color-mix(in srgb,var(--terminal-fg) 20%,transparent)}.notice{margin:0;padding:5px 14px}header,footer,.notice{flex-shrink:0}@media(max-width:650px){header{padding:8px}header span{font-size:.58rem}form{padding:6px;gap:4px}form>span{max-width:60px;overflow:hidden}.output{padding:10px 8px}.btn{font-size:.64rem;padding:5px 6px}footer{padding:6px 8px;font-size:.6rem}}
 @media(pointer:coarse){.btn{min-width:40px;min-height:40px}}
 @media(max-height:600px){header span,footer{display:none}header{padding:5px 8px}form{padding:4px 8px}.output{padding:6px 8px}}
</style>
