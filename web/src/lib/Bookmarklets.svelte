<script lang="ts">
  import { bookmarklet } from './bookmarklet'

  let { token, hint = true }: { token: string; hint?: boolean } = $props()

  const origin = location.origin
</script>

<p class="links">
  <!-- svelte-ignore a11y_invalid_attribute -->
  <a class="pill" href={bookmarklet('like', origin, token)}>♡ Like on reet</a>
  <!-- svelte-ignore a11y_invalid_attribute -->
  <a class="pill" href={bookmarklet('save', origin, token)}>☆ To test on reet</a>
  <!-- svelte-ignore a11y_invalid_attribute -->
  <a class="pill" href={bookmarklet('used', origin, token)}>✔ In use on reet</a>
</p>
{#if hint && token}<p class="hint">These links contain your access token, so keep them private.</p>{/if}

<style>
  .links { display: flex; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .pill {
    display: inline-flex; align-items: center; padding: 6px 14px; border-radius: 999px;
    border: 1px dashed var(--accent); color: var(--ink); text-decoration: none; min-height: 36px;
  }
  .pill:hover { background: var(--tint); }
  .hint { margin: 4px 0 0; font-size: 0.85rem; color: var(--muted); }
  @media (pointer: coarse) { .pill { min-height: 44px; } }
  @media (max-width: 560px) {
    .links { flex-direction: column; align-items: stretch; }
    .pill { justify-content: center; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .pill { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
  }
</style>
