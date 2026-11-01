<script lang="ts">
  import type { Snippet } from 'svelte'

  let { title, lede, eyebrow = 'Ey up. Your AI workspace', children }: { title: string; lede: string; eyebrow?: string; children: Snippet } = $props()
</script>

<section class="auth" aria-labelledby="auth-title">
  <aside class="intro" aria-label="About your workspace">
    <p class="eyebrow">{eyebrow}</p>
    <p class="headline">Your whole AI setup. <br />On every machine.</p>
    <p class="intro-copy">Enrol the machines you work on and your agents, skills and setups go with you. Open a terminal to use your installed tools and return to ongoing work.</p>
    <ol class="stages" aria-label="How your workspace works">
      <li><span aria-hidden="true">01</span><div><strong>Your machines</strong><small>Enrol a laptop, a VPS or a lab box with one command.</small></div></li>
      <li><span aria-hidden="true">02</span><div><strong>Your agent toolkit</strong><small>Skills and reusable setups, in sync across them.</small></div></li>
      <li><span aria-hidden="true">03</span><div><strong>Your ongoing work</strong><small>Continue the same session when you return.</small></div></li>
    </ol>
    <p class="local">Self-hosted. No telemetry. AGPL-3.0.</p>
  </aside>
  <div class="form-panel">
    <h1 id="auth-title">{title}</h1>
    <p class="lede">{lede}</p>
    {@render children()}
  </div>
</section>

<style>
  .auth {
    display: grid; grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
    margin: 12px auto 24px; overflow: hidden;
    background: var(--card); border: 1px solid var(--line); border-radius: 18px;
    box-shadow: var(--shadow);
  }
  .intro { display: flex; flex-direction: column; padding: clamp(24px, 4vw, 40px); background: color-mix(in srgb, var(--tint) 28%, var(--bg)); border-right: 1px solid var(--line); }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .headline { margin: 22px 0 0; font: 500 clamp(1.75rem, 3vw, 2.25rem)/1.12 var(--serif); letter-spacing: -0.03em; }
  .intro-copy { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  .stages { list-style: none; display: grid; gap: 20px; padding: 0; margin: 32px 0; }
  .stages li { display: flex; align-items: baseline; gap: 14px; }
  .stages li > span { color: var(--accent-text); font: 0.72rem var(--mono); }
  .stages strong { display: block; font-size: 0.88rem; font-weight: 600; }
  .stages small { display: block; margin-top: 2px; color: var(--muted); font-size: 0.8rem; }
  .local { margin: auto 0 0; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.75rem; }
  .form-panel { min-width: 0; align-self: center; padding: clamp(24px, 4vw, 40px); }
  h1 { margin: 0; font: 500 1.8rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 40ch; }
  @media (max-width: 560px) {
    .auth { grid-template-columns: 1fr; margin-top: 24px; border-radius: 14px; }
    /* The form and its button come first on a phone; the explanation follows it. */
    .intro { order: 2; padding: 22px; border-right: 0; border-top: 1px solid var(--line); }
    .form-panel { order: 1; }
    .headline { margin-top: 10px; font-size: 1.65rem; }
    .headline br { display: none; }
    .intro-copy { font-size: 0.84rem; }
    .stages { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0 0; }
    .stages li { gap: 5px; align-items: center; }
    .stages strong { display: inline; font-size: 0.76rem; }
    .stages small { display: none; }
    .stages li > span { font-size: 0.64rem; }
    .local { margin-top: 12px; padding-top: 10px; font-size: 0.72rem; }
    .form-panel { padding: 24px 22px; }
    h1 { font-size: 1.5rem; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .auth { animation: rise 320ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  }
</style>
