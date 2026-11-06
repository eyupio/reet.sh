<script lang="ts">
  import Logo from '../Logo.svelte'
  import type { Tab } from './tabs'
  import { DOCS_URL } from '../links'
  let { tabs, moreCurrent, moreOpen, onmore, oncollapsed }: {
    tabs: Tab[]; moreCurrent: boolean; moreOpen: boolean; onmore: (opener: HTMLElement) => void
    /** Folds the sidebar away, leaving its reopen control in the top bar. */
    oncollapsed: () => void
  } = $props()
  const icons: Record<string, string> = {
    '/': 'M3 11l9-8 9 8v10H3zM9 21v-8h6v8',
    '/sessions': 'M4 6h16M4 12h16M4 18h10',
    '/machines': 'M3 4h18v13H3zM8 21h8M12 17v4',
    '/skills': 'm12 2 2.5 7.5L22 12l-7.5 2.5L12 22l-2.5-7.5L2 12l7.5-2.5Z',
    '/activity': 'M3 12h4l3-8 4 16 3-8h4',
  }
</script>

<aside class="workspace-sidebar" aria-label="This Reet installation">
  <div class="brand-row">
    <a class="brand" href="/" aria-label="Reet workspace home"><Logo /></a>
    <button type="button" class="fold" aria-label="Fold the navigation" title="Fold the navigation" onclick={() => oncollapsed()}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 5l-7 7 7 7" /></svg>
    </button>
  </div>
  <p class="label">YOUR WORKSPACE</p>
  <nav aria-label="Main navigation">
    {#each tabs as tab (tab.href)}
      <a href={tab.href} aria-current={tab.current ? 'page' : undefined}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d={icons[tab.href] ?? 'M4 12h16'}/></svg>
        {tab.label}
      </a>
    {/each}
    <button type="button" class:current={moreCurrent} aria-haspopup="dialog" aria-expanded={moreOpen} aria-label={moreCurrent ? 'More, current page inside' : undefined} onclick={event => onmore(event.currentTarget)}><svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/></svg>More</button>
  </nav>
  <div class="help"><a href={DOCS_URL} target="_blank" rel="noopener noreferrer">Documentation <svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M5 19 19 5M5 5h14v14"/></svg><span class="vh"> (opens in a new tab)</span></a><span>Yours to run. Yours to make.</span></div>
</aside>

<style>
  .workspace-sidebar { display: none; }
  @media (min-width: 1100px) {
    .workspace-sidebar { position: fixed; inset: 0 auto 0 0; z-index: 20; display: flex; flex-direction: column; width: var(--workspace-sidebar-width, 228px); min-width: 0; overflow-y: auto; background: var(--bg-sunk); border-right: 1px solid var(--line); padding: 27px 17px 20px; }
    .brand-row { display: flex; align-items: center; gap: 6px; padding: 0 4px 35px 11px; }
    .brand { display: block; flex: 1; min-width: 0; color: inherit; text-decoration: none; }
    .brand :global(.logo) { width: 134px; max-width: 100%; }
    .fold { flex: none; display: grid; place-items: center; width: 26px; height: 26px; padding: 0; border: 0; border-radius: 6px; background: transparent; color: var(--muted); cursor: pointer; }
    .fold:hover { background: var(--card); color: var(--ink); }
    .fold svg { width: 15px; height: 15px; }
    .label { font: 14px var(--mono); letter-spacing: .08em; color: var(--muted); margin: 0 12px 12px; }
    nav { display: grid; gap: 4px; }
    nav a, nav button { display: flex; align-items: center; gap: 12px; padding: 11px 12px; min-height: 44px; border-radius: 5px; color: var(--muted); font: 400 14px var(--sans); text-decoration: none; border: 0; background: none; cursor: pointer; text-align: left; }
    nav a > svg, nav button > svg { width: 18px; height: 18px; flex: none; }
    nav a[aria-current='page'], nav button.current { color: var(--accent-text); background: var(--card); font-weight: 600; }
    nav a:hover, nav button:hover { background: var(--card); color: var(--ink); }
    a:focus-visible, button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 3px; }
    .help { margin-top: auto; padding: 35px 11px 0; display: grid; gap: 15px; }
    .help a { display: flex; align-items: center; justify-content: space-between; font-size: 14px; color: var(--muted); text-decoration: none; min-height: 30px; }
    .help a:hover { color: var(--accent-text); }
    .help > span { font: 14px var(--mono); color: var(--muted); letter-spacing: .02em; }
  }
</style>
