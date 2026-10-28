<script lang="ts">
  import AppShell from './lib/shell/AppShell.svelte'
  import type { JumpItem, Tab } from './lib/shell/tabs'
  import ActivityPage from './lib/ActivityPage.svelte'
  import { listFolderProjects, sessionHref, terminalMachines, terminalSessions } from './lib/terminal/api'
  import SessionsPage from './lib/sessions/SessionsPage.svelte'
  import Machines from './lib/Machines.svelte'
  import Home from './lib/home/Home.svelte'
  import { groupLabel, installationReports, inventoryMachines } from './lib/machines'
  import Community from './lib/Community.svelte'
  import SharedLists from './lib/SharedLists.svelte'
  import Spaces from './lib/Spaces.svelte'
  import { audienceLabel } from './lib/api'
  import { onMount, tick } from 'svelte'
  import {
    addSkill, addSkillTags, applyTheme, clearToken, getAuthStatus, getNotificationCount, getProfile, getToken, listEnvironments, listEvents, listLists, listSkills, listTags,
    patchSkill, patchSkills, refreshSkill, removeSkill, removeSkills, resolveSkill, restoreAll, saveProfile, signOutAccount, tagSkills, UnauthorizedError,
    type Counts, type Environment, type Event, type Flags, type ListRef, type SharedList, type Skill, type Sort, type Tag, type User,
  } from './lib/api'
  import Account from './lib/Account.svelte'
  import AccountMenu from './lib/AccountMenu.svelte'
  import MCPConsent from './lib/MCPConsent.svelte'
  import WebMCP from './lib/WebMCP.svelte'
  import Hosting from './lib/Hosting.svelte'
  import AccountSignIn from './lib/AccountSignIn.svelte'
  import Admin from './lib/Admin.svelte'
  import BulkBar from './lib/BulkBar.svelte'
  import ConfirmDialog from './lib/ConfirmDialog.svelte'
  import CopyButton from './lib/CopyButton.svelte'
  import Footer from './lib/Footer.svelte'
  import { canAutoFocus } from './lib/focus'
  import Join from './lib/Join.svelte'
  import ListView from './lib/ListView.svelte'
  import Logo from './lib/Logo.svelte'
  import NewList from './lib/NewList.svelte'
  import NotFound from './lib/NotFound.svelte'
  import PublicList from './lib/PublicList.svelte'
  import ResetPassword from './lib/ResetPassword.svelte'
  import PageHeader from './lib/PageHeader.svelte'
  import Setup from './lib/Setup.svelte'
  import SetupProfiles from './lib/SetupProfiles.svelte'
  import SignIn from './lib/SignIn.svelte'
  import SignUp from './lib/SignUp.svelte'
  import Notifications from './lib/Notifications.svelte'
  import Support from './lib/Support.svelte'
  import Ticket from './lib/Ticket.svelte'
  import Welcome from './lib/Welcome.svelte'
  import SkillCard from './lib/SkillCard.svelte'
  import SyncActivity from './lib/SyncActivity.svelte'
  import { installSkillOnMachine } from './lib/syncActions'
  import Bookmarklets from './lib/Bookmarklets.svelte'
  import Toast from './lib/Toast.svelte'
  import { ssoErrorFromSearch, ssoErrorMessage, withoutSsoError, type SsoStatus } from './lib/sso'
  import Tools from './lib/Tools.svelte'
  import { copyText } from './lib/clipboard'
  import { installCommand } from './lib/install'
  import { stageOf, stages, stageTitle, type Stage } from './lib/shelf'
  import { isOldEnvironment } from './lib/time'

  let skills = $state<Skill[]>([])
  let counts = $state<Counts>({ all: 0, liked: 0, bookmarked: 0, used: 0 })
  let show = $state<'all' | Stage>('all')
  let onlyLiked = $state(false)
  let q = $state('')
  let sort = $state<Sort>('recent')
  let view = $state<'list' | 'board'>('list')
  let expandedSkills = $state<Record<string, boolean>>({})
  let defaultExpanded = $state(false)
  let viewSave = Promise.resolve()
  let addInput = $state('')
  let adding = $state(false)
  let addError = $state('')
  let message = $state('')
  let error = $state('')
  let loading = $state(true)
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
  let search = $state<HTMLInputElement>()
  let addEl = $state<HTMLInputElement>()
  type ToastState = { id: number; message: string; actionLabel?: string; onaction?: () => void | Promise<void> }
  let toast = $state<ToastState | null>(null)
  let toastId = 0
  let environments = $state<Environment[]>([])
  let allTags = $state<Tag[]>([])
  let lists = $state<SharedList[]>([])
  let linkEnabled = $state(true)
  let newListOpen = $state(false)
  let selectedTags = $state<number[]>([])
  let selectedEnv = $state('')
  let selectedMachine = $state('')
  let machines = $state<import('./lib/api').Machine[]>([])
  let inventoryNow = $state(Date.now())
  let installing = $state('')
  let syncWatch = $state<{ host: string; title: string; note: string } | null>(null)
  const shelfMachines = $derived(inventoryMachines(machines))
  const machineReports = $derived(installationReports(shelfMachines, inventoryNow))
  let onlyOne = $state(false)
  let copying = $state(false)
  let removePending = $state<Skill | null>(null)
  let metaLoaded = $state(false)
  let selecting = $state(false)
  let selected = $state<string[]>([])
  let lastPicked = $state<number | null>(null)
  let bulkBusy = $state(false)
  let bulkRemoveOpen = $state(false)
  let bulkRemoving = $state<Skill[]>([])

  const origin = location.origin
  // /projects and /projects/<id> are retired: folder projects live in Home's rail. The address becomes / at once (Home then
  // looks an old Project id up and shows /?project=<its folder project>), and Home says so once, for this page load only.
  const retiredProjects = location.pathname.match(/^\/projects(?:\/([^/]+))?\/?$/)
  // A malformed escape in an old link (/projects/%E0) still goes to Home, just without a project to look up.
  const legacyProject = (() => { try { return retiredProjects?.[1] ? decodeURIComponent(retiredProjects[1]) : '' } catch { return '' } })()
  if (retiredProjects) window.history.replaceState(window.history.state, '', '/')
  // Control is folded into Home and Machines (R4): /control goes to Home. The Sessions tab is /sessions (R17): /terminals goes
  // there keeping its query and fragment, so an old link to a session (?agent=…&session=…) still opens it.
  if (/^\/control\/?$/.test(location.pathname)) window.history.replaceState(window.history.state, '', '/')
  if (/^\/terminals\/?$/.test(location.pathname)) window.history.replaceState(window.history.state, '', `/sessions${location.search}${location.hash}`)
  // `/` is Home: the full Home for a writer in accounts mode, the fleet strip alone for everyone else signed in (R16).
  const isHome = location.pathname === '/'
  const isActivity = location.pathname === '/activity'
  const isSkills = location.pathname === '/skills'
  const machineView = new URLSearchParams(location.search).get('view')
  const legacyMachinePage = ['/machines', '/account/machines', '/account/sync'].includes(location.pathname)
  // /sessions lists the sessions; with a session (or a machine, or the workspace's own view=terminal) in the query it opens the
  // terminal workspace on it, as /terminals did, until the session screen replaces the workspace.
  const sessionsQuery = new URLSearchParams(location.search)
  const isSessionsPage = location.pathname === '/sessions'
  const isTerminals = isSessionsPage && (sessionsQuery.has('session') || sessionsQuery.has('agent') || machineView === 'terminal') || legacyMachinePage && machineView === 'terminal'
  const isSessions = isSessionsPage && !isTerminals
  const isSync = location.pathname === '/sync' || legacyMachinePage && !isTerminals && (machineView === 'compare' || location.pathname === '/account/sync')
  const isHosting = location.pathname === '/hosting'
  const isCommunity = location.pathname === '/community'
  const isShared = location.pathname === '/shared'
  const isSpaces = location.pathname === '/spaces'
  const isProfiles = ['/profiles', '/setups'].includes(location.pathname)
  // Without Sessions access (a viewer, a token or an open instance) /sessions shows Machines, as /terminals did.
  const isMachines = isSync || isTerminals || isSessions || location.pathname === '/machines' || ['/account/machines', '/account/sync'].includes(location.pathname)
  const isAdmin = location.pathname.startsWith('/admin')
  const isAccount = location.pathname.startsWith('/account')
  const isMCPConsent = location.pathname === '/account/mcp/authorize'
  const isJoin = location.pathname.startsWith('/join')
  const isReset = location.pathname === '/reset'
  const isSupport = location.pathname === '/support' || location.pathname === '/support/'
  const supportMatch = location.pathname.match(/^\/support\/(\d+)\/?$/)
  const isTicket = supportMatch !== null
  const isNotifications = location.pathname === '/notifications'
  const supportTicketID = supportMatch ? Number(supportMatch[1]) : 0
  const isWelcome = location.pathname === '/' || location.pathname === '/welcome'
  const isSignIn = location.pathname === '/signin'
  const isSignUp = location.pathname === '/signup'
  // Addresses that only make sense before sign-in. Finishing sign-in or setup on one of them continues at Control.
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
  const terminalLayout = $derived(isTerminals && authMode === 'accounts' && authenticated && canWrite && !needsAuth)
  const isAdminUser = $derived(authMode !== 'accounts' || role === 'owner' || role === 'admin')
  // Navigation (R13): five tabs on a desktop; Home, Sessions and Machines stay in view on a tablet or phone and the rest
  // move into More. Sessions and the bell are for a writer in accounts mode only (R16), as Terminals and Projects were.
  const sessionsAccess = $derived(authMode === 'accounts' && canWrite)
  // The bell's Needs you count, as Home reads it. Nothing signals that a session needs the person today, so it stays 0.
  let attention = $state(0)
  // Home spans the page: the project rail on the left and the docked Start an agent at the foot.
  const homeLayout = $derived(isHome && authChecked && !!authMode && authenticated && !needsAuth && !(authMode === 'accounts' && setupRequired))
  const tabs = $derived<Tab[]>([
    { href: '/', label: 'Home', current: isHome },
    ...(sessionsAccess ? [{ href: '/sessions', label: 'Sessions', current: isTerminals || isSessions }] : []),
    { href: '/machines', label: 'Machines', current: isMachines && !isSync && !isTerminals && !isSessions },
    { href: '/skills', label: 'Skills', current: isSkills },
    { href: '/activity', label: 'Activity', current: isActivity },
  ])
  const barTabs = $derived(tabs.filter(tab => ['/', '/sessions', '/machines'].includes(tab.href)))
  const more = $derived<Tab[]>([
    { href: '/profiles', label: 'Setups', current: isProfiles },
    { href: '/sync', label: 'Sync', current: isSync },
    { href: '/shared', label: 'Lists', current: isShared || isList },
    { href: '/community', label: 'Community', current: isCommunity },
    { href: '/support', label: 'Support', current: isSupport || isTicket },
    { href: '/spaces', label: 'Teams & organisations', current: isSpaces },
    ...(canWrite ? [{ href: '/hosting', label: 'Host skills', current: isHosting }] : []),
  ])
  // Jump searches what the workspace already lists: sessions, folder projects and machines (a writer's terminal machines,
  // and the machines on the shelf inventory for everyone). Read when Jump is used; a list that cannot be read is left out.
  async function jumpItems(): Promise<JumpItem[]> {
    const items: JumpItem[] = []
    const hosts = new Map<string, string>()
    if (sessionsAccess) {
      const [sessions, projects, terminal] = await Promise.allSettled([terminalSessions(), listFolderProjects(), terminalMachines()])
      if (terminal.status === 'fulfilled') for (const machine of terminal.value) hosts.set(machine.agentId, machine.host)
      if (sessions.status === 'fulfilled') for (const session of sessions.value) {
        const host = hosts.get(session.agentId)
        items.push({ kind: 'session', label: session.title, detail: host ? `Session · ${host}` : 'Session', href: sessionHref(session) })
      }
      if (projects.status === 'fulfilled') for (const project of projects.value) items.push({ kind: 'project', label: project.name, detail: project.machine ? `Project · ${project.machine}` : 'Project', href: `/?project=${encodeURIComponent(project.id)}` })
      if ([sessions, projects, terminal].every(result => result.status === 'rejected')) throw new Error('Nothing could be read.')
    }
    const names = new Set([...hosts.values(), ...shelfMachines.map(machine => machine.host)])
    for (const host of names) items.push({ kind: 'machine', label: host, detail: 'Machine', href: `/machines?machine=${encodeURIComponent(host)}` })
    return items
  }
  const scanCommand = $derived(`reet scan --prune --server ${origin}`)
  const suggestions = [
    { slug: 'anthropics/skills/pdf', name: 'PDF', blurb: 'Read, extract and combine PDF files.' },
    { slug: 'anthropics/skills/frontend-design', name: 'Frontend design', blurb: 'Layouts, type and UI polish for real interfaces.' },
    { slug: 'vercel-labs/skills/find-skills', name: 'Find skills', blurb: 'Find and install the right skill for a task.' },
  ]

  const filtering = $derived(q.trim() !== '' || onlyLiked || selectedTags.length > 0 || selectedEnv !== '' || selectedMachine !== '' || onlyOne)
  const grouped = $derived(stages.map((st) => ({ ...st, items: skills.filter((k) => stageOf(k) === st.id) })))
  // The board keeps empty columns as lanes. The list hides empty stages so the
  // first added skill appears immediately instead of below two dead sections.
  const orderedGroups = $derived(view === 'board' ? [grouped[2], grouped[1], grouped[0]] : grouped)
  const visible = $derived(orderedGroups.filter((g) => (show === 'all' || show === g.id) && ((view === 'board' && !filtering) || g.items.length > 0)))
  const total = $derived(visible.reduce((n, g) => n + g.items.length, 0))
  const anyExpanded = $derived(visible.some(g => g.items.some(skill => expandedSkills[skill.id] ?? defaultExpanded)))
  const likedCount = $derived(skills.filter((s) => s.liked).length)
  const tagOptions = $derived(allTags.map((t) => t.name))
  const filterKey = $derived(`${q}|${sort}|${onlyLiked}|${selectedTags.join(',')}|${selectedMachine}|${selectedEnv}|${onlyOne}|${show}`)
  const tagCounts = $derived.by(() => {
    const map = new Map<number, { tag: Tag; count: number }>()
    for (const skill of skills) {
      for (const tag of skill.tags ?? []) {
        const current = map.get(tag.id)
        if (current) current.count++
        else map.set(tag.id, { tag, count: 1 })
      }
    }
    for (const id of selectedTags) {
      if (!map.has(id)) {
        const tag = allTags.find((t) => t.id === id)
        if (tag) map.set(id, { tag, count: 0 })
      }
    }
    return [...map.values()].sort((a, b) => a.tag.name.localeCompare(b.tag.name))
  })

  let clearTimer: ReturnType<typeof setTimeout> | undefined
  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) {
      clearToken()
      token = ''
      if (authMode === 'accounts') currentUser = null
      needsAuth = true
      message = ''
      toast = null
      removePending = null
      return
    }
    error = e instanceof Error ? e.message : String(e)
    message = ''
  }
  function ok(m: string) {
    message = m
    error = ''
    clearTimeout(clearTimer)
    clearTimer = setTimeout(() => (message = ''), 6000)
  }
  function showToast(message: string, actionLabel?: string, onaction?: () => void | Promise<void>) {
    toast = { id: ++toastId, message, actionLabel, onaction }
  }
  function dismissToast() {
    toast = null
  }

  let loadVersion = 0

  async function load(): Promise<boolean> {
    const version = ++loadVersion
    try {
      const r = await listSkills({
        tab: onlyLiked ? 'liked' : 'all',
        q,
        sort,
        tags: selectedTags,
        env: selectedEnv, machine: selectedMachine,
        only: onlyOne ? 'one-machine' : '',
      })
      if (version !== loadVersion) return false
      skills = r.skills
      counts = r.counts
      error = ''
      needsAuth = false
      if (!metaLoaded) void loadMeta().then((ok) => (metaLoaded = ok))
      return true
    } catch (e) {
      if (version !== loadVersion) return false
      fail(e)
      return false
    } finally {
      if (version === loadVersion) loading = false
    }
  }

  async function loadMeta(): Promise<boolean> {
    try {
      const [envs, tags, shared] = await Promise.all([listEnvironments(), listTags(), listLists()])
      environments = envs.environments
      machines = envs.machines
      allTags = tags.tags
      lists = shared.lists
      linkEnabled = shared.linkEnabled
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  function openList(list: SharedList | ListRef) {
    location.assign(`/lists/${list.id}`)
  }

  async function reloadAll() {
    await load()
    metaLoaded = await loadMeta()
  }

  async function loadNotificationCount() {
    if (!authenticated) return
    try { unreadNotifications = (await getNotificationCount()).unread } catch { /* a count is not worth an error banner */ }
  }

  onMount(() => {
    try { view = localStorage.getItem('reet-shelf-view') === 'board' ? 'board' : 'list' } catch { /* use the default when storage is unavailable */ }
    void initAuth().then(() => { if (authenticated) void loadNotificationCount() })
    const changed = () => { if (authenticated) void reloadAll() }
    window.addEventListener('reet-shelf-changed', changed)
    const timer = setInterval(() => (inventoryNow = Date.now()), 60000)
    const notifications = setInterval(() => { if (authenticated) void loadNotificationCount() }, 60000)
    return () => { clearInterval(timer); clearInterval(notifications); window.removeEventListener('reet-shelf-changed', changed) }
  })

  function chooseView(next: 'list' | 'board') {
    view = next
    try { localStorage.setItem('reet-shelf-view', next) } catch { /* the choice still works for this visit */ }
    const actor = currentUser?.id
    if (authMode === 'accounts' && actor) {
      // Serialize rapid switches so an older request cannot overwrite the last choice.
      viewSave = viewSave.catch(() => {}).then(async () => {
        if (currentUser?.id === actor) await saveProfile({ shelfView: next })
      }).catch(e => { if (currentUser?.id === actor) fail(e) })
    }
  }

  async function initAuth() {
    try {
      const status = await getAuthStatus()
      authMode = status.mode
      setupRequired = status.setupRequired
      bootstrapRequired = status.bootstrapRequired
      signupEnabled = status.signupEnabled ?? false
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
        showToast(ssoProblem)
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
    } catch {
      authMode = ''
    } finally {
      authChecked = true
    }
  }

  async function loadPreferences() {
    try {
      const actor = currentUser?.id
      const { profile } = await getProfile()
      if (actor !== currentUser?.id) return
      applyTheme(profile.theme)
      view = profile.shelfView
      defaultExpanded = profile.cardsExpanded
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
    loading = true
    await load()
    await loadMeta()
    await loadNotificationCount()
    await tick()
    if (canAutoFocus()) addEl?.focus()
  }

  async function afterJoin(user: User) {
    await afterAuth(user)
    location.assign('/')
  }

  async function attemptSignIn(): Promise<string | null> {
    loading = true
    const accepted = await load()
    if (accepted) {
      token = getToken()
      message = ''
      await tick()
      if (canAutoFocus()) addEl?.focus()
      return null
    }
    if (needsAuth) return 'That token was not accepted.'
    return error || 'Could not reach the shelf.'
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
      applyTheme('system')
      needsAuth = true
      token = ''
      clearToken()
      message = ''
      error = ''
      toast = null
      removePending = null
      selecting = false
      selected = []
      expandedSkills = {}
      defaultExpanded = false
      lastPicked = null
      return
    }
    clearToken()
    token = ''
    message = ''
    error = ''
    toast = null
    removePending = null
    loading = true
    await load()
  }

  $effect(() => {
    void [onlyLiked, q, sort, selectedTags, selectedEnv, selectedMachine, onlyOne, canLoad]
    if (!canLoad) return
    const t = setTimeout(load, 150)
    return () => clearTimeout(t)
  })


  let lastFilterKey = ''
  $effect(() => {
    const key = filterKey
    if (key !== lastFilterKey) {
      lastFilterKey = key
      if (selecting) {
        selected = []
        lastPicked = null
      }
    }
  })

  // A moved skill is re-rendered in another section, so put keyboard focus back on the control that was used.
  async function refocus(id: string, act: string) {
    await tick()
    const card = document.querySelector(`[data-skill="${CSS.escape(id)}"]`)
    const el = card?.querySelector(`[data-act="${act}"]`) ?? card?.querySelector('[data-act="m0"]') ?? document.querySelector('.filters button[aria-pressed="true"]')
    ;(el as HTMLElement | null)?.focus()
  }

  async function flash(id: string) {
    await tick()
    const card = document.querySelector(`[data-skill="${CSS.escape(id)}"]`)
    if (!card) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    card.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'center' })
    card.classList.remove('flash')
    void (card as HTMLElement).offsetWidth
    card.classList.add('flash')
    setTimeout(() => card.classList.remove('flash'), 1600)
  }

  function friendlyAddError(e: unknown): string {
    const text = e instanceof Error ? e.message : String(e)
    if (text.toLowerCase().includes('expected a skills.sh')) {
      return "That doesn't look like a skill yet. Paste a skills.sh link like skills.sh/anthropics/skills/pdf, or an npx skills add command."
    }
    return text
  }

  async function addValue(value: string): Promise<boolean> {
    if (!value.trim() || adding) return false
    adding = true
    addError = ''
    try {
      const s = await addSkill(value.trim())
      addInput = ''
      show = stageOf(s)
      onlyLiked = false
      q = ''
      selectedTags = []
      selectedEnv = ''
      selectedMachine = ''
      onlyOne = false
      ok(s.created ? `Reet, added ${s.name} to the shelf.` : `Already on the shelf, in ${stageTitle(stageOf(s))}.`)
      await reloadAll()
      await flash(s.id)
      setTimeout(load, 2500) // pick up fetched metadata
      return true
    } catch (e) {
      if (e instanceof UnauthorizedError) {
        fail(e)
        return false
      }
      addError = friendlyAddError(e)
      await tick()
      addEl?.focus()
      addEl?.select()
      return false
    } finally {
      adding = false
    }
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    await addValue(addInput)
  }

  async function addSuggestion(slug: string) {
    await addValue(slug)
  }

  function focusAdd() {
    addEl?.scrollIntoView({ block: 'center', behavior: 'smooth' })
    addEl?.focus()
  }
  async function patch(s: Skill, flags: Flags, act: string): Promise<boolean> {
    try {
      const updated = await patchSkill(s.id, flags)
      const to = stageOf(updated)
      if (to !== stageOf(s)) ok(`Sorted. ${s.name} is now ${stageTitle(to)}.`)
      await reloadAll()
      await refocus(s.id, act)
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
  async function refreshOne(s: Skill) {
    try {
      const r = await refreshSkill(s.id)
      if (!r.refreshed) fail(new Error(`Could not refresh ${s.name}.`))
      else if (!r.changed) ok(`No change for ${s.name}.`)
      else ok(`Sorted. Refreshed ${s.name}.`)
      await reloadAll()
    } catch (e) {
      fail(e)
    }
  }
  function envTitle(env: Environment): string {
    const when = new Date(env.lastSeen).toLocaleString()
    return `${env.path ? env.path + ' · ' : ''}last seen ${when}`
  }

  function toggleSelecting() {
    selecting = !selecting
    if (!selecting) {
      selected = []
      lastPicked = null
    }
  }

  function select(skill: Skill, e: MouseEvent) {
    const flat = visible.flatMap((g) => g.items)
    const index = flat.findIndex((s) => s.id === skill.id)
    if (index < 0) return
    if (e.shiftKey && lastPicked !== null) {
      const [from, to] = [lastPicked, index].sort((a, b) => a - b)
      const range = flat.slice(from, to + 1).map((s) => s.id)
      const all = range.every((id) => selected.includes(id))
      selected = all ? selected.filter((id) => !range.includes(id)) : [...new Set([...selected, ...range])]
    } else {
      selected = selected.includes(skill.id) ? selected.filter((id) => id !== skill.id) : [...selected, skill.id]
    }
    lastPicked = index
  }

  async function bulkPatch(flags: Flags) {
    if (!selected.length) return
    bulkBusy = true
    try {
      await patchSkills(selected, flags)
      selecting = false
      selected = []
      lastPicked = null
      await reloadAll()
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function bulkStage(stage: 'use' | 'try' | 'shelf') {
    void bulkPatch(stage === 'use' ? { used: true } : stage === 'try' ? { used: false, bookmarked: true } : { used: false, bookmarked: false })
  }

  async function bulkTag(name: string) {
    if (!selected.length) return
    bulkBusy = true
    try {
      await tagSkills(selected, [name], [])
      selecting = false
      selected = []
      lastPicked = null
      await reloadAll()
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function bulkRemove() {
    if (!selected.length) return
    bulkRemoving = skills.filter((s) => selected.includes(s.id))
    bulkRemoveOpen = true
  }

  async function confirmBulkRemove() {
    const removed = bulkRemoving
    bulkRemoveOpen = false
    if (!removed.length) return
    bulkBusy = true
    try {
      await removeSkills(removed.map((s) => s.id))
      selecting = false
      selected = []
      lastPicked = null
      bulkRemoving = []
      await reloadAll()
      showToast(removed.length === 1 ? `Removed ${removed[0].name}` : `Removed ${removed.length} skills`, 'Undo', async () => {
        try {
          await restoreAll({ version: 2, exportedAt: Date.now(), skills: removed })
          await reloadAll()
          ok(removed.length === 1 ? `Restored ${removed[0].name}.` : `Restored ${removed.length} skills.`)
        } catch (e) {
          fail(e)
        }
      })
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function toggleTag(id: number) {
    selectedTags = selectedTags.includes(id) ? selectedTags.filter((x) => x !== id) : [...selectedTags, id]
  }
  function toggleEnv(id: string) {
    selectedEnv = selectedEnv === id ? '' : id
  }
  function filterTag(tag: Tag) {
    toggleTag(tag.id)
  }
  function filterEnv(env: Environment) {
    selectedMachine = env.host; toggleEnv(env.id)
  }

  async function installNow(skill: Skill, host: string) {
    if (!canWrite || installing) return
    const key = `${skill.id}:${host}`
    installing = key
    try {
      const result = await installSkillOnMachine(skill, host)
      syncWatch = { host: result.host, title: `Installing ${skill.name}`, note: `on ${result.host} · live progress` }
      ok(`Installing ${skill.name} on ${result.host}. Follow the live progress in the corner.`)
      void loadMeta()
    } catch (e) {
      fail(e)
    } finally {
      installing = ''
    }
  }

  async function copyMissing() {
    if (!selectedEnv && !selectedMachine) return
    copying = true
    try {
      const all = await listSkills({ tab: 'all', q: '', sort: 'recent' })
      const missing = all.skills.filter((s) => {
        const command = installCommand(s)
        return command !== null && !(s.environments ?? []).some((e) => (selectedEnv ? e.id === selectedEnv : e.host === selectedMachine))
      })
      const text = missing.map((s) => installCommand(s)).filter((x): x is string => Boolean(x)).join('\n')
      if (!text) {
        ok('Everything on the shelf is already installed there.')
        return
      }
      if (await copyText(text)) {
        ok(`Copied ${missing.length} install ${missing.length === 1 ? 'command' : 'commands'}.`)
      } else {
        fail(new Error('Could not copy to the clipboard.'))
      }
    } catch (e) {
      fail(e)
    } finally {
      copying = false
    }
  }

  async function tags(s: Skill, add: string[], remove: number[]): Promise<boolean> {
    try {
      const updated = await addSkillTags(s.id, add, remove)
      // Older list requests must not overwrite the tag mutation response.
      ++loadVersion
      skills = skills.map((skill) => skill.id === updated.id ? updated : skill)
      await reloadAll()
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  async function history(s: Skill): Promise<Event[]> {
    try {
      return (await listEvents(20, s.id)).events
    } catch (e) {
      fail(e)
      return []
    }
  }

  function remove(s: Skill) {
    if (s.sourceType === 'local' || (s.environments?.length ?? 0) > 0) {
      removePending = s
      return
    }
    void doRemove(s, false)
  }

  async function doRemove(s: Skill, ignore: boolean) {
    try {
      await removeSkill(s.id, ignore)
      await reloadAll()
      search?.focus()
      showToast(ignore ? `Removed ${s.name} and ignored future scans` : `Removed ${s.name}`, 'Undo', async () => {
        try {
          await restoreAll({ version: 2, exportedAt: Date.now(), skills: [s] })
          await reloadAll()
          ok(`Restored ${s.name}.`)
        } catch (e) {
          fail(e)
        }
      })
    } catch (e) {
      fail(e)
    }
  }
  async function link(s: Skill, input: string): Promise<boolean> {
    try {
      const r = await resolveSkill(s.id, input)
      ok(`Linked to ${r.id}`)
      await reloadAll()
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
</script>

<svelte:head>
  {#if !isPublicList}
    <title>{isCommunity ? 'Public community' : isShared ? 'Your lists' : isSpaces ? 'Teams & organisations' : isJoin ? 'Join reet.sh' : isReset ? 'Choose a new password' : isWelcome && !currentUser && authMode === 'accounts' ? 'Your AI fleet control room' : authMode === 'accounts' && setupRequired ? 'Set up reet.sh' : isSignUp ? 'Create your account' : authChecked && !authenticated ? 'Sign in' : isHosting ? 'Host skills' : isNotifications ? 'Notifications' : isSupport ? 'Support' : isTicket ? 'Support ticket' : isAdmin ? 'Admin' : isHome ? 'Home' : isActivity ? 'Activity' : isSync ? 'Sync' : isTerminals || isSessions ? 'Sessions' : isMachines ? 'Machines' : isAccount ? 'Your account' : isProfiles ? 'Setups' : isList ? 'Shared list' : isSkills ? 'Skills' : 'Page not found'} · reet.sh</title>
  {/if}
</svelte:head>

<a class="skip" href="#main-content">Skip to content</a>
<header class="site-header" class:terminal-chrome={terminalLayout}>
  {#if authenticated && !needsAuth && !isPublicList && !isJoin}
    <AppShell {tabs} bar={barTabs} {more} {attention} compact={terminalLayout} bell={sessionsAccess} moreInAccount={authMode !== 'none'} jump={jumpItems}>
      {#snippet account()}
        {#if authMode !== 'none'}
          <AccountMenu
            name={currentUser?.name ?? ''}
            email={currentUser?.email ?? ''}
            admin={isAdminUser}
            accounts={authMode === 'accounts'}
            active={isAccount || isAdmin || isNotifications || more.some(item => item.current)}
            unread={unreadNotifications}
            {more}
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
    {#if !authenticated && authMode==='accounts' && !isPublicList && !isCommunity && !isJoin}
      <nav class="site-nav" aria-label="Public navigation">{#if !isWelcome}<a href="/welcome">Workspace</a>{/if}<a href="/welcome#self-host">Self-host</a>{#if !setupRequired}<a class="entry-link" href="/signin">Sign in →</a>{/if}</nav>
    {:else if isPublicList || isCommunity || isJoin}
      <nav class="site-nav" aria-label="Public navigation"><a href="/community">Public community</a><a class="entry-link" href="/">Open your workspace →</a></nav>
    {/if}
    {#if (!authenticated || needsAuth) && authMode !== 'accounts' && !isPublicList && !isCommunity && !isJoin && !isWelcome}<span class="tag">Your AI workspace</span>{/if}
  </div>
  {/if}
</header>
<main id="main-content" tabindex="-1" class:terminal-layout={terminalLayout} class:home-layout={homeLayout} class:auth-page={!isPublicList && !isCommunity && !(isWelcome && !currentUser) && (!authenticated || needsAuth || isJoin)}>
  {#if isCommunity}
    <Community canSave={authenticated && !needsAuth && canWrite} />
  {:else if isPublicList}
    <PublicList slug={publicSlug} />
  {:else if isJoin}
    <Join token={joinToken} onsuccess={afterJoin} />
  {:else if isReset}
    <ResetPassword />
  {:else if !authChecked}
    <p class="empty" role="status">Loading…</p>
  {:else if authMode === 'accounts' && setupRequired && !isWelcome}
    <Setup {bootstrapRequired} onsuccess={afterAuth} />
  {:else if authMode === 'accounts' && !currentUser}
    {#if isWelcome}
      <Welcome {signupEnabled} {contactEmail} {setupRequired} />
    {:else if isSignUp && signupEnabled}
      <SignUp onsuccess={afterAuth} />
    {:else}
      <AccountSignIn onsuccess={afterAuth} {signupEnabled} {contactEmail} {passwordReset} {sso} />
    {/if}
  {:else if authMode === 'token' && needsAuth}
    <SignIn onattempt={attemptSignIn} />
  {:else if !authMode}
    <PageHeader eyebrow="Connection problem" title="Your workspace is out of reach." />
    <p class="notice bad" role="alert">Could not check how this instance signs in. Reload the page to try again.</p>
  {:else if isHosting}
    <Hosting {canWrite} />
  {:else if isShared}
    <SharedLists userId={currentUser?.id ?? 0} {canWrite} />
  {:else if isActivity}
    <ActivityPage accounts={authMode === 'accounts'} writer={sessionsAccess} onsignedout={fail} />
  {:else if isHome}
    <Home userId={currentUser?.id ?? 0} writer={sessionsAccess} canAdd={canWrite} accounts={authMode === 'accounts'} moved={!!retiredProjects} {legacyProject} onsignedout={fail} onattention={count => (attention = count)} />
  {:else if isSessions && sessionsAccess}
    <SessionsPage userId={currentUser?.id ?? 0} onsignedout={fail} />
  {:else if isMachines}
    <Machines userId={currentUser?.id??0} onsignedout={fail} {canWrite} monitoring={authMode==='accounts'||authMode==='token'} accounts={authMode === 'accounts'} compare={isSync || (location.pathname === '/machines' && new URLSearchParams(location.search).get('view') === 'compare') || location.pathname === '/account/sync'} />
  {:else if isProfiles}
    <SetupProfiles {canWrite} onerror={fail} />
  {:else if isNotifications}
    <Notifications onchange={loadNotificationCount} />
  {:else if isSupport}
    <Support />
  {:else if isTicket}
    <Ticket id={supportTicketID} />
  {:else if isSpaces}
    <Spaces {canWrite} />
  {:else if isMCPConsent && authMode === 'accounts'}
    <MCPConsent />
  {:else if isAccount && authMode === 'accounts'}
    <Account user={currentUser!} onsignout={signOut} onerror={fail} onuserchange={(u) => (currentUser = u)} />
  {:else if isList}
    <ListView id={listId} {canWrite} userRole={role} onerror={fail} />
  {:else if isAccount}
    <PageHeader eyebrow="Your account" title="Accounts are not enabled." />
    <p class="notice bad" role="alert">Accounts are not enabled on this instance.</p>
  {:else if isAdmin && !isAdminUser}
    <PageHeader eyebrow="Instance administration" title="Admin access is needed." />
    <p class="notice bad" role="alert">Admins only. Your account does not have access to this page.</p>
  {:else if isAdmin}
    <Admin onerror={fail} {role} accounts={authMode === 'accounts'} currentEmail={currentUser?.email ?? ''} />
  {:else if isSkills}
    <PageHeader eyebrow="Ey up. Your skill shelf." title="Skills." lede="Keep the good finds here, then choose which machines receive them. Shelf stages organise your library; Sync controls installation.">
      {#snippet actions()}{#if loading || counts.all > 0}<a class="btn" href="/sync">Sync skills</a>{/if}{#if canWrite}<a class="btn" href="/hosting">Create a skill</a>{/if}{/snippet}
    </PageHeader>
    {#if canWrite}
      <form class="add" onsubmit={add} aria-busy={adding}>
        <label class="vh" for="add-input">Skill install command, URL or slug</label>
        <input id="add-input" bind:this={addEl} placeholder="Paste a skills.sh link or install command" autocomplete="off" bind:value={addInput}
          aria-invalid={addError ? 'true' : undefined}
          aria-describedby={addError ? 'add-hint add-error' : 'add-hint'} />
        <button class="btn primary" type="submit" disabled={adding || !addInput.trim()}>{adding ? 'Adding…' : 'Add to shelf'}</button>
      </form>
      <p class="add-hint" id="add-hint">Paste a link like <code>skills.sh/anthropics/skills/pdf</code>, or an <code>npx skills add</code> command.</p>
      {#if addError}<p class="add-error" id="add-error" role="alert">{addError}</p>{/if}
    {/if}

    <div class="notices">
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
      <p class="notice good" role="status" hidden={!message}>{message}</p>
    </div>

    {#if counts.all > 0 || filtering}
      <div class="toolbar" role="group" aria-label="Filter and sort">
        <div class="filters">
          <button type="button" aria-pressed={show === 'all'} onclick={() => (show = 'all')}>All<small>{skills.length}</small></button>
          {#each grouped as g}
            <button type="button" aria-pressed={show === g.id} onclick={() => (show = g.id)}>{g.title}<small>{g.items.length}</small></button>
          {/each}
          <span class="sep" aria-hidden="true"></span>
          <button type="button" class="liked" aria-pressed={onlyLiked} onclick={() => (onlyLiked = !onlyLiked)}>♥ Liked<small>{likedCount}</small></button>
          {#if canWrite}
            <span class="sep" aria-hidden="true"></span>
            <button type="button" aria-pressed={selecting} onclick={toggleSelecting}>Select</button>
          {/if}
        </div>
        <div class="controls">
          <div class="view-switch" role="group" aria-label="Shelf layout">
            <button type="button" aria-pressed={view === 'list'} onclick={() => chooseView('list')}>List</button>
            <button type="button" aria-pressed={view === 'board'} onclick={() => chooseView('board')}>Board</button>
          </div>
          <label class="vh" for="search">Search skills</label>
          <input id="search" type="search" placeholder="Search" bind:value={q} bind:this={search} />
          <label class="vh" for="sort">Sort by</label>
          <span class="select">
            <select id="sort" bind:value={sort}>
              <option value="recent">Recent</option>
              <option value="name">Name</option>
              <option value="installs">Installs</option>
            </select>
          </span>
        </div>
      </div>
    {/if}

    {#if tagCounts.length > 0 || shelfMachines.length > 0 || lists.length > 0 || filtering}
    <details class="shelf-filters">
      <summary>Filters & lists <span>{selectedTags.length + Number(!!selectedMachine) + Number(!!selectedEnv) + Number(onlyOne) ? `${selectedTags.length + Number(!!selectedMachine) + Number(!!selectedEnv) + Number(onlyOne)} active` : `${tagCounts.length} ${tagCounts.length === 1 ? 'tag' : 'tags'} · ${shelfMachines.length} ${shelfMachines.length === 1 ? 'environment' : 'environments'}`}</span></summary>
      <div class="filter-panel">
    {#if lists.length > 0 || canWrite}
      <div class="filterbar" role="group" aria-label="Shared lists">
        <span class="bar-label">Your lists</span>
        <a class="chip" href="/shared">Browse & manage lists</a>
        <a class="chip" href="/community">Public community ↗</a>
        {#each lists.filter(l => l.ownerId === currentUser?.id || (authMode !== 'accounts' && l.role === 'owner')) as list (list.id)}
          <button type="button" class="chip" onclick={() => openList(list)} title={`${list.skillCount} skills · ${list.visibility}`}>
            {list.name}<small>{audienceLabel(list.visibility)} · {list.skillCount}</small>
          </button>
        {/each}
        {#if canWrite}
          <button type="button" class="chip action" onclick={() => (newListOpen = true)}>+ New list</button>
        {/if}
      </div>
    {/if}

    {#if tagCounts.length > 0 || selectedTags.length > 0}
      <div class="filterbar" role="group" aria-label="Tags">
        <span class="bar-label">Tags</span>
        {#each tagCounts as item (item.tag.id)}
          <button type="button" class="chip" aria-pressed={selectedTags.includes(item.tag.id)} onclick={() => toggleTag(item.tag.id)}>
            {item.tag.name}<small>{item.count}</small>
          </button>
        {/each}
        {#if selectedTags.length > 0}
          <button type="button" class="chip clear" onclick={() => (selectedTags = [])}>Clear</button>
        {/if}
      </div>
    {/if}

    {#if shelfMachines.length > 0 || selectedEnv !== '' || selectedMachine !== '' || onlyOne}
      <div class="filterbar" role="group" aria-label="Environments">
        <span class="bar-label">Environments</span>
        {#each shelfMachines as machine (machine.host)}
          <button type="button" class="chip" aria-pressed={selectedMachine === machine.host}
            onclick={() => { selectedMachine = selectedMachine === machine.host ? '' : machine.host; selectedEnv = '' }}
            title={`${machine.groups.filter(g => !g.agent.startsWith('reet')).length} agent groups · ${machine.skillCount} unique skills`}>
            {machine.host}<small>{machine.skillCount}</small>
          </button>
        {/each}
        <span class="sep" aria-hidden="true"></span>
        <button type="button" class="chip" aria-pressed={onlyOne} onclick={() => (onlyOne = !onlyOne)}>Only on one machine</button>
        {#if selectedMachine}
          <button type="button" class="chip action" onclick={copyMissing} disabled={copying}>
            {copying ? 'Copying…' : 'Copy missing installs'}
          </button>
        {/if}
      </div>
      {#if selectedMachine}
        <div class="filterbar" role="group" aria-label="Agent groups on selected machine">
          <span class="bar-label">Agent groups</span>
          <button type="button" class="chip" aria-pressed={!selectedEnv} onclick={() => selectedEnv = ''}>All agents</button>
          {#each environments.filter(e => e.host === selectedMachine && !e.agent.startsWith('reet')) as env (env.id)}
            <button type="button" class="chip" class:old={isOldEnvironment(env.lastSeen)} aria-pressed={selectedEnv === env.id} onclick={() => toggleEnv(env.id)} title={envTitle(env)}>{groupLabel(env)}<small>{env.skillCount ?? 0}</small></button>
          {/each}
        </div>
      {/if}
    {/if}

      </div>
    </details>
    {/if}

    {#if !loading && (counts.all > 0 || filtering)}
      <div class="shelf-summary">
        <p class="count" role="status">{total === 1 ? '1 skill' : `${total} skills`}</p>
        {#if total > 0}<button type="button" class="btn quiet" onclick={() => { expandedSkills = { ...expandedSkills, ...Object.fromEntries(visible.flatMap(g => g.items.map(skill => [skill.id, !anyExpanded]))) } }}>{anyExpanded ? 'Collapse all' : 'Expand all'}</button>{/if}
      </div>
    {/if}


    <div id="sections" class:board={view === 'board' && !loading && counts.all > 0 && total > 0} class:single-stage={show !== 'all'}>
      {#if loading}
        <div class="shelf-skeleton" role="status" aria-busy="true" aria-label="Loading your shelf">
          <span class="vh">Loading your shelf…</span>
          {#each [0, 1, 2] as n (n)}<div class="skeleton-card" data-skeleton aria-hidden="true"><i></i><i></i><i></i></div>{/each}
        </div>
      {:else if counts.all === 0 && !filtering && !canWrite}
        <section class="start" aria-labelledby="start-title">
          <p class="eyebrow">Read-only</p>
          <h2 id="start-title">Your account can browse, but not edit the shelf.</h2>
          <p class="lede">Ask the owner for a shared list or a member role.</p>
        </section>
      {:else if counts.all === 0 && !filtering}
        <section class="start" aria-labelledby="start-title">
          <p class="eyebrow">Ey up. Start here.</p>
          <h2 id="start-title">Put the first skill on your shelf.</h2>
          <p class="lede">Capture it now, sort it later. Everything stays on this instance.</p>
          <button class="btn primary" type="button" onclick={focusAdd}>Add your first skill</button>
          <ul class="suggestions" aria-label="Good skills to start with">
            {#each suggestions as item (item.slug)}
              <li>
                <div><strong>{item.name}</strong><code>{item.slug}</code><span>{item.blurb}</span></div>
                <button class="btn" type="button" onclick={() => addSuggestion(item.slug)} disabled={adding}>{adding ? 'Adding…' : 'Add'}</button>
              </li>
            {/each}
          </ul>
          <div class="start-options">
            {#if authMode === 'accounts'}<a class="btn" href="/machines#add-machine" title="Install the reet CLI and scan your agent folders.">Connect this machine</a>{/if}
            <a class="btn" href="/community">Explore community</a>
            <a href="https://www.skills.sh/topic" target="_blank" rel="noopener noreferrer">Find a skill on skills.sh<span class="vh"> (opens in a new tab)</span></a>
          </div>
          {#if authMode === 'accounts'}<p class="start-note">Connecting a machine takes about 2 minutes and needs the reet CLI.</p>{/if}
          <details class="save-anywhere">
            <summary>Save skills from skills.sh with a bookmarklet</summary>
            <p>Drag one of these to your bookmarks bar, then click it on any skills.sh skill page.</p>
            <Bookmarklets {token} />
          </details>
          {#if authMode !== 'accounts'}
            <details class="scan-help"><summary>Already have skills installed?</summary><p>Run this on your machine. It reports installed skills as in use.</p><div class="command"><code>{scanCommand}</code><CopyButton text={scanCommand} compact /></div></details>
          {/if}
        </section>
      {:else if total === 0}
        <div class="no-matches">
          <h2>Nowt matches just yet.</h2>
          <p>Try another search or clear your filters.</p>
          <button type="button" class="btn" onclick={() => { q = ''; show = 'all'; onlyLiked = false; selectedTags = []; selectedEnv = ''; selectedMachine = ''; onlyOne = false }}>Clear filters</button>
        </div>
      {:else}
        {#each visible as g (g.id)}
          <section class={g.id} class:empty-stage={g.items.length === 0} aria-labelledby="h-{g.id}">
            <h2 id="h-{g.id}">{g.title}<small>{g.items.length}</small></h2>
            <p class="hint">{g.hint}</p>
            <ul>
              {#each g.items as s (s.id)}
                <SkillCard
                skill={s} stage={g.id} tagOptions={tagOptions} compact={view === 'board'}
                expanded={expandedSkills[s.id] ?? defaultExpanded} onexpand={expanded => (expandedSkills[s.id] = expanded)}
                installationReports={machineReports}
                selecting={selecting} selectedIDs={selected} onselect={select} {canWrite}
                oninstall={(s, host) => void installNow(s, host)} {installing}
                onpatch={patch} onrefresh={refreshOne} onlink={link} onremove={remove}
                onfiltertag={filterTag} onfilterenv={filterEnv} ontags={tags} onhistory={history}
                onopenlist={openList}
              />
              {:else}
                <li class="none">{filtering ? 'Nowt matches these filters in this stage.' : g.empty}</li>
              {/each}
            </ul>
          </section>
        {/each}
      {/if}
    </div>

    {#if !loading && canWrite}
      {#key counts.all === 0}
        <Tools
          onchange={reloadAll} {ok} {fail} {token} canManageTags={isAdminUser}
          filters={{ tab: onlyLiked ? 'liked' : 'all', q, sort, tags: selectedTags, env: selectedEnv, machine: selectedMachine, only: onlyOne ? 'one-machine' : '', stage: show === 'all' ? '' : show }}
          startOpen={false} quiet={counts.all === 0}
        />
      {/key}
    {/if}
  {:else}
    <NotFound />
  {/if}
</main>

{#if authMode === 'accounts' && currentUser && !needsAuth && !isMCPConsent}<WebMCP />{/if}
{#if isSkills && authenticated && !needsAuth}<SyncActivity open={syncWatch !== null} context={syncWatch} onclose={() => (syncWatch = null)} />{/if}
{#if !terminalLayout && !homeLayout}<Footer
  signedIn={!isPublicList && authMode !== 'none' && authenticated && !needsAuth}
  showAdmin={!isPublicList && authenticated && !needsAuth && isAdminUser}
  userName={currentUser?.name ?? ''}
  onsignout={signOut}
/>{/if}
{#if newListOpen}
  <NewList
    {linkEnabled}
    oncreated={(list) => { newListOpen = false; location.assign(`/lists/${list.id}`) }}
    oncancel={() => (newListOpen = false)}
  />
{/if}

{#if removePending}
  <ConfirmDialog
    title={`Remove ${removePending.name}?`}
    body="This skill is on your scanner's list. It will come back on the next scan unless you ignore it. Installed files and saved sync sets stay as they are; review machine removals in Sync."
    confirmLabel="Remove"
    secondaryLabel="Remove and ignore"
    danger
    onconfirm={() => { const skill = removePending!; removePending = null; void doRemove(skill, false) }}
    onsecondary={() => { const skill = removePending!; removePending = null; void doRemove(skill, true) }}
    oncancel={() => (removePending = null)}
  />
{/if}

{#if bulkRemoveOpen}
  <ConfirmDialog
    title={`Remove ${bulkRemoving.length} ${bulkRemoving.length === 1 ? 'skill' : 'skills'}?`}
    body="They will be removed from the library. Installed files and saved sync sets stay as they are; review machine removals in Sync. Use Undo in the toast if you change your mind."
    confirmLabel="Remove"
    danger
    onconfirm={confirmBulkRemove}
    oncancel={() => (bulkRemoveOpen = false)}
  />
{/if}

{#if selecting && !needsAuth}
  <BulkBar
    count={selected.length} {tagOptions} busy={bulkBusy}
    onclear={toggleSelecting} ontag={bulkTag} onstage={bulkStage}
    onlike={() => bulkPatch({ liked: true })} onremove={bulkRemove}
  />
{/if}

{#if toast && !needsAuth}
  {#key toast.id}
    <Toast
      message={toast.message}
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
  main.auth-page { max-width: 960px; }
  main.home-layout { max-width: none; padding: 0; }
  main:focus { outline: none; }
  .site-header { border-bottom: 1px solid var(--line-soft); }
  .header-inner { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 24px; max-width: var(--content-width); margin: auto; padding: 24px 40px; }
  .site-nav { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
  .site-nav a, .entry-link { display: inline-flex; align-items: center; min-height: 44px; padding: 8px 12px; color: var(--muted); font-size: 0.8rem; font-weight: 600; text-decoration: none; border-radius: 8px; }
  .site-nav a:hover, .entry-link:hover { color: var(--ink); background: var(--card); }
  .home { display: inline-block; color: inherit; text-decoration: none; }
  .tag { color: var(--muted); font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.1em; }

  .add { display: flex; flex-wrap: wrap; gap: 8px; margin: 24px 0 6px; }
  .add input { flex: 1; }
  .add-hint { margin: 0 2px 12px; color: var(--muted); font-size: 0.78rem; }
  .add-hint code { font-size: 0.92em; }
  .add-error { margin: 0 2px 12px; color: var(--bad); font-size: 0.82rem; }

  .notices { display: grid; gap: 8px; }
  .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice[hidden] { display: none; }
  .good { border-color: var(--good); }
  .bad { border-color: var(--bad); }

  .toolbar {
    position: sticky; top: 0; z-index: 5; margin-top: 16px; padding: 14px 0; border-bottom: 1px solid var(--line);
    background: color-mix(in srgb, var(--bg) 92%, transparent); backdrop-filter: blur(8px);
    display: flex; flex-wrap: wrap; gap: 10px 16px; justify-content: space-between; align-items: center;
  }
  .filters { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }
  .filters button { background: none; border: 0; padding: 6px 10px; border-radius: 999px; cursor: pointer; color: var(--muted); font-weight: 500; min-height: 36px; }
  .filters button:hover { color: var(--ink); }
  .filters button[aria-pressed='true'] { background: var(--tint); color: var(--ink); font-weight: 650; }
  .filters .liked[aria-pressed='true'] { background: var(--heart); color: var(--bg); }
  .filters small { margin-left: 4px; font-variant-numeric: tabular-nums; }
  .sep { width: 1px; height: 20px; background: var(--line); margin: 0 6px; }
  .controls { display: flex; gap: 8px; }
  .view-switch { display: inline-flex; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .view-switch button { min-height: 34px; padding: 4px 10px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); cursor: pointer; }
  .view-switch button[aria-pressed='true'] { color: var(--ink); background: var(--tint); font-weight: 600; }
  .controls input { width: 10rem; }

  .filterbar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; padding: 4px 0 2px; }
  .bar-label {
    color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; margin-right: 4px;
  }
  .filterbar .chip {
    display: inline-flex; align-items: center; gap: 5px; max-width: min(260px, 100%); min-height: 32px;
    padding: 4px 10px; border: 1px solid var(--line); border-radius: 999px;
    background: transparent; color: var(--muted); font-size: 0.8rem; text-align: left; cursor: pointer;
    /* A machine's host name can be one long word: it wraps inside the chip rather than running out of it. */
    overflow-wrap: anywhere;
  }
  .filterbar .chip:hover { border-color: var(--muted); color: var(--ink); }
  .filterbar .chip[aria-pressed='true'] { background: var(--tint); color: var(--ink); border-color: var(--accent); }
  .filterbar .chip small { font-variant-numeric: tabular-nums; font-size: 0.75rem; }
  .filterbar .chip.old { opacity: 0.55; }
  .filterbar .chip.action { border-style: dashed; border-color: var(--accent); color: var(--accent-text); }
  .filterbar .chip.clear { border-style: dashed; }
  .filterbar .sep { height: 18px; }
  @media (pointer: coarse) { .filterbar .chip { min-height: 44px; } }

  .select { position: relative; display: inline-flex; }
  .select select { appearance: none; padding-right: 38px; cursor: pointer; }
  .select select:hover { border-color: var(--muted); }
  .select::after {
    content: ''; position: absolute; right: 16px; top: 50%; width: 7px; height: 7px; pointer-events: none;
    border: solid var(--muted); border-width: 0 2px 2px 0; transform: translateY(-70%) rotate(45deg);
  }
  .select:has(select:open)::after { transform: translateY(-30%) rotate(225deg); }
  .select option { background: var(--card); color: var(--ink); }
  @media (prefers-reduced-motion: no-preference) {
    .select::after { transition: transform 0.15s; }
  }
  /* Chromium can draw the list itself, so it can match the cards instead of the OS menu. */
  @supports (appearance: base-select) {
    .select select, .select select::picker(select) { appearance: base-select; }
    .select select::picker-icon { display: none; }
    .select select::picker(select) {
      margin-top: 6px; padding: 6px; border: 1px solid var(--line); border-radius: 12px; background: var(--card);
      box-shadow: 0 16px 40px -12px rgb(0 0 0 / 0.28);
    }
    .select option { padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
    .select option + option { margin-top: 4px; }
    .select option:hover, .select option:focus { background: var(--tint); border-color: var(--accent); outline: none; }
    .select option:checked { font-weight: 650; }
    .select option::checkmark { color: var(--accent-text); }
  }

  .count { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  .shelf-summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 10px; }
  .shelf-summary .count { margin: 0; }
  #sections { scroll-margin-top: 140px; }

  #sections.board { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; align-items: start; margin-top: 22px; }
  #sections.board.single-stage { grid-template-columns: minmax(0, 1fr); max-width: 620px; }
  .board > section { margin: 0; min-width: 0; padding: 14px; background: color-mix(in srgb, var(--card) 42%, var(--bg)); border: 1px solid var(--line); border-radius: var(--radius); }
  .board > section > h2 { font-size: 1.35rem; }
  .board .none { padding: 22px 4px; font-size: 0.95rem; }
  section { margin-top: 28px; }
  h2 { margin: 0; display: flex; align-items: baseline; gap: 10px; font: 500 1.65rem/1.2 var(--serif); letter-spacing: -0.02em; }
  h2 small { font: 500 0.9rem var(--sans); color: var(--muted); font-variant-numeric: tabular-nums; }
  .hint { margin: 4px 0 0; color: var(--muted); font: 0.78rem/1.5 var(--sans); }
  ul { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 12px; }
  .none { color: var(--muted); font: italic 1.05rem/1.5 var(--serif); padding: 18px 0 6px 22px; }
  .no-matches { padding: 48px 20px; text-align: center; }
  .no-matches h2 { display: block; }
  .no-matches p { color: var(--muted); }
  .shelf-skeleton { display: grid; gap: 12px; margin-top: 8px; }
  .skeleton-card { display: grid; align-content: start; gap: 14px; height: 112px; padding: 20px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius); }
  .skeleton-card i { display: block; height: 14px; border-radius: 5px; background: var(--line); }
  .skeleton-card i:first-child { width: 36%; height: 18px; }
  .skeleton-card i:last-child { width: 70%; }
  .empty { text-align: center; color: var(--muted); font: italic 1.15rem/1.5 var(--serif); padding: 56px 0; }

  .start {
    margin-top: 32px; padding: clamp(22px, 4vw, 30px);
    background: var(--card); border: 1px solid var(--line); border-radius: 16px;
    box-shadow: 0 28px 70px -56px rgb(0 0 0 / 0.35);
  }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .start h2 { display: block; margin: 8px 0 0; font: 600 clamp(1.5rem, 4vw, 1.9rem)/1.1 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 48ch; }
  .start > p { max-width: 64ch; }
  .start > .btn { margin-top: 18px; }
  .suggestions { list-style: none; display: grid; gap: 8px; margin: 14px 0 0; padding: 0; }
  .suggestions li { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .suggestions li > div { min-width: 0; }
  .suggestions strong { display: block; font-size: 0.86rem; }
  .suggestions code { display: block; margin-top: 1px; color: var(--muted); font-size: 0.72rem; overflow-wrap: anywhere; }
  .suggestions span { display: block; margin-top: 2px; color: var(--muted); font-size: 0.78rem; }
  .suggestions .btn { flex: none; }
  .start-note { margin: 10px 0 0; color: var(--muted); font-size: 0.78rem; }
  .save-anywhere { margin-top: 18px; padding-top: 10px; border-top: 1px solid var(--line); }
  .save-anywhere > summary { display: flex; align-items: center; gap: 8px; min-height: 44px; cursor: pointer; font-weight: 600; font-size: 0.85rem; }
  .save-anywhere > summary::before { content: '+'; color: var(--accent-text); font-size: 1.05rem; width: 12px; }
  .save-anywhere[open] > summary::before { content: '−'; }
  .save-anywhere > summary::-webkit-details-marker { display: none; }
  .save-anywhere > p { margin: 2px 0 0; color: var(--muted); font-size: 0.8rem; }
  .start-options { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-top: 22px; }
  .start-options > a:not(.btn) { color: var(--accent-text); padding: 10px 0; }
  .scan-help { margin-top: 20px; }
  .scan-help summary { cursor: pointer; font-weight: 600; }
  .command { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-top: 10px; }
  .command code {
    flex: 1 1 240px; min-width: 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px;
    background: var(--bg); font-size: 0.8rem; overflow-wrap: anywhere;
  }

  :global([data-skill]) { scroll-margin-top: 92px; }
  :global([data-skill].flash) { background: var(--tint); border-radius: 8px; }
  @media (prefers-reduced-motion: no-preference) {
    :global([data-skill].flash) { animation: flash 1.6s var(--ease-out); }
    @keyframes flash {
      0%, 60% { background: var(--tint); }
      100% { background: transparent; }
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    .filters button { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
    .start { animation: rise 340ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  }
  @media (max-width: 560px) {
    main { padding: 28px 16px 40px; }
    .header-inner { flex-wrap: wrap; gap: 14px; padding: 20px 16px; }
    .tag { margin-left: auto; max-width: 26ch; text-align: right; white-space: nowrap; }
    .site-nav { width: 100%; }
    .site-nav a { flex: 1; justify-content: center; border: 1px solid var(--line); }
    .add { flex-direction: column; align-items: stretch; }
    .add input { flex: 1 1 auto; width: 100%; }
    .add .btn { width: 100%; flex: none; font-size: 0.9rem; }
    .suggestions li { align-items: flex-start; }
    .suggestions .btn { min-height: 44px; }
    .controls { width: 100%; flex-wrap: wrap; }
    .controls input { flex: 1; width: auto; }
    .sep { display: none; }
    .start { margin-top: 24px; padding: 20px; border-radius: 14px; }
  }
  .shelf-filters { border-bottom: 1px solid var(--line); }
  .shelf-filters > summary { display: flex; align-items: center; gap: 12px; padding: 12px 2px; min-height: 44px; cursor: pointer; font-size: 0.85rem; font-weight: 600; }
  .shelf-filters > summary::before { content: '+'; color: var(--accent-text); font-size: 1.1rem; width: 12px; }
  .shelf-filters[open] > summary::before { content: '−'; }
  .shelf-filters > summary span { margin-left: auto; color: var(--muted); font-size: 0.75rem; font-weight: 400; }
  .filter-panel { display: grid; gap: 8px; padding: 4px 0 16px; }
  @media (max-width: 1000px) {
    main { padding: 24px 24px 40px; }
    .header-inner { display: flex; gap: 16px; padding: 16px 24px; }
    .site-nav { flex-wrap: nowrap; width: 100%; min-width: 0; overflow-x: auto; padding-bottom: 8px; }
    .site-nav a { flex: none; justify-content: center; white-space: nowrap; border: 0; }
    .toolbar { position: static; padding: 10px 0; gap: 8px; }
    .filters { flex-wrap: wrap; width: 100%; min-width: 0; }
    .filters button { white-space: nowrap; }
    .controls { display: flex; width: 100%; flex-wrap: nowrap; min-width: 0; }
    .controls input { flex: 1; width: 0; min-width: 50px; }
    .view-switch { flex: none; }
    .add { margin-top: 16px; }
  }
  @media (max-width: 900px) {
    #sections.board { grid-template-columns: minmax(0, 1fr); margin-top: 14px; }
    .board > section.empty-stage { display: none; }
  }
  @media (max-width: 560px) {
    main { padding: 20px 16px 36px; }
    .header-inner { padding-left: 16px; padding-right: 16px; }
    .controls { gap: 6px; }
    .controls select { padding-left: 8px; padding-right: 26px; }
  }
  @media (pointer: coarse) { .filters button, .view-switch button { min-height: 44px; } }
  main.terminal-layout {max-width:none;padding:8px 12px;min-height:0;flex:1;}
  @media(max-width:700px){main.terminal-layout{padding:4px 6px}}
</style>
