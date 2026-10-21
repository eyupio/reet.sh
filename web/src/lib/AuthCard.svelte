<script lang="ts">
  import type { Snippet } from 'svelte'

  let { title, lede, eyebrow = 'Your personal library', children }: { title: string; lede: string; eyebrow?: string; children: Snippet } = $props()
</script>

<section class="auth" aria-labelledby="auth-title">
  <aside class="intro" aria-label="About your shelf">
    <p class="eyebrow">{eyebrow}</p>
    <p class="headline">A home for the skills<br />worth keeping.</p>
    <p class="intro-copy">Find it. Try it. Make it part of your toolkit.</p>
    <ol class="stages" aria-label="How the shelf works">
      <li><span aria-hidden="true">01</span><div><strong>On the shelf</strong><small>Keep a good find for later.</small></div></li>
      <li><span aria-hidden="true">02</span><div><strong>To test</strong><small>Give something new a go.</small></div></li>
      <li><span aria-hidden="true">03</span><div><strong>In use</strong><small>Know what you reach for.</small></div></li>
    </ol>
    <p class="local">Self-hosted. Yours to keep.</p>
  </aside>
  <div class="form-panel">
    <h2 id="auth-title">{title}</h2>
    <p class="lede">{lede}</p>
    {@render children()}
  </div>
</section>

<style>
  .auth {
    display: grid; grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr);
    margin: 36px auto 0; overflow: hidden;
    background: var(--card); border: 1px solid var(--line); border-radius: 18px;
    box-shadow: var(--shadow);
  }
  .intro { display: flex; flex-direction: column; padding: clamp(24px, 4vw, 40px); background: color-mix(in srgb, var(--tint) 28%, var(--bg)); border-right: 1px solid var(--line); }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .headline { margin: 22px 0 0; font: 500 clamp(1.75rem, 3vw, 2.25rem)/1.12 var(--serif); letter-spacing: -0.03em; }
  .intro-copy { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  .stages { list-style: none; display: grid; gap: 20px; padding: 0; margin: 32px 0; }
  .stages li { display: flex; align-items: baseline; gap: 14px; }
  .stages li > span { color: var(--accent); font: 0.72rem var(--mono); }
  .stages strong { display: block; font-size: 0.88rem; font-weight: 600; }
  .stages small { display: block; margin-top: 2px; color: var(--muted); font-size: 0.8rem; }
  .local { margin: auto 0 0; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.75rem; }
  .form-panel { min-width: 0; align-self: center; padding: clamp(24px, 4vw, 40px); }
  h2 { margin: 0; font: 600 1.7rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 40ch; }
  @media (max-width: 560px) {
    .auth { grid-template-columns: 1fr; margin-top: 24px; border-radius: 14px; }
    .intro { padding: 22px; border-right: 0; border-bottom: 1px solid var(--line); }
    .headline { margin-top: 10px; font-size: 1.65rem; }
    .headline br { display: none; }
    .intro-copy, .stages, .local { display: none; }
    .form-panel { padding: 24px 22px; }
    h2 { font-size: 1.5rem; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .auth { animation: rise 320ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  }
</style>
