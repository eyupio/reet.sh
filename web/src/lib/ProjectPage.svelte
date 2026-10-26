<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { getSyncState, UnauthorizedError, type Machine } from './api'
 import { getProject, projectRuns, terminalMachines, terminalSessions, TerminalError, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { toolName } from './agentLaunch'
 import PageHeader from './PageHeader.svelte'
 import ProjectDialog from './ProjectDialog.svelte'
 import StartRunDialog from './StartRunDialog.svelte'
 import RunView from './RunView.svelte'
 import DeleteProjectDialog from './DeleteProjectDialog.svelte'
 import { projectRefusalText, resolvedModel, runStateLabel, type Project, type ProjectInput, type ProjectRun } from './projects'

 // One Project: its defaults and repositories, Start all, and its newest 20 runs. Writers in accounts mode only; the page
 // that includes this decides that. Only the Project id is in the address; nothing is kept in browser storage.
 let { id, onsignedout }: { id: string; onsignedout: (e: unknown) => void } = $props()

 let project = $state<Project | undefined>()
 let runs = $state<ProjectRun[]>([])
 let machines = $state<TerminalMachine[]>([])
 // The account's sessions: undefined until the first read succeeds, and again after a read fails. Unknown is never "ended".
 let sessions = $state<TerminalSession[] | undefined>(), sessionsFailed = $state(false)
 let loadError = $state(''), notFound = $state(false), message = $state(''), runsError = $state('')
 let deleted = $state<string>('')
 let editing = $state<{ opener: HTMLElement | null } | null>(null)
 let starting = $state<{ opener: HTMLElement | null } | null>(null)
 let deleting = $state<{ opener: HTMLElement | null } | null>(null)
 let folders = $state<Machine[] | undefined>()
 // Which runs are shown open. Decided once per run when it first appears (and for a run just started), so a run that
 // becomes pending again on Retry keeps its component, its focus and its status line.
 let expanded = $state<Record<string, boolean>>({})
 // Start all is reading the machines and sessions before it opens the sheet.
 let checking = $state(false)
 const life = new AbortController()

 const when = (ms: number) => new Date(ms).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
 const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host ?? 'A machine no longer on this account'
 function handOff(e: unknown): boolean {
  if (e instanceof TerminalError && e.status === 401) { life.abort(); onsignedout(new UnauthorizedError()); return true }
  return false
 }

 async function loadProject() {
  loadError = ''; notFound = false
  try {
   const next = await getProject(id, life.signal)
   if (!life.signal.aborted) project = next
  } catch (e) {
   if (life.signal.aborted || handOff(e)) return
   if (e instanceof TerminalError && e.status === 404) { notFound = true; return }
   loadError = e instanceof TerminalError ? projectRefusalText(e.status, e.code, e.message) : "Couldn't load this Project just now. Reload the page to try again."
  }
 }
 async function loadRuns() {
  try {
   const next = await projectRuns(id, life.signal)
   if (!life.signal.aborted) { remember(next); runs = next; runsError = '' }
  } catch (e) {
   if (life.signal.aborted || handOff(e)) return
   runsError = "Couldn't read this Project's runs just now. Reload the page to try again."
  }
 }
 function remember(list: ProjectRun[]) {
  list.forEach((run, i) => { if (!(run.id in expanded)) expanded[run.id] = i === 0 || run.members.some(m => m.state === 'pending') })
 }
 // The machines and sessions say what can start now and which worktrees may be removed. Read again every 15 seconds
 // while the tab is visible. A failed machines read keeps what was last known; a failed sessions read makes them unknown.
 // Each read gives up after 15 seconds, so one that never answers cannot hold the next ones (or Start all) back.
 let refreshing: Promise<boolean> | null = null
 /** True when both reads answered just now. */
 async function readMachines(): Promise<boolean> {
  const signal = AbortSignal.any([life.signal, AbortSignal.timeout(15000)])
  const [m, s] = await Promise.allSettled([terminalMachines(signal), terminalSessions(signal)])
  if (life.signal.aborted) return false
  for (const r of [m, s]) if (r.status === 'rejected' && handOff(r.reason)) return false
  if (m.status === 'fulfilled') machines = m.value
  if (s.status === 'fulfilled') { sessions = s.value; sessionsFailed = false } else { sessions = undefined; sessionsFailed = true }
  return m.status === 'fulfilled' && s.status === 'fulfilled'
 }
 async function refreshMachines(force = false): Promise<boolean> {
  if (!force && document.visibilityState !== 'visible') return false
  // A forced read never reuses one already under way: it may have started before something changed.
  if (refreshing) { if (!force) return false; await refreshing }
  const read = readMachines()
  refreshing = read
  try { return await read } finally { if (refreshing === read) refreshing = null }
 }

 function readFolders() {
  folders = undefined
  void getSyncState().then(s => { if (!life.signal.aborted) folders = s.machines ?? [] }).catch(() => {})
 }
 function openEdit(opener: HTMLElement) { message = ''; editing = { opener }; readFolders() }
 async function openStart(opener: HTMLElement) {
  if (checking) return
  checking = true
  message = 'Checking your machines and sessions…'
  // The sheet plans from the machines and sessions as they are now, read afresh; nothing is planned from an unknown list.
  const fresh = await refreshMachines(true)
  checking = false
  if (life.signal.aborted) return
  if (!fresh || !sessions) { message = "Couldn't check your machines and sessions just now, so Start all can't open. Try again in a moment."; return }
  message = ''
  starting = { opener }
 }
 function saved(p: Project, sent: ProjectInput) {
  // The save reply leaves the shared prompt out: keep the one that is now saved, then read the Project again.
  project = { ...p, sharedPrompt: sent.sharedPrompt ?? project?.sharedPrompt }
  message = `Saved. Runs already started keep what they used.`
  void getProject(id, life.signal).then(next => { if (!life.signal.aborted && project?.version === next.version) project = next }).catch(e => { if (!life.signal.aborted) handOff(e) })
 }
 // The sheet was made from a version someone has since changed: nothing started. Read it again and say so.
 async function sheetChanged() {
  await loadProject()
  if (life.signal.aborted) return
  message = 'This Project changed since that sheet was made, so nothing was started. Look it over, then press Start all again.'
  await tick()
  document.querySelector<HTMLElement>('[data-start-all]')?.focus()
 }
 async function reload() {
  await loadProject()
  if (project) message = 'Read the Project again. Make your change once more if it is still needed.'
 }
 async function runStarted(run: ProjectRun) {
  expanded[run.id] = true
  runs = [run, ...runs.filter(r => r.id !== run.id)].slice(0, 20)
  message = `Right, that's under way: ${run.members.length === 1 ? 'one repository' : `${run.members.length} repositories`} starting. Each one shows its own state below.`
  if (project) project = { ...project, lastRun: { id: run.id, state: run.state, startedAt: run.startedAt } }
  await tick()
  document.getElementById('project-runs-title')?.focus()
  void refreshMachines()
 }
 function runChanged(run: ProjectRun) {
  runs = runs.map(r => (r.id === run.id ? run : r))
  void refreshMachines()
 }

 onMount(() => {
  void loadProject(); void loadRuns(); void refreshMachines()
  const wake = () => void refreshMachines()
  document.addEventListener('visibilitychange', wake)
  const timer = setInterval(wake, 15000)
  return () => { life.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', wake) }
 })
</script>

{#if deleted}
 <PageHeader eyebrow="Projects" title="Project deleted." lede={deleted} />
 <p><a class="btn" href="/projects">Back to Projects</a></p>
{:else if notFound}
 <PageHeader eyebrow="Projects" title="Project not found." lede="That Project isn't on this account." />
 <p><a class="btn" href="/projects">Back to Projects</a></p>
{:else if loadError}
 <PageHeader eyebrow="Projects" title="Project" />
 <p class="notice bad" role="alert">{loadError}</p>
{:else if !project}
 <p class="empty" role="status">Loading the Project…</p>
{:else}
 <PageHeader eyebrow="Project" title={project.name} lede={project.members.length === 0 ? 'No repositories yet. Change the Project to add the folders an agent should work in.' : `${project.members.length === 1 ? 'One repository' : `${project.members.length} repositories`}, each started with ${toolName(project.defaultTool)} unless it says otherwise.`}>
  {#snippet actions()}
   <a class="btn quiet" href="/projects">All Projects</a>
   <button type="button" class="btn" onclick={(e) => openEdit(e.currentTarget)}>Change</button>
   <button type="button" class="btn danger" onclick={(e) => (deleting = { opener: e.currentTarget })}>Delete</button>
   <button type="button" class="btn primary" data-start-all onclick={(e) => openStart(e.currentTarget)} disabled={!project?.members.length || checking} aria-busy={checking}>Start all</button>
  {/snippet}
 </PageHeader>

 <p class="notice good" role="status" aria-live="polite" hidden={!message}>{message}</p>

 <div class="layout">
  <section aria-labelledby="project-defaults-title" class="panel">
   <h2 id="project-defaults-title">Defaults</h2>
   <dl>
    <div><dt>Agent</dt><dd>{toolName(project.defaultTool)}</dd></div>
    <div><dt>Model</dt><dd>{project.defaultModel || 'Agent default'}</dd></div>
    <div><dt>Shared prompt</dt><dd>{project.hasSharedPrompt ? "Set. It is sent as each repository's opening prompt and is visible to other users of each machine in its process list." : 'None'}</dd></div>
    <div><dt>Worktrees</dt><dd>{project.useWorktrees ? 'Each run gets its own git worktree and branch per repository, under ~/.reet/worktrees.' : 'None. Agents work in the saved folders themselves, so any other session there shares that checkout.'}</dd></div>
   </dl>
  </section>

  <section aria-labelledby="project-members-title" class="panel">
   <h2 id="project-members-title">Repositories</h2>
   {#if project.members.length === 0}
    <p class="muted">None yet.</p>
   {:else}
    <ol class="repos">
     {#each project.members as m (m.position)}
      <li>
       <p><strong>{hostOf(m.host)}</strong> <span class="mono">{m.dir}</span></p>
       <p class="muted">{m.tool ? toolName(m.tool) : `Project default (${toolName(project.defaultTool)})`} · {resolvedModel(project, m) || 'Agent default'}</p>
      </li>
     {/each}
    </ol>
   {/if}
  </section>
 </div>

 <section aria-labelledby="project-runs-title" class="runs">
  <h2 id="project-runs-title" tabindex="-1">Runs</h2>
  {#if runsError}<p class="notice bad" role="alert">{runsError}</p>{/if}
  {#if runs.length === 0 && !runsError}
   <p class="muted">No runs yet. Start all asks for a confirmation first, then starts each repository on its own.</p>
  {:else}
   <p class="muted small">The newest 20, newest first. Closing this page doesn't stop a run.</p>
   {#each runs as run (run.id)}
    <details class="run-card" bind:open={expanded[run.id]}>
     <summary><h3>Run of {when(run.startedAt)}</h3> <span class="state" data-state={run.state}>{runStateLabel(run.state)}</span></summary>
     <RunView {run} {machines} {sessions} {sessionsFailed} onchange={runChanged} {onsignedout} />
    </details>
   {/each}
  {/if}
 </section>

 {#if editing}
  <ProjectDialog {project} {machines} snapshot={folders} opener={editing.opener} onsaved={saved} onreload={() => void reload()} {onsignedout} onclose={() => (editing = null)} />
 {/if}
 {#if starting}
  <StartRunDialog {project} {machines} sessions={sessions ?? []} opener={starting.opener} onstarted={runStarted} onchanged={() => void sheetChanged()} onaddmachine={() => location.assign('/machines#add-machine')} {onsignedout} onclose={() => (starting = null)} />
 {/if}
 {#if deleting}
  <DeleteProjectDialog {project} opener={deleting.opener} ondeleted={(r) => (deleted = r.message || 'Its runs are kept.')} onreload={() => void reload()} {onsignedout} onclose={() => (deleting = null)} />
 {/if}
{/if}

<style>
 .notice { margin: 0 0 16px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
 .notice.good { border-color: var(--good); }
 .notice.bad { border-color: var(--bad); }
 .notice[hidden] { display: none; }
 .empty, .muted { color: var(--muted); }
 .small { margin: 0; font-size: .8rem; }
 .layout { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr); gap: 16px; margin-bottom: 28px; }
 .panel { padding: 16px 18px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); min-width: 0; overflow-wrap: anywhere; }
 h2 { margin: 0 0 10px; font: 500 1.25rem/1.3 var(--serif); }
 dl { display: grid; gap: 8px; margin: 0; font-size: .88rem; }
 dt { color: var(--muted); font-size: .78rem; text-transform: uppercase; letter-spacing: .06em; }
 dd { margin: 2px 0 0; line-height: 1.5; }
 .repos { margin: 0; padding-left: 1.4em; display: grid; gap: 8px; font-size: .88rem; }
 .repos p { margin: 0; line-height: 1.5; }
 .mono { font-family: var(--mono); font-size: .8rem; }
 .runs { display: grid; gap: 12px; }
 .runs > h2 { margin: 0; }
 .run-card { padding: 16px 18px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--bg); min-width: 0; }
 details.run-card > summary { cursor: pointer; display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 12px; min-height: 32px; overflow-wrap: anywhere; }
 summary h3 { display: inline; margin: 0; font: 500 1.2rem/1.3 var(--serif); }
 summary .state { padding: 2px 8px; border: 1px solid var(--line); border-radius: 999px; font-size: .75rem; color: var(--muted); }
 summary .state[data-state='complete'] { border-color: var(--good); color: var(--good); }
 summary .state[data-state='partial'] { border-color: var(--warn); color: var(--warn); }
 summary .state[data-state='failed'] { border-color: var(--bad); color: var(--bad); }
 details.run-card[open] > summary { margin-bottom: 10px; }
 @media (max-width: 800px) { .layout { grid-template-columns: minmax(0, 1fr); } }
</style>
