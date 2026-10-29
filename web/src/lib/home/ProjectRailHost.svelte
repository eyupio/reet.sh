<script lang="ts">
  import { tick, type Snippet } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { UnauthorizedError } from '../api'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import { addedLine } from '../folderBrowser'
  import type { ProjectColour } from '../projectColour'
  import { deleteFolderProject, getFolderProject, importSavedProjects, refusalSentence, reorderFolderProjects, terminalMachines, updateFolderProject, TerminalError,
    type FolderProject, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import AddProjectDialog from './AddProjectDialog.svelte'
  import ProjectRail from './ProjectRail.svelte'
  import ProjectWorktrees from './ProjectWorktrees.svelte'

  // The project rail with everything its cards and menus do: Add and Edit (the folder dialog), Remove from reet (confirmed),
  // Move up and down, the project colour, the migration note and the waiting folders, and Worktrees. Home holds it on a
  // desktop; the Sessions screen holds it on a tablet or phone (the phone rail, PR H). The page owns the list it read and is
  // handed every write's answer (`onsettle`), so a poll on its way never overwrites it; sentences go to the page's own live
  // region (`onnotice`). Nothing is stored in the browser.
  let { projects, failed = false, savedWaiting = 0, sessions, sessionsFailed = false, reporting, running, known, worktrees, chosen = '',
    selecting = false, selected = [], onselecting, ontoggle, onchoose, onnew, signal, onsettle, onwaiting, onmachines, onnotice, onreload, onsignedout,
    variant = 'home', selectAction, rail = true }: {
    projects: FolderProject[] | undefined
    failed?: boolean
    savedWaiting?: number
    sessions: TerminalSession[] | undefined
    sessionsFailed?: boolean
    reporting: Map<string, boolean>
    running: Map<string, number>
    known: boolean
    worktrees: Map<string, number>
    chosen?: string
    selecting?: boolean
    selected?: readonly string[]
    onselecting?: (on: boolean, first?: FolderProject) => void
    ontoggle?: (project: FolderProject) => void
    onchoose: (project: FolderProject, opener: HTMLElement) => void
    /** Starts a session in a project at once (the card's +, and New session here). Without it the card chooses the project. */
    onnew?: (project: FolderProject, opener: HTMLElement) => void
    /** A bounded abort signal for each request (the page's life and its timeout). */
    signal: () => AbortSignal
    /** A write's answer: the list as it now is. */
    onsettle: (projects: FolderProject[]) => void
    onwaiting?: (count: number) => void
    /** The terminal machines read again for the folder dialog. */
    onmachines?: (machines: TerminalMachine[]) => void
    onnotice: (notice: { text: string; bad?: boolean } | null) => void
    onreload: () => void
    onsignedout: () => void
    /** 'sessions': the phone Sessions screen's heading ("Projects · N" and Add a project). */
    variant?: 'home' | 'sessions'
    /** What Select leads to where there is no launch bar (the Sessions screen). */
    selectAction?: Snippet
    /** False where the page shows no rail (Home on a tablet or phone) but still adds projects (the launch bar's slot). */
    rail?: boolean
  } = $props()

  const refused = (e: unknown) => e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)
  const signOut = () => onsignedout()

  let terminals = $state<TerminalMachine[] | undefined>()
  let checking = $state(false), importing = $state(false), moving = $state(false)
  // One change at a time: a project with a dismiss or remove on its way, and a move on its way, ignore a second press, so
  // the second never meets the first one's new version as "someone changed this project".
  const pending = new SvelteSet<string>()
  // The Add project dialog: a new project, or the one being changed (read with its starter prompt).
  let adding = $state<{ opener: HTMLElement | null; project?: FolderProject } | null>(null)
  let removing = $state<{ opener: HTMLElement | null; project: FolderProject } | null>(null)
  let listing = $state<{ opener: HTMLElement | null; project: FolderProject } | null>(null)

  // Read again on every open of Add project, never from an earlier one: a machine's browse capability can change while the
  // page stays open. `checking` stops a second open racing the first read.
  async function machinesForDialog(): Promise<boolean> {
    checking = true
    try { terminals = await terminalMachines(signal()); onmachines?.(terminals); return true }
    catch (e) {
      if (refused(e)) { signOut(); return false }
      onnotice({ text: "Couldn't read your machines just now. Try again in a moment.", bad: true })
      return false
    } finally { checking = false }
  }
  /** Add a project, from the rail or from elsewhere on the page (the launch bar's project slot). */
  export async function openAdd(opener: HTMLElement) {
    if (checking) return
    onnotice(null)
    if (await machinesForDialog()) adding = { opener }
  }
  // Edit reads the project again with its starter prompt (only the single read carries it), then opens the dialog.
  async function openEdit(project: FolderProject, opener: HTMLElement) {
    if (checking) return
    onnotice(null)
    checking = true
    try {
      const [machines, full] = await Promise.all([terminalMachines(signal()), getFolderProject(project.id, signal())])
      terminals = machines
      onmachines?.(machines)
      adding = { opener, project: full }
    } catch (e) {
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
      if (e instanceof TerminalError && e.status === 404) onreload()
    } finally { checking = false }
  }
  function added(p: FolderProject, wasEdit: boolean) {
    onsettle([...(projects ?? []).filter(x => x.id !== p.id), p].sort((a, b) => a.position - b.position))
    onnotice({ text: wasEdit ? `Sorted. ${p.name} saved.` : addedLine(p.name) })
  }
  // After a remove the focus goes to the card now in its place (or the one before, or Add); a refused remove gives it back
  // to the control that opened the confirmation.
  async function remove(project: FolderProject, opener: HTMLElement | null) {
    if (pending.has(project.id)) return
    pending.add(project.id)
    onnotice(null)
    const at = (projects ?? []).findIndex(p => p.id === project.id)
    try {
      const answer = await deleteFolderProject(project.id, project.version, signal())
      onsettle((projects ?? []).filter(p => p.id !== project.id))
      onnotice({ text: answer.message })
      await tick()
      const shown = (el: HTMLElement) => el.getClientRects().length > 0
      const cards = [...document.querySelectorAll<HTMLElement>('aside.rail .card, aside.rail .mini')].filter(shown)
      const add = [...document.querySelectorAll<HTMLElement>('aside.rail button[aria-label="Add a project"]')].find(shown)
      ;(cards[Math.min(Math.max(at, 0), cards.length - 1)] ?? add)?.focus()
    } catch (e) {
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
      if (opener?.isConnected) opener.focus()
      onreload()
    } finally { pending.delete(project.id) }
  }
  // Moving a card sends the whole order with the version of each project as read; a changed list is read again.
  async function move(project: FolderProject, by: -1 | 1) {
    // The order names every project's version as read: a dismiss or remove on its way would make it stale.
    if (moving || pending.size > 0) return
    const list = [...(projects ?? [])]
    const at = list.findIndex(p => p.id === project.id), to = at + by
    if (at < 0 || to < 0 || to >= list.length) return
    ;[list[at], list[to]] = [list[to], list[at]]
    onnotice(null)
    moving = true
    try {
      onsettle(await reorderFolderProjects(list.map(p => p.id), list.map(p => p.version), signal()))
    } catch (e) {
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
      onreload()
    } finally { moving = false }
  }
  // A rail colour is saved on the project, so it follows the person to every browser. The card shows it at once; a
  // refusal puts the project back and says why.
  async function colour(project: FolderProject, choice: ProjectColour) {
    if (pending.has(project.id) || (project.colour || '') === choice) return
    pending.add(project.id)
    onnotice(null)
    const before = projects ?? []
    onsettle(before.map(p => p.id === project.id ? { ...p, colour: choice } : p))
    try {
      const saved = await updateFolderProject(project.id, { colour: choice }, project.version, signal())
      onsettle((projects ?? []).map(p => p.id === saved.id ? saved : p))
    } catch (e) {
      onsettle(before)
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
      onreload()
    } finally { pending.delete(project.id) }
  }
  // The migration's note goes once the person has read it: clearing it is the one change made.
  async function dismiss(project: FolderProject) {
    if (pending.has(project.id)) return
    pending.add(project.id)
    try {
      const saved = await updateFolderProject(project.id, { note: '' }, project.version, signal())
      onsettle((projects ?? []).map(p => p.id === saved.id ? saved : p))
    } catch (e) {
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
      onreload()
    } finally { pending.delete(project.id) }
  }
  // The saved Project folders that waited for room come in only when the person asks; the answer is said as it came.
  async function bringIn() {
    if (importing) return
    importing = true; onnotice(null)
    try {
      const answer = await importSavedProjects(signal())
      onsettle(answer.projects)
      onwaiting?.(answer.savedWaiting)
      onnotice({ text: answer.message })
    } catch (e) {
      if (refused(e)) { signOut(); return }
      onnotice({ text: refusalSentence(e), bad: true })
    } finally { importing = false }
  }
