<script lang="ts">
  import type { Snippet } from 'svelte'
  import GafferAvatar from './terminal/GafferAvatar.svelte'

  let { title, lede, eyebrow = 'Your own corner of Reet', children }: { title: string; lede: string; eyebrow?: string; children: Snippet } = $props()
  const instance = location.host
</script>

<section class="auth" aria-labelledby="auth-title">
  <aside class="intro" aria-label="Your Reet installation">
    <p class="eyebrow">{eyebrow}</p>
    <p class="headline">Your work.<br />All reet.</p>
    <p class="intro-copy">Your sessions, projects and machines.<br />Right where you left them.</p>
    <div class="gaffer-welcome"><GafferAvatar paused /><div><strong>“Go on, make yourself at home.”</strong><span>Gaffer · Ready when you are</span></div></div>
  </aside>
  <div class="form-panel">
    <div class="instance"><span class="instance-mark" aria-hidden="true">{instance[0]?.toUpperCase() || 'R'}</span><div><strong>{instance}</strong><span>Your Reet installation</span></div><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg></div>
    <h1 id="auth-title">{title}</h1>
    <p class="lede">{lede}</p>
    {@render children()}
  </div>
</section>

<style>
  .auth { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr); gap: clamp(32px, 6vw, 80px); align-items: center; margin: 12px auto 32px; }
  .intro { min-width: 0; padding: 16px 0; }
  .eyebrow { margin: 0 0 20px; color: var(--accent-text); font: 500 14px var(--mono); letter-spacing: 0.01em; }
  .headline { margin: 0; font: 600 clamp(44px, 5vw, 64px)/1.05 var(--serif); letter-spacing: -0.04em; }
  .intro-copy { margin: 24px 0 0; color: var(--muted); font-size: 15px; line-height: 1.75; }
  .gaffer-welcome { display: flex; align-items: center; gap: 16px; margin-top: 32px; }
  .gaffer-welcome :global(.gaffer-avatar) { width: 52px; height: 52px; flex: none; }
  .gaffer-welcome strong { display: block; font: 600 17px/1.4 var(--serif); }
  .gaffer-welcome span { display: block; font: 14px var(--mono); color: var(--muted); letter-spacing: 0.01em; margin-top: 8px; }
  .form-panel { min-width: 0; align-self: center; padding: 28px; background: var(--card); border: 1px solid var(--line); border-radius: 12px; box-shadow: var(--shadow); }
  /* Leave room around focused controls when a short viewport scrolls the form. */
  .form-panel :global(:is(button, a, input, select, summary)) { scroll-margin-block: 16px; }
  .instance { display: flex; align-items: center; gap: 12px; padding-bottom: 20px; margin-bottom: 20px; border-bottom: 1px solid var(--line); }
  .instance-mark { width: 32px; height: 32px; flex: none; display: grid; place-items: center; background: var(--ink); color: var(--bg); border-radius: 6px; font: 600 20px var(--serif); }
  .instance > div { min-width: 0; flex: 1; }
  .instance strong { display: block; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
  .instance div > span { display: block; font-size: 14px; color: var(--muted); margin-top: 2px; }
  .instance > svg { flex: none; width: 16px; height: 16px; color: var(--muted); }
  h1 { margin: 0; font: 600 clamp(28px, 3vw, 34px)/1.15 var(--serif); letter-spacing: -0.03em; }
  .lede { margin: 12px 0 0; color: var(--muted); max-width: 40ch; font-size: 14px; line-height: 1.7; }
  @media (max-width: 760px) {
    .auth { grid-template-columns: 1fr; max-width: 490px; gap: 0; }
    .intro { display: none; }
    .eyebrow { font-size: 14px; margin-bottom: 16px; }
    .headline { font-size: 44px; }
    .intro-copy { margin: 16px 0 0; font-size: 14px; }
    .gaffer-welcome { display: none; }
    .form-panel { padding: 24px; }
    h1 { font-size: 28px; }
    .instance { padding-bottom: 16px; margin-bottom: 16px; }
  }
</style>
