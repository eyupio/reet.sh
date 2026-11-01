<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { shell } from '../dialogShell'
  import { UnauthorizedError } from '../api'
  import { branchPrunePlan, pruneBranches, refusalSentence, TerminalError, type BranchPrunePlan, type BranchPruneResult, type FolderProject } from '../terminal/api'

  // Pruning one repository's merged local branches on its machine. Nothing is deleted until the person presses the
  // button: the plan is shown first, with what is kept and why, and a prune deletes only the branches still safe and
  // still at the tip shown.
  let { project, opener = null, onsignedout, onclose }: {
    project: FolderProject
    opener?: HTMLElement | null
    onsignedout?: (e: unknown) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  const life = new AbortController()
  let dialog = $state<HTMLDialogElement>(), closeButton = $state<HTMLButtonElement>(), primary = $state<HTMLButtonElement>()
  let plan = $state<BranchPrunePlan | undefined>()
  let failed = $state(''), problem = $state('')
  let reading = $state(true), deleting = $state(false)
  let result = $state<BranchPruneResult | null>(null)
  const host = $derived(project.machine || 'its machine')
  const safe = $derived((plan?.branches ?? []).filter(b => !b.skip))
  const kept = $derived((plan?.branches ?? []).filter(b => b.skip))
  const base = $derived(plan?.base || project.baseBranch || 'the base branch')

  function why(skip: string): string {
    switch (skip) {
      case 'not_merged': return `its work isn't in ${base}`
      case 'unpushed': return "it has commits its remote doesn't"
      case 'checked_out': return 'a worktree has it checked out'
      case 'current': return 'it is the branch checked out in the folder'
      case 'default': return "it is the repository's default branch"
      case 'base': return 'it is the branch merged work is measured against'
      case 'moved': return 'it changed while you were looking'
      case 'git_failed': return "git couldn't read it"
      default: return 'reet kept it'
    }
  }

  async function read() {
    reading = true; failed = ''; problem = ''
    try {
      plan = await branchPrunePlan(project.host, project.dir, project.baseBranch || undefined, life.signal)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401 && onsignedout) { life.abort(); onsignedout(new UnauthorizedError()); return }
      failed = refusalSentence(e)
    } finally {
      if (!life.signal.aborted) reading = false
    }
  }

  async function prune() {
    if (!plan || safe.length === 0 || deleting) return
    deleting = true; problem = ''
    try {
      result = await pruneBranches(project.host, project.dir, plan.base, safe.map(b => ({ name: b.name, oid: b.oid })), life.signal)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401 && onsignedout) { life.abort(); onsignedout(new UnauthorizedError()); return }
      problem = refusalSentence(e)
    } finally {
      if (!life.signal.aborted) deleting = false
    }
  }

  function close() {
    life.abort()
    dialog?.close()
    if (opener?.isConnected) opener.focus()
    onclose()
  }

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
      <h2 id={`${id}-title`}>Prune merged branches in {project.name}</h2>
      <p class="lede" data-shell-aside>Local branches in <span class="mono">{project.dir}</span> whose work is already in <strong>{base}</strong>. Remote branches are not touched.</p>
    </div>
    <button type="button" class="btn" bind:this={closeButton} onclick={close} aria-label="Close" data-shell-close>Close ×</button>
  </header>
  <div data-shell-body>
    {#if result}
      <p class="notice" role="status">
        {#if result.deleted.length}
          Deleted {result.deleted.length} {result.deleted.length === 1 ? 'branch' : 'branches'}. Their commits stay in {base}, and remote branches are unchanged.
        {:else}
          Nothing was deleted. The branches changed since the plan, so reet left them alone.
        {/if}
      </p>
      {#if result.deleted.length}
        <ul class="branches" aria-label="Deleted branches">{#each result.deleted as name (name)}<li class="gone"><span class="mono">{name}</span></li>{/each}</ul>
      {/if}
      {#if result.skipped.length}
        <h3>Kept</h3>
        <ul class="branches" aria-label="Kept branches">{#each result.skipped as item (item.name)}<li><span class="mono">{item.name}</span><small>{why(item.skip)}</small></li>{/each}</ul>
      {/if}
    {:else if reading}
      <p role="status">Reading the repository's branches…</p>
    {:else if failed}
      <p class="notice bad" role="alert">{failed}</p>
      <button type="button" class="btn" onclick={() => void read()}>Try again</button>
    {:else if plan}
      {#if plan.base === ''}
        <p class="notice bad">reet couldn't find a base branch to compare against, so nothing can be pruned here.</p>
      {:else if safe.length === 0}
        <p class="notice">Nothing is safe to prune: every local branch is either unmerged, checked out, or the base itself.</p>
      {:else}
        <div class="warning" role="note">
          <strong>This deletes {safe.length} local {safe.length === 1 ? 'branch' : 'branches'}.</strong>
          reet never deletes unmerged work, the current or default branch, or one checked out in a worktree. The commits stay in {base}; only the local branch names go, and remote branches are left alone.
        </div>
        <h3>Will be deleted</h3>
        <ul class="branches" aria-label="Branches to delete">{#each safe as branch (branch.name)}<li><span class="mono">{branch.name}</span></li>{/each}</ul>
      {/if}
      {#if kept.length}
        <h3>Kept</h3>
        <ul class="branches" aria-label="Branches kept">{#each kept as branch (branch.name)}<li><span class="mono">{branch.name}</span><small>{why(branch.skip)}</small></li>{/each}</ul>
      {/if}
      {#if plan.more}<p class="hint">Only the first 200 branches are listed. Prune these, then look again for the rest.</p>{/if}
      {#if problem}<p class="notice bad" role="alert">{problem}</p>{/if}
    {/if}
  </div>
  <footer data-shell-foot>
    <button type="button" class="btn" onclick={close}>{result ? 'Done' : 'Cancel'}</button>
    {#if !result && plan && plan.base !== ''}
      <button type="button" class="btn primary" bind:this={primary} disabled={deleting || safe.length === 0} onclick={() => void prune()}>
        {deleting ? 'Deleting…' : safe.length === 0 ? 'Nothing to delete' : `Delete ${safe.length} ${safe.length === 1 ? 'branch' : 'branches'}`}
      </button>
    {/if}
  </footer>
</dialog>

<style>
  .mono { font-family: var(--mono); font-size: .95em; }
  h3 { margin: 16px 0 6px; font-size: 12px; font-weight: 650; letter-spacing: .04em; text-transform: uppercase; color: var(--muted); }
  .warning { margin: 0 0 6px; padding: 11px 13px; border: 1px solid color-mix(in srgb, var(--warn) 40%, var(--line)); border-left: 3px solid var(--warn); border-radius: 8px; background: color-mix(in srgb, var(--warn) 6%, var(--card)); color: var(--ink-soft); font-size: 13px; line-height: 1.5; }
  .warning strong { display: block; margin-bottom: 3px; color: var(--ink); }
  .branches { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 3px; }
  .branches li { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; min-height: 28px; padding: 4px 9px; border: 1px solid var(--line-soft); border-radius: 7px; background: var(--bg-sunk); font-size: 13px; }
  .branches li.gone { color: var(--muted); text-decoration: line-through; }
  .branches small { color: var(--muted); font-size: 12px; }
  .hint { margin: 10px 0 0; font-size: 12px; color: var(--muted); }
  .notice { margin: 0 0 10px; }
</style>
