<script lang="ts">
 import {onMount} from 'svelte'
 import WorkspaceDialog from './WorkspaceDialog.svelte'
 import TerminalIcon from './TerminalIcon.svelte'
 import {savedCommands,saveCommand,removeCommand,TerminalError,type TerminalCommand} from './api'
 let {onclose,oninsert,canInsert=false,context=''}:{onclose:()=>void;oninsert?:(item:TerminalCommand)=>void;canInsert?:boolean;context?:string}=$props()
 let items=$state<TerminalCommand[]>([]),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),query=$state('')
 let view=$state<'list'|'edit'|'delete'|'detail'>('list'),editing=$state<TerminalCommand|null>(null),name=$state(''),command=$state(''),description=$state(''),conflict=$state(false),search=$state<HTMLInputElement>()
 let menu=$state<{item:TerminalCommand;x:number;y:number}|null>(null),menuEl=$state<HTMLDivElement>(),menuTrigger=$state<HTMLElement|null>(null)
 let alive=true;const abort=new AbortController()
 const needsReview=(text:string)=>/[\r\n\t]/.test(text)
 const filtered=$derived(items.filter(item=>`${item.name} ${item.command} ${item.description}`.toLowerCase().includes(query.toLowerCase())))
 const valid=$derived(name.trim().length>0&&Array.from(name.trim()).length<=80&&command.trim().length>0&&new TextEncoder().encode(command).length<=16384&&Array.from(description).length<=500)
 $effect(()=>{if(view==='list')search?.focus()})
 $effect(()=>{if(menu&&menuEl)menuEl.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()})
 function failed(e:unknown){error=e instanceof Error?e.message:'The command library is unavailable.';if(e instanceof TerminalError&&[401,403].includes(e.status)){items=[];editing=null;name='';command='';description='';view='list'}conflict=e instanceof TerminalError&&e.status===409}
 async function refresh(){loading=true;error='';try{const result=await savedCommands(abort.signal);if(alive)items=result}catch(e){if(alive&&!abort.signal.aborted)failed(e)}finally{if(alive)loading=false}}
 onMount(()=>{void refresh();return()=>{alive=false;abort.abort()}})
 function edit(item:TerminalCommand|null){closeMenu(false);editing=item;name=item?.name||'';command=item?.command||'';description=item?.description||'';error='';notice='';conflict=false;view='edit'}
 function openMenu(item:TerminalCommand,x:number,y:number,trigger:HTMLElement|null){
  const width=200,height=260,pad=8
  menu={item,x:Math.max(pad,Math.min(x,innerWidth-width-pad)),y:Math.max(pad,Math.min(y,innerHeight-height-pad))}
  menuTrigger=trigger
 }
 function rowMenu(event:MouseEvent,item:TerminalCommand){event.preventDefault();openMenu(item,event.clientX,event.clientY,null)}
 function moreMenu(event:MouseEvent,item:TerminalCommand){
  event.preventDefault();event.stopPropagation()
  const rect=(event.currentTarget as HTMLElement).getBoundingClientRect()
  openMenu(item,rect.right-200,rect.bottom+4,event.currentTarget as HTMLElement)
 }
 function closeMenu(restore=true){const trigger=menuTrigger;menu=null;menuTrigger=null;if(restore)queueMicrotask(()=>(trigger||search)?.focus())}
 function menuKey(event:KeyboardEvent){
  if(event.key==='Escape'){event.preventDefault();closeMenu();return}
  if(!['ArrowDown','ArrowUp','Home','End'].includes(event.key))return
  const buttons=Array.from(menuEl?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')||[])
  if(!buttons.length)return
  const current=buttons.indexOf(document.activeElement as HTMLButtonElement)
  let next=current
  if(event.key==='ArrowDown')next=current<0?0:(current+1)%buttons.length
  else if(event.key==='ArrowUp')next=current<0?buttons.length-1:(current-1+buttons.length)%buttons.length
  else if(event.key==='Home')next=0
  else next=buttons.length-1
  buttons[next]?.focus();event.preventDefault()
 }
 function insertFromMenu(){if(!menu)return;const item=menu.item;closeMenu(false);oninsert?.(item)}
 function copyFromMenu(){if(!menu)return;const text=menu.item.command;closeMenu();void copy(text)}
 function viewFromMenu(){if(!menu)return;const item=menu.item;closeMenu(false);editing=item;error='';notice='';view='detail'}
 function editFromMenu(){if(!menu)return;const item=menu.item;closeMenu(false);edit(item)}
 function askRemove(item:TerminalCommand){closeMenu(false);editing=item;error='';notice='';conflict=false;view='delete'}
 function removeFromMenu(){if(menu)askRemove(menu.item)}
 async function save(){if(busy||!valid)return;busy=true;error='';try{const result=await saveCommand({name,command,description},editing||undefined);if(!alive)return;items=[...items.filter(item=>item.id!==result.id),result].sort((a,b)=>a.name.localeCompare(b.name));view='list';notice='Saved to your account.'}catch(e){if(alive)failed(e)}finally{if(alive)busy=false}}
 async function remove(){if(busy||!editing)return;const item=editing;busy=true;error='';try{await removeCommand(item);if(!alive)return;items=items.filter(existing=>existing.id!==item.id);view='list';notice='Command removed.'}catch(e){if(alive)failed(e)}finally{if(alive)busy=false}}
 async function loadLatest(){const id=editing?.id;await refresh();const latest=items.find(item=>item.id===id);if(latest)edit(latest);else{view='list';error='This command is no longer in your library.'}}
 async function copy(text:string){try{await navigator.clipboard.writeText(text);notice='Command copied.'}catch{error='Clipboard access was denied. Select and copy the command text.'}}
</script>
<svelte:window onresize={()=>closeMenu(false)} />
<WorkspaceDialog title={view==='edit'?(editing?'Edit command':'New command'):view==='detail'?'Command details':view==='delete'?'Remove saved command?':'Saved commands'} placement="drawer" {onclose}>
 <div class="library">
  {#if error}<div class="message bad" role="alert">{error}{#if conflict}<button onclick={()=>void loadLatest()} disabled={busy}>Discard draft and load latest</button>{:else if view==='list'}<button onclick={()=>void refresh()}>Try again</button>{/if}</div>{/if}
  {#if notice}<p class="message" role="status">{notice}</p>{/if}
  {#if view==='list'}
   <div><p class="eyebrow">Your account. Every machine.</p><p class="intro">The commands you reach for, ready when you need them.</p>{#if context}<p class="target">Insert into <strong>{context}</strong></p>{/if}</div>
   <div class="command-toolbar">
    <button class="btn primary new-command" disabled={items.length>=200} title={items.length>=200?'Your library is full':undefined} onclick={()=>edit(null)}><span aria-hidden="true">＋</span> New command</button>
    <button class="btn quiet refresh" aria-label="Refresh saved commands" title="Refresh saved commands" disabled={loading} onclick={()=>void refresh()}>↻</button>
   </div>
   <label class="search-field"><TerminalIcon name="search"/><span class="vh">Search saved commands</span><input type="search" aria-label="Search saved commands" bind:this={search} bind:value={query} placeholder="Find a command…" /></label>
   {#if loading}<p class="empty" role="status">Loading your command library…</p>{:else if !items.length&&!error}<div class="empty"><span aria-hidden="true">⌘</span><h3>A shortcut to your next task.</h3><p>Save a command once. Find it from any connected machine.</p><button class="btn" onclick={()=>edit(null)}>Save your first command</button></div>{:else if !filtered.length}<div class="empty"><p>No commands match “{query}”.</p><button class="btn" onclick={()=>{query='';search?.focus()}}>Clear search</button></div>{:else}
    <ul class="command-list">{#each filtered as item (item.id)}<li oncontextmenu={(event)=>rowMenu(event,item)}>
     <div class="command-main"><div class="command-copy">
      <div class="command-heading"><strong title={item.name}>{item.name}</strong>{#if needsReview(item.command)}<span class="tag" title="Multiple lines insert with paste review">multi-line</span>{/if}</div>
      {#if item.description}<p class="description" title={item.description}>{item.description}</p>{/if}
      <code class="command-preview" title={item.command}>{item.command}</code>
     </div><div class="row-actions">
      {#if oninsert}<button type="button" class="btn insert" disabled={!canInsert} aria-label={`Insert ${item.name}`} title={canInsert?`Insert ${item.name} into the terminal`:'Connect a live terminal to insert'} onclick={()=>oninsert?.(item)}>Insert</button>{/if}
      <button type="button" class="btn quiet more" aria-label={`More actions for ${item.name}`} aria-haspopup="menu" aria-expanded={menu?.item.id===item.id} title="More actions" onclick={(event)=>moreMenu(event,item)}>⋯</button>
     </div></div>
    </li>{/each}</ul>
   {/if}
   <p class="footnote">{#if oninsert&&!canInsert}Connect a live terminal to insert a command. {/if}Right-click a command for more actions. Insert places text without running it; multiple lines need paste review.</p>
  {:else if view==='edit'}
   <form onsubmit={e=>{e.preventDefault();void save()}}>
    <label>Name<input aria-label="Command name" bind:value={name} maxlength="80" required placeholder="Check disk space" disabled={busy}/></label>
    <label>Command<textarea aria-label="Saved command text" bind:value={command} required spellcheck="false" autocomplete="off" rows="6" placeholder="df -h" disabled={busy}></textarea></label>
    <label>Notes <span class="optional">optional</span><textarea aria-label="Command notes" bind:value={description} maxlength="500" rows="2" placeholder="When is this useful?" disabled={busy}></textarea></label>
    <p class="footnote">Saved privately to your reet account. Saving a command never runs it.</p>
    <div class="form-actions"><button type="button" class="btn" disabled={busy} onclick={()=>{view='list';error=''}}>Cancel</button><button type="button" class="btn quiet" onclick={()=>void copy(command)}>Copy draft</button><button class="btn primary" disabled={busy||!valid}>{busy?'Saving…':'Save command'}</button></div>
   </form>
  {:else if view==='detail'&&editing}
   {@const item=editing}
   <div class="detail"><div><p class="eyebrow">Saved command</p><h3>{item.name}</h3>{#if item.description}<p class="intro">{item.description}</p>{/if}</div><pre class="full-command">{item.command}</pre><div class="form-actions"><button type="button" class="btn" onclick={()=>{view='list';error=''}}>Back</button><button type="button" class="btn quiet" onclick={()=>void copy(item.command)}>Copy</button><button type="button" class="btn" onclick={()=>edit(item)}>Edit</button>{#if oninsert}<button type="button" class="btn primary" disabled={!canInsert} title={canInsert?'':'Connect a live terminal first'} onclick={()=>oninsert?.(item)}>Insert command</button>{/if}</div></div>
  {:else if editing}
   <p class="intro">Remove <strong>{editing.name}</strong> from your saved commands?</p><pre class="delete-preview">{editing.command}</pre><div class="form-actions"><button class="btn" disabled={busy} onclick={()=>{view='list';error=''}}>Cancel</button><button class="btn primary" disabled={busy} onclick={()=>void remove()}>{busy?'Removing…':'Remove command'}</button></div>
  {/if}
  {#if menu}
   <button type="button" class="menu-backdrop" tabindex="-1" aria-label="Close command actions" onclick={()=>closeMenu()} oncontextmenu={(event)=>{event.preventDefault();closeMenu()}}></button>
   <div class="command-menu" role="menu" aria-label={`Actions for ${menu.item.name}`} tabindex="-1" bind:this={menuEl} style={`left:${menu.x}px;top:${menu.y}px`} onkeydown={menuKey}>
    <p class="menu-title">{menu.item.name}</p>
    <button type="button" role="menuitem" disabled={!oninsert||!canInsert} title={canInsert?'':'Connect a live terminal first'} onclick={insertFromMenu}>Insert command</button>
    <button type="button" role="menuitem" onclick={copyFromMenu}>Copy command</button>
    <button type="button" role="menuitem" onclick={viewFromMenu}>View full command</button>
    <button type="button" role="menuitem" onclick={editFromMenu}>Edit command</button>
    <div class="menu-sep" role="separator"></div>
    <button type="button" role="menuitem" class="danger" onclick={removeFromMenu}>Remove command</button>
   </div>
  {/if}
 </div>
</WorkspaceDialog>
<style>
 .library{display:grid;gap:14px;min-width:0}
 .eyebrow{font-size:.63rem;letter-spacing:.1em;text-transform:uppercase;color:var(--accent);margin:0 0 8px}
 .intro{font-size:.83rem;line-height:1.6;color:var(--muted);margin:0}
 .target{font-size:.7rem;margin:10px 0 0;padding:8px 10px;border-radius:6px;background:var(--bg);color:var(--muted);overflow-wrap:anywhere}
 .target strong{color:var(--ink)}
 .command-toolbar{display:flex;align-items:center;gap:6px}
 .new-command{flex:1;min-height:38px;font-size:.78rem}
 .refresh{flex:none;min-width:38px;min-height:38px;padding:0 10px;font-size:1rem}
 .search-field{display:flex;align-items:center;gap:8px;min-height:40px;padding:0 10px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--muted)}
 .search-field:focus-within{border-color:var(--accent);outline:2px solid color-mix(in srgb,var(--accent) 24%,transparent);outline-offset:1px}
 .search-field input{flex:1;min-width:0;width:100%;min-height:38px;padding:8px 0;border:0;background:transparent;font-size:.8rem}
 .search-field input:focus,.search-field input:focus-visible{outline:0;border:0}
 .command-list{display:grid;gap:0;list-style:none;margin:0;padding:0;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:var(--card)}
 .command-list li{padding:10px 12px;border-bottom:1px solid var(--line);min-width:0}
 .command-list li:last-child{border-bottom:0}
 .command-list li:hover,.command-list li:focus-within{background:var(--bg)}
 .command-main{display:flex;align-items:flex-start;gap:8px}
 .command-copy{display:grid;gap:3px;flex:1;min-width:0}
 .command-heading{display:flex;align-items:center;gap:7px;min-width:0}
 .command-heading strong{font-size:.82rem;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .tag{flex:none;padding:1px 5px;border:1px solid var(--line);border-radius:999px;color:var(--muted);font-size:.56rem;letter-spacing:.04em;text-transform:uppercase}
 .description{margin:0;color:var(--muted);font-size:.72rem;line-height:1.45;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .command-preview{display:block;padding:0;border:0;background:none;color:color-mix(in srgb,var(--good) 72%,var(--ink));font:.73rem/1.5 var(--mono);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
 .row-actions{display:flex;align-items:center;gap:2px;flex:none}
 .row-actions .btn{min-height:30px;padding:4px 9px;font-size:.7rem}
 .insert{color:var(--accent);border-color:color-mix(in srgb,var(--accent) 40%,var(--line))}
 .more{min-width:30px;padding:0 7px;font-size:1rem;line-height:1}
 .footnote{font-size:.68rem;line-height:1.55;color:var(--muted);margin:0}
 .message{font-size:.76rem;padding:10px;border-radius:6px;background:var(--tint);margin:0;overflow-wrap:anywhere}
 .message button{display:block;margin-top:7px;border:0;background:transparent;color:inherit;text-decoration:underline;cursor:pointer}
 .bad{color:var(--bad)}
 .empty{text-align:center;padding:22px 10px;color:var(--muted);font-size:.8rem}
 .empty>span{font:2.5rem var(--mono);color:var(--accent)}
 .empty h3{font:500 1.3rem var(--serif);color:var(--ink);margin:12px 0}
 .empty p{line-height:1.6}
 .empty .btn{margin-top:8px}
 form{display:grid;gap:16px}
 form label{display:grid;gap:7px;font-size:.78rem}
 form input,form textarea{width:100%;font-size:.8rem}
 form textarea[aria-label="Saved command text"]{font-family:var(--mono);line-height:1.65;resize:vertical}
 .optional{color:var(--muted);font-size:.65rem}
 .form-actions{display:flex;justify-content:flex-end;gap:6px;flex-wrap:wrap}
 .form-actions .btn{font-size:.75rem}
 .detail{display:grid;gap:14px;min-width:0}
 .detail h3{margin:0;font:500 1.2rem var(--serif);color:var(--ink);overflow-wrap:anywhere}
 .full-command{white-space:pre-wrap;overflow-wrap:anywhere;font:.76rem/1.6 var(--mono);padding:12px;background:var(--bg);border:1px solid var(--line);border-radius:8px;margin:0;max-height:320px;overflow:auto}
 .delete-preview{white-space:pre-wrap;overflow-wrap:anywhere;font:.76rem/1.6 var(--mono);padding:12px;background:var(--bg);border:1px solid var(--line);border-radius:8px;margin:0;max-height:240px;overflow:auto}
 .vh{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
 .menu-backdrop{position:fixed;inset:0;z-index:40;padding:0;border:0;background:transparent;cursor:default}
 .command-menu{position:fixed;z-index:41;display:grid;gap:2px;width:200px;max-height:calc(100dvh - 16px);overflow:auto;padding:6px;border:1px solid var(--line);border-radius:10px;background:var(--card);box-shadow:0 16px 40px -16px rgb(0 0 0 / .45)}
 .menu-title{margin:5px 8px 6px;color:var(--muted);font-size:.68rem;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
 .command-menu button{display:flex;align-items:center;width:100%;min-height:34px;padding:6px 9px;border:0;border-radius:6px;background:transparent;color:var(--ink);font-size:.78rem;text-align:left;cursor:pointer}
 .command-menu button:hover:not(:disabled),.command-menu button:focus-visible{background:var(--tint);outline:0}
 .command-menu button:disabled{opacity:.45;cursor:default}
 .command-menu .danger{color:var(--bad)}
 .menu-sep{height:1px;margin:4px 0;background:var(--line)}
</style>
