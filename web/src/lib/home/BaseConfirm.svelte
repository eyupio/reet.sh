<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'

  // Before a session starts in a saved repository: what its worktree will be cut from, when the repository's base branch
  // and GitHub's do not hold the same commit. The sentence is made by baseNotice.ts from what the machine reported
  // (docs/terminal-worktrees.md); this only shows it and asks. Nothing has started when it opens, and Cancel starts
  // nothing: the launch bar sends its create only after Start session.
  let { sentence, host, oncancel, onstart }: {
    /** The one sentence from `baseSentence`, naming the branch, the machine and the commit. */
    sentence: string
    /** The machine's name, for the eyebrow; empty when the page has no name for it, and then left off. */
    host: string
    oncancel: () => void
    onstart: () => void
  } = $props()

  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let keep = $state<HTMLButtonElement>()

  onMount(() => {
    dialog?.showModal()
    keep?.focus()
    return () => dialog?.close()
  })
  function cancel() { dialog?.close(); oncancel() }
  function start() { dialog?.close(); onstart() }
</script>

<dialog bind:this={dialog} role="alertdialog" use:shell={{ sheet: 'bottom' }} aria-labelledby={id} aria-describedby={`${id}-body`}
  oncancel={(event) => { event.preventDefault(); cancel() }}>
  <div class="box" data-shell-card>
    {#if host}<p class="eyebrow" data-shell-aside>{host}</p>{/if}
    <h2 {id}>Check where this starts</h2>
    <p class="say" id={`${id}-body`}>{sentence}</p>
    <p class="quiet">Nothing has started yet. Start the session to work from that commit, or cancel and sort the branch out on the machine first.</p>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" bind:this={keep} onclick={cancel}>Cancel</button>
      <button type="button" class="btn primary" onclick={start}>Start session</button>
    </div>
  </div>
</dialog>

<style>
  dialog { max-width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; overflow-x: hidden; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { padding: 24px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius); box-shadow: var(--shadow-dock); overflow-wrap: anywhere; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 14px; font-weight: 650; letter-spacing: .1em; text-transform: uppercase; }
  h2 { margin: 4px 0 0; font: 600 1.4rem/1.2 var(--serif); letter-spacing: -.02em; text-wrap: balance; }
  .say { margin: 12px 0 0; line-height: 1.6; }
  .quiet { margin: 10px 0 0; color: var(--muted); font-size: 0.875rem; line-height: 1.6; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 14px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
