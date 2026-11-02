<script lang="ts">
  // One thin bar along the top edge while any request is outstanding (lib/inflight.ts decides when). It is the whole
  // app's "working" signal, so a page that reads its data on arrival needs no indicator of its own, and a refresh that
  // answers at once shows nothing at all.
  import { onMount } from 'svelte'
  import { inflight } from '../inflight'

  let visible = $state(inflight.visible())
  onMount(() => inflight.subscribe(v => (visible = v)))
</script>

<div class="progress" class:visible aria-hidden="true"><span></span></div>

<style>
  .progress { position: fixed; inset: 0 0 auto 0; z-index: 60; height: 3px; overflow: hidden; pointer-events: none; opacity: 0; background: color-mix(in srgb, var(--accent) 22%, transparent); }
  .progress.visible { opacity: 1; }
  .progress span { display: block; width: 36%; height: 100%; border-radius: 0 3px 3px 0; background: var(--accent); transform: translateX(-100%); }
  .progress.visible span { animation: progress-sweep 1.1s cubic-bezier(0.4, 0, 0.6, 1) infinite; }
  @keyframes progress-sweep { 0% { transform: translateX(-100%); } 100% { transform: translateX(280%); } }
  @media (prefers-reduced-motion: no-preference) {
    .progress { transition: opacity 160ms var(--ease-out); }
  }
  @media (prefers-reduced-motion: reduce) {
    /* Steady rather than sweeping: a bar that is simply there says the same thing without the motion. */
    .progress.visible span { animation: none; width: 100%; transform: none; }
  }
</style>
