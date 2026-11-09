<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import SelectedLibrary from './SelectedLibrary.svelte'
  import Icon from './Icon.svelte'
  import type { SyncState } from './api'

  let { host, snapshot, opener, onclose, onimport }: { host: string; snapshot: SyncState; opener: HTMLElement; onclose: () => void; onimport?: (source: { environmentId: string; name: string }) => void } = $props()
  const titleId = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let closeButton = $state<HTMLButtonElement>()
  // A native dialog reports `close` a task after it closes. If the button reopened the dialog in between, this instance is
  // already destroyed and its late event must not clear the new one.
  let alive = true
  onMount(() => {
    dialog?.showModal()
    closeButton?.focus()
    return () => {
      alive = false
      if (dialog?.open) dialog.close()
      if (opener.isConnected) opener.focus({ preventScroll: true })
    }
  })
</script>

<dialog bind:this={dialog} use:shell aria-labelledby={titleId} onclose={() => { if (alive) onclose() }}>
  <header data-shell-head>
    <h2 id={titleId}>Skills on {host}</h2>
    <button bind:this={closeButton} class="btn quiet" type="button" aria-label="Close machine skills" data-shell-close onclick={() => dialog?.close()}><Icon name="x" size={18} /></button>
  </header>
  <div class="body" data-shell-body><SelectedLibrary {host} {snapshot} {onimport} /></div>
</dialog>

<style>
  dialog{width:min(640px,calc(100vw - 32px));max-height:calc(100dvh - 32px);padding:0;border:1px solid var(--line);border-radius:14px;background:var(--card);color:var(--ink)}
  dialog::backdrop{background:var(--scrim)}
  header{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:18px 20px;border-bottom:1px solid var(--line)}
  h2{margin:0;font-size:1.1rem;overflow-wrap:anywhere;min-width:0}header button{flex:none;min-height:44px;min-width:44px}.body{padding:20px;min-width:0}
</style>
