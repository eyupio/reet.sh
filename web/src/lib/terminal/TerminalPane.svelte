<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal, type ITheme } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { SearchAddon } from '@xterm/addon-search'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import '@xterm/xterm/css/xterm.css'
  import { TerminalConnection, type ConnectionState } from './connection'
  import { terminalHistory, type TerminalSession } from './api'

  let { session, host }: { session: TerminalSession; host: string } = $props()
  let element = $state<HTMLDivElement>()
  let connectionState = $state<ConnectionState>('connecting'), message = $state('')
  let connection: TerminalConnection | undefined
  let terminal: Terminal | undefined
  let search: SearchAddon | undefined, fitTerminal: (()=>void)|undefined
  let query = $state(''), searching = $state(false), found = $state(true), fontSize = $state(14), maximised = $state(false)
  let feedback = $state(''), snapshot = $state<string|null>(null), historyBusy = $state(false)
  let pendingPaste = $state<{text:string;generation:number}|null>(null), generation=0, alive=true
  const historyAbort=new AbortController()
  let modifier = $state<'Ctrl'|'Alt'|null>(null)
  function find(next=true) { found=!query||!!(next?search?.findNext(query,{caseSensitive:false,regex:false}):search?.findPrevious(query,{caseSensitive:false,regex:false})) }
  function changeFont(delta:number) { fontSize=Math.min(22,Math.max(10,fontSize+delta));if(terminal)terminal.options.fontSize=fontSize;try{localStorage.setItem('reet-terminal-font',String(fontSize))}catch{/* preferences are optional */}fitTerminal?.() }
  async function copy() { const text=terminal?.getSelection();if(!text){feedback='Select terminal text first.';return}try{await navigator.clipboard.writeText(text);feedback='Selection copied.'}catch{feedback='Clipboard access was denied. Use your browser’s copy command.'} }
  function paste(text:string) {
    if(!live){feedback='Reconnect before pasting. Nothing was sent.';return}
    if(new TextEncoder().encode(text).length>240*1024){feedback='Paste is too large. Limit it to 240 KiB.';return}
    if(/[\r\n\x00-\x08\x0b-\x1f\x7f]/.test(text)||text.length>2000)pendingPaste={text,generation}
    else terminal?.paste(text)
  }
  function confirmPaste() { const pending=pendingPaste;pendingPaste=null;if(!pending)return;if(!live||pending.generation!==generation){feedback='Connection changed. Paste again after reconnecting; nothing was sent.';return}terminal?.paste(pending.text);terminal?.focus() }
  async function readClipboard() {const started=generation;try{const text=await navigator.clipboard.readText();if(!alive)return;if(started!==generation){feedback='Connection changed. Paste again; nothing was sent.';return}paste(text)}catch{if(alive)feedback='Clipboard access was denied. Paste with your keyboard inside the terminal.'} }
  function key(value:string) {if(live){connection?.text(value);terminal?.focus()} }
  async function historySnapshot() { if(historyBusy)return;historyBusy=true;feedback='';try{const text=await terminalHistory(session.id,historyAbort.signal);if(alive)snapshot=text}catch(e){if(alive)feedback=e instanceof Error?e.message:'History is unavailable.'}finally{historyBusy=false} }
  onMount(()=>()=>historyAbort.abort())
  const live = $derived(connectionState === 'live')
  const stateLabel = $derived(({ connecting:'Connecting',resynchronising:'Restoring screen',live:'Live',reconnecting:'Reconnecting',detached:'Detached',taken_over:'Open elsewhere',ended:'Ended',lost:'Shell unavailable',access_denied:'Access denied' })[connectionState])

  onMount(() => {
    if (!element) return
    const fit = new FitAddon()
    try{const stored=Number(localStorage.getItem('reet-terminal-font'));if(stored>=10&&stored<=22)fontSize=stored}catch{/* optional preference */}
    const term = new Terminal({ cursorBlink:true,fontSize:14,fontFamily:'"DejaVu Sans Mono", "SFMono-Regular", Consolas, monospace',scrollback:5000,screenReaderMode:true,allowProposedApi:false,windowOptions:{},minimumContrastRatio:4.5 })
    terminal = term;term.options.fontSize=fontSize;term.loadAddon(fit);search=new SearchAddon();term.loadAddon(search);term.open(element)
    const onPaste=(event:ClipboardEvent)=>{event.preventDefault();event.stopImmediatePropagation();paste(event.clipboardData?.getData('text/plain')||'')}
    element.addEventListener('paste',onPaste,true)
    term.attachCustomKeyEventHandler(event=>{if(event.type==='keydown'&&(event.ctrlKey||event.metaKey)&&event.shiftKey&&event.key.toLowerCase()==='f'){event.preventDefault();searching=!searching;return false}return true})
    // No terminal-controlled clipboard writes or active OSC hyperlinks.
    const clipboard = term.parser.registerOscHandler(52, () => true)
    const links = term.parser.registerOscHandler(8, () => true)
    function theme() {
      const choice = document.documentElement.dataset.theme
      const dark = choice === 'dark' || !choice && matchMedia('(prefers-color-scheme: dark)').matches
      const colors: ITheme = dark ? { background:'#1a1816',foreground:'#efece6',cursor:'#fb923c',selectionBackground:'#68503d',black:'#171513',brightBlack:'#81766b' } : { background:'#fffdf9',foreground:'#25221f',cursor:'#c2410c',selectionBackground:'#f3d4bd',black:'#25221f',brightBlack:'#756e63' }
      term.options.theme = colors
    }
    theme()
    const observer = new MutationObserver(theme); observer.observe(document.documentElement,{attributes:true,attributeFilter:['data-theme']})
    const media = matchMedia('(prefers-color-scheme: dark)'); media.addEventListener('change',theme)
    let frame = 0
    function resize() { cancelAnimationFrame(frame); frame=requestAnimationFrame(()=>{if (!element?.clientWidth || !element.clientHeight) return;fit.fit();const cols=Math.min(500,Math.max(2,term.cols)),rows=Math.min(300,Math.max(1,term.rows));if(cols!==term.cols||rows!==term.rows)term.resize(cols,rows);connection?.resize(cols,rows)}) }
    const sizes = new ResizeObserver(resize); sizes.observe(element);fit.fit()
    fitTerminal=resize
    connection = new TerminalConnection({session:session.id,dimensions:()=>({cols:Math.min(500,Math.max(2,term.cols)),rows:Math.min(300,Math.max(1,term.rows))}),renderer:{write:(bytes,reset,done)=>{if(reset)term.reset();term.write(bytes,done)}},onstate:(next,detail)=>{generation++;modifier=null;connectionState=next;message=detail;term.options.disableStdin=next!=='live';if(next==='live')term.focus()}})
    term.options.disableStdin=true
    const input=term.onData((text)=>{let value=text;if(modifier==='Ctrl'&&/^[a-zA-Z@\[\]\\^_]$/.test(text))value=String.fromCharCode(text.toUpperCase().charCodeAt(0)&31);else if(modifier==='Alt')value='\x1b'+text;modifier=null;connection?.text(value)})
    const binary=term.onBinary((data)=>connection?.send(Uint8Array.from(data,c=>c.charCodeAt(0)&255)))
    const online=()=>connection?.recover(),visible=()=>{if(document.visibilityState==='visible'){resize();connection?.recover()}}
    window.addEventListener('online',online);document.addEventListener('visibilitychange',visible)
    void connection.connect()
    return ()=>{alive=false;element?.removeEventListener('paste',onPaste,true);search?.dispose();search=undefined;fitTerminal=undefined;connection?.dispose();connection=undefined;cancelAnimationFrame(frame);sizes.disconnect();observer.disconnect();media.removeEventListener('change',theme);window.removeEventListener('online',online);document.removeEventListener('visibilitychange',visible);input.dispose();binary.dispose();clipboard.dispose();links.dispose();term.dispose();terminal=undefined}
  })
