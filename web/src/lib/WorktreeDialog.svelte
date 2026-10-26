<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { UnauthorizedError } from './api'
 import { removeWorktree, worktreeStatus, TerminalError } from './terminal/api'
 import { branchKept, removeConfirmText, worktreeBlockers, worktreeRefusalText, type RunMember, type WorktreeStatus } from './projects'

 // Remove one run's worktree. The machine's status is read and shown first; Remove is offered only when nothing blocks
 // it, and the remove request repeats the counts that were shown, so changes made after the person looked are never lost.
 // The branch is kept. reet never removes a worktree by itself.
 let { member, host, opener = null, onremoved, onsignedout, onclose }: {
  member: RunMember
  /** The machine's name, for the words. */
  host: string
  opener?: HTMLElement | null
  /** The worktree is gone (or was already): the run is read again. */
  onremoved: () => void
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const id = $props.id()
 // svelte-ignore state_referenced_locally
 const target = { agentId: member.host, project: member.worktreeProject, repo: member.worktreeRepo, run: member.worktreeRun }
 let dialog = $state<HTMLDialogElement>(), mainButton = $state<HTMLButtonElement>()
 let busy = $state<'' | 'checking' | 'removing'>(''), message = $state(''), failed = $state(false), detail = $state('')
 let status = $state<WorktreeStatus | null>(null)
 // The remove finished, or the worktree was not there: there is nothing left to do here but close.
 let gone = $state(false)
 const life = new AbortController()
 const blockers = $derived(status ? worktreeBlockers(status) : [])
 const canRemove = $derived(!!status && blockers.length === 0 && !gone)
 const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

 function shutdown(returnFocus: boolean) {
  life.abort()
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 function close() { if (busy) return; const removed = gone; shutdown(true); if (removed) onremoved(); onclose() }
 async function focusAfter() { await tick(); if (!life.signal.aborted) mainButton?.focus() }
 function signedOut(e: unknown): boolean {
  if (!(e instanceof TerminalError) || e.status !== 401 || !onsignedout) return false
  const hand = onsignedout; shutdown(false); onclose(); hand(new UnauthorizedError())
  return true
 }

 async function check() {
  if (busy || life.signal.aborted) return
  busy = 'checking'; failed = false; detail = ''; status = null
  message = `Checking the worktree on ${host}…`
  try {
   const next = await worktreeStatus(target, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
   if (life.signal.aborted) return
   status = next
   message = worktreeBlockers(next).length ? "This worktree can't be removed yet." : 'Checked. Nothing here stops the remove.'
  } catch (e) {
   if (life.signal.aborted || signedOut(e)) return
   failed = true
   if (e instanceof TerminalError) {
    message = worktreeRefusalText(e.status, e.code, e.message)
    detail = e.detail ?? ''
    if (e.status === 404 && e.code === 'not_found') gone = true
   } else message = "We couldn't reach that machine just now. Check again in a moment."
  } finally {
   if (!life.signal.aborted) { busy = ''; void focusAfter() }
  }
 }

 async function remove() {
  if (busy || life.signal.aborted || !status || worktreeBlockers(status).length) return
  // Exactly the counts on screen: if the machine sees different ones, it refuses and the person looks again.
  const shown = { changed: status.changed, unpushed: status.unpushed }
  busy = 'removing'; failed = false; detail = ''
  message = `Removing the worktree on ${host}…`
  try {
   const done = await removeWorktree(target, shown, AbortSignal.any([life.signal, AbortSignal.timeout(15000)]))
   if (life.signal.aborted) return
   gone = true
   message = `Sorted. The worktree${done.path ? ` at ${done.path}` : ''} is removed. ${branchKept(done.branch || status.branch)}`
  } catch (e) {
   if (life.signal.aborted || signedOut(e)) return
   failed = true
   if (e instanceof TerminalError && e.status !== 503 && e.status !== 504 && e.status !== 502) {
    message = worktreeRefusalText(e.status, e.code, e.message)
    detail = e.detail ?? ''
    if (e.status === 404 && e.code === 'not_found') gone = true
    // Whatever stopped it, the counts shown are no longer the ones to act on.
    else status = null
   } else {
    status = null
    message = e instanceof TerminalError && e.code ? `${worktreeRefusalText(e.status, e.code, e.message)} The worktree may have been removed. Check again.` : "We didn't hear back, so the worktree may have been removed. Check again."
   }
  } finally {
   if (!life.signal.aborted) { busy = ''; void focusAfter() }
  }
 }

 onMount(() => {
  dialog?.showModal()
  void check()
  return () => life.abort()
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={`${id}-title`}>
 <header>
  <div>
   <p class="eyebrow">{host}</p>
   <h2 id={`${id}-title`}>Remove worktree</h2>
   <p class="lede">reet looks first, and removes nothing you haven't saved.</p>
  </div>
  <button type="button" class="btn" onclick={close} disabled={!!busy} aria-label="Close">Close ×</button>
 </header>
 <div class="body">
  <dl>
   <div><dt>Folder</dt><dd class="mono">{status?.path || member.worktreePath || 'Not known yet'}</dd></div>
   <div><dt>Branch</dt><dd class="mono">{status?.branch || member.worktreeBranch || 'Not known yet'}</dd></div>
   <div><dt>Saved folder</dt><dd class="mono">{member.dir}</dd></div>
  </dl>
  {#if status}
   <ul class="counts" aria-label="What the machine found">
    <li class:bad={status.changed > 0}>{status.changed === 0 ? 'No uncommitted changes' : `${plural(status.changed, 'file', 'files')} with uncommitted changes`}</li>
    <li>{status.unpushed === 0 ? 'No unpushed commits' : `${plural(status.unpushed, 'commit', 'commits')} not pushed anywhere else (kept on the branch)`}</li>
    {#if status.unreferenced > 0}<li class="bad">{plural(status.unreferenced, 'commit', 'commits')} on no branch</li>{/if}
    {#if status.hidden > 0}<li class="bad">{plural(status.hidden, 'file', 'files')} git is told not to check</li>{/if}
    {#if status.nested}<li class="bad">Another git repository inside</li>{/if}
    {#if status.incomplete}<li class="bad">Not checked completely</li>{/if}
   </ul>
   {#if status.changed > 0}
    <div class="paths">
     <p id={`${id}-paths`}>Changed {status.changed === 1 ? 'file' : 'files'}:</p>
     <ul aria-labelledby={`${id}-paths`}>{#each status.paths as path, i (i)}<li class="mono">{path}</li>{/each}</ul>
     {#if status.changed > status.paths.length}<p class="hint">and {plural(status.changed - status.paths.length, 'more', 'more')} not listed.</p>{/if}
    </div>
   {/if}
   {#if blockers.length}
    <div class="blocked">{#each blockers as line (line)}<p>{line}</p>{/each}</div>
   {:else if !gone}
    <p class="confirm" id={`${id}-confirm`}>{removeConfirmText(status)}</p>
   {/if}
  {/if}
  {#if detail}<p class="detail"><span>Git said:</span> <span class="mono">{detail}</span></p>{/if}
 </div>
 <footer>
  <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
  {#if gone}
   <button type="button" class="btn primary" bind:this={mainButton} onclick={close}>Done</button>
  {:else if canRemove}
   <button type="button" class="btn" onclick={check} disabled={!!busy}>Check again</button>
   <button type="button" class="btn primary danger" bind:this={mainButton} onclick={remove} disabled={!!busy} aria-busy={busy === 'removing'} aria-describedby={`${id}-confirm`}>{busy === 'removing' ? 'Removing…' : `Remove with ${status!.unpushed === 0 ? 'no unpushed commits' : plural(status!.unpushed, 'unpushed commit', 'unpushed commits')}`}</button>
  {:else}
   <button type="button" class="btn primary" bind:this={mainButton} onclick={check} disabled={!!busy} aria-busy={busy === 'checking'}>{busy === 'checking' ? 'Checking…' : 'Check again'}</button>
  {/if}
 </footer>
</dialog>

<style>
 dialog { width: min(600px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr) auto; }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); }
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; overflow-wrap: anywhere; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 .body { display: grid; gap: 14px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; font-size: .88rem; line-height: 1.5; }
 dl { display: grid; gap: 6px; margin: 0; }
 dl div { display: grid; grid-template-columns: 7.5em minmax(0, 1fr); gap: 8px; }
 dt { color: var(--muted); }
 dd { margin: 0; min-width: 0; }
 .mono { font-family: var(--mono); font-size: .8rem; overflow-wrap: anywhere; }
 .counts { margin: 0; padding-left: 1.2em; display: grid; gap: 2px; }
 .counts .bad, .blocked p { color: var(--bad); }
 .paths { display: grid; gap: 4px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
 .paths p { margin: 0; }
 .paths ul { margin: 0; padding-left: 1.2em; }
 .blocked { display: grid; gap: 6px; padding: 10px 12px; border: 1px solid var(--bad); border-radius: 8px; }
 .blocked p { margin: 0; }
 .confirm { margin: 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 8px; background: var(--bg); }
 .detail { margin: 0; color: var(--muted); }
 .hint { margin: 0; color: var(--muted); font-size: .8rem; }
 .status { flex: 1 0 100%; margin: 0; font-size: .85rem; line-height: 1.5; }
 .status.bad { color: var(--bad); }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 10px 12px; padding: 14px 22px 22px; border-top: 1px solid var(--line); background: var(--card); overflow-wrap: anywhere; }
 @media (max-width: 500px) { header { padding: 16px; } .body { padding: 16px; } footer { padding: 12px 16px 16px; } h2 { font-size: 1.35rem; } footer .btn { width: 100%; } dl div { grid-template-columns: minmax(0, 1fr); gap: 0; } }
</style>
