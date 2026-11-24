<script lang="ts">
  import { UnauthorizedError, createEnrolCode } from './api'
  import { isSignedOut } from './contentReview'
  import { enrolCodeProblem, enrolCountdown } from './enrolCode'
  import CopyButton from './CopyButton.svelte'

  // The key step of connecting a machine: one button makes a code that works once, for one machine. Shared by the Add a machine
  // dialog and the step-by-step panel. The code lives in this component only: not in the URL, browser storage or a log, and it
  // is gone when the component is. `connected` is true once the machine has reported in, which takes the code off the page.
  let { connected = false, issued = $bindable(), spent = $bindable(''), onsignedout }: {
    connected?: boolean
    /** Held by the parent of the steps, so the code stays while they come and go; never beyond the parent. */
    issued?: { code: string; expiresAt: number }
    spent?: '' | 'lapsed' | 'used'
    onsignedout?: (e: unknown) => void
  } = $props()

  let making = $state(false), codeError = $state('')
  let tick = $state(Date.now())
  let gone = false
  const countdown = $derived(issued ? enrolCountdown(issued.expiresAt, tick) : undefined)

  async function makeCode() {
    if (making) return
    making = true; codeError = ''
    try {
      const made = await createEnrolCode()
      if (gone) return
      issued = made; spent = ''
    } catch (e) {
      if (gone) return
      codeError = enrolCodeProblem(e)
      if (isSignedOut(e) || e instanceof UnauthorizedError) onsignedout?.(e)
    } finally { making = false }
  }
  $effect(() => () => { gone = true })
  $effect(() => {
    if (!issued) return
    tick = Date.now()
    const timer = setInterval(() => (tick = Date.now()), 1000)
    return () => clearInterval(timer)
  })
  $effect(() => { if (countdown?.expired) { issued = undefined; spent = 'lapsed' } })
  $effect(() => { if (issued && connected) { issued = undefined; spent = 'used' } })
</script>

{#if issued}
  <div class="fresh">
    <p>Paste this when the machine asks for an enrolment code. It works once, for one machine. Copy it now; it is not shown again.</p>
    <div class="secret"><code>{issued.code}</code><CopyButton text={issued.code} label="Copy enrolment code" /></div>
    <p class="left">{countdown?.text}</p>
  </div>
{:else}
  <p class="hint" role="status">{#if spent === ''}Make the code when the machine asks for it. It lasts 15 minutes and works once.{:else if spent === 'lapsed'}This code has run out.{:else}The machine has connected, so its code is no longer shown.{/if}</p>
  {#if spent !== 'used'}<button type="button" class="btn" disabled={making} aria-busy={making} onclick={makeCode}>{making ? 'Making…' : spent === 'lapsed' ? 'Make another code' : 'Make an enrolment code'}</button>{/if}
{/if}
{#if codeError}<p class="reason" role="alert">{codeError}</p>{/if}

<style>
  .hint { margin: 6px 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .reason { margin: 8px 0 0; color: var(--bad); font-size: 0.875rem; line-height: 1.5; }
  .fresh { padding: 12px; border: 1px solid var(--accent); border-radius: 8px; background: var(--tint); }
  .fresh p { margin: 0 0 8px; font-size: 0.875rem; }
  .fresh .left { margin: 8px 0 0; color: var(--muted); font-variant-numeric: tabular-nums; }
  .secret { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .secret code { min-width: 0; overflow-wrap: anywhere; font: 0.95rem/1.5 var(--mono); user-select: all; }
</style>
