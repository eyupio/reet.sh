<script lang="ts">
  import GafferChatWidget from './lib/GafferChatWidget.svelte'
  import { gafferAnimations, gafferAutoMerge, gafferCompact, gafferStanding } from './lib/gafferMotion'
  import AppShell from './lib/shell/AppShell.svelte'
  import ProgressBar from './lib/shell/ProgressBar.svelte'
  import { pageGroup, type JumpItem, type Tab } from './lib/shell/tabs'
  import { getDesk, listDesks, type DeskSummary } from './lib/desks/api'
  import { ADVANCED_SYNC, syncRedirect } from './lib/syncRoutes'
  import { PAGE_GROUP } from './lib/shell/pageNav'
  import { onAttentionChanged, signalAttentionChanged } from './lib/attentionSignal'
  import { listFolderProjects, markSessionSeen, sessionHref, terminalMachines, terminalSessions, type FolderProject } from './lib/terminal/api'
  import { sessionOwners, titleWithAttention } from './lib/home'
  import { whatWaits } from './lib/home/whatWaits'
  import { deskPaneReader } from './lib/home/deskSessions'
  import type { WaitingEntry } from './lib/home/homeWaiting'
  import { installationReports, inventoryMachines } from './lib/machines'
  import { settledJobsKey } from './lib/skillSync'
  import { listGafferGoals, listGafferOutcomes, type GafferGoalOverview, type GafferOutcome } from './lib/api'
  import { DOCS_URL } from './lib/links'
  import FenceStrip from './lib/shell/FenceStrip.svelte'
  import { onMount, setContext } from 'svelte'
  import {
    applyTheme, clearThemeChoice, clearToken, getAuthStatus, getNotificationCount, getProfile, getSyncState, getToken, listEnvironments, listSkills,
    saveProfile, signOutAccount, syncLibraryMachines, takeThemeChoice, UnauthorizedError,
    type Environment, type Skill, type User,
  } from './lib/api'
  import AccountMenu from './lib/AccountMenu.svelte'
  import WebMCP from './lib/WebMCP.svelte'
  import BootSkeleton from './lib/BootSkeleton.svelte'
  import RouteSkeleton from './lib/RouteSkeleton.svelte'
  import PageLoadError from './lib/PageLoadError.svelte'
  import { loadPage } from './lib/pageLoad'
  import Footer from './lib/Footer.svelte'
  import Logo from './lib/Logo.svelte'
  import NotFound from './lib/NotFound.svelte'
  import PageHeader from './lib/PageHeader.svelte'
  import ThemePicker from './lib/ThemePicker.svelte'
  import SyncActivity from './lib/SyncActivity.svelte'
  import { fleetSyncSentence, fleetSyncStatus } from './lib/fleetSync'
  import Toast from './lib/Toast.svelte'
  import { ssoErrorFromSearch, ssoErrorMessage, withoutSsoError, type SsoStatus } from './lib/sso'
  import Tools from './lib/Tools.svelte'
  import { skillsExport } from './lib/skills/exportFilters.svelte'

  // Each route keeps its code outside the shell, including the public and sign-in screens.
  const pages = {
    ActivityPage: () => loadPage(() => import('./lib/ActivityPage.svelte')),
    SessionsPage: () => loadPage(() => import('./lib/sessions/SessionsPage.svelte')),
    Factory: () => loadPage(() => import('./lib/factory/Factory.svelte')),
    Desks: () => loadPage(() => import('./lib/desks/DesksPage.svelte')),
    Machines: () => loadPage(() => import('./lib/Machines.svelte')),
    Home: () => loadPage(() => import('./lib/home/Home.svelte')),
    Community: () => loadPage(() => import('./lib/Community.svelte')),
    SharedLists: () => loadPage(() => import('./lib/SharedLists.svelte')),
    Spaces: () => loadPage(() => import('./lib/Spaces.svelte')),
    Account: () => loadPage(() => import('./lib/Account.svelte')),
    MCPConsent: () => loadPage(() => import('./lib/MCPConsent.svelte')),
    Hosting: () => loadPage(() => import('./lib/Hosting.svelte')),
    AccountSignIn: () => loadPage(() => import('./lib/AccountSignIn.svelte')),
    Admin: () => loadPage(() => import('./lib/Admin.svelte')),
    Join: () => loadPage(() => import('./lib/Join.svelte')),
    ListView: () => loadPage(() => import('./lib/ListView.svelte')),
    PublicList: () => loadPage(() => import('./lib/PublicList.svelte')),
    ResetPassword: () => loadPage(() => import('./lib/ResetPassword.svelte')),
    Setup: () => loadPage(() => import('./lib/Setup.svelte')),
    Library: () => loadPage(() => import('./lib/Library.svelte')),
    Skills: () => loadPage(() => import('./lib/skills/SkillsPage.svelte')),
    SetupProfiles: () => loadPage(() => import('./lib/SetupProfiles.svelte')),
    Prompts: () => loadPage(() => import('./lib/Prompts.svelte')),
    Usage: () => loadPage(() => import('./lib/UsagePage.svelte')),
    SignIn: () => loadPage(() => import('./lib/SignIn.svelte')),
    SignUp: () => loadPage(() => import('./lib/SignUp.svelte')),
    Notifications: () => loadPage(() => import('./lib/Notifications.svelte')),
    Support: () => loadPage(() => import('./lib/Support.svelte')),
    Ticket: () => loadPage(() => import('./lib/Ticket.svelte')),
  }

  // The library as Jump searches it, on every page. The Skills page reads its own rows.
  let skills = $state<Skill[]>([])
  let timeZone = $state('')
  let dateFormat = $state('locale')
  let error = $state('')
  let needsAuth = $state(false)
  let token = $state(getToken())
  let authMode = $state<'accounts' | 'token' | 'none' | ''>('')
  let authChecked = $state(false)
  let setupRequired = $state(false)
  let bootstrapRequired = $state(false)
  let signupEnabled = $state(false)
  let contactEmail = $state('')
  let passwordReset = $state(false)
  let sso = $state<SsoStatus | undefined>(undefined)
  let unreadNotifications = $state(0)
  let currentUser = $state<User | null>(null)
  type ToastState = { id: number; message: string; tone: 'ok' | 'bad'; actionLabel?: string; onaction?: () => void | Promise<void> }
  let toast = $state<ToastState | null>(null)
  let toastId = 0
  let environments = $state<Environment[]>([])
  let machines = $state<import('./lib/api').Machine[]>([])
  let inventoryNow = $state(Date.now())
  let syncWatch = $state<{ host: string; title: string; note: string } | null>(null)
  let syncingAll = $state(false)
  const shelfMachines = $derived(inventoryMachines(machines))
  const machineReports = $derived(installationReports(shelfMachines, inventoryNow))
  let syncSnapshot = $state<import('./lib/api').SyncState | undefined>(undefined)
  let metaLoaded = $state(false)

  // /projects and /projects/<id> are retired: folder projects live in Home's rail. The address becomes / at once (Home then
  // looks an old Project id up and shows /?project=<its folder project>), and Home says so once, for this page load only.
  const retiredProjects = location.pathname.match(/^\/projects(?:\/([^/]+))?\/?$/)
  // A malformed escape in an old link (/projects/%E0) still goes to Home, just without a project to look up.
  const legacyProject = (() => { try { return retiredProjects?.[1] ? decodeURIComponent(retiredProjects[1]) : '' } catch { return '' } })()
  if (retiredProjects) window.history.replaceState(window.history.state, '', '/')
  // Control is folded into Home and Machines (R4): /control goes to Home. The Sessions tab is /sessions (R17): /terminals goes
  // there keeping its query and fragment, so an old link to a session (?agent=…&session=…) still opens it.
  if (/^\/control\/?$/.test(location.pathname)) window.history.replaceState(window.history.state, '', '/')
  // /sessions/ (a trailing slash) is /sessions.
  if (location.pathname === '/sessions/') window.history.replaceState(window.history.state, '', `/sessions${location.search}${location.hash}`)
  if (/^\/terminals\/?$/.test(location.pathname)) window.history.replaceState(window.history.state, '', `/sessions${location.search}${location.hash}`)
  // Sync is two pages. A Sync address that carries something only the plan controls read (a machine, a skill to remove, a
  // list, or their anchor) goes to Advanced sync with all of it, so an older link never lands on a page that ignores it.
  const syncMoved = syncRedirect(location.pathname, location.search, location.hash)
  if (syncMoved) window.history.replaceState(window.history.state, '', syncMoved)
  // `/` is Home: the full Home for a writer in accounts mode, the fleet strip alone for everyone else signed in (R16).
  const isHome = location.pathname === '/'
  const bootApp = document.documentElement.dataset.boot === 'app'
  const bootLayout = document.documentElement.dataset.bootLayout ?? 'content'
  const isActivity = location.pathname === '/activity'
  const isSkills = location.pathname === '/skills'
  // The Factory: Gaffer's outcomes drawn as pipelines. It is for an account that can start sessions.
  const isFactory = location.pathname === '/factory'
  // Desks: saved sets of terminals. /desks lists them (or opens the last one used) and /desks/<id> opens one; for the same
  // accounts as the Factory. An open desk takes the whole window, as the session workspace does (`deskOpen`).
  const isDesks = /^\/desks(?:\/[^/]+)?\/?$/.test(location.pathname)
  let deskOpen = $state(/^\/desks\/[^/]+\/?$/.test(location.pathname))
  const machineView = new URLSearchParams(location.search).get('view')
  const legacyMachinePage = ['/machines', '/account/machines', '/account/sync'].includes(location.pathname)
  // /sessions lists the sessions; with `session` or `agent` (a session, or a machine's sessions) or the workspace's own
  // view=terminal in the query it opens the terminal workspace there, with the session screen around the chosen session.
  const sessionsQuery = new URLSearchParams(location.search)
  const isSessionsPage = location.pathname === '/sessions'
  // A bare /sessions is the workspace too (its empty state offers a new session); the older list stays at /sessions?view=list.
  const isSessions = isSessionsPage && machineView === 'list'
  const isTerminals = isSessionsPage && !isSessions || legacyMachinePage && machineView === 'terminal'
  // The sync-plan controls on their own page. It is a Sync page for the tabs and the header's row.
  const isSyncAdvanced = location.pathname === ADVANCED_SYNC
  const isSync = location.pathname === '/sync' || isSyncAdvanced || legacyMachinePage && !isTerminals && (machineView === 'compare' || location.pathname === '/account/sync')
  const isHosting = location.pathname === '/hosting'
  const isCommunity = location.pathname === '/community'
  const isShared = location.pathname === '/shared'
  const isSpaces = location.pathname === '/spaces'
  const isLibrary = location.pathname === '/library'
  const isProfiles = ['/profiles', '/setups'].includes(location.pathname)
  const isPrompts = location.pathname === '/prompts' || location.pathname === '/prompts/'
  // Usage: each agent's plan left and the tokens its sessions reported, for the accounts that have Sessions.
  const isUsage = location.pathname === '/usage' || location.pathname === '/usage/'
  // Without Sessions access (a viewer, a token or an open instance) /sessions shows Machines, as /terminals did.
  const isMachines = isSync || isTerminals || isSessions || location.pathname === '/machines' || ['/account/machines', '/account/sync'].includes(location.pathname)
  const isAdmin = location.pathname.startsWith('/admin')
  const isAccount = location.pathname.startsWith('/account')
  const isMCPConsent = location.pathname === '/account/mcp/authorize'
  const isJoin = location.pathname.startsWith('/join')
  const isReset = location.pathname === '/reset'
  // Support exists only where the operator turned the support desk on; elsewhere its pages are not found.
  let supportDesk = $state(false)
  const isSupport = $derived(supportDesk && (location.pathname === '/support' || location.pathname === '/support/'))
  const supportMatch = location.pathname.match(/^\/support\/(\d+)\/?$/)
  const isTicket = $derived(supportDesk && supportMatch !== null)
  const isNotifications = location.pathname === '/notifications'
  const supportTicketID = supportMatch ? Number(supportMatch[1]) : 0
  const isWelcome = location.pathname === '/' || location.pathname === '/welcome'
  const isSignIn = location.pathname === '/signin'
  const isSignUp = location.pathname === '/signup'
  // Addresses that only make sense before sign-in. Finishing sign-in or setup on one of them continues at Home (`/`).
  const entryPaths = ['/welcome', '/signin', '/signup', '/setup']
  const listMatch = location.pathname.match(/^\/lists\/(\d+)\/?$/)
  const isList = listMatch !== null
  const listId = listMatch ? Number(listMatch[1]) : 0
  const publicMatch = location.pathname.match(/^\/l\/([^/]+)\/?$/)
  const isPublicList = publicMatch !== null
  const publicSlug = publicMatch ? decodeURIComponent(publicMatch[1]) : ''
  const joinToken = new URLSearchParams(location.search).get('token') ?? ''
  const signedIn = $derived(token !== '')
  const authenticated = $derived(authMode === 'none' || (authMode === 'token' && token !== '') || (authMode === 'accounts' && currentUser !== null))
  const canLoad = $derived(authChecked && authenticated && !needsAuth && !isCommunity && !isPublicList)
  const role = $derived(currentUser?.role ?? (authMode === 'accounts' ? 'viewer' : 'owner'))
  const canWrite = $derived(authMode !== 'accounts' || role !== 'viewer')
  // Sync all machines is one button wherever it appears. With a machine to sync to and a role that may, it does the job
  // where it is. Otherwise it opens Sync, which explains what is missing.
  const syncInPlace = $derived(canWrite && (syncSnapshot?.agents?.length ?? 0) > 0)
  const terminalLayout = $derived(isTerminals && authMode === 'accounts' && authenticated && canWrite && !needsAuth)
  // Desks run the whole width with no footer; an open desk also takes the slim header, as the session workspace does.
  const desksLayout = $derived(isDesks && authMode === 'accounts' && authenticated && canWrite && !needsAuth && !setupRequired)
  const deskScreen = $derived(desksLayout && deskOpen)
  const isAdminUser = $derived(authMode !== 'accounts' || role === 'owner' || role === 'admin')
  // Navigation (R13): four tabs on a desktop (Activity lives in the account; without accounts it is a fifth tab); Home,
  // Sessions and Machines stay in view on a tablet or phone and the rest
  // move into More. Sessions and the bell are for a writer in accounts mode only (R16), as Terminals and Projects were.
  const sessionsAccess = $derived(authMode === 'accounts' && canWrite)
  // What waits for the person, by the one rule Home uses (whatWaits): the bell lists it, and its badge and the title
  // count it, so the three always agree. Home reports its own reading; every other page reads it here.
  let waitingEntries = $state<WaitingEntry[]>([])
  const attention = $derived(waitingEntries.length)
  let onAnswered = $state<(() => void | Promise<void>) | undefined>(undefined)
  let onMarkSeen = $state<((sessionId: string) => void | Promise<void>) | undefined>(undefined)
  let workspaceTitle = $state('')
  // Home spans the page: the project rail on the left and the docked Start an agent at the foot.
  // Home and the Factory hold the project rail as their own left column, so both take the whole width and draw no footer.
  const homeLayout = $derived((isHome || (isFactory && sessionsAccess && authMode === 'accounts')) && authChecked && !!authMode && authenticated && !needsAuth && !(authMode === 'accounts' && setupRequired))
  const tabs = $derived<Tab[]>([
    { href: '/', label: 'Home', current: isHome },
    ...(sessionsAccess ? [{ href: '/sessions', label: 'Sessions', current: isTerminals || isSessions }] : []),
    ...(sessionsAccess && authMode === 'accounts' ? [{ href: '/factory', label: 'Factory', current: isFactory }] : []),
    ...(sessionsAccess && authMode === 'accounts' ? [{ href: '/desks', label: 'Desks', current: isDesks }] : []),
    { href: '/machines', label: 'Machines', current: isMachines && !((isTerminals || isSessions) && sessionsAccess) },
    { href: '/skills', label: 'Skills', current: isSkills },
    ...(authMode === 'accounts' ? [] : [{ href: '/activity', label: 'Activity' as const, current: isActivity }]),
  ])
  // The phone's bottom bar and a tablet's top bar: Home, Sessions, Factory, Desks and Machines (each where it is offered),
  // then More, which holds Skills and the rest there.
  const barTabs = $derived(tabs.filter(tab => ['/', '/sessions', '/factory', '/desks', '/machines'].includes(tab.href)))
  const more = $derived<Tab[]>([
    ...(canWrite ? [{ href: '/library', label: 'Library', group: 'Skills & tools', current: isLibrary }] : []),
    ...(authMode === 'accounts' && canWrite ? [{ href: '/prompts', label: 'Prompts', group: 'Skills & tools', current: isPrompts }] : []),
    { href: '/profiles', label: 'Setups', group: 'Skills & tools', current: isProfiles },
    { href: '/shared', label: 'Lists', group: 'Skills & tools', current: isShared || isList },
    ...(canWrite ? [{ href: '/hosting', label: 'Write a skill', group: 'Skills & tools', current: isHosting }] : []),
    ...(sessionsAccess ? [{ href: '/usage', label: 'Usage', group: 'Workspace', current: isUsage }] : []),
    { href: '/spaces', label: 'Teams & organisations', group: 'Workspace', current: isSpaces },
    // An open instance has no account menu.
    ...(authMode === 'none' ? [{ href: '/admin/overview', label: 'Instance administration', group: 'Workspace', current: isAdmin }] : []),
    { href: '/community', label: 'Community', group: 'Help & community', current: isCommunity },
    ...(supportDesk ? [{ href: '/support', label: 'Support', group: 'Help & community', current: isSupport || isTicket }] : []),
    { href: DOCS_URL, label: 'Documentation', group: 'Help & community', current: false },
  ])
  // Every routed page's header reads the current page's group from here (the getter keeps it current as `more` changes).
  // Signed out, a public page names its group but has no siblings to offer (the other pages need a sign-in), so it gets the label only.
  setContext(PAGE_GROUP, { get group() {
    const found = pageGroup(more)
    return found && !authenticated ? { ...found, items: found.items.filter(item => item.current) } : found
  } })
  // Jump searches what the workspace already lists: sessions, folder projects and machines (a writer's terminal machines,
  // and the machines on the shelf inventory for everyone). The sessions, projects and terminal machines are read when Jump
  // is used, and a list that cannot be read is left out. Skills and shelf machines are already in memory, so they are passed
  // as they change (`jumpLive`) instead of being read once and held, which left them out when they finished loading later.
  async function jumpItems(): Promise<JumpItem[]> {
    const items: JumpItem[] = []
    const hosts = new Map<string, string>()
    if (sessionsAccess) {
      const [sessions, projects, terminal, desks] = await Promise.allSettled([terminalSessions(), listFolderProjects(), terminalMachines(), authMode === 'accounts' ? listDesks() : Promise.resolve([])])
      if (terminal.status === 'fulfilled') for (const machine of terminal.value) hosts.set(machine.agentId, machine.host)
      if (sessions.status === 'fulfilled') for (const session of sessions.value) {
        const host = hosts.get(session.agentId)
        items.push({ kind: 'session', label: session.title, detail: host ? `Session · ${host}` : 'Session', href: sessionHref(session) })
      }
      if (desks.status === 'fulfilled') for (const desk of desks.value) if (!desk.template) items.push({ kind: 'desk', label: desk.name, detail: `Desk · ${desk.tabs} ${desk.tabs === 1 ? 'tab' : 'tabs'}`, href: `/desks/${encodeURIComponent(desk.id)}` })
      if (projects.status === 'fulfilled') for (const project of projects.value) items.push({ kind: 'project', label: project.name, detail: project.machine ? `Project · ${project.machine}` : 'Project', href: `/?project=${encodeURIComponent(project.id)}` })
      if ([sessions, projects, terminal].every(result => result.status === 'rejected')) throw new Error('Nothing could be read.')
    }
    for (const host of new Set(hosts.values())) items.push({ kind: 'machine', label: host, detail: 'Machine', href: `/machines?machine=${encodeURIComponent(host)}` })
    return items
  }
  const jumpLive = $derived.by<JumpItem[]>(() => {
    const items: JumpItem[] = []
    // A skill opens on the Skills page, which finds its row by this id and brings it into view.
    for (const skill of skills) {
      const detail = skill.source ? `Skill · ${skill.source}` : 'Skill'
      items.push({ kind: 'skill', label: skill.name, detail, href: `/skills?skill=${encodeURIComponent(skill.id)}` })
    }
    for (const host of new Set(shelfMachines.map(machine => machine.host))) items.push({ kind: 'machine', label: host, detail: 'Machine', href: `/machines?machine=${encodeURIComponent(host)}` })
    return items
  })

  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) {
      clearToken()
      token = ''
      if (authMode === 'accounts') currentUser = null
      gafferAnimations.set(true); gafferCompact.set(true)
      needsAuth = true
      toast = null
      return
    }
    error = e instanceof Error ? e.message : String(e)
  }
  function ok(m: string) {
    showToast(m)
    error = ''
  }
  function showToast(message: string, actionLabel?: string, onaction?: () => void | Promise<void>, tone: 'ok' | 'bad' = 'ok') {
    toast = { id: ++toastId, message, actionLabel, onaction, tone }
  }
  function dismissToast() {
    toast = null
  }

  let loadVersion = 0

  // Reads the library for Jump. It is also how a single-token sign-in is checked: a token the instance refuses fails here.
  async function load(): Promise<boolean> {
    const version = ++loadVersion
    try {
      const r = await listSkills({ tab: 'all', q: '', sort: 'name' })
      if (version !== loadVersion) return false
      skills = r.skills
      error = ''
      needsAuth = false
      if (!metaLoaded) void loadMeta().then((ok) => (metaLoaded = ok))
      return true
    } catch (e) {
      if (version !== loadVersion) return false
      fail(e)
      return false
    }
  }

  async function loadMeta(): Promise<boolean> {
    try {
      const [envs, sync] = await Promise.all([listEnvironments(), getSyncState().catch(() => undefined)])
      environments = envs.environments
      syncSnapshot = sync
      settledJobs = settledJobsKey(sync)
      machines = envs.machines
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  async function reloadAll() {
    await load()
    metaLoaded = await loadMeta()
  }

  async function loadNotificationCount() {
    if (!authenticated) return
    try { unreadNotifications = (await getNotificationCount()).unread } catch { /* a count is not worth an error banner */ }
  }

  // A slow earlier load must not overwrite a newer one (an answer's refresh can overtake the poll's).
  // The Factory's outcomes and Gaffer's goals as last read: a failed read of either keeps the last count, as a failed
  // sessions read does.
  // The desks too: a waiting session on a desk is marked with it and opens on it. Until they are read none is marked.
  let attentionGeneration = 0, lastOutcomes: GafferOutcome[] = [], lastGoals: GafferGoalOverview[] | undefined, lastDesks: DeskSummary[] | undefined
  const readDeskPanes = deskPaneReader(getDesk)
  async function loadAttention() {
    if (!authenticated || needsAuth || !sessionsAccess || isHome) return
    const mine = ++attentionGeneration
    try {
      // A background read: like the terminal reads, it does not drive the progress bar.
      const [read, outcomes, goals, desks] = await Promise.allSettled([terminalSessions(), listGafferOutcomes(undefined, true), listGafferGoals(undefined, true), listDesks()])
      if (mine !== attentionGeneration) return
      if (read.status === 'rejected') return
      const sessions = read.value
      if (outcomes.status === 'fulfilled') lastOutcomes = outcomes.value
      if (goals.status === 'fulfilled') lastGoals = goals.value.goals
      if (desks.status === 'fulfilled') lastDesks = desks.value
      let projects: FolderProject[] = []
      const hosts = environments.map(e => ({ agentId: e.id, host: e.host }))
      if (sessions.length > 0) {
        const [p, m] = await Promise.allSettled([sessions.some(s => s.projectId) ? listFolderProjects() : Promise.resolve([]), terminalMachines()])
        if (mine !== attentionGeneration) return
        if (p.status === 'fulfilled') projects = p.value
        if (m.status === 'fulfilled') for (const tm of m.value) hosts.push({ agentId: tm.agentId, host: tm.host })
      }
      if (mine !== attentionGeneration) return
      const owners = sessionOwners(sessions, projects), at = Date.now()
      let entries = whatWaits(sessions, at, owners, hosts, lastOutcomes, lastGoals, { desks: lastDesks }).entries
      // A waiting desk session opens on the desk at its pane: the panes of the desks holding one (each read once per version).
      const onDesks = new Set(entries.flatMap(entry => entry.source !== 'factory' && entry.desk ? [entry.source === 'session' ? entry.card.sessionId : entry.goal.sessionId] : []))
      if (lastDesks && onDesks.size) {
        const panes = await readDeskPanes(lastDesks, onDesks, AbortSignal.timeout(15_000))
        if (mine !== attentionGeneration) return
        entries = whatWaits(sessions, at, owners, hosts, lastOutcomes, lastGoals, { desks: lastDesks, panes }).entries
      }
      waitingEntries = entries
      onAnswered = () => loadAttention()
      onMarkSeen = async (sessionId: string) => {
        try {
          await markSessionSeen(sessionId)
          signalAttentionChanged()
          await loadAttention()
        } catch { /* next poll recovers */ }
      }
    } catch { /* bell stays quiet when terminal sessions cannot be read */ }
  }

  onMount(() => {
    void initAuth().then(() => {
      if (authenticated) {
        void loadNotificationCount()
        void loadAttention()
        if (isHosting) void loadMeta()
      }
    })
    const changed = () => { if (authenticated) void reloadAll() }
    window.addEventListener('reet-shelf-changed', changed)
    // A page that cannot sign anyone in itself (the Skills page) says its sign-in has ended; the sign-in screen follows.
    const signedOut = () => fail(new UnauthorizedError())
    window.addEventListener('reet-signed-out', signedOut)
    const timer = setInterval(() => (inventoryNow = Date.now()), 60000)
    const notifications = setInterval(() => { if (authenticated) void loadNotificationCount() }, 60000)
    const attentionPoll = setInterval(() => { if (authenticated && document.visibilityState === 'visible') void loadAttention() }, 15000)
    const offAttention = onAttentionChanged(() => { void loadAttention() })
    return () => { offAttention(); clearInterval(timer); clearInterval(notifications); clearInterval(attentionPoll); window.removeEventListener('reet-shelf-changed', changed); window.removeEventListener('reet-signed-out', signedOut) }
  })

  async function initAuth() {
    try {
      const status = await getAuthStatus()
      authMode = status.mode
      setupRequired = status.setupRequired
      bootstrapRequired = status.bootstrapRequired
      signupEnabled = status.signupEnabled ?? false
      supportDesk = status.supportDesk ?? false
      contactEmail = status.contactEmail ?? ''
      passwordReset = status.passwordReset ?? false
      sso = status.sso
      currentUser = status.user
      // The server sends a sign-in that could not finish to the home page, which for someone signed out is the welcome page:
      // take the code to the sign-in page, where it is said in words.
      if (!status.user && status.mode === 'accounts' && ssoErrorFromSearch(location.search) && location.pathname !== '/signin') {
        location.replace('/signin' + location.search)
        return
      }
      // A link that could not finish lands on a signed-in page with a code in the address: say it once.
      const ssoProblem = ssoErrorMessage(ssoErrorFromSearch(location.search))
      if (status.user && ssoProblem) {
        showToast(ssoProblem, undefined, undefined, 'bad')
        window.history.replaceState(null, '', withoutSsoError(location.pathname + location.search + location.hash))
      }
      if (status.user && entryPaths.includes(location.pathname)) {
        location.replace('/')
        return
      }
      if (status.mode === 'token') {
        token = getToken()
        needsAuth = token === ''
      } else {
        clearToken()
        token = ''
        needsAuth = status.mode === 'accounts' ? !status.user : false
      }
      if (status.user) await loadPreferences()
      await applyPendingTheme()
    } catch {
      authMode = ''
    } finally {
      authChecked = true
    }
  }

  // The profile's saved theme is the baseline, but a colour chosen on the public header while signed out is the most
  // recent deliberate choice: apply it and make it the account's saved theme, so it survives sign-in.
  async function applyPendingTheme() {
    if (authMode !== 'accounts' || !currentUser) return
    const pending = takeThemeChoice()
    if (!pending) return
    applyTheme(pending)
    try {
      await saveProfile({ theme: pending })
    } catch {
      /* the choice still applies for this visit */
    }
  }

  async function loadPreferences() {
    try {
      const actor = currentUser?.id
      const { profile } = await getProfile()
      if (actor !== currentUser?.id) return
      applyTheme(profile.theme)
      gafferAnimations.set(profile.gafferAnimations ?? true); gafferCompact.set(profile.gafferCompact ?? true); gafferStanding.set(profile.gafferStanding ?? true); gafferAutoMerge.set(profile.gafferAutoMerge ?? false)
      timeZone = profile.timezone
      dateFormat = profile.dateFormat || 'locale'
    } catch {
      /* the shelf still works without a stored preference */
    }
  }

  async function afterAuth(user: User) {
    authMode = 'accounts'
    currentUser = user
    setupRequired = false
    needsAuth = false
    token = ''
    clearToken()
    if (entryPaths.includes(location.pathname)) {
      location.replace('/')
      return
    }
    await loadPreferences()
    await applyPendingTheme()
    await load()
    await loadMeta()
    await loadNotificationCount()
    await loadAttention()
  }

  async function afterJoin(user: User) {
    await afterAuth(user)
    location.assign('/')
  }

  async function attemptSignIn(): Promise<string | null> {
    const accepted = await load()
    if (accepted) {
      token = getToken()
      return null
    }
    if (needsAuth) return 'That token was not accepted.'
    return error || 'Could not reach your library.'
  }

  async function signOut() {
    unreadNotifications = 0
    if (authMode === 'accounts') {
      try {
        await signOutAccount()
      } catch {
        /* the local session is cleared even if the server call cannot be made */
      }
      currentUser = null
      clearThemeChoice()
      gafferAnimations.set(true); gafferCompact.set(true)
      applyTheme('system')
      needsAuth = true
      token = ''
      clearToken()
      error = ''
      toast = null
      return
    }
    clearToken()
    token = ''
    error = ''
    toast = null
    await load()
  }

  $effect(() => {
    if (!canLoad) return
    const t = setTimeout(load, 150)
    return () => clearTimeout(t)
  })

  // The activity dock polls sync while an install runs. Keep Write a skill's sync line on that state, and when a job
  // settles re-read inventory too, so Pending becomes Set without a reload.
  let settledJobs = ''
  function syncUpdated(next: import('./lib/api').SyncState) {
    syncSnapshot = next
    const key = settledJobsKey(next)
    if (key === settledJobs) return
    settledJobs = key
    // The agent scans after applying, so this settle already holds the new inventory.
    void reloadAll()
  }

  // The whole job in one press. When work starts, the activity dock follows it live; when there is nothing to start, one
  // line says where things stand. A failure is a toast, so it shows on every page this button is on.
  async function syncAll() {
    if (!canWrite || syncingAll) return
    syncingAll = true
    try {
      const next = await syncLibraryMachines()
      syncUpdated(next)
      const status = fleetSyncStatus(next)
      if (status.state === 'syncing') syncWatch = { host: '', title: 'Syncing your machines', note: 'All connected machines · live progress' }
      else ok(fleetSyncSentence(status))
    } catch (e) {
      if (e instanceof UnauthorizedError) fail(e)
      else showToast(e instanceof Error ? e.message : String(e), undefined, undefined, 'bad')
    } finally {
      syncingAll = false
    }
  }

  // Library tools sit under the Skills page. Their changes reach the page (and Jump) through the shelf-changed signal.
  function toolsFail(e: unknown) {
    if (e instanceof UnauthorizedError) fail(e)
    else showToast(e instanceof Error ? e.message : String(e), undefined, undefined, 'bad')
  }
  async function toolsChanged() {
    window.dispatchEvent(new Event('reet-shelf-changed'))
  }

  const defaultPageTitle = $derived(
    (isCommunity ? 'Public community' : isShared ? 'Your lists' : isSpaces ? 'Teams & organisations' : isJoin ? 'Join reet.sh' : isReset ? 'Choose a new password' : authMode === 'accounts' && setupRequired ? 'Set up reet.sh' : isSignUp ? 'Create your account' : authChecked && !authenticated ? 'Sign in' : isHosting ? 'Write a skill' : isLibrary ? 'Library' : isNotifications ? 'Notifications' : isSupport ? 'Support' : isTicket ? 'Support ticket' : isAdmin ? 'Instance administration' : isHome ? 'Home' : isFactory ? 'Factory' : isDesks && sessionsAccess ? 'Desks' : isActivity ? 'Activity' : isSyncAdvanced ? 'Advanced sync' : isSync ? 'Sync' : (isTerminals || isSessions) && sessionsAccess ? 'Sessions' : isMachines ? 'Machines' : isAccount ? 'Personal settings' : isProfiles ? 'Setups' : isPrompts ? 'Prompts' : isUsage ? 'Usage' : isList ? 'Shared list' : isSkills ? 'Skills' : 'Page not found') + ' · reet.sh'
  )
  const pageTitle = $derived(workspaceTitle || defaultPageTitle)

  // Public pages draw without the first auth answer: the shared list reads its own content, and the join and reset
  // screens need nothing from it. Their shell must not wait on an unrelated request that may be slow or stalled.
  const authIndependent = isCommunity || isPublicList || isJoin || isReset

  // The static boot shell in index.html fades once the page below it is the real one: at the first auth answer, or at
  // once on a public route that never depended on it. There is no half-drawn screen behind the fade either way.
  $effect(() => {
    if (authChecked || authIndependent) document.documentElement.dataset.ready = '1'
    // The boot shell reserves the signed-in workspace's full width. A signed-out or auth answer gives the page back its
    // reading measure, including a static preview that guessed "app" before the auth answer arrived.
    if (authChecked && (!authenticated || needsAuth || isPublicList || isJoin)) document.documentElement.removeAttribute('data-app-shell')
  })

  // A link to a section (/welcome#self-host) lands before the app has drawn that section, so the browser has nowhere to
  // scroll to. Once the page below the boot shell is real, take the fragment to its target ourselves.
  $effect(() => {
    if (!authChecked && !authIndependent) return
    let id = location.hash.slice(1)
    if (!id) return
    try { id = decodeURIComponent(id) } catch { return }
    document.getElementById(id)?.scrollIntoView({ block: 'start' })
  })
</script>

<svelte:head>
  {#if !isPublicList}
    <title>{titleWithAttention(pageTitle, attention)}</title>
  {/if}
</svelte:head>

<a class="skip" href="#main-content">Skip to content</a>
<ProgressBar />
<header class="site-header" class:terminal-chrome={terminalLayout}>
  {#if authenticated && !needsAuth && !isPublicList && !isJoin}
    <AppShell {tabs} bar={barTabs} {more} waiting={waitingEntries} onseen={onMarkSeen} onanswered={onAnswered} compact={terminalLayout || deskScreen} bell={sessionsAccess} jump={jumpItems} jumpLive={jumpLive}>
      {#snippet account()}
        {#if authMode !== 'none'}
          <AccountMenu
            name={currentUser?.name ?? ''}
            email={currentUser?.email ?? ''}
            accounts={authMode === 'accounts'}
            personalCurrent={isAccount || (authMode === 'accounts' && isActivity)}
            instanceAdmin={isAdminUser}
            adminCurrent={isAdmin}
            notificationsCurrent={isNotifications}
            unread={unreadNotifications}
            onsignout={signOut}
          />
        {/if}
      {/snippet}
    </AppShell>
  {:else}
  <div class="header-inner">
    <div>
      <a class="home" href="/" aria-label="reet.sh home"><Logo /></a>
    </div>
    {#if !isPublicList && !isCommunity}
      <nav class="site-nav" aria-label="Instance help"><a href={DOCS_URL} target="_blank" rel="noopener noreferrer">Documentation <svg aria-hidden="true" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M5 19 19 5M5 5h14v14"/></svg></a><ThemePicker /></nav>
    {:else}
      <nav class="site-nav" aria-label="Public navigation"><a href="/community">Public community</a><a href={DOCS_URL}>Docs</a><ThemePicker /><a class="entry-link" href="/">Open your workspace →</a></nav>
    {/if}
    {#if (!authenticated || needsAuth) && authMode !== 'accounts' && !isPublicList && !isCommunity && !isJoin && !isWelcome}<span class="tag">Your AI workspace</span>{/if}
  </div>
  {/if}
</header>
{#if authenticated && !needsAuth && !isPublicList && !isJoin}
  <!-- The session workspace fills the viewport to the pixel; Backups and Move say the same thing in full. They keep the
       listening and drop the line. -->
  <FenceStrip admin={isAdminUser} quiet={terminalLayout || deskScreen || ['/admin/backups', '/admin/move'].includes(location.pathname)} {timeZone} {dateFormat} />
{/if}
<main id="main-content" tabindex="-1" class:terminal-layout={terminalLayout || (!authChecked && bootApp && bootLayout === 'workspace')} class:home-layout={homeLayout || (!authChecked && bootApp && bootLayout === 'home')} class:desks-layout={desksLayout} class:auth-page={(authChecked || !bootApp) && !isPublicList && !isCommunity && (!authenticated || needsAuth || isJoin)}>
  {#if isCommunity}
    {#await pages.Community()}
      <RouteSkeleton variant="cards" />
    {:then { default: Page }}
      <Page canSave={authenticated && !needsAuth && canWrite} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isPublicList}
    {#await pages.PublicList()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page slug={publicSlug} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isJoin}
    {#await pages.Join()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page token={joinToken} onsuccess={afterJoin} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isReset}
    {#await pages.ResetPassword()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if !authChecked}
    <BootSkeleton layout={bootApp ? bootLayout : 'content'} />
  {:else if authMode === 'accounts' && setupRequired}
    {#await pages.Setup()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page {bootstrapRequired} onsuccess={afterAuth} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if authMode === 'accounts' && !currentUser}
    {#if isSignUp && signupEnabled}
      {#await pages.SignUp()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page onsuccess={afterAuth} />
    {:catch}
      <PageLoadError />
    {/await}
    {:else}
      {#await pages.AccountSignIn()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page onsuccess={afterAuth} {signupEnabled} {contactEmail} {passwordReset} {sso} />
    {:catch}
      <PageLoadError />
    {/await}
    {/if}
  {:else if authMode === 'token' && needsAuth}
    {#await pages.SignIn()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page onattempt={attemptSignIn} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if !authMode}
    <PageHeader group="Connection problem" title="Your workspace is out of reach." />
    <p class="notice bad" role="alert">Could not check how this instance signs in. Reload the page to try again.</p>
  {:else if isHosting}
    {#await pages.Hosting()}
      <RouteSkeleton variant="form" />
    {:then { default: Page }}
      <Page {canWrite} reports={machineReports} snapshot={syncSnapshot} onsync={syncInPlace ? syncAll : undefined} syncing={syncingAll} {timeZone} {dateFormat} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isShared}
    {#await pages.SharedLists()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page userId={currentUser?.id ?? 0} {canWrite} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isActivity && authMode !== 'accounts'}
    {#await pages.ActivityPage()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page accounts={false} writer={sessionsAccess} onsignedout={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isHome}
    {#await pages.Home()}
      <RouteSkeleton variant={homeLayout && sessionsAccess ? 'home' : 'cards'} />
    {:then { default: Page }}
      <Page userId={currentUser?.id ?? 0} name={currentUser?.name ?? ''} writer={sessionsAccess} canAdd={canWrite} accounts={authMode === 'accounts'} moved={!!retiredProjects} {legacyProject} {timeZone} {dateFormat} onsignedout={fail} onattention={(entries, seen, answered) => { waitingEntries = entries; onMarkSeen = seen; onAnswered = answered }} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isFactory && sessionsAccess && authMode === 'accounts'}
    {#await pages.Factory()}
      <RouteSkeleton variant="cards" />
    {:then { default: Page }}
      <Page userId={currentUser?.id ?? 0} {timeZone} {dateFormat} onsignedout={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isDesks && sessionsAccess && authMode === 'accounts'}
    {#await pages.Desks()}
      <RouteSkeleton variant={deskOpen ? 'workspace' : 'cards'} />
    {:then { default: Page }}
      <Page userId={currentUser?.id ?? 0} onsignedout={fail} onscreen={open => (deskOpen = open)} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isSessions && sessionsAccess}
    {#await pages.SessionsPage()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page userId={currentUser?.id ?? 0} onsignedout={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isMachines}
    {#await pages.Machines()}
      <RouteSkeleton variant={terminalLayout ? 'workspace' : 'rows'} />
    {:then { default: Page }}
      <Page userId={currentUser?.id??0} onsignedout={fail} {canWrite} {timeZone} {dateFormat} monitoring={authMode==='accounts'||authMode==='token'} accounts={authMode === 'accounts'} compare={isSync || (location.pathname === '/machines' && new URLSearchParams(location.search).get('view') === 'compare') || location.pathname === '/account/sync'} advanced={isSyncAdvanced} ontitle={title => (workspaceTitle = title)} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isLibrary}
    {#await pages.Library()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page {canWrite} {timeZone} onerror={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isProfiles}
    {#await pages.SetupProfiles()}
      <RouteSkeleton variant="cards" />
    {:then { default: Page }}
      <Page {canWrite} {timeZone} {dateFormat} onerror={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isPrompts}
    {#if authMode !== 'accounts'}
      <PageHeader group="Skills & tools" title="Prompts need an account." />
      <p class="notice bad" role="status">Sign in with an account to keep prompts and use them in a session.</p>
    {:else if !canWrite}
      <PageHeader group="Skills & tools" title="Prompts need a writer account." />
      <p class="notice bad" role="status">Your role can read the workspace, but prompts are saved with a writer's account.</p>
    {:else}
      {#await pages.Prompts()}
        <RouteSkeleton variant="rows" />
      {:then { default: Page }}
        <Page {canWrite} />
      {:catch}
        <PageLoadError />
      {/await}
    {/if}
  {:else if isUsage}
    {#if authMode !== 'accounts'}
      <PageHeader group="Workspace" title="Usage needs an account." />
      <p class="notice bad" role="status">Sign in with an account to see what your agents have used.</p>
    {:else if !canWrite}
      <PageHeader group="Workspace" title="Usage needs a writer account." />
      <p class="notice bad" role="status">Your role can read the workspace, but usage comes from sessions, which a writer's account starts.</p>
    {:else}
      {#await pages.Usage()}
        <RouteSkeleton variant="cards" />
      {:then { default: Page }}
        <Page userId={currentUser?.id ?? 0} {timeZone} {dateFormat} onsignedout={fail} />
      {:catch}
        <PageLoadError />
      {/await}
    {/if}
  {:else if isNotifications}
    {#await pages.Notifications()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page onchange={loadNotificationCount} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isSupport}
    {#await pages.Support()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isTicket}
    {#await pages.Ticket()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page id={supportTicketID} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isSpaces}
    {#await pages.Spaces()}
      <RouteSkeleton variant="cards" />
    {:then { default: Page }}
      <Page {canWrite} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isMCPConsent && authMode === 'accounts'}
    {#await pages.MCPConsent()}
      <RouteSkeleton variant="auth" />
    {:then { default: Page }}
      <Page />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if (isAccount || isActivity) && authMode === 'accounts'}
    {#await pages.Account()}
      <RouteSkeleton variant="form" />
    {:then { default: Page }}
      <Page user={currentUser!} onsignout={signOut} onerror={fail} onuserchange={(u) => (currentUser = u)} writer={canWrite} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isList}
    {#await pages.ListView()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page id={listId} {canWrite} userRole={role} onerror={fail} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isAccount}
    <PageHeader group="Your account" title="Accounts are not enabled." />
    <p class="notice bad" role="alert">Accounts are not enabled on this instance.</p>
  {:else if isAdmin && !isAdminUser}
    <PageHeader group="Instance administration" title="Admin access is needed." />
    <p class="notice bad" role="alert">Admins only. Your account does not have access to this page.</p>
  {:else if isAdmin}
    {#await pages.Admin()}
      <RouteSkeleton variant="cards" />
    {:then { default: Page }}
      <Page onerror={fail} {role} accounts={authMode === 'accounts'} {supportDesk} currentEmail={currentUser?.email ?? ''} />
    {:catch}
      <PageLoadError />
    {/await}
  {:else if isSkills}
    {#await pages.Skills()}
      <RouteSkeleton variant="rows" />
    {:then { default: Page }}
      <Page {canWrite} />
      {#if canWrite}
        <Tools onchange={toolsChanged} {ok} fail={toolsFail} {token} canManageTags={isAdminUser} filters={skillsExport} />
      {/if}
    {:catch}
      <PageLoadError />
    {/await}
  {:else}
    <NotFound />
  {/if}
</main>

{#if authMode === 'accounts' && currentUser && !needsAuth && !isMCPConsent}<WebMCP />{/if}
{#if authMode === 'accounts' && currentUser && canWrite && !needsAuth && !isMCPConsent && !isPublicList && !isCommunity && !isJoin && !isReset && !setupRequired && !deskScreen}
  {#key currentUser.id}<GafferChatWidget userId={currentUser.id} onerror={fail} {timeZone} {dateFormat} />{/key}
{/if}
{#if isHosting && authenticated && !needsAuth}<SyncActivity open={syncWatch !== null} context={syncWatch} {canWrite} onclose={() => (syncWatch = null)} onupdate={syncUpdated} />{/if}
{#if !terminalLayout && !homeLayout && !desksLayout}<Footer
  signedIn={!isPublicList && authMode !== 'none' && authenticated && !needsAuth}
  showAdmin={!isPublicList && authenticated && !needsAuth && isAdminUser}
  userName={currentUser?.name ?? ''}
  onsignout={signOut}
/>{/if}
{#if toast && !needsAuth}
  {#key toast.id}
    <Toast
      message={toast.message}
      tone={toast.tone}
      actionLabel={toast.actionLabel}
      onaction={toast.onaction}
      ondismiss={dismissToast}
    />
  {/key}
{/if}

<style>
  .skip { position: absolute; left: 8px; top: -48px; background: var(--accent); color: var(--accent-ink); padding: 8px 12px; border-radius: 8px; z-index: 10; }
  .skip:focus { top: 8px; }
  main { width: 100%; max-width: var(--content-width); margin: 0 auto; padding: 36px 40px 56px; flex: 1; }
  main.auth-page { max-width: 1150px; padding-top: clamp(32px, 7vh, 88px); }
  @media (max-width: 760px) { main.auth-page { padding: 24px 20px 36px; } }
  main.home-layout { max-width: none; padding: 0; }
  main.desks-layout { max-width: none; padding: 0; display: flex; flex-direction: column; }
  main:focus { outline: none; }
  .site-header { border-bottom: 1px solid var(--line-soft); }
  .header-inner { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 24px; max-width: var(--content-width); margin: auto; padding: 24px 40px; }
  .site-nav { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .site-nav a, .entry-link { display: inline-flex; align-items: center; gap: 4px; min-height: 40px; padding: 8px 12px; color: var(--muted); font-size: 0.875rem; font-weight: 600; text-decoration: none; border-radius: 8px; }
  .site-nav a:hover, .entry-link:hover { color: var(--ink); background: var(--card); }
  .home { display: inline-block; color: inherit; text-decoration: none; }
  .tag { color: var(--muted); font-size: 0.875rem; font-weight: 500; letter-spacing: 0.01em; }

  .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .bad { border-color: var(--bad); }

  @media (max-width: 1000px) {
    main { padding: 24px 24px 40px; }
    .header-inner { display: flex; gap: 16px; padding: 16px 24px; }
    .site-nav { flex-wrap: nowrap; width: auto; margin-left: auto; justify-content: flex-end; min-width: 0; overflow-x: auto; padding-bottom: 8px; }
    .site-nav a { flex: none; justify-content: center; white-space: nowrap; border: 0; }
  }
  @media (max-width: 560px) {
    main { padding: 20px 16px 36px; }
    .header-inner { flex-wrap: wrap; gap: 12px; padding: 16px; }
    .tag { margin-left: auto; max-width: 26ch; text-align: right; white-space: nowrap; }
    /* The public links sit under the logo on a phone, at the right edge like the signed-in account menu. */
    .site-nav { width: 100%; margin-left: 0; flex-wrap: wrap; justify-content: flex-end; }
    .site-nav a { flex: none; justify-content: center; border: 1px solid var(--line); }
  }
  @media (pointer: coarse) { .site-nav a, .entry-link { min-height: 44px; } }
  main.terminal-layout { max-width: none; padding: 8px 12px; min-height: 0; flex: 1; }
  @media(max-width: 700px) { main.terminal-layout { padding: 4px 8px; } }
</style>