</script>

{#if rail}<ProjectRail {projects} {failed} {reporting} {running} {known} {worktrees} {savedWaiting} {chosen} busy={checking} {importing} {pending} {moving}
  {selecting} {selected} {onselecting} {ontoggle} {variant} {selectAction}
  {onnew} onadd={(opener) => void openAdd(opener)} {onchoose} onedit={openEdit} onworktrees={(project, opener) => (listing = { project, opener })}
  onremove={(project, opener) => (removing = { project, opener })} onmove={move} ondismiss={dismiss} onimport={bringIn} oncolour={colour} />{/if}

{#if adding && terminals}
  {@const wasEdit = !!adding.project}
  <AddProjectDialog machines={terminals} project={adding.project} opener={adding.opener} onadded={(p) => added(p, wasEdit)} onsignedout={signOut} onclose={() => (adding = null)} />
{/if}
{#if removing}
  {@const p = removing.project}
  <ConfirmDialog title={`Remove ${p.name} from reet?`} body={`It leaves the rail. Its sessions and any worktrees stay on ${p.machine || 'its machine'}, and the folder isn't touched.`}
    confirmLabel="Remove from reet" danger onconfirm={() => { const target = p, opener = removing?.opener ?? null; removing = null; void remove(target, opener) }} oncancel={() => { const o = removing?.opener; removing = null; o?.focus() }} />
{/if}
{#if listing}
  <ProjectWorktrees project={listing.project} {sessions} {sessionsFailed} opener={listing.opener} onsignedout={signOut} onclose={() => (listing = null)} />
{/if}
