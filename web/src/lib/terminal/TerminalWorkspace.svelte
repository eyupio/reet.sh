<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import {getProfile,getSyncState,saveProfile,type Machine} from '../api'
  import StartAgentDialog from '../StartAgentDialog.svelte'
  import AddProjectDialog from '../home/AddProjectDialog.svelte'
  import SessionSidebar from './SessionSidebar.svelte'
  import CopyButton from '../CopyButton.svelte'
  import { createTerminal, folderProjectList, machineHealth, readiness, terminalMachines, terminalSessions, updateFolderProject, type FolderProject, type HealthResponse, type TerminalMachine, type TerminalSession } from './api'
  import type { ProjectColour } from '../projectColour'
  import { projectRunning, sessionTabTitle } from '../home'
  import MachineHealthStrip from './MachineHealthStrip.svelte'
  import TerminalPane from './TerminalPane.svelte'
  import SessionScreen from '../sessions/SessionScreen.svelte'
  import SessionTabs from '../sessions/SessionTabs.svelte'
  import { closeTab, openTab } from '../sessions'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import HealthHistory from './HealthHistory.svelte'
  import { renameTerminal, endTerminal, archiveTerminal, terminalClosed } from './api'
  import { TerminalError, validTerminalTitle } from './api'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import TerminalCommandReference from './TerminalCommandReference.svelte'
  import { pageViewport, browserViewport, keyboardLikelyOpen, PHONE_SCREEN } from './sizing'
  import { chromeFocus } from '../shell/chrome.svelte'
  import { ALWAYS_DOCKED, MINI_WIDTH, keepOpen, PANEL_DEFAULT, changeMode, chevronNext, clampPanelWidth, defaultLayout, defaultPanelWidth, dragWidth, isPhone, keyWidth, layoutRecord, panelDocks, panelMode, readLayout, screenShortSide, widestPanel, type PanelMode } from './panelLayout'

  let {userId=0}:{userId?:number}=$props()
  // svelte-ignore state_referenced_locally
  const preferenceKey=`reet-terminal-layout:${userId}`
  let layoutLoaded=false
  // A session's menu on the Sessions page opens it in theatre view (?theatre=1); the address then forgets it.
  let theatre=$state(new URLSearchParams(location.search).get('theatre')==='1')
  let terminalPane=$state<{disconnect:()=>void}>()
  // The sessions panel (see panelLayout.ts): full, the 64 px mini rail, or closed. Full is a docked column beside the
  // terminal from 600 px (below 960 px while Keep open is on, by default from 768 px); on a phone, or a tablet whose person turned
  // Keep open off, it is a drawer over the terminal. A phone is told by its screen's shorter side, not the window's height.
  const readScreen=()=>screenShortSide(window.screen,innerWidth,innerHeight)
  let viewportWidth=$state(innerWidth),screenSide=$state(readScreen()),bodyWidth=$state(0),resizing=$state(false)
  let mode=$state<PanelMode>(panelMode(innerWidth,undefined,undefined,isPhone(innerWidth,readScreen()))),pinned=$state<boolean|undefined>(undefined),sidebarWidth=$state(defaultPanelWidth(innerWidth))
  const phone=$derived(isPhone(viewportWidth,screenSide))
  // Keep open as shown: the person's choice, or on by default from 768 px.
  const keptOpen=$derived(keepOpen(pinned,viewportWidth))
  const panelView=$derived<'docked'|'drawer'|'mini'|'closed'>(phone?(mode==='full'?'drawer':'closed'):mode==='full'?(panelDocks(viewportWidth,pinned)?'docked':'drawer'):mode)
  const panelOpen=$derived(panelView==='docked'||panelView==='drawer')
  // Theatre view hides the panel without changing it.
  const shown=$derived(theatre?'closed':panelView)
  // The separator's 8 px track sits between the panel and the terminal; the panel takes at most 40 % of what is left.
  const SEPARATOR_TRACK=8
  const panelRoom=$derived(Math.max(0,bodyWidth-SEPARATOR_TRACK))
  const panelWidth=$derived(clampPanelWidth(sidebarWidth,panelRoom)),panelMax=$derived(widestPanel(panelRoom))
  // Saved only when a person changes the layout on a tablet or desktop: never on load, never the untouched defaults, never
  // a phone's drawer opening and closing (so a phone visit leaves the wider screen's choice as it was).
  function saveLayout(){
    if(!layoutLoaded||phone)return
    const record=JSON.stringify(layoutRecord(mode,sidebarWidth,pinned))
    try{const stored=localStorage.getItem(preferenceKey);if(stored===record||stored===null&&record===JSON.stringify(defaultLayout(viewportWidth)))return;localStorage.setItem(preferenceKey,record)}catch{/* optional browser preference */}
  }
  // Every change of mode goes through here: expanding the rail on a tablet or desktop turns Keep open on, so it docks.
  function setMode(next:PanelMode){const changed=changeMode(mode,next,pinned,phone);mode=changed.mode;pinned=changed.pinned}
  // Focus moves to the control that undoes the change, never to the page.
  async function focusPanel(selector:string){await tick();workspace?.querySelector<HTMLElement>(selector)?.focus()}
  const TOOLBAR_SESSIONS='.workspace-actions [aria-controls="terminal-sessions"]'
  function toggleSidebar(){if(theatre){theatre=false;setMode('full')}else setMode(panelOpen?'closed':'full');saveLayout()}
  // The terminal's own Show/Hide sessions: shows the panel as it was (the rail stays a rail), or hides it.
  function paneSessions(){if(theatre){theatre=false;if(panelView==='closed')setMode('full')}else setMode(panelView==='closed'?'full':'closed');saveLayout()}
  function chevron(){const next=chevronNext(mode,phone);setMode(next);saveLayout();void focusPanel(next==='mini'?'#terminal-sessions .rail-expand':next==='full'?'#terminal-sessions .collapse':TOOLBAR_SESSIONS)}
  function expand(){setMode('full');saveLayout();void focusPanel('#terminal-sessions .collapse')}
  function findSessions(){setMode('full');saveLayout();void focusPanel('#terminal-sessions input[type=search]')}
  function togglePin(){pinned=!keptOpen;saveLayout()}
  function closeDrawer(){if(panelView==='drawer')mode='closed'}
  // Dragging the separator: the pointer is captured, so leaving the handle keeps the drag. The grid column changes with it
  // and the terminal refits locally once per frame; the host's size is held (holdSize) and sent once when the drag ends.
  let drag:{x:number;width:number}|null=null
  function separatorDown(event:PointerEvent){if(event.pointerType==='mouse'&&event.button!==0)return;event.preventDefault();const handle=event.currentTarget as HTMLElement;handle.setPointerCapture(event.pointerId);handle.focus({preventScroll:true});drag={x:event.clientX,width:panelView==='mini'?MINI_WIDTH:panelWidth};resizing=true}
  function separatorMove(event:PointerEvent){if(!drag||!(event.currentTarget as HTMLElement).hasPointerCapture(event.pointerId))return;const next=dragWidth(drag.width+event.clientX-drag.x,panelRoom);setMode(next.mode);if(next.width!==null)sidebarWidth=next.width}
  function separatorEnd(){if(!drag)return;drag=null;resizing=false;saveLayout()}
  function separatorKey(event:KeyboardEvent){const next=keyWidth(event.key,mode,sidebarWidth,panelRoom);if(!next)return;event.preventDefault();setMode(next.mode);sidebarWidth=next.width;saveLayout()}
  function separatorReset(){setMode('full');sidebarWidth=PANEL_DEFAULT;saveLayout()}
  let machines = $state<TerminalMachine[]>([]), sessions = $state<TerminalSession[]>([])
  // The folder projects, read with the rest of the workspace inventory, for the panel's Projects list and the tab title.
  let projects = $state<FolderProject[] | undefined>()
  let selectedAgent = $state(new URLSearchParams(location.search).get('agent') || '')
  let selectedSession = $state(new URLSearchParams(location.search).get('session') || '')
  let health = $state<HealthResponse>({ serverNow:Date.now(),machines:[] })
  let receivedAt = performance.now(), elapsed = $state(0)
  let loading = $state(true), busy = $state(false), error = $state(''), title = $state('Shell')
  type Creation = { requestId: string; title: string; error: string }
  let creations = $state<Record<string, Creation>>({})
  const pendingCreation = $derived(creations[selectedAgent])
  const requestId = $derived(pendingCreation?.requestId || '')
  let accessDenied=$state(false), machineSearchInput=$state<HTMLInputElement>()
  let creationRequest:AbortController|undefined
  let statusError = $state(''), machineSearch = $state(''), machineSearchOpen = $state(false)
  let machinePrompt = $state<''|'quick'|'create'>('')
  const matchingMachines = $derived(machines.filter(item => `${item.host} ${item.capabilities?.osUser || ''}`.toLowerCase().includes(machineSearch.toLowerCase())))
  const rememberedSessions = new Map<string, string>()
  let workspace = $state<HTMLElement>(), workspaceHeight = $state(Math.max(120, innerHeight - 78))
  // Phone keyboard open while typing in the terminal: this toolbar steps aside, and the shell's chrome with it (keyboardChrome
  // with the 'session' focus, read by AppShell through chromeFocus).
  let keyboard=$state(false)
  // ?viewport-debug=1 shows the measured heights live, for checking a real phone with its keyboard open. Nothing is stored or sent.
  const viewportDebug=new URLSearchParams(location.search).get('viewport-debug')==='1'
  let debugReading=$state('')
  $effect(()=>()=>{chromeFocus.session=false})
  let detailsOpen = $state(false)
  let healthIssue=$state(''),showCreate=$state(false)
  // The Start an agent dialog, with the button that opened it, and the folders the machines have reported (read when it opens).
  let agentStart=$state<{agent:string;opener:HTMLElement|null;dir?:string;projectId?:string}|null>(null),agentFolders=$state<Machine[]|undefined>(),folderRead=0
  // Add a project from the session panel: the same dialog as Home, without leaving the terminal.
  let addingProject=$state<{opener:HTMLElement|null}|null>(null)
  let action = $state<'rename'|'end'|'archive'|null>(null), actionSession = $state(''), rename = $state(''), actionTitle = $state(''), actionBusy = $state(false)
  let removeOnClose=$state(false),removeDefault=$state(false),saveRemoveDefault=$state(false),preferenceError=$state('')
  const endKeys = new Map<string,{key:string;remove:boolean}>()
  let refreshing = false, refreshAgain = false, disposed = false, inventoryVersion = 0
  let refreshRequest: AbortController | undefined
  const machine = $derived(machines.find(item=>item.agentId===selectedAgent))
  const ownSessions = $derived(sessions.filter(item=>item.agentId===selectedAgent))
  const activeSessions = $derived(ownSessions.filter(item=>['creating','running','end_requested'].includes(item.state)))
  const accountActive = $derived(sessions.filter(item=>['creating','running','end_requested'].includes(item.state)).length)
  const atQuota = $derived(activeSessions.length>=8 || accountActive>=32)
  const session = $derived(ownSessions.find(item=>item.id===selectedSession))
  const machineStatus = $derived(health.machines.find(item=>item.agentId===selectedAgent))
  const setup = $derived(readiness(machine))
  const serverNow = $derived(health.serverNow+elapsed)
  const runningByProject = $derived(projectRunning(sessions))
  const currentProjectId = $derived(session?.projectId ?? '')
  // The tab names the work: the session, then its project (or its machine when it has none), then reet.
  const tabTitle = $derived(sessionTabTitle(session, projects?.find(p => p.id === session?.projectId)?.name, machine?.host))
  // The tab names the work, not just the page: two reet tabs are told apart by the session they hold. Setting it here, rather
  // than in another <svelte:head>, keeps one <title> in the document.
  $effect(() => { document.title = tabTitle })
  $effect(()=>{if(machineSearchOpen||machinePrompt)machineSearchInput?.focus()})
  // The session screen's tabs (R20): the sessions opened in this browser tab since it loaded, in memory only, at most eight.
  // A session that leaves the listing (removed from history) leaves the strip.
  let openTabs=$state<string[]>([])
  $effect(()=>{const id=session?.id;if(id)untrack(()=>{openTabs=openTab(openTabs,id)})})
  // A session that left the listing leaves the tabs too, so it never counts towards the eight nor comes back on a close.
  // Only a listing that was read successfully and lists something prunes: an empty or failed read never drops open tabs.
  let listingLoaded=$state(false)
  $effect(()=>{if(!listingLoaded||!sessions.length)return;const listed=new Set(sessions.map(item=>item.id));untrack(()=>{const kept=openTabs.filter(id=>listed.has(id));if(kept.length!==openTabs.length)openTabs=kept})})
  const tabSessions=$derived(openTabs.map(id=>sessions.find(item=>item.id===id)).filter((item):item is TerminalSession=>!!item))
  // Closing a tab closes the view, never the session: the current one gives way to its neighbour, the last one to the list.
  function closeSessionTab(item:TerminalSession){
    const next=closeTab(openTabs,item.id,selectedSession)
    openTabs=next.tabs
    if(next.next===selectedSession)return
    const shown=sessions.find(other=>other.id===next.next)
    if(shown)chooseSession(shown);else location.assign('/sessions')
  }

  function remember() { const url=new URL(location.href);url.searchParams.set('view','terminal');url.searchParams.delete('theatre');if(selectedAgent)url.searchParams.set('agent',selectedAgent);else url.searchParams.delete('agent');if(selectedSession)url.searchParams.set('session',selectedSession);else url.searchParams.delete('session');history.replaceState(null,'',url) }
  function chooseMachine(id: string) {
    if(selectedAgent && selectedSession)rememberedSessions.set(selectedAgent,selectedSession)
    selectedAgent=id
    const previous=rememberedSessions.get(id)
    selectedSession=sessions.find(item=>item.agentId===id&&item.id===previous)?.id || sessions.find(item=>item.agentId===id&&item.state==='running')?.id || ''
    title=creations[id]?.title || 'Shell';error='';machineSearchOpen=false;remember()
  }
  function chooseSession(item: TerminalSession) { if(selectedAgent&&selectedSession)rememberedSessions.set(selectedAgent,selectedSession);selectedAgent=item.agentId;selectedSession=item.id;title=creations[selectedAgent]?.title||'Shell';error='';remember();closeDrawer() }
  async function refresh(signal?: AbortSignal) {
    if(disposed)return
    if(refreshing){refreshAgain=true;return}
    refreshing=true
    const version=inventoryVersion, request=new AbortController();refreshRequest=request
    const stop=()=>request.abort()
    if(signal?.aborted)request.abort()
    signal?.addEventListener('abort',stop,{once:true})
    const timeout=setTimeout(stop,10000)
    const firstLoad=loading
    try {
      const [nextMachines,nextSessions,nextHealth,nextProjects]=await Promise.all([terminalMachines(request.signal),terminalSessions(request.signal),machineHealth(request.signal).then(data=>({data,error:''})).catch(()=>({data:null,error:'Resource readings are temporarily unavailable. Existing readings are marked by their observation time.'})),folderProjectList(request.signal).then(answer=>({projects:answer.projects})).catch(()=>({projects:null}))])
      if(disposed)return
      // A pre-mutation observation must not overwrite a confirmed local result.
      if(version!==inventoryVersion){refreshAgain=true;return}
      statusError='';accessDenied=false
      const removedAfterClose=sessions.find(item=>item.id===selectedSession)?.removeHistory&&!nextSessions.some(item=>item.id===selectedSession)
      machines=nextMachines;sessions=nextSessions;listingLoaded=true;healthIssue=nextHealth.error
      if(nextProjects.projects)projects=nextProjects.projects
      if(removedAfterClose){selectedSession='';remember()}
      if(nextHealth.data){health=nextHealth.data;receivedAt=performance.now();elapsed=0}
      if(firstLoad&&!selectedSession)selectedSession=sessions.find(item=>item.agentId===selectedAgent&&item.state==='running')?.id||''
      remember()
    } catch(e){
      if(!signal?.aborted&&!disposed){
        statusError=request.signal.aborted?'Status refresh timed out. Your existing shell is preserved.':e instanceof Error?e.message:'Could not refresh machines.'
        if(e instanceof TerminalError&&[401,403].includes(e.status)){accessDenied=true;machines=[];sessions=[]}
      }
    }
    finally{
      clearTimeout(timeout);signal?.removeEventListener('abort',stop);request.abort();refreshing=false;loading=false
      if(refreshAgain&&!disposed){refreshAgain=false;void refresh(signal)}
    }
  }
  function canCreateFor(agent:string){return !busy&&!actionBusy&&!accessDenied&&!!machines.find(m=>m.agentId===agent&&m.serverEnabled!==false)?.ready&&(!!creations[agent]||accountActive<32&&sessions.filter(s=>s.agentId===agent&&['creating','running','end_requested'].includes(s.state)).length<8)}
  function quickCreate(agent:string){if(!canCreateFor(agent))return;if(selectedAgent&&selectedSession)rememberedSessions.set(selectedAgent,selectedSession);selectedAgent=agent;selectedSession='';title=creations[agent]?.title||'Shell';showCreate=false;error='';remember();closeDrawer();void create()}
  function requestMachinePrompt(mode:'quick'|'create'){machineSearch='';machinePrompt=mode}
  function pickMachineForNew(id:string){const mode=machinePrompt;machinePrompt='';if(mode==='quick')return quickCreate(id);if(selectedAgent&&selectedSession)rememberedSessions.set(selectedAgent,selectedSession);selectedAgent=id;selectedSession='';title=creations[id]?.title||'Shell';error='';showCreate=true;remember();closeDrawer()}
  function newSession(){if(!machines.length)return;if(!selectedAgent)return requestMachinePrompt('create');if(session){title=pendingCreation?.title||title;showCreate=true}else workspace?.querySelector<HTMLInputElement>('.create-form input')?.focus()}
  function submitCreate(){if(!selectedAgent&&machines.length)return requestMachinePrompt('create');void create()}
  async function create() {
    if(busy||!machine?.ready||!validTerminalTitle(title.trim())||atQuota&&!requestId)return
    busy=true;error=''
    // One immutable retry operation per machine, including its original title.
    const agent=selectedAgent
    const operation=creations[agent] || {requestId:crypto.randomUUID(),title:title.trim(),error:''}
    creations={...creations,[agent]:{...operation,error:''}}
    const request=new AbortController();creationRequest=request
    const timeout=setTimeout(()=>request.abort(),15000)
    try {
      const created=await createTerminal(agent,operation.requestId,operation.title,100,30,request.signal)
      if(disposed)return
      inventoryVersion++
      // A retry can resolve to an old closed record; let the filtered history list
      // determine visibility instead of reintroducing an archived session locally.
      if(!terminalClosed(created.state))sessions=[created,...sessions.filter(item=>item.id!==created.id)]
      if(creations[agent]?.requestId===operation.requestId){const next={...creations};delete next[agent];creations=next}
      if(selectedAgent===agent){selectedSession=created.id;showCreate=false;remember()}
      await refresh()
    } catch(e){
      if(disposed)return
      if(e instanceof TerminalError&&e.status===400){const next={...creations};delete next[agent];creations=next;if(selectedAgent===agent)error=e.message}
      else creations={...creations,[agent]:{...operation,error:e instanceof Error?e.message:'Creation could not be confirmed. Retry the same operation.'}}
    } finally{clearTimeout(timeout);busy=false}
  }
  function openAgentStart(agent:string,opener:HTMLElement|null){
    if(accessDenied||!machines.length||agentStart)return
    agentStart={agent,opener};agentFolders=undefined
    const read=++folderRead
    void getSyncState().then(state=>{if(!disposed&&read===folderRead)agentFolders=state.machines??[]}).catch(()=>{/* the home folder and a typed path still work */})
  }
  function closeAgentStart(){agentStart=null;folderRead++}
  // A project's + opens Start an agent in that project's folder: same machine, folder and project, so the new session
  // belongs to the project from its first frame. (Home's New session here sets the launch bar's project instead.)
  function startInProject(project:FolderProject,opener:HTMLElement){
    if(accessDenied||agentStart||!machines.length)return
    openAgentStart(project.host,opener)
    agentStart={agent:project.host,opener,dir:project.dir,projectId:project.id}
  }
  function openAddProject(opener:HTMLElement){
    if(accessDenied||addingProject)return
    addingProject={opener}
  }
  function projectAdded(project:FolderProject){
    inventoryVersion++
    projects=[...(projects??[]).filter(item=>item.id!==project.id),project].sort((a,b)=>a.position-b.position)
    void refresh()
  }
  // The colour is saved on the project (it follows the person to every browser); the rail shows the choice at once and
  // puts the saved project back if the write is refused.
  async function saveProjectColour(project:FolderProject,colour:ProjectColour){
    if((project.colour||'')===colour)return
    const before=projects??[]
    projects=before.map(p=>p.id===project.id?{...p,colour}:p)
    try{
      const saved=await updateFolderProject(project.id,{colour},project.version)
      projects=(projects??[]).map(p=>p.id===saved.id?saved:p)
    }catch(e){
      projects=before
      error=e instanceof Error?e.message:'That colour could not be saved. Try again in a moment.'
    }
  }
  // A started agent lands exactly as a new shell does: it joins the list and the workspace switches to it.
  function agentStarted(item:TerminalSession){inventoryVersion++;if(!terminalClosed(item.state))sessions=[item,...sessions.filter(other=>other.id!==item.id)];showCreate=false;chooseSession(item);void refresh()}
  function openAction(kind:'rename'|'end'|'archive',target=session) { if(!target)return;actionSession=target.id;actionTitle=target.title;rename=target.title;removeOnClose=endKeys.get(target.id)?.remove??(target.state==='end_requested'?(target.removeHistory??false):removeDefault);saveRemoveDefault=false;preferenceError='';action=kind }
  async function performAction() {
    const kind=action,id=actionSession;action=null;if(!kind||actionBusy)return
    actionBusy=true;error=''
    try {
      if(kind==='rename'){if(!validTerminalTitle(rename.trim())){error='Use a session name of 1–80 characters without control characters.';return}await renameTerminal(id,rename.trim())}
      else if(kind==='archive'){
        await archiveTerminal(id)
        if(disposed)return
        sessions=sessions.filter(item=>item.id!==id)
        for(const [agent,selected] of rememberedSessions)if(selected===id)rememberedSessions.delete(agent)
        if(selectedSession===id){selectedSession='';remember()}
      }
      else {let operation=endKeys.get(id);if(!operation){operation={key:crypto.randomUUID(),remove:removeOnClose};endKeys.set(id,operation)}await endTerminal(id,operation.key,operation.remove);if(saveRemoveDefault){try{await saveProfile({terminalRemoveOnClose:operation.remove});removeDefault=operation.remove}catch{preferenceError='Session stop requested, but your default could not be saved. Change it in Account → Profile.'}}}
      inventoryVersion++
      await refresh()
    } catch(e){if(!disposed)error=e instanceof Error?e.message:'Action could not be confirmed. Retry the same action.'}
    finally{actionBusy=false}
  }
  onMount(() => {
    let stored:string|null=null
    try{stored=localStorage.getItem(preferenceKey)}catch{/* optional browser preference */}
    const saved=readLayout(stored)
    sidebarWidth=saved.width??defaultPanelWidth(innerWidth);pinned=saved.pinned;mode=panelMode(innerWidth,saved.mode,saved.pinned,isPhone(innerWidth,readScreen()))
    layoutLoaded=true
    // Only this workspace asks Android browsers to resize the page for the keyboard; other pages keep the default.
    const viewportMeta=document.querySelector<HTMLMetaElement>('meta[name="viewport"]'),viewportContent=viewportMeta?.getAttribute('content')??null
    if(viewportMeta&&viewportContent!==null&&!/interactive-widget/.test(viewportContent))viewportMeta.setAttribute('content',`${viewportContent}, interactive-widget=resizes-content`)
    let frame=0
    const coarse=matchMedia('(pointer:coarse)'),narrow=matchMedia(PHONE_SCREEN)
    const editing=()=>{const focus=document.activeElement;return focus instanceof HTMLElement&&!!focus.closest('.pane')&&focus.matches('textarea,input,[contenteditable]:not([contenteditable="false"])')}
    const measure=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(()=>{
      if(!workspace)return
      const usable=pageViewport.read(browserViewport())
      const typing=editing()
      chromeFocus.session=typing
      keyboard=keyboardLikelyOpen(usable.shrink,{coarse:coarse.matches,narrow:narrow.matches,editing:typing,scale:window.visualViewport?.scale??1})
      workspaceHeight=Math.max(120,usable.height-Math.max(0,workspace.getBoundingClientRect().top-usable.top)-8)
      if(viewportDebug){const visual=window.visualViewport;debugReading=`visualViewport.height ${visual?Math.round(visual.height):'none'} · offsetTop ${visual?Math.round(visual.offsetTop):'none'}\ninnerHeight ${innerHeight} · screen.height ${screen.height}\nused height ${Math.round(usable.height)} · keyboard ${keyboard?'open':'closed'}`}
    })}
    const observer=new ResizeObserver(measure),header=document.querySelector('.site-header')
    if(header)observer.observe(header)
    // The shell hides and shows the site's chrome (keyboardChrome) after this measure has set the session focus: measure again
    // whenever it does, so the height is never one taken while the top bar was aside.
    const chrome=new MutationObserver(measure)
    chrome.observe(document.documentElement,{attributes:true,attributeFilter:['data-chrome-aside','data-bottom-tabs']})
    // A docked panel that would become a drawer (a narrower window, a phone) closes rather than cover the terminal.
    const viewportChanged=()=>{const wasDocked=panelView==='docked';viewportWidth=innerWidth;screenSide=readScreen();if(wasDocked&&!panelDocks(viewportWidth,pinned,phone))mode='closed'}
    window.addEventListener('resize',viewportChanged)
    measure();window.addEventListener('resize',measure);window.visualViewport?.addEventListener('resize',measure);window.visualViewport?.addEventListener('scroll',measure);document.addEventListener('focusin',measure);document.addEventListener('focusout',measure)
    return()=>{if(viewportMeta&&viewportContent!==null)viewportMeta.setAttribute('content',viewportContent);cancelAnimationFrame(frame);observer.disconnect();chrome.disconnect();window.removeEventListener('resize',viewportChanged);window.removeEventListener('resize',measure);window.visualViewport?.removeEventListener('resize',measure);window.visualViewport?.removeEventListener('scroll',measure);document.removeEventListener('focusin',measure);document.removeEventListener('focusout',measure)}
  })
  onMount(()=>{
    const abort=new AbortController();void refresh(abort.signal)
    void getProfile().then(result=>{if(!disposed)removeDefault=result.profile.terminalRemoveOnClose??false}).catch(()=>{/* safe default keeps history */})
    const poll=setInterval(()=>{if(document.visibilityState==='visible')void refresh(abort.signal)},15000)
    const clock=setInterval(()=>{elapsed=performance.now()-receivedAt},1000)
    const wake=()=>{if(document.visibilityState==='visible')void refresh(abort.signal)}
    document.addEventListener('visibilitychange',wake);window.addEventListener('online',wake)
    return()=>{disposed=true;abort.abort();refreshRequest?.abort();creationRequest?.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}
  })
