<script lang="ts">
  import { applyTheme, THEME_EVENT } from './api'

  // The public header's colour choice: light, dark or follow the device, as three icons with the
  // chosen one ringed. It applies at once and is kept in this browser by applyTheme; after sign-in
  // the profile's saved choice wins, so the control reads the page rather than keeping a copy, and
  // follows a change made anywhere else.
  type Theme = 'system' | 'light' | 'dark'
  // A signed-in menu passes onchoose to save the choice; it applies at once either way, and a save
  // that fails puts the page's theme back through applyTheme, which this control then follows.
  let { onchoose }: { onchoose?: (next: Theme) => void | Promise<void> } = $props()
  const themes: { id: Theme; label: string }[] = [{ id: 'light', label: 'Light' }, { id: 'dark', label: 'Dark' }, { id: 'system', label: 'Match this device' }]
  const currentTheme = (): Theme => {
    const value = document.documentElement.dataset.theme
    return value === 'light' || value === 'dark' ? value : 'system'
  }
  let theme = $state<Theme>(currentTheme())
  $effect(() => {
    const follow = () => { theme = currentTheme() }
    window.addEventListener(THEME_EVENT, follow)
    return () => window.removeEventListener(THEME_EVENT, follow)
  })
  function choose(next: Theme) {
    theme = next
    applyTheme(next)
    void onchoose?.(next)
  }
</script>

<div class="theme-picker" role="group" aria-label="Theme">
  {#each themes as option (option.id)}
    <button type="button" aria-pressed={theme === option.id} aria-label={option.label} title={option.label} onclick={() => choose(option.id)}>
      {#if option.id === 'light'}
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="4" /><path d="M12 2v2m0 16v2M2 12h2m16 0h2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4m0-14.2-1.4 1.4M6.3 17.7l-1.4 1.4" /></svg>
      {:else if option.id === 'dark'}
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" /></svg>
      {:else}
        <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="12" rx="2" /><path d="M8 20h8m-4-4v4" /></svg>
      {/if}
    </button>
  {/each}
</div>

<style>
  .theme-picker { display: inline-flex; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: 999px; background: var(--card); }
  button { display: inline-grid; place-items: center; width: 36px; height: 36px; padding: 0; border: 1px solid transparent; border-radius: 50%; background: none; color: var(--muted); cursor: pointer; }
  button svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
  button:hover { color: var(--ink); }
  button[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--accent-text); }
  /* A finger gets the touch size the rest of the chrome keeps. */
  @media (pointer: coarse) { button { width: 44px; height: 44px; } }
</style>
