<script lang="ts">
  import type { DraftImage } from '../sessionBoard'
  import { placePreview, type PreviewPlace, type PreviewAnchor } from './imagePreview'

  let { images, count, loading = false, error = '', anchor, onenter, onleave }: {
    images: DraftImage[]
    count: number
    loading?: boolean
    error?: string
    anchor: PreviewAnchor
    onenter: () => void
    onleave: () => void
  } = $props()
  const id = $props.id()

  let card: HTMLElement | undefined = $state()
  let place = $state<PreviewPlace>({ left: 0, top: 0, placement: 'below' })
  let ready = $state(false)
  let frame = 0

  function measure() {
    if (!card) return
    const next = placePreview(anchor, { width: card.offsetWidth, height: card.offsetHeight }, { width: innerWidth, height: innerHeight })
    place = next
    ready = true
  }
  function schedule() {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(measure)
  }
  $effect(() => {
    void images.length; void loading; void error
    schedule()
  })
  $effect(() => {
    const observer = new ResizeObserver(schedule)
    if (card) observer.observe(card)
    return () => observer.disconnect()
  })
</script>

<svelte:window onresize={schedule} />
<svelte:document onscroll={schedule} />

<div class="image-preview" bind:this={card} role="tooltip" id={id} data-placement={place.placement} class:ready
  style={`left:${place.left}px;top:${place.top}px`} onpointerenter={onenter} onpointerleave={onleave}>
  <header>
    <span class="eyebrow">Reference image{count === 1 ? '' : 's'}</span>
    <span class="count" aria-hidden="true">{count}</span>
  </header>
  {#if loading && images.length === 0}
    <div class="grid skeleton" aria-hidden="true">
      {#each Array(Math.min(Math.max(count, 1), 4)) as _, index (index)}<span></span>{/each}
    </div>
    <p class="reading" role="status">Reading the images…</p>
  {:else if error}
    <p class="reading bad" role="status">{error}</p>
  {:else if images.length === 0}
    <p class="reading" role="status">No preview to show.</p>
  {:else}
    <div class="grid" class:single={images.length === 1}>
      {#each images as image, index (image.name + index)}
        <figure>
          <span class="frame"><img src={image.data} alt={image.name} loading="lazy" decoding="async" /></span>
          <figcaption title={image.name}>{image.name}</figcaption>
        </figure>
      {/each}
    </div>
    <footer>Click the image icon to view full size</footer>
  {/if}
</div>

<style>
  .image-preview { position: fixed; z-index: 70; width: min(326px, calc(100vw - 16px)); padding: 12px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card-raised); color: var(--ink); box-shadow: 0 20px 44px -20px var(--scrim), 0 2px 8px color-mix(in srgb, var(--ink) 6%, transparent); opacity: 0; transform: translateY(-3px) scale(.985); transition: opacity .14s ease, transform .18s var(--ease-out); }
  .image-preview[data-placement='above'] { transform: translateY(3px) scale(.985); }
  .image-preview.ready { opacity: 1; transform: none; }
  header { display: flex; align-items: center; gap: 8px; margin-bottom: 9px; }
  .eyebrow { font-size: 14px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; color: var(--muted); }
  .count { margin-left: auto; min-width: 20px; text-align: center; padding: 1px 6px; border: 1px solid var(--line); border-radius: 99px; background: var(--bg-sunk); font-size: 14px; font-weight: 600; color: var(--ink-soft); }
  .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
  .grid.single { grid-template-columns: minmax(0, 1fr); }
  figure { margin: 0; min-width: 0; }
  .frame { display: block; aspect-ratio: 4 / 3; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; background: var(--bg-sunk); }
  .single .frame { aspect-ratio: 16 / 10; }
  img { display: block; width: 100%; height: 100%; object-fit: cover; }
  figcaption { margin-top: 5px; font-size: 14px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .skeleton span { display: block; aspect-ratio: 4 / 3; border-radius: 8px; background: linear-gradient(100deg, var(--bg-sunk) 30%, var(--line-soft) 50%, var(--bg-sunk) 70%); background-size: 220% 100%; animation: sheen 1.2s linear infinite; }
  .reading { margin: 9px 0 0; font-size: 14px; color: var(--muted); }
  .reading.bad { color: var(--warn); }
  footer { margin-top: 9px; padding-top: 8px; border-top: 1px solid var(--line-soft); font-size: 14px; color: var(--muted); }
  @keyframes sheen { to { background-position: -120% 0; } }
  @media (prefers-reduced-motion: reduce) {
    .image-preview { transition: none; transform: none; }
    .skeleton span { animation: none; }
  }
</style>
