<script lang="ts">
  // The one image picker shared by Gaffer's goal and outcome modals. It validates, previews and
  // removes a file, but never uploads it: the two modals upload to different places at different
  // times. A parent-held File is the whole of the selection, so a closed modal leaves nothing.
  import { ATTACHMENT_ACCEPT, attachmentMessage, validateAttachment } from './imageAttachment'
  import { formatImageSize } from './terminal/upload'

  let { label, hint = 'Paste, or choose a PNG, JPEG or WebP · up to 5 MiB', disabled = false, busy = false, error = '', file = $bindable<File | null>(null) }: {
    label: string
    hint?: string
    disabled?: boolean
    busy?: boolean
    error?: string
    file?: File | null
  } = $props()

  const id = $props.id()
  let input = $state<HTMLInputElement>()
  let preview = $state('')
  let localError = $state('')
  const shownError = $derived(localError || error)

  $effect(() => {
    const chosen = file
    if (!chosen) { preview = ''; return }
    const url = URL.createObjectURL(chosen)
    preview = url
    return () => URL.revokeObjectURL(url)
  })

  function pick(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    const chosen = target.files?.[0] ?? null
    // Clearing the value lets the same file be chosen again after a removal.
    target.value = ''
    // Closing the picker without a file changes nothing.
    if (!chosen) return
    const check = validateAttachment({ type: chosen.type, size: chosen.size })
    if (!check.ok) { localError = attachmentMessage(check.reason); return }
    localError = ''
    file = chosen
  }

  function remove() { localError = ''; file = null }
  function unreadable() { localError = attachmentMessage('read'); file = null }
</script>

<div class="attachment">
  <label class="sr-only" for={id}>{label}</label>
  <input bind:this={input} id={id} class="sr-only" type="file" accept={ATTACHMENT_ACCEPT} tabindex="-1"
    aria-hidden="true" onchange={pick} />
  {#if file}
    <div class="chosen">
      {#if preview}<img src={preview} alt={`Preview of ${file.name}`} onerror={unreadable} />{/if}
      <div class="facts">
        <span class="name" title={file.name}>{file.name}</span>
        <span class="size">{formatImageSize(file.size)}</span>
      </div>
      <div class="chosen-actions">
        <button type="button" class="link" disabled={disabled || busy} aria-label={`Replace ${file.name}`} onclick={() => input?.click()}>Replace</button>
        <button type="button" class="link" disabled={disabled || busy} aria-label={`Remove ${file.name}`} onclick={remove}>Remove</button>
      </div>
    </div>
  {:else}
    <button type="button" class="attach" disabled={disabled || busy} aria-describedby={`${id}-hint`} onclick={() => input?.click()}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14" /></svg>
      {label}
    </button>
    <p class="hint" id={`${id}-hint`}>{hint}</p>
  {/if}
  {#if busy}<p class="status" role="status">Uploading the image…</p>{/if}
  {#if shownError}<p class="error" role="alert">{shownError}</p>{/if}
</div>

<style>
  .attachment { display: grid; gap: 6px; min-width: 0; }
  .sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
  .attach { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0; border: 0; background: none; color: var(--accent-text); font: 600 14px var(--sans); cursor: pointer; justify-self: start; }
  .attach svg { width: 14px; height: 14px; flex: none; }
  .attach:disabled { color: var(--muted); cursor: default; }
  .attach:focus-visible, .link:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  .hint, .status { margin: 0; color: var(--muted); font-size: 0.875rem; }
  .error { margin: 0; color: var(--bad); font-size: 0.875rem; }
  .chosen { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 6px; border: 1px solid var(--line); border-radius: 8px; }
  .chosen img { width: 48px; height: 48px; object-fit: cover; border-radius: 6px; flex: none; background: var(--bg-sunk); }
  .facts { display: grid; gap: 2px; min-width: 0; flex: 1 1 auto; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.875rem; }
  .size { color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .chosen-actions { display: flex; gap: 8px; flex: none; }
  .link { border: 0; background: none; padding: 4px 0; color: var(--accent-text); font: 600 14px var(--sans); cursor: pointer; min-height: 44px; }
  .link:disabled { color: var(--muted); cursor: default; }
  @media (max-width: 559px) { .chosen { flex-wrap: wrap; } .chosen-actions { width: 100%; justify-content: flex-end; } }
</style>
