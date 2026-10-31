<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import type { Machine, ReleaseInfo, SyncAgent } from './api'
  import { agentBuild, channelBuild } from './machines'
  import { CATALOGUE } from './catalogue'
  import { softwareLine, softwarePlanActions, type SoftwarePlanAction } from './software'
  import { formatAge, type MachineModels } from './models'
  import { allModels, applyMachineSoftware, machineModels, machineSoftwarePlan, refreshMachineModels, type SoftwareEntry, type SoftwarePlan, type TerminalMachine } from './terminal/api'

  // A machine's policies live here rather than beside the buttons that act on
  // it, so a card says what is happening and this dialog says how it is set.
  let {
    machine,
    agent,
    terminal,
    releases,
    busy,
    onautoupdate,
    onpause,
    onscan,
    onswitch,
    onrevoke,
    onforget,
    onclose,
  }: {
    machine: Machine
    agent?: SyncAgent
    terminal?: TerminalMachine
    releases?: ReleaseInfo
    busy: boolean
    onautoupdate: (enabled: boolean) => void
    onpause: () => void
    onscan: () => void
    onswitch: () => void
    onrevoke: () => void
    onforget: () => void
    onclose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  const id = $props.id()
  const onDev = $derived(agent?.version === 'dev')
  const pointer = $derived(channelBuild(releases, onDev ? 'dev' : 'latest'))

  type ModelRead = { loading: boolean; machine: MachineModels | null; error: string; refreshError: string }
  let reads = $state<Record<string, ModelRead>>({})
  let refreshing = $state('')

  const syncTools = $derived(machine.groups.map(group => group.agent === 'agents' ? 'codex' : group.agent))
  const installed = $derived.by(() => {
    const tools = terminal?.capabilities?.tools
    const ids = tools ? new Set(tools) : new Set(syncTools)
    return CATALOGUE.filter(item => item.enabled && ids.has(item.id))
  })
  const canRefresh = $derived(!!terminal && terminal.ready && !!terminal.capabilities?.models && !terminal.capabilities?.modelsOff)

  type PlanRead = { loading: boolean; plan: SoftwarePlan | null; error: string }
  let plans = $state<Record<string, PlanRead>>({})
  let localEntries = $state<Record<string, SoftwareEntry>>({})
  let applying = $state('')
  let applyError = $state<Record<string, string>>({})
  let notice = $state<Record<string, string>>({})
  let confirm = $state<{ tool: string; action: SoftwarePlanAction; plan: SoftwarePlan } | null>(null)
  let confirmText = $state('')
  let confirmDialog = $state<HTMLDialogElement>()
  const softwareEntries: SoftwareEntry[] = $derived((terminal?.capabilities?.software ?? []).map(entry => localEntries[entry.tool] ?? entry))
  const canApply = $derived(terminal?.capabilities?.softwareApply === true)
  function catalogueName(tool: string): string { return CATALOGUE.find(item => item.id === tool)?.name ?? tool }
  function openApply(tool: string, action: SoftwarePlanAction, plan: SoftwarePlan) {
    applyError[tool] = ''
    notice[tool] = ''
    confirmText = ''
    confirm = { tool, action, plan }
    confirmDialog?.showModal()
  }
  function confirmReady(): boolean {
    return !!confirm && (confirm.action !== 'remove' || confirmText.trim().toLowerCase() === machine.host.toLowerCase())
  }
  async function applyConfirm() {
    if (!terminal || !confirm || applying) return
    const { tool, action } = confirm
    applying = tool
    applyError[tool] = ''
    try {
      const answer = await applyMachineSoftware(terminal.agentId, tool, action)
      if (answer.result.entry) localEntries[tool] = answer.result.entry
      notice[tool] = action === 'install'
        ? `Installed OpenCode ${answer.result.version} on ${machine.host}.`
        : action === 'update'
          ? `Updated OpenCode to ${answer.result.version} on ${machine.host}.`
          : action === 'rollback'
            ? `Rolled OpenCode back to ${answer.result.version} on ${machine.host}.`
            : `Removed reet's managed OpenCode on ${machine.host}.`
      confirmDialog?.close()
      plans[tool] = { loading: false, plan: null, error: '' }
    } catch (error) {
      applyError[tool] = error instanceof Error ? error.message : "That install didn't finish."
    } finally {
      applying = ''
    }
  }
  async function review(tool: string, action: SoftwarePlanAction) {
    if (!terminal || plans[tool]?.loading) return
    plans[tool] = { loading: true, plan: plans[tool]?.plan ?? null, error: '' }
    try {
      plans[tool] = { loading: false, plan: await machineSoftwarePlan(terminal.agentId, tool, action), error: '' }
    } catch (error) {
      plans[tool] = { loading: false, plan: plans[tool]?.plan ?? null, error: error instanceof Error ? error.message : "Couldn't build that plan just now." }
    }
  }

  const refreshHint = $derived.by(() => {
    if (!installed.length || canRefresh) return ''
    if (!terminal) return `${machine.host} is not connected for terminals. Refresh will be available when it connects.`
    if (!terminal.ready) return `${machine.host} is offline. Refresh will be available when it reconnects.`
    if (terminal.capabilities?.modelsOff) return `Model reads are switched off on ${machine.host}. Turn them on with reet terminal models on.`
    if (!terminal.capabilities?.models) return `Update reet on ${machine.host} to read its models.`
    return ''
  })

  const REASONS: Record<string, string> = {
    not_installed: 'not installed', unsupported: 'not supported', timed_out: 'timed out',
    too_large: 'the list was too large', exited: 'the command exited', unreadable: 'the output was unreadable',
    denied: 'the machine refused', conflict: 'another read was running', off: 'reads are off',
  }

  function modelLine(read: ModelRead | undefined): string {
    if (!read || read.loading) return 'Reading the saved model list…'
    if (read.error) return read.error
    const list = read.machine
    const age = formatAge(Date.now(), list?.fetchedAt)
    if (read.refreshError) return `${read.refreshError}${age ? ` Last read ${age}.` : ''}`
    if (!list) return 'No model list has been read from this machine yet.'
    switch (list.state) {
      case 'listed': return `${list.models.length}${list.more ? '+' : ''} read${age ? ` ${age}` : ''}`
      case 'unsupported': return 'Not supported; reet uses suggestions.'
      case 'failed': {
        const reason = REASONS[list.reason] ?? ''
        const kept = list.models?.length
          ? ` ${list.models.length}${list.more ? '+' : ''} kept${age ? ` from ${age}` : ''}.`
          : age ? ` Last read ${age}.` : ' No earlier list was kept.'
        return `Couldn't read${reason ? `: ${reason}` : ''}.${kept}`
      }
      case 'off': return 'Off; model reads are switched off on this machine.'
      case 'offline': return 'Offline; showing no live list.'
      case 'update': return "Update reet to read this agent's models."
    }
  }

  async function loadModels(tool: string, signal: AbortSignal) {
    reads[tool] = { loading: true, machine: reads[tool]?.machine ?? null, error: '', refreshError: '' }
    try {
      let next: MachineModels
      if (terminal) next = await machineModels(terminal.agentId, tool, signal)
      else {
        const answer = await allModels(tool, signal)
        const row = answer.machines?.find(item => item.host === machine.host)
        next = row ? { ...row, tool } : { tool, state: 'offline', models: [], more: false, source: '', version: '', fetchedAt: null, checkedAt: null, reason: '', sentence: '' }
      }
      if (!signal.aborted) reads[tool] = { loading: false, machine: next, error: '', refreshError: '' }
    } catch {
      if (!signal.aborted) reads[tool] = { loading: false, machine: reads[tool]?.machine ?? null, error: "Couldn't read the saved model list just now.", refreshError: '' }
    }
  }

  async function refresh(tool: string) {
    if (!terminal || !canRefresh || refreshing) return
    const previous = reads[tool]
    if (previous?.loading) return
    refreshing = tool
    reads[tool] = { loading: false, machine: previous?.machine ?? null, error: previous?.error ?? '', refreshError: '' }
    try {
      const next = await refreshMachineModels(terminal.agentId, tool)
      reads[tool] = { loading: false, machine: next, error: '', refreshError: '' }
    } catch {
      reads[tool] = { loading: false, machine: previous?.machine ?? null, error: previous?.error ?? '', refreshError: "Couldn't refresh the list just now. Showing the last read." }
    } finally {
      refreshing = ''
    }
  }

  onMount(() => {
    dialog?.showModal()
    return () => {
      if (dialog?.open) dialog.close()
    }
  })

  onMount(() => {
    if (!installed.length) return
    const abort = new AbortController()
    for (const item of installed) void loadModels(item.id, abort.signal)
    return () => abort.abort()
  })
</script>

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby={`${id}-title`}>
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Machine settings</p>
        <h2 id={`${id}-title`}>{machine.host}</h2>
        <p class="lede" data-shell-aside>How this machine keeps itself current, and what the controller may ask of it. A change applies straight away.</p>
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close settings" data-shell-close>×</button>
    </header>

    {#if agent}
      <section aria-labelledby={`${id}-updates`}>
        <h3 id={`${id}-updates`}>Updates</h3>
        <div class="row">
          <div class="copy">
            <strong id={`${id}-auto`}>Automatic updates</strong>
            <small>Install new reet builds on this machine without a review. The agent verifies each build and keeps the previous binary; a build that cannot report back is put back.</small>
          </div>
          <button type="button" role="switch" class="switch" aria-checked={agent.autoUpdate} aria-labelledby={`${id}-auto`} disabled={busy} onclick={() => onautoupdate(!agent.autoUpdate)}>
            <span class="knob" aria-hidden="true"></span>
          </button>
        </div>
        <div class="row">
          <div class="copy">
            <strong>Release channel</strong>
            <small>
              {onDev ? 'On the development channel, which follows main.' : 'On the release channel, which follows tagged builds.'}
              Runs <code>{agentBuild(agent)}</code>{#if pointer}; the channel points at <code>{pointer}</code>{/if}.
            </small>
          </div>
          <button class="btn" type="button" disabled={busy || agent.paused} onclick={onswitch}>Switch channel…</button>
        </div>
      </section>

      <section aria-labelledby={`${id}-sync`}>
        <h3 id={`${id}-sync`}>Sync</h3>
        <div class="row">
          <div class="copy">
            <strong id={`${id}-pause`}>Pause sync jobs</strong>
            <small>While paused, no new sync jobs, scans or updates run on this machine. A job already running may finish.</small>
          </div>
          <button type="button" role="switch" class="switch" aria-checked={agent.paused} aria-labelledby={`${id}-pause`} disabled={busy} onclick={onpause}>
            <span class="knob" aria-hidden="true"></span>
          </button>
        </div>
        <div class="row">
          <div class="copy">
            <strong>Inventory</strong>
            <small>Ask the agent to report its installed skills again. Nothing is installed or removed.</small>
          </div>
          <button class="btn" type="button" disabled={busy || agent.paused} onclick={onscan}>Scan now</button>
        </div>
      </section>
    {/if}

    <section aria-labelledby={`${id}-agents`}>
      <h3 id={`${id}-agents`}>Agents</h3>
      <p class="hint">Installed agents and the model list this machine last read. A list is a suggestion; a start never waits for it.</p>
      {#if refreshHint}<p class="hint">{refreshHint}</p>{/if}
      {#if installed.length === 0}
        <p class="hint">No launchable agents have reported from this machine yet.</p>
      {:else}
        {#each installed as item (item.id)}
          {@const read = reads[item.id]}
          {@const modelState = read?.machine?.state ?? ''}
          {@const ready = canRefresh && !read?.loading && (!modelState || modelState === 'listed' || modelState === 'failed')}
          <div class="row agent" data-agent={item.id} data-model-state={modelState}>
            <div class="copy">
              <strong>{item.name}</strong>
              <small class="model-state" aria-live="polite">{modelLine(read)}</small>
            </div>
            <button class="btn" type="button" disabled={!ready || refreshing === item.id} onclick={() => void refresh(item.id)} aria-label={`Refresh ${item.name} models`}>
              {refreshing === item.id ? 'Refreshing…' : 'Refresh'}
            </button>
          </div>
        {/each}
      {/if}
    </section>

    {#if softwareEntries.length}
      <section aria-labelledby={`${id}-software`}>
        <h3 id={`${id}-software`}>Software</h3>
        <p class="hint">Installation evidence reet can see. A plan names the reviewed version and is never run from here.</p>
        {#each softwareEntries as entry (entry.tool)}
          {@const actions = softwarePlanActions(entry)}
          {@const read = plans[entry.tool]}
          <div class="row software" data-software={entry.tool} data-software-state={entry.state}>
            <div class="copy">
              <strong>{catalogueName(entry.tool)}{#if entry.state === 'installed' && entry.version}<code> {entry.version}</code>{/if}</strong>
              <small class="software-detail">{softwareLine(entry)}</small>
              {#if notice[entry.tool]}<small class="plan-notice">{notice[entry.tool]}</small>{/if}
              {#if read?.loading}<small aria-live="polite">Reading the plan…</small>{/if}
              {#if read?.error}<small class="plan-error">{read.error}</small>{/if}
              {#if read?.plan}
                {@const plan = read.plan}
                <div class="plan" data-plan-state={plan.state}>
                  <p>{plan.sentence}</p>
                  {#if plan.steps.length}<ol>{#each plan.steps as step}<li>{step}</li>{/each}</ol>{/if}
                  {#each plan.effects as effect}<small>{effect}</small>{/each}
                  {#if plan.source}<small class="plan-source">Source: <code>{plan.source}</code></small>{/if}
                  {#if plan.command}<code class="plan-command">{plan.command}</code>{/if}
                  {#if plan.state === 'ready' && canApply}
                    <button class="btn plan-apply" type="button" disabled={!!applying || read?.loading} onclick={() => openApply(entry.tool, plan.action, plan)}>
                      {applying === entry.tool ? 'Applying…' : `Apply ${plan.action}`}
                    </button>
                  {:else if plan.state === 'ready'}
                    <small class="hint">Update reet on {machine.host} to apply this plan.</small>
                  {/if}
                  {#if applyError[entry.tool]}<small class="plan-error">{applyError[entry.tool]}</small>{/if}
                </div>
              {/if}
            </div>
            {#if actions.length}
              <div class="plan-actions">
                {#each actions as action (action)}
                  <button class="btn" type="button" disabled={read?.loading} onclick={() => void review(entry.tool, action)}>
                    Review {action} plan
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </section>
    {/if}

    <section aria-labelledby={`${id}-access`}>
      <h3 id={`${id}-access`}>Access</h3>
      {#if agent}
        <div class="row">
          <div class="copy">
            <strong>Live access</strong>
            <small>Close private access and stop new terminal authorisation and sync jobs. Installed files and running shells stay.</small>
          </div>
          <button class="btn" type="button" disabled={busy} onclick={onrevoke}>Revoke live access…</button>
        </div>
      {/if}
      <div class="row">
        <div class="copy">
          <strong>Forget this machine</strong>
          <small>Remove its inventory reports and saved sync references here. Enrollment stays, and a future scan can report it again.</small>
        </div>
        <button class="btn danger" type="button" disabled={busy} onclick={onforget}>Forget machine…</button>
      </div>
    </section>
  </div>
</dialog>

<dialog bind:this={confirmDialog} class="confirm" aria-labelledby={`${id}-confirm-title`} onclose={() => { confirm = null; confirmText = '' }}>
  <div class="confirm-body">
    <h2 id={`${id}-confirm-title`}>Apply this {confirm?.action}?</h2>
    {#if confirm}
      <p>{confirm.plan.sentence}</p>
      <dl>
        <dt>Machine</dt><dd>{machine.host}</dd>
        <dt>Version</dt><dd>{confirm.plan.target || confirm.plan.current}</dd>
        <dt>Source</dt><dd><code>{confirm.plan.source}</code></dd>
      </dl>
      {#each confirm.plan.effects as effect}<p class="hint">{effect}</p>{/each}
      {#if confirm.action === 'remove'}
        <p class="hint">This removes reet's managed copy and every version it kept. Any OpenCode installed outside reet is untouched.</p>
        <label class="typed-confirm">Type <code>{machine.host}</code> to confirm
          <input bind:value={confirmText} autocomplete="off" spellcheck="false" placeholder={machine.host} />
        </label>
      {:else}
        <p class="hint">reet downloads the artifact, verifies its recorded checksum, and only then makes it current.</p>
      {/if}
      {#if applyError[confirm.tool]}<p class="plan-error">{applyError[confirm.tool]}</p>{/if}
    {/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => confirmDialog?.close()} disabled={!!applying}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void applyConfirm()} disabled={!!applying || !confirmReady()}>{applying ? 'Applying…' : 'Apply now'}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { width: min(640px, calc(100vw - 32px)); max-height: min(88vh, 760px); padding: 0; border: 1px solid var(--line); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px rgb(0 0 0 / 0.28); }
  dialog::backdrop { background: rgb(20 18 16 / 0.45); backdrop-filter: blur(2px); }
  .sheet { display: grid; gap: 18px; padding: clamp(20px, 4vw, 30px); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; overflow-wrap: anywhere; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.86rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  section { display: grid; gap: 0; padding-top: 14px; border-top: 1px solid var(--line); }
  h3 { margin: 0 0 4px; color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.1em; text-transform: uppercase; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 10px 0; }
  .row + .row { border-top: 1px solid color-mix(in srgb, var(--line) 60%, transparent); }
  .copy { display: grid; gap: 2px; min-width: 0; }
  .copy strong { font-size: 0.86rem; }
  .copy small { color: var(--muted); font-size: 0.76rem; line-height: 1.5; }
  .copy code { font-size: 0.74rem; }
  .hint { margin: 4px 0 0; color: var(--muted); font-size: 0.76rem; line-height: 1.5; }
  .model-state { overflow-wrap: anywhere; }
  .software-detail { overflow-wrap: anywhere; }
  .plan { display: grid; gap: 6px; margin-top: 6px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .plan p, .plan ol { margin: 0; }
  .plan ol { display: grid; gap: 4px; padding-left: 18px; }
  .plan-command { font: 12px/1.4 var(--mono, monospace); overflow-wrap: anywhere; }
  .plan-error { color: var(--bad); }
  .plan-source { overflow-wrap: anywhere; }
  .plan-source code { font-size: 0.72rem; }
  .plan-notice { color: var(--good); }
  .plan-apply { align-self: start; margin-top: 2px; }
  .plan-actions { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
  .typed-confirm { display: grid; gap: 6px; color: var(--muted); font-size: 0.82rem; }
  .typed-confirm input { height: 36px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--mono, monospace); }
  .confirm { width: min(520px, calc(100vw - 32px)); border-radius: 16px; }
  .confirm-body { display: grid; gap: 12px; padding: clamp(20px, 4vw, 28px); }
  .confirm h2 { margin: 0; font: 500 clamp(1.3rem, 3vw, 1.7rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  .confirm p { margin: 0; color: var(--ink); font-size: 0.88rem; line-height: 1.55; }
  .confirm dl { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; font-size: 0.82rem; }
  .confirm dt { color: var(--muted); }
  .confirm dd { margin: 0; overflow-wrap: anywhere; }
  .confirm-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
  .confirm-actions .btn { min-height: 38px; padding: 8px 14px; }
  .confirm-actions .primary { border-color: var(--accent); background: var(--accent); color: var(--accent-ink); }
  .row .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  .btn.danger { color: var(--bad); }
  .switch { position: relative; flex: none; width: 42px; height: 24px; padding: 0; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); cursor: pointer; }
  .switch[aria-checked='true'] { background: var(--accent); border-color: var(--accent); }
  .switch:disabled { opacity: 0.55; cursor: default; }
  .switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .knob { position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--card); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .switch[aria-checked='true'] .knob { transform: translateX(18px); }
  @media (max-width: 560px) {
    .row { flex-wrap: wrap; }
    .row .btn { flex: 1 1 auto; }
  }
  @media (pointer: coarse) {
    .row .btn { min-height: 44px; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .switch { transition: background-color 160ms ease, border-color 160ms ease; }
    .knob { transition: transform 160ms var(--ease-out); }
  }
</style>
