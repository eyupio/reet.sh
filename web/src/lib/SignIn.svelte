<script lang="ts">
  import { tick } from 'svelte'
  import AuthCard from './AuthCard.svelte'
  import { setToken } from './api'

  let { onattempt }: { onattempt: () => Promise<string | null> } = $props()

  let value = $state('')
  let error = $state('')
  let busy = $state(false)
  let field = $state<HTMLInputElement>()

  $effect(() => {
    field?.focus()
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const token = value.trim()
    if (!token) {
      error = 'Paste the token from your .env file.'
      field?.focus()
      return
    }
    busy = true
    error = ''
    setToken(token)
    try {
      const problem = await onattempt()
      if (problem) {
        error = problem
        await tick()
        field?.focus()
        field?.select()
      }
    } catch {
      error = 'Could not reach the shelf. Check that the server is running.'
    } finally {
      busy = false
    }
  }
</script>

<AuthCard title="Open your shelf" lede="This instance uses an access token. Paste yours below to continue." eyebrow="Your private shelf">
  <form onsubmit={submit} aria-busy={busy}>
    <input class="vh" type="text" name="username" autocomplete="username" value="reet" tabindex="-1" aria-hidden="true" />
    <label for="access-token">Access token</label>
    <input
      id="access-token"
      bind:this={field}
      bind:value={value}
      oninput={() => (error = '')}
      required
      type="password"
      name="token"
      autocomplete="current-password"
      autocapitalize="off"
      autocorrect="off"
      spellcheck="false"
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? 'signin-error' : undefined}
      placeholder="Paste your token"
    />
    {#if error}<p class="error" id="signin-error" role="alert">{error}</p>{/if}
    <button class="btn primary submit" type="submit" disabled={busy}>{busy ? 'Checking…' : 'Open the shelf'}</button>
  </form>

  <details class="help">
    <summary>Where do I find this?</summary>
    <p>On the machine that runs reet.sh, the token is in the same folder as the compose file:</p>
    <code>grep REET_TOKEN .env</code>
  </details>
</AuthCard>

<style>
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input:not(.vh) { width: 100%; }
  .submit { width: 100%; margin-top: 4px; }
  .error { margin: 0; color: var(--bad); font-size: 0.9rem; }
  .help { margin-top: 22px; padding-top: 16px; border-top: 1px solid var(--line); }
  .help summary { cursor: pointer; color: var(--muted); font-size: 0.9rem; width: fit-content; }
  .help summary:hover { color: var(--ink); }
  .help p { margin: 10px 0 6px; color: var(--muted); font-size: 0.9rem; }
  .help code {
    display: block; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px;
    background: var(--bg); font-size: 0.85rem; overflow-wrap: anywhere;
  }
</style>
