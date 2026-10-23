<script lang="ts">
  import { resetPassword } from './api'
  import AuthCard from './AuthCard.svelte'
  import PasswordInput from './PasswordInput.svelte'

  const token = new URLSearchParams(location.search).get('token') ?? ''
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let done = $state('')
  let busy = $state(false)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!token) {
      error = 'That reset link is not complete. Ask for a new one.'
      return
    }
    if (password.length < 8) {
      error = 'Use at least 8 characters for the new password.'
      return
    }
    if (password !== confirm) {
      error = 'The new passwords do not match.'
      return
    }
    busy = true
    error = ''
    try {
      const r = await resetPassword(token, password)
      done = r.message || 'Your password changed. Every other session was signed out.'
      password = ''
      confirm = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<svelte:head><title>Choose a new password · reet.sh</title></svelte:head>

<AuthCard title="Choose a new password" lede="A reset link works once. Choosing a new password signs out every other session.">
  {#if done}
    <p class="done" role="status">{done}</p>
    <p class="help"><a href="/signin">Sign in with the new password</a></p>
  {:else}
    <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? 'reset-error' : undefined}>
      <label for="reset-password">New password</label>
      <PasswordInput id="reset-password" bind:value={password} autocomplete="new-password" placeholder="At least 8 characters" minlength={8} />
      <label for="reset-confirm">New password again</label>
      <PasswordInput id="reset-confirm" bind:value={confirm} autocomplete="new-password" placeholder="Repeat the password" minlength={8} />
      {#if error}<p class="error" id="reset-error" role="alert">{error}</p>{/if}
      <button type="submit" class="btn primary submit" disabled={busy || !token}>{busy ? 'Saving…' : 'Set the new password'}</button>
    </form>
    <p class="help">The link has expired or was already used? <a href="/signin">Ask for a new one from the sign-in page</a>.</p>
  {/if}
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
  .done { margin: 22px 0 0; padding: 12px; border: 1px solid var(--good); border-radius: 8px; }
  .help { margin: 20px 0 0; color: var(--muted); font-size: 0.82rem; }
  .help a { color: var(--accent); font-weight: 600; }
</style>