</script>

<section class="pane" class:maximised aria-label={`Terminal on ${host}`} data-connection-state={connectionState}>
  <header><div class="identity"><span class="terminal-icon" aria-hidden="true">›_</span><strong>{session.title}</strong><span class="host">{host}</span></div><div class="controls"><span class="state" class:live><span aria-hidden="true">●</span> {stateLabel}</span>{#if live}<button type="button" onclick={()=>connection?.detach()}>Detach</button>{:else if connectionState==='taken_over'}<button type="button" class="continue" onclick={()=>void connection?.connect(true)}>Continue here</button>{:else if connectionState==='detached'}<button type="button" onclick={()=>void connection?.connect()}>Reconnect</button>{:else if connectionState==='reconnecting'}<button type="button" onclick={()=>connection?.recover()}>Retry now</button>{/if}</div></header>
  {#if message}<p class="connection-message" role="status">{message}</p>{/if}
  <div class="tools" aria-label="Terminal tools"><button onclick={()=>searching=!searching} aria-expanded={searching}>Search</button><button onclick={()=>void copy()}>Copy selection</button><button onclick={()=>void readClipboard()} disabled={!live}>Paste</button><button onclick={()=>changeFont(-1)} disabled={fontSize<=10} aria-label="Decrease terminal font">A−</button><span aria-label="Terminal font size">{fontSize}</span><button onclick={()=>changeFont(1)} disabled={fontSize>=22} aria-label="Increase terminal font">A+</button><button onclick={()=>void historySnapshot()} disabled={historyBusy}>{historyBusy?'Loading…':'Recent history'}</button><button onclick={()=>{maximised=!maximised;fitTerminal?.()}} aria-pressed={maximised}>{maximised?'Restore size':'Maximise'}</button></div>
  {#if searching}<form class="tools" onsubmit={e=>{e.preventDefault();find()}}><label>Find in terminal <input bind:value={query} maxlength="200" oninput={()=>find()} /></label><button>Next match</button><button type="button" onclick={()=>find(false)}>Previous match</button><button type="button" onclick={()=>{searching=false;search?.clearDecorations();terminal?.focus()}}>Close search</button><span role="status">{found?'':'No matches'}</span></form>{/if}
  {#if feedback}<p class="connection-message" role="status">{feedback}</p>{/if}
  <div class="screen" bind:this={element} aria-label="Remote terminal screen"></div>
  <div class="tools mobile-keys" aria-label="Terminal extra keys">{#each [['Esc','\x1b'],['Tab','\t'],['↑','\x1b[A'],['↓','\x1b[B'],['←','\x1b[D'],['→','\x1b[C'],['Ctrl+C','\x03'],['Enter','\r']] as [label,value]}<button disabled={!live} onpointerdown={e=>e.preventDefault()} onclick={()=>key(value)} aria-label={`Send ${label}`}>{label}</button>{/each}{#each ['Ctrl','Alt'] as mod}<button disabled={!live} aria-pressed={modifier===mod} onpointerdown={e=>e.preventDefault()} onclick={()=>{modifier=modifier===mod?null:mod as 'Ctrl'|'Alt';terminal?.focus()}}>{mod}</button>{/each}</div>
  <footer><span>{live?'Connected privately · input goes directly to this shell':'Input paused · disconnected keystrokes are never replayed'}</span><button type="button" disabled={!live} onclick={()=>terminal?.focus()}>Focus terminal</button></footer>
</section>
{#if pendingPaste}<ConfirmDialog title="Paste into the remote shell?" body="This text contains multiple lines, control characters, or a large block. It may execute commands immediately." details={pendingPaste.text.slice(0,2000)+(pendingPaste.text.length>2000?'\n… preview truncated':'')} confirmLabel="Paste text" onconfirm={confirmPaste} oncancel={()=>{pendingPaste=null;terminal?.focus()}} />{/if}
{#if snapshot!==null}<ConfirmDialog title="Recent terminal history" body="Read-only snapshot, up to 2,000 lines / 256 KiB. It is separate from the live screen and may contain sensitive output." confirmLabel="Close history" onconfirm={()=>snapshot=null} oncancel={()=>snapshot=null}><pre class="snapshot">{snapshot}</pre></ConfirmDialog>{/if}

<style>
  .tools{display:flex;gap:6px;align-items:center;flex-wrap:wrap;padding:8px 12px;border-bottom:1px solid var(--line);font-size:.72rem}.tools label{display:flex;gap:8px;align-items:center;min-width:0}.tools input{min-width:0;max-width:180px;font-size:.8rem}.tools button[aria-pressed=true]{border-color:var(--accent);color:var(--accent)}.mobile-keys{border-top:1px solid var(--line);border-bottom:0}.snapshot{white-space:pre-wrap;overflow-wrap:anywhere;max-height:55dvh;overflow:auto;font:.75rem/1.5 monospace}.pane.maximised{position:fixed;inset:12px;z-index:90;display:flex;flex-direction:column}.pane.maximised .screen{flex:1;height:0;min-height:0}.pane.maximised footer{flex-shrink:0}@media(max-width:650px){.tools{padding:6px;gap:4px}.tools button{min-height:40px}.pane.maximised{inset:0;border-radius:0}}
  .pane{border:1px solid var(--line);border-radius:14px;overflow:hidden;min-width:0;background:var(--card);box-shadow:0 6px 24px rgb(0 0 0 / .035)}header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 16px;background:var(--bg);border-bottom:1px solid var(--line)}.identity,.controls{display:flex;gap:12px;align-items:center;min-width:0}.identity strong{font-size:.85rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.terminal-icon{font:700 1rem monospace;color:var(--accent)}.host{font-size:.75rem;color:var(--muted);overflow:hidden;text-overflow:ellipsis}.state{font-size:.72rem;color:var(--muted);white-space:nowrap}.state.live{color:var(--good)}button{font:inherit;font-size:.75rem;background:var(--card);border:1px solid var(--line);border-radius:7px;padding:6px 10px;min-height:32px;cursor:pointer;white-space:nowrap}button.continue{background:var(--accent);color:var(--accent-ink);border-color:var(--accent)}.screen{height:clamp(300px,58vh,650px);padding:12px 10px 8px 14px;min-width:0;overflow:hidden}.screen :global(.xterm){height:100%;padding:0}.screen :global(.xterm-viewport){overscroll-behavior:contain}.connection-message{padding:10px 16px;margin:0;font-size:.8rem;background:var(--tint);border-bottom:1px solid var(--line)}footer{padding:8px 16px;border-top:1px solid var(--line);display:flex;gap:12px;justify-content:space-between;align-items:center;font-size:.65rem;color:var(--muted)}footer button{font-size:.65rem;min-height:28px;padding:3px 8px}
  @media(max-width:650px){header{padding:10px;align-items:flex-start}.identity{gap:7px;flex-wrap:wrap}.host{width:100%;margin-left:24px}.controls{gap:8px;flex-wrap:wrap;justify-content:flex-end}.screen{padding:10px 6px;height:56vh;min-height:300px}footer{padding:8px 10px}footer span{max-width:65%}}
</style>
