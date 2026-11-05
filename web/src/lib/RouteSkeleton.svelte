<script lang="ts">
  import { onMount } from 'svelte'
  import Skeleton from './Skeleton.svelte'

  let { variant = 'rows' }: { variant?: 'rows' | 'cards' | 'auth' | 'form' | 'welcome' | 'home' | 'workspace' } = $props()
  let slow = $state(false)
  const railCollapsed = (() => {
    try { return localStorage.getItem('reet.rail.collapsed') === '1' } catch { return false }
  })()
  onMount(() => {
    const timer = setTimeout(() => (slow = true), 10000)
    return () => clearTimeout(timer)
  })
</script>

<div class="route-skeleton {variant}" class:rail-collapsed={railCollapsed} data-loading-page={variant} role="status" aria-busy="true" aria-label="Loading page">
  <span class="vh">Loading page…</span>
  <div class="structure" aria-hidden="true">
    {#if variant === 'auth'}
      <div class="auth-card"><i class="eyebrow"></i><i class="title"></i><i></i><div class="field"></div><div class="field"></div><div class="button"></div></div>
    {:else if variant === 'home' || variant === 'workspace'}
      <div class="workspace-rail"><i class="eyebrow"></i><div class="field"></div><i></i><i></i><i class="short"></i></div>
      <div class="workspace-content">
        <div class="head"><i class="eyebrow"></i><i class="title"></i></div>
        {#if variant === 'workspace'}<div class="terminal"><i></i><i class="short"></i></div>
        {:else}<div class="columns">{#each [1, 2, 3] as column (column)}<div class="column"><i class="eyebrow"></i><div class="tile"><i></i><i class="short"></i></div><div class="tile"><i></i><i class="short"></i></div></div>{/each}</div>{/if}
      </div>
    {:else if variant === 'welcome'}
      <div class="hero"><div class="copy"><i class="eyebrow"></i><i class="hero-title"></i><i class="hero-title short"></i><i></i><i></i><i class="short"></i><div class="button"></div></div><div class="preview"><div class="field"></div><div class="field"></div><div class="terminal"></div></div></div>
    {:else}
      <div class="head"><i class="eyebrow"></i><i class="title"></i><i class="lede"></i></div>
      {#if variant === 'form'}
        <div class="form-card"><i class="eyebrow"></i><div class="field"></div><div class="field"></div><div class="field"></div><div class="button"></div></div>
      {:else}
        <div class="toolbar"><div class="field"></div><div class="button"></div></div>
        <Skeleton shape={variant} count={variant === 'cards' ? 3 : 6} height="180px" />
      {/if}
    {/if}
  </div>
</div>
{#if slow}<p class="load-slow" role="status">Taking longer than usual. <a href={location.href}>Reload page</a> to try again.</p>{/if}

<style>
  .route-skeleton { min-width: 0; min-height: 400px; }
  .structure { display: grid; gap: 24px; }
  i { display: block; height: 12px; border-radius: 5px; background: var(--line); }
  .eyebrow { width: 110px; height: 10px; }
  .title { width: min(340px, 70%); height: 44px; border-radius: 8px; }
  .short { width: 60%; }
  .lede { width: min(560px, 90%); }
  .head { display: grid; gap: 14px; padding-bottom: 4px; }
  .field { height: 42px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .button { width: 120px; height: 40px; background: var(--line); border-radius: 8px; }
  .toolbar { display: flex; justify-content: space-between; gap: 16px; }
  .toolbar .field { width: min(360px, 65%); }
  .auth-card, .form-card, .preview { display: grid; align-content: start; gap: 22px; padding: 28px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .auth-card { width: min(430px, 100%); margin: 32px auto; }
  .form-card { max-width: 720px; }
  .hero { display: grid; grid-template-columns: 1fr 1.05fr; align-items: center; gap: 48px; padding: 20px 0 80px; }
  .copy { display: grid; gap: 18px; }
  .hero-title { height: clamp(34px, 4.4vw, 52px); width: 94%; }
  .hero-title.short { width: 68%; }
  .home, .workspace { height: 100%; min-height: calc(100dvh - 100px); }
  .home .structure, .workspace .structure { grid-template-columns: 220px minmax(0, 1fr); min-height: inherit; gap: 0; }
  .home .structure { grid-template-columns: 301px minmax(0, 1fr); }
  .home.rail-collapsed .structure { grid-template-columns: 56px minmax(0, 1fr); }
  .home .workspace-content { padding: 20px 24px; }
  .workspace-rail { display: grid; align-content: start; gap: 24px; padding: 24px 18px; border-right: 1px solid var(--line); }
  .workspace-content { padding: 24px; display: flex; flex-direction: column; gap: 24px; min-width: 0; }
  .terminal { background: var(--bg-sunk, var(--bg)); border: 1px solid var(--line); border-radius: 10px; padding: 24px; min-height: 250px; flex: 1; }
  .terminal i { margin: 0 0 14px; width: 50%; }
  .columns { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
  .column { display: grid; align-content: start; gap: 16px; }
  .tile { display: grid; gap: 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-height: 145px; align-content: start; }
  .load-slow { position: fixed; bottom: 80px; inset-inline: 16px; margin: auto; width: fit-content; max-width: calc(100% - 32px); padding: 12px 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); z-index: 65; font-size: 0.875rem; }
  @media (prefers-reduced-motion: no-preference) {
    i, .button { background: linear-gradient(90deg, var(--line), var(--line-soft), var(--line)); background-size: 200% 100%; animation: route-shimmer 1.8s ease-in-out infinite; }
  }
  @keyframes route-shimmer { to { background-position: -200% 0; } }
  @media (max-width: 1239px) { .home .structure { grid-template-columns: 56px minmax(0, 1fr); } .home .workspace-rail { overflow: hidden; } }
  @media (max-width: 1099px) { .home .structure, .home.rail-collapsed .structure { grid-template-columns: minmax(0, 1fr); } .home .workspace-rail { display: none; } }
  @media (max-width: 850px) { .hero { grid-template-columns: 1fr; gap: 30px; } .columns { grid-template-columns: 1fr; } .column:not(:first-child) { display: none; } }
  @media (max-width: 650px) { .home .structure, .workspace .structure { grid-template-columns: minmax(0, 1fr); } .workspace-rail { display: none; } .workspace-content { padding: 16px; } .home .workspace-content { padding: 14px 16px; } .title { height: 32px; } .auth-card, .form-card { padding: 22px; } }
</style>
