<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { copyText } from '../clipboard'

  let { text, onclose }: { text: string; onclose: () => void } = $props()
  let dialog = $state<HTMLDialogElement>(), pre = $state<HTMLPreElement>(), status = $state('')
  const titleId = $props.id()

  function close() {
    dialog?.close()
    onclose()
  }

  async function copySelection() {
    const selection = window.getSelection()
    const anchor = selection?.anchorNode, focus = selection?.focusNode
    const inside = anchor && focus && pre?.contains(anchor) && pre?.contains(focus)
    const selected = inside ? selection?.toString() ?? '' : ''
    if (!selected) {
      status = 'Select text first.'
      return
    }
    status = (await copyText(selected)) ? 'Selection copied.' : 'Clipboard access was denied. Use your browser’s copy command.'
  }

  async function copyAll() {
    status = !text
      ? 'There is nothing to copy yet.'
      : (await copyText(text))
        ? 'Copied all terminal text.'
        : 'Clipboard access was denied. Use your browser’s copy command.'
  }

  onMount(() => {
    dialog?.showModal()
    pre?.focus()
    return () => dialog?.close()
  })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={(event) => { event.preventDefault(); close() }}>
  <div class="box" data-shell-card>
    <header data-shell-head><h2 id={titleId}>Select text</h2></header>
    <div class="body" data-shell-body>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (a read-only pre must be keyboard reachable to select text) -->
      <pre bind:this={pre} tabindex="0" aria-label="Terminal text">{text}</pre>
    </div>
    <footer data-shell-foot>
      {#if status}<p role="status">{status}</p>{/if}
      <button type="button" class="btn" onmousedown={(event) => event.preventDefault()} onclick={() => void copySelection()}>Copy selection</button>
      <button type="button" class="btn" onclick={() => void copyAll()}>Copy all</button>
      <button type="button" class="btn quiet" data-shell-close aria-label="Close select text" onclick={close}>Close</button>
    </footer>
  </div>
</dialog>

<style>
  dialog {width:min(720px,calc(100vw - 24px));max-height:calc(100dvh - 24px);overflow:hidden;padding:0;border:1px solid var(--line);border-radius:14px;background:var(--card);color:var(--ink);margin:auto}
  dialog::backdrop {background:var(--scrim)}
  .box {display:flex;flex-direction:column;min-height:0;max-height:inherit}
  header {display:flex;align-items:center;justify-content:space-between;gap:12px;flex-shrink:0;padding:12px 18px;border-bottom:1px solid var(--line);background:var(--card)}
  h2 {margin:0;font:500 1.2rem/1.2 var(--serif)}
  .body {flex:1;min-height:0;overflow:auto;overscroll-behavior:contain;padding:12px 18px;background:var(--bg)}
  pre {margin:0;width:max-content;min-width:100%;font:var(--terminal-font-size,14px)/1.45 "DejaVu Sans Mono","SFMono-Regular",Consolas,monospace;white-space:pre;tab-size:8;user-select:text;cursor:text;color:var(--ink)}
  pre:focus-visible {outline:2px solid var(--accent);outline-offset:2px}
  footer {display:flex;align-items:center;flex-wrap:wrap;justify-content:flex-end;gap:8px;flex-shrink:0;padding:10px 18px;border-top:1px solid var(--line);background:var(--card)}
  footer p {flex:1 1 100%;margin:0;font-size:.72rem;color:var(--muted)}
  .btn {font:inherit;font-size:.78rem;padding:8px 12px;min-height:40px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);cursor:pointer}
  .btn:hover {border-color:var(--accent);background:var(--tint)}
  .quiet {color:var(--muted)}
  [data-shell-close] {min-width:44px;min-height:44px}
  @media(max-width:559.98px) {footer {padding-bottom:max(10px,env(safe-area-inset-bottom))}}
</style>
