<script lang="ts">
  import { chatAnswer } from '../terminal/api'
  import type { NeedCard } from '../home'

  // One unanswered tool approval on a Needs you card (Home and the bell): what the agent wants to run, and Allow once /
  // Deny without opening the chat. A card whose command was cut short cannot be allowed here: Open shows all of it.
  let { sessionId, approval, onanswered, compact = false }: {
    sessionId: string
    approval: NonNullable<NeedCard['approval']>
    onanswered?: () => void | Promise<void>
    compact?: boolean
  } = $props()

  let busy = $state(false)
  let failed = $state(false)

  async function answer(allow: boolean) {
    if (busy) return
    busy = true
    failed = false
    try {
      await chatAnswer(sessionId, approval.permissionId, allow)
    } catch {
      failed = true
    }
    try { await onanswered?.() } finally { busy = false }
  }

  const detail = $derived(approval.command || approval.path)
</script>

<div class="approval" class:compact>
  <span class="what">{approval.title}{approval.queued > 0 ? ` · ${approval.queued} more waiting` : ''}</span>
  {#if detail}<code class="cmd">{detail}</code>{/if}
  {#if approval.truncated}<span class="note">Too long to show here. Open the chat to read it all.</span>{/if}
  <div class="answers">
    <button type="button" class="btn allow" disabled={busy || approval.truncated} onclick={() => void answer(true)}>Allow once</button>
    <button type="button" class="btn deny" disabled={busy} onclick={() => void answer(false)}>Deny</button>
  </div>
  <span class="status" role="status">{failed ? 'That answer was not sent. Open the chat to try again.' : ''}</span>
</div>

<style>
  .approval { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  .what { font-size: 12px; font-weight: 600; color: var(--ink); overflow-wrap: anywhere; }
  .cmd {
    display: block; max-height: 4.2em; overflow: hidden; padding: 4px 6px; border-radius: 6px;
    background: var(--card-raised); border: 1px solid var(--line-soft, var(--line));
    font: 12px/1.4 var(--mono, monospace); color: var(--ink); overflow-wrap: anywhere; white-space: pre-wrap;
  }
  .note { font-size: 12px; color: var(--muted); }
  .answers { display: flex; align-items: center; gap: 8px; margin-top: 2px; }
  .btn {
    display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 0 12px;
    border-radius: 8px; font: 500 13px/1 var(--sans); cursor: pointer;
  }
  .btn:disabled { opacity: 0.55; cursor: not-allowed; }
  .allow { border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); }
  .deny { border: 1px solid var(--line-strong, var(--line)); background: transparent; color: var(--ink); }
  .deny:hover:not(:disabled) { background: var(--card-raised); }
  .status { font-size: 12px; color: var(--bad); min-height: 0; }
  .status:empty { display: none; }
  .compact .btn { min-height: 32px; font-size: 12px; padding: 0 10px; border-radius: 7px; }
  @media (pointer: coarse), (max-width: 1099px) {
    .btn { min-height: 48px; flex: 1 1 0; }
    .compact .btn { min-height: 44px; }
  }
</style>
