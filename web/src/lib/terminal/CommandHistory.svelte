<script lang="ts">
 import {onMount,untrack} from 'svelte'
 import WorkspaceDialog from './WorkspaceDialog.svelte'
 import {commandHistory,TerminalError,type CommandHistorySnapshot} from './api'
 let {session,context,supported=false,canInsert=false,onclose,oninsert,onoutput}:{session:string;context:string;supported?:boolean;canInsert?:boolean;onclose:()=>void;oninsert:(text:string,run:boolean)=>void;onoutput:()=>void}=$props()
 let snapshot=$state<CommandHistorySnapshot>({entries:[],sources:[],truncated:false,skipped:0}),query=$state(''),source=$state('all'),loading=$state(false),error=$state(''),notice=$state('')
 let alive=true;const abort=new AbortController()
 const matches=$derived(snapshot.entries.filter(item=>(source==='all'||item.source===source)&&item.command.toLowerCase().includes(query.toLowerCase())))
 async function refresh(){
  if(loading||!supported)return;loading=true;error='';notice=''
  try{const next=await commandHistory(session,abort.signal);if(alive)snapshot=next}catch(e){if(alive&&!abort.signal.aborted){error=e instanceof Error?e.message:'Command history is unavailable.';if(e instanceof TerminalError&&[401,403].includes(e.status))snapshot={entries:[],sources:[],truncated:false,skipped:0}}}finally{if(alive)loading=false}
 }
 $effect(()=>{if(supported)untrack(()=>void refresh())})
 onMount(()=>()=>{alive=false;abort.abort()})
 async function copy(text:string){try{await navigator.clipboard.writeText(text);notice='Command copied.'}catch{error='Clipboard access was denied. Select the command text and copy it.'}}
 function date(timestamp?:number){return timestamp?new Date(timestamp).toLocaleString():''}
</script>
<WorkspaceDialog title="Command history" placement="drawer" {onclose}>
 <div class="recall">
  <div><p class="eyebrow">Saved by your shell</p><p class="context">{context}</p><p class="intro">Find a previous command. Paste it to adjust, or review it before running.</p></div>
  {#if !supported}<div class="message"><strong>Update this machine’s reet agent for command recall.</strong><p>Saved Bash and Zsh history needs a compatible agent. Your existing terminal keeps working.</p><code>reet upgrade<br/>reet agent restart</code></div>
  {:else}
   <div class="filters"><label><span class="vh">Search command history</span><input type="search" aria-label="Search command history" bind:value={query} placeholder="Search saved shell commands…"/></label><button class="btn quiet" aria-label="Refresh command history" onclick={()=>void refresh()} disabled={loading}>↻</button></div>
   <label class="source">Source<select aria-label="Command history source" bind:value={source}><option value="all">All available shells</option><option value="bash">Bash · .bash_history</option><option value="zsh">Zsh · .zsh_history</option></select></label>
   {#if error}<p class="message bad" role="alert">{error}</p>{/if}{#if notice}<p class="message" role="status">{notice}</p>{/if}
   {#if loading}<p class="empty" role="status">Reading saved command history…</p>{:else if !snapshot.entries.length&&!error}<div class="empty"><h3>No saved commands yet.</h3><p>History comes from this OS user’s Bash/Zsh files. Your shell may save commands when its session ends.</p></div>{:else if !matches.length&&!error}<p class="empty">No commands match this search.</p>{:else}
    <ol class="entries" aria-label="Saved shell commands">{#each matches as entry,index(index)}<li><header><span>{entry.source==='bash'?'Bash':'Zsh'}</span>{#if entry.timestamp}<time datetime={new Date(entry.timestamp).toISOString()}>{date(entry.timestamp)}</time>{/if}</header><pre>{entry.command}</pre><div class="actions"><button class="btn quiet" aria-label={`Copy history command ${index+1}`} onclick={()=>void copy(entry.command)}>Copy</button><button class="btn" disabled={!canInsert} aria-label={`Paste history command ${index+1}`} onclick={()=>oninsert(entry.command,false)}>Paste to edit</button><button class="btn primary" disabled={!canInsert} aria-label={`Run history command ${index+1}`} onclick={()=>oninsert(entry.command,true)}>Run…</button></div></li>{/each}</ol>
   {/if}
   {#each snapshot.sources.filter(item=>!item.available) as item}<p class="source-note">{item.source==='bash'?'Bash':'Zsh'}: {item.reason}</p>{/each}
   {#if snapshot.truncated||snapshot.skipped}<p class="footnote">Showing up to 500 recent saved commands within the file limit.{snapshot.skipped?` ${snapshot.skipped} unsupported or oversized entries were skipped.`:''}</p>{/if}
   <p class="footnote">Read from this machine when requested; command history is not saved to your reet account or browser storage. Open shells may not have saved their latest commands yet; Ctrl+R at a Bash/Zsh prompt also searches current-session history. Paste adds no extra Enter; multiple lines need paste review.</p>
  {/if}
  <button class="output-link" onclick={onoutput}>Terminal output history →</button>
 </div>
</WorkspaceDialog>
<style>
 .recall{display:grid;gap:14px;min-width:0}.eyebrow{font-size:.63rem;letter-spacing:.1em;text-transform:uppercase;color:var(--accent-text);margin:0 0 8px}.context{font-size:.74rem;font-weight:600;color:var(--ink);overflow-wrap:anywhere;margin:0 0 8px}.intro,.footnote,.source-note{font-size:.72rem;line-height:1.7;color:var(--muted);margin:0}.filters{display:flex;gap:7px}.filters label{flex:1;min-width:0}.filters input{width:100%;font-size:.78rem;padding:9px}.filters button{padding:7px 10px}.source{display:flex;align-items:center;justify-content:space-between;gap:12px;font-size:.7rem;color:var(--muted)}.source select{max-width:75%;font-size:.73rem;padding:6px}.entries{display:grid;gap:12px;list-style:none;padding:0;margin:0}.entries li{min-width:0;border:1px solid var(--line);border-radius:10px;padding:12px;background:var(--card)}li header{display:flex;justify-content:space-between;gap:10px;font-size:.61rem;color:var(--muted)}li time{text-align:right}pre{font:.73rem/1.6 var(--mono);white-space:pre-wrap;overflow-wrap:anywhere;max-height:170px;overflow:auto;margin:10px 0;padding:10px;background:var(--bg);border-radius:6px;border:1px solid var(--line)}.actions{display:flex;gap:5px;justify-content:flex-end;flex-wrap:wrap}.actions .btn{font-size:.7rem;min-height:30px;padding:5px 8px}.message{padding:12px;background:var(--tint);font-size:.75rem;line-height:1.7;border-radius:8px;margin:0;overflow-wrap:anywhere}.message p{margin:8px 0}.message code{font:.7rem/1.7 var(--mono);user-select:all}.bad{color:var(--bad)}.empty{padding:20px 4px;text-align:center;color:var(--muted);font-size:.78rem;line-height:1.7}.empty h3{font:500 1.3rem var(--serif);color:var(--ink);margin:0 0 10px}.output-link{border:0;border-top:1px solid var(--line);padding:14px 0 0;background:none;color:var(--accent-text);font-size:.75rem;cursor:pointer;text-align:left}.output-link:hover{text-decoration:underline}.vh{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
