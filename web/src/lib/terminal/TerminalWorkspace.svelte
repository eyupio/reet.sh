<script lang="ts">
  import { listGafferOutcomes } from '../api'
  import { factoryMembers, type FactoryMember } from '../gafferOutcome'
  let factory=$state<Map<string,FactoryMember>>(new Map())
 import { structuredAgent } from './api'
 import type { ChatState } from '../chat'
 import type { ChatProgressState } from '../gafferProgress'
  import { nativeTerminalView, type NativeViewInfo } from './api'
  import { onMount, tick, untrack } from 'svelte'
  import {getProfile,getSyncState,saveProfile,type Machine} from '../api'
  import { titleFor, validateDir } from '../agentLaunch'
  import StartAgentDialog from '../StartAgentDialog.svelte'
  import AddProjectDialog from '../home/AddProjectDialog.svelte'
  import SessionSidebar from './SessionSidebar.svelte'
  import { onAttentionChanged } from '../attentionSignal'
  import CopyButton from '../CopyButton.svelte'
  import { createTerminal, folderProjectList, machineHealth, readiness, terminalMachines, terminalSessions, updateFolderProject, type FolderProject, type HealthResponse, type LaunchRequest, type TerminalMachine, type TerminalSession } from './api'
  import type { ProjectColour } from '../projectColour'
  import { projectRunning, sessionTabTitle } from '../home'
  import MachineHealthStrip from './MachineHealthStrip.svelte'
  import TerminalPane from './TerminalPane.svelte'
  import TerminalIcon from './TerminalIcon.svelte'
  import ChatPane from './ChatPane.svelte'
  import SessionScreen from '../sessions/SessionScreen.svelte'
  import SessionTabs from '../sessions/SessionTabs.svelte'
  import WorkspaceSkeleton from './WorkspaceSkeleton.svelte'
  import PanelResizer from '../shell/PanelResizer.svelte'
  import { canContinue, canContinueChat, canReattachChat, closeTab, continueChatLaunch, continueLaunch, openTab } from '../sessions'
  import { continuationFailureInPane, offersContinueAfterReattach, quietReattachRefusal, reattachTrust, recoveryInFlight, shouldAutoReattach } from './reattachState'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import HealthHistory from './HealthHistory.svelte'
  import { renameTerminal, endTerminal, terminalArchiveBody, archiveTerminal, confirmDiscardWorktree, terminalClosed } from './api'
  import { TerminalError, reattachChat, validTerminalTitle } from './api'
  import WorkspaceDialog from './WorkspaceDialog.svelte'
  import TerminalCommandReference from './TerminalCommandReference.svelte'
  import { pageViewport, browserViewport, keyboardLikelyOpen, PHONE_SCREEN } from './sizing'
  import { chromeFocus } from '../shell/chrome.svelte'
  import { ALWAYS_DOCKED, MINI_WIDTH, PANEL_MIN, SNAP_TO_MINI, keepOpen, PANEL_DEFAULT, changeMode, chevronNext, clampPanelWidth, defaultLayout, defaultPanelWidth, isPhone, layoutRecord, panelDocks, panelMode, readLayout, screenShortSide, widestPanel, type PanelMode } from './panelLayout'

  let {userId=0,ontitle}:{userId?:number;ontitle?:(title:string)=>void}=$props()
  // svelte-ignore state_referenced_locally
  const preferenceKey=`reet-terminal-layout:${userId}`
  // The saved layout is read before the first paint, so the waiting skeleton and the real workspace agree on every width.
  let layoutLoaded=true
  // A session's menu on the Sessions page opens it in theatre view (?theatre=1); the address then forgets it.
  let theatre=$state(new URLSearchParams(location.search).get('theatre')==='1')
  let terminalPane=$state<{disconnect:()=>void}>()
  let sessionPanel=$state<{openFind?:()=>void}>()
  // The sessions panel (see panelLayout.ts): full, the 64 px mini rail, or closed. Full is a docked column beside the
  // terminal from 600 px (below 960 px while Keep open is on, by default from 768 px); on a phone, or a tablet whose person turned
  // Keep open off, it is a drawer over the terminal. A phone is told by its screen's shorter side, not the window's height.
  const readScreen=()=>screenShortSide(window.screen,innerWidth,innerHeight)
  let viewportWidth=$state(innerWidth),screenSide=$state(readScreen()),bodyWidth=$state(0),resizing=$state(false),contextResizing=$state(false)
  // The right-hand session details panel, toggled from the same toolbar row as the sessions panel.
  let contextOpen=$state(false),contextButton=$state<HTMLButtonElement>(),sessionScreen=$state<{toggleContext?:()=>void}>()
  const savedLayout=(()=>{try{return readLayout(localStorage.getItem(preferenceKey))}catch{return readLayout(null)}})()
  const initialMode=panelMode(innerWidth,savedLayout.mode,savedLayout.pinned,isPhone(innerWidth,readScreen())),initialWidth=savedLayout.width??defaultPanelWidth(innerWidth)
  // The waiting skeleton reads the same record, so the panel never changes width as the real workspace takes over.
  try{localStorage.setItem('reet.workspace.panel',JSON.stringify(layoutRecord(initialMode,initialWidth,savedLayout.pinned)))}catch{/* optional browser preference */}
  let mode=$state<PanelMode>(initialMode),pinned=$state<boolean|undefined>(savedLayout.pinned),sidebarWidth=$state(initialWidth)
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
    try{localStorage.setItem('reet.workspace.panel',record);const stored=localStorage.getItem(preferenceKey);if(stored===record||stored===null&&record===JSON.stringify(defaultLayout(viewportWidth)))return;localStorage.setItem(preferenceKey,record)}catch{/* optional browser preference */}
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
  function findSessions(){setMode('full');saveLayout();void tick().then(()=>sessionPanel?.openFind?.())}
  function togglePin(){pinned=!keptOpen;saveLayout()}
  function closeDrawer(){if(panelView==='drawer')mode='closed'}
  // The separator is the shared PanelResizer: it captures the pointer, so leaving the handle keeps the drag. The grid column
  // changes with it and the terminal refits locally once per frame; the host's size is held (holdSize) and sent when the drag ends.
  // A requested width below SNAP_TO_MINI is the mini rail; widening a key from there restores the full panel at PANEL_MIN.
  function setPanelWidth(width:number){
    if(width<SNAP_TO_MINI){setMode('mini');return}
    setMode('full');sidebarWidth=clampPanelWidth(width,panelRoom)
  }
  function commitPanelWidth(width:number){setPanelWidth(width);saveLayout()}
  function setPanelResizing(active:boolean){resizing=active}
  let machines = $state<TerminalMachine[]>([]), sessions = $state<TerminalSession[]>([])
  // The folder projects, read with the rest of the workspace inventory, for the panel's Projects list and the tab title.
  let projects = $state<FolderProject[] | undefined>()
  let selectedAgent = $state(new URLSearchParams(location.search).get('agent') || '')
  let selectedSession = $state(new URLSearchParams(location.search).get('session') || '')
  let sideSession = $state<TerminalSession | null>(null)
  // The open chat's word on whether it is busy, for the header's TUI switch; kept with the session it is about. Undefined
  // until its stream has replayed, so the header keeps to the row's word meanwhile.
  let insightChat = $state<{ id: string; state: ChatState; progress: ChatProgressState } | null>(null)
  let chatBusy = $state<{ id: string; busy: boolean | undefined } | null>(null)
  let health = $state<HealthResponse>({ serverNow:Date.now(),machines:[] })
  let receivedAt = performance.now(), elapsed = $state(0)
  let loading = $state(true), busy = $state(false), error = $state(''), title = $state('')
  type Creation = { requestId: string; title: string; dir: string; projectId?: string; error: string }
  let creations = $state<Record<string, Creation>>({})
  // A continuation whose reply was lost keeps its request id and launch, so Continue again replays the same operation.
  type Continuation = { requestId: string; title: string; launch: LaunchRequest }
  let continuations = $state<Record<string, Continuation>>({})
  // What the chat pane says about a Continue with history in flight for a saved chat, or why the last one failed.
  type PaneOutcome = { phase: 'pending' | 'failed'; reason?: string; code?: string; detail?: string }
  let continuing = $state<Record<string, PaneOutcome>>({})
  // An in-place reconnect of a lost chat, per session: in flight, or why the last attempt failed. No entry is idle.
  let reattach = $state<Record<string, PaneOutcome>>({})
  // The chats this page load already reconnected by itself, so a failure is never retried in a loop: only the person retries.
  const autoTried = new Set<string>()
  // Chats the server refused to reconnect because the person ended them (rows stored exited before ended chats were
  // recorded as ended): from then on they are offered Continue with history, like any ended chat.
  let personEnded = $state<Record<string, true>>({})
  /** Whether a chat is offered a reconnect in place: the machine can reattach it and the server has not said the person ended it. */
  function reconnectable(target: TerminalSession): boolean {
    return !personEnded[target.id] && canReattachChat(target, machines.find(item => item.agentId === target.agentId))
  }
  /** Whether the explicit Continue with history is offered for a chat: where it cannot reconnect in place, or after a
   *  folder-trust refusal of the reconnect or one that says the person ended the chat (ruling R10). */
  function continueOffered(target: TerminalSession): boolean {
    return !reconnectable(target) || offersContinueAfterReattach(reattach[target.id])
  }
  const reconnecting = $derived(new Set(Object.entries(reattach).filter(([, item]) => item.phase === 'pending').map(([id]) => id)))
  const pendingCreation = $derived(creations[selectedAgent])
  // Draft settings do not switch the workspace behind the dialog.
  let terminalAgent=$state(''),terminalDir=$state(''),terminalProjectId=$state('')
  // A shell started without a name is called what the controller would call it: Terminal, and its folder when it has one.
  const autoName=$derived(titleFor('',terminalDir.trim()))
  const createMachine=$derived(machines.find(item=>item.agentId===terminalAgent))
  const formCreation=$derived(creations[terminalAgent])
  const requestId=$derived(formCreation?.requestId || '')
  const createActive=$derived(sessions.filter(item=>item.agentId===terminalAgent&&['creating','running','end_requested'].includes(item.state)).length)
  const dirError=$derived(terminalDir ? validateDir(terminalDir) : null)
  let accessDenied=$state(false), machineSearchInput=$state<HTMLInputElement>()
  let creationRequest:AbortController|undefined
  let statusError = $state(''), machineSearch = $state(''), machineSearchOpen = $state(false)
  const matchingMachines = $derived(machines.filter(item => `${item.host} ${item.capabilities?.osUser || ''}`.toLowerCase().includes(machineSearch.toLowerCase())))
  const rememberedSessions = new Map<string, string>()
  const headerHeight = () => document.querySelector('.site-header')?.getBoundingClientRect().height ?? 56
  let workspace = $state<HTMLElement>(), workspaceHeight = $state(Math.max(120, innerHeight - headerHeight() - (innerWidth <= 700 ? 8 : 16)))
  // Phone keyboard open while typing in the terminal: this toolbar steps aside, and the shell's chrome with it (keyboardChrome
  // with the 'session' focus, read by AppShell through chromeFocus).
  let keyboard=$state(false)
  // ?viewport-debug=1 shows the measured heights live, for checking a real phone with its keyboard open. Nothing is stored or sent.
  const viewportDebug=new URLSearchParams(location.search).get('viewport-debug')==='1'
  let debugReading=$state('')
  $effect(()=>()=>{chromeFocus.session=false})
  let detailsOpen = $state(false)
  let healthIssue=$state(''),showCreate=$state(false)
  // New session asks what to start before showing either form: an agent session, or a plain shell. A project's + opens the
  // same choice with that project's machine and folder already in hand, so either answer belongs to the project.
  let newSessionOpen=$state(false),newSessionProject=$state<FolderProject|null>(null),newSessionOpener=$state<HTMLElement|null>(null)
  // The Start an agent dialog, with the button that opened it, and the folders the machines have reported (read when it opens).
  let agentStart=$state<{agent:string;opener:HTMLElement|null;projectId?:string}|null>(null),agentFolders=$state<Machine[]|undefined>(),folderRead=0
  // Add a project from the session panel: the same dialog as Home, without leaving the terminal.
  let addingProject=$state<{opener:HTMLElement|null}|null>(null)
  let action = $state<'rename'|'end'|'archive'|null>(null), actionSession = $state(''), rename = $state(''), actionTitle = $state(''), actionState = $state<TerminalSession['state']>('ended'), actionBusy = $state(false)
  let removeOnClose=$state(false),removeDefault=$state(false),defaultOn=$state(false),defaultEdited=$state(false),preferenceError=$state('')
  const endKeys = new Map<string,{key:string;remove:boolean}>()
  let refreshing = false, refreshAgain = false, disposed = false, inventoryVersion = 0
  let refreshRequest: AbortController | undefined
  const machine = $derived(machines.find(item=>item.agentId===selectedAgent))
  const ownSessions = $derived(sessions.filter(item=>item.agentId===selectedAgent))
  const activeSessions = $derived(ownSessions.filter(item=>['creating','running','end_requested'].includes(item.state)))
  const accountActive = $derived(sessions.filter(item=>['creating','running','end_requested'].includes(item.state)).length)
  const createAtQuota=$derived(createActive>=8||accountActive>=32)
  const session = $derived(ownSessions.find(item=>item.id===selectedSession))
  let presentations=$state<Record<string,'chat'|'terminal'>>({}), terminalMounted=$state<Record<string,boolean>>({})
  let nativeInfo=$state<{key:string;pending:boolean;info:NativeViewInfo}>({key:'',pending:false,info:{available:false}})
  const nativeKey=$derived(session?.structured ? JSON.stringify([session.id,session.state,session.acpSessionId,machine?.capabilities?.nativeView,machine?.ready]) : '')
  const presentation=$derived(session?.structured ? presentations[session.id] ?? 'chat' : 'terminal')
  $effect(()=>{
    const key=nativeKey
    if(!key){nativeInfo={key,pending:false,info:{available:false}};return}
    const [id,state,,supported,ready]=JSON.parse(key)
    if(state!=='running'||!supported||!ready){nativeInfo={key,pending:false,info:{available:false,reason:'This running agent has no shared native terminal. Update reet on the machine and start a supported OpenCode chat.'}};return}
    const request=new AbortController()
    nativeInfo={key,pending:true,info:{available:false}}
    void nativeTerminalView(id,request.signal).then(info=>{if(!request.signal.aborted)nativeInfo={key,pending:false,info}}).catch(()=>{if(!request.signal.aborted)nativeInfo={key,pending:false,info:{available:false,reason:'Could not check the native terminal. Reconnect the machine or reload this session.'}}})
    return ()=>request.abort()
  })
  function choosePresentation(next:'chat'|'terminal'){
    if(!session?.structured || next==='terminal' && !nativeInfo.info.available)return
    presentations={...presentations,[session.id]:next}
    if(next==='terminal')terminalMounted={...terminalMounted,[session.id]:true}
  }

  const machineStatus = $derived(health.machines.find(item=>item.agentId===selectedAgent))
  const setup = $derived(readiness(machine))
  const serverNow = $derived(health.serverNow+elapsed)
  const runningByProject = $derived(projectRunning(sessions))
  const currentProjectId = $derived(session?.projectId ?? '')
  // The tab names the work: the session, then its project (or its machine when it has none), then reet.
  const tabTitle = $derived(sessionTabTitle(session, projects?.find(p => p.id === session?.projectId)?.name, machine?.host))
  // The tab names the work, not just the page: two reet tabs are told apart by the session they hold. Coordinating it
  // with the document title owner keeps session names aligned with global attention counts.
  $effect(() => {
    if (ontitle) {
      ontitle(tabTitle)
      return () => { ontitle('') }
    }
    document.title = tabTitle
  })
  $effect(()=>{if(machineSearchOpen)machineSearchInput?.focus()})
  // A saved default that arrives after the dialog opened shows in the toggle until the user changes it themselves.
  $effect(()=>{if(!defaultEdited)defaultOn=removeDefault})
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

  // Only locally requested ends dismiss their view after confirmed closure.
  // Offline/uncertain ends stay visible until a successful listing confirms closure.
  const dismissAfterEnd = new Set<string>()
  function dismissConfirmedEnds() {
    for (const id of dismissAfterEnd) {
      const ended = sessions.find(item => item.id === id)
      if (ended && !['ended', 'exited'].includes(ended.state)) continue
      dismissAfterEnd.delete(id)
      const wasSelected = selectedSession === id
      const next = closeTab(openTabs, id, selectedSession)
      openTabs = next.tabs
      for (const [agent, remembered] of rememberedSessions) if (remembered === id) rememberedSessions.delete(agent)
      if (!wasSelected) continue
      const eligible = (item: TerminalSession) => next.tabs.includes(item.id) && !terminalClosed(item.state)
      const target = sessions.find(item => item.id === next.next && eligible(item)) || sessions.find(eligible)
      // A replacement must not remember the outgoing, dismissed session again.
      if (target) { selectedSession = ''; chooseSession(target) }
      else { selectedSession = ''; remember(); location.assign('/sessions?view=list') }
    }
  }

  function remember() { const url=new URL(location.href);if(!selectedAgent&&!selectedSession&&!url.searchParams.has('view'))return;url.searchParams.set('view','terminal');url.searchParams.delete('theatre');if(selectedAgent)url.searchParams.set('agent',selectedAgent);else url.searchParams.delete('agent');if(selectedSession)url.searchParams.set('session',selectedSession);else url.searchParams.delete('session');history.replaceState(null,'',url) }
  function chooseMachine(id: string) {
    if(selectedAgent && selectedSession)rememberedSessions.set(selectedAgent,selectedSession)
    selectedAgent=id
    const previous=rememberedSessions.get(id)
    selectedSession=sessions.find(item=>item.agentId===id&&item.id===previous)?.id || sessions.find(item=>item.agentId===id&&item.state==='running')?.id || ''
    title=creations[id]?.title || '';error='';machineSearchOpen=false;remember()
  }
  onMount(()=>{
    const showSession=(event:Event)=>{
      if(!(event instanceof CustomEvent)||typeof event.detail!=='string')return
      const item=sessions.find(candidate=>candidate.id===event.detail&&candidate.structured)
      if(!item)return
      event.preventDefault();chooseSession(item)
    }
    window.addEventListener('reet:show-session',showSession)
    return()=>window.removeEventListener('reet:show-session',showSession)
  })
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
      const selectionBeforeRefresh=selectedSession
      const removedAfterClose=sessions.find(item=>item.id===selectedSession)?.removeHistory&&!nextSessions.some(item=>item.id===selectedSession)
      machines=nextMachines;sessions=nextSessions;listingLoaded=true;healthIssue=nextHealth.error
      // Which sessions belong to a Factory pipeline. A helper only: without it the lists simply carry no marker.
      void listGafferOutcomes().then(found=>{if(!disposed)factory=factoryMembers(found)}).catch(()=>{})
      if(nextProjects.projects)projects=nextProjects.projects
      dismissConfirmedEnds()
      if(removedAfterClose&&selectedSession===selectionBeforeRefresh){selectedSession='';remember()}
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
  function selectTerminalMachine(agent:string){
    terminalAgent=agent
    const pending=creations[agent]
    title=pending?.title||'';terminalDir=pending?.dir||'';terminalProjectId=pending?.projectId||'';error=''
  }
  function openTerminalStart(agent=selectedAgent){
    if(accessDenied||!machines.length||busy)return
    selectTerminalMachine(agent);showCreate=true;closeDrawer()
  }
  function quickCreate(agent:string){openTerminalStart(agent)}
  function newSession(opener?:HTMLElement){
    if(!machines.length||busy||accessDenied)return
    // An unconfirmed creation keeps its own retry: New session opens that, never a second choice.
    if(pendingCreation){openTerminalStart();return}
    newSessionProject=null;newSessionOpener=opener??null
    newSessionOpen=true
  }
  function newSessionInProject(project:FolderProject,opener:HTMLElement){
    if(!machines.length||busy||accessDenied)return
    newSessionProject=project;newSessionOpener=opener;newSessionOpen=true
  }
  async function chooseNewSession(kind:'agent'|'terminal'){
    const project=newSessionProject,opener=newSessionOpener
    newSessionOpen=false;newSessionProject=null;newSessionOpener=null
    await tick()
    if(kind==='agent'){
      if(project){openAgentStart(project.host,opener);agentStart={agent:project.host,opener,projectId:project.id};return}
      openAgentStart(selectedAgent,opener);return
    }
    if(project){selectTerminalMachine(project.host);terminalDir=project.dir||'';terminalProjectId=project.id;showCreate=true;closeDrawer();return}
    openTerminalStart()
  }
  function submitCreate(){void create()}
  async function create() {
    if(busy||accessDenied||!createMachine?.ready||createMachine.serverEnabled===false||!validTerminalTitle(title.trim()||autoName)||dirError||createAtQuota&&!requestId)return
    busy=true;error=''
    // One immutable retry operation per machine, including its original title. A project launch keeps its projectId only
    // while the folder is the project's; editing the folder clears it, and a validation refusal (422) puts the form back so
    // the person can correct the folder instead of retrying an invalid request forever.
    const agent=terminalAgent
    const originAgent=selectedAgent,originSession=selectedSession
    const operation=creations[agent] || {requestId:crypto.randomUUID(),title:title.trim()||autoName,dir:terminalDir,projectId:terminalProjectId||undefined,error:''}
    creations={...creations,[agent]:{...operation,error:''}}
    const request=new AbortController();creationRequest=request
    const timeout=setTimeout(()=>request.abort(),15000)
    try {
      const created=await createTerminal(agent,operation.requestId,operation.title,100,30,request.signal,(operation.dir||operation.projectId)?{...(operation.dir?{dir:operation.dir}:{}),...(operation.projectId?{projectId:operation.projectId}:{})}:undefined)
      if(disposed)return
      inventoryVersion++
      // A retry can resolve to an old closed record; let the filtered history list
      // determine visibility instead of reintroducing an archived session locally.
      if(!terminalClosed(created.state))sessions=[created,...sessions.filter(item=>item.id!==created.id)]
      if(creations[agent]?.requestId===operation.requestId){const next={...creations};delete next[agent];creations=next}
      if(selectedAgent===originAgent&&selectedSession===originSession){showCreate=false;chooseSession(created)}
      await refresh()
    } catch(e){
      if(disposed)return
      if(e instanceof TerminalError&&[400,422].includes(e.status)){const next={...creations};delete next[agent];creations=next;if(e.status===422)terminalProjectId='';if(terminalAgent===agent)error=e.message}
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
    agentStart={agent:project.host,opener,projectId:project.id}
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
  // The machine switched a session between Gaffer and the OpenCode TUI (204, also when it was already there): the
  // row follows at once, so the pane swaps, and the listing is read again (an older read in flight is discarded).
  function modeSwitched(id:string,to:'terminal'|'chat'){inventoryVersion++;sessions=sessions.map(item=>item.id===id?{...item,structured:to==='chat'}:item);void refresh()}
  // A started agent lands exactly as a new shell does: it joins the list and the workspace switches to it.
  function agentStarted(item:TerminalSession){inventoryVersion++;if(!terminalClosed(item.state))sessions=[item,...sessions.filter(other=>other.id!==item.id)];showCreate=false;chooseSession(item);void refresh()}
  // Continue a closed session as a new terminal in the same folder, asking the agent to resume its last conversation there.
  // The operation is kept per session until its outcome is known: an uncertain failure retries the same request id.
  // A plain shell in the folder an agent session is using. It is a separate
  // session (and row), so it can sit beside the agent rather than replace it.
  async function openPlainTerminal(target=session){
    if(!target||actionBusy)return
    actionBusy=true;error=''
    try{
      const requestId=crypto.randomUUID()
      const folder=target.dir||''
      const name=folder.split('/').filter(Boolean).pop()||target.title
      const created=await createTerminal(target.agentId,requestId,`Terminal · ${name}`,100,30,AbortSignal.timeout(15000),folder?{dir:folder}:undefined)
      if(disposed)return
      inventoryVersion++
      if(!terminalClosed(created.state))sessions=[created,...sessions.filter(item=>item.id!==created.id)]
      chooseSession(created)
      await refresh()
    }catch(e){
      if(disposed)return
      error=e instanceof TerminalError?(e.message||'That terminal could not be opened.'):'That terminal could not be opened.'
    }finally{actionBusy=false}
  }
  async function openSideTerminal(target=session){
    if(!target||actionBusy||phone||sideSession)return
    actionBusy=true;error=''
    try{
      const requestId=crypto.randomUUID()
      const folder=target.dir||''
      const name=folder.split('/').filter(Boolean).pop()||target.title
      const created=await createTerminal(target.agentId,requestId,`Terminal · ${name}`,100,30,AbortSignal.timeout(15000),folder?{dir:folder}:undefined)
      if(disposed)return
      inventoryVersion++
      if(!terminalClosed(created.state))sessions=[created,...sessions.filter(item=>item.id!==created.id)]
      sideSession=created
      await refresh()
    }catch(e){
      if(disposed)return
      error=e instanceof TerminalError?(e.message||'That terminal could not be opened.'):'That terminal could not be opened.'
    }finally{actionBusy=false}
  }
  async function closeSideTerminal(){
    const target=sideSession
    sideSession=null
    if(!target||target.state!=='running')return
    try{await endTerminal(target.id,crypto.randomUUID(),true);await refresh()}
    catch{/* the shell stays listed; the panel is only a view */}
  }
  async function continueSession(target=session){
    if(!target||actionBusy||recoveryInFlight(target.id,reattach,continuing))return
    const existing=continuations[target.id]
    const launch=existing?.launch??(target.structured?continueChatLaunch(target):continueLaunch(target,projects??[]))
    if(!launch)return
    const operation=existing??{requestId:crypto.randomUUID(),title:target.title,launch}
    continuations={...continuations,[target.id]:operation}
    actionBusy=true;error=''
    // A chat's pane says the new session is starting, and keeps saying why if it could not.
    const shown=target.structured
    if(shown)continuing={...continuing,[target.id]:{phase:'pending'}}
    const settle=()=>{const next={...continuations};delete next[target.id];continuations=next}
    const quiet=()=>{if(!shown)return;const next={...continuing};delete next[target.id];continuing=next}
    try{
      const created=await createTerminal(target.agentId,operation.requestId,operation.title,100,30,AbortSignal.timeout(operation.launch.structured?60000:15000),operation.launch)
      settle()
      if(disposed)return
      quiet()
      inventoryVersion++
      if(!terminalClosed(created.state))sessions=[created,...sessions.filter(item=>item.id!==created.id)]
      chooseSession(created)
      await refresh()
    }catch(e){
      if(disposed)return
      // A stated refusal of this request started nothing; an uncertain failure may have, so the operation is kept.
      const definite=e instanceof TerminalError&&[400,401,403,409,422].includes(e.status)
      if(definite)settle()
      const reason=e instanceof TerminalError?(e.message||'The session could not be continued.'):'The continuation may have started. Continue again to check the same operation.'
      if(shown)continuing={...continuing,[target.id]:{phase:'failed',reason}}
      // Where the chat pane will not show it (another session on screen, or the pane offers Reconnect), the workspace says it.
      const offersReconnect=(reconnectable(target)||!!reattach[target.id])&&!offersContinueAfterReattach(reattach[target.id])
      if(!continuationFailureInPane({structured:target.structured===true,selected:selectedSession===target.id,offersReconnect}))error=reason
    }
    finally{actionBusy=false}
  }
  // Reconnect a lost chat in place: the same row, URL and transcript carry on. One attempt per chat at a time (the
  // server also joins a second caller to the one in flight); it never navigates and never starts another session.
  // Trust of the folder (it has OpenCode plugins) is sent only from the person's explicit Trust this folder and
  // reconnect: never by the automatic attempt, and never remembered for a later one.
  const reattachAttempts = new Map<string, symbol>()
  async function reattachSession(target: TerminalSession, opts: { auto?: boolean; trust?: boolean } = {}) {
    const auto = opts.auto === true
    const trust = reattachTrust({ auto, trust: opts.trust })
    if (recoveryInFlight(target.id, reattach, continuing)) return
    // A chat the person reconnected by hand is not also tried by itself later in this page load.
    if (!auto) autoTried.add(target.id)
    const attempt = Symbol(target.id)
    reattachAttempts.set(target.id, attempt)
    reattach = { ...reattach, [target.id]: { phase: 'pending' } }
    const current = () => !disposed && reattachAttempts.get(target.id) === attempt
    try {
      await reattachChat(target.id, { trust }, AbortSignal.timeout(80000))
      if (!current()) return
      // The controller recorded the row as running before it answered: the pane follows at once, and the listing is read again.
      inventoryVersion++
      sessions = sessions.map(item => item.id === target.id ? { ...item, state: 'running' } : item)
      await refresh()
      if (!current()) return
      reattachAttempts.delete(target.id)
      const next = { ...reattach }; delete next[target.id]; reattach = next
    } catch (e) {
      if (!current()) return
      reattachAttempts.delete(target.id)
      const code = e instanceof TerminalError ? e.code : undefined
      const quiet = quietReattachRefusal(code, auto)
      if (quiet) {
        // Only an automatic attempt is quiet: an ended chat falls back to the ended note with Continue with history; a chat
        // found still running is read again from the listing. A reconnect the person asked for shows its refusal.
        if (quiet === 'ended') personEnded = { ...personEnded, [target.id]: true }
        const next = { ...reattach }; delete next[target.id]; reattach = next
        if (quiet === 'drop') void refresh()
        return
      }
      const detail = e instanceof TerminalError ? e.detail : undefined
      const reason = e instanceof TerminalError ? e.message
        : e instanceof DOMException && e.name === 'TimeoutError' ? 'Reconnecting took too long. Try again.'
        : 'reet could not be reached to reconnect this chat. Try again.'
      reattach = { ...reattach, [target.id]: { phase: 'failed', reason, ...(code ? { code } : {}), ...(detail ? { detail } : {}) } }
    }
  }
  // The open chat, once lost on a machine that can reattach it, reconnects by itself: once per chat per page load, on
  // view or when its machine comes back. After a failure only the person retries.
  $effect(() => {
    const target = session
    if (!target) return
    const canReattach = reconnectable(target)
    const phase = reattach[target.id]?.phase ?? 'idle'
    // Not while a Continue with history of this chat is in flight; the effect runs again once it settles.
    const continuingNow = continuing[target.id]?.phase === 'pending'
    untrack(() => {
      // A chat that is running again (reconnected from another tab, say) drops a failure left from before.
      if (phase === 'failed' && !terminalClosed(target.state)) { const next = { ...reattach }; delete next[target.id]; reattach = next; return }
      if (continuingNow) return
      if (!shouldAutoReattach({ canReattach, phase, tried: autoTried, sessionId: target.id })) return
      autoTried.add(target.id)
      void reattachSession(target, { auto: true })
    })
  })
  function openAction(kind:'rename'|'end'|'archive',target=session) { if(!target)return;actionSession=target.id;actionTitle=target.title;actionState=target.state;rename=target.title;removeOnClose=endKeys.get(target.id)?.remove??(target.state==='end_requested'?(target.removeHistory??false):removeDefault);defaultOn=removeDefault;defaultEdited=false;preferenceError='';action=kind }
  async function performAction() {
    const kind=action,id=actionSession;action=null;if(!kind||actionBusy)return
    actionBusy=true;error=''
    try {
      if(kind==='rename'){if(!validTerminalTitle(rename.trim())){error='Use a session name of 1–80 characters without control characters.';return}await renameTerminal(id,rename.trim())}
      else if(kind==='archive'){
        await archiveTerminal(id,confirmDiscardWorktree)
        if(disposed)return
        sessions=sessions.filter(item=>item.id!==id)
        for(const [agent,selected] of rememberedSessions)if(selected===id)rememberedSessions.delete(agent)
        if(selectedSession===id){selectedSession='';remember()}
      }
      else {let operation=endKeys.get(id);if(!operation){operation={key:crypto.randomUUID(),remove:removeOnClose};endKeys.set(id,operation)}dismissAfterEnd.add(id);await endTerminal(id,operation.key,operation.remove);if(defaultEdited&&defaultOn!==removeDefault){try{await saveProfile({terminalRemoveOnClose:defaultOn});removeDefault=defaultOn}catch{preferenceError='Session stop requested, but your default could not be saved. Change it in Account → Profile.'}}}
      inventoryVersion++
      await refresh()
    } catch(e){
      if(kind==='end'&&e instanceof TerminalError&&e.status>=400&&e.status<500&&![408,429].includes(e.status))dismissAfterEnd.delete(id)
      if(!disposed)error=e instanceof Error?e.message:'Action could not be confirmed. Retry the same action.'
    }
    finally{actionBusy=false}
  }
  $effect(() => {
    // Loading initially renders a skeleton: install measurement only once the
    // real workspace is bound, and tear it down if that surface is removed.
    if (!workspace) return
    // The layout was read above, before the first paint.
    // Only this workspace asks Android browsers to resize the page for the keyboard; other pages keep the default.
    const viewportMeta=document.querySelector<HTMLMetaElement>('meta[name="viewport"]'),viewportContent=viewportMeta?.getAttribute('content')??null
    if(viewportMeta&&viewportContent!==null&&!/interactive-widget/.test(viewportContent))viewportMeta.setAttribute('content',`${viewportContent}, interactive-widget=resizes-content`)
    let frame=0
    const coarse=matchMedia('(pointer:coarse)'),narrow=matchMedia(PHONE_SCREEN)
    const editing=()=>{const focus=document.activeElement;return focus instanceof HTMLElement&&!!focus.closest('.pane,.chat-pane')&&focus.matches('textarea,input,[contenteditable]:not([contenteditable="false"])')}
    const measure=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(()=>{
      if(!workspace)return
      const usable=pageViewport.read(browserViewport())
      const typing=editing()
      chromeFocus.session=typing
      keyboard=keyboardLikelyOpen(usable.shrink,{coarse:coarse.matches,narrow:narrow.matches,editing:typing,scale:window.visualViewport?.scale??1})
      const reserved=document.querySelector<HTMLElement>('[data-gaffer-space]')?.getBoundingClientRect().height??0
      workspaceHeight=Math.max(120,usable.height-Math.max(0,workspace.getBoundingClientRect().top-usable.top)-reserved-8)
      if(viewportDebug){const visual=window.visualViewport;debugReading=`visualViewport.height ${visual?Math.round(visual.height):'none'} · offsetTop ${visual?Math.round(visual.offsetTop):'none'}\ninnerHeight ${innerHeight} · screen.height ${screen.height}\nused height ${Math.round(usable.height)} · keyboard ${keyboard?'open':'closed'}\ninnerWidth ${innerWidth} · visualViewport.width ${visual?Math.round(visual.width):'none'} · scale ${visual?Math.round(visual.scale*100)/100:'none'} · offsetLeft ${visual?Math.round(visual.offsetLeft):'none'}\nclientWidth ${document.documentElement.clientWidth} · scrollWidth ${document.documentElement.scrollWidth} · screen.width ${screen.width} · dpr ${devicePixelRatio}\nmeta ${viewportMeta?.getAttribute('content')??'none'}`}
    })}
    const observer=new ResizeObserver(measure),header=document.querySelector('.site-header')
    if(header)observer.observe(header)
    // The shell hides and shows the site's chrome (keyboardChrome) after this measure has set the session focus: measure again
    // whenever it does, so the height is never one taken while the top bar was aside.
    const chrome=new MutationObserver(measure)
    chrome.observe(document.documentElement,{attributes:true,attributeFilter:['data-chrome-aside','data-bottom-tabs','data-session-focus','data-gaffer-anchor','data-gaffer-editing']})
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
    // An answer made anywhere reloads this list now, through the same fenced refresh as the poll.
    const offAttention=onAttentionChanged(()=>{void refresh(abort.signal)})
    return()=>{offAttention();disposed=true;abort.abort();refreshRequest?.abort();creationRequest?.abort();clearInterval(poll);clearInterval(clock);document.removeEventListener('visibilitychange',wake);window.removeEventListener('online',wake)}
  })
</script>

{#snippet endedView(heading:string,lede:string)}
        <div class="empty ended" role="status">
          <div class="ended-glow" aria-hidden="true"></div>
          <span class="empty-icon ended-icon" aria-hidden="true">›_</span>
          <h2>{heading}</h2>
          <p>{lede}</p>
          {@render startActions()}
          {#if activeSessions.length}
            <div class="ended-resume" aria-label="Running sessions">
              <h3>Or jump back into a running session</h3>
              <ul>{#each activeSessions.slice(0,4) as item (item.id)}<li><button type="button" onclick={()=>chooseSession(item)}><span class="ended-dot" aria-hidden="true"></span><span class="ended-name">{item.title}</span></button></li>{/each}</ul>
            </div>
          {/if}
        </div>
{/snippet}
{#snippet startActions()}
  <div class="ended-actions">
    <button type="button" class="btn primary ended-cta" disabled={!machines.length || accountActive>=32} onclick={event=>openAgentStart(selectedAgent,event.currentTarget)}>✦ Start a new agent session</button>
    <button type="button" class="btn ended-cta" disabled={!machines.length || busy} onclick={()=>openTerminalStart()}>✦ Start a new Terminal session</button>
  </div>
{/snippet}
{#snippet createForm()}
  <form class="create-form" onsubmit={event => { event.preventDefault(); submitCreate() }}>
    {#if formCreation?.error}<p class="creation-error" role="alert">{formCreation.error} The creation is unconfirmed. Retry checks the original operation.</p>{/if}
    <label>Machine<select aria-label="Machine" value={terminalAgent} disabled={busy} required onchange={event=>selectTerminalMachine(event.currentTarget.value)}>
      <option value="" disabled>Choose a machine…</option>
      {#each machines as item (item.agentId)}<option value={item.agentId}>{item.host}{item.ready?'':' · Unavailable'}</option>{/each}
    </select></label>
    <label>Folder<input bind:value={terminalDir} disabled={busy || !!requestId} oninput={()=>{terminalProjectId=''}} placeholder="Home folder (default)" aria-invalid={!!dirError} aria-describedby="terminal-folder-help" /></label>
    <p id="terminal-folder-help">{dirError || 'Leave empty for the machine’s home folder, or enter a full path starting with /.'}</p>
    <label>New session name<input bind:value={title} disabled={busy || !!requestId} maxlength="80" placeholder={autoName} aria-describedby="terminal-name-help" /></label>
    <p id="terminal-name-help">Optional. Left empty, the session is called {autoName}.</p>
    {#if !createMachine?.ready || createMachine.serverEnabled===false}<p class="create-reason" role="status">{readiness(createMachine) || 'Choose a machine before creating a shell.'}</p>{/if}
    <p>{createActive} / 8 active on this machine · {accountActive} / 32 across your account{createAtQuota ? ' · Session limit reached' : ''}. Existing shells keep running when you switch or disconnect.</p>
    <button class="btn primary" disabled={busy || !validTerminalTitle(title.trim()||autoName) || !!dirError || !createMachine?.ready || createMachine.serverEnabled===false || createAtQuota && !requestId} aria-busy={busy}>{busy ? 'Creating…' : requestId ? 'Retry creation' : 'New terminal'}</button>

  </form>
{/snippet}

{#snippet sessionDetailsControl()}
  <button class="btn details-toggle" bind:this={contextButton} aria-expanded={sideSession ? false : contextOpen} aria-controls="session-details-panel" aria-label="Session details panel"
    title={sideSession ? 'Close the terminal beside the session to use details' : contextOpen ? 'Hide session details' : 'Show session details'} disabled={!!sideSession}
    onclick={() => sessionScreen?.toggleContext?.()}><TerminalIcon name="panelRight"/><span>Details</span></button>
{/snippet}


{#if loading}
  <WorkspaceSkeleton label="Loading your sessions" context={!theatre && (!!selectedSession || !!selectedAgent)} />
{:else}
<section class="workspace" class:theatre class:keyboard class:resizing bind:this={workspace} style={`--workspace-height:${workspaceHeight}px;--sidebar-width:${sidebarWidth}px;--panel-width:${shown==='mini'?MINI_WIDTH:panelWidth}px`} aria-label="Remote terminal workspace">
  <header class="workspace-toolbar">
    <div class="workspace-title"><a href="/sessions" aria-label="Back to sessions">←</a>{#if !session}<h1>Sessions</h1>{/if}</div>
    {#if panelView==='closed'}<button class="btn quick-machine" aria-label={selectedAgent ? `New terminal on ${machine?.host || 'selected machine'}` : 'Choose a machine for a new terminal'} title={selectedAgent ? 'Start a terminal on this machine' : 'Choose a machine for a new terminal'} disabled={selectedAgent ? !canCreateFor(selectedAgent) : !machines.length} onclick={()=>openTerminalStart()}>+</button>{/if}
    <button class="btn find-machine" onclick={()=>{machineSearch='';machineSearchOpen=true}} aria-label="Find machine">Find</button>
    <!-- The session screen's tabs share this row, so the terminal keeps its height. -->
    {#if session && !loading && !accessDenied && !phone}<SessionTabs tabs={tabSessions} current={session.id} onselect={chooseSession} onclose={closeSessionTab} />{/if}
    <div class="workspace-actions"><button class="btn panel-toggle" aria-expanded={panelOpen} aria-controls="terminal-sessions" aria-label="Sessions panel" title={panelOpen?'Hide sessions panel':'Show sessions panel'} onclick={toggleSidebar}><TerminalIcon name="panel"/><span class="count" aria-hidden="true">{accountActive}</span></button><button class="btn primary new-button" disabled={!machines.length} onclick={()=>newSession()} aria-label="New session"><span class="wide-label">New session</span><span class="mobile-label">New</span></button><button class="btn" aria-label="Help & shortcuts" onclick={() => detailsOpen = true}><span class="wide-label">Help</span><span class="mobile-label">Help</span></button></div>
  </header>
  {#if preferenceError}<p class="workspace-notice bad" role="status">{preferenceError}</p>{/if}
  {#if statusError}<p class="workspace-notice bad" role="status">{statusError} <button onclick={()=>void refresh()}>Retry status</button></p>{/if}
  {#if pendingCreation && !showCreate && session}<p class="workspace-notice" role="status">A creation on this machine is unconfirmed. <button onclick={()=>openTerminalStart()}>Review creation</button></p>{/if}
  {#if error && !showCreate}<p class="workspace-notice bad" role="alert">{error} <button onclick={() => void refresh()}>Retry status</button></p>{/if}
  <div class="workspace-body" class:docked={shown==='docked'||shown==='mini'} bind:clientWidth={bodyWidth}>
    {#if shown!=='closed'}<SessionSidebar bind:this={sessionPanel} {factory} {machines} {sessions} {reconnecting} {projects} projectRunning={runningByProject} currentProject={currentProjectId} onprojectstart={startInProject} onprojectnew={newSessionInProject} onprojectadd={openAddProject} onprojectcolour={saveProjectColour} {serverNow} {selectedAgent} {selectedSession} layout={shown} collapseLabel={phone?'Close session navigation':'Shrink sessions to a mini rail'} pinnable={!phone&&viewportWidth<ALWAYS_DOCKED} pinned={keptOpen} onpin={togglePin} onexpand={expand} onfind={findSessions} onnewsession={newSession} preferenceKey={`${preferenceKey}:sessions`} onnew={quickCreate} cancreate={canCreateFor} onselect={chooseSession} onclose={chevron} onrefresh={()=>void refresh()} onagent={openAgentStart} onaction={openAction} ondisconnect={()=>terminalPane?.disconnect()} busy={actionBusy} />{/if}
    {#if shown==='docked'||shown==='mini'}
      <!-- A focusable window splitter: drag (pointer capture), Left/Right 16 px, Home the mini rail, End the widest, double-click 280 px. -->
      <PanelResizer value={shown==='mini'?MINI_WIDTH:panelWidth} min={MINI_WIDTH} max={panelMax} resetValue={PANEL_DEFAULT}
        label="Resize session navigation" controls="terminal-sessions" extraClass="panel-separator" mode="inline"
        position="grid-column: 2; grid-row: 1 / -1" minText="Mini rail"
        snap={{ value: MINI_WIDTH, below: SNAP_TO_MINI, restore: PANEL_MIN }}
        onchange={setPanelWidth} oncommit={commitPanelWidth} onresizing={setPanelResizing} />
    {/if}
    <div class="terminal-stage">
      {#if accessDenied}<div class="empty"><h2>Terminal access unavailable</h2><p>Your account no longer has terminal access. Sign in again or ask the owner to check your permissions. Existing shells were not ended.</p><button class="btn" onclick={()=>void refresh()}>Check access again</button></div>
      {:else if !machines.length}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>Connect your first machine</h2><p>Enroll a machine and enable terminal access locally to open a shell here.</p><a class="btn primary" href="/machines#add-machine">Connect a machine</a></div>
      {:else if session}
        <!-- The session screen hosts the live terminal pane, unchanged: tabs and header above it, the context panel beside it. -->
        <SessionScreen bind:this={sessionScreen} factory={factory.get(session.id)} {session} {sessions} {machines} health={health.machines} {projects} now={serverNow} {theatre} {keyboard} busy={actionBusy}
          view={presentation} onview={choosePresentation} nativeAvailable={nativeInfo.key === nativeKey && nativeInfo.info.available} nativePending={nativeInfo.pending} nativeReason={nativeInfo.info.reason} contextControls={sessionDetailsControl} contextKey={`${preferenceKey}:context`} contextId="session-details-panel" bind:contextOpen oncontextclosed={() => contextButton?.focus()} onresizing={active => contextResizing = active}
          onaction={kind => openAction(kind)} ontheatre={() => theatre = !theatre} onrefresh={() => void refresh()} onsignedout={() => { accessDenied = true }}
          insightChat={insightChat?.id === session.id ? insightChat.state : undefined}
          insightProgress={insightChat?.id === session.id ? insightChat.progress : undefined}
          chatBusy={session.structured && chatBusy?.id === session.id ? chatBusy.busy : undefined} onmode={modeSwitched}
          oncontinue={session.structured
            ? (continueOffered(session) && canContinueChat(session, machines.find(item => item.agentId === session.agentId)) ? () => void continueSession(session) : undefined)
            : (canContinue(session, machines.find(item => item.agentId === session.agentId), projects ?? []) ? () => void continueSession(session) : undefined)}
          onterminal={() => void openPlainTerminal(session)}
          onside={!phone ? () => void openSideTerminal(session) : undefined}
          showSide={!!sideSession}
          >
          {#snippet side()}
            {#if sideSession}
              {@const pane = sideSession}
              <div class="side-terminal">
                <div class="side-terminal-head"><strong>{pane.title}</strong><button type="button" onclick={() => void closeSideTerminal()}>Close</button></div>
                <TerminalPane {keyboard} session={pane} historySupported={false} uploadsSupported={false} uploadsOff host={machines.find(item => item.agentId === pane.agentId)?.host || 'Unavailable machine'} user={machine?.capabilities?.osUser || ''} actionDisabled={actionBusy} />
              </div>
            {/if}
          {/snippet}
          {#if setup || session.state === 'end_requested'}<div class="workspace-notice" role="status">{session.state === 'end_requested' ? 'End requested. Waiting for this machine to confirm; the shell still counts towards its limit.' : setup}<button onclick={() => detailsOpen = true}>View details</button></div>{/if}
          {#if terminalClosed(session.state) && reattach[session.id]?.phase !== 'pending'}<div class="workspace-notice ended-banner" role="status"><span>This session has ended. Its output stays below.</span><span class="ended-banner-actions">{#if !session.structured}<button type="button" disabled={!machines.length || accountActive>=32} onclick={event=>openAgentStart(session.agentId,event.currentTarget)}>Start a new agent session</button>{/if}<button type="button" disabled={!machines.length || busy} onclick={()=>openTerminalStart()}>New terminal</button></span></div>{/if}
          <!-- Keyed by the session and its mode, so a switch between Gaffer and the OpenCode TUI mounts the other pane. -->
          {#key session.id + ':' + (session.structured ? 'chat' : 'term')}
            {#if session.structured}
              {@const reattachable = reconnectable(session)}
              <div class="session-presentation" data-session-presentation="chat" hidden={presentation !== 'chat'} inert={presentation !== 'chat'}>
              <ChatPane {session} project={projects?.find(p => p.id === session.projectId)} liveControls={machine?.capabilities?.acpLiveControls??false} reattach={reattach[session.id]} onreattach={reattachable || reattach[session.id] ? () => void reattachSession(session, { auto: false }) : undefined} ontrustreattach={reattachable || reattach[session.id] ? () => void reattachSession(session, { auto: false, trust: true }) : undefined} continuation={continuing[session.id]} oncontinue={continueOffered(session) && canContinueChat(session,machine)?()=>void continueSession(session):undefined} continueOff={actionBusy||accountActive>=32||!machine?.ready||machine?.serverEnabled===false} onnewchat={opener=>openAgentStart(session.agentId,opener)} newChatOff={actionBusy||!machines.length||accountActive>=32} onstate={(state, progress) => { insightChat = { id: session.id, state, progress } }} onbusy={busy => { chatBusy = { id: session.id, busy } }} uploadsSupported={machine?.capabilities?.uploads??false} uploadsOff={machine?.capabilities?.uploadsOff??false} acpImages={structuredAgent(machine?.capabilities, session.tool)?.images??false} acpModel={structuredAgent(machine?.capabilities, session.tool)?.model??false} agentModes={structuredAgent(machine?.capabilities, session.tool)?.modes??[]} />
              </div>
              {#if terminalMounted[session.id]}
                <div class="session-presentation" data-session-presentation="terminal" hidden={presentation !== 'terminal'} inert={presentation !== 'terminal'}>
                  <TerminalPane nativeView visible={presentation === 'terminal'} {keyboard} headerTheatre historySupported={false} uploadsSupported={false} holdSize={resizing || contextResizing} sessionsVisible={shown!=='closed'} onsessions={paneSessions} bind:theatre {session} host={machine?.host || 'Unavailable machine'} user={machine?.capabilities?.osUser || ''} />
                </div>
              {/if}

            {:else}
              <TerminalPane {keyboard} headerTheatre footnote={phone ? '' : `Approvals: ${session.yolo ? 'YOLO' : 'Ask me'}`} holdSize={resizing || contextResizing} sessionsVisible={shown!=='closed'} onsessions={paneSessions} bind:theatre bind:this={terminalPane} {session} historySupported={machine?.capabilities?.commandHistory??false} uploadsSupported={machine?.capabilities?.uploads??false} uploadsOff={machine?.capabilities?.uploadsOff??false} host={machine?.host || 'Unavailable machine'} user={machine?.capabilities?.osUser || ''} onrename={() => openAction('rename')} onend={() => openAction('end')} onarchive={() => openAction('archive')} archiveDisabled={actionBusy} actionDisabled={actionBusy || machine?.serverEnabled === false} />
            {/if}
          {/key}
        </SessionScreen>
      {:else if selectedSession}
        {@render endedView('This session has ended', "It's no longer running, so there's nothing to reconnect to. Nothing was started in its place. Pick up where you left off, or start fresh.")}
      {:else if !selectedAgent || machine && !setup}
        {@render endedView('Start a new session', 'Nothing is open right now. Start a new agent session or a plain terminal, or jump back into one that is still running.')}
      {:else}<div class="empty"><span class="empty-icon" aria-hidden="true">›_</span><h2>{selectedSession ? 'This session is unavailable' : selectedAgent && !machine ? 'This machine is unavailable' : 'Your next shell starts here.'}</h2><p>{selectedSession ? 'Choose an existing session or create one explicitly. No replacement shell was created.' : setup || 'Create a terminal, then return to the same session whenever you need it.'}</p>{@render startActions()}{#if setup}<button class="btn" onclick={() => detailsOpen = true}>View setup &amp; connection details</button>{/if}</div>{/if}
    </div>
  </div>
</section>
{/if}

{#if viewportDebug}<pre class="viewport-debug" aria-hidden="true">{debugReading}</pre>{/if}
{#if machineSearchOpen}<WorkspaceDialog title="Find a machine" onclose={()=>machineSearchOpen=false}>
  <label class="machine-search">Machine or shell user<input type="search" bind:this={machineSearchInput} bind:value={machineSearch} placeholder="Search connected machines…" /></label>
  <nav class="machine-results" aria-label="Matching machines">{#each matchingMachines as item (item.agentId)}<button class="machine-result" aria-current={item.agentId===selectedAgent?'true':undefined} onclick={()=>chooseMachine(item.agentId)}><strong>{item.host}</strong><span>{item.capabilities?.osUser || 'Shell user unavailable'} · {item.ready?'Ready':readiness(item)}</span></button>{:else}<p>No machines match.</p>{/each}</nav>
</WorkspaceDialog>{/if}
{#if newSessionOpen}<WorkspaceDialog title={newSessionProject ? `New session in ${newSessionProject.name}` : 'New session'} onclose={() => { newSessionOpen = false; newSessionProject = null; newSessionOpener = null }}>
  <p class="dialog-copy">Choose what to start {#if newSessionProject}in <strong>{newSessionProject.name}</strong> on <strong>{newSessionProject.machine || machine?.host || 'this machine'}</strong>{:else}on <strong>{machine?.host || 'your machine'}</strong>{/if}.</p>
  <div class="new-session-choice">
    <button type="button" class="new-session-option" aria-label="Agent session" onclick={() => void chooseNewSession('agent')}><strong>Agent session</strong><span>Give an agent a brief and let it work in a folder.</span></button>
    <button type="button" class="new-session-option" aria-label="Terminal session" onclick={() => void chooseNewSession('terminal')}><strong>Terminal session</strong><span>A shell on the machine, with no agent.</span></button>
  </div>
</WorkspaceDialog>{/if}
{#if showCreate}<WorkspaceDialog title="New terminal session" onclose={() => showCreate = false}><p class="dialog-copy">Choose where your terminal will run.</p>{#if error}<p class="dialog-copy" role="alert">{error}</p>{/if}{@render createForm()}</WorkspaceDialog>{/if}
{#if agentStart}<StartAgentDialog {machines} {projects} snapshot={agentFolders} initialAgent={agentStart.agent} projectId={agentStart.projectId ?? ''} opener={agentStart.opener} onstarted={agentStarted} onaddmachine={()=>location.assign('/machines#add-machine')} onclose={closeAgentStart} />{/if}
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
{#if action === 'rename'}<ConfirmDialog title="Rename session" body="Only the display name changes. The running shell stays the same." confirmLabel="Save name" onconfirm={() => void performAction()} oncancel={() => action = null}><label>Session name<input bind:value={rename} maxlength="80" required /></label></ConfirmDialog>{:else if action === 'end'}<ConfirmDialog title={`End ${actionTitle}?`} body="This stops the shell and its running jobs. Disconnecting keeps them running. If the machine is offline, the request stays pending until it reconnects." confirmLabel="End session" danger onconfirm={() => void performAction()} oncancel={() => action = null}><div class="end-options"><div class="option"><label><input type="checkbox" bind:checked={removeOnClose} disabled={endKeys.has(actionSession)||sessions.find(item=>item.id===actionSession)?.state==='end_requested'} /><span>Remove from history when this session ends</span></label><p>The worktree is removed when it can be; unsaved work stays listed for you to decide. Pending sessions remain visible until the machine confirms they have ended.</p></div><div class="option"><label><input type="checkbox" bind:checked={defaultOn} onchange={()=>defaultEdited=true} /><span>Remove from history by default</span></label><p>{defaultOn===removeDefault?(removeDefault?'This is your current default. Untick to change it.':'Not your current default. Tick to change it.'):(defaultOn?'Will become your default when you end this session.':'Your default will change to keeping history when you end this session.')}</p></div><a href="/account#profile">Manage terminal preferences →</a></div></ConfirmDialog>{:else if action === 'archive'}<ConfirmDialog title={`Remove ${actionTitle} from history?`} body={terminalArchiveBody(actionState)} confirmLabel="Remove from history" onconfirm={()=>void performAction()} oncancel={()=>action=null} />{/if}

<style>
  .ended{position:relative;overflow:hidden}.ended>*{position:relative}.ended-glow{display:none}
  .ended-icon{font-size:2.8rem}
  .ended h2{font-size:2rem}.ended-actions{display:flex;flex-direction:column;gap:12px;margin:0 0 16px;max-width:100%}.ended-cta{font-size:1rem;padding:12px 24px;min-height:48px}
  .ended-banner-actions{display:flex;gap:12px;flex-wrap:wrap}.ended-banner button{font-weight:600}
  .ended-resume{margin-top:24px;max-width:520px;width:100%;text-align:left}.ended-resume h3{font-size:0.875rem;letter-spacing:0.01em;margin:0 0 8px;color:var(--muted)}
  .ended-resume ul{list-style:none;margin:0;padding:0;display:grid;gap:6px}.ended-resume button{width:100%;display:flex;align-items:center;gap:8px;min-height:40px;padding:0 12px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);font-size:0.875rem;text-align:left}.ended-resume button:hover{border-color:var(--accent-text)}
  .ended-dot{width:7px;height:7px;border-radius:50%;background:var(--good)}.ended-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .workspace.theatre>.workspace-toolbar{display:none}
  /* Keyboard focus layout (phones only, see keyboardLikelyOpen): the terminal toolbar keeps Show sessions. */
  .workspace.keyboard>.workspace-toolbar{display:none}
  /* A phone's session screen has its own back arrow in the slim header (held sideways the header gives its rows to the terminal, so the toolbar keeps its arrow). */
  .side-terminal-head { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--line-soft); background: var(--bg-sunk); }
  .side-terminal-head strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.875rem; }
  .side-terminal-head button { min-height: 36px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font-size: 0.875rem; }
  @media (max-width:650px){.workspace:has(:global(.session-screen)) .workspace-title>a{display:none}}
  .session-presentation{display:flex;flex:1;min-height:0;min-width:0;flex-direction:column}.session-presentation[hidden]{display:none}
  .workspace{height:var(--workspace-height);min-height:0;display:flex;flex-direction:column;border:0;border-radius:0;background:var(--bg);overflow:hidden}.workspace-toolbar{display:flex;align-items:center;gap:12px;padding:8px 12px;border-bottom:1px solid var(--line);flex-shrink:0}.workspace-title{display:flex;align-items:center;gap:8px}.workspace-title>a{font-size:1.2rem;text-decoration:none;padding:4px}.workspace-title h1{font:500 1.2rem var(--serif);margin:0}.workspace-actions{display:flex;gap:6px;margin-left:auto;flex-shrink:0}.workspace-actions .btn{font-size:0.875rem;padding:6px 10px;min-height:34px}.workspace-actions .count{font-size:0.875rem;border-radius:6px;background:var(--tint);padding:1px 6px;font-variant-numeric:tabular-nums}.details-toggle{display:inline-flex;align-items:center;gap:7px;min-height:36px;padding:6px 10px;font-size:14px;border-color:var(--line-soft);background:none;color:var(--muted)}.workspace-actions .panel-toggle{display:inline-flex;align-items:center;gap:6px;color:var(--muted)}.workspace-actions .panel-toggle:hover{color:var(--ink)}.workspace-body{display:grid;grid-template-columns:minmax(0,1fr);grid-template-rows:minmax(0,1fr);flex:1;min-height:0;position:relative}.workspace-body.docked{grid-template-columns:var(--panel-width) 8px minmax(0,1fr)}.terminal-stage{min-height:0;min-width:0;display:flex;flex-direction:column;overflow:hidden}.terminal-stage :global(.pane:not(.maximised)){flex:1;min-height:0;border:0;border-radius:0;box-shadow:none}.empty{display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:24px;flex:1;min-height:0;overflow:auto;color:var(--muted)}.empty-icon{font:500 2.2rem var(--mono);color:var(--accent-text)}.empty h2{font:500 1.6rem var(--serif);color:var(--ink);margin:12px 0}.empty>p{font-size:0.875rem;max-width:55ch;margin:0 0 20px}.create-form{display:grid;grid-template-columns:minmax(0,1fr);gap:8px;max-width:480px;width:100%;text-align:left;align-items:end}.create-form label{display:grid;gap:6px;font-size:0.875rem}.create-form input,.create-form select{width:100%;min-width:0}.create-form p{grid-column:1/-1;font-size:0.875rem;color:var(--muted);line-height:1.5;margin:0 0 16px;font-variant-numeric:tabular-nums}.workspace-notice{margin:0;padding:8px 12px;background:var(--tint);font-size:0.875rem;flex-shrink:0;display:flex;align-items:center;gap:8px;justify-content:space-between}.workspace-notice button{border:0;background:none;text-decoration:underline;cursor:pointer;white-space:nowrap;color:inherit;font:inherit}.bad{color:var(--bad)}.dialog-copy,.setup-notice p,.shortcut-help p{font-size:0.875rem;color:var(--muted);line-height:1.6;margin:0}.setup-notice{border:1px solid var(--line);border-left:3px solid var(--accent);padding:12px;border-radius:8px}.setup-notice pre{font-size:0.875rem;white-space:pre-wrap;overflow-wrap:anywhere}h3{font-size:0.875rem;margin:0 0 8px}.shortcut-help p+p{margin-top:8px}
  .creation-error{color:var(--bad)!important}.find-machine{font-size:0.875rem;padding:6px 8px;min-height:34px}.machine-search{display:grid;gap:8px;font-size:0.875rem}.machine-results{display:grid;gap:8px;max-height:50dvh;overflow:auto}.machine-result{display:grid;gap:4px;text-align:left;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);cursor:pointer}.machine-result[aria-current=true]{border-color:var(--accent)}.machine-result span{font-size:0.875rem;color:var(--muted)}
  .new-session-choice{display:grid;gap:8px}.new-session-option{display:grid;gap:4px;text-align:left;padding:12px 16px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);cursor:pointer}.new-session-option:hover,.new-session-option:focus-visible{border-color:var(--accent)}.new-session-option strong{font-size:.9rem}.new-session-option span{font-size:0.875rem;color:var(--muted);line-height:1.5}
  .end-options{display:grid;gap:12px;margin-top:16px;font-size:0.875rem}.end-options .option{display:grid;gap:4px}.end-options label{display:flex;align-items:flex-start;gap:8px;line-height:1.5;cursor:pointer}.end-options input{flex:none;width:16px;height:16px;min-height:0;margin:2px 0 0;padding:0;border:0;background:none;accent-color:var(--accent)}.end-options p,.end-options a{margin:0;font-size:0.875rem;line-height:1.6;color:var(--muted)}.end-options p{margin-left:24px}.end-options a{justify-self:start;color:var(--accent-text)}
  .mobile-label{display:none}
  .viewport-debug{position:fixed;top:4px;right:4px;z-index:200;margin:0;padding:4px 6px;max-width:calc(100vw - 8px);white-space:pre-wrap;font:14px/1.3 var(--mono);color:#fff;background:rgb(0 0 0 / .72);border-radius:4px;pointer-events:none;user-select:none}
  @media(max-height:500px) and (pointer:coarse) and (orientation:landscape){.workspace-toolbar{padding-top:0;padding-bottom:0}}
  .workspace.resizing{cursor:col-resize;user-select:none}
  @media(max-width:650px){.workspace{border-radius:8px}.workspace-toolbar{gap:6px;padding:5px 6px}.workspace-title h1{display:none}.workspace-actions>button[aria-controls="terminal-sessions"]{max-width:52px;white-space:normal;line-height:1.1}.quick-machine{padding:4px 7px;min-width:28px}.workspace-toolbar .find-machine{display:none}.workspace-title>a{padding:3px}.workspace-actions{gap:4px}.workspace-actions .btn{padding:5px 7px;min-height:32px;font-size:0.875rem}.workspace-actions .count,.workspace-actions .wide-label{display:none}.workspace-actions .mobile-label{display:inline}.empty{padding:20px 14px}.empty h2{font-size:1.35rem}.create-form{grid-template-columns:minmax(0,1fr)}.create-form p{grid-column:1}.workspace-notice{font-size:0.875rem;padding:5px 8px}}
  /* Gaffer's super focus on a phone, held upright or sideways: even the workspace frame and toolbar give their room to
     the conversation. */
  @media (max-width:650px), (max-height:500px) and (pointer:coarse) and (orientation:landscape){
    .workspace:has(:global(.session-screen.focus)){border:0;border-radius:0;background:var(--bg)}
    .workspace:has(:global(.session-screen.focus))>.workspace-toolbar{display:none}
  }
  @media(pointer:coarse){.workspace-actions .btn{min-height:40px;min-width:40px}.quick-machine{min-width:40px}.workspace-toolbar .find-machine{min-height:40px}.workspace-title>a{display:inline-flex;align-items:center;justify-content:center;min-width:40px;min-height:40px;box-sizing:border-box}}
  /* A touch tablet (wider than a phone, not a phone held sideways): the toolbar's targets are 44 px, as everything around the session screen. */
  @media(pointer:coarse) and (min-width:651px) and (max-width:1099px) and (min-height:501px){.workspace-actions .btn,.workspace-toolbar .find-machine,.quick-machine{min-height:44px;min-width:44px}.workspace-title>a{display:inline-flex;align-items:center;justify-content:center;min-width:44px;min-height:44px;padding:0;box-sizing:border-box}}

 @media(max-width:650px){.details-toggle{min-width:44px;min-height:44px;padding:0;justify-content:center}.details-toggle span{display:none}}
 @media(pointer:coarse){.details-toggle{min-height:44px}}

</style>
