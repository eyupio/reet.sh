<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import { shell } from '../dialogShell'
  import { UnauthorizedError } from '../api'
  import { CATALOGUE, agentChoices } from '../catalogue'
  import { cleanModel, modelSuggestions, notStartedText, refusalText, toolName, validateModel } from '../agentLaunch'
  import { afterLaunch, defaultTitle, initialChoice, lastChoice, sentence, startable, startedLine, summary, toCreate, toSelectionRun, withHome, withProject, withSelection, sendKey,
    type LaunchChoice, type LaunchCreate, type LaunchDefaults, type SelectionRunRequest, type SentencePart } from '../launchBar'
  import { createTerminal, getFolderProject, getSelectionRun, listBranches, readiness, refusalSentence, retrySelectionMember, sessionHref, startSelectionRun, TerminalError,
    type BranchesResult, type FolderProject, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { memberLine, runPending, STARTED_MEANS, type ProjectRun } from '../projects'
  import { keyboardLikelyOpen, PHONE_SCREEN, ViewportTracker, browserViewport } from '../terminal/sizing'
  import { chromeFocus } from '../shell/chrome.svelte'
  import SlotPicker, { type SlotOption } from './SlotPicker.svelte'

  // The launch bar docked at the foot of Home: "Describe the job" and a sentence whose bold words are the choices (spec: The
  // launch bar). It opens on the last launch (read from the sessions list, never from browser storage), so a second session
  // is one press; Start sends one create through the idempotent create path and opens the new session. While the request is
  // out the bar is locked; a reply that never came keeps the same request for Retry. On a phone the sentence folds into one
  // summary line with Change, and the tab bar steps aside while the prompt has the keyboard up. The prompt lives only here.
  let { projects, machines, sessions, profile, ready, initialProject = '', onproject, onstarted, onleavefailed, onsignedout, onaddproject, onrun, onrundone }: {
    projects: FolderProject[] | undefined
    /** The terminal machines, with readiness and launch capabilities; undefined until read. */
    machines: TerminalMachine[] | undefined
    sessions: TerminalSession[] | undefined
    profile: LaunchDefaults
    /** The projects, sessions and profile have been read (or failed): the first choice can be made. */
    ready: boolean
    /** The project an address chose (`/?project=<id>`). */
    initialProject?: string
    /** The project the sentence names now ('' for none, or several), so the rail can mark it. */
    onproject?: (id: string) => void
    /** Hands over the started session; 'leaving' when the page is opening it at another address. */
    onstarted: (session: TerminalSession) => void | 'leaving'
    /** The next page never arrived (a few seconds after 'leaving'): the page may say what started. */
    onleavefailed?: (session: TerminalSession) => void
    onsignedout?: (e: unknown) => void
    /** The project slot's *Add a project…*. */
    onaddproject?: (opener: HTMLElement) => void
    /** A start in several projects moved on (a member started): the page may read its sessions again. */
    onrun?: () => void
    /** The status sheet of a start in several projects was closed: the rail leaves Select. */
    onrundone?: () => void
  } = $props()

  const id = $props.id()
  const ADD = '\u0000add', HOME = '\u0000home:'
  const LABELS: Record<string, string> = { agent: 'Agent', model: 'Model', project: 'Project', machine: 'Machine', branch: 'Branch', worktree: 'Worktree', effort: 'Thinking' }
  const LOST_TAIL = 'Check your sessions before starting another. If you discard it and start again, a second agent may start.'
  const LOST = `We didn't hear back, so the agent may have started. ${LOST_TAIL}`
  const LOST_RUN = "We didn't hear back, so the agents may have started. Check your sessions before starting again. If you discard it and start again, more agents may start."

  let choice = $state<LaunchChoice | null>(null)
  let busy = $state(false), leaving = $state(false), message = $state(''), failed = $state(false)
  // One attempt: the request id, the run token and every field, kept while its outcome is unknown so Retry asks about the same
  // start and a lost reply can never start a second agent. Dropped only when the server has definitely answered.
  // A start in several projects keeps its one run request (request id and every field) the same way.
  let operation = $state<({ kind: 'one' } & LaunchCreate & { host: string }) | { kind: 'run'; body: SelectionRunRequest } | null>(null)
  const life = new AbortController()
  let form = $state<HTMLFormElement>(), field = $state<HTMLTextAreaElement>(), startButton = $state<HTMLButtonElement>()

  // The first choice: the last launch, then the project an address named.
  let named = ''
  $effect(() => {
    if (choice || !ready || !projects) return
    untrack(() => {
      let next = initialChoice(lastChoice(sessions ?? [], projects!, profile), projects!, profile)
      const p = initialProject && projects!.find(x => x.id === initialProject)
      if (p) { next = withProject(next, p, profile); named = p.id }
      choice = next
    })
  })
  // An address that names a project later (an old /projects/<id> link, looked up after the bar opened) chooses it once.
  $effect(() => {
    const want = initialProject
    if (!want || !choice || want === named) return
    untrack(() => {
      const p = projects?.find(x => x.id === want)
      if (!p || locked) return
      named = want
      choice = withProject(choice!, p, profile)
    })
  })
  // A project removed while chosen: the bar falls back as a fresh open would, keeping the prompt.
  $effect(() => {
    const list = projects
    if (!choice || !list) return
    untrack(() => {
      if (choice!.projectIds.every(pid => list.some(p => p.id === pid))) return
      choice = { ...initialChoice(choice!, list, profile), prompt: choice!.prompt }
    })
  })

  $effect(() => {
    const ids = choice?.projectIds ?? []
    const named = ids.length === 1 ? ids[0] : ''
    untrack(() => onproject?.(named))
  })

  /** The rail's card (or New session here): this project in the sentence, then the prompt. */
  export function choose(project: FolderProject) {
    if (!choice || locked) return
    choice = withProject(choice, project, profile)
    message = ''
    void tick().then(() => field?.focus({ preventScroll: phone }))
  }
  export function focusPrompt() { field?.focus() }
  /** The rail's selection, in rail order: two or more projects start together; one is that project; none goes back to one. */
  export function select(ids: string[]) {
    if (!choice || !projects || locked) return
    choice = withSelection(choice, ids, projects, profile)
    message = ''
  }

  const picked = $derived(choice ? choice.projectIds.map(pid => projects?.find(p => p.id === pid)).filter((p): p is FolderProject => !!p) : [])
  const one = $derived(picked.length === 1 ? picked[0] : null)
  const targetId = $derived(one ? one.host : choice?.homeMachine ?? '')
  const target = $derived(machines?.find(m => m.agentId === targetId))
  const hostOf = (agentId: string) => machines?.find(m => m.agentId === agentId)?.host ?? projects?.find(p => p.host === agentId)?.machine ?? agentId

  // The checkout's branches, read once per folder from the machine (only for a repository on a machine that lists them).
  const branchReads = new SvelteMap<string, BranchesResult | 'failed'>()
  const branchKey = $derived(one?.isRepo && target?.capabilities?.branches ? `${one.host}\u0000${one.dir}` : '')
  $effect(() => {
    const key = branchKey, p = one
    if (!key || !p || untrack(() => branchReads.has(key))) return
    branchReads.set(key, 'failed')
    listBranches(p.host, p.dir, undefined, life.signal).then(r => branchReads.set(key, r), () => {})
  })
  const branches = $derived(branchKey ? branchReads.get(branchKey) : undefined)
  const current = $derived(branches && branches !== 'failed' ? branches.current : '')

  const parts = $derived(choice && projects ? sentence(choice, projects, machines ?? [], current) : [])
  const words = $derived(parts.filter(p => p.slot !== 'checkout'))
  const checkout = $derived(parts.find(p => p.slot === 'checkout')?.text ?? '')
  // The phone's summary line, with the project in bold as the mock-up draws it.
  const summaryParts = $derived.by(() => {
    const line = summary(words), project = words.find(p => p.slot === 'project')?.text ?? ''
    const at = project ? line.indexOf(` on ${project} at `) : -1
    return at < 0 ? { before: line, bold: '', after: '' } : { before: line.slice(0, at + 4), bold: project, after: line.slice(at + 4 + project.length) }
  })
  // A tablet's sentence is the drawing's: the model and worktree slots (with the word before each) are in the Change sheet.
  const tabletHidden = $derived(new Set(words.flatMap((p, i) => p.slot === 'model' || p.slot === 'worktree' ? [i - 1, i] : [])))
  // Why Start can't be pressed yet. Until the machines have been read nothing is checked, so nothing is sent.
  const reason = $derived(!choice || !projects ? '' : !machines ? 'Start waits until your machines have been read.' : startable(choice, projects, machines, profile))
  const locked = $derived(busy || leaving || operation !== null)
  const shownStatus = $derived(message || (!locked ? reason ?? '' : ''))
  // The session's title, shown before sending (the prompt's first line).
  const titled = $derived(choice?.prompt ? defaultTitle(choice.prompt) : '')
  const starter = $derived(!!choice && choice.prompt === '' && !!one?.hasStarterPrompt && !locked)

  // Touch gets sheets and never sends on a key; a phone gets the one-line layout and the keyboard rule.
  let touch = $state(false), phone = $state(false), mac = $state(false)

  // --- slots -------------------------------------------------------------------------------------------------------------
  let open = $state<{ slot: string; opener: HTMLElement } | null>(null)
  let branchFilter = $state<{ text: string; result: BranchesResult | 'failed' | null } | null>(null)
  const machineFor = (agentId: string) => machines?.find(m => m.agentId === agentId)

  function optionsFor(slot: string): SlotOption[] {
    if (!choice) return []
    if (slot === 'agent') {
      const offers = target ? agentChoices(target) : CATALOGUE.map(agent => ({ agent, available: agent.enabled, reason: agent.enabled ? undefined : agent.disabledReason }))
      return offers.map(o => ({ value: o.agent.id, label: o.agent.name, disabled: !o.available, reason: o.available ? undefined : o.reason }))
    }
    if (slot === 'model') {
      const hints = modelSuggestions(choice.agent)
      const own = cleanModel(choice.model)
      return [{ value: '', label: 'default' }, ...hints.map(m => ({ value: m, label: m })), ...(own && !hints.includes(own) ? [{ value: own, label: own }] : [])]
    }
    if (slot === 'project') {
      const out: SlotOption[] = (projects ?? []).map(p => {
        const why = readiness(machineFor(p.host))
        return { value: p.id, label: p.name, detail: p.dir, group: p.machine || 'Machine not enrolled', disabled: !!why, reason: why || undefined }
      })
      for (const m of machines ?? []) {
        const why = readiness(m)
        out.push({ value: HOME + m.agentId, label: `Home folder on ${m.host}`, group: 'Home folder', disabled: !!why, reason: why || undefined })
      }
      if (onaddproject) out.push({ value: ADD, label: 'Add a project…', group: ' ' })
      return out
    }
    if (slot === 'machine') return (machines ?? []).map(m => { const why = readiness(m); return { value: m.agentId, label: m.host, disabled: !!why, reason: why || undefined } })
    if (slot === 'branch') {
      const read = branchFilter?.result ?? branches
      const names = read && read !== 'failed' ? read.branches : []
      const listed = choice.branch && !names.includes(choice.branch) ? [choice.branch, ...names] : names
      return [{ value: '', label: current ? `the current branch (${current})` : 'the current branch' }, ...listed.map(b => ({ value: b, label: b }))]
    }
    if (slot === 'worktree') return [{ value: 'yes', label: 'a fresh worktree' }, { value: 'no', label: 'the checkout' }]
    if (slot === 'effort') return [{ value: '', label: 'default' }, { value: 'low', label: 'low' }, { value: 'medium', label: 'medium' }, { value: 'high', label: 'high' }]
    return []
  }
  function valueOf(slot: string): string {
    if (!choice) return ''
    if (slot === 'agent') return choice.agent
    if (slot === 'model') return cleanModel(choice.model)
    if (slot === 'project') return choice.projectIds[0] ?? (choice.homeMachine ? HOME + choice.homeMachine : '')
    if (slot === 'machine') return choice.homeMachine ?? ''
    if (slot === 'branch') return choice.branch
    if (slot === 'worktree') return choice.worktree ? 'yes' : 'no'
    if (slot === 'effort') return choice.effort
    return ''
  }
  function set(slot: string, value: string) {
    if (!choice || locked) return
    message = ''
    if (slot === 'agent') choice = { ...choice, agent: value, model: value === choice.agent ? choice.model : '', effort: CATALOGUE.find(a => a.id === value)?.effort ? choice.effort : '' }
    else if (slot === 'model') choice = { ...choice, model: value }
    else if (slot === 'project') {
      if (value === ADD) { const opener = open?.opener; if (opener) onaddproject?.(opener); return }
      if (value.startsWith(HOME)) { choice = withHome(choice, value.slice(HOME.length)); return }
      const p = projects?.find(x => x.id === value)
      if (p) choice = withProject(choice, p, profile)
    } else if (slot === 'machine') choice = withHome(choice, value)
    else if (slot === 'branch') choice = { ...choice, branch: value }
    else if (slot === 'worktree') choice = { ...choice, worktree: value === 'yes' }
    else if (slot === 'effort') choice = { ...choice, effort: value as LaunchChoice['effort'] }
  }
  function openSlot(slot: string, opener: HTMLElement) {
    if (locked) return
    branchFilter = null
    open = open?.slot === slot ? null : { slot, opener }
  }
  // The branch list is filtered by the machine (it lists at most 200).
  function filterBranches(text: string) {
    const p = one
    if (!p) return
    if (!text) { branchFilter = null; return }
    branchFilter = { text, result: null }
    listBranches(p.host, p.dir, text, life.signal).then(r => { if (branchFilter?.text === text) branchFilter = { text, result: r } },
      e => { if (branchFilter?.text === text) { branchFilter = { text, result: 'failed' }; branchError = refusalSentence(e) } })
  }
  let branchError = $state('')
  const pickerStatus = $derived(open?.slot !== 'branch' ? '' : branchFilter && branchFilter.result === null ? 'Reading branches…'
    : (branchFilter?.result ?? branches) === 'failed' ? branchError || "Couldn't read the branches just now." : !target?.capabilities?.branches ? 'Update reet on that machine to list its branches.' : '')
  // The machine follows its project (R18); with several projects the projects come from the rail and each keeps its own base.
  const slotButton = (part: SentencePart) => part.slot !== '' && part.slot !== 'checkout' && (part.slot !== 'machine' || !!choice?.homeMachine && picked.length === 0)
    && (picked.length < 2 || (part.slot !== 'project' && part.slot !== 'branch'))

  // A finger-sized slot (44 px on touch) keeps the sentence's spacing: the extra width hangs into the gaps beside the word
  // (negative side margins), so short words are not pushed apart.
  function hit(node: HTMLElement) {
    const word = node.firstElementChild as HTMLElement | null
    const fit = () => {
      if (!word) return
      const extra = Math.min(0, (word.getBoundingClientRect().width - (parseFloat(getComputedStyle(node).minWidth) || 0)) / 2)
      node.style.marginInline = extra ? `${extra}px` : ''
    }
    const watch = new ResizeObserver(fit)
    if (word) watch.observe(word)
    watch.observe(node)
    fit()
    return { destroy() { watch.disconnect() } }
  }

  // --- the phone's Change sheet ------------------------------------------------------------------------------------------
  let changing = $state(false), changeButton: HTMLElement | null = null, changeDialog = $state<HTMLDialogElement>()
  async function openChange(opener: HTMLElement) {
    if (locked) return
    changeButton = opener
    changing = true
    await tick()
    changeDialog?.showModal()
    changeDialog?.querySelector<HTMLElement>('.row')?.focus()
  }
  // Escape, Close and Done always close it, and a list opened from it with it.
  function closeChange() {
    open = null; branchFilter = null
    changeDialog?.close()
    changing = false
    changeButton?.focus()
  }

  // --- prompt ------------------------------------------------------------------------------------------------------------
  async function useStarter() {
    const p = one
    if (!p || !choice) return
    try {
      const full = await getFolderProject(p.id, life.signal)
      if (!choice || choice.projectIds[0] !== p.id || choice.prompt !== '') return
      choice = { ...choice, prompt: full.starterPrompt ?? '' }
      message = ''
      await tick()
      field?.focus()
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
      failed = true; message = refusalSentence(e)
    }
  }
  function keydown(event: KeyboardEvent) {
    if (!sendKey(event, touch)) return
    event.preventDefault()
    void start()
  }

  // --- start -------------------------------------------------------------------------------------------------------------
  // A refusal the server gave for certain: nothing started, so the person may change the choices and start again.
  const definite = (e: unknown) => e instanceof TerminalError && [400, 401, 403, 404, 409, 422].includes(e.status)
  async function start() {
    if (busy || leaving || !choice || !projects || life.signal.aborted) return
    if (!operation) {
      if (reason !== null) return
      if (picked.length > 1) operation = { kind: 'run', body: toSelectionRun(choice, crypto.randomUUID()) }
      else {
        const req = toCreate(choice, one, crypto.randomUUID(), crypto.randomUUID().replaceAll('-', ''), profile)
        operation = { kind: 'one', ...req, host: hostOf(req.agentId) }
      }
    }
    if (operation.kind === 'run') return startRun(operation.body)
    const op = operation
    busy = true; failed = false
    message = `Starting ${toolName(op.launch.tool)} on ${op.host}…`
    try {
      const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]), op.launch)
      if (life.signal.aborted) return
      // Answered, but the machine did not start it: nothing is running, so this request is done and the next Start is a new one.
      const notStarted = notStartedText(session.state)
      if (notStarted) { operation = null; failed = true; message = notStarted; return }
      operation = null
      const next = afterLaunch(choice!)
      if (onstarted(session) === 'leaving') {
        // The page is going to the session: stay locked so nothing is pressed again before the next page arrives.
        leaving = true
        message = `Opening ${session.title}…`
        leave(session, next)
        return
      }
      choice = next; message = ''
    } catch (e) {
      if (life.signal.aborted) return
      failed = true
      if (definite(e)) {
        const err = e as TerminalError
        operation = null
        message = refusalText(err.status, err.code, err.message)
        if (err.status === 401) onsignedout?.(new UnauthorizedError())
      } else {
        message = e instanceof TerminalError && e.status === 503 ? `${refusalText(503)} ${LOST_TAIL}` : LOST
      }
    } finally {
      busy = false
      if (!leaving && !life.signal.aborted) void tick().then(() => (operation ? startButton : field)?.focus())
    }
  }
  // --- start in several projects ------------------------------------------------------------------------------------------
  // One request makes the run; the controller starts its members (2 at a time, 1 per machine) and the sheet reads the run
  // until none is waiting. "Started" counts only what the controller says started.
  let run = $state<ProjectRun | null>(null), runDialog = $state<HTMLDialogElement>(), runNote = $state('')
  const retrying = new SvelteMap<number, string>()
  let poll: ReturnType<typeof setTimeout> | undefined, startedSeen = 0
  async function startRun(body: SelectionRunRequest) {
    busy = true; failed = false
    message = `Starting ${toolName(body.tool)} in ${body.projectIds.length} projects…`
    try {
      const answer = await startSelectionRun(body, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
      if (life.signal.aborted) return
      operation = null
      choice = afterLaunch(choice!)
      message = ''
      await showRun(answer)
    } catch (e) {
      if (life.signal.aborted) return
      failed = true
      if (definite(e)) {
        const err = e as TerminalError
        operation = null
        message = refusalText(err.status, err.code, err.message)
        if (err.status === 401) onsignedout?.(new UnauthorizedError())
      } else message = LOST_RUN
    } finally {
      busy = false
      if (!run && !life.signal.aborted) void tick().then(() => (operation ? startButton : field)?.focus())
    }
  }
  async function showRun(next: ProjectRun) {
    run = next; runNote = ''; startedSeen = 0
    noteStarted(next)
    await tick()
    runDialog?.showModal()
    runDialog?.querySelector<HTMLElement>('[data-shell-close]')?.focus()
    follow()
  }
  function noteStarted(next: ProjectRun) {
    const n = next.members.filter(m => m.state === 'started').length
    if (n > startedSeen) onrun?.()
    startedSeen = n
  }
  // The run is read again every 1.5 seconds while a member waits to start; a read that fails is tried again.
  function follow() {
    clearTimeout(poll)
    if (!run || !runPending(run)) return
    const id = run.id
    poll = setTimeout(async () => {
      try {
        const next = await getSelectionRun(id, life.signal)
        if (run?.id !== id) return
        run = next; runNote = ''
        noteStarted(next)
      } catch (e) {
        if (life.signal.aborted || run?.id !== id) return
        if (e instanceof TerminalError && e.status === 401) { onsignedout?.(new UnauthorizedError()); return }
        runNote = "Couldn't read the start just now. Trying again."
      }
      follow()
    }, 1500)
  }
  async function retryMember(position: number) {
    if (!run || retrying.has(position)) return
    const id = run.id
    retrying.set(position, '')
    try {
      const next = await retrySelectionMember(id, position, life.signal)
      if (run?.id !== id) return
      run = next
      retrying.delete(position)
    } catch (e) {
      if (life.signal.aborted || run?.id !== id) return
      // Another retry (or the controller) already moved it on: read the run as it is.
      if (e instanceof TerminalError && e.status === 409) { retrying.delete(position); try { run = await getSelectionRun(id, life.signal) } catch { /* the next read says */ } }
      else retrying.set(position, refusalSentence(e))
    }
    follow()
  }
  function closeRun() {
    clearTimeout(poll)
    runDialog?.close()
    run = null; retrying.clear()
    onrundone?.()
    void tick().then(() => field?.focus())
  }

  function discard() {
    if (busy) return
    operation = null; message = ''; failed = false
    void tick().then(() => field?.focus())
  }

  // While the page is leaving: if it never goes, let go after a few seconds and say what started; if the browser brings the
  // page back from its cache on Back, unlock at once. Once the page has gone (pagehide), the timer stops.
  const LEAVE_MS = 5000
  let stopLeaving = () => {}
  function leave(session: TerminalSession, next: LaunchChoice) {
    const failedLeave = onleavefailed
    const done = () => { clearTimeout(timer); removeEventListener('pagehide', gone); removeEventListener('pageshow', back); stopLeaving = () => {} }
    const unlock = () => { leaving = false; choice = next; message = '' }
    const letGo = () => { done(); unlock(); failedLeave?.(session) }
    const gone = () => clearTimeout(timer)
    const back = (e: PageTransitionEvent) => { if (!e.persisted) return; done(); unlock() }
    const timer = setTimeout(letGo, LEAVE_MS)
    addEventListener('pagehide', gone)
    addEventListener('pageshow', back)
    stopLeaving = done
  }

  // --- the phone keyboard ------------------------------------------------------------------------------------------------
  // While the prompt has the keyboard up on a phone, the bar sits on the keyboard's top edge (the tab bar has stepped aside).
  let focused = $state(false), lift = $state(0)
  const tracker = new ViewportTracker()
  let frame = 0
  function measureKeyboard() {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      const reading = browserViewport()
      const usable = tracker.read(reading)
      const open = keyboardLikelyOpen(usable.shrink, { coarse: touch, narrow: phone, editing: focused, scale: reading.visual?.scale ?? 1 })
      lift = open && reading.visual ? Math.max(0, reading.innerHeight - reading.visual.offsetTop - reading.visual.height) : 0
    })
  }
  $effect(() => {
    const on = focused && phone
    chromeFocus.launch = on
    measureKeyboard()
    return () => { chromeFocus.launch = false }
  })

  // The docked bar must never hide the control that has focus (WCAG 2.4.11): the page keeps that much room at its foot when it
  // scrolls something into view. Only while the bar is on the page.
  $effect(() => {
    const bar = form
    if (!bar) return
    const root = document.documentElement
    const fit = () => {
      const below = parseFloat(getComputedStyle(bar).bottom) || 0
      root.style.scrollPaddingBottom = `${Math.ceil(bar.getBoundingClientRect().height + below + 8)}px`
    }
    const watch = new ResizeObserver(fit)
    watch.observe(bar)
    addEventListener('resize', fit)
    fit()
    return () => { watch.disconnect(); removeEventListener('resize', fit); root.style.removeProperty('scroll-padding-bottom') }
  })

  onMount(() => {
    const coarse = matchMedia('(pointer: coarse)'), narrow = matchMedia(PHONE_SCREEN)
    const read = () => { touch = coarse.matches; phone = narrow.matches; measureKeyboard() }
    read()
    mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
    coarse.addEventListener('change', read); narrow.addEventListener('change', read)
    const vv = window.visualViewport
    addEventListener('resize', measureKeyboard)
    vv?.addEventListener('resize', measureKeyboard)
    vv?.addEventListener('scroll', measureKeyboard)
    return () => {
      stopLeaving(); life.abort(); cancelAnimationFrame(frame); clearTimeout(poll)
      coarse.removeEventListener('change', read); narrow.removeEventListener('change', read)
      removeEventListener('resize', measureKeyboard); vv?.removeEventListener('resize', measureKeyboard); vv?.removeEventListener('scroll', measureKeyboard)
      chromeFocus.launch = false
    }
  })
