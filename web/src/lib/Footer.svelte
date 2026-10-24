<script lang="ts">
  import { onMount } from 'svelte'
  import { getBuildInfo } from './api'
  import { DEVELOPER_NAME, DEVELOPER_URL } from './links'

  let {
    signedIn = false, showAdmin = false, userName = '', onsignout,
  }: { signedIn?: boolean; showAdmin?: boolean; userName?: string; onsignout?: () => void } = $props()

  let build = $state<{ version: string; commit: string } | null>(null)

  onMount(async () => {
    try {
      build = await getBuildInfo()
    } catch {
      /* the footer stays quiet when the build info cannot be read */
    }
  })
</script>

<footer>
  <div class="inner">
    <span class="brand">
      <span class="name">reet.sh</span>
      {#if build}
        <span class="version" title={build.commit !== 'unknown' ? `commit ${build.commit}` : undefined}>{build.version}</span>
      {/if}
      <span class="descriptor">A reet good home for your AI fleet</span>
      <span class="meaning">reet: Yorkshire for “right”.</span>
    </span>
    <span class="end">
      {#if signedIn && userName}<a class="user" href="/account">{userName}</a>{/if}
      {#if showAdmin}<a class="admin" href="/admin">Admin</a>{/if}
      {#if signedIn}<button type="button" class="signout" onclick={() => onsignout?.()}>Sign out</button>{/if}
      <!-- The credit leaves the app, so it opens in a new tab and the list the person was on is not lost. -->
      <span class="credit">
        Developed by
        <a href={DEVELOPER_URL} target="_blank" rel="noopener noreferrer">{DEVELOPER_NAME}<span class="vh"> (opens in a new tab)</span></a>
      </span>
    </span>
  </div>
</footer>

<style>
  footer { color: var(--muted); font-size: 0.85rem; padding: 0 40px; }
  .inner {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 6px 16px;
    max-width: calc(var(--content-width) - 80px); margin: 0 auto; padding: 22px 0 32px; border-top: 1px solid var(--line);
  }
  .brand { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
  .name { font-weight: 600; color: var(--ink); }
  .version { color: var(--muted); font-size: 0.72rem; letter-spacing: 0.04em; font-variant-numeric: tabular-nums; }
  .meaning { font-size: 0.75rem; }
  .descriptor { letter-spacing: 0.06em; text-transform: uppercase; font-size: 0.75rem; }
  .end { display: inline-flex; align-items: center; gap: 16px; flex-wrap: wrap; }
  .credit { white-space: nowrap; }
  .credit a { color: var(--muted); font-weight: 500; text-underline-offset: 3px; }
  .credit a:hover, .credit a:focus-visible { color: var(--ink); text-decoration: underline; }
  .signout, .admin {
    background: none; border: 0; padding: 4px 0; min-height: 32px; cursor: pointer;
    color: var(--muted); font-size: 0.85rem; text-underline-offset: 3px; text-decoration: none;
  }
  .admin { display: inline-flex; align-items: center; }
  .user { color: var(--muted); font-size: 0.85rem; max-width: 180px; overflow-wrap: anywhere; text-decoration: none; }
  .user:hover, .user:focus-visible { color: var(--ink); text-decoration: underline; }
  .signout:hover, .signout:focus-visible, .admin:hover, .admin:focus-visible { color: var(--ink); text-decoration: underline; }
  @media (pointer: coarse) { .signout, .admin { min-height: 44px; } }
  @media (max-width: 560px) { footer { padding: 0 16px; } .descriptor { display: none; } }
</style>
