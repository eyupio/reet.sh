<script lang="ts">
  import { tick } from 'svelte'
  import { signInAccount, type User } from './api'
  import AuthCard from './AuthCard.svelte'

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

<AuthCard title="Sign in to your shelf" lede="Use your reet.sh account to open the shelf.">
  <form onsubmit={submit}>
    <label for="signin-email">Email</label>
    <input id="signin-email" bind:this={emailInput} bind:value={email} type="email" autocomplete="username" placeholder="you@example.com" />
    <label for="signin-password">Password</label>
    <input id="signin-password" bind:value={password} type="password" autocomplete="current-password" placeholder="Your password" />
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button type="submit" class="btn primary submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
  </form>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
</style>
