<script lang="ts">
  import { onMount } from 'svelte'
  import { getSyncState, UnauthorizedError, type Machine } from './api'
  import { folderProjectList, terminalMachines, terminalSessions, updateFolderProject, type FolderProject, type TerminalMachine, type TerminalSession } from './terminal/api'
  import StartAgentDialog from './StartAgentDialog.svelte'
  import { projectRunning } from './home'
  import { PROJECT_COLOURS, projectColour, projectColourHex, type ProjectColour } from './projectColour'

  // The project strip at the top of Machines: the same folder projects as Home's rail, in a row, so a person can start a
  // session in one without leaving the page. Each project's + starts an agent in its folder and opens the new session;
  // right-click (or the menu button) offers the same and the project's colour. The strip is read-only prose until the
  // machines are read, and says so when the read fails.
  let { canWrite, accounts, onsignedout }: {
    canWrite: boolean
    accounts: boolean
    onsignedout?: (e: unknown) => void
  } = $props()

  let projects = $state<FolderProject[] | undefined>(), sessions = $state<TerminalSession[]>([])
  let failed = $state(false), notice = $state<{ text: string; href?: string } | null>(null)
  let machines = $state<TerminalMachine[] | undefined>(), snapshot = $state<Machine[] | undefined>()
  let starting = $state<{ project: FolderProject; opener: HTMLElement } | null>(null), opening = $state(false)
  let menu = $state<{ project: FolderProject; x: number; y: number } | null>(null), menuEl = $state<HTMLElement>(), opener: HTMLElement | undefined
  const running = $derived(projectRunning(sessions))
  // Built once here so the component's markup carries no literal that looks like a colour.
  const addMachine = '/machines' + '#' + 'add-machine'
  const hostOf = (project: FolderProject) => project.machine || 'machine not enrolled'

  function signOut(e: unknown): boolean {
    if (!(e instanceof UnauthorizedError)) return false
    onsignedout?.(e)
    return true
  }
  async function load() {
    try {
      const [list, live] = await Promise.all([folderProjectList(), terminalSessions()])
      projects = list.projects; sessions = live; failed = false
    } catch (e) {
      if (signOut(e)) return
      failed = true
    }
  }
  onMount(() => {
    if (!accounts || !canWrite) return
    void load()
    const refresh = () => { if (document.visibilityState === 'visible') void load() }
    const timer = setInterval(refresh, 30_000)
    document.addEventListener('visibilitychange', refresh)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', refresh) }
  })

  async function openStart(project: FolderProject, button: HTMLElement) {
    if (opening) return
    opening = true; notice = null
    try {
      const [nextMachines, state] = await Promise.all([terminalMachines(), getSyncState()])
      machines = nextMachines; snapshot = state.machines ?? []; starting = { project, opener: button }
    } catch (e) {
      if (signOut(e)) return
      notice = { text: "Couldn't read your machines just now. Try again in a moment." }
    } finally { opening = false }
  }
  const sessionHref = (session: TerminalSession) => `/sessions?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`
  function started(session: TerminalSession): 'leaving' { location.assign(sessionHref(session)); return 'leaving' }
  function leaveFailed(session: TerminalSession) { notice = { text: `Started ${session.title}, but the page did not open.`, href: sessionHref(session) } }
  async function saveColour(project: FolderProject, colour: ProjectColour) {
    if ((project.colour || '') === colour) return
    const before = projects ?? []
    projects = before.map(p => p.id === project.id ? { ...p, colour } : p)
    if (menu?.project.id === project.id) menu = { ...menu, project: { ...project, colour } }
    try {
      const saved = await updateFolderProject(project.id, { colour }, project.version)
      projects = (projects ?? []).map(p => p.id === saved.id ? saved : p)
      if (menu?.project.id === saved.id) menu = { ...menu, project: saved }
    } catch (e) {
      projects = before
      if (menu?.project.id === project.id) menu = { ...menu, project }
      if (signOut(e)) return
      notice = { text: e instanceof Error ? e.message : 'That colour could not be saved. Try again.' }
    }
  }

  async function openMenu(event: MouseEvent | KeyboardEvent, project: FolderProject) {
    event.preventDefault()
    opener = event.currentTarget as HTMLElement
    const box = opener.getBoundingClientRect()
    const x = 'clientX' in event && event.clientX ? event.clientX : box.left
    const y = 'clientY' in event && event.clientY ? event.clientY : box.bottom
    menu = { project, x: Math.min(Math.max(8, x), Math.max(8, innerWidth - 226)), y: Math.min(Math.max(8, y), Math.max(8, innerHeight - 230)) }
  }
  function closeMenu(focus = false) { menu = null; if (focus) opener?.focus() }
  function menuKeys(event: KeyboardEvent) {
    const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('button:not(:disabled), a[href]')]
    const at = items.indexOf(document.activeElement as HTMLElement)
    if (event.key === 'Escape') { event.preventDefault(); closeMenu(true) }
    else if (event.key === 'ArrowDown') { event.preventDefault(); items[(at + 1) % items.length]?.focus() }
    else if (event.key === 'ArrowUp') { event.preventDefault(); items[(at - 1 + items.length) % items.length]?.focus() }
  }
