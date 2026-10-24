<script lang="ts">
 import {onMount} from 'svelte'
 import WorkspaceDialog from './WorkspaceDialog.svelte'
 import {savedCommands,saveCommand,removeCommand,TerminalError,type TerminalCommand} from './api'
 let {onclose,oninsert,canInsert=false,context=''}:{onclose:()=>void;oninsert?:(item:TerminalCommand)=>void;canInsert?:boolean;context?:string}=$props()
 let items=$state<TerminalCommand[]>([]),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),query=$state('')
 let view=$state<'list'|'edit'|'delete'>('list'),editing=$state<TerminalCommand|null>(null),name=$state(''),command=$state(''),description=$state(''),conflict=$state(false),search=$state<HTMLInputElement>()
 let alive=true;const abort=new AbortController()
 const needsReview=(text:string)=>/[\r\n\t]/.test(text)
 const filtered=$derived(items.filter(item=>`${item.name} ${item.command} ${item.description}`.toLowerCase().includes(query.toLowerCase())))
 const valid=$derived(name.trim().length>0&&Array.from(name.trim()).length<=80&&command.trim().length>0&&new TextEncoder().encode(command).length<=16384&&Array.from(description).length<=500)
 $effect(()=>{if(view==='list')search?.focus()})
 function failed(e:unknown){error=e instanceof Error?e.message:'The command library is unavailable.';if(e instanceof TerminalError&&[401,403].includes(e.status)){items=[];editing=null;name='';command='';description='';view='list'}conflict=e instanceof TerminalError&&e.status===409}
 async function refresh(){loading=true;error='';try{const result=await savedCommands(abort.signal);if(alive)items=result}catch(e){if(alive&&!abort.signal.aborted)failed(e)}finally{if(alive)loading=false}}
 onMount(()=>{void refresh();return()=>{alive=false;abort.abort()}})
 function edit(item:TerminalCommand|null){editing=item;name=item?.name||'';command=item?.command||'';description=item?.description||'';error='';notice='';conflict=false;view='edit'}
 async function save(){if(busy||!valid)return;busy=true;error='';try{const result=await saveCommand({name,command,description},editing||undefined);if(!alive)return;items=[...items.filter(item=>item.id!==result.id),result].sort((a,b)=>a.name.localeCompare(b.name));view='list';notice='Saved to your account.'}catch(e){if(alive)failed(e)}finally{if(alive)busy=false}}
 async function remove(){if(busy||!editing)return;const item=editing;busy=true;error='';try{await removeCommand(item);if(!alive)return;items=items.filter(existing=>existing.id!==item.id);view='list';notice='Command removed.'}catch(e){if(alive)failed(e)}finally{if(alive)busy=false}}
 async function loadLatest(){const id=editing?.id;await refresh();const latest=items.find(item=>item.id===id);if(latest)edit(latest);else{view='list';error='This command is no longer in your library.'}}
 async function copy(text:string){try{await navigator.clipboard.writeText(text);notice='Command copied.'}catch{error='Clipboard access was denied. Select and copy the command text.'}}
