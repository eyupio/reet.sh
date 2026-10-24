<script lang="ts">
 import {tick,type Snippet} from 'svelte'
 import {saveProfile} from '../api'
 import ConfirmDialog from '../ConfirmDialog.svelte'
 import CommandLibrary from './CommandLibrary.svelte'
 import QuickTerminalButton from './QuickTerminalButton.svelte'
 import {renameTerminal,endTerminal,validTerminalTitle,type TerminalMachine,type TerminalSession} from './api'
 let {machine,session,disabled=false,removeDefault=false,ondefault,onrefresh,children}:{machine:TerminalMachine;session?:TerminalSession;disabled?:boolean;removeDefault?:boolean;ondefault?:(value:boolean)=>void;onrefresh?:()=>void;children:Snippet<[(event:MouseEvent|KeyboardEvent)=>void,Snippet]>}=$props()
 let open=$state<{x:number;y:number}|null>(null),menu=$state<HTMLElement>(),quick=$state<{start:()=>Promise<void>}>(),returnFocus:HTMLElement|undefined
 let action=$state<'rename'|'end'|null>(null),name=$state(''),remove=$state(false),saveDefault=$state(false),busy=$state(false),error=$state(''),commands=$state(false)
 const endKeys=new Map<string,{key:string;remove:boolean}>()
 const ready=$derived(machine.ready&&machine.serverEnabled!==false&&!disabled)
 const url=$derived(`/terminals?agent=${encodeURIComponent(machine.agentId)}${session?`&session=${encodeURIComponent(session.id)}`:''}`)
 async function show(event:MouseEvent|KeyboardEvent){event.preventDefault();event.stopPropagation();returnFocus=event.currentTarget as HTMLElement;const b=returnFocus.getBoundingClientRect();open={x:Math.max(8,Math.min('clientX'in event&&event.clientX?event.clientX:b.left,innerWidth-248)),y:Math.max(8,Math.min('clientY'in event&&event.clientY?event.clientY:b.bottom,innerHeight-(session?310:220)))};await tick();menu?.querySelector<HTMLElement>('a,button:not(:disabled)')?.focus()}
 function close(focus=true){open=null;if(focus)returnFocus?.focus({preventScroll:true})}
 function key(event:KeyboardEvent){if(event.key==='Escape'){event.preventDefault();close();return}if(event.key==='Tab'){close(false);return}if(!['ArrowDown','ArrowUp','Home','End'].includes(event.key))return;event.preventDefault();const items=[...menu!.querySelectorAll<HTMLElement>('a[href],button:not(:disabled)')],at=items.indexOf(document.activeElement as HTMLElement);items[event.key==='Home'?0:event.key==='End'?items.length-1:(at+(event.key==='ArrowDown'?1:-1)+items.length)%items.length]?.focus()}
 function begin(kind:'rename'|'end'){if(!session||busy)return;close(false);error='';action=kind;name=session.title;remove=endKeys.get(session.id)?.remove??(session.state==='end_requested'?session.removeHistory??false:removeDefault);saveDefault=false}
 async function perform(){
  if(!session||busy||!action)return;const intended=session,kind=action,title=name.trim();if(kind==='rename'&&!validTerminalTitle(title)){error='Use a session name of 1–80 characters without control characters.';return}
  busy=true;error=''
  try{if(kind==='rename')await renameTerminal(intended.id,title);else{let op=endKeys.get(intended.id);if(!op){op={key:crypto.randomUUID(),remove};endKeys.set(intended.id,op)}await endTerminal(intended.id,op.key,op.remove);if(saveDefault){try{await saveProfile({terminalRemoveOnClose:op.remove});ondefault?.(op.remove)}catch{error='Stop requested, but the account default could not be saved. Check Account → Profile.'}}}action=null;onrefresh?.()}
  catch(e){error=e instanceof Error?e.message:'The action is unconfirmed. Retry checks the same session.'}
  finally{busy=false}
 }
