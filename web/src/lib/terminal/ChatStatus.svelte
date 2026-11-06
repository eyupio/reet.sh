<script lang="ts">
  import type { ChatState } from '../chat'
  import { gafferProgress, type ChatProgressState } from '../gafferProgress'

  let { chat, dir = '', agent = 'Agent', replayed, ended, sessionFailed, reattaching }: ChatProgressState & { chat: ChatState; dir?: string; agent?: string } = $props()
  const view = $derived(gafferProgress(chat, dir, replayed, ended, agent, sessionFailed, reattaching))
  const attention = $derived(view.phase === 'approval' || view.phase === 'question')
  const problem = $derived(view.phase === 'error' || view.phase === 'reconnecting')
  const catchingUp = $derived(!ended && (chat.connection === 'connecting' || (!replayed && chat.connection !== 'reconnecting')))
  const label = $derived(catchingUp ? `Catching up with ${agent}` : view.label)
  const detail = $derived(catchingUp ? 'Loading conversation…' : view.target || (view.active ? 'You can queue your next instruction.' : ''))
</script>

<div class="conversation-status" class:attention class:problem data-phase={view.phase} aria-label="Conversation status">
  <span class="indicator" class:spinning={view.active || catchingUp} aria-hidden="true">
    {#if view.active || catchingUp}<svg viewBox="0 0 20 20" fill="none"><circle cx="10" cy="10" r="7" stroke="currentColor" stroke-width="2" opacity=".18"/><path d="M10 3a7 7 0 0 1 7 7" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
    {:else if attention || problem}!{:else if view.phase === 'idle'}✓{:else}<span class="dot"></span>{/if}
  </span>
  <strong class="label" role="status" aria-live="polite" aria-atomic="true">{label}</strong>
  {#if detail}<span class="detail" title={detail}>{detail}</span>{/if}
  {#if !catchingUp && view.recent.length}
    <span class="counts" aria-label={`${view.partial || problem ? 'Last known loaded' : 'This turn’s'} tool results`} title={view.partial ? 'Counts cover only the loaded part of this turn.' : problem ? 'Last known tool results; live progress may be unavailable.' : 'Reported tool results this turn.'}>
      {#if problem}Last seen: {/if}{view.completed} done{#if view.running && !problem}{' · '}{view.running} active{/if}{#if view.failed}<span class="failed">{' · '}{view.failed} failed</span>{/if}
    </span>
  {/if}
</div>

<style>
  .conversation-status { --tone: var(--accent-text); flex: none; display: flex; align-items: center; gap: 8px; min-width: 0; min-height: 38px; margin: 0 14px 8px; padding: 7px 11px; border: 1px solid var(--line-soft); border-radius: 8px; background: color-mix(in srgb, var(--card) 65%, transparent); color: var(--muted); font: 14px/1.5 var(--sans); container: chat-status / inline-size; }
  .attention { --tone: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); background: color-mix(in srgb, var(--warn) 5%, var(--card)); }
  .problem { --tone: var(--bad); border-color: color-mix(in srgb, var(--bad) 40%, var(--line)); }
  .indicator { flex: none; display: grid; place-items: center; width: 16px; height: 16px; color: var(--tone); font-weight: 600; }
  .indicator svg { width: 16px; height: 16px; }
  .spinning svg { animation: status-spin 1s linear infinite; }
  .dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
  .label { flex: 0 1 auto; min-width: 0; color: var(--ink-soft); font-weight: 500; overflow-wrap: anywhere; }
  .detail { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .counts { flex: none; margin-left: auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .failed { color: var(--bad); }
  @keyframes status-spin { to { transform: rotate(360deg); } }
  @container chat-status (max-width: 620px) { .detail { display: none; } }
  @media (prefers-reduced-motion: reduce) { .spinning svg { animation: none; } }
</style>
