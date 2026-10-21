<script lang="ts">
  import { tick } from 'svelte'
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

<section class="signin" aria-labelledby="signin-title">
  <span class="mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="8" cy="15" r="4"></circle>
      <path d="m10.8 12.2 8.4-8.4M16 7l3 3M13 10l3 3"></path>
    </svg>
  </span>

  <h2 id="signin-title">Sign in to your shelf</h2>
  <p class="lede">This instance is protected. Paste the access token from its <code>.env</code> file.</p>

  <form onsubmit={submit}>
    <input class="vh" type="text" name="username" autocomplete="username" value="reet" tabindex="-1" aria-hidden="true" />
    <label for="access-token">Access token</label>
    <input
      id="access-token"
      bind:this={field}
      bind:value={value}
      oninput={() => (error = '')}
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
</section>

<style>
  .signin {
    max-width: 420px; margin: 40px auto 0; padding: 28px;
    background: var(--card); border: 1px solid var(--line); border-radius: 18px;
    box-shadow: 0 24px 60px -38px rgb(0 0 0 / 0.35);
  }
  .mark {
    display: grid; place-items: center; width: 44px; height: 44px; border-radius: 999px;
    background: var(--tint); color: var(--accent);
  }
  .mark svg { width: 22px; height: 22px; }
  h2 { margin: 16px 0 0; font: 600 1.7rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 36ch; }
  .lede code { font-size: 0.9em; color: var(--ink); }
  form { display: grid; gap: 8px; margin-top: 22px; }
  label { font-weight: 600; font-size: 0.9rem; }
  input { width: 100%; }
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
  @media (max-width: 560px) {
    .signin { margin-top: 24px; padding: 22px; border-radius: 14px; }
    h2 { font-size: 1.5rem; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .signin { animation: rise 320ms var(--ease-out) both; }
    .help summary { transition: color 0.15s; }
    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  }
</style>
