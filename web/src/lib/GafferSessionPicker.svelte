<script lang="ts">
  import { onMount } from 'svelte'
  import { sessionHref, terminalSessions, type TerminalSession } from './terminal/api'
  import WorkspaceDialog from './terminal/WorkspaceDialog.svelte'
  let { onclose, onselect }: { onclose: () => void; onselect: () => void } = $props()
  let sessions = $state<TerminalSession[]>([]), loading = $state(true), error = $state('')
  const life = new AbortController()
  async function load() {
    loading = true; error = ''
    try {
      const found = await terminalSessions(AbortSignal.any([life.signal, AbortSignal.timeout(10000)]))
      if (!life.signal.aborted) sessions = found.filter(item => item.structured && item.state === 'running')
    } catch (e) { if (!life.signal.aborted) error = e instanceof Error ? e.message : 'Could not read your sessions.' }
    finally { if (!life.signal.aborted) loading = false }
  }
  onMount(() => { void load(); return () => life.abort() })
  function choose(event: MouseEvent, item: TerminalSession) {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
    const request = new CustomEvent('reet:show-session', { detail: item.id, cancelable: true })
    if (!window.dispatchEvent(request)) event.preventDefault()
    onselect()
  }
</script>
<WorkspaceDialog title="Choose a session for Gaffer" {onclose}>
  <p>Choose a running chat, then use Look after this session to review permissions, set a goal and choose its limits. Selecting a session does not enable follow-up or send an instruction.</p>
  {#if loading}<p role="status">Reading your sessions…</p>
  {:else if error}<p role="alert">{error}</p><button type="button" class="btn" onclick={()=>void load()}>Try again</button>
  {:else if !sessions.length}<p>No running chats are available. Start an agent in Chat, then return to Gaffer.</p><a class="btn" href="/sessions">Open Sessions</a>
  {:else}<ul>{#each sessions as item (item.id)}<li><a href={`${sessionHref(item)}&gaffer=1`} onclick={event=>choose(event,item)}><strong>{item.title}</strong><span>{item.tool || 'Agent'} · {item.dir || 'Directory not reported'}</span></a></li>{/each}</ul>{/if}
</WorkspaceDialog>
<style>
  p { margin: 0 0 14px; color: var(--muted); line-height: 1.55; }
  ul { list-style: none; margin: 0; padding: 0; }
  li + li { border-top: 1px solid var(--line); }
  li a { display: flex; flex-direction: column; gap: 4px; min-height: 44px; padding: 12px 0; color: var(--ink); text-decoration: none; overflow-wrap: anywhere; }
  li span { font-size: 13px; color: var(--muted); }
  li a:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
</style>
