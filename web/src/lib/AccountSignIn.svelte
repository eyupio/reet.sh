<script lang="ts">
  import { tick } from 'svelte'
  import { signInAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import PasswordInput from './PasswordInput.svelte'

  let { onsuccess, signupEnabled = false, contactEmail = '' }: {
    onsuccess: (user: User) => Promise<void> | void
    signupEnabled?: boolean
    contactEmail?: string
  } = $props()

  const inviteHref = $derived(contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent('reet.sh account request')}&body=${encodeURIComponent('Hi, I would like an account on your reet.sh instance.')}`
    : '')
  const resetHref = $derived(contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent('reet.sh password reset')}&body=${encodeURIComponent('Hi, I need a password reset on your reet.sh instance. My email is: ')}`
    : '')

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

<AuthCard title="Sign in to your shelf" lede="Your shelf, lists, agent sync and machine reports are waiting.">
  <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? "signin-error" : undefined}>
    <label for="signin-email">Email</label>
    <input id="signin-email" bind:this={emailInput} bind:value={email} type="email" required autocomplete="username" placeholder="you@example.com" />
    <label for="signin-password">Password</label>
    <PasswordInput id="signin-password" bind:value={password} autocomplete="current-password" placeholder="Your password" />
    {#if error}<p class="error" id="signin-error" role="alert">{error}</p>{/if}
    <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
  <div class="help">
    {#if signupEnabled}
      <p>New here? <a href="/signup">Create an account</a>.</p>
    {:else if inviteHref}
      <p>Need an account? <a href={inviteHref}>Request an invite</a>.</p>
    {:else}
      <p>Need an account? Ask your instance admin for an invitation.</p>
    {/if}
    <details class="forgot">
      <summary>Forgot password?</summary>
      <p>Ask your instance admin; they can issue a temporary password from Admin → Users.{#if resetHref} Or <a href={resetHref}>email {contactEmail}</a>.{/if}</p>
    </details>
  </div>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .help { margin: 20px 0 0; padding-top: 16px; border-top: 1px solid var(--line); color: var(--muted); font-size: 0.8rem; }
  .help p { margin: 0; }
  .help a { color: var(--accent); font-weight: 600; }
  .forgot { margin-top: 8px; }
  .forgot summary { min-height: 32px; display: inline-flex; align-items: center; color: var(--muted); cursor: pointer; font-weight: 600; }
  .forgot summary:hover { color: var(--ink); }
  .forgot p { margin: 6px 0 0; line-height: 1.5; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
</style>
