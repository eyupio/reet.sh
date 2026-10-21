<script lang="ts">
  import { onMount } from 'svelte'
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
  <a class="back" href="/">← Back to the shelf</a>
  <p class="eyebrow">Account</p>
  <h2 id="account-title">{user.name}</h2>
  <p class="who">{user.email} · {user.role}</p>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}

  {#if canTokens}
    <section class="block" aria-labelledby="tokens-title">
      <h3 id="tokens-title">API tokens</h3>
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
    <button type="button" class="btn" onclick={onsignout}>Sign out</button>
  </section>
</section>

<style>
  .account { padding-top: 4px; }
  .back { color: var(--muted); font-size: 0.9rem; text-decoration: none; }
  .back:hover { color: var(--ink); text-decoration: underline; text-underline-offset: 3px; }
  .eyebrow {
    margin: 22px 0 0; color: var(--accent); font-size: 0.72rem; font-weight: 700;
    letter-spacing: 0.12em; text-transform: uppercase;
  }
  h2 { margin: 6px 0 0; font: 600 2rem/1.1 var(--serif); letter-spacing: -0.02em; }
  .who { margin: 6px 0 0; color: var(--muted); }
  .notice { margin: 16px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .block { margin-top: 28px; padding-top: 18px; border-top: 1px solid var(--line); }
  h3 {
    margin: 0; color: var(--muted); font-size: 0.68rem; font-weight: 700;
    letter-spacing: 0.1em; text-transform: uppercase;
  }
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
    h2 { font-size: 1.7rem; }
  }
</style>
