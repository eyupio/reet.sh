<script lang="ts">
  import { UnauthorizedError, createEnrolCode } from './api'
  import { isSignedOut } from './contentReview'
  import { enrolCodeProblem, enrolCountdown } from './enrolCode'
  import CopyButton from './CopyButton.svelte'

  // The key step of connecting a machine: one button makes a code that works once, for one machine. Shared by the Add a machine
  // dialog and the step-by-step panel. The code lives in this component only: not in the URL, browser storage or a log, and it
  // is gone when the component is. `connected` is true once the machine has reported in, which takes the code off the page.
  let { connected = false, onsignedout }: { connected?: boolean; onsignedout?: (e: unknown) => void } = $props()

  let issued = $state<{ code: string; expiresAt: number } | undefined>()
  let making = $state(false), codeError = $state(''), spent = $state<'' | 'lapsed' | 'used'>('')
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
      if (isSignedOut(e) || e instanceof UnauthorizedError) onsignedout?.(e)
      else codeError = enrolCodeProblem(e)
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
  <p class="hint" role="status">{#if spent === ''}The machine asks for an enrolment code when it connects. It works once, for one machine.{:else if spent === 'lapsed'}This code has run out.{:else}The machine has connected, so its code is no longer shown.{/if}</p>
  <button type="button" class="btn" disabled={making} aria-busy={making} onclick={makeCode}>{making ? 'Making…' : spent === 'lapsed' ? 'Make another code' : 'Make an enrolment code'}</button>
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