</script>

{#snippet approvals(name: string)}
  <div class="approvals" role="radiogroup" aria-label="Approvals">
    <span class="lead" aria-hidden="true">Approvals</span>
    <span class="segments">
      <label class:on={choice?.approvals !== 'yolo'}><input type="radio" name={name} value="ask" checked={choice?.approvals !== 'yolo'} onchange={() => choice && (choice = { ...choice, approvals: 'ask' })} />Ask me</label>
      <label class="off"><input type="radio" name={name} value="yolo" disabled aria-describedby={`${id}-${name}-why`} />YOLO</label>
    </span>
    <span class="vh" id={`${id}-${name}-why`}>reet doesn't start YOLO sessions yet.</span>
  </div>
{/snippet}

<form class="launch" aria-label="Start a session" bind:this={form} style:--lift={`${lift}px`} class:lifted={lift > 0}
  onsubmit={(event) => { event.preventDefault(); void start() }}>
  <fieldset disabled={locked || !choice}>
    <div class="ask">
      <label class="vh" for={`${id}-prompt`}>Describe the job</label>
      <textarea id={`${id}-prompt`} bind:this={field} rows="1" placeholder={phone ? 'Describe the job…' : 'Describe the job. Say what done looks like.'}
        value={choice?.prompt ?? ''} oninput={(e) => { if (choice) { choice = { ...choice, prompt: e.currentTarget.value }; message = '' } }}
        onkeydown={keydown} onfocus={() => { focused = true }} onblur={() => { focused = false }}
        aria-describedby={[choice?.prompt ? `${id}-secrets` : '', shownStatus ? `${id}-status` : ''].filter(Boolean).join(' ') || undefined}
        autocomplete="off" spellcheck="true"></textarea>
    </div>
    {#if starter || choice?.prompt}
      <div class="under">
        {#if starter}<button type="button" class="chip" onclick={useStarter}>Use the starter prompt</button>{/if}
        {#if titled}<p class="titled">Titled “{titled}”</p>{/if}
        {#if choice?.prompt}<p class="secrets" id={`${id}-secrets`}>Secrets don't belong here. The prompt is visible in the machine's process list.</p>{/if}
      </div>
    {/if}
    <div class="line">
      <p class="sentence">
        {#each words as part, i (i)}
          {#if slotButton(part)}
            <button type="button" class="part slot" class:tablet-hide={tabletHidden.has(i)} aria-label={`${LABELS[part.slot]}: ${part.text}`} aria-haspopup="listbox" aria-expanded={open?.slot === part.slot}
              use:hit onclick={(e) => openSlot(part.slot, e.currentTarget)}><span>{part.text}</span></button>
          {:else if part.slot}
            <b class="part fixed" class:tablet-hide={tabletHidden.has(i)}>{part.text}</b>
          {:else}
            <span class="part" class:tablet-hide={tabletHidden.has(i)}>{part.text}</span>
          {/if}
        {/each}
      </p>
      <button type="button" class="change tablet-change" aria-haspopup="dialog" onclick={(e) => openChange(e.currentTarget)}>Change</button>
      {@render approvals(`${id}-approvals`)}
    </div>
    {#if checkout}<p class="checkout">{checkout}</p>{/if}
    <div class="summary">
      <p class="summary-text">{#if summaryParts.bold}{summaryParts.before}<b>{summaryParts.bold}</b>{summaryParts.after}{:else}{summaryParts.before}{/if}</p>
      <button type="button" class="change" aria-haspopup="dialog" onclick={(e) => openChange(e.currentTarget)}>Change</button>
    </div>
  </fieldset>
  <div class="go">
    {#if operation && !busy}<button type="button" class="discard" onclick={discard}>Discard this request</button>{/if}
    <button type="submit" class="start" bind:this={startButton} disabled={busy || leaving || !choice || (!operation && reason !== null)} aria-busy={busy || leaving}
      aria-label={operation ? 'Try the same request again' : 'Start the session'} aria-describedby={shownStatus ? `${id}-status` : undefined}>
      <svg class="arrow" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M12 19V5M5 12l7-7 7 7" /></svg>
      <span class="word">{operation ? 'Try the same request again' : 'Start'}</span>{#if !operation}<kbd aria-hidden="true">{mac ? '⌘ ↵' : 'Ctrl ↵'}</kbd>{/if}
    </button>
  </div>
  <p class="status" class:bad={failed} id={`${id}-status`} role="status" aria-live="polite" aria-atomic="true">{shownStatus}</p>
</form>

{#if open && choice}
  {@const slot = open?.slot ?? ''}
  <SlotPicker label={LABELS[slot]} options={optionsFor(slot)} value={valueOf(slot)} sheet={touch || changing} opener={open?.opener ?? null}
    filter={slot === 'project' ? 'Find a project' : slot === 'branch' && target?.capabilities?.branches ? 'Filter branches' : undefined}
    onfilter={slot === 'branch' ? filterBranches : undefined} status={pickerStatus}
    other={slot === 'model' ? { label: 'Other…', field: 'Model name', validate: validateModel } : undefined}
    onpick={(value) => set(slot, value)} onclose={() => { open = null; branchFilter = null }} />
{/if}

{#if changing && choice}
  <dialog class="change-sheet" bind:this={changeDialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-change-title`}
    oncancel={(event) => { event.preventDefault(); closeChange() }} onclick={(event) => { if (event.target === changeDialog) closeChange() }}>
    <div class="sheet" data-shell-card>
      <header data-shell-head>
        <h2 id={`${id}-change-title`}>Change the launch</h2>
        <button type="button" class="close" data-shell-close aria-label="Close" onclick={closeChange}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
      </header>
      <div class="rows" data-shell-body>
        {#each words.filter(p => p.slot) as part (part.slot)}
          {#if slotButton(part)}
            <button type="button" class="row" aria-label={`${LABELS[part.slot]}: ${part.text}`} aria-haspopup="listbox" onclick={(e) => openSlot(part.slot, e.currentTarget)}>
              <span class="row-label">{LABELS[part.slot]}</span><span class="row-value">{part.text}</span><span class="chev" aria-hidden="true">›</span>
            </button>
          {:else}
            <p class="row fixed"><span class="row-label">{LABELS[part.slot]}</span><span class="row-value">{part.text}</span></p>
          {/if}
        {/each}
        {#if checkout}<p class="checkout">{checkout}</p>{/if}
        {@render approvals(`${id}-sheet-approvals`)}
      </div>
      <footer data-shell-foot>
        <button type="button" class="done" onclick={closeChange}>Done</button>
      </footer>
    </div>
  </dialog>
{/if}

{#if run}
  {@const shown = run}
  <dialog class="run-sheet" bind:this={runDialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-run-title`}
    oncancel={(event) => { event.preventDefault(); closeRun() }}>
    <div class="sheet" data-shell-card>
      <header data-shell-head>
        <h2 id={`${id}-run-title`}>Start in several projects</h2>
        <button type="button" class="close" data-shell-close aria-label="Close" onclick={closeRun}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
      </header>
      <div class="run-body" data-shell-body>
        <p class="started" role="status" aria-live="polite" aria-atomic="true">{startedLine(shown)}</p>
        <p class="means" data-shell-aside>{STARTED_MEANS}</p>
        <ul class="members">
          {#each shown.members as m (m.position)}
            {@const name = m.name || m.dir}
            {@const why = retrying.get(m.position)}
            <li class="member {m.state}">
              <span class="member-head"><b class="member-name">{name}</b><span class="member-machine">{hostOf(m.host)}</span></span>
              <span class="member-line">{memberLine(m)}</span>
              {#if why}<span class="member-line bad">{why}</span>{/if}
              {#if (m.state === 'started' || m.state === 'removed') && m.sessionId}
                <a class="member-act" href={sessionHref({ agentId: m.host, id: m.sessionId })} aria-label={`Open ${name}`}>Open</a>
              {:else if m.state === 'failed'}
                <button type="button" class="member-act" aria-label={`Retry ${name}`} disabled={retrying.has(m.position) && !why} onclick={() => retryMember(m.position)}>Retry</button>
              {/if}
            </li>
          {/each}
        </ul>
        {#if runNote}<p class="member-line">{runNote}</p>{/if}
      </div>
      <footer data-shell-foot>
        <button type="button" class="done" onclick={closeRun}>Done</button>
      </footer>
    </div>
  </dialog>
{/if}

<style>
  /* The mock-up's docked bar: a raised card with a 16 px radius; the prompt row above the sentence row. */
  .launch {
    position: sticky; bottom: calc(var(--bottom-chrome, 0px) + 18px + var(--lift, 0px)); z-index: 2; margin: auto 0 18px;
    display: grid; grid-template-columns: minmax(0, 1fr) auto;
    border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card-raised); box-shadow: var(--shadow-dock);
    line-height: 1.5;
  }
  fieldset { display: contents; margin: 0; padding: 0; border: 0; min-width: 0; }
  .ask { grid-column: 1; grid-row: 1; padding: 14px 0 8px 18px; min-width: 0; display: flex; }
  .go { grid-column: 2; grid-row: 1; display: flex; align-items: flex-start; gap: 10px; padding: 14px 18px 8px 12px; }
  .under { grid-column: 1 / -1; grid-row: 2; display: flex; align-items: center; flex-wrap: wrap; gap: 6px 12px; padding: 0 18px 8px; }
  .line { grid-column: 1 / -1; grid-row: 3; display: flex; align-items: center; gap: 8px 16px; flex-wrap: wrap; padding: 8px 18px 14px; border-top: 1px solid var(--line-soft); }
  .checkout { grid-column: 1 / -1; grid-row: 4; margin: -8px 0 0; padding: 0 18px 12px; font-size: 13px; color: var(--muted); }
  .summary, .tablet-change { display: none; }
  .titled { margin: 0; max-width: 100%; font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { grid-column: 1 / -1; grid-row: 5; margin: 0; padding: 0 18px 12px; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .status:empty { display: none; }
  .status.bad { color: var(--bad); }

  textarea {
    flex: 1; min-width: 0; min-height: 24px; max-height: 168px; padding: 0; border: 0; outline: none; resize: none; background: transparent;
    color: var(--ink); font: 400 16px/1.5 var(--sans); field-sizing: content; overflow-y: auto;
  }
  textarea::placeholder { color: var(--faint); opacity: 1; }
  /* The prompt has no box of its own (the bar is its frame): focus draws a line under the field. */
  textarea:focus-visible { box-shadow: inset 0 -2px 0 var(--accent-text); }
  textarea:hover { border: 0; }
  .chip { min-height: 30px; padding: 2px 12px; border: 1px solid var(--line-mid); border-radius: 99px; background: transparent; color: var(--ink); font: inherit; font-size: 13px; font-weight: 500; cursor: pointer; }
  .chip:hover { border-color: var(--muted); }
  .secrets { margin: 0; font-size: 12px; color: var(--muted); }

  .start {
    display: flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border: 0; border-radius: 10px;
    background: var(--accent); color: var(--accent-ink); font: inherit; font-weight: 600; cursor: pointer; white-space: nowrap;
  }
  .start:disabled { opacity: 0.55; cursor: default; }
  /* The press the other buttons give (app.css .btn), only when motion is welcome. */
  @media (prefers-reduced-motion: no-preference) {
    .start, .chip, .discard { transition: background-color 0.15s, border-color 0.15s, transform 160ms var(--ease-out); }
    .start:active:not(:disabled), .chip:active, .discard:active { transform: scale(0.97); }
  }
  /* The mock-up dims the hint to 80%; it stays at full strength so it keeps AA contrast on the accent. */
  .start kbd { font: 11px var(--mono); }
  .start .arrow { display: none; width: 20px; height: 20px; }
  .discard { height: 40px; padding: 0 12px; border: 1px solid var(--line-strong); border-radius: 10px; background: transparent; color: var(--ink); font: inherit; cursor: pointer; }

  .sentence { flex: 0 1 auto; display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 8px; min-width: 0; margin: 0; font-size: 14px; color: var(--muted); }
  .part { overflow-wrap: anywhere; }
  /* A slot is a button drawn as a bold word with a dashed underline (the mock-up's border-bottom, kept on the text so a
     finger-sized button on touch does not move it). The machine that follows its project is bold without the underline. */
  .slot, .fixed {
    padding: 0; border: 0; background: none; color: var(--ink); font: inherit; font-weight: 600; cursor: pointer; text-align: left;
    text-decoration: underline dashed var(--line-deep) 1px; text-underline-offset: 5px;
  }
  /* At least 24 px to click (WCAG 2.5.8) without moving the line. */
  .slot { display: inline-flex; align-items: center; min-height: 24px; margin-block: -2px; }
  .fixed { text-decoration: none; cursor: default; }
  .slot:hover:not(:disabled) { text-decoration-color: var(--ink); }
  .slot[aria-expanded='true'] { text-decoration-style: solid; text-decoration-color: var(--accent-text); }
  .slot:disabled { cursor: default; }

  .approvals { display: flex; align-items: center; gap: 10px; margin-left: auto; font-size: 14px; }
  .lead { color: var(--muted); }
  .segments { display: flex; border: 1px solid var(--line-strong); border-radius: 9px; overflow: hidden; font-size: 13px; line-height: 1.3; }
  .segments label { position: relative; display: flex; align-items: center; padding: 6px 12px; color: var(--muted); cursor: pointer; }
  .segments label.on { background: var(--line-soft); color: var(--ink); font-weight: 600; }
  .segments label.off { cursor: default; }
  .segments label:has(:focus-visible) { outline: 2px solid var(--accent); outline-offset: -2px; }
  .segments input { position: absolute; inset: 0; opacity: 0; margin: 0; cursor: inherit; }

  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }

  /* The Change sheet: the slots as large rows, then Approvals and Done. */
  dialog { width: min(420px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; overflow: auto; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; min-height: 0; padding: 8px 8px 12px; border: 1px solid var(--line); border-radius: 16px; background: var(--card-raised); }
  .sheet header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 4px 4px 12px; }
  .sheet h2 { margin: 0; font: 600 1.15rem/1.3 var(--serif); }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--muted); cursor: pointer; }
  .close svg { width: 20px; height: 20px; }
  .rows { display: flex; flex-direction: column; gap: 2px; padding: 4px; overflow: auto; }
  .row { display: flex; align-items: center; gap: 12px; min-height: 52px; margin: 0; padding: 6px 12px; border: 0; border-radius: 10px; background: none; color: var(--ink); font: inherit; font-size: 15px; text-align: left; cursor: pointer; }
  button.row:hover { background: var(--line-soft); }
  .row.fixed { cursor: default; }
  .row-label { flex: none; width: 84px; color: var(--muted); font-size: 14px; }
  .row-value { flex: 1; min-width: 0; font-weight: 600; overflow-wrap: anywhere; }
  .chev { color: var(--muted); font-size: 20px; }
  .rows .checkout { margin: 0; padding: 4px 12px; }
  .rows .approvals { margin: 8px 12px 0; }
  .rows .approvals .lead { display: inline; width: 72px; font-size: 14px; }
  .rows .segments label { min-height: 44px; padding: 0 16px; }
  .sheet footer { display: flex; justify-content: flex-end; padding: 8px 4px 0; }
  /* The status sheet of a start in several projects: no drawing, so the Change sheet's card, type and spacing. */
  .run-body { display: flex; flex-direction: column; gap: 8px; padding: 4px 12px; overflow: auto; }
  .started { margin: 0; font: 600 15px/1.4 var(--sans); }
  .means { margin: 0; font-size: 12px; color: var(--muted); }
  .members { list-style: none; margin: 4px 0 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .member { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 2px 12px; min-height: 52px; padding: 6px 0; border-top: 1px solid var(--line-soft); }
  .member-head { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
  .member-name { min-width: 0; overflow-wrap: anywhere; font-weight: 600; }
  .member-machine { flex: none; font-size: 12px; color: var(--muted); }
  .member-line { grid-column: 1; margin: 0; font-size: 13px; color: var(--muted); overflow-wrap: anywhere; }
  .member.failed .member-line { color: var(--bad); }
  .member.started .member-line { color: var(--good); }
  .member-act { grid-column: 2; grid-row: 1 / span 2; display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 64px; padding: 0 14px;
    border: 1px solid var(--line-strong); border-radius: 10px; background: transparent; color: var(--ink); font: inherit; font-size: 14px; font-weight: 500; text-decoration: none; cursor: pointer; }
  .member-act:disabled { opacity: 0.55; cursor: default; }
  .done { min-height: 48px; padding: 0 24px; border: 0; border-radius: 10px; background: var(--accent); color: var(--accent-ink); font: inherit; font-weight: 600; cursor: pointer; }

  @media (pointer: coarse) {
    .start kbd { display: none; }
    .start, .discard { height: 44px; }
    .chip { min-height: 44px; }
    .segments label { min-height: 44px; padding: 0 14px; }
    /* Finger-sized slots that keep the sentence's line spacing: the extra height overlaps the gap between lines. */
    .slot { display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 44px; margin-block: -10px; }
    .sentence { row-gap: 14px; }
  }
  /* Tablet: the mock-up's 44 px Start without the key hint, the same two rows. */
  @media (max-width: 1099px) {
    .start { height: 44px; padding: 0 22px; }
    .line { padding-bottom: 12px; }
    .approvals .lead { display: none; }
  }
  /* Tablet only: the drawing's shorter sentence, with Change for the rest (owner ruling, 27 Oct). */
  @media (min-width: 651px) and (max-width: 1099px) {
    .sentence .tablet-hide { display: none; }
    .tablet-change { display: inline-flex; align-items: center; min-height: 44px; padding: 0 8px; border: 0; background: none; color: var(--ink-soft); font: inherit; font-weight: 500; cursor: pointer; }
  }
  /* Phone: a one-line prompt with a round 48 px Start, and the sentence folded into one summary line with Change. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .launch { margin: auto -4px 8px; bottom: calc(var(--bottom-chrome, 0px) + 8px + var(--lift, 0px)); box-shadow: 0 -6px 24px var(--scrim); }
    .launch.lifted { bottom: calc(var(--lift, 0px) + 4px); }
    .ask { padding: 12px 0 12px 16px; align-items: center; }
    textarea { max-height: 96px; }
    .go { padding: 6px 6px 6px 8px; align-items: center; }
    .start { width: 48px; height: 48px; padding: 0; justify-content: center; border-radius: 50%; }
    .start .word, .start kbd { display: none; }
    .start .arrow { display: block; }
    .discard { height: 48px; }
    .line, .checkout { display: none; }
    .under { padding: 0 16px 8px; }
    .summary { grid-column: 1 / -1; grid-row: 4; display: flex; align-items: center; gap: 8px; min-height: 44px; padding: 4px 8px 4px 16px; border-top: 1px solid var(--line-soft); font-size: 13px; color: var(--muted); }
    .summary-text { flex: 1; min-width: 0; margin: 0; overflow-wrap: anywhere; }
    .summary-text b { color: var(--ink); font-weight: 600; }
    .change { flex: none; min-height: 44px; min-width: 44px; padding: 0 8px; border: 0; background: none; color: var(--ink-soft); font: inherit; font-weight: 500; cursor: pointer; }
    .status { padding: 0 16px 10px; }
  }
</style>
