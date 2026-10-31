<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { UnauthorizedError } from '../api'
  import { agentChoices } from '../catalogue'
  import { notStartedText, refusalText, toolName } from '../agentLaunch'
  import { initialChoice, startable, toCreate, withAgent, withProject, type LaunchChoice, type LaunchCreate, type LaunchDefaults } from '../launchBar'
  import { createTerminal, structuredAgent, TerminalError, type FolderProject, type TerminalMachine, type TerminalSession } from '../terminal/api'

  // Starts a session straight from the Active column. The interface choice is the launch bar's own: the reet TUI is the default
  // wherever the machine offers it, and the agent's own TUI is one click away. Everything else (defaults, refusals, the one
  // idempotent create) is the launch bar's logic, so both start sessions the same way.
  let { projects, machines, profile, initialProject = '', onstarted, onsignedout, onclose }: {
    projects: FolderProject[]; machines: TerminalMachine[]; profile: LaunchDefaults; initialProject?: string
    onstarted: (session: TerminalSession) => void; onsignedout: (e: unknown) => void; onclose: () => void
  } = $props()

  const id = $props.id()
  const life = new AbortController()
  let dialog = $state<HTMLDialogElement>()
  let busy = $state(false), message = $state('')
  // Kept while a start's outcome is unknown, so Retry asks about the same request and cannot start a second agent.
  let request: (LaunchCreate & { structured: boolean }) | null = null
  const first = (): LaunchChoice => {
    const options = { machines }
    const start = initialChoice(projects, profile, options)
    const picked = projects.find(p => p.id === initialProject)
    return picked ? withProject(start, picked, profile, options) : start
  }
  let choice = $state<LaunchChoice>(first())
  const project = $derived(projects.find(p => p.id === choice.projectIds[0]) ?? null)
  const machine = $derived(machines.find(m => m.agentId === project?.host))
  const offers = $derived(machine ? agentChoices(machine).filter(o => o.available) : [])
  const hasChat = $derived(!!structuredAgent(machine?.capabilities, choice.agent))
  const chat = $derived(hasChat && choice.chat === true)
  const why = $derived(startable(choice, projects, machines, profile))

  function pickProject(value: string) {
    const next = projects.find(p => p.id === value)
    if (next) choice = withProject(choice, next, profile, { machines })
  }
  function pickAgent(value: string) { choice = withAgent(choice, value, project ? { ...project } : null, profile, { machines }) }
  function pickTui(reet: boolean) { const { trust: _t, ...rest } = choice; choice = { ...rest, chat: reet } }

  async function start(event: SubmitEvent) {
    event.preventDefault()
    if (busy || why) { if (why) message = why; return }
    if (!request || request.launch.prompt !== (choice.prompt || undefined)) {
      const made = toCreate(choice, project, crypto.randomUUID(), crypto.randomUUID().replaceAll('-', ''), profile)
      request = { ...made, structured: !!made.launch.structured }
    }
    const op = request
    busy = true; message = `Starting ${toolName(op.launch.tool)}…`
    try {
      const session = await createTerminal(op.agentId, op.requestId, op.title, 100, 30, AbortSignal.any([life.signal, AbortSignal.timeout(op.structured ? 60000 : 15000)]), op.launch)
      if (life.signal.aborted) return
      request = null
      const notStarted = notStartedText(session.state)
      if (notStarted) { message = notStarted; return }
      onstarted(session)
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status !== 503) {
        request = null
        message = refusalText(e.status, e.code, e.message)
        if (e.status === 401) onsignedout(new UnauthorizedError())
      } else message = "We didn't hear back, so the agent may have started. Check Active before starting another, or press Start to retry the same request."
    } finally { busy = false }
  }
  function close() { if (!busy) { dialog?.close(); onclose() } }
  onMount(() => { dialog?.showModal(); return () => { life.abort(); dialog?.close() } })
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`} oncancel={e => { e.preventDefault(); close() }}>
  <form class="sheet" data-shell-card onsubmit={start}>
    <header data-shell-head>
      <div>
        <p class="eyebrow" data-shell-aside>New session</p>
        <h2 id={`${id}-title`}>Start a session</h2>
        <p class="lede">Pick a project and an agent, then start it now.</p>
      </div>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close} disabled={busy}>×</button>
    </header>
    <div class="body" data-shell-body>
      {#if !projects.length}
        <p class="none">Add a project from Home first, then start a session in it.</p>
      {:else}
        <fieldset disabled={busy}>
          <label class="field"><span>Project</span>
            <select aria-label="Project" value={choice.projectIds[0] ?? ''} onchange={e => pickProject(e.currentTarget.value)}>
              {#each projects as p}<option value={p.id}>{p.name} · {machines.find(m => m.agentId === p.host)?.host ?? p.machine}</option>{/each}
            </select></label>
          <label class="field"><span>Agent</span>
            <select aria-label="Agent" value={choice.agent} onchange={e => pickAgent(e.currentTarget.value)}>
              {#each offers as o}<option value={o.agent.id}>{o.agent.name}</option>{/each}
              {#if !offers.some(o => o.agent.id === choice.agent)}<option value={choice.agent}>{toolName(choice.agent)}</option>{/if}
            </select></label>
          <div class="field"><span>Interface</span>
            <div class="mode" role="group" aria-label="Interface">
              <button type="button" class:on={chat} aria-pressed={chat} disabled={!hasChat} title={hasChat ? '' : 'This machine does not offer the reet TUI for that agent.'} onclick={() => pickTui(true)}>reet TUI</button>
              <button type="button" class:on={!chat} aria-pressed={!chat} onclick={() => pickTui(false)}>Agent TUI</button>
            </div>
            <small>{chat ? 'A native chat with approvals, history and images.' : `${toolName(choice.agent)}’s own terminal interface.`}</small>
          </div>
          <label class="field"><span>Prompt <small>Optional</small></span>
            <textarea aria-label="Prompt" rows="4" bind:value={choice.prompt} placeholder="What should the agent do?"></textarea></label>
          {#if chat}
            <label class="trust"><input type="checkbox" checked={choice.trust === true} onchange={e => choice = { ...choice, trust: e.currentTarget.checked }} />I trust this folder. OpenCode plugins in it may run.</label>
          {/if}
        </fieldset>
      {/if}
      {#if message || (why && projects.length)}<p class="note" role="status">{message || why}</p>{/if}
    </div>
    <footer data-shell-foot>
      <button type="button" class="btn" onclick={close} disabled={busy}>Cancel</button>
      <button class="btn primary" disabled={busy || !projects.length || !!why}>{busy ? 'Starting…' : 'Start'}</button>
    </footer>
  </form>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); padding: 0; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--card); color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: flex; flex-direction: column; max-height: 90dvh; }
  header { display: flex; justify-content: space-between; gap: 16px; padding: 22px 24px 16px; border-bottom: 1px solid var(--line); }
  .eyebrow { margin: 0; font-size: 10px; font-weight: 650; letter-spacing: .13em; text-transform: uppercase; color: var(--accent-text); }
  h2 { font: 500 24px/1.2 var(--serif); margin: 6px 0; }
  .lede { margin: 0; color: var(--muted); font-size: 13px; }
  .close { align-self: start; width: 34px; height: 34px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font-size: 22px; cursor: pointer; }
  .body { padding: 18px 24px; overflow-y: auto; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
  .field { display: flex; flex-direction: column; gap: 7px; margin-bottom: 15px; font-size: 13px; font-weight: 600; }
  .field small { font-size: 11px; font-weight: 400; color: var(--muted); }
  select, textarea { width: 100%; box-sizing: border-box; font-size: 13px; font-weight: 400; color: var(--ink); }
  textarea { resize: vertical; min-height: 90px; }
  .mode { display: inline-flex; align-self: flex-start; border: 1px solid var(--line); border-radius: 8px; padding: 3px; background: var(--bg-sunk); }
  .mode button { padding: 8px 14px; border: 1px solid transparent; border-radius: 5px; background: transparent; color: var(--muted); font: 500 12px var(--sans); cursor: pointer; }
  .mode button.on { background: var(--card); color: var(--ink); border-color: var(--line); }
  .mode button:disabled { opacity: .5; cursor: default; }
  .trust { display: flex; align-items: center; gap: 8px; font-size: 12px; }
  .note, .none { font-size: 12px; color: var(--muted); margin: 8px 0 0; }
  footer { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 24px; border-top: 1px solid var(--line); }
  @media (max-width: 650px) { select, textarea { font-size: 16px; } header, .body, footer { padding-left: 16px; padding-right: 16px; } }
</style>
