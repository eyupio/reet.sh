<script lang="ts">
 import { onMount } from 'svelte'
 import type { ChatPreviewRef } from '../chat'
 import { chatPreview } from './api'
 import ImageLightbox from './ImageLightbox.svelte'
 let { session, preview, onresize }: { session: string; preview: ChatPreviewRef; onresize?: () => void } = $props()
 let element: HTMLElement
 let reserved = $state(0)
 let url = $state(''), loading = $state(false), problem = $state(''), expanded = $state(false)
 let request: AbortController | undefined, revision = 0, alive = true
 const reason = $derived(preview.reason === 'too_large' ? 'This image is larger than the 5 MB preview limit.' : preview.reason === 'unsupported' ? 'This image format or size cannot be previewed.' : 'This image preview is unavailable.')
 function release() { if (url) URL.revokeObjectURL(url); url = '' }
 async function load() {
  if (!preview.id || preview.reason || loading || url) return
  const own = ++revision
  request?.abort(); request = new AbortController(); loading = true; problem = ''
  try {
   const blob = await chatPreview(session, preview.id, request.signal)
   if (!alive || own !== revision) return
   if (blob.size !== preview.size || blob.type !== preview.mime) throw new Error('This image preview could not be verified.')
   url = URL.createObjectURL(blob)
  } catch (e) {
   if (alive && own === revision && !request?.signal.aborted) problem = e instanceof Error ? e.message : 'Couldn’t load this preview. Try again.'
  } finally { if (alive && own === revision) loading = false }
 }
 function retire() { if (url && element) reserved = element.getBoundingClientRect().height; ++revision; request?.abort(); loading = false; if (!expanded) release() }
 onMount(() => {
  const observer = typeof IntersectionObserver === 'undefined' ? undefined : new IntersectionObserver(entries => {
   if (entries.some(entry => entry.isIntersecting)) void load()
   else retire()
  }, { rootMargin: '200px' })
  if (observer) observer.observe(element); else void load()
  return () => { alive = false; observer?.disconnect(); retire(); release() }
 })
</script>

<figure class="agent-preview" bind:this={element} style:min-height={reserved ? `${reserved}px` : undefined} aria-label={`Agent image: ${preview.name}`}>
 {#if url}
  <button type="button" class="image" aria-label={`Open image preview: ${preview.name}`} onclick={() => expanded = true}><img src={url} alt={preview.name} loading="lazy" onload={() => onresize?.()} onerror={() => { problem = 'This image could not be decoded.'; release() }} /></button>
 {:else}
  <div class="placeholder" aria-live="polite">{loading ? 'Loading image preview…' : preview.reason ? reason : problem || 'Image preview'}</div>
  {#if !preview.reason && !loading}<button type="button" class="retry" onclick={() => void load()}>{problem ? 'Retry preview' : 'Load preview'}</button>{/if}
 {/if}
 <figcaption>{preview.name}{#if url}<span>Open for full size</span>{/if}</figcaption>
</figure>
{#if expanded && url}<ImageLightbox items={[{url, name: preview.name}]} onclose={() => expanded = false} />{/if}

<style>
 .agent-preview { margin: 8px 0 12px; max-width: 100%; min-width: 0; padding: 10px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
 .image { display: block; width: 100%; min-height: 44px; border: 0; padding: 0; cursor: zoom-in; background: transparent; }
 img { display: block; max-width: 100%; max-height: 340px; margin: auto; object-fit: contain; border-radius: 5px; }
 figcaption { display: flex; flex-wrap: wrap; gap: 4px 12px; justify-content: space-between; margin-top: 8px; color: var(--muted); font: 14px/1.5 var(--sans); overflow-wrap: anywhere; }
 .placeholder { min-height: 44px; color: var(--muted); font: 14px/1.6 var(--sans); }
 .retry { min-height: 44px; padding: 6px 12px; color: var(--ink); border: 1px solid var(--line); border-radius: 6px; background: var(--card); cursor: pointer; }
 button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
 @media (max-width: 650px) { .agent-preview { padding: 8px; } img { max-height: 260px; } }
</style>
