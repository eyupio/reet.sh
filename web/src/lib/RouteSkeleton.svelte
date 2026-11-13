<script lang="ts">
  // The shape a page holds while its code downloads. Every signed-in page uses the same two faces the static boot shell
  // already paints: the ledger page, or the terminal workspace. A slow chunk therefore keeps the layout it started with
  // instead of flashing a sequence of different skeletons.
  import { onMount } from 'svelte'
  import PageSkeleton from './PageSkeleton.svelte'
  import WorkspaceSkeleton from './terminal/WorkspaceSkeleton.svelte'

  let { variant = 'rows' }: { variant?: 'rows' | 'cards' | 'auth' | 'form' | 'welcome' | 'home' | 'workspace' | 'content' } = $props()
  let slow = $state(false)
  const query = new URLSearchParams(location.search)
  const context = query.get('theatre') !== '1' && !!(query.get('session') || query.get('agent'))
  onMount(() => {
    const timer = setTimeout(() => (slow = true), 10000)
    return () => clearTimeout(timer)
  })
</script>

{#if variant === 'workspace'}
  <div class="route-skeleton workspace" data-loading-page="workspace">
    <WorkspaceSkeleton {context} />
  </div>
{:else}
  <div class="route-skeleton {variant}" data-loading-page={variant} role="status" aria-busy="true" aria-label="Loading page">
    <span class="vh">Loading page…</span>
    {#if variant === 'auth'}
      <div class="structure" aria-hidden="true">
        <div class="auth-card"><i class="eyebrow"></i><i class="title"></i><i></i><div class="field"></div><div class="field"></div><div class="button"></div></div>
      </div>
    {:else if variant === 'home'}
      <div class="structure" aria-hidden="true">
        <div class="workspace-content">
          <div class="head"><i class="eyebrow"></i><i class="title"></i></div>
          <div class="columns">{#each [1, 2, 3] as column (column)}<div class="column"><i class="eyebrow"></i><div class="tile"><i></i><i class="short"></i></div><div class="tile"><i></i><i class="short"></i></div></div>{/each}</div>
        </div>
      </div>
    {:else if variant === 'welcome'}
      <div class="structure" aria-hidden="true">
        <div class="hero"><div class="copy"><i class="eyebrow"></i><i class="hero-title"></i><i class="hero-title short"></i><i></i><i></i><i class="short"></i><div class="button"></div></div><div class="preview"><div class="field"></div><div class="field"></div><div class="terminal"></div></div></div>
      </div>
    {:else}
      <PageSkeleton />
    {/if}
  </div>
{/if}
{#if slow}<p class="load-slow" role="status">Taking longer than usual. <a href={location.href}>Reload page</a> to try again.</p>{/if}

<style>
  .route-skeleton { min-width: 0; min-height: 400px; }
  .route-skeleton.workspace { min-height: 0; height: 100%; display: flex; flex-direction: column; }
  .structure { display: grid; gap: 24px; }
  i { display: block; height: 12px; border-radius: 5px; background: var(--line); }
  .eyebrow { width: 110px; height: 10px; }
  .title { width: min(340px, 70%); height: 44px; border-radius: 8px; }
  .short { width: 60%; }
  .head { display: grid; gap: 14px; padding-bottom: 4px; }
  .field { height: 42px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .button { width: 120px; height: 40px; background: var(--line); border-radius: 8px; }
  .auth-card, .preview { display: grid; align-content: start; gap: 22px; padding: 28px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .auth-card { width: min(430px, 100%); margin: 32px auto; }
  .hero { display: grid; grid-template-columns: 1fr 1.05fr; align-items: center; gap: 48px; padding: 20px 0 80px; }
  .copy { display: grid; gap: 18px; }
  .hero-title { height: clamp(34px, 4.4vw, 52px); width: 94%; }
  .hero-title.short { width: 68%; }
  /* Home draws its project rail as the page's left column, so the skeleton keeps that column's room and content starts
     where the real board will. The boot script leaves --boot-home-rail for exactly this handover. */
  .home { height: 100%; min-height: calc(100dvh - 100px); padding-left: var(--boot-home-rail, 0px); box-sizing: border-box; }
  .home .structure { grid-template-columns: minmax(0, 1fr); min-height: inherit; gap: 0; }
  .home .workspace-content { padding: 20px 24px; }
  .workspace-content { padding: 24px; display: flex; flex-direction: column; gap: 24px; min-width: 0; }
  .columns { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
  .column { display: grid; align-content: start; gap: 16px; }
  .tile { display: grid; gap: 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-height: 145px; align-content: start; }
  .load-slow { position: fixed; bottom: 80px; inset-inline: 16px; margin: auto; width: fit-content; max-width: calc(100% - 32px); padding: 12px 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); z-index: 65; font-size: 0.875rem; }
  @media (prefers-reduced-motion: no-preference) {
    i, .button { background: linear-gradient(90deg, var(--line), var(--line-soft), var(--line)); background-size: 200% 100%; animation: route-shimmer 1.8s ease-in-out infinite; }
  }
  @keyframes route-shimmer { to { background-position: -200% 0; } }
  @media (max-width: 850px) { .hero { grid-template-columns: 1fr; gap: 30px; } .columns { grid-template-columns: 1fr; } .column:not(:first-child) { display: none; } }
  @media (max-width: 650px) { .workspace-content { padding: 16px; } .home .workspace-content { padding: 14px 16px; } .title { height: 32px; } .auth-card, .preview { padding: 22px; } }
</style>
