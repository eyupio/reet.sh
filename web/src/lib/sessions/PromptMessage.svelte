<script lang="ts">
  import ImageLightbox from '../terminal/ImageLightbox.svelte'
  import { loadThumb } from '../terminal/thumbs'
  import { baseName, splitImageRefs } from '../terminal/upload'

  // The stored prompt appends an `Image:` line naming each upload. The person sees the picture, not its path: the
  // browser keeps a thumbnail of every image it sends. A thumbnail the browser no longer holds is named, never linked.
  let { text }: { text: string } = $props()

  const parts = $derived(splitImageRefs(text))
  const images = $derived(parts.refs.map(ref => {
    const preview = loadThumb(ref)
    return { ref, preview, name: preview?.name || baseName(ref) || 'Image' }
  }))
  const gallery = $derived(images.flatMap(image => image.preview ? [image.preview] : []))

  let lightbox = $state<{ items: { url: string; name: string }[]; index: number } | null>(null)
  let root = $state<HTMLElement>()
  // Escape belongs to the picture while it is open: closing it must not also close the prompt dialog around it.
  $effect(() => {
    if (!lightbox || !root) return
    const dialog = root.closest('dialog')
    if (!dialog) return
    const guard = (event: Event) => event.preventDefault()
    dialog.addEventListener('cancel', guard)
    return () => dialog.removeEventListener('cancel', guard)
  })
  function openPreview(preview: { url: string; name: string }) {
    const index = gallery.indexOf(preview)
    lightbox = { items: gallery, index: index < 0 ? 0 : index }
  }
</script>

<div class="prompt-message" bind:this={root}>
  {#if parts.text}<div class="text">{parts.text}</div>{/if}
  {#if images.length}
    <div class="thumbs" class:single={images.length === 1}>
      {#each images as image (image.ref)}
        {#if image.preview}
          <button type="button" class="thumb" aria-label={`Preview ${image.name}`} title="Click to enlarge" onclick={() => openPreview(image.preview!)}>
            <img src={image.preview.url} alt={image.name} loading="lazy" />
            <span class="zoom" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7" /></svg></span>
          </button>
        {:else}
          <span class="thumb ghost" title={image.ref}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2.5" /><circle cx="9" cy="10" r="1.6" /><path d="M21 16l-5-5-8 9" /></svg>
            <span>{image.name}</span>
          </span>
        {/if}
      {/each}
    </div>
  {/if}
  {#if lightbox}
    <ImageLightbox items={lightbox.items} index={lightbox.index} onclose={() => { lightbox = null }} />
  {/if}
</div>

<style>
  .text { white-space: pre-wrap; overflow-wrap: anywhere; font: 14px/1.65 var(--sans); }
  .thumbs { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 12px; }
  .thumbs:first-child { margin-top: 0; }
  .thumb { position: relative; display: block; width: 112px; height: 112px; padding: 0; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; background: var(--bg-sunk); cursor: zoom-in; transition: transform .15s var(--ease-out), box-shadow .15s; }
  .thumbs.single .thumb { width: 240px; max-width: 100%; height: auto; max-height: 240px; }
  .thumb img { display: block; width: 100%; height: 100%; object-fit: cover; }
  .thumbs.single .thumb img { height: auto; max-height: 240px; object-fit: contain; }
  .thumb:hover { transform: scale(1.015); box-shadow: 0 4px 14px color-mix(in srgb, var(--ink) 18%, transparent); }
  .thumb:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .thumb .zoom { position: absolute; right: 6px; bottom: 6px; display: grid; place-items: center; width: 24px; height: 24px; border-radius: 6px; background: color-mix(in srgb, var(--ink) 72%, transparent); color: var(--bg); opacity: 0; transition: opacity .15s; }
  .thumb:hover .zoom, .thumb:focus-visible .zoom { opacity: 1; }
  .thumb .zoom svg { width: 13px; height: 13px; }
  .thumb.ghost { display: inline-flex; align-items: center; gap: 7px; width: auto; height: auto; max-width: 100%; padding: 7px 10px; cursor: default; color: var(--muted); font: 14px var(--mono); }
  .thumb.ghost:hover { transform: none; box-shadow: none; }
  .thumb.ghost svg { width: 16px; height: 16px; flex: none; }
  .thumb.ghost span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  @media (prefers-reduced-motion: reduce) { .thumb { transition: none; } .thumb:hover { transform: none; } }
</style>