</script>
<WorkspaceDialog title={view==='edit'?(editing?'Edit command':'New command'):view==='delete'?'Remove saved command?':'Saved commands'} placement="drawer" {onclose}>
 <div class="library">
  {#if error}<div class="message bad" role="alert">{error}{#if conflict}<button onclick={()=>void loadLatest()} disabled={busy}>Discard draft and load latest</button>{:else if view==='list'}<button onclick={()=>void refresh()}>Try again</button>{/if}</div>{/if}
  {#if notice}<p class="message" role="status">{notice}</p>{/if}
  {#if view==='list'}
   <div><p class="eyebrow">Your account. Every machine.</p><p class="intro">The commands you reach for, ready when you need them.</p>{#if context}<p class="target">Insert into <strong>{context}</strong></p>{/if}</div>
   <div class="library-actions"><label><span class="vh">Search saved commands</span><input type="search" aria-label="Search saved commands" bind:this={search} bind:value={query} placeholder="Find a command…" /></label><button class="btn primary" disabled={items.length>=200} onclick={()=>edit(null)}>New</button><button class="btn quiet" aria-label="Refresh saved commands" disabled={loading} onclick={()=>void refresh()}>↻</button></div>
   {#if loading}<p class="empty" role="status">Loading your command library…</p>{:else if !items.length&&!error}<div class="empty"><span aria-hidden="true">⌘</span><h3>A shortcut to your next task.</h3><p>Save a command once. Find it from any connected machine.</p><button class="btn" onclick={()=>edit(null)}>Save your first command</button></div>{:else if !filtered.length}<p class="empty">No commands match this search.</p>{:else}
    <ul class="command-list">{#each filtered as item (item.id)}<li>
     <div class="command-heading"><strong>{item.name}</strong><div><button class="icon-button" aria-label={`Edit ${item.name}`} onclick={()=>edit(item)}>✎</button><button class="icon-button" aria-label={`Remove ${item.name}`} onclick={()=>{editing=item;error='';conflict=false;view='delete'}}>×</button></div></div>
     {#if item.description}<p>{item.description}</p>{/if}<pre>{item.command}</pre>
     <div class="command-actions"><small>{needsReview(item.command)?'Multiple lines · paste review':'Insert, then press Enter when ready'}</small><button class="btn quiet" aria-label={`Copy ${item.name}`} onclick={()=>void copy(item.command)}>Copy</button>{#if oninsert}<button class="btn insert" disabled={!canInsert} aria-label={`Insert ${item.name}`} onclick={()=>oninsert?.(item)}>Insert ↗</button>{/if}</div>
    </li>{/each}</ul>
   {/if}
   <p class="footnote">{#if oninsert&&!canInsert}Connect a live terminal to insert a command. {/if}Single-line commands insert without an extra Enter. Multiple lines need paste review.</p>
  {:else if view==='edit'}
   <form onsubmit={e=>{e.preventDefault();void save()}}>
    <label>Name<input aria-label="Command name" bind:value={name} maxlength="80" required placeholder="Check disk space" disabled={busy}/></label>
    <label>Command<textarea aria-label="Saved command text" bind:value={command} required spellcheck="false" autocomplete="off" rows="6" placeholder="df -h" disabled={busy}></textarea></label>
    <label>Notes <span class="optional">optional</span><textarea aria-label="Command notes" bind:value={description} maxlength="500" rows="2" placeholder="When is this useful?" disabled={busy}></textarea></label>
    <p class="footnote">Saved privately to your reet account. Saving a command never runs it.</p>
    <div class="form-actions"><button type="button" class="btn" disabled={busy} onclick={()=>{view='list';error=''}}>Cancel</button><button type="button" class="btn quiet" onclick={()=>void copy(command)}>Copy draft</button><button class="btn primary" disabled={busy||!valid}>{busy?'Saving…':'Save command'}</button></div>
   </form>
  {:else if editing}
   <p class="intro">Remove <strong>{editing.name}</strong> from your saved commands?</p><pre class="delete-preview">{editing.command}</pre><div class="form-actions"><button class="btn" disabled={busy} onclick={()=>{view='list';error=''}}>Cancel</button><button class="btn primary" disabled={busy} onclick={()=>void remove()}>{busy?'Removing…':'Remove command'}</button></div>
  {/if}
 </div>
</WorkspaceDialog>
<style>
 .library{display:grid;gap:16px;min-width:0}.eyebrow{font-size:.63rem;letter-spacing:.1em;text-transform:uppercase;color:var(--accent);margin:0 0 8px}.intro{font-size:.83rem;line-height:1.6;color:var(--muted);margin:0}.target{font-size:.7rem;margin:10px 0 0;padding:8px 10px;border-radius:6px;background:var(--bg);color:var(--muted);overflow-wrap:anywhere}.target strong{color:var(--ink)}.library-actions{display:flex;gap:6px;align-items:center}.library-actions label{flex:1;min-width:0}.library-actions input{width:100%;padding:9px;font-size:.8rem}.library-actions .btn{padding:8px 10px;font-size:.75rem}.command-list{display:grid;gap:12px;list-style:none;margin:0;padding:0}.command-list li{border:1px solid var(--line);border-radius:10px;padding:12px;min-width:0}.command-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}.command-heading>strong{font-size:.83rem;overflow-wrap:anywhere}.command-heading>div{display:flex;gap:3px}.icon-button{border:0;background:transparent;color:var(--muted);cursor:pointer;min-height:28px;min-width:28px;border-radius:5px;font:1.1rem var(--sans)}.icon-button:hover{color:var(--accent);background:var(--tint)}li>p{font-size:.73rem;color:var(--muted);line-height:1.5;margin:4px 0 9px}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:.73rem/1.55 var(--mono);padding:10px;background:var(--bg);border:1px solid var(--line);border-radius:6px;margin:10px 0;max-height:120px;overflow:auto}.command-actions{display:flex;align-items:center;gap:4px;flex-wrap:wrap}.command-actions small{margin-right:auto;font-size:.61rem;color:var(--muted)}.command-actions .btn{font-size:.7rem;padding:5px 7px;min-height:28px}.insert{color:var(--accent);border-color:color-mix(in srgb,var(--accent) 40%,var(--line))}.footnote{font-size:.68rem;line-height:1.6;color:var(--muted);margin:0}.message{font-size:.76rem;padding:10px;border-radius:6px;background:var(--tint);margin:0;overflow-wrap:anywhere}.message button{display:block;margin-top:7px;border:0;background:transparent;color:inherit;text-decoration:underline;cursor:pointer}.bad{color:var(--bad)}.empty{text-align:center;padding:26px 10px;color:var(--muted);font-size:.8rem}.empty>span{font:2.5rem var(--mono);color:var(--accent)}.empty h3{font:500 1.3rem var(--serif);color:var(--ink);margin:12px 0}.empty p{line-height:1.6}.empty .btn{margin-top:8px}form{display:grid;gap:16px}form label{display:grid;gap:7px;font-size:.78rem}form input,form textarea{width:100%;font-size:.8rem}form textarea[aria-label="Saved command text"]{font-family:var(--mono);line-height:1.65;resize:vertical}.optional{color:var(--muted);font-size:.65rem}.form-actions{display:flex;justify-content:flex-end;gap:6px;flex-wrap:wrap}.form-actions .btn{font-size:.75rem}.delete-preview{max-height:240px}.vh{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
