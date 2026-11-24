<script lang="ts">
  import type { ChatState } from '../chat'
  import { chatLine, gafferProgress, type ChatProgressState } from '../gafferProgress'
  import type { ChatUsageView } from '../chatUsage'
  import type { PlanEntry } from '../agentAccounts'
  import PlanMeters from '../PlanMeters.svelte'

  let { chat, dir = '', agent = 'Agent', usage = null, plan = [], planNow = Date.now(), replayed, ended, sessionFailed, reattaching, heldSince = 0 }: ChatProgressState & { chat: ChatState; dir?: string; agent?: string; usage?: ChatUsageView | null; plan?: PlanEntry[]; planNow?: number; heldSince?: number } = $props()
  const view = $derived(gafferProgress(chat, dir, replayed, ended, agent, sessionFailed, reattaching))
  const attention = $derived(view.phase === 'approval' || view.phase === 'question')
  // The turn has ended: a spinner means the agent's turn, so held work gets the hollow ring instead.
  const held = $derived(view.phase === 'background')
  const problem = $derived(view.phase === 'error' || view.phase === 'reconnecting')
  const catchingUp = $derived(!ended && (chat.connection === 'connecting' || (!replayed && chat.connection !== 'reconnecting')))
  // How long ago the held turn ended, read again each half minute so the minutes move without animating anything.
  let clock = $state(Date.now())
  $effect(() => { if (!held) return; const timer = setInterval(() => clock = Date.now(), 30_000); return () => clearInterval(timer) })
  const stopped = $derived(chat.status.state === 'idle' && chat.status.code === 'unwatched')
  const label = $derived(catchingUp ? `Catching up with ${agent}` : chatLine(view, heldSince || undefined, clock))
  const detail = $derived(catchingUp ? 'Loading conversation…' : view.target || (view.active && !held ? 'You can queue your next instruction.' : ''))
</script>

<div class="conversation-status" class:attention class:problem class:held data-phase={view.phase} data-status={held ? 'working' : stopped ? 'finished' : undefined} data-stopped={stopped ? '' : undefined} aria-label="Conversation status">
  <span class="indicator" class:spinning={(view.active && !held) || catchingUp} aria-hidden="true">
    {#if held && !catchingUp}<span class="status-ring"></span>
    {:else if view.active || catchingUp}<svg viewBox="0 0 20 20" fill="none"><circle cx="10" cy="10" r="7" stroke="currentColor" stroke-width="2" opacity=".18"/><path d="M10 3a7 7 0 0 1 7 7" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
    {:else if attention || problem}!{:else if view.phase === 'idle' && !stopped}✓{:else if stopped}<span class="status-mark"></span>{:else}<span class="dot"></span>{/if}
  </span>
  <strong class="label" role="status" aria-live="polite" aria-atomic="true">{label}</strong>
  {#if detail}<span class="detail" title={detail}>{detail}</span>{/if}
  {#if !catchingUp && view.recent.length && !held}
    <span class="counts" aria-label={`${view.partial || problem ? 'Last known loaded' : 'This turn’s'} tool results`} title={view.partial ? 'Counts cover only the loaded part of this turn.' : problem ? 'Last known tool results; live progress may be unavailable.' : 'Reported tool results this turn.'}>
      {#if problem}Last seen: {/if}{view.completed} done{#if view.running && !problem && !held}{' · '}{view.running} active{/if}{#if view.failed}<span class="failed">{' · '}{view.failed} failed</span>{/if}
    </span>
  {/if}
  {#if usage}
    <span class="usage" class:low={usage.low} title={usage.detail} data-chat-usage>
      {#if usage.percentLeft !== null}<span class="usage-meter" aria-hidden="true"><span style:width={`${usage.percentLeft}%`}></span></span>{/if}
      <span>{usage.label}</span><span class="vh">. {usage.detail}</span>
    </span>
  {/if}
  {#if plan.length}
    <!-- What is left of the agent's plan: its mark and the window with the least left. Hover says every window; a click opens them with their resets. -->
    <span class="usage plan"><PlanMeters entries={plan} now={planNow} max={1} /></span>
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
  /* What the agent reports: context left, or the chat's tokens. After the counts when there are any, else at the end of the line. */
  .usage { flex: none; display: inline-flex; align-items: center; gap: 6px; margin-left: auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
  .counts + .usage, .usage + .usage { margin-left: 0; padding-left: 8px; border-left: 1px solid var(--line-soft); }
  .usage.low { color: var(--warn); }
  .usage-meter { display: block; width: 36px; height: 3px; border-radius: 2px; background: var(--line-soft); overflow: hidden; }
  .usage-meter > span { display: block; height: 100%; border-radius: 2px; background: var(--accent); }
  .usage.low .usage-meter > span { background: var(--warn); }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @keyframes status-spin { to { transform: rotate(360deg); } }
  /* The detail gives way on a narrow line, except while work is held: then it is how the person knows they may type. */
  @container chat-status (max-width: 620px) { .detail { display: none; } .held .detail { display: inline; flex: 1 1 auto; } }
  .conversation-status.held { flex-wrap: wrap; }
  @container chat-status (max-width: 420px) { .usage-meter { display: none; } }
  /* On a desktop the floating Ask Gaffer circle sits over this line's right-hand end, above the composer. Keep the
     figures out from under it; the circle is not shown beside a docked details panel, on a phone or on a touch tablet. */
  @media (min-width: 768px) and (not (pointer: coarse)) {
    :global(html:has(.gaffer-launcher:not([data-side=left])):not(:has(.context-docked)):not(:has(.context-sheet))) .conversation-status { padding-right: 68px; }
  }
  /* On a phone, an approval or a question is already said twice above this line: by the pane's status line and by the
     card that asks. A third time costs a row of the conversation, so this line waits until the answer is given. */
  @media (max-width: 650px) { .conversation-status.attention { display: none; } }
  @media (prefers-reduced-motion: reduce) { .spinning svg { animation: none; } }
</style>
