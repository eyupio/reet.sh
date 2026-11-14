<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { shell } from '../dialogShell'
  import { cloneFolder, parseGitHubRepo } from '../githubRepo'
  import { CLONE_REMOVE_CODES, deleteFolderProject, projectClone, refusalCode, TerminalError, type FolderProject, type ProjectClone, type RemovedFolderProject } from '../terminal/api'

  // Remove from reet for a project reet cloned from GitHub (docs/projects.md, "Removing a project from GitHub"). It is the
  // rail's confirmation with one more thing: when the machine says, at this moment, that the clone holds nothing that is
  // not on GitHub, a tick offers to delete the clone too. Otherwise one sentence says why the clone stays. The plain
  // remove never waits for that check. With the tick, the dialog stays until the machine has answered: the clone goes
  // first, and the project only when the machine confirms it. A refusal stays in the dialog and the clone is read again.
  let { project, onplain, onremoved, onrefused, onsignedout, oncancel }: {
    project: FolderProject
    /** Remove from reet without the tick: the project only, as for any project. The dialog has closed. */
    onplain: () => void
    /** The machine deleted the clone and the project is gone. The dialog has closed. */
    onremoved: (answer: RemovedFolderProject) => void
    /** A refusal that is not about the clone (the project changed, is gone, too many requests). The dialog has closed. */
    onrefused: (error: unknown) => void
    onsignedout: () => void
    oncancel: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
  let cancelButton = $state<HTMLButtonElement>(), confirmButton = $state<HTMLButtonElement>()
  const life = new AbortController()
  // svelte-ignore state_referenced_locally
  const machine = project.machine || 'its machine'
  // svelte-ignore state_referenced_locally
  const origin = parseGitHubRepo(project.origin ?? '')
  const folder = origin ? cloneFolder(origin) : 'the clone'

  // What the machine last said about the clone; null while it is read, and when it could not be read.
  let clone = $state<ProjectClone | null>(null)
  let checking = $state(true)
  // Why the clone stays, when it does.
  let kept = $state('')
  // The tick: off whenever the dialog opens and whenever the clone is read again.
  let also = $state(false)
  // The delete is on its way: the dialog cannot be closed until the machine has answered.
  let busy = $state(false)
  // The last delete's refusal, kept beside what the clone holds now.
  let refusal = $state('')
  const offered = $derived(!checking && !!clone?.removable && !!clone.fingerprint)
  // True once this component has closed its dialog on purpose, so a close the browser makes on its own is undone.
  let done = false

  function shut() { done = true; life.abort(); dialog?.close() }

  async function check(): Promise<boolean> {
    checking = true; also = false
    try {
      const answer = await projectClone(project.id, life.signal)
      if (life.signal.aborted) return false
      clone = answer
      kept = answer.removable ? '' : answer.sentence
    } catch (e) {
      if (life.signal.aborted) return false
      clone = null
      if (e instanceof TerminalError && e.status === 401) { shut(); onsignedout(); return false }
      if (e instanceof TerminalError && e.code === 'project_not_found') { shut(); onrefused(e); return false }
      // A project reet did not clone has nothing to offer or to explain.
      kept = e instanceof TerminalError && e.code === 'not_cloned' ? '' : `The clone on ${machine} is kept: reet couldn't check it just now.`
    } finally {
      if (!life.signal.aborted) checking = false
    }
    return true
  }

  function cancel() {
    if (busy) return
    shut()
    oncancel()
  }

  async function confirm() {
    if (busy) return
    const fingerprint = also && offered ? clone?.fingerprint : undefined
    if (!fingerprint) { shut(); onplain(); return }
    busy = true; refusal = ''
    try {
      const answer = await deleteFolderProject(project.id, project.version, life.signal, { fingerprint })
      if (life.signal.aborted) return
      shut()
      onremoved(answer)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401) { shut(); onsignedout(); return }
      const aboutClone = !!refusalCode(e, CLONE_REMOVE_CODES)
      if (e instanceof TerminalError && !aboutClone) { shut(); onrefused(e); return }
      // The clone was refused, or the answer never came: say so here, and look at the clone again.
      refusal = e instanceof TerminalError ? e.message : `reet couldn't confirm whether the clone on ${machine} was removed. Check again.`
      const still = await check()
      if (!still) return
      // The same sentence is not said twice.
      if (kept === refusal) refusal = ''
      busy = false
      await tick()
      confirmButton?.focus()
    }
  }

  // A browser closes a dialog on a second Escape even when the first was refused. While the delete is on its way that
  // close is undone, and at any other time it is a Cancel.
  function closed() {
    if (done) return
    if (busy) { dialog?.showModal(); return }
    cancel()
  }

  onMount(() => {
    dialog?.showModal()
    cancelButton?.focus()
    void check()
    return () => { done = true; life.abort(); dialog?.close() }
  })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={(e) => { e.preventDefault(); cancel() }} onclose={closed}>
  <div class="box" data-shell-card>
    <h2 id={titleId}>Remove {project.name} from reet?</h2>
    <p>It leaves the rail. Its sessions and any worktrees stay on {machine}.</p>
    {#if offered}
      <label class="also">
        <input type="checkbox" bind:checked={also} disabled={busy} aria-describedby={`${titleId}-ignored`} />
        <span>Also delete <span class="path">{folder}</span> from {machine}</span>
      </label>
      <p class="ignored" id={`${titleId}-ignored`}>Files git ignores there, such as .env or build output, are deleted too.</p>
    {/if}
    <!-- In the accessibility tree from the start and empty until there is something to say, so what is put in it is announced. -->
    <div class="live" role="status" aria-live="polite" aria-atomic="true">{#if checking}<p class="checking">Checking the clone on {machine}…</p>{:else if kept}<p class="kept">{kept}</p>{/if}</div>
    <div class="live" role="status" aria-live="polite" aria-atomic="true">{#if refusal}<p class="refusal">{refusal}</p>{/if}</div>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" bind:this={cancelButton} onclick={cancel} disabled={busy}>Cancel</button>
      <button type="button" class="btn primary danger" bind:this={confirmButton} onclick={() => void confirm()} disabled={busy} aria-busy={busy}>{busy ? 'Removing…' : 'Remove from reet'}</button>
    </div>
  </div>
</dialog>

<style>
  /* The frame is ConfirmDialog's, so this reads as the same confirmation. */
  dialog {
    max-width: min(440px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0;
    background: transparent; color: var(--ink);
  }
  dialog::backdrop { background: var(--scrim); }
  .box {
    padding: 24px; background: var(--card); border: 1px solid var(--line); border-radius: 16px;
    box-shadow: var(--shadow); overflow-wrap: anywhere;
  }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; text-wrap: balance; }
  p { margin: 8px 0 0; color: var(--muted); overflow-wrap: anywhere; }
  .also { display: flex; align-items: flex-start; gap: 10px; margin: 16px 0 0; min-height: 44px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; color: var(--ink); cursor: pointer; }
  .also input { flex: none; width: 18px; height: 18px; margin: 2px 0 0; accent-color: var(--bad); }
  .also:has(input:disabled) { cursor: default; opacity: .6; }
  .path { font-family: var(--mono); font-size: 0.9375em; }
  .ignored { margin: 6px 0 0; font-size: 14px; line-height: 1.45; }
  .live p { margin: 16px 0 0; font-size: 14px; line-height: 1.45; }
  .live:empty { margin: 0; padding: 0; }
  .kept { color: var(--ink); }
  .refusal { color: var(--bad); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; flex-wrap: wrap; }
  .danger { background: var(--bad); border-color: var(--bad); color: var(--bg); }
  .btn:disabled { opacity: .5; cursor: default; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 12px; }
    .actions { flex-direction: column; }
    .actions button { flex: none; width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: scale(0.97); }
      to { opacity: 1; transform: scale(1); }
    }
  }
</style>
