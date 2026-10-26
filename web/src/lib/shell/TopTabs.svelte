<script lang="ts">
  import type { Tab } from './tabs'
  // The tabs along the top bar: a nav of links, not an ARIA tablist (each one changes the page). A desktop shows every tab;
  // a tablet shows the bar's tabs (Home, Sessions, Machines) and More, which opens the rest. A phone hides this row and uses
  // the bottom tab bar instead. `moreOnDesktop` keeps More on a desktop when no account menu can hold it (an open instance).
  let { tabs, bar, moreCurrent, moreOnDesktop, moreOpen, onmore }: {
    tabs: Tab[]; bar: Tab[]; moreCurrent: boolean; moreOnDesktop: boolean; moreOpen: boolean; onmore: (opener: HTMLElement) => void
  } = $props()
  const inBar = (tab: Tab) => bar.some(b => b.href === tab.href)
</script>

<nav class="top-tabs" aria-label="Main navigation">
  {#each tabs as tab (tab.href)}
    <a href={tab.href} class:wide={!inBar(tab)} aria-current={tab.current ? 'page' : undefined}>{tab.label}</a>
  {/each}
  <button type="button" class="more" class:current={moreCurrent} class:desktop={moreOnDesktop} aria-haspopup="dialog" aria-expanded={moreOpen}
    aria-label={moreCurrent ? 'More, current page inside' : undefined} onclick={(event) => onmore(event.currentTarget)}>More</button>
</nav>

<style>
  .top-tabs { display: flex; align-items: stretch; align-self: stretch; gap: 2px; min-width: 0; }
  a, .more {
    position: relative; display: flex; align-items: center; min-height: 44px; padding: 0 14px;
    border: 0; background: none; color: var(--muted); font: 500 0.9rem/1 var(--sans); text-decoration: none; white-space: nowrap; cursor: pointer;
  }
  a:hover, .more:hover { color: var(--ink); }
  a[aria-current='page'], .more.current { color: var(--ink); font-weight: 600; }
  a[aria-current='page']::after, .more.current::after {
    content: ''; position: absolute; left: 10px; right: 10px; bottom: 0; height: 2px; border-radius: 2px; background: var(--accent-text);
  }
  a:focus-visible, .more:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -4px; border-radius: 8px; }
  .more { display: none; }
  .more.desktop { display: flex; }
  /* Tablet: Home, Sessions, Machines and More. */
  @media (max-width: 1099.98px) {
    a.wide { display: none; }
    .more { display: flex; }
    a, .more { padding: 0 12px; }
  }
  /* Phone: the bottom tab bar takes over. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .top-tabs { display: none; }
  }
</style>
