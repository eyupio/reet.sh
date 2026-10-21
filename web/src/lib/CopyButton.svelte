<script lang="ts">
  import { onDestroy } from 'svelte'
  import { copyText } from './clipboard'

  let { text, label = 'Copy', copiedLabel = 'Copied', compact = false, oncopy }: {
    text: string
    label?: string
    copiedLabel?: string
    compact?: boolean
    oncopy?: () => void
  } = $props()

  let copied = $state(false)
  let timer: ReturnType<typeof setTimeout> | undefined

  async function copy() {
    if (!(await copyText(text))) return
    oncopy?.()
    copied = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied = false), 1800)
  }

  onDestroy(() => clearTimeout(timer))
</script>

<button type="button" class="btn copy" class:compact onclick={copy}>
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    {#if copied}
      <g class="success"><path d="m5 12 4 4L19 6"></path></g>
    {:else}
      <rect x="8" y="8" width="11" height="11" rx="2"></rect>
      <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"></path>
    {/if}
  </svg>
  <span>{copied ? copiedLabel : label}</span>
  <span class="vh" role="status">{copied ? 'Copied to clipboard' : ''}</span>
</button>

<style>
  .copy { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
  .copy svg { width: 15px; height: 15px; flex: none; }
  .copy.compact { min-height: 32px; padding: 5px 10px; border-radius: 8px; font-size: 0.82rem; }
  .success {
    opacity: 1; transform: scale(1); transform-box: fill-box; transform-origin: center;
    transition: opacity 140ms var(--ease-out), transform 140ms var(--ease-out);
    @starting-style { opacity: 0; transform: scale(0.95); }
  }
  @media (prefers-reduced-motion: reduce) {
    .success {
      transform: none; transition: opacity 100ms var(--ease-out);
      @starting-style { opacity: 0; transform: none; }
    }
  }
  @media (pointer: coarse) { .copy.compact { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) {
    .copy svg { transition: transform 160ms var(--ease-out); }
    .copy:active svg { transform: scale(0.9); }
  }
</style>
