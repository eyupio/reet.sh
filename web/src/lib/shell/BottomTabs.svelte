<script lang="ts">
  import type { Tab } from './tabs'
  // The phone's tab bar: Home, Sessions, Machines and More, 64 px tall plus the safe area, each target at least 44 px. It
  // shows only on a phone (PHONE_SCREEN in terminal/sizing.ts) and steps aside while the keyboard is up for a field that
  // asks it to (keyboardChrome).
  let { tabs, moreCurrent, moreOpen, hidden, onmore }: { tabs: Tab[]; moreCurrent: boolean; moreOpen: boolean; hidden: boolean; onmore: (opener: HTMLElement) => void } = $props()
  const icons: Record<string, string> = {
    '/': 'M3 11l9-8 9 8v10H3z',
    '/sessions': 'M4 6h16M4 12h16M4 18h10',
    '/machines': 'M5 5h14a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2zM8 21h8M12 17v4',
  }
</script>

<nav class="bottom-tabs" class:hidden aria-label="Main navigation">
  {#each tabs as tab (tab.href)}
    <a href={tab.href} aria-current={tab.current ? 'page' : undefined}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d={icons[tab.href] ?? 'M4 12h16'} /></svg>
      <span>{tab.label}</span>
    </a>
  {/each}
  <button type="button" class:current={moreCurrent} aria-haspopup="dialog" aria-expanded={moreOpen}
    aria-label={moreCurrent ? 'More, current page inside' : undefined} onclick={(event) => onmore(event.currentTarget)}>
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="19" cy="12" r="1.6" /></svg>
    <span>More</span>
  </button>
</nav>

<style>
  .bottom-tabs { display: none; }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .bottom-tabs {
      position: fixed; left: 0; right: 0; bottom: 0; z-index: 12; display: flex;
      height: calc(64px + env(safe-area-inset-bottom, 0px)); padding: 0 env(safe-area-inset-right, 0px) env(safe-area-inset-bottom, 0px) env(safe-area-inset-left, 0px);
      box-sizing: border-box; border-top: 1px solid var(--line-soft); background: var(--dock-bg);
    }
    .bottom-tabs.hidden { display: none; }
  }
  a, button {
    flex: 1 1 0; min-width: 44px; min-height: 44px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 2px;
    padding: 0; border: 0; background: none; color: var(--muted); font: 400 0.75rem/1.2 var(--sans); text-decoration: none; cursor: pointer;
  }
  a[aria-current='page'], button.current { position: relative; color: var(--accent-text); font-weight: 600; }
  /* Not colour alone: a bar along the top edge marks the current tab, or More when the page in view is inside it. */
  a[aria-current='page']::after, button.current::after {
    content: ''; position: absolute; top: 0; left: 50%; width: 28px; height: 3px; margin-left: -14px; border-radius: 0 0 3px 3px; background: var(--accent-text);
  }
  a:focus-visible, button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -4px; border-radius: 10px; }
  svg { width: 22px; height: 22px; }
</style>
