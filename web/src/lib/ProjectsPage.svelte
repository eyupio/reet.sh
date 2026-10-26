<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { getSyncState, UnauthorizedError, type Machine } from './api'
 import { listProjects, terminalMachines, TerminalError, type TerminalMachine } from './terminal/api'
 import { toolName } from './agentLaunch'
 import PageHeader from './PageHeader.svelte'
 import ProjectDialog from './ProjectDialog.svelte'
 import { MAX_PROJECTS, projectRefusalText, runStateLabel, type Project } from './projects'

 // The account's Projects: each with its repositories and its last run, and New project. Writers in accounts mode only;
 // the page that includes this decides that. Nothing here is kept in browser storage.
 let { onsignedout }: { onsignedout: (e: unknown) => void } = $props()

 let projects = $state<Project[] | undefined>()
 let machines = $state<TerminalMachine[]>([])
 let loadError = $state(''), message = $state('')
 let creating = $state<{ opener: HTMLElement | null } | null>(null)
 let folders = $state<Machine[] | undefined>()
 const life = new AbortController()

 const when = (ms: number) => new Date(ms).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
 const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host
 function hosts(p: Project): string {
  const names = [...new Set(p.members.map(m => hostOf(m.host) ?? 'a machine no longer on this account'))]
  return names.length === 0 ? '' : names.length <= 3 ? names.join(', ') : `${names.slice(0, 3).join(', ')} and ${names.length - 3} more`
 }
 function failedWith(e: unknown): boolean {
  if (e instanceof TerminalError && e.status === 401) { life.abort(); onsignedout(new UnauthorizedError()); return true }
  return false
 }

 async function load() {
  loadError = ''
  try {
   const [list, known] = await Promise.all([listProjects(life.signal), terminalMachines(life.signal).catch(() => [] as TerminalMachine[])])
   if (life.signal.aborted) return
   projects = list; machines = known
  } catch (e) {
   if (life.signal.aborted || failedWith(e)) return
   loadError = e instanceof TerminalError ? projectRefusalText(e.status, e.code, e.message) : "Couldn't load your Projects just now. Reload the page to try again."
  }
 }

 function openNew(opener: HTMLElement) {
  message = ''
  creating = { opener }
  folders = undefined
  // The folders the machines reported, for the folder choices. Without them a typed path still works.
  void getSyncState().then(s => { if (!life.signal.aborted) folders = s.machines ?? [] }).catch(() => {})
 }
 async function saved(p: Project) {
  projects = [p, ...(projects ?? []).filter(x => x.id !== p.id)]
  message = `Grand, ${p.name} is saved. Open it to start its repositories.`
  await tick()
 }

 onMount(() => { void load(); return () => life.abort() })
</script>

<PageHeader eyebrow="Projects" title="Projects." lede="Save a set of repositories once, then start an agent in each with one press. Right handy when the work spans more than one folder.">
 {#snippet actions()}{#if projects}<button type="button" class="btn primary" onclick={(e) => openNew(e.currentTarget)} disabled={projects.length >= MAX_PROJECTS}>New project</button>{/if}{/snippet}
</PageHeader>

<p class="notice good" role="status" aria-live="polite" hidden={!message}>{message}</p>
{#if loadError}
 <p class="notice bad" role="alert">{loadError}</p>
{:else if !projects}
 <p class="empty" role="status">Loading your Projects…</p>
{:else if projects.length === 0}
 <div class="none">
  <h2>No Projects yet.</h2>
  <p>A Project is a named set of repository folders on your machines, with the agent and model each one starts with. Press New project to make one.</p>
 </div>
{:else}
 {#if projects.length >= MAX_PROJECTS}<p class="hint">This account has {MAX_PROJECTS} Projects, the most it can have. Delete one to make another.</p>{/if}
 <ul class="projects">
  {#each projects as p (p.id)}
   <li>
    <h2><a href={`/projects/${encodeURIComponent(p.id)}`}>{p.name}</a></h2>
    <p class="meta">{p.members.length === 1 ? '1 repository' : `${p.members.length} repositories`}{p.members.length ? ` on ${hosts(p)}` : ''}</p>
    <p class="meta">{toolName(p.defaultTool)}{p.defaultModel ? ` · ${p.defaultModel}` : ''} · {p.useWorktrees ? 'Worktrees' : 'No worktrees'}</p>
    <p class="last">{#if p.lastRun}Last run {when(p.lastRun.startedAt)}: <span class="state" data-state={p.lastRun.state}>{runStateLabel(p.lastRun.state)}</span>{:else}Not run yet{/if}</p>
   </li>
  {/each}
 </ul>
{/if}

{#if creating}
 <ProjectDialog machines={machines} snapshot={folders} otherNames={(projects ?? []).map(p => p.name)} opener={creating.opener} onsaved={saved} {onsignedout} onclose={() => (creating = null)} />
{/if}

<style>
 .notice { margin: 0 0 16px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
 .notice.good { border-color: var(--good); }
 .notice.bad { border-color: var(--bad); }
 .notice[hidden] { display: none; }
 .empty, .hint { color: var(--muted); }
 .none { padding: 24px; border: 1px dashed var(--line); border-radius: var(--radius); background: var(--card); }
 .none h2 { margin: 0 0 6px; font: 500 1.3rem/1.3 var(--serif); }
 .none p { margin: 0; color: var(--muted); max-width: 60ch; }
 .projects { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr)); gap: 12px; }
 .projects li { display: grid; gap: 4px; align-content: start; padding: 16px 18px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); min-width: 0; overflow-wrap: anywhere; }
 .projects h2 { margin: 0 0 4px; font: 500 1.25rem/1.3 var(--serif); }
 .projects a { color: var(--ink); text-decoration-color: var(--accent); text-underline-offset: 3px; }
 .projects a:hover { color: var(--accent); }
 .meta, .last { margin: 0; font-size: .84rem; color: var(--muted); }
 .last { margin-top: 6px; color: var(--ink); }
 .state[data-state='complete'] { color: var(--good); }
 .state[data-state='partial'] { color: var(--warn); }
 .state[data-state='failed'] { color: var(--bad); }
</style>
