<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { shell } from '../dialogShell'
  import { originalChanges, UnauthorizedError, type OriginalChanges } from '../api'

  // Update from original on an imported skill. It fetches the original again and lists which files changed against
  // reet's copy; nothing changes until the person confirms. When they edited the skill since it was imported, it says
  // those edits go and stay in the skill's history. Confirm sends the digest and revision it showed, so an original or
  // a skill that moved on meanwhile is refused rather than replaced unseen.
  let { name, id, onupdate, onclose, onerror }: {
    name: string
    id: string
    /** Confirmed: replace the skill's content with what was shown. Resolves once saved; rejects with why it wasn't. */
    onupdate: (digest: string, revision: string) => Promise<void>
    onclose: () => void
    /** A refusal the dialog cannot answer itself, such as a sign-in that has ended. */
    onerror: (e: unknown) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let foot = $state<HTMLElement>()
  const titleId = $props.id()
  let found = $state<OriginalChanges | null>(null)
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')

  const KIND: Record<string, string> = { added: 'New', removed: 'Removed', modified: 'Changed' }
  const changed = $derived(!!found && found.changes.length > 0)

  async function look() {
    loading = true
    error = ''
    try {
      found = await originalChanges(id)
    } catch (e) {
      if (e instanceof UnauthorizedError) { onerror(e); close(); return }
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
    // The footer's buttons change once the changes are in: focus stays on its first one.
    await tick()
    foot?.querySelector<HTMLButtonElement>('[data-first]')?.focus()
  }

  onMount(() => {
    dialog?.showModal()
    foot?.querySelector<HTMLButtonElement>('[data-first]')?.focus()
    void look()
    return () => dialog?.close()
  })

  function close() {
    if (busy) return
    dialog?.close()
    onclose()
  }

  async function confirm() {
    if (!found || busy) return
    busy = true
    error = ''
    try {
      await onupdate(found.digest, found.revision)
      busy = false
      close()
    } catch (e) {
      busy = false
      if (e instanceof UnauthorizedError) { onerror(e); close(); return }
      error = e instanceof Error ? e.message : String(e)
    }
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} aria-busy={loading || busy} oncancel={e => { e.preventDefault(); close() }}>
  <div class="box" data-shell-card>
    <header class="head" data-shell-head>
      <h2 id={titleId}>Update {name} from its original?</h2>
      <p class="lede" data-shell-aside>reet fetches the original again and shows what changed. Nothing changes until you confirm; the new version then goes on every machine.</p>
    </header>
    <div class="body" data-shell-body>
      {#if loading}
        <p class="quiet" role="status">Fetching the original…</p>
      {:else if found && changed}
        <ul class="changes" aria-label="What changed">
          {#each found.changes as change (change.path)}
            <li><code>{change.path}</code><span class="kind" data-kind={change.kind}>{KIND[change.kind] ?? change.kind}</span></li>
          {/each}
        </ul>
        {#if found.revision === ''}<p class="quiet">reet has no copy of {name} yet, so every file is new.</p>{/if}
        {#if found.edited}<p class="warn">Your edits will be replaced; they stay in this skill's history.</p>{/if}
      {:else if found}
        <p class="quiet">Nothing has changed at the original since reet took its copy.</p>
      {/if}
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>
    <footer class="foot" bind:this={foot} data-shell-foot>
      {#if found && changed}
        <button type="button" class="btn quiet" data-first disabled={busy} onclick={close}>Not now</button>
        <button type="button" class="btn primary" disabled={busy} aria-busy={busy} onclick={confirm}>{busy ? 'Updating…' : 'Update from original'}</button>
      {:else}
        <button type="button" class="btn" data-first onclick={close}>Close</button>
        {#if error && !loading}<button type="button" class="btn" onclick={look}>Try again</button>{/if}
      {/if}
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); max-width: none; max-height: calc(100dvh - 32px); overflow: auto; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); overflow-wrap: anywhere; }
  .head { padding: 24px 24px 8px; display: flex; flex-direction: column; gap: 8px; }
  h2 { margin: 0; font: 600 1.5rem/1.2 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 0; color: var(--ink-soft); font-size: 0.9375rem; line-height: 1.5; }
  .body { padding: 8px 24px; display: flex; flex-direction: column; gap: 10px; }
  .changes { list-style: none; margin: 0; padding: 0; }
  .changes li { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; padding: 10px 0; border-top: 1px solid var(--line-soft); font-size: 0.9375rem; }
  .changes li:first-child { border-top: 0; }
  code { font-family: var(--mono); font-size: 0.875rem; overflow-wrap: anywhere; }
  .kind { flex: none; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .kind[data-kind='added'] { color: var(--good); }
  .kind[data-kind='removed'] { color: var(--bad); }
  .quiet { margin: 0; color: var(--ink-soft); font-size: 0.9375rem; }
  .warn { margin: 0; padding: 10px 12px; border-radius: 8px; background: color-mix(in srgb, var(--warn) 12%, var(--card)); color: var(--ink); font-size: 0.9375rem; line-height: 1.45; }
  .error { margin: 0; color: var(--bad); font-size: 0.875rem; }
  .foot { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; padding: 12px 24px 24px; border-top: 1px solid var(--line-soft); margin-top: 8px; }
  .foot .btn { min-height: 44px; border-radius: 10px; }
  @media (max-width: 560px) {
    .head { padding: 20px 20px 8px; }
    .body { padding: 8px 20px; }
    .foot { padding: 12px 20px 20px; }
    .foot .btn { flex: 1; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
