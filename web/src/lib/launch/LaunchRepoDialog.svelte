<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import RepoField from '../home/RepoField.svelte'
  import { cloneForStart } from '../cloneForStart'
  import { cloneFolder, parseGitHubRepo, repoProject } from '../githubRepo'
  import type { FolderProject, TerminalMachine } from '../terminal/api'

  // The launch bar's project slot, "A repository from GitHub…": a machine and a repository. A project that already is that
  // repository on that machine is used as it is; otherwise the machine clones it as a temporary project (removed with its
  // clone once nothing uses it and nothing would be lost), as Start an agent does. The bar then names it and the person
  // presses Start: nothing starts from here, because the prompt is usually still to be written.
  let { machines, projects, onchosen, oncancel, onsignedout }: {
    /** The machines that can clone from GitHub now. */
    machines: TerminalMachine[]
    projects: FolderProject[]
    onchosen: (project: FolderProject) => void
    oncancel: () => void
    onsignedout?: (e: unknown) => void
  } = $props()

  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let repoField = $state<{ focus: () => void }>()
  let host = $state(untrackedFirst())
  let repo = $state(''), left = $state(false)
  let working = $state(false), message = $state(''), failed = $state(false), repoRefused = $state('')
  const life = new AbortController()

  function untrackedFirst() { return machines.length === 1 ? machines[0].agentId : '' }
  const machine = $derived(machines.find(m => m.agentId === host))
  const parsed = $derived(parseGitHubRepo(repo))
  const known = $derived(parsed && machine ? repoProject(projects, { host: machine.agentId, repo: `${parsed.owner}/${parsed.repo}` }) : undefined)
  const repoProblem = $derived(repoRefused || (left && repo.trim() !== '' && !parsed ? 'Write the repository as owner/name, or paste its github.com link.' : ''))
  const ready = $derived(!!machine && !!parsed)

  onMount(() => {
    dialog?.showModal()
    repoField?.focus()
    return () => { life.abort(); dialog?.close() }
  })

  // Closing while it clones stops the waiting, never the clone: the project still appears when it is done.
  function cancel() { life.abort(); dialog?.close(); oncancel() }

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    left = true
    if (working || !ready || !machine) return
    working = true; failed = false; repoRefused = ''
    message = ''
    const answer = await cloneForStart(machine.agentId, machine.host, repo, projects, life.signal, {
      onwaiting: () => { if (!life.signal.aborted && parsed) message = `Cloning ${parsed.owner}/${parsed.repo} on ${machine.host}… This can take a few minutes; the bar names it when it's done.` },
    })
    if (life.signal.aborted) return
    working = false
    if ('project' in answer) { dialog?.close(); onchosen(answer.project); return }
    if ('signedout' in answer) { onsignedout?.(answer.signedout); return }
    failed = true
    message = answer.sentence
    if (answer.field === 'repo') { repoRefused = answer.sentence; repoField?.focus() }
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`} aria-busy={working}
  oncancel={(event) => { event.preventDefault(); cancel() }}>
  <form class="box" data-shell-card onsubmit={submit} novalidate>
    <header data-shell-head>
      <h2 id={`${id}-title`}>A repository from GitHub</h2>
    </header>
    <div class="body" data-shell-body>
      <fieldset disabled={working}>
        <div class="field">
          <label class="label" for={`${id}-machine`}>Machine</label>
          <div class="select-wrap">
            <select id={`${id}-machine`} bind:value={host} onchange={() => { repoRefused = ''; message = ''; failed = false }}>
              {#if host === ''}<option value="" disabled>Choose a machine</option>{/if}
              {#each machines as m (m.agentId)}<option value={m.agentId}>{m.host}</option>{/each}
            </select>
            <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
          </div>
        </div>
        <div class="field">
          <label class="label" for={`${id}-repo`}>Repository</label>
          <RepoField bind:this={repoField} id={`${id}-repo`} {machine} bind:value={repo} problem={repoProblem} disabled={working}
            onleave={() => (left = true)} oninput={() => { repoRefused = ''; if (failed) { message = ''; failed = false } }} onsignedout={(e) => onsignedout?.(e)} />
        </div>
        {#if parsed && machine}
          <p class="where">
            {#if known}Uses the project <b>{known.name}</b> on {machine.host}.
            {:else}reet clones it to <span class="path">{cloneFolder(parsed)}</span> on {machine.host} and removes it once nothing uses it and nothing would be lost.{/if}
          </p>
        {/if}
      </fieldset>
    </div>
    <footer data-shell-foot>
      <p class="status" class:bad={failed} class:quiet={!message} role="status" aria-live="polite" aria-atomic="true">{message}</p>
      <div class="actions">
        <button type="button" class="btn quiet" onclick={cancel}>{working ? 'Close' : 'Cancel'}</button>
        <button type="submit" class="btn primary" disabled={working || !ready} aria-busy={working}>{working ? 'Cloning…' : 'Use this repository'}</button>
      </div>
    </footer>
  </form>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; background: transparent; color: var(--ink); overflow: visible; }
  dialog::backdrop { background: var(--scrim); }
  .box { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 32px); border: 1px solid var(--line); border-radius: 16px; background: var(--card-raised); box-shadow: var(--shadow-dock, 0 18px 50px var(--scrim)); }
  header { padding: 18px 24px 0; }
  h2 { margin: 0; font: 500 22px/1.2 var(--serif); letter-spacing: -.02em; }
  .body { min-height: 0; overflow: auto; padding: 16px 24px; }
  fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: flex; flex-direction: column; gap: 14px; }
  .field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .label { color: var(--muted); font-size: 14px; }
  select { width: 100%; min-width: 0; height: 42px; padding: 0 34px 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--sans); appearance: none; cursor: pointer; }
  .select-wrap { position: relative; }
  .chev { position: absolute; right: 12px; top: 14px; width: 14px; height: 14px; color: var(--muted); pointer-events: none; }
  .where { margin: 0; font-size: 14px; line-height: 1.45; color: var(--ink); overflow-wrap: anywhere; }
  .path { font-family: var(--mono); }
  footer { display: flex; align-items: center; gap: 10px; padding: 16px 24px; border-top: 1px solid var(--line-soft); }
  .status { flex-grow: 1; min-width: 0; margin: 0; font-size: 14px; color: var(--ink); overflow-wrap: anywhere; }
  .status.bad { color: var(--bad); }
  .actions { display: flex; gap: 10px; flex: none; margin-left: auto; }
  @media (pointer: coarse) { select { height: 44px; } .chev { top: 15px; } }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    dialog { width: 100vw; max-height: 100dvh; }
    .box { max-height: 100dvh; border-radius: 16px 16px 0 0; }
    .body { padding: 14px 16px 16px; }
    select { height: 48px; border-radius: 10px; background: var(--card); font-size: 16px; }
    .chev { top: 17px; }
    footer { flex-direction: column; align-items: stretch; padding: 12px 16px 16px; }
    .status.quiet { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .actions { margin-left: 0; }
    .actions .primary { flex: 1; }
  }
</style>