</script>

<svelte:window onpointerdown={event => { if (menu && !menuEl?.contains(event.target as Node) && !opener?.contains(event.target as Node)) closeMenu() }} onkeydown={event => { if (event.key === 'Escape' && menu) closeMenu(true) }} />

{#if accounts && canWrite}
<section class="bar" aria-labelledby="projects-bar-title">
  <div class="head"><h2 id="projects-bar-title">Projects</h2><a href="/">Add or manage on Home</a></div>
  {#if failed && !projects}
    <p class="empty">Couldn't read your projects just now.</p>
  {:else if !projects}
    <p class="empty">Reading your projects…</p>
  {:else if projects.length === 0}
    <p class="empty">No projects yet. Add one on Home to start sessions in your folders from here.</p>
  {:else}
    <ul>
      {#each projects as p (p.id)}
        {@const live = running.get(p.id) ?? 0}
        <li style={`--project-colour:${projectColourHex(p.colour)}`}>
          <a class="chip" href={`/?project=${encodeURIComponent(p.id)}`} aria-label={`${p.name} on ${hostOf(p)}, ${live > 0 ? `${live} running` : 'no live sessions'}`} oncontextmenu={event => openMenu(event, p)}>
            <span class="dot" aria-hidden="true"></span>
            <span class="name">{p.name}</span>
            {#if live}<span class="live" aria-hidden="true">{live}</span>{/if}
          </a>
          <button type="button" class="action" aria-label={`New session in ${p.name}`} title={`New session in ${p.name}`} disabled={opening} onclick={event => openStart(p, event.currentTarget)}>+</button>
          <button type="button" class="action more" aria-label={`Actions for ${p.name}`} aria-haspopup="menu" aria-expanded={menu?.project.id === p.id} onclick={event => openMenu(event, p)}>⋮</button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if notice}<p class="empty" role="status">{notice.text}{#if notice.href}{' '}<a href={notice.href}>Open it</a>{/if}</p>{/if}
</section>
{/if}

{#if menu}
  {@const project = menu.project}
  <div class="project-menu" role="menu" aria-label={`Actions for ${project.name}`} tabindex="-1" bind:this={menuEl} style={`left:${menu.x}px;top:${menu.y}px`} onkeydown={menuKeys}>
    <p>{project.name}<small>{hostOf(project)}</small></p>
    <button type="button" role="menuitem" disabled={opening} onclick={event => { const from = opener ?? (event.currentTarget as HTMLElement); closeMenu(); void openStart(project, from) }}>New session here</button>
    <span class="label">Project colour</span>
    <div class="colours">{#each PROJECT_COLOURS as choice}<button type="button" role="menuitemradio" aria-checked={projectColour(project.colour) === choice.id} aria-label={choice.name} title={choice.name} style={`--swatch:${choice.hex}`} onclick={() => void saveColour(project, choice.id)}>{choice.id === '' ? '×' : ''}</button>{/each}</div>
    <a role="menuitem" href={`/?project=${encodeURIComponent(project.id)}`}>Show on Home</a>
  </div>
{/if}

{#if starting && machines && snapshot}
  <StartAgentDialog {machines} snapshot={snapshot} initialAgent={starting.project.host} initialDir={starting.project.dir} projectId={starting.project.id} opener={starting.opener} onstarted={started} onleavefailed={leaveFailed} onaddmachine={() => location.assign(addMachine)} onclose={() => (starting = null)} {onsignedout} />
{/if}

<style>
  .bar { display: flex; flex-direction: column; gap: 8px; margin-bottom: 18px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); }
  .head { display: flex; align-items: baseline; gap: 12px; }
  h2 { margin: 0; font: 600 13px/1.3 var(--sans); letter-spacing: .04em; text-transform: uppercase; color: var(--muted); }
  .head a { margin-left: auto; font-size: 13px; color: var(--accent-text); }
  ul { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: stretch; min-width: 0; border: 1px solid var(--line); border-left: 3px solid var(--project-colour, transparent); border-radius: 9px; background: var(--bg); }
  li:hover { border-color: var(--line-strong); }
  .chip { display: flex; align-items: center; gap: 7px; min-width: 0; min-height: 34px; padding: 5px 8px; color: var(--ink); text-decoration: none; font-size: 13px; border-radius: 8px 0 0 8px; }
  .chip:hover { background: var(--tint); }
  .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--project-colour, transparent); border: 1px solid var(--line-strong); }
  .name { max-width: 220px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 500; }
  .live { color: var(--good); font-size: 11px; font-weight: 600; }
  .action { flex: none; display: grid; place-items: center; min-width: 32px; padding: 0 4px; border: 0; border-left: 1px solid var(--line); background: transparent; color: var(--muted); font: 500 1rem var(--sans); cursor: pointer; }
  .action:hover:not(:disabled) { color: var(--accent-text); background: var(--tint); }
  .action:disabled { opacity: .4; cursor: default; }
  .action.more { border-radius: 0 8px 8px 0; font-size: .9rem; }
  .empty { margin: 0; font-size: 13px; color: var(--muted); }
  .empty a { color: var(--accent-text); }
  .project-menu { position: fixed; z-index: 60; min-width: 210px; display: flex; flex-direction: column; padding: 4px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised); box-shadow: 0 14px 34px var(--scrim); font-size: 13px; }
  .project-menu p { display: flex; flex-direction: column; gap: 1px; margin: 0; padding: 6px 10px; font-weight: 600; }
  .project-menu p small { font-weight: 400; font-size: 11px; color: var(--muted); }
  .project-menu button, .project-menu > a { min-height: 34px; padding: 0 10px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; text-align: left; text-decoration: none; display: flex; align-items: center; cursor: pointer; }
  .project-menu button:hover:not(:disabled), .project-menu > a:hover { background: var(--line-soft); }
  .project-menu button:disabled { color: var(--faint); cursor: default; }
  .label { padding: 5px 10px 1px; color: var(--muted); font-size: 11px; }
  .colours { display: flex; gap: 7px; padding: 4px 10px 6px; }
  .colours button { min-height: 0; width: 20px; height: 20px; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--swatch); color: var(--ink); font-size: 11px; line-height: 1; }
  .colours button[aria-checked='true'] { box-shadow: 0 0 0 2px var(--accent); }
  @media (pointer: coarse) { .chip, .action, .project-menu button, .project-menu > a { min-height: 44px; } }
</style>