</script>

{#snippet createForm()}
  <form class="create-form" onsubmit={event => { event.preventDefault(); submitCreate() }}>
    {#if pendingCreation?.error}<p class="creation-error" role="alert">{pendingCreation.error} The creation is unconfirmed. Retry checks the original operation.</p>{/if}
    <label>New session name<input bind:value={title} disabled={busy || !!requestId} maxlength="80" required placeholder="Build, deploy, investigate…" /></label>
    <button class="btn primary" disabled={busy || !validTerminalTitle(title.trim()) || (selectedAgent ? !machine?.ready || atQuota && !requestId : !machines.length)}>{busy ? 'Creating…' : requestId ? 'Retry creation' : 'New terminal'}</button>
    <button type="button" class="btn new-agent" disabled={!machines.length} onclick={event=>openAgentStart(selectedAgent,event.currentTarget)}>New agent session</button>
    {#if !machine?.ready}<p class="create-reason" role="status">{setup || 'Choose a machine before creating a shell.'}</p>{/if}
    <p>{activeSessions.length} / 8 active on this machine · {accountActive} / 32 across your account{atQuota ? ' · Session limit reached' : ''}. Existing shells keep running when you switch or disconnect.</p>
  </form>
{/snippet}

<section class="workspace" class:theatre class:keyboard class:resizing bind:this={workspace} style={`--workspace-height:${workspaceHeight}px;--sidebar-width:${sidebarWidth}px;--panel-width:${shown==='mini'?MINI_WIDTH:panelWidth}px`} aria-label="Remote terminal workspace">
  <header class="workspace-toolbar">
    <div class="workspace-title"><a href="/sessions" aria-label="Back to sessions">←</a><h1>Sessions</h1></div>
    {#if panelView==='closed'}<button class="btn quick-machine" aria-label={selectedAgent ? `New terminal on ${machine?.host || 'selected machine'}` : 'Choose a machine for a new terminal'} title={selectedAgent ? 'Start a terminal on this machine' : 'Choose a machine for a new terminal'} disabled={selectedAgent ? !canCreateFor(selectedAgent) : !machines.length} onclick={()=>selectedAgent?quickCreate(selectedAgent):requestMachinePrompt('quick')}>+</button>{/if}
    <button class="btn find-machine" onclick={()=>{machineSearch='';machineSearchOpen=true}} aria-label="Find machine">Find</button>
    <!-- The session screen's tabs share this row, so the terminal keeps its height. -->
    {#if session && !loading && !accessDenied && !phone}<SessionTabs tabs={tabSessions} current={session.id} onselect={chooseSession} onclose={closeSessionTab} />{/if}
    <div class="workspace-actions"><button class="btn" aria-expanded={panelOpen} aria-controls="terminal-sessions" onclick={toggleSidebar}>{panelOpen?'Sessions':'Show sessions'} <span class="count">{accountActive}</span></button><button class="btn primary new-button" disabled={!machines.length} onclick={newSession} aria-label="New session"><span class="wide-label">New session</span><span class="mobile-label">New</span></button><button class="btn" aria-label="Details & help" onclick={() => detailsOpen = true}><span class="wide-label">Details &amp; help</span><span class="mobile-label">Help</span></button></div>
  </header>
  {#if preferenceError}<p class="workspace-notice bad" role="status">{preferenceError}</p>{/if}
  {#if statusError}<p class="workspace-notice bad" role="status">{statusError} <button onclick={()=>void refresh()}>Retry status</button></p>{/if}
  {#if pendingCreation && !showCreate && session}<p class="workspace-notice" role="status">A creation on this machine is unconfirmed. <button onclick={()=>{title=pendingCreation?.title||title;showCreate=true}}>Review creation</button></p>{/if}
  {#if error && !showCreate}<p class="workspace-notice bad" role="alert">{error} <button onclick={() => void refresh()}>Retry status</button></p>{/if}
  <div class="workspace-body" class:docked={shown==='docked'||shown==='mini'} bind:clientWidth={bodyWidth}>
    {#if shown!=='closed'}<SessionSidebar {machines} {sessions} {projects} projectRunning={runningByProject} currentProject={currentProjectId} onprojectstart={startInProject} onprojectadd={openAddProject} onprojectcolour={saveProjectColour} health={health.machines} {serverNow} {selectedAgent} {selectedSession} layout={shown} collapseLabel={phone?'Close session navigation':'Shrink sessions to a mini rail'} pinnable={!phone&&viewportWidth<ALWAYS_DOCKED} pinned={keptOpen} onpin={togglePin} onexpand={expand} onfind={findSessions} onnewsession={newSession} preferenceKey={`${preferenceKey}:sessions`} onnew={quickCreate} cancreate={canCreateFor} onselect={chooseSession} onclose={chevron} onrefresh={()=>void refresh()} onagent={openAgentStart} onaction={openAction} ondisconnect={()=>terminalPane?.disconnect()} busy={actionBusy} />{/if}
    {#if shown==='docked'||shown==='mini'}
      <!-- A focusable window splitter: drag (pointer capture), Left/Right 16 px, Home the mini rail, End the widest, double-click 280 px. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
      <div class="panel-separator" class:mini={shown==='mini'} role="separator" tabindex="0" aria-label="Resize session navigation" aria-controls="terminal-sessions" aria-orientation="vertical" aria-valuemin={MINI_WIDTH} aria-valuemax={panelMax} aria-valuenow={shown==='mini'?MINI_WIDTH:panelWidth} aria-valuetext={shown==='mini'?'Mini rail':`${panelWidth} pixels`} title="Drag to resize · double-click resets" onpointerdown={separatorDown} onpointermove={separatorMove} onpointerup={separatorEnd} onpointercancel={separatorEnd} onlostpointercapture={separatorEnd} onkeydown={separatorKey} ondblclick={separatorReset}></div>
    {/if}
    <div class="terminal-stage">
      {#if loading}<div class="empty" role="status"><span class="empty-icon" aria-hidden="true">›_</span><h2>Finding your shells…</h2><p>Connecting to your existing workspace.</p></div>
      {:else if accessDenied}<div class="empty"><h2>Terminal access unavailable</h2><p>Your account no longer has terminal access. Sign in again or ask the owner to check your permissions. Existing shells were not ended.</p><button class="btn" onclick={()=>void refresh()}>Check access again</button></div>
      {:else if !machines.length}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>Connect your first machine</h2><p>Enroll a machine and enable terminal access locally to open a shell here.</p><a class="btn primary" href="/machines#add-machine">Connect a machine</a></div>
      {:else if session}
        <!-- The session screen hosts the live terminal pane, unchanged: tabs and header above it, the context panel beside it. -->
        <SessionScreen {session} {sessions} {machines} health={health.machines} {projects} now={serverNow} {theatre} {keyboard} busy={actionBusy}
          onaction={kind => openAction(kind)} ontheatre={() => theatre = !theatre} onrefresh={() => void refresh()} onsignedout={() => { accessDenied = true }}>
          {#if setup || session.state === 'end_requested'}<div class="workspace-notice" role="status">{session.state === 'end_requested' ? 'End requested. Waiting for this machine to confirm; the shell still counts towards its limit.' : setup}<button onclick={() => detailsOpen = true}>View details</button></div>{/if}
          {#key session.id}<TerminalPane {keyboard} footnote={phone ? '' : `Approvals: ${session.yolo ? 'YOLO' : 'Ask me'}`} holdSize={resizing} sessionsVisible={shown!=='closed'} onsessions={paneSessions} bind:theatre bind:this={terminalPane} {session} historySupported={machine?.capabilities?.commandHistory??false} uploadsSupported={machine?.capabilities?.uploads??false} uploadsOff={machine?.capabilities?.uploadsOff??false} host={machine?.host || 'Unavailable machine'} user={machine?.capabilities?.osUser || ''} onrename={() => openAction('rename')} onend={() => openAction('end')} onarchive={() => openAction('archive')} archiveDisabled={actionBusy} actionDisabled={actionBusy || machine?.serverEnabled === false} />{/key}
        </SessionScreen>
      {:else}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>{selectedSession ? 'This session is unavailable' : selectedAgent && !machine ? 'This machine is unavailable' : 'Your next shell starts here.'}</h2><p>{selectedSession ? 'Choose an existing session or create one explicitly. No replacement shell was created.' : setup || 'Create a terminal, then return to the same session whenever you need it.'}</p>{@render createForm()}{#if setup}<button class="btn" onclick={() => detailsOpen = true}>View setup &amp; connection details</button>{/if}</div>{/if}
    </div>
  </div>
</section>

{#if viewportDebug}<pre class="viewport-debug" aria-hidden="true">{debugReading}</pre>{/if}
{#if machineSearchOpen}<WorkspaceDialog title="Find a machine" onclose={()=>machineSearchOpen=false}>
  <label class="machine-search">Machine or shell user<input type="search" bind:this={machineSearchInput} bind:value={machineSearch} placeholder="Search connected machines…" /></label>
  <nav class="machine-results" aria-label="Matching machines">{#each matchingMachines as item (item.agentId)}<button class="machine-result" aria-current={item.agentId===selectedAgent?'true':undefined} onclick={()=>chooseMachine(item.agentId)}><strong>{item.host}</strong><span>{item.capabilities?.osUser || 'Shell user unavailable'} · {item.ready?'Ready':readiness(item)}</span></button>{:else}<p>No machines match.</p>{/each}</nav>
</WorkspaceDialog>{/if}
{#if machinePrompt}<WorkspaceDialog title="Choose a machine" onclose={()=>machinePrompt=''}>
  <p class="dialog-copy">Select the machine where this terminal should run.</p>
  <label class="machine-search">Machine or shell user<input type="search" bind:this={machineSearchInput} bind:value={machineSearch} placeholder="Search connected machines…" /></label>
  <nav class="machine-results" aria-label="Machines for a new terminal">{#each matchingMachines as item (item.agentId)}<button class="machine-result" aria-current={item.agentId===selectedAgent?'true':undefined} disabled={!canCreateFor(item.agentId)} onclick={()=>pickMachineForNew(item.agentId)}><strong>{item.host}</strong><span>{item.capabilities?.osUser || 'Shell user unavailable'} · {item.ready?'Ready':readiness(item)}</span></button>{:else}<p>No machines match.</p>{/each}</nav>
</WorkspaceDialog>{/if}
{#if showCreate}<WorkspaceDialog title="New terminal session" onclose={() => showCreate = false}><p class="dialog-copy">Create a shell on <strong>{machine?.host || 'the selected machine'}</strong>. Changing machines does not stop existing sessions.</p>{#if error}<p class="dialog-copy" role="alert">{error}</p>{/if}{@render createForm()}</WorkspaceDialog>{/if}
{#if agentStart}<StartAgentDialog {machines} snapshot={agentFolders} initialAgent={agentStart.agent} initialDir={agentStart.dir ?? ''} projectId={agentStart.projectId ?? ''} opener={agentStart.opener} onstarted={agentStarted} onaddmachine={()=>location.assign('/machines#add-machine')} onclose={closeAgentStart} />{/if}
{#if addingProject}<AddProjectDialog {machines} opener={addingProject.opener} onadded={projectAdded} onsignedout={()=>{accessDenied=true;addingProject=null}} onclose={()=>addingProject=null} />{/if}
{#if detailsOpen}<WorkspaceDialog title="Machine & terminal details" onclose={() => detailsOpen = false}>
  <p class="dialog-copy">{machine?.host || 'No machine selected'}{machine?.capabilities?.osUser ? ` · Shell user ${machine.capabilities.osUser}` : ''}</p>
  {#if setup}<section class="setup-notice"><h3>Terminal setup</h3><p>{setup}</p>{#if machine?.serverEnabled !== false && !machine?.capabilities?.terminalEnabled}<pre>reet agent terminal setup</pre><CopyButton text={'reet agent terminal setup'} label="Copy local terminal setup" />{:else if machine?.capabilities?.readiness === 'supervisor_unavailable'}<pre>reet agent stop
reet agent service install</pre><CopyButton text={'reet agent stop\nreet agent service install'} label="Copy service install command" />{/if}</section>{/if}
  {#if healthIssue}<p class="dialog-copy" role="status">{healthIssue}</p>{/if}
  <section aria-label="Machine diagnostics"><h3>Machine health</h3><MachineHealthStrip health={machineStatus} {serverNow} />{#key selectedAgent}<HealthHistory agent={selectedAgent} />{/key}</section>
  <TerminalCommandReference />
  <section class="shortcut-help"><h3>Keyboard &amp; clipboard</h3><p>Ctrl+C interrupts the shell. On Windows/Linux, Ctrl+C copies selected terminal output; use Interrupt to send Ctrl+C when text is selected. On Mac, Cmd+C copies.</p><p>Ctrl+V / Cmd+V pastes. Ctrl+Shift+C/V are alternatives. Right-click the terminal for Copy/Paste; Shift+right-click keeps the browser menu. Ctrl+X stays a shell/editor key; cut works normally in editable app fields.</p><p>Multiple lines, control characters and large pastes need review. Disconnect keeps the shell running. End session stops its running work.</p></section>
</WorkspaceDialog>{/if}
{#if action === 'rename'}<ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={() => void performAction()} oncancel={() => action = null}><label>Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>{:else if action === 'end'}<ConfirmDialog title={`End ${actionTitle}?`} body="This stops the shell and its running jobs. Disconnecting keeps them running. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={() => void performAction()} oncancel={() => action = null}><div class="end-options"><div class="option"><label><input type="checkbox" bind:checked={removeOnClose} disabled={endKeys.has(actionSession)||sessions.find(item=>item.id===actionSession)?.state==='end_requested'} /><span>Remove from history when this session ends</span></label><p>Files are kept. Pending sessions remain visible until the machine confirms they have ended.</p></div><div class="option"><label><input type="checkbox" bind:checked={saveRemoveDefault} /><span>Use this as my account default</span></label></div><a href="/account#profile">Manage terminal preferences →</a></div></ConfirmDialog>{:else if action === 'archive'}<ConfirmDialog title={`Remove ${actionTitle} from history?`} body="This removes the closed session from your history. Files on the machine are kept. You cannot restore it from the web interface." confirmLabel="Remove from history" onconfirm={()=>void performAction()} oncancel={()=>action=null} />{/if}

<style>
  .workspace.theatre>.workspace-toolbar{display:none}
  /* Keyboard focus layout (phones only, see keyboardLikelyOpen): the terminal toolbar keeps Show sessions. */
  .workspace.keyboard>.workspace-toolbar{display:none}
  /* A phone's session screen has its own back arrow in the slim header (held sideways the header gives its rows to the terminal, so the toolbar keeps its arrow). */
  @media (max-width:650px){.workspace:has(:global(.session-screen)) .workspace-title>a{display:none}}
  .workspace{height:var(--workspace-height);min-height:0;display:flex;flex-direction:column;border:1px solid var(--line);border-radius:12px;background:var(--card);overflow:hidden}.workspace-toolbar{display:flex;align-items:center;gap:14px;padding:8px 12px;border-bottom:1px solid var(--line);flex-shrink:0}.workspace-title{display:flex;align-items:center;gap:8px}.workspace-title>a{font-size:1.2rem;text-decoration:none;padding:4px}.workspace-title h1{font:500 1.2rem var(--serif);margin:0}.workspace-actions{display:flex;gap:6px;margin-left:auto;flex-shrink:0}.workspace-actions .btn{font-size:.75rem;padding:6px 10px;min-height:34px}.workspace-actions .count{font-size:.65rem;border-radius:20px;background:var(--tint);padding:1px 5px}.workspace-body{display:grid;grid-template-columns:minmax(0,1fr);flex:1;min-height:0;position:relative}.workspace-body.docked{grid-template-columns:var(--panel-width) 8px minmax(0,1fr)}.terminal-stage{min-height:0;min-width:0;display:flex;flex-direction:column;overflow:hidden}.terminal-stage :global(.pane:not(.maximised)){flex:1;min-height:0;border:0;border-radius:0;box-shadow:none}.empty{display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:24px;flex:1;min-height:0;overflow:auto;color:var(--muted)}.empty-icon{font:500 2.2rem var(--mono);color:var(--accent-text)}.empty h2{font:500 1.6rem var(--serif);color:var(--ink);margin:12px 0}.empty>p{font-size:.85rem;max-width:55ch;margin:0 0 20px}.create-form{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;max-width:480px;width:100%;text-align:left;align-items:end}.create-form label{display:grid;gap:6px;font-size:.75rem}.create-form input{width:100%;min-width:0}.create-form .new-agent{grid-column:1/-1;justify-self:start}.create-form p{grid-column:1/-1;font-size:.72rem;color:var(--muted);line-height:1.5;margin:0 0 16px}.workspace-notice{margin:0;padding:7px 12px;background:var(--tint);font-size:.74rem;flex-shrink:0;display:flex;align-items:center;gap:10px;justify-content:space-between}.workspace-notice button{border:0;background:none;text-decoration:underline;cursor:pointer;white-space:nowrap;color:inherit;font:inherit}.bad{color:var(--bad)}.dialog-copy,.setup-notice p,.shortcut-help p{font-size:.8rem;color:var(--muted);line-height:1.6;margin:0}.setup-notice{border:1px solid var(--line);border-left:3px solid var(--accent);padding:14px;border-radius:8px}.setup-notice pre{font-size:.75rem;white-space:pre-wrap;overflow-wrap:anywhere}h3{font-size:.85rem;margin:0 0 10px}.shortcut-help p+p{margin-top:8px}
  .creation-error{color:var(--bad)!important}.find-machine{font-size:.72rem;padding:6px 8px;min-height:34px}.machine-search{display:grid;gap:8px;font-size:.8rem}.machine-results{display:grid;gap:8px;max-height:50dvh;overflow:auto}.machine-result{display:grid;gap:5px;text-align:left;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);cursor:pointer}.machine-result[aria-current=true]{border-color:var(--accent)}.machine-result span{font-size:.74rem;color:var(--muted)}
  .end-options{display:grid;gap:14px;margin-top:16px;font-size:.8rem}.end-options .option{display:grid;gap:5px}.end-options label{display:flex;align-items:flex-start;gap:9px;line-height:1.5;cursor:pointer}.end-options input{flex:none;width:16px;height:16px;min-height:0;margin:2px 0 0;padding:0;border:0;background:none;accent-color:var(--accent)}.end-options p,.end-options a{margin:0;font-size:.72rem;line-height:1.6;color:var(--muted)}.end-options p{margin-left:25px}.end-options a{justify-self:start;color:var(--accent-text)}
  .mobile-label{display:none}
  .viewport-debug{position:fixed;top:4px;right:4px;z-index:200;margin:0;padding:4px 6px;max-width:calc(100vw - 8px);white-space:pre-wrap;font:10px/1.3 var(--mono);color:#fff;background:rgb(0 0 0 / .72);border-radius:4px;pointer-events:none;user-select:none}
  @media(max-height:500px) and (pointer:coarse) and (orientation:landscape){.workspace-toolbar{padding-top:0;padding-bottom:0}}
  /* The separator's 8 px track plus 8 px either side: a 24 px hit area over the panel's padding and the terminal's edge. On a touch
     tablet its grip (the middle 88 px) is 44 px wide and reaches into the panel: it ends 4 px past the track's middle, so over the
     terminal it reaches no further than the track does, and the panel's padding there is wide enough that it covers no control.
     Beside the mini rail (64 px, no spare padding) there is no grip: the 24 px track is the grab area and covers no rail button. */
  .panel-separator{position:relative;z-index:3;margin:0 -8px;display:flex;align-items:center;justify-content:center;cursor:col-resize;touch-action:none;outline:0}.panel-separator::after{content:'';width:4px;height:40px;border-radius:2px;background:var(--line)}.panel-separator:hover::after,.workspace.resizing .panel-separator::after{background:var(--accent);height:64px}.panel-separator:focus-visible::after{background:var(--accent);height:64px;outline:2px solid var(--accent);outline-offset:3px}
  .workspace.resizing{cursor:col-resize;user-select:none}
  @media(max-width:650px){.workspace{border-radius:8px}.workspace-toolbar{gap:6px;padding:5px 6px}.workspace-title h1{display:none}.workspace-actions>button[aria-controls="terminal-sessions"]{max-width:52px;white-space:normal;line-height:1.1}.quick-machine{padding:4px 7px;min-width:28px}.workspace-toolbar .find-machine{display:none}.workspace-title>a{padding:3px}.workspace-actions{gap:4px}.workspace-actions .btn{padding:5px 7px;min-height:32px;font-size:.67rem}.workspace-actions .count,.workspace-actions .wide-label{display:none}.workspace-actions .mobile-label{display:inline}.empty{padding:20px 14px}.empty h2{font-size:1.35rem}.create-form{grid-template-columns:minmax(0,1fr)}.create-form p{grid-column:1}.workspace-notice{font-size:.65rem;padding:5px 8px}}
  @media(pointer:coarse){.workspace-actions .btn{min-height:40px;min-width:40px}.quick-machine{min-width:40px}.workspace-toolbar .find-machine{min-height:40px}.workspace-title>a{display:inline-flex;align-items:center;justify-content:center;min-width:40px;min-height:40px;box-sizing:border-box}}
  /* A touch tablet (wider than a phone, not a phone held sideways): the toolbar's targets are 44 px, as everything around the session screen; the panel's separator has a 44 × 88 px grip. */
  @media(pointer:coarse) and (min-width:651px) and (max-width:1099px) and (min-height:501px){.panel-separator:not(.mini)::before{content:'';position:absolute;top:50%;right:calc(50% - 4px);width:44px;height:88px;transform:translateY(-50%)}}
  @media(pointer:coarse) and (min-width:651px) and (max-width:1099px) and (min-height:501px){.workspace-actions .btn,.workspace-toolbar .find-machine,.quick-machine{min-height:44px;min-width:44px}.workspace-title>a{display:inline-flex;align-items:center;justify-content:center;min-width:44px;min-height:44px;padding:0;box-sizing:border-box}}
</style>
