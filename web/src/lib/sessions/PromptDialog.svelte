<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import CopyButton from '../CopyButton.svelte'
  import type { TerminalSession } from '../terminal/api'
  import { startLabel } from './runClock'

  // The parent decides what the text is and whether it is the whole prompt: `ready` means it is, `loading` and `failed`
  // mean it is only the excerpt so far, `none` means the server kept no text at all. Copy is offered only for the whole
  // text, so a person never pastes a cut-off prompt thinking it is complete.
  let { session, text, now, phase, notKept, onretry, onclose }: {
    session: Pick<TerminalSession, 'tool' | 'model' | 'effort' | 'yolo' | 'createdAt'>
    text: string; now: number; phase: 'ready' | 'loading' | 'failed' | 'none'; notKept: string
    onretry: () => void; onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const titleId = $props.id()
  // The dialog sits inside the details sheet on a phone, whose key handler would close the sheet on Escape and trap Tab over its own controls. The keys are stopped here, never prevented, so the dialog's own Escape and focus handling still run.

  onMount(() => { dialog?.showModal(); return () => dialog?.close() })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={(event) => { event.preventDefault(); onclose() }} onkeydown={(event) => event.stopPropagation()}>
  <header data-shell-head>
    <h2 id={titleId}>You asked</h2>
    <button type="button" class="btn quiet" onclick={onclose} aria-label="Close" data-shell-close>Close ×</button>
  </header>
  <div class="chips" role="group" aria-label="How the session was started">
    {#if session.tool}<span class="chip">{session.tool}</span>{/if}
    {#if session.model}<span class="chip">{session.model}</span>{/if}
    {#if session.effort}<span class="chip">{session.effort}</span>{/if}
    <span class="chip">Started {startLabel(session.createdAt, now)}</span>
    {#if session.yolo}<span class="chip warn">YOLO</span>{/if}
  </div>
  <div class="body" data-shell-body>
    {#if phase === 'none'}<p class="muted">{notKept}</p>{:else}<div class="text">{text}</div>{/if}
  </div>
  <footer data-shell-foot>
    <p class="status" role="status">{#if phase === 'loading'}Loading the full prompt…{/if}</p>
    {#if phase === 'failed'}<p class="failure" role="alert">Couldn't load the full prompt. Try again.</p><button type="button" class="btn" onclick={onretry}>Try again</button>{/if}
    {#if phase === 'ready'}<CopyButton text={text} label="Copy prompt" copiedLabel="Copied" primary />{/if}
  </footer>
</dialog>

<style>
  dialog { padding: 0; border: 1px solid var(--line); border-radius: 14px; background: var(--card); color: var(--ink); width: min(620px, calc(100vw - 24px)); max-height: calc(100dvh - 24px); overflow: hidden; }
  dialog[open] { display: flex; flex-direction: column; }
  dialog::backdrop { background: var(--scrim); backdrop-filter: blur(2px); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 20px; border-bottom: 1px solid var(--line); }
  h2 { margin: 0; font: 500 1.35rem var(--serif); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; padding: 12px 20px 0; }
  .chip { border-radius: 999px; padding: 3px 9px; background: var(--tint); color: var(--muted); font: 500 11px var(--mono); overflow-wrap: anywhere; }
  .chip.warn { color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, transparent); }
  .body { flex: 1 1 auto; min-height: 0; overflow: auto; padding: 14px 20px; }
  .text { white-space: pre-wrap; overflow-wrap: anywhere; font: 13px/1.65 var(--sans); }
  .muted { margin: 0; color: var(--muted); font-size: 13px; }
  footer { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 10px 12px; padding: 12px 20px; border-top: 1px solid var(--line); }
  footer .status, footer .failure { margin: 0; flex: 1 1 12em; min-width: 0; font-size: 12px; color: var(--muted); }
  footer .status:empty { display: none; }
  footer .failure { color: var(--bad); }
  @media (max-width: 560px) { header, .chips, .body, footer { padding-inline: 16px; } footer > :global(.btn) { flex: 1 1 100%; } }
</style>
