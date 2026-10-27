<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { shell } from '../dialogShell'
  import { UnauthorizedError } from '../api'
  import { listProjects, projectRuns, TerminalError, type FolderProject, type TerminalSession } from '../terminal/api'
  import { cleanPath, removeOffer, type RunMember } from '../projects'
  import WorktreeDialog from '../WorktreeDialog.svelte'

  // The worktrees reet has a record of for one folder project: the ones its live sessions are working in now, and the ones
  // old Project runs made in the same folder on the same machine, each with the existing Check and remove sheet (which looks
  // at the worktree first and removes nothing unsaved). A worktree with no record here is not listed: reet never guesses.
  let { project, sessions, sessionsFailed = false, opener = null, onsignedout, onclose }: {
    project: FolderProject
    /** The account's sessions as last read, or undefined while unknown (then nothing is offered for removal). */
    sessions: TerminalSession[] | undefined
    sessionsFailed?: boolean
    opener?: HTMLElement | null
    onsignedout?: (e: unknown) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  const life = new AbortController()
  let dialog = $state<HTMLDialogElement>(), closeButton = $state<HTMLButtonElement>()
  type Recorded = { member: RunMember; project: string; startedAt: number }
  let recorded = $state<Recorded[] | undefined>()
  let failed = $state('')
  // Saved Projects whose runs could not be read: the rest are still listed.
  let unread = $state<string[]>([])
  let removing = $state<{ member: RunMember; opener: HTMLElement } | null>(null)
  const host = $derived(project.machine || 'its machine')
  // Saved folders are matched as the server cleaned them (path.Clean): /a//b/ and /a/./b are /a/b.
  const clean = cleanPath
  const live = $derived((sessions ?? []).filter(s => s.projectId === project.id && s.worktreePath && ['creating', 'running', 'end_requested'].includes(s.state)))

  // Old runs are read through the saved Projects (read-only now): every run of a Project with a member in this folder.
  async function read() {
    failed = ''
    try {
      const saved = (await listProjects(life.signal)).filter(p => p.members.some(m => m.host === project.host && clean(m.dir) === project.dir))
      const read = await Promise.allSettled(saved.map(p => projectRuns(p.id, life.signal)))
      if (life.signal.aborted) return
      if (read.some(r => r.status === 'rejected' && r.reason instanceof TerminalError && r.reason.status === 401) && onsignedout) { life.abort(); onsignedout(new UnauthorizedError()); return }
      unread = saved.filter((_, i) => read[i].status === 'rejected').map(p => p.name)
      recorded = read.flatMap(r => r.status === 'fulfilled' ? r.value : []).flatMap(run => run.members
        .filter(m => m.host === project.host && clean(m.dir) === project.dir && m.worktreeRun && (m.state === 'started' || m.state === 'removed'))
        .map(member => ({ member, project: run.projectName, startedAt: run.startedAt })))
        .sort((a, b) => b.startedAt - a.startedAt)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401 && onsignedout) { life.abort(); onsignedout(new UnauthorizedError()); return }
      failed = "Couldn't read the old runs' worktrees just now."
      recorded = recorded ?? []
    }
  }
  function close() {
    life.abort()
    dialog?.close()
    if (opener?.isConnected) opener.focus()
    onclose()
  }
  const day = (at: number) => new Date(at).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })

  onMount(() => {
    dialog?.showModal()
    void tick().then(() => closeButton?.focus())
    void read()
    return () => life.abort()
  })
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
  <header data-shell-head>
    <div>
      <p class="eyebrow" data-shell-aside>{host}</p>
      <h2 id={`${id}-title`}>Worktrees for {project.name}</h2>
      <p class="lede" data-shell-aside>The worktrees reet has a record of in <span class="mono">{project.dir}</span>: those live sessions are in, and those made by the newest 20 runs of each saved Project.</p>
    </div>
    <button type="button" class="btn" bind:this={closeButton} onclick={close} aria-label="Close" data-shell-close>Close ×</button>
  </header>
  <div class="body" data-shell-body>
    {#if failed}<p class="bad" role="alert">{failed}</p>{/if}
    {#each unread as name (name)}<p class="bad" role="alert">Couldn't read the runs of {name} just now.</p>{/each}
    {#if !recorded && !failed}
      <p class="muted" role="status">Reading the worktrees…</p>
    {:else if live.length === 0 && (recorded ?? []).length === 0}
      <p class="muted">No worktrees are recorded for this project.</p>
    {:else}
      <ul>
        {#each live as s (s.id)}
          <li>
            <p class="mono">{s.worktreeBranch || 'branch not reported'}</p>
            <p class="meta mono">{s.worktreePath}</p>
            <p class="meta">In use by {s.title}</p>
            <a class="btn" href={`/terminals?agent=${encodeURIComponent(s.agentId)}&session=${encodeURIComponent(s.id)}`}>Open the session</a>
          </li>
        {/each}
        {#each recorded ?? [] as r (`${r.member.worktreeRun}:${r.member.position}`)}
          {@const offer = removeOffer(r.member, sessions, sessionsFailed)}
          <li>
            <p class="mono">{r.member.worktreeBranch || 'branch not reported'}</p>
            <p class="meta mono">{r.member.worktreePath || 'folder not reported'}</p>
            <p class="meta">{r.member.state === 'removed' ? 'Removed' : 'Made'} by a run of the saved Project {r.project}, started {day(r.startedAt)}</p>
            {#if offer.offer}
              <button type="button" class="btn" aria-label={`Check and remove ${r.member.worktreeBranch || r.member.worktreePath}`} onclick={(e) => (removing = { member: r.member, opener: e.currentTarget })}>Check and remove</button>
            {:else if offer.reason}<p class="meta">{offer.reason}</p>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</dialog>

{#if removing}
  <WorktreeDialog member={removing.member} host={host} opener={removing.opener} onremoved={() => void read()} {onsignedout} onclose={() => (removing = null)} />
{/if}

<style>
  dialog { width: min(600px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
  dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr); }
  dialog::backdrop { background: var(--scrim); }
  header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
  header > div { min-width: 0; }
  h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
  .eyebrow { margin: 0; color: var(--accent-text); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; overflow-wrap: anywhere; }
  .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; overflow-wrap: anywhere; }
  header .btn { flex: none; white-space: nowrap; }
  .body { display: grid; gap: 12px; padding: 22px; min-height: 0; overflow: auto; align-content: start; font-size: .88rem; line-height: 1.5; }
  ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  li { display: grid; gap: 2px; justify-items: start; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); min-width: 0; }
  li p { margin: 0; overflow-wrap: anywhere; }
  li .btn { margin-top: 6px; min-height: 40px; }
  .mono { font-family: var(--mono); font-size: .8rem; }
  .meta, .muted { color: var(--muted); margin: 0; }
  .bad { color: var(--bad); margin: 0; }
  @media (pointer: coarse) { li .btn { min-height: 44px; } }
  @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } h2 { font-size: 1.35rem; } }
</style>
