<script lang="ts">
  import { applyTheme, THEME_EVENT } from './api'

  // The public header's colour choice: light, dark or follow the device. It applies at once and is
  // kept in this browser by applyTheme; after sign-in the profile's saved choice wins, so the control
  // reads the page rather than keeping a copy, and follows a change made anywhere else.
  type Theme = 'system' | 'light' | 'dark'
  const themes: { id: Theme; label: string }[] = [{ id: 'system', label: 'Match this device' }, { id: 'light', label: 'Light' }, { id: 'dark', label: 'Dark' }]
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
  }
</script>

<div class="theme-picker" role="group" aria-label="Theme">
  {#each themes as option (option.id)}
    <button type="button" aria-pressed={theme === option.id} onclick={() => choose(option.id)}>{option.label}</button>
  {/each}
</div>

<style>
  .theme-picker { display: inline-flex; gap: 4px; padding: 3px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  button { min-height: 38px; padding: 6px 10px; border: 1px solid transparent; border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 0.74rem; font-weight: 600; white-space: nowrap; cursor: pointer; }
  button:hover { color: var(--ink); }
  button[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); color: var(--ink); font-weight: 650; }
  @media (max-width: 560px) {
    .theme-picker { width: 100%; }
    button { flex: 1; padding: 6px 4px; }
  }
</style>
