<script lang="ts">
  import { tick } from 'svelte'
  import { setupAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import { canAutoFocus } from './focus'
  import PasswordInput from './PasswordInput.svelte'

  let { bootstrapRequired = false, onsuccess }: {
    bootstrapRequired?: boolean
    onsuccess: (user: User) => Promise<void> | void
  } = $props()

  let name = $state('')
  let email = $state('')
  let password = $state('')
  let confirm = $state('')
  let bootstrap = $state('')
  let error = $state('')
  let busy = $state(false)
  let nameInput = $state<HTMLInputElement>()

  $effect(() => {
    if (canAutoFocus()) nameInput?.focus()
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!email.trim()) {
      error = 'Enter an email address for the owner.'
      return
    }
    if (password.length < 8) {
      error = 'Use at least 8 characters for the password.'
      return
    }
    if (password !== confirm) {
      error = 'The passwords do not match.'
      return
    }
    if (bootstrapRequired && !bootstrap.trim()) {
      error = 'Paste the access token from the .env file.'
      return
    }
    busy = true
    error = ''
    try {
      const user = await setupAccount({ name: name.trim(), email: email.trim(), password }, bootstrap.trim())
      await onsuccess(user)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      await tick()
      nameInput?.focus()
    } finally {
      busy = false
    }
  }
</script>

<AuthCard eyebrow="Ey up. Start here." title="Create your account" lede="You're setting up this reet.sh instance and will be its owner. Enrol your first machine, enable terminal access with local consent, and open your workspace. Everything stays on this server: no telemetry, one SQLite file.">
  <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? "setup-error" : undefined}>
    <label for="setup-name">Your name</label>
    <input id="setup-name" bind:this={nameInput} bind:value={name} autocomplete="name" placeholder="Sam" />
    <label for="setup-email">Email</label>
    <input id="setup-email" bind:value={email} type="email" required autocomplete="username" placeholder="you@example.com" />
    <label for="setup-password">Password</label>
    <PasswordInput id="setup-password" bind:value={password} autocomplete="new-password" placeholder="At least 8 characters" minlength={8} />
    <label for="setup-confirm">Confirm password</label>
    <PasswordInput id="setup-confirm" bind:value={confirm} autocomplete="new-password" placeholder="Repeat the password" minlength={8} />
    {#if bootstrapRequired}
      <label for="setup-token">Access token</label>
      <input id="setup-token" bind:value={bootstrap} type="password" autocomplete="off" spellcheck="false" placeholder="From your .env file" />
      <p class="hint">This is the bootstrap credential. Find it on the server: <code>grep REET_TOKEN .env</code></p>
    {/if}
    {#if error}<p class="error" id="setup-error" role="alert">{error}</p>{/if}
    <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</button>
  </form>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
  .hint { margin: 0; color: var(--muted); font-size: 0.82rem; }
  .hint code { font-size: 0.85em; color: var(--ink); overflow-wrap: anywhere; }
</style>
