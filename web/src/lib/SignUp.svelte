<script lang="ts">
  import { tick } from 'svelte'
  import { signUpAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { onsuccess }: { onsuccess: (user: User) => Promise<void> | void } = $props()

  let name = $state('')
  let email = $state('')
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)
  let nameInput = $state<HTMLInputElement>()

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!email.trim()) {
      error = 'Enter your email address.'
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
    busy = true
    error = ''
    try {
      const user = await signUpAccount({ name: name.trim(), email: email.trim(), password })
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

<AuthCard
  eyebrow="Your shelf for AI"
  title="Create your account"
  lede="A shelf for the AI skills, agents and setups you and your team use. Save them, test them, share them, and keep every machine in sync."
>
  <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? 'signup-error' : undefined}>
    <label for="signup-name">Your name</label>
    <input id="signup-name" bind:this={nameInput} bind:value={name} autocomplete="name" placeholder="Sam" />
    <label for="signup-email">Email</label>
    <input id="signup-email" bind:value={email} type="email" required autocomplete="username" placeholder="you@example.com" />
    <label for="signup-password">Password</label>
    <PasswordInput id="signup-password" bind:value={password} autocomplete="new-password" placeholder="At least 8 characters" minlength={8} />
    <label for="signup-confirm">Confirm password</label>
    <PasswordInput id="signup-confirm" bind:value={confirm} autocomplete="new-password" placeholder="Repeat the password" minlength={8} />
    {#if error}<p class="error" id="signup-error" role="alert">{error}</p>{/if}
    <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</button>
  </form>
  <p class="help">Already have an account? <a href="/signin">Sign in</a>.</p>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
  .help { margin: 20px 0 0; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.82rem; }
  .help a { color: var(--accent); font-weight: 600; }
</style>
