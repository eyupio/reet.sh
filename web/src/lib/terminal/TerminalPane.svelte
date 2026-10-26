<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Terminal, type ITheme } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { SearchAddon } from '@xterm/addon-search'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import '@xterm/xterm/css/xterm.css'
  import { TerminalConnection, type ConnectionState } from './connection'
  import { terminalClosed, type TerminalSession } from './api'
  import { terminalShortcut } from './keyboard'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import TerminalAppearance from './TerminalAppearance.svelte'
  import CommandHistory from './CommandHistory.svelte'
  import OutputScrollback from './OutputScrollback.svelte'
  import CommandLibrary from './CommandLibrary.svelte'
  import TerminalIcon from './TerminalIcon.svelte'
  import {initialFontSize,hostSize,ResizeSender,pageViewport,browserViewport,overflowEdges,repaintOnReturn,FONT_MIN,FONT_MAX,LANDSCAPE_PHONE,PHONE_TOUCH,type Size} from './sizing'
  import {paletteFor,terminalPalettes,type CursorShape} from './themes'
  import type {TerminalCommand} from './api'

  let { session, host, user = '', onrename, onend, onarchive, archiveDisabled=false, historySupported=false, actionDisabled = false, theatre=$bindable(false),sessionsVisible=false,onsessions,keyboard=false }: { session: TerminalSession; host: string; user?: string; onrename?: () => void; onend?: () => void; onarchive?:()=>void; archiveDisabled?:boolean;historySupported?:boolean; actionDisabled?: boolean; theatre?:boolean;sessionsVisible?:boolean;onsessions?:()=>void;keyboard?:boolean } = $props()
  let element = $state<HTMLDivElement>(),screenFrame=$state<HTMLDivElement>()
  let pane=$state<HTMLElement>(),searchInput=$state<HTMLInputElement>(),viewportHeight=$state(innerHeight),viewportTop=$state(0)
  let connectionState = $state<ConnectionState>('connecting'), message = $state('')
  let connection: TerminalConnection | undefined
  let terminal: Terminal | undefined
  let search: SearchAddon | undefined, fitTerminal: (()=>void)|undefined
  let query = $state(''), searching = $state(false), found = $state(true), fontSize = $state(14), maximised = $state(false)
  let contextMenu = $state<{x:number;y:number}|null>(null), menu = $state<HTMLElement>(), hasSelection = $state(false)
  const mac = /Mac|iPhone|iPad|iPod/.test(navigator.platform)
  let feedback = $state(''), terminalBackground=$state('#101820'),terminalForeground=$state('#dce5ef')
  let pendingPaste = $state<{text:string;generation:number;execute?:boolean}|null>(null), generation=0, alive=true
  let historyOpen=$state(false),appearanceOpen=$state(false),commandsOpen=$state(false),palette=$state('system'),cursor=$state<CursorShape>('bar')
  let scrollbackOpen=$state(false)
  let landscape=$state(false)
  function toggleSessions(){if(maximised){maximised=false;if(!sessionsVisible)onsessions?.()}else onsessions?.()}
  function openScrollback(){historyOpen=false;scrollbackOpen=true}
  async function closeScrollback(){scrollbackOpen=false;await tick();terminal?.focus()}
  let applyTheme:(()=>void)|undefined
  let modifier = $state<'Ctrl'|'Alt'|null>(null)
  export function disconnect(){connection?.detach()}
  function choosePalette(id:string){palette=id;try{localStorage.setItem('reet-terminal-palette',id)}catch{/* optional preference */}applyTheme?.()}
  function chooseCursor(shape:CursorShape){cursor=shape;if(terminal)terminal.options.cursorStyle=shape;try{localStorage.setItem('reet-terminal-cursor',shape)}catch{/* optional preference */}}
  async function insertSaved(item:TerminalCommand){
    const intended=generation;scrollbackOpen=false;commandsOpen=false;await tick()
    if(!alive||intended!==generation||!live){feedback='Connection changed. Open Commands again after reconnecting; nothing was inserted.';return}
    paste(item.command,/\t/.test(item.command));restoreFocus()
  }
  async function insertHistory(text:string,run:boolean){
    const intended=generation;scrollbackOpen=false;if(!run){historyOpen=false;await tick()}
    if(!alive||intended!==generation||!live){feedback='Connection changed. Open History again after reconnecting; nothing was sent.';return}
    if(run)pendingPaste={text,generation,execute:true};else paste(text,/\t/.test(text));restoreFocus()
  }
  function find(next=true) { found=!query||!!(next?search?.findNext(query,{caseSensitive:false,regex:false}):search?.findPrevious(query,{caseSensitive:false,regex:false})) }
  function changeFont(delta:number) { fontSize=Math.min(FONT_MAX,Math.max(FONT_MIN,fontSize+delta));if(terminal)terminal.options.fontSize=fontSize;try{localStorage.setItem('reet-terminal-font',String(fontSize))}catch{/* preferences are optional */}fitTerminal?.() }
  function restoreFocus() { if(alive&&!scrollbackOpen&&!pendingPaste&&!document.querySelector('dialog[open]'))terminal?.focus() }
  async function copy() { const text=scrollbackOpen?window.getSelection()?.toString():terminal?.getSelection();if(!text){feedback='Select terminal text first.';return}try{await navigator.clipboard.writeText(text);feedback='Selection copied.'}catch{feedback='Clipboard access was denied. Use your browser’s copy command.'}finally{restoreFocus()} }
  function paste(text:string,review=false) {
    feedback=''
    if(scrollbackOpen){feedback='Return to live output before pasting.';return}
    if(!live){feedback='Reconnect before pasting. Nothing was sent.';return}
    if(new TextEncoder().encode(text).length>240*1024){feedback='Paste is too large. Limit it to 240 KiB.';return}
    if(review||/[\r\n\x00-\x08\x0b-\x1f\x7f]/.test(text)||text.length>2000)pendingPaste={text,generation}
    else {modifier=null;terminal?.paste(text)}
  }
  async function confirmPaste() {
    const pending=pendingPaste;pendingPaste=null;if(!pending)return
    if(pending.execute){historyOpen=false;await tick()}
    if(!alive||!live||pending.generation!==generation){feedback=pending.execute?'Connection changed. Choose the command again after reconnecting; nothing was sent.':'Connection changed. Paste again after reconnecting; nothing was sent.';return}
    modifier=null;terminal?.paste(pending.text)
    if(pending.execute)feedback=connection?.text('\r')?'Command sent to the active terminal.':'Connection changed while sending. Check the terminal before retrying.'
    terminal?.focus()
  }
  async function readClipboard() {feedback='';const started=generation;try{const text=await navigator.clipboard.readText();if(!alive)return;if(started!==generation){feedback='Connection changed. Paste again; nothing was sent.';return}paste(text)}catch{if(alive)feedback='Clipboard access was denied. Paste with your keyboard inside the terminal.'}finally{restoreFocus()} }
  function key(value:string) {modifier=null;if(live&&!scrollbackOpen){connection?.text(value);terminal?.focus()} }
  $effect(()=>{const query=matchMedia(LANDSCAPE_PHONE),update=()=>landscape=query.matches;update();query.addEventListener('change',update);return()=>query.removeEventListener('change',update)})
  $effect(()=>{if(searching)searchInput?.focus()})
  $effect(()=>{if(contextMenu)menu?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()})
  function openContext(event: MouseEvent) { if(event.shiftKey){contextMenu=null;return}event.preventDefault();contextMenu={x:Math.max(8,Math.min(event.clientX,innerWidth-190)),y:Math.max(8,Math.min(event.clientY,(window.visualViewport?.height||innerHeight)-330))} }
  function closeContext() { contextMenu=null;terminal?.focus() }
  function contextKey(event: KeyboardEvent) {
    if(['Escape','Tab'].includes(event.key)){event.preventDefault();event.stopPropagation();closeContext();return}
    if(!['ArrowDown','ArrowUp','Home','End'].includes(event.key))return
    event.preventDefault();const buttons=Array.from(menu?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')||[]);if(!buttons.length)return
    const at=buttons.indexOf(document.activeElement as HTMLButtonElement),next=event.key==='Home'?0:event.key==='End'?buttons.length-1:(at+(event.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length;buttons[next]?.focus()
  }
  // Marks the ends of a sideways-scrolling row that hide controls, for the fade hint.
  function edges(row:HTMLElement){
    const update=()=>{const {start,end}=overflowEdges(row.scrollLeft,row.scrollWidth,row.clientWidth);row.classList.toggle('more-start',start);row.classList.toggle('more-end',end)}
    const sizes=new ResizeObserver(update);sizes.observe(row);for(const child of row.children)sizes.observe(child)
    const content=new MutationObserver(update);content.observe(row,{childList:true,subtree:true,characterData:true})
    row.addEventListener('scroll',update,{passive:true});update()
    return {destroy(){sizes.disconnect();content.disconnect();row.removeEventListener('scroll',update)}}
  }
  $effect(()=>{if(!maximised)return;const previous=document.body.style.overflow,focus=document.activeElement;document.body.style.overflow='hidden';return()=>{document.body.style.overflow=previous;if(focus instanceof HTMLElement&&focus.isConnected)focus.focus({preventScroll:true})}})
  onMount(()=>{
    const viewport=()=>{const usable=pageViewport.read(browserViewport());viewportHeight=usable.height;viewportTop=usable.top;fitTerminal?.()}
    const keyboard=(event:KeyboardEvent)=>{
      if(document.querySelector('dialog[open]'))return
      if(event.altKey&&event.shiftKey&&event.key.toLowerCase()==='m'){event.preventDefault();maximised=!maximised;fitTerminal?.();return}
      if(!maximised||event.key!=='Tab'||event.ctrlKey||event.altKey||(event.target instanceof Element&&event.target.closest('.xterm')))return
      const buttons=Array.from(pane?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),[tabindex="0"]')||[]).filter(item=>item.getClientRects().length)
      const first=buttons[0],last=buttons.at(-1)
      if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
    }
    viewport();window.visualViewport?.addEventListener('resize',viewport);window.visualViewport?.addEventListener('scroll',viewport);window.addEventListener('resize',viewport);window.addEventListener('keydown',keyboard,true)
    return()=>{window.visualViewport?.removeEventListener('resize',viewport);window.visualViewport?.removeEventListener('scroll',viewport);window.removeEventListener('resize',viewport);window.removeEventListener('keydown',keyboard,true)}
  })
  const live = $derived(connectionState === 'live')
  const stateLabel = $derived(({ connecting:'Connecting',resynchronising:'Restoring screen',live:'Live',reconnecting:'Reconnecting',detached:'Disconnected',taken_over:'Open elsewhere',ended:'Ended',lost:'Shell unavailable',access_denied:'Access denied' })[connectionState])

  onMount(() => {
    if (!element) return
    const fit = new FitAddon()
    // Phones (portrait or landscape) start at 11 px so agent layouts get about 54 columns at 390 px; a stored size always wins.
    const phone=matchMedia(PHONE_TOUCH).matches
    try{fontSize=initialFontSize(localStorage.getItem('reet-terminal-font'),phone)}catch{fontSize=initialFontSize(null,phone)/* optional preference */}
    try{const saved=localStorage.getItem('reet-terminal-palette');if(saved==='system'||terminalPalettes.some(item=>item.id===saved))palette=saved!;const shape=localStorage.getItem('reet-terminal-cursor');if(shape==='bar'||shape==='block'||shape==='underline')cursor=shape}catch{/* optional preferences */}
    const term = new Terminal({ cursorStyle:cursor,cursorBlink:true,fontSize:14,fontFamily:'"DejaVu Sans Mono", "SFMono-Regular", Consolas, monospace',scrollback:5000,screenReaderMode:true,allowProposedApi:false,windowOptions:{},minimumContrastRatio:4.5 })
    terminal = term;term.options.fontSize=fontSize;term.loadAddon(fit);search=new SearchAddon();term.loadAddon(search);term.open(element)
    const onPaste=(event:ClipboardEvent)=>{event.preventDefault();event.stopImmediatePropagation();paste(event.clipboardData?.getData('text/plain')||'')}
    element.addEventListener('paste',onPaste,true)
    term.attachCustomKeyEventHandler(event=>{
      const shortcut=terminalShortcut(event,term.hasSelection(),mac)
      if(event.type==='keydown'&&event.shiftKey&&event.key==='PageUp'){event.preventDefault();openScrollback();return false}
      if(shortcut==='native-paste')return false
      if(shortcut==='copy'){event.preventDefault();void copy();return false}
      if(shortcut==='search'){event.preventDefault();searching=!searching;return false}
      return true
    })
    const selection=term.onSelectionChange(()=>hasSelection=term.hasSelection())
    // No terminal-controlled clipboard writes or active OSC hyperlinks.
    const clipboard = term.parser.registerOscHandler(52, () => true)
    const links = term.parser.registerOscHandler(8, () => true)
    // Queued observers, redraws and connection notifications can outlive this
    // pane during session removal. A disposed xterm has no render dimensions.
    function theme() {
      if (!alive) return
      const choice = document.documentElement.dataset.theme
      const dark = choice === 'dark' || !choice && matchMedia('(prefers-color-scheme: dark)').matches
      const colors:ITheme=paletteFor(palette,dark)
      term.options.theme = colors
      terminalBackground=colors.background||'#101820'
      terminalForeground=colors.foreground||'#dce5ef'
      if(element)element.style.backgroundColor=colors.background||''
      if(screenFrame)screenFrame.style.backgroundColor=colors.background||''
    }
    applyTheme=theme;theme()
    const observer = new MutationObserver(theme); observer.observe(document.documentElement,{attributes:true,attributeFilter:['data-theme']})
    const media = matchMedia('(prefers-color-scheme: dark)'); media.addEventListener('change',theme)
    // The first host redraw uses the ticket's grid. Transient status/layout
    // changes must not resize xterm before that redraw has been parsed.
    let frame = 0, clipFrame = 0, restoringGrid=false
    // The host is told a debounced size, never one it already has. Below 20×8 the
    // grid stays at the floor and its bottom rows (prompt, agent input) stay in view.
    const sender=new ResizeSender(size=>connection?.resize(size.cols,size.rows)??false)
    function grid():Size{const size=hostSize(term.cols,term.rows);if(size.cols!==term.cols||size.rows!==term.rows)term.resize(size.cols,size.rows);return size}
    function clip(){cancelAnimationFrame(clipFrame);clipFrame=requestAnimationFrame(()=>{const shell=element?.querySelector<HTMLElement>('.xterm'),rendered=element?.querySelector('.xterm-screen');if(!alive||!element||!shell||!rendered)return;const over=Math.max(0,Math.ceil(rendered.getBoundingClientRect().height-element.clientHeight));shell.style.transform=over?`translateY(-${over}px)`:''})}
    // Above the floor this is exactly fit.fit(); at the floor the grid is resized once, to the floor.
    function fitted(){const proposed=fit.proposeDimensions();if(!proposed||isNaN(proposed.cols)||isNaN(proposed.rows))return;const size=hostSize(proposed.cols,proposed.rows);if(size.cols===proposed.cols&&size.rows===proposed.rows)fit.fit();else if(size.cols!==term.cols||size.rows!==term.rows)term.resize(size.cols,size.rows)}
    function resize() { if(!alive)return;cancelAnimationFrame(frame); frame=requestAnimationFrame(()=>{if (!alive || restoringGrid || !element?.clientWidth || !element.clientHeight) return;fitted();const size=grid();clip();sender.request(size);term.refresh(0,term.rows-1)}) }
    // One full repaint: a one-row size change makes tmux pass a real SIGWINCH to the app.
    // tmux repaints its own grid on every attach; a reattach or a return to the page also asks the app.
    // A return repaints only on a touch screen or after a long absence, never twice within 5 s (repaintOnReturn).
    let attached=false,hiddenAt:number|null=null,repaintedAt=-Infinity
    const coarse=matchMedia('(pointer:coarse)')
    function redraw(){if(alive&&connectionState==='live'){repaintedAt=performance.now();sender.nudge()}}
    const sizes = new ResizeObserver(resize); sizes.observe(element);fit.fit()
    fitTerminal=resize
    // Refresh after parsing and fitting so accessibility rows match full redraws.
    connection = new TerminalConnection({session:session.id,dimensions:()=>{if(!alive)return {cols:80,rows:24};const size=grid();clip();sender.known(size);restoringGrid=true;return size},renderer:{write:(bytes,reset,done)=>{if(!alive)return;if(reset)term.reset();term.write(bytes,()=>{if(!alive)return;term.refresh(0,term.rows-1);done()})}},onstate:(next,detail)=>{if(!alive)return;if(next!=='resynchronising'){restoringGrid=false;if(next==='live')resize()}generation++;modifier=null;if(next==='access_denied'){historyOpen=false;scrollbackOpen=false};connectionState=next;if(next==='live'){if(attached)redraw();attached=true}message=detail;term.options.disableStdin=next!=='live';if(next==='live'&&!scrollbackOpen&&!searching&&!pendingPaste&&!contextMenu&&!document.querySelector('dialog[open]'))term.focus()}})
    term.options.disableStdin=true
    const input=term.onData((text)=>{if(scrollbackOpen)return;let value=text;if(modifier==='Ctrl'&&/^[a-zA-Z@\[\]\\^_]$/.test(text))value=String.fromCharCode(text.toUpperCase().charCodeAt(0)&31);else if(modifier==='Alt')value='\x1b'+text;modifier=null;connection?.text(value)})
    const binary=term.onBinary((data)=>!scrollbackOpen&&connection?.send(Uint8Array.from(data,c=>c.charCodeAt(0)&255)))
    const online=()=>connection?.recover(),visible=()=>{if(document.visibilityState!=='visible'){hiddenAt=performance.now();return}const now=performance.now(),hiddenFor=hiddenAt===null?0:now-hiddenAt;hiddenAt=null;resize();if(repaintOnReturn({hiddenFor,coarse:coarse.matches,sinceRepaint:now-repaintedAt}))redraw();connection?.recover()}
    window.addEventListener('online',online);document.addEventListener('visibilitychange',visible)
    void connection.connect()
    return ()=>{alive=false;element?.removeEventListener('paste',onPaste,true);search?.dispose();search=undefined;fitTerminal=undefined;sender.dispose();connection?.dispose();connection=undefined;cancelAnimationFrame(frame);cancelAnimationFrame(clipFrame);sizes.disconnect();observer.disconnect();media.removeEventListener('change',theme);window.removeEventListener('online',online);document.removeEventListener('visibilitychange',visible);selection.dispose();input.dispose();binary.dispose();clipboard.dispose();links.dispose();applyTheme=undefined;term.dispose();terminal=undefined}
  })
</script>

<svelte:window onclick={event=>{if(contextMenu&&event.target instanceof Node&&!menu?.contains(event.target))contextMenu=null}} onblur={()=>contextMenu=null} />
<section class="pane" class:maximised class:keyboard bind:this={pane} role={maximised?'dialog':'region'} aria-modal={maximised?true:undefined} tabindex="-1" aria-label={`${session.title} · Terminal on ${host}`} style={`--terminal-viewport:${viewportHeight}px;--terminal-top:${viewportTop}px;--terminal-font-size:${fontSize}px;--terminal-bg:${terminalBackground};--terminal-fg:${terminalForeground}`} data-connection-state={connectionState}>
  <header><div class="identity"><span class="terminal-icon" aria-hidden="true">›_</span><strong>{session.title}</strong><span class="host" title={`${user ? user+'@' : ''}${host}`}>{user ? `${user}@` : ''}{host}</span></div><div class="controls"><span class="state" class:live><span aria-hidden="true">●</span> {stateLabel}</span>{#if live}<button type="button" onclick={()=>connection?.detach()}>Disconnect</button>{:else if connectionState==='taken_over'}<button type="button" class="continue" onclick={()=>void connection?.connect(true)}>Continue here</button>{:else if connectionState==='detached'}<button type="button" onclick={()=>void connection?.connect()}>Reconnect</button>{:else if connectionState==='reconnecting'}<button type="button" onclick={()=>connection?.recover()}>Retry now</button>{/if}{#if onrename||onend||onarchive&&terminalClosed(session.state)}<details class="session-menu"><summary>Manage</summary><div>{#if onrename}<button disabled={actionDisabled} onclick={event=>{event.currentTarget.closest('details')?.removeAttribute('open');onrename?.()}}>Rename</button>{/if}{#if onend}<button class="end" disabled={actionDisabled||!['creating','running','end_requested'].includes(session.state)} onclick={event=>{event.currentTarget.closest('details')?.removeAttribute('open');onend?.()}}>{session.state==='end_requested'?'Retry end request':'End session'}</button>{/if}{#if onarchive&&terminalClosed(session.state)}<button disabled={archiveDisabled} onclick={event=>{event.currentTarget.closest('details')?.removeAttribute('open');onarchive?.()}}>Remove from history</button>{/if}</div></details>{/if}</div></header>
  {#if message}<p class="connection-message" role="status">{message}</p>{/if}
  {#if maximised}<p class="connection-message maximise-hint">Alt+Shift+M restores the workspace. Tab inside the terminal remains a shell key.</p>{/if}
  {#snippet toolbar()}
    <div class="tools terminal-toolbar" aria-label="Terminal tools" use:edges>
      {#if onsessions&&(!sessionsVisible||maximised)}<button class="show-sessions" onclick={toggleSessions} aria-label="Show sessions" title="Show session sidebar"><span aria-hidden="true">›</span><span>Show sessions</span></button>{/if}
      <button onclick={()=>{if(scrollbackOpen)pane?.querySelector<HTMLInputElement>('[aria-label="Find in earlier output"]')?.focus();else searching=!searching}} aria-expanded={searching} aria-label="Search" title="Search terminal output"><TerminalIcon name="search"/><span class="tool-label">Search</span></button>
      <button onclick={()=>void copy()} aria-label="Copy selection" title="Copy selected output"><TerminalIcon name="copy"/><span class="tool-label">Copy</span></button>
      <button onclick={()=>void readClipboard()} disabled={!live||scrollbackOpen} aria-label="Paste" title="Paste clipboard"><TerminalIcon name="paste"/><span class="tool-label">Paste</span></button>
      <span class="tool-divider" aria-hidden="true"></span>
      <button onclick={()=>key("\x03")} disabled={!live||scrollbackOpen} aria-label="Interrupt shell" title="Interrupt · Ctrl+C"><TerminalIcon name="interrupt"/><span class="tool-label">Interrupt</span></button>
      <button onclick={()=>commandsOpen=true} aria-label="Saved commands" title="Your saved commands"><TerminalIcon name="commands"/><span>Commands</span></button>
      <button onclick={openScrollback} aria-label="Earlier output" aria-pressed={scrollbackOpen} title="Scroll earlier terminal output · Shift+PageUp"><TerminalIcon name="history"/><span>Scrollback</span></button>
      <span class="tool-space"></span>
      <button onclick={()=>appearanceOpen=true} aria-label="Appearance" title="Theme, text size and cursor"><TerminalIcon name="appearance"/><span class="tool-label">Appearance</span></button>
      <button onclick={()=>historyOpen=true} aria-label="Recent history" title="Find saved shell commands or read terminal output"><TerminalIcon name="history"/><span class="tool-label">History</span></button>
      <button onclick={()=>theatre=!theatre} aria-pressed={theatre} aria-label={theatre?'Exit theatre view':'Theatre view'} title={theatre?'Restore session panel':'Wide terminal · theatre view'}><TerminalIcon name="theatre"/></button>
      <button onclick={()=>{maximised=!maximised;fitTerminal?.()}} aria-pressed={maximised} aria-label={maximised?'Restore size':'Maximise'} title={maximised?'Restore size':'Maximise terminal'}><TerminalIcon name="expand"/></button>
    </div>
  {/snippet}
  {#snippet keys()}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (a row that scrolls sideways must be reachable by keyboard, even while its keys are disabled) -->
    <div class="tools mobile-keys" role="group" aria-label="Terminal extra keys" tabindex="0" use:edges>{#each [['Esc','\x1b'],['Tab','\t'],['↑','\x1b[A'],['↓','\x1b[B'],['←','\x1b[D'],['→','\x1b[C'],['Ctrl+C','\x03'],['Enter','\r']] as [label,value]}<button disabled={!live||scrollbackOpen} onpointerdown={e=>e.preventDefault()} onclick={()=>key(value)} aria-label={`Send ${label}`}>{label}</button>{/each}{#each ['Ctrl','Alt'] as mod}<button disabled={!live||scrollbackOpen} aria-pressed={modifier===mod} onpointerdown={e=>e.preventDefault()} onclick={()=>{modifier=modifier===mod?null:mod as 'Ctrl'|'Alt';terminal?.focus()}}>{mod}</button>{/each}</div>
  {/snippet}
  <!-- A phone held sideways puts the tools and the extra keys side by side, so the screen keeps the rows. -->
  {#if landscape}<div class="tool-rows">{@render toolbar()}{@render keys()}</div>{:else}{@render toolbar()}{/if}
  {#if searching}<form class="tools" onsubmit={e=>{e.preventDefault();find()}}><label>Find in terminal <input bind:this={searchInput} bind:value={query} maxlength="200" autocomplete="off" spellcheck="false" oninput={e=>{query=e.currentTarget.value;find()}} /></label><button>Next match</button><button type="button" onclick={()=>find(false)}>Previous match</button><button type="button" onclick={()=>{searching=false;search?.clearDecorations();terminal?.focus()}}>Close search</button><span role="status">{found?'':'No matches'}</span></form>{/if}
  {#if feedback}<p class="connection-message" role="status">{feedback}</p>{/if}
  <div class="screen-frame" bind:this={screenFrame}><div class="screen" class:reading={scrollbackOpen} inert={scrollbackOpen} bind:this={element} role="region" aria-label="Remote terminal screen" oncontextmenu={openContext}></div>{#if scrollbackOpen}<OutputScrollback session={session.id} onclose={()=>void closeScrollback()}/>{/if}</div>
  {#if !landscape}{@render keys()}{/if}
  <footer><span>{scrollbackOpen?'Reading earlier output · your live shell keeps running':live?'Connected privately · input goes directly to this shell':'Input paused · disconnected keystrokes are never replayed'}</span><button type="button" disabled={!live} onclick={()=>void closeScrollback()}>Focus terminal</button></footer>
{#if contextMenu}<div class="clipboard-menu" role="menu" aria-label="Terminal actions" tabindex="-1" bind:this={menu} style={`left:${contextMenu.x}px;top:${contextMenu.y}px`} onkeydown={contextKey}><button role="menuitem" disabled={!hasSelection} onclick={()=>{contextMenu=null;void copy()}}>Copy selection</button><button role="menuitem" disabled={!live} onclick={()=>{contextMenu=null;void readClipboard()}}>Paste</button><button role="menuitem" onclick={()=>{contextMenu=null;commandsOpen=true}}>Saved commands…</button><button role="menuitem" onclick={()=>{contextMenu=null;historyOpen=true}}>Command history…</button><button role="menuitem" onclick={()=>{contextMenu=null;openScrollback()}}>Earlier output…</button><button role="menuitem" onclick={()=>{contextMenu=null;appearanceOpen=true}}>Appearance…</button>{#if onsessions}<button role="menuitem" onclick={()=>{contextMenu=null;toggleSessions()}}>{sessionsVisible&&!maximised?'Hide sessions':'Show sessions'}</button>{/if}<button role="menuitem" onclick={closeContext}>Close menu</button></div>{/if}
</section>

{#if appearanceOpen}<WorkspaceDialog title="Terminal appearance" placement="drawer" onclose={()=>appearanceOpen=false}><TerminalAppearance {palette} {fontSize} {cursor} onpalette={choosePalette} onfont={changeFont} oncursor={chooseCursor}/></WorkspaceDialog>{/if}
{#if commandsOpen}<CommandLibrary onclose={()=>commandsOpen=false} oninsert={item=>void insertSaved(item)} canInsert={live} context={`${user?user+'@':''}${host} · ${session.title}`}/>{/if}
{#if historyOpen}<CommandHistory session={session.id} context={`${user?user+'@':''}${host} · ${session.title}`} supported={historySupported} canInsert={live} onclose={()=>historyOpen=false} oninsert={(text,run)=>void insertHistory(text,run)} onoutput={openScrollback}/>{/if}
{#if pendingPaste}<ConfirmDialog title={pendingPaste.execute?'Run this command in the active terminal?':'Paste into the remote shell?'} body={pendingPaste.execute?`Send this command to ${host} · ${session.title} and press Enter. Be at a shell prompt; an open program receives terminal input instead.`:'This text contains multiple lines, control characters, or a large block. It may execute commands immediately.'} details={pendingPaste.execute?pendingPaste.text:pendingPaste.text.slice(0,2000)+(pendingPaste.text.length>2000?'\n… preview truncated':'')} confirmLabel={pendingPaste.execute?'Run command':'Paste text'} onconfirm={confirmPaste} oncancel={()=>{const running=pendingPaste?.execute;pendingPaste=null;if(!running)terminal?.focus()}} />{/if}

<style>
  .pane {display:flex;flex-direction:column;position:relative;min-height:0;min-width:0;overflow:hidden;border:1px solid var(--line);border-radius:12px;background:var(--card)}
  .pane>header,.pane>.tools,.pane>footer,.pane>.connection-message {flex-shrink:0}
  header {display:flex;align-items:center;justify-content:space-between;gap:8px;padding:7px 12px;background:var(--bg);border-bottom:1px solid var(--line)}
  .identity,.controls {display:flex;align-items:center;gap:8px;min-width:0}
  .identity {flex:1;flex-wrap:wrap;column-gap:6px;row-gap:1px}
  .identity strong {font-size:.85rem;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .terminal-icon {font:700 1rem monospace;color:var(--accent-text)}
  .host {font-size:.75rem;color:var(--muted);max-width:160px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .controls {flex-shrink:0}
  .state {font-size:.72rem;color:var(--muted);white-space:nowrap}
  .state.live {color:var(--good)}
  button {font:inherit;font-size:.75rem;background:var(--card);color:var(--ink);border:1px solid var(--line);border-radius:7px;padding:6px 10px;min-height:32px;cursor:pointer;white-space:nowrap}
  button:disabled {opacity:.45;cursor:default}
  button.continue {background:var(--accent);color:var(--accent-ink);border-color:var(--accent)}
  .tools {display:flex;gap:6px;align-items:center;flex-wrap:nowrap;overflow-x:auto;min-width:0;scrollbar-width:thin;padding:6px 10px;border-bottom:1px solid var(--line);font-size:.72rem}
  .tools>button,.tools label {flex-shrink:0}
  .terminal-toolbar{background:var(--bg);padding:5px 8px;gap:2px}.terminal-toolbar button{display:flex;align-items:center;gap:6px;border-color:transparent;background:transparent;font-size:.72rem;padding:6px 8px;min-height:32px}.terminal-toolbar button:hover:not(:disabled){background:var(--card);border-color:var(--line)}.tool-space{flex:1}.tool-divider{height:16px;width:1px;background:var(--line);margin:0 5px;flex-shrink:0}
  .tools label {display:flex;gap:8px;align-items:center;min-width:0}
  .tools input {min-width:0;max-width:180px;font-size:.8rem}
  .tools button:hover:not(:disabled) {border-color:var(--accent);background:var(--tint)}
  .tools button[aria-pressed=true] {border-color:var(--accent);color:var(--accent-text)}
  .tools.mobile-keys {display:none;border-top:1px solid var(--line);border-bottom:0}
  .screen-frame {position:relative;flex:1;height:0;min-height:0;min-width:0;overflow:hidden;padding:12px 10px 8px 14px}
  .screen {height:100%;min-height:0;min-width:0;overflow:hidden;padding:0}
  .screen.reading {visibility:hidden}
  .screen :global(.xterm) {height:100%;padding:0}
  .screen :global(.xterm-viewport) {overscroll-behavior:contain}
  .connection-message {max-height:72px;overflow:auto;padding:6px 12px;margin:0;font-size:.73rem;background:var(--tint);border-bottom:1px solid var(--line)}
  footer {padding:5px 10px;border-top:1px solid var(--line);display:flex;gap:12px;justify-content:space-between;align-items:center;font-size:.65rem;color:var(--muted)}
  footer button {font-size:.65rem;min-height:28px;padding:3px 8px}
  .session-menu {position:relative}
  .session-menu>summary {cursor:pointer;font-size:.75rem;min-height:32px;padding:6px 9px;border:1px solid var(--line);border-radius:7px;list-style:none}
  .session-menu>summary::-webkit-details-marker {display:none}
  .session-menu>div {position:absolute;right:0;top:calc(100% + 4px);min-width:150px;display:grid;gap:5px;padding:6px;border:1px solid var(--line);border-radius:8px;background:var(--card);box-shadow:var(--shadow);z-index:10}
  .session-menu button {text-align:left}
  .end {color:var(--bad)}
  .clipboard-menu {position:fixed;z-index:110;min-width:180px;max-height:calc(100dvh - 16px);overflow:auto;padding:6px;display:grid;gap:4px;border:1px solid var(--line);border-radius:9px;background:var(--card);box-shadow:0 8px 30px rgb(0 0 0 / .2)}
  .clipboard-menu button {text-align:left}
  .pane.maximised {position:fixed;inset:12px;bottom:auto;height:calc(var(--terminal-viewport,100dvh) - 24px);z-index:90}
  /* Phones (narrow, or a touch screen held sideways; see PHONE_SCREEN in sizing.ts): every row of chrome is one line, so the screen keeps the height. */
  @media(max-width:650px),(max-height:500px) and (pointer:coarse) and (orientation:landscape) {
    header {padding:2px 4px 2px 8px}
    .identity {flex-wrap:nowrap}
    .identity strong {font-size:.74rem;flex:0 1 auto}
    .terminal-icon {display:none}
    .host {flex:0 1 auto;min-width:0;max-width:none;font-size:.62rem}
    .controls {gap:4px}
    .state {font-size:.65rem}
    .controls>button,.session-menu>summary {font-size:.67rem;padding:5px 7px;min-height:30px}
    .tools {padding:2px 4px;gap:4px}
    .tools button {min-height:32px;font-size:.69rem;padding:5px 8px}
    .terminal-toolbar .tool-label{display:none}.terminal-toolbar{padding:2px 4px}.terminal-toolbar button{padding:6px;min-width:30px}.terminal-toolbar .tool-divider{margin:0 2px}.terminal-toolbar .tool-space{display:none}
    .tools.mobile-keys {display:flex}
    .mobile-keys button {min-width:34px;min-height:32px}
    /* Rows that scroll sideways fade at the end that hides more controls. */
    .tools:global(.more-end) {-webkit-mask-image:linear-gradient(to right,#000 calc(100% - 36px),transparent);mask-image:linear-gradient(to right,#000 calc(100% - 36px),transparent)}
    .tools:global(.more-start) {-webkit-mask-image:linear-gradient(to left,#000 calc(100% - 36px),transparent);mask-image:linear-gradient(to left,#000 calc(100% - 36px),transparent)}
    .tools:global(.more-start.more-end) {-webkit-mask-image:linear-gradient(to right,transparent,#000 36px,#000 calc(100% - 36px),transparent);mask-image:linear-gradient(to right,transparent,#000 36px,#000 calc(100% - 36px),transparent)}
    .mobile-keys:focus-visible {outline:2px solid var(--accent);outline-offset:-2px}
    .screen-frame {padding:4px}
    footer {font-size:.6rem;padding:2px 8px}
    footer span {max-width:75%}
    footer button {font-size:.6rem;min-height:24px}
    /* While the on-screen keyboard is open the terminal already has focus. */
    .pane.keyboard>footer {display:none}
    .connection-message {font-size:.68rem;max-height:55px}
    .pane.maximised {inset:0;top:var(--terminal-top,0px);height:var(--terminal-viewport,100dvh);border-radius:0}
    .pane.maximised footer {padding-bottom:max(2px,env(safe-area-inset-bottom))}
  }
  @media(pointer:coarse) {.tools button,.controls>button,.session-menu>summary {min-height:40px}}
  /* Touch targets keep a 40 × 40 px hit area; on phones the rows lose their padding instead. */
  @media(max-width:650px) and (pointer:coarse),(max-height:500px) and (pointer:coarse) and (orientation:landscape) {header,.tools,.terminal-toolbar {padding-top:0;padding-bottom:0}.terminal-toolbar button,.mobile-keys button {min-width:40px}}
  /* Sideways: tools and extra keys share one row, and the footer gives its line to the screen (the state stays in the header). */
  @media(max-height:500px) and (pointer:coarse) and (orientation:landscape) {
    .tool-rows {display:flex;flex-shrink:0;min-width:0;border-bottom:1px solid var(--line)}
    .tool-rows>.tools {flex:1 1 50%;min-width:0;border-bottom:0}
    .tool-rows>.mobile-keys {border-top:0;border-left:1px solid var(--line)}
    .pane>footer {display:none}
    /* With the keyboard open there are too few rows for the header; it returns when the keyboard closes. */
    .pane.keyboard>header {display:none}
  }
  /* The Alt+Shift+M advice is for a hardware keyboard; the Restore size button stays. */
  @media(hover:none) and (pointer:coarse) {.maximise-hint {display:none}}
</style>