</script>
<svelte:window onpointerdown={event=>{if(open&&!menu?.contains(event.target as Node))close(false)}} onblur={()=>close(false)}/>
{#snippet controls()}<div class="card-actions"><QuickTerminalButton bind:this={quick} agent={machine.agentId} host={machine.host} {ready} disabled={busy}/><button class="more" aria-label={`Actions for ${session?`${session.title} on `:''}${machine.host}`} title="Terminal actions · right-click the card" aria-haspopup="menu" aria-expanded={!!open} onclick={show} oncontextmenu={show}>⋮</button>{#if error&&!action}<span class="error" role="alert">{error}</span>{/if}</div>{/snippet}
{@render children(show,controls)}
{#if open}<div class="card-menu" role="menu" aria-label={`Terminal actions for ${session?`${session.title} on `:''}${machine.host}`} tabindex="-1" bind:this={menu} style={`left:${open.x}px;top:${open.y}px`} onkeydown={key}>
 <p>{session?.title||machine.host}<small>{session?machine.host:'Machine terminal actions'}</small></p>
 <a role="menuitem" href={url}>{session?'Continue session':'Open terminals'}</a>
 <button role="menuitem" disabled={!ready||busy} onclick={()=>{close(false);void quick?.start()}}>New terminal</button>
 {#if session}<button role="menuitem" disabled={disabled||busy} onclick={()=>begin('rename')}>Rename session…</button><button role="menuitem" class="danger" disabled={disabled||busy} onclick={()=>begin('end')}>{session.state==='end_requested'?'Retry stop request…':'Stop session…'}</button>{/if}
 <button role="menuitem" onclick={()=>{close(false);commands=true}}>Saved commands…</button>
 <a role="menuitem" href={`/machines?machine=${encodeURIComponent(machine.host)}`}>Manage machine…</a>
 <button role="menuitem" onclick={()=>close()}>Close menu</button>
</div>{/if}
{#if commands}<CommandLibrary onclose={()=>commands=false} context={machine.host}/>{/if}
{#if action}<ConfirmDialog title={action==='rename'?'Rename session':`End ${session?.title}?`} body={action==='rename'?'Change the display name. The running shell stays the same.':'This stops the shell and its jobs. An offline request remains pending until the machine confirms. Files are kept.'} confirmLabel={busy?'Sending…':action==='rename'?'Save name':'End session'} danger={action==='end'} onconfirm={()=>void perform()} oncancel={()=>action=null}>
 {#if error}<p class="error" role="alert">{error}</p>{/if}
 {#if action==='rename'}<label>Session name<input bind:value={name} maxlength="80" disabled={busy} required/></label>{:else}<div class="end-options"><label><input type="checkbox" bind:checked={remove} disabled={busy||!!session&&endKeys.has(session.id)||session?.state==='end_requested'}/> Remove from history when this session ends</label><p>Pending sessions stay visible until closure is confirmed.</p><label><input type="checkbox" bind:checked={saveDefault} disabled={busy}/> Use this as my account default</label></div>{/if}
</ConfirmDialog>{/if}
<style>
 .card-actions{display:flex;align-items:center;gap:4px;position:relative;z-index:2;flex-wrap:wrap}.more{width:28px;min-height:32px;border:1px solid transparent;border-radius:6px;color:var(--muted);background:none;font-size:1.2rem;cursor:pointer}.more:hover{border-color:var(--line);background:var(--tint);color:var(--accent)}.card-menu{position:fixed;z-index:110;width:240px;max-height:calc(100dvh - 16px);overflow:auto;border:1px solid var(--line);border-radius:10px;padding:6px;background:var(--card);box-shadow:0 12px 35px rgb(0 0 0 / .2);display:grid;gap:2px}.card-menu p{font-size:.74rem;font-weight:600;margin:5px 8px 8px;overflow-wrap:anywhere}.card-menu small{display:block;font-size:.63rem;font-weight:400;color:var(--muted);margin-top:4px}.card-menu a,.card-menu button{display:block;min-height:32px;border:0;border-radius:5px;padding:7px 9px;text-align:left;font-size:.72rem;text-decoration:none;color:var(--ink);background:none;cursor:pointer}.card-menu a:hover,.card-menu button:hover,.card-menu a:focus-visible,.card-menu button:focus-visible{background:var(--tint)}.card-menu button:disabled{opacity:.4;cursor:default}.card-menu .danger,.error{color:var(--bad)}.error{font-size:.72rem;line-height:1.5;overflow-wrap:anywhere}.end-options{display:grid;gap:12px;font-size:.8rem}.end-options label{display:flex;gap:8px;align-items:flex-start}.end-options input{margin-top:3px;accent-color:var(--accent)}.end-options p{margin:0;font-size:.72rem;color:var(--muted)}label{display:grid;gap:6px;font-size:.8rem}@media(pointer:coarse){.more,.card-menu a,.card-menu button{min-height:40px}}
</style>
