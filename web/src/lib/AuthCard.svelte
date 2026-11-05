<script lang="ts">
  import type { Snippet } from 'svelte'
  import GafferAvatar from './terminal/GafferAvatar.svelte'

  let { title, lede, eyebrow = 'YOUR OWN CORNER OF REET', children }: { title: string; lede: string; eyebrow?: string; children: Snippet } = $props()
  const instance = location.host
</script>

<section class="auth" aria-labelledby="auth-title">
  <aside class="intro" aria-label="Your Reet installation">
    <p class="eyebrow">{eyebrow}</p>
    <p class="headline">Your work.<br />All reet.</p>
    <p class="intro-copy">Your sessions, projects and machines.<br />Right where you left them.</p>
    <div class="gaffer-welcome"><GafferAvatar paused /><div><strong>“Go on, make yourself at home.”</strong><span>GAFFER / READY WHEN YOU ARE</span></div></div>
    <p class="local">This is the Reet installation at <strong>{instance}</strong>.</p>
  </aside>
  <div class="form-panel">
    <div class="instance"><span class="instance-mark" aria-hidden="true">{instance[0]?.toUpperCase() || 'R'}</span><div><strong>{instance}</strong><span>Your Reet installation</span></div><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></svg></div>
    <h1 id="auth-title">{title}</h1>
    <p class="lede">{lede}</p>
    {@render children()}
  </div>
</section>

<style>
  .auth { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr); gap: clamp(35px, 6vw, 85px); align-items: center; margin: 12px auto 35px; }
  .intro { min-width: 0; padding: 18px 0; }
  .eyebrow { margin: 0 0 23px; color: var(--accent-text); font: 14px var(--mono); letter-spacing: .12em; text-transform: uppercase; }
  .headline { margin: 0; font: 600 clamp(43px, 5.1vw, 66px)/1.04 var(--serif); letter-spacing: -.045em; }
  .intro-copy { margin: 25px 0 0; color: var(--muted); font-size: 15px; line-height: 1.85; }
  .gaffer-welcome { display: flex; align-items: center; gap: 15px; margin-top: 36px; }
  .gaffer-welcome :global(.gaffer-avatar) { width: 52px; height: 52px; flex: none; }
  .gaffer-welcome strong { display: block; font: 600 17px/1.4 var(--serif); }
  .gaffer-welcome span { display: block; font: 14px var(--mono); color: var(--muted); letter-spacing: .04em; margin-top: 8px; }
  .local { margin: 34px 0 0; padding-top: 22px; border-top: 1px solid var(--line); color: var(--muted); font-size: 14px; line-height: 1.8; overflow-wrap: anywhere; }
  .local strong { font-weight: 500; color: var(--ink); }
  .form-panel { min-width: 0; align-self: center; padding: 30px; background: var(--card); border: 1px solid var(--line); border-radius: 11px; box-shadow: 0 18px 50px color-mix(in srgb, var(--ink) 5%, transparent); }
  /* Leave room around focused controls when a short viewport scrolls the form. */
  .form-panel :global(:is(button, a, input, select, summary)) { scroll-margin-block: 16px; }
  .instance { display: flex; align-items: center; gap: 11px; padding-bottom: 23px; margin-bottom: 25px; border-bottom: 1px solid var(--line); }
  .instance-mark { width: 32px; height: 35px; flex: none; display: grid; place-items: center; background: var(--ink); color: var(--bg); border-radius: 5px; font: 600 21px var(--serif); }
  .instance > div { min-width: 0; flex: 1; }
  .instance strong { display: block; font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
  .instance div > span { display: block; font-size: 14px; color: var(--muted); margin-top: 3px; }
  .instance > svg { flex: none; width: 15px; height: 15px; color: var(--muted); }
  h1 { margin: 0; font: 600 clamp(28px, 3vw, 35px)/1.15 var(--serif); letter-spacing: -.03em; }
  .lede { margin: 11px 0 0; color: var(--muted); max-width: 40ch; font-size: 14px; line-height: 1.8; }
  @media (max-width: 760px) {
    .auth { grid-template-columns: 1fr; max-width: 490px; gap: 0; }
    .intro { display: none; }
    .eyebrow { font-size: 14px; margin-bottom: 15px; }
    .headline { font-size: 44px; }
    .intro-copy { margin: 17px 0 0; font-size: 14px; }
    .gaffer-welcome, .local { display: none; }
    .form-panel { padding: 25px; }
    h1 { font-size: 28px; }
    .instance { padding-bottom: 16px; margin-bottom: 19px; }
  }
</style>
