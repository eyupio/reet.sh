<script lang="ts">
  import Logo from '../Logo.svelte'
  import type { Tab } from './tabs'
  import { DOCS_URL } from '../links'
  let { tabs, moreCurrent, moreOpen, onmore }: { tabs: Tab[]; moreCurrent: boolean; moreOpen: boolean; onmore: (opener: HTMLElement) => void } = $props()
  const instance = location.host
  const icons: Record<string, string> = {
    '/': 'M3 11l9-8 9 8v10H3zM9 21v-8h6v8',
    '/sessions': 'M4 6h16M4 12h16M4 18h10',
    '/machines': 'M3 4h18v13H3zM8 21h8M12 17v4',
    '/skills': 'm12 2 2.5 7.5L22 12l-7.5 2.5L12 22l-2.5-7.5L2 12l7.5-2.5Z',
    '/activity': 'M3 12h4l3-8 4 16 3-8h4',
  }
</script>

<aside class="workspace-sidebar" aria-label="This Reet installation">
  <a class="brand" href="/" aria-label="Reet workspace home"><Logo /></a>
  <div class="instance">
    <span class="instance-mark" aria-hidden="true">{instance[0]?.toUpperCase() || 'R'}</span>
    <div><strong title={instance}>{instance}</strong><span>Your Reet installation</span></div>
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg>
  </div>
  <p class="label">YOUR WORKSPACE</p>
  <nav aria-label="Main navigation">
    {#each tabs as tab (tab.href)}
      <a href={tab.href} aria-current={tab.current ? 'page' : undefined}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d={icons[tab.href] ?? 'M4 12h16'}/></svg>
        {tab.label}
      </a>
    {/each}
    <button type="button" class:current={moreCurrent} aria-haspopup="dialog" aria-expanded={moreOpen} aria-label={moreCurrent ? 'More, current page inside' : undefined} onclick={event => onmore(event.currentTarget)}><span class="more-icon" aria-hidden="true">···</span>More</button>
  </nav>
  <div class="help"><a href={DOCS_URL} target="_blank" rel="noopener noreferrer">Documentation <svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M5 19 19 5M5 5h14v14"/></svg><span class="vh"> (opens in a new tab)</span></a><span>Yours to run. Yours to make.</span></div>
</aside>

<style>
  .workspace-sidebar { display: none; }
  @media (min-width: 1100px) {
    .workspace-sidebar { position: fixed; inset: 0 auto 0 0; z-index: 20; display: flex; flex-direction: column; width: 228px; overflow-y: auto; background: var(--bg-sunk); border-right: 1px solid var(--line); padding: 27px 17px 20px; }
    .brand { display: block; color: inherit; text-decoration: none; padding: 0 11px 29px; }
    .brand :global(.logo) { width: 134px; }
    .instance { display: flex; align-items: center; gap: 9px; padding: 12px 10px; border: 1px solid var(--line); border-radius: 7px; background: var(--bg); margin-bottom: 30px; }
    .instance-mark { display: grid; place-items: center; flex: none; width: 31px; height: 34px; background: var(--ink); color: var(--bg); border-radius: 5px; font: 600 20px var(--serif); }
    .instance div { min-width: 0; flex: 1; }
    .instance strong { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 600; }
    .instance div > span { display: block; font-size: 10px; color: var(--muted); margin-top: 3px; }
    .instance > svg { width: 12px; height: 12px; color: var(--muted); flex: none; }
    .label { font: 9px var(--mono); letter-spacing: .08em; color: var(--muted); margin: 0 12px 12px; }
    nav { display: grid; gap: 4px; }
    nav a, nav button { display: flex; align-items: center; gap: 12px; padding: 11px 12px; min-height: 43px; border-radius: 5px; color: var(--muted); font: 400 14px var(--sans); text-decoration: none; border: 0; background: none; cursor: pointer; text-align: left; }
    nav a > svg { width: 18px; height: 18px; flex: none; }
    nav a[aria-current='page'], nav button.current { color: var(--accent-text); background: var(--card); font-weight: 600; }
    nav a:hover, nav button:hover { background: var(--card); color: var(--ink); }
    a:focus-visible, button:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 3px; }
    .more-icon { font: 600 20px/1 var(--mono); width: 18px; text-align: center; }
    .help { margin-top: auto; padding: 35px 11px 0; display: grid; gap: 15px; }
    .help a { display: flex; align-items: center; justify-content: space-between; font-size: 12px; color: var(--muted); text-decoration: none; min-height: 30px; }
    .help a:hover { color: var(--accent-text); }
    .help > span { font: 8px var(--mono); color: var(--muted); letter-spacing: .02em; }
  }
</style>
