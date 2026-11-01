<script lang="ts">
  import { onMount } from 'svelte'
  import { getInvite, joinAccount, type Invite, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { token, onsuccess }: { token: string; onsuccess: (user: User) => Promise<void> | void } = $props()

  let invite = $state<Invite | null>(null)
  let loading = $state(true)
  let name = $state('')
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)

  onMount(async () => {
    if (!token) {
      error = 'This invite link is missing its token.'
      loading = false
      return
    }
    try {
      invite = await getInvite(token)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (password.length < 8) {
      error = 'Use at least 8 characters for the password.'
      return
    }
    if (password !== confirm) {
      error = 'The passwords do not match.'
      return
    }
    busy = true
    error = ''
    try {
      const user = await joinAccount({ token, name: name.trim(), password })
      await onsuccess(user)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<AuthCard eyebrow="Better together" title="Join reet.sh" lede={invite ? `You have been invited as ${invite.email}. Your team's agents, skills and setups are one sign-in away.` : 'Checking this invitation…'}>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if !invite}
    <p class="error" id="join-error" role="alert">{error || 'This invite is not valid any more.'}</p>
    <p class="muted"><a href="/">Go to your workspace</a></p>
  {:else}
    <p class="role">Role: <strong>{invite.role}</strong></p>
    <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? "join-error" : undefined}>
      <label for="join-name">Your name</label>
      <input id="join-name" bind:value={name} autocomplete="name" placeholder="Sam" />
      <label for="join-password">Password</label>
      <PasswordInput id="join-password" bind:value={password} autocomplete="new-password" placeholder="At least 8 characters" minlength={8} />
      <label for="join-confirm">Confirm password</label>
      <PasswordInput id="join-confirm" bind:value={confirm} autocomplete="new-password" placeholder="Repeat the password" minlength={8} />
      {#if error}<p class="error" id="join-error" role="alert">{error}</p>{/if}
      <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Joining…' : 'Join the workspace'}</button>
    </form>
  {/if}
</AuthCard>

<style>
  .muted { margin: 16px 0 0; color: var(--muted); }
  .muted a { color: var(--accent-text); }
  .role { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  form { display: grid; gap: 8px; margin-top: 18px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
</style>
