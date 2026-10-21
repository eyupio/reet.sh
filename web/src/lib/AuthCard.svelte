<script lang="ts">
  import type { Snippet } from 'svelte'

  let { title, lede, children }: { title: string; lede: string; children: Snippet } = $props()
</script>

<section class="auth" aria-labelledby="auth-title">
  <span class="mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="8" cy="15" r="4"></circle>
      <path d="m10.8 12.2 8.4-8.4M16 7l3 3M13 10l3 3"></path>
    </svg>
  </span>
  <h2 id="auth-title">{title}</h2>
  <p class="lede">{lede}</p>
  {@render children()}
</section>

<style>
  .auth {
    max-width: 440px; margin: 40px auto 0; padding: 28px;
    background: var(--card); border: 1px solid var(--line); border-radius: 18px;
    box-shadow: 0 24px 60px -38px rgb(0 0 0 / 0.35);
  }
  .mark {
    display: grid; place-items: center; width: 44px; height: 44px; border-radius: 999px;
    background: var(--tint); color: var(--accent);
  }
  .mark svg { width: 22px; height: 22px; }
  h2 { margin: 16px 0 0; font: 600 1.7rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 40ch; }
  @media (max-width: 560px) {
    .auth { margin-top: 24px; padding: 22px; border-radius: 14px; }
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
