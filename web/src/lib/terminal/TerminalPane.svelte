<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal, type ITheme } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { TerminalConnection, type ConnectionState } from './connection'
  import type { TerminalSession } from './api'

  let { session, host }: { session: TerminalSession; host: string } = $props()
  let element = $state<HTMLDivElement>()
  let connectionState = $state<ConnectionState>('connecting'), message = $state('')
  let connection: TerminalConnection | undefined
  let terminal: Terminal | undefined
  const live = $derived(connectionState === 'live')
  const stateLabel = $derived(({ connecting:'Connecting',resynchronising:'Restoring screen',live:'Live',reconnecting:'Reconnecting',detached:'Detached',taken_over:'Open elsewhere',ended:'Ended',lost:'Shell unavailable',access_denied:'Access denied' })[connectionState])

  onMount(() => {
    if (!element) return
    const fit = new FitAddon()
    const term = new Terminal({ cursorBlink:true,fontSize:14,fontFamily:'"DejaVu Sans Mono", "SFMono-Regular", Consolas, monospace',scrollback:5000,screenReaderMode:true,allowProposedApi:false,windowOptions:{},minimumContrastRatio:4.5 })
    terminal = term; term.loadAddon(fit); term.open(element)
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
    connection = new TerminalConnection({session:session.id,dimensions:()=>({cols:Math.min(500,Math.max(2,term.cols)),rows:Math.min(300,Math.max(1,term.rows))}),renderer:{write:(bytes,reset,done)=>{if(reset)term.reset();term.write(bytes,done)}},onstate:(next,detail)=>{connectionState=next;message=detail;term.options.disableStdin=next!=='live';if(next==='live')term.focus()}})
    term.options.disableStdin=true
    const input=term.onData((text)=>connection?.text(text))
    const binary=term.onBinary((data)=>connection?.send(Uint8Array.from(data,c=>c.charCodeAt(0)&255)))
    const online=()=>connection?.recover(),visible=()=>{if(document.visibilityState==='visible'){resize();connection?.recover()}}
    window.addEventListener('online',online);document.addEventListener('visibilitychange',visible)
    void connection.connect()
    return ()=>{connection?.dispose();connection=undefined;cancelAnimationFrame(frame);sizes.disconnect();observer.disconnect();media.removeEventListener('change',theme);window.removeEventListener('online',online);document.removeEventListener('visibilitychange',visible);input.dispose();binary.dispose();clipboard.dispose();links.dispose();term.dispose();terminal=undefined}
  })
</script>

<section class="pane" aria-label={`Terminal on ${host}`} data-connection-state={connectionState}>
  <header><div class="identity"><span class="terminal-icon" aria-hidden="true">›_</span><strong>{session.title}</strong><span class="host">{host}</span></div><div class="controls"><span class="state" class:live><span aria-hidden="true">●</span> {stateLabel}</span>{#if live}<button type="button" onclick={()=>connection?.detach()}>Detach</button>{:else if connectionState==='taken_over'}<button type="button" class="continue" onclick={()=>void connection?.connect(true)}>Continue here</button>{:else if connectionState==='detached'}<button type="button" onclick={()=>void connection?.connect()}>Reconnect</button>{:else if connectionState==='reconnecting'}<button type="button" onclick={()=>connection?.recover()}>Retry now</button>{/if}</div></header>
  {#if message}<p class="connection-message" role="status">{message}</p>{/if}
  <div class="screen" bind:this={element} aria-label="Remote terminal screen"></div>
  <footer><span>{live?'Connected privately · input goes directly to this shell':'Input paused · disconnected keystrokes are never replayed'}</span><button type="button" disabled={!live} onclick={()=>terminal?.focus()}>Focus terminal</button></footer>
</section>

<style>
  .pane{border:1px solid var(--line);border-radius:14px;overflow:hidden;min-width:0;background:var(--card);box-shadow:0 6px 24px rgb(0 0 0 / .035)}header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px 16px;background:var(--bg);border-bottom:1px solid var(--line)}.identity,.controls{display:flex;gap:12px;align-items:center;min-width:0}.identity strong{font-size:.85rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.terminal-icon{font:700 1rem monospace;color:var(--accent)}.host{font-size:.75rem;color:var(--muted);overflow:hidden;text-overflow:ellipsis}.state{font-size:.72rem;color:var(--muted);white-space:nowrap}.state.live{color:var(--good)}button{font:inherit;font-size:.75rem;background:var(--card);border:1px solid var(--line);border-radius:7px;padding:6px 10px;min-height:32px;cursor:pointer;white-space:nowrap}button.continue{background:var(--accent);color:var(--accent-ink);border-color:var(--accent)}.screen{height:clamp(300px,58vh,650px);padding:12px 10px 8px 14px;min-width:0;overflow:hidden}.screen :global(.xterm){height:100%;padding:0}.screen :global(.xterm-viewport){overscroll-behavior:contain}.connection-message{padding:10px 16px;margin:0;font-size:.8rem;background:var(--tint);border-bottom:1px solid var(--line)}footer{padding:8px 16px;border-top:1px solid var(--line);display:flex;gap:12px;justify-content:space-between;align-items:center;font-size:.65rem;color:var(--muted)}footer button{font-size:.65rem;min-height:28px;padding:3px 8px}
  @media(max-width:650px){header{padding:10px;align-items:flex-start}.identity{gap:7px;flex-wrap:wrap}.host{width:100%;margin-left:24px}.controls{gap:8px;flex-wrap:wrap;justify-content:flex-end}.screen{padding:10px 6px;height:56vh;min-height:300px}footer{padding:8px 10px}footer span{max-width:65%}}
</style>
