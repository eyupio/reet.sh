<script lang="ts">
 import TerminalIcon from '../terminal/TerminalIcon.svelte'
 let { view, chat, terminal, pending = false, reason = '', onview, onunavailable }: {
  view: 'chat' | 'terminal'; chat: boolean; terminal: boolean; pending?: boolean; reason?: string
  onview: (view: 'chat' | 'terminal') => void; onunavailable?: (reason: string) => void
 } = $props()
</script>
<div class="view-switch" role="group" aria-label="Session view" data-keeps-focus>
 <button type="button" aria-pressed={view === 'chat'} aria-disabled={!chat} title={chat ? 'Show the conversation; the agent keeps running' : 'This terminal session has no shared conversation view'} onclick={() => chat ? onview('chat') : onunavailable?.('This terminal session has no shared conversation view.')}><svg viewBox="0 0 16 16" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.3" aria-hidden="true"><path d="M13.5 7.5a5.5 5.5 0 0 1-7.8 5L2 14l1.3-3.6a5.5 5.5 0 1 1 10.2-2.9Z"/></svg>Chat</button>
 <button type="button" aria-pressed={view === 'terminal'} aria-disabled={!terminal && view !== 'terminal'} aria-busy={pending || undefined} title={terminal ? 'Show the agent’s native terminal; the agent keeps running' : pending ? 'Checking this running agent’s terminal support…' : reason || 'This session has no shared native terminal'} onclick={() => terminal ? onview('terminal') : onunavailable?.(pending ? 'Checking this running agent’s terminal support…' : reason || 'This session has no shared native terminal.')}><TerminalIcon name="screen"/>Terminal</button>
</div>
<style>
 .view-switch{display:inline-flex;align-items:center;flex:none;gap:2px;padding:3px;background:var(--line-soft);border-radius:9px}
 button{display:inline-flex;align-items:center;justify-content:center;gap:6px;min-height:30px;padding:4px 10px;border:1px solid transparent;border-radius:6px;background:none;color:var(--muted);font:500 14px var(--sans);white-space:nowrap;cursor:pointer}
 button[aria-pressed=true]{background:var(--bg);border-color:var(--line);color:var(--ink)}
 button:hover:not([aria-disabled=true]){color:var(--ink);background:var(--card-raised,var(--card))}
 button[aria-disabled=true]{cursor:default;color:var(--muted);opacity:.65}
 button:focus-visible{outline:2px solid var(--accent-text);outline-offset:2px}
 @media(pointer:coarse){button{min-height:44px;padding:4px 9px}}
</style>
