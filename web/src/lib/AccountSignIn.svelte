<script lang="ts">
  import { tick } from 'svelte'
  import { signInAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { onsuccess }: { onsuccess: (user: User) => Promise<void> | void } = $props()

  let email = $state('')
  let password = $state('')
  let error = $state('')
  let busy = $state(false)
  let emailInput = $state<HTMLInputElement>()

  $effect(() => {
    emailInput?.focus()
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!email.trim() || !password) {
      error = 'Enter your email and password.'
      return
    }
    busy = true
    error = ''
    try {
      const user = await signInAccount(email.trim(), password)
      await onsuccess(user)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      await tick()
      emailInput?.focus()
      emailInput?.select()
    } finally {
      busy = false
    }
  }
</script>

<AuthCard title="Sign in to your shelf" lede="Welcome back. Pick up where you left off.">
  <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? "signin-error" : undefined}>
    <label for="signin-email">Email</label>
    <input id="signin-email" bind:this={emailInput} bind:value={email} type="email" required autocomplete="username" placeholder="you@example.com" />
    <label for="signin-password">Password</label>
    <PasswordInput id="signin-password" bind:value={password} autocomplete="current-password" placeholder="Your password" />
    {#if error}<p class="error" id="signin-error" role="alert">{error}</p>{/if}
    <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
  <p class="help">Need an account or a password reset? Ask your instance admin for an invitation.</p>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .help { margin: 20px 0 0; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.8rem; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
</style>
