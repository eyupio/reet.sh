<script lang="ts">
  // The image picker shared by Gaffer's goal and outcome modals. It validates, previews and
  // removes files, but never uploads them: the two modals upload to different places at different
  // times. Parent-held Files are the whole of the selection, so a closed modal leaves nothing.
  import { ATTACHMENT_ACCEPT, MAX_ATTACHMENTS, attachmentMessage, validateAttachment, type AttachmentReason } from './imageAttachment'
  import { formatImageSize } from './terminal/upload'

  let { label, hint = 'Paste, or choose a PNG, JPEG or WebP · up to 5 MiB each', max = 1, disabled = false, busy = false, error = '', files = $bindable<File[]>([]) }: {
    label: string
    hint?: string
    /** How many images the field may hold. One unless the caller asks for more. */
    max?: number
    disabled?: boolean
    busy?: boolean
    error?: string
    files?: File[]
  } = $props()

  const id = $props.id()
  let input = $state<HTMLInputElement>()
  let previews = $state<string[]>([])
  let localError = $state('')
  const shownError = $derived(localError || error)
  const room = $derived(Math.max(0, Math.min(max, MAX_ATTACHMENTS) - files.length))
  const full = $derived(room === 0)

  // Keep the picker's own multi-select in step with the field's limit.
  $effect(() => { if (input) input.multiple = max > 1 })

  $effect(() => {
    const chosen = files
    const urls = chosen.map(file => URL.createObjectURL(file))
    previews = urls
    return () => { for (const url of urls) URL.revokeObjectURL(url) }
  })

  function pick(event: Event) {
    const target = event.currentTarget as HTMLInputElement
    const chosen = Array.from(target.files ?? [])
    // Clearing the value lets the same file be chosen again after a removal.
    target.value = ''
    // Closing the picker without a file changes nothing.
    if (!chosen.length) return
    const valid: File[] = []
    let reason: AttachmentReason | 'full' | '' = ''
    for (const file of chosen) {
      const check = validateAttachment({ type: file.type, size: file.size })
      if (!check.ok) { reason = check.reason; continue }
      if (valid.length >= room) { reason = reason || 'full'; continue }
      valid.push(file)
    }
    localError = reason === 'full' ? `Attach up to ${Math.min(max, MAX_ATTACHMENTS)} ${max === 1 ? 'image' : 'images'}.` : reason ? attachmentMessage(reason) : ''
    if (!valid.length) return
    // A single-image field replaces its image; a field with room adds to the selection.
    files = max === 1 ? [valid[0]] : [...files, ...valid]
  }

  function remove(index: number) { localError = ''; files = files.filter((_, at) => at !== index) }
  function unreadable(index: number) { localError = attachmentMessage('read'); files = files.filter((_, at) => at !== index) }
</script>

<div class="attachment">
  <label class="sr-only" for={id}>{label}</label>
  <input bind:this={input} id={id} class="sr-only" type="file" accept={ATTACHMENT_ACCEPT} multiple={max > 1} tabindex="-1"
    aria-hidden="true" onchange={pick} />
  {#if files.length}
    <ul class="chosen" aria-label="Attached images">
      {#each files as file, index (index)}
        <li>
          {#if previews[index]}<img src={previews[index]} alt={`Preview of ${file.name}`} onerror={() => unreadable(index)} />{/if}
          <div class="facts">
            <span class="name" title={file.name}>{file.name}</span>
            <span class="size">{formatImageSize(file.size)}</span>
          </div>
          <button type="button" class="link" disabled={disabled || busy} aria-label={`Remove ${file.name}`} onclick={() => remove(index)}>Remove</button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if !full}
    <button type="button" class="attach" disabled={disabled || busy} aria-describedby={`${id}-hint`} onclick={() => input?.click()}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14" /></svg>
      {files.length ? 'Attach another image' : label}
    </button>
    <p class="hint" id={`${id}-hint`}>{hint}{max > 1 ? ` · up to ${Math.min(max, MAX_ATTACHMENTS)} images` : ''}</p>
  {/if}
  {#if busy}<p class="status" role="status">Uploading the {files.length === 1 ? 'image' : 'images'}…</p>{/if}
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
  .chosen { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .chosen li { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 6px; border: 1px solid var(--line); border-radius: 8px; }
  .chosen img { width: 48px; height: 48px; object-fit: cover; border-radius: 6px; flex: none; background: var(--bg-sunk); }
  .facts { display: grid; gap: 2px; min-width: 0; flex: 1 1 auto; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.875rem; }
  .size { color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .link { border: 0; background: none; padding: 4px 0; color: var(--accent-text); font: 600 14px var(--sans); cursor: pointer; min-height: 44px; flex: none; }
  .link:disabled { color: var(--muted); cursor: default; }
  @media (max-width: 559px) { .chosen li { flex-wrap: wrap; } .link { width: 100%; justify-content: flex-end; } }
</style>
