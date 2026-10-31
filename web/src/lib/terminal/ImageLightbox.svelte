<script lang="ts">
 import { onMount, tick } from 'svelte'
 import { fade, scale } from 'svelte/transition'

 let { items, index = 0, onclose }: { items: { url: string; name: string }[]; index?: number; onclose: () => void } = $props()
 let at = $state(0)
 $effect.pre(() => { at = index })
 let closeButton: HTMLButtonElement | undefined = $state()
 let opener: Element | null = null
 const current = $derived(items[at])
 const many = $derived(items.length > 1)

 function step(by: number) { at = (at + by + items.length) % items.length }
 function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onclose() }
  else if (many && event.key === 'ArrowLeft') { event.preventDefault(); step(-1) }
  else if (many && event.key === 'ArrowRight') { event.preventDefault(); step(1) }
  else if (event.key === 'Tab') { event.preventDefault(); closeButton?.focus() }
 }
 onMount(() => {
  opener = document.activeElement
  void tick().then(() => closeButton?.focus())
  return () => { if (opener instanceof HTMLElement && opener.isConnected) opener.focus() }
 })
</script>

<svelte:window onkeydown={keydown} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="lightbox" role="dialog" tabindex="-1" aria-modal="true" aria-label={`Image preview: ${current?.name ?? ''}`} transition:fade={{ duration: 140 }} onclick={(event) => { if (event.target === event.currentTarget) onclose() }}>
 <button type="button" class="close" bind:this={closeButton} aria-label="Close preview" onclick={onclose}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg></button>
 {#if many}<button type="button" class="nav prev" aria-label="Previous image" onclick={() => step(-1)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 6l-6 6 6 6" /></svg></button>{/if}
 {#key at}
  <figure in:scale={{ duration: 160, start: 0.96 }}>
   <img src={current?.url} alt={current?.name ?? ''} />
   <figcaption>{current?.name}{#if many} · {at + 1} of {items.length}{/if}</figcaption>
  </figure>
 {/key}
 {#if many}<button type="button" class="nav next" aria-label="Next image" onclick={() => step(1)}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 6l6 6-6 6" /></svg></button>{/if}
</div>

<style>
 .lightbox { position: fixed; inset: 0; z-index: 100; display: grid; place-items: center; padding: 56px 64px; background: color-mix(in srgb, black 82%, transparent); backdrop-filter: blur(4px); }
 figure { margin: 0; display: flex; flex-direction: column; align-items: center; gap: 10px; max-width: 100%; max-height: 100%; min-height: 0; }
 img { max-width: 100%; max-height: calc(100vh - 150px); border-radius: 10px; object-fit: contain; background: color-mix(in srgb, white 12%, transparent); box-shadow: 0 18px 60px color-mix(in srgb, black 60%, transparent); }
 figcaption { font: 500 12.5px var(--sans); color: color-mix(in srgb, white 94%, transparent); max-width: 80vw; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
 button { position: absolute; display: grid; place-items: center; width: 44px; height: 44px; border: 1px solid color-mix(in srgb, white 20%, transparent); border-radius: 50%; background: color-mix(in srgb, black 55%, transparent); color: color-mix(in srgb, white 94%, transparent); cursor: pointer; transition: background .15s; }
 button:hover { background: color-mix(in srgb, black 80%, transparent); }
 button:focus-visible { outline: 2px solid var(--accent, white); outline-offset: 2px; }
 button svg { width: 20px; height: 20px; }
 .close { top: 14px; right: 14px; }
 .nav { top: 50%; transform: translateY(-50%); }
 .prev { left: 12px; }
 .next { right: 12px; }
 @media (max-width: 650px) { .lightbox { padding: 64px 12px 24px; } .nav { top: auto; bottom: 14px; transform: none; } }
 @media (prefers-reduced-motion: reduce) { button { transition: none; } }
</style>
