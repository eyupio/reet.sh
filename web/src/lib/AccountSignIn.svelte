<script lang="ts">
  import { tick } from 'svelte'
  import { forgotPassword, signInAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'
  import { canAutoFocus } from './focus'
  import PasswordInput from './PasswordInput.svelte'
  import { signInChoices, ssoErrorFromSearch, ssoErrorMessage, ssoNext, ssoStartHref, withoutSsoError, type SsoStatus } from './sso'

  let { onsuccess, signupEnabled = false, contactEmail = '', passwordReset = false, sso }: {
    onsuccess: (user: User) => Promise<void> | void
    signupEnabled?: boolean
    contactEmail?: string
    passwordReset?: boolean
    sso?: SsoStatus
  } = $props()

  const choices = $derived(signInChoices(sso))
  const startHref = $derived(ssoStartHref(ssoNext(location.pathname, location.search)))
  // A sign-in that could not finish comes back with a code in the address: say it in words once, and take it off the
  // address so a reload does not say it again.
  const ssoError = ssoErrorMessage(ssoErrorFromSearch(location.search))
  if (ssoError) history.replaceState(null, '', withoutSsoError(location.pathname + location.search + location.hash))

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
  let resetEmail = $state('')
  let resetNotice = $state('')
  let resetError = $state('')
  let resetBusy = $state(false)

  $effect(() => {
    if (canAutoFocus()) emailInput?.focus()
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

  async function forgot(e: SubmitEvent) {
    e.preventDefault()
    if (!resetEmail.trim()) {
      resetError = 'Enter the email address on the account.'
      return
    }
    resetBusy = true
    resetError = ''
    resetNotice = ''
    try {
      const r = await forgotPassword(resetEmail.trim())
      resetNotice = r.message
    } catch (e) {
      resetError = e instanceof Error ? e.message : String(e)
    } finally {
      resetBusy = false
    }
  }
</script>

{#snippet passwordForm()}
  <form onsubmit={submit} aria-busy={busy} aria-describedby={error ? "signin-error" : undefined}>
    <label for="signin-email">Email</label>
    <input id="signin-email" bind:this={emailInput} bind:value={email} type="email" required autocomplete="username" placeholder="you@example.com" />
    <label for="signin-password">Password</label>
    <PasswordInput id="signin-password" bind:value={password} autocomplete="current-password" placeholder="Your password" />
    {#if error}<p class="error" id="signin-error" role="alert">{error}</p>{/if}
    <button type="submit" class={choices.button ? 'btn submit' : 'btn primary submit'} disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
{/snippet}

<AuthCard title="Sign in to your workspace" lede="Your machines, with their agents, skills and setups in sync, and the sessions you left running.">
  {#if ssoError}<p class="error sso-error" role="alert">{ssoError}</p>{/if}
  {#if choices.button}
    <a class="btn primary sso" href={startHref}>{choices.label}</a>
  {/if}
  {#if choices.button && choices.passwordForm === 'open'}
    <p class="or"><span>or sign in with a password</span></p>
  {/if}
  {#if choices.passwordForm === 'folded'}
    <details class="fold">
      <summary>Sign in with a password</summary>
      <p class="muted">Only the owner can sign in with a password here.</p>
      {@render passwordForm()}
    </details>
  {:else}
    {@render passwordForm()}
  {/if}
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
      {#if passwordReset}
        <form onsubmit={forgot} aria-busy={resetBusy}>
          <label for="forgot-email">Email</label>
          <input id="forgot-email" bind:value={resetEmail} type="email" autocomplete="email" placeholder="you@example.com" required />
          <button type="submit" class="btn" disabled={resetBusy}>{resetBusy ? 'Sending…' : 'Email a reset link'}</button>
        </form>
        {#if resetNotice}<p class="reset-note" role="status">{resetNotice}</p>{/if}
        {#if resetError}<p class="error" role="alert">{resetError}</p>{/if}
      {:else}
        <p>Ask your instance admin; they can issue a temporary password from Admin → Users.{#if resetHref} Or <a href={resetHref}>email {contactEmail}</a>.{/if}</p>
      {/if}
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
  .help a { color: var(--accent-text); font-weight: 600; }
  .forgot { margin-top: 8px; }
  .forgot summary { min-height: 32px; display: inline-flex; align-items: center; color: var(--muted); cursor: pointer; font-weight: 600; }
  .forgot summary:hover { color: var(--ink); }
  .forgot p { margin: 6px 0 0; line-height: 1.5; }
  .forgot form { display: grid; gap: 8px; margin-top: 8px; }
  .forgot form label { font-weight: 600; font-size: 0.82rem; }
  .reset-note { color: var(--good); }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
  .sso-error { margin-top: 18px; }
  .sso { display: flex; width: 100%; min-height: 44px; margin-top: 22px; align-items: center; justify-content: center; text-align: center; box-sizing: border-box; }
  .or { display: flex; align-items: center; gap: 12px; margin: 18px 0 0; color: var(--muted); font-size: 0.8rem; }
  .or::before, .or::after { content: ''; flex: 1; border-top: 1px solid var(--line); }
  .fold { margin-top: 14px; }
  .fold summary { min-height: 40px; display: flex; align-items: center; color: var(--muted); cursor: pointer; font-weight: 600; }
  .fold summary:hover { color: var(--ink); }
  .fold > p { margin: 4px 0 0; color: var(--muted); font-size: 0.8rem; }
  .fold form { margin-top: 12px; }
</style>
