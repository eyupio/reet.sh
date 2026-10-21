<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { createToken, listTokens, revokeToken, type APIToken, type User } from './api'
  import CopyButton from './CopyButton.svelte'

  let { user, onsignout, onerror }: {
    user: User
    onsignout: () => void
    onerror?: (e: unknown) => void
  } = $props()

  let tokens = $state<APIToken[]>([])
  let tokenName = $state('scan token')
  let freshToken = $state('')
  let error = $state('')
  let busy = $state(false)

  const canTokens = $derived(user.role !== 'viewer')

  async function load() {
    if (!canTokens) return
    try {
      tokens = (await listTokens()).tokens
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    }
  }

  onMount(() => {
    void load()
  })

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    freshToken = ''
    error = ''
    try {
      const r = await createToken(tokenName.trim() || 'scan token')
      freshToken = r.token
      tokenName = 'scan token'
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = false
    }
  }

  async function revoke(id: number) {
    try {
      await revokeToken(id)
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    }
  }

  function when(ms?: number): string {
    return ms ? new Date(ms).toLocaleString() : 'never'
  }
</script>

<section class="account" aria-labelledby="account-title">
  <PageHeader eyebrow="Your account" title={user.name || user.email} titleId="account-title" lede={`${user.email} · ${user.role}`} />

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}

  {#if canTokens}
    <section class="block" aria-labelledby="tokens-title">
      <h2 id="tokens-title">Personal API tokens</h2>
      <p class="muted">Use a token with <code>reet scan --token …</code> or an API client. A token is shown once, then only its name remains.</p>

      {#if freshToken}
        <div class="fresh" role="status">
          <p>Copy this token now. It will not be shown again.</p>
          <div class="tokenline">
            <code>{freshToken}</code>
            <CopyButton text={freshToken} />
          </div>
        </div>
      {/if}

      {#if tokens.length}
        <ul class="tokens">
          {#each tokens as token (token.id)}
            <li>
              <span class="tname">{token.name}</span>
              <span class="tmeta">created {when(token.createdAt)} · used {when(token.lastUsedAt)}</span>
              <button type="button" class="btn quiet danger" onclick={() => revoke(token.id)}>Revoke</button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted">No API tokens yet.</p>
      {/if}

      <form onsubmit={create}>
        <label for="token-name">New token name</label>
        <div class="createrow">
          <input id="token-name" bind:value={tokenName} placeholder="scan token" autocomplete="off" />
          <button type="submit" class="btn primary" disabled={busy}>{busy ? 'Creating…' : 'Create token'}</button>
        </div>
      </form>
    </section>
  {:else}
    <p class="muted">Viewer accounts cannot create API tokens.</p>
  {/if}

  <section class="block">
    <h2>Session</h2>
    <p class="muted">Sign out of your account on this browser.</p>
    <button type="button" class="btn" onclick={onsignout}>Sign out</button>
  </section>
</section>

<style>
  .account { padding-top: 4px; }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  h2 { margin: 0 0 10px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.025em; }
  .muted { margin: 8px 0 0; color: var(--muted); }
  .fresh { margin-top: 14px; padding: 12px; border: 1px solid var(--accent); border-radius: 10px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.88rem; }
  .tokenline { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
  .tokenline code { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: 0.82rem; }
  .tokens { list-style: none; margin: 14px 0 0; padding: 0; }
  .tokens li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 0; border-bottom: 1px solid var(--line); }
  .tname { font-weight: 600; }
  .tmeta { flex: 1; min-width: 0; color: var(--muted); font-size: 0.8rem; }
  form { display: grid; gap: 6px; margin-top: 18px; max-width: 460px; }
  label { font-size: 0.9rem; font-weight: 600; }
  .createrow { display: flex; gap: 8px; flex-wrap: wrap; }
  .createrow input { flex: 1; min-width: 0; }
  @media (max-width: 560px) {
    .createrow { flex-direction: column; }
  }
</style>
