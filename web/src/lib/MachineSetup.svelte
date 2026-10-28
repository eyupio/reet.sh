<script lang="ts">
  import type { Snippet } from 'svelte'
  import { tick, untrack } from 'svelte'
  import { terminalControllerStatus, terminalMachines, readiness, type TerminalControllerStatus, type TerminalMachine } from './terminal/api'
  import { controllerDetail } from './terminal/controllerStatus'
  import {
    updateSyncAgent,
    type Machine, type SetupProfileSummary, type SyncJob, type SyncState,
  } from './api'
  import { agentBuild, agentGroups, agentUpdateTarget, latestRelease, updateAvailable } from './machines'
  import {
    configureCommand, connectionStage, enrollCommands, installCommands, profileFetchCommand, profilePullCommand,
    profileRestoreCommand, profileVerifyCommand, retryDelay, scanCommand, syncLink, upgradeCommands,
    type ReleaseChannel,
  } from './machineSetup'
  import CopyButton from './CopyButton.svelte'

  let {
    site,
    machines,
    profiles = [],
    token = '',
    tokenSetup,
    serverBuild = null,
    snapshot,
    open = $bindable(false),
    host = $bindable(''),
    initialMode = 'connect',
    terminalAccess = false,
    onrefresh,
  }: {
    site: string
    machines: Machine[]
    profiles?: SetupProfileSummary[]
    token?: string
    tokenSetup?: Snippet
    serverBuild?: { version: string; commit: string } | null
    snapshot?: SyncState
    open?: boolean
    /** The machine name field; the Add a machine dialog fills it in when the person asks for the step-by-step setup. */
    host?: string
    initialMode?: 'connect' | 'scratch'
    terminalAccess?: boolean
    onrefresh: () => Promise<void> | void
  } = $props()

  type Mode = 'connect' | 'scratch'
  // svelte-ignore state_referenced_locally
  let mode = $state<Mode>(initialMode)
  let step = $state(1)
  let channel = $state<ReleaseChannel>('stable')
  let channelTouched = $state(false)
  // svelte-ignore state_referenced_locally
  let profileName = $state(profiles[0]?.name ?? '')
  let allowApply = $state(false)
  let checking = $state(false)
  let terminalError = $state('')
  let terminalHosts = $state<TerminalMachine[]>([])
  let lastCheckedAt = $state(0)
  let checkFailures = $state(0)
  let now = $state(Date.now())
  let updateBusy = $state(false)
  let updateNotice = $state('')
  let updateError = $state('')
  let terminalRequest: AbortController | null = null
  // The server's own terminal switch, read the first time step 3 is opened. Undefined while it is being read;
  // null when the read failed.
  let serverTerminal = $state<TerminalControllerStatus | null | undefined>(undefined)

  const wantedHost = $derived(host.trim().toLowerCase())
  const terminalHost = $derived.by(() => {
    if (!wantedHost) return undefined
    const matches = terminalHosts.filter((item) => item.host.toLowerCase() === wantedHost)
    return matches.length === 1 ? matches[0] : undefined
  })
  const stage = $derived(connectionStage(terminalHost, terminalAccess))
  const terminalReady = $derived(stage === 'ready' && !terminalError)

  const selectedProfile = $derived(profiles.find((profile) => profile.name === profileName))
  const revision = $derived(selectedProfile?.revision ?? 1)
  const name = $derived(host.trim() || 'my-machine')
  const install = $derived(installCommands(channel))
  const configure = $derived(configureCommand(site))
  const scan = $derived(scanCommand(site, name))
  const enroll = $derived(enrollCommands(site, name, allowApply))
  const pull = $derived(selectedProfile ? profilePullCommand(site, selectedProfile.name) : '')
  const fetch = $derived(selectedProfile ? profileFetchCommand(selectedProfile.name, revision) : '')
  const preview = $derived(selectedProfile ? profileRestoreCommand(selectedProfile.name, revision) : '')
  const apply = $derived(preview ? `${preview} --apply` : '')
  const verify = $derived(selectedProfile ? profileVerifyCommand(selectedProfile.name, revision) : '')
  const upgrade = $derived(upgradeCommands(channel))
  const connected = $derived(machines.find((machine) =>
    machine.host.toLowerCase() === wantedHost &&
    agentGroups(machine).some((group) => group.complete && group.scannedAt > Date.now() - 14 * 86400000)))
  const enrolledAgent = $derived.by(() => {
    if (!wantedHost) return undefined
    return (snapshot?.agents ?? []).find((agent) => agent.host.toLowerCase() === wantedHost)
  })
  const latest = $derived(enrolledAgent ? latestRelease(enrolledAgent, snapshot?.releases) : null)
  const updateReady = $derived(!!enrolledAgent && !!snapshot && updateAvailable(enrolledAgent, snapshot.releases))
  const updateJob = $derived.by((): SyncJob | undefined => {
    if (!enrolledAgent) return undefined
    return (snapshot?.jobs ?? [])
      .filter((job) => job.agentId === enrolledAgent.id && job.kind === 'update')
      .sort((a, b) => b.createdAt - a.createdAt)[0]
  })
  const steps = $derived(mode === 'connect'
    ? [[1, 'Install'], [2, 'Authenticate'], [3, 'Connect terminal'], [4, 'Inventory & sync']]
    : [[1, 'Install'], [2, 'Restore setup'], [3, 'Connect terminal'], [4, 'Inventory & sync']])

  const watchTitle = $derived(
    terminalReady
      ? `${terminalHost?.host ?? name} is ready for terminals.`
      : terminalError
        ? 'The connection check needs another try'
        : 'Waiting for terminal readiness',
  )
  const watchDetail = $derived(
    terminalError ||
    (!terminalAccess
      ? 'Browser terminals require account sign-in with a write role.'
      : terminalHost
        ? readiness(terminalHost)
        : stage === 'blocked'
          ? readiness(terminalHost)
          : 'Readiness comes from this enrolled machine’s live connection; a completed scan alone does not establish it.'),
  )

  let panelHeading = $state<HTMLHeadingElement | null>(null)
  let announcedStep = $state(1)

  $effect(() => {
    if (step === announcedStep) return
    announcedStep = step
    panelHeading?.focus()
  })

  $effect(() => {
    if (!profileName && profiles.length > 0) profileName = profiles[0].name
  })
  $effect(() => {
    if (!channelTouched && serverBuild?.version === 'dev' && channel !== 'dev') channel = 'dev'
  })

  $effect(() => {
    if (!open) return
    now = Date.now()
    const timer = setInterval(() => (now = Date.now()), 5000)
    return () => clearInterval(timer)
  })

  $effect(() => {
    if (!open || step !== 3 || !terminalAccess || serverTerminal !== undefined) return
    const abort = new AbortController()
    void terminalControllerStatus(AbortSignal.any([abort.signal, AbortSignal.timeout(10000)]))
      .then((status) => { if (!abort.signal.aborted) serverTerminal = status })
      .catch(() => { if (!abort.signal.aborted) serverTerminal = null })
    return () => abort.abort()
  })

  // Ask the controller while the operator is over on the other machine. The
  // watch starts immediately on entering a step, wakes on tab focus and on
  // reconnect, and backs off only while checks keep failing. Old responses are
  // aborted so a slow request can never overwrite a newer answer.
  $effect(() => {
    const waiting = open && (step === 3 || step === 4)
    if (!waiting) {
      abortTerminalCheck()
      return
    }
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined

    const tick = async () => {
      if (stopped) return
      if (document.visibilityState === 'visible') {
        if (step === 3) {
          if (terminalAccess) await checkTerminal()
        } else {
          await refreshInventory()
        }
      }
      if (stopped) return
      const base = step === 4 ? 5000 : undefined
      timer = setTimeout(() => void tick(), untrack(() => retryDelay(checkFailures, base)))
    }
    const wake = () => {
      if (stopped || document.visibilityState !== 'visible') return
      clearTimeout(timer)
      void tick()
    }

    void tick()
    document.addEventListener('visibilitychange', wake)
    window.addEventListener('online', wake)
    return () => {
      stopped = true
      clearTimeout(timer)
      document.removeEventListener('visibilitychange', wake)
      window.removeEventListener('online', wake)
      abortTerminalCheck()
    }
  })

  function abortTerminalCheck() {
    terminalRequest?.abort()
    terminalRequest = null
  }

  async function checkTerminal(): Promise<boolean> {
    const controller = new AbortController()
    abortTerminalCheck()
    terminalRequest = controller
    try {
      const next = await terminalMachines(AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]))
      if (controller.signal.aborted) return false
      terminalHosts = next
      terminalError = ''
      checkFailures = 0
      lastCheckedAt = Date.now()
      return true
    } catch (error) {
      if (controller.signal.aborted) return false
      terminalError = error instanceof Error ? error.message : 'Terminal connection status is unavailable.'
      checkFailures += 1
      lastCheckedAt = Date.now()
      return false
    } finally {
      if (terminalRequest === controller) terminalRequest = null
    }
  }

  async function refreshInventory() {
    try {
      await onrefresh()
      checkFailures = 0
    } catch {
      checkFailures += 1
    } finally {
      lastCheckedAt = Date.now()
    }
  }

  async function check() {
    if (checking) return
    checking = true
    try {
      await onrefresh()
      if (terminalAccess) await checkTerminal()
    } finally {
      checking = false
    }
  }

  async function updateAgent() {
    if (!enrolledAgent || updateBusy) return
    updateBusy = true
    updateNotice = ''
    updateError = ''
    try {
      await updateSyncAgent(enrolledAgent.id, agentUpdateTarget(enrolledAgent))
      updateNotice = `Update to ${agentUpdateTarget(enrolledAgent)} queued. The machine verifies the release, keeps the previous binary, then restarts.`
      await onrefresh()
    } catch (error) {
      updateError = error instanceof Error ? error.message : 'The update could not be queued.'
    } finally {
      updateBusy = false
    }
  }

  function chooseMode(next: Mode) {
    mode = next
    step = 1
  }

  function go(next: number) {
    step = next
  }

  // A name is needed before the commands mean anything. Say so, put the person on the field, and let the button stay pressable.
  let nameError = $state('')
  let nameInput = $state<HTMLInputElement>()
  async function continueTo(next: number) {
    if (!host.trim()) {
      nameError = 'Name this machine to continue.'
      step = 1
      await tick()
      nameInput?.focus()
      return
    }
    nameError = ''
    go(next)
  }

  function ago(at: number): string {
    if (!at) return 'Not yet'
    const seconds = Math.max(0, Math.round((now - at) / 1000))
    if (seconds < 5) return 'Just now'
    if (seconds < 60) return `${seconds}s ago`
    const minutes = Math.round(seconds / 60)
    return `${minutes}m ago`
  }

  function updateJobLabel(job: SyncJob): string {
    const when = new Date(job.createdAt).toLocaleString()
    switch (job.status) {
      case 'queued': return `Update queued ${when}`
      case 'running': return 'Update in progress on the machine'
      case 'awaiting-approval': return 'Update waiting for the agent'
      case 'succeeded': return `Updated to ${job.revision || 'latest'} ${when}`
      case 'failed': return `Update failed: ${job.message || 'unknown error'}`
      case 'cancelled': return 'Update cancelled'
      default: return `Update ${job.status}`
    }
  }
</script>

<details class="setup" id="connect-machine" bind:open>
  <summary>
    <span>
      <span class="eyebrow">Machine setup</span>
      <strong>Advanced: step-by-step setup</strong>
      <small>Install the CLI, authenticate and enroll by hand. Most people can use Add a machine instead.</small>
    </span>
    <span class="summary-state">Optional</span>
    <span class="chevron" aria-hidden="true"></span>
  </summary>

  <div class="setup-body">
    <div class="modes" role="group" aria-label="Setup type">
      <button type="button" class:active={mode === 'connect'} aria-pressed={mode === 'connect'} onclick={() => chooseMode('connect')}>
        <span class="mode-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16M7 4 4 7l3 3M20 17H4m13-3 3 3-3 3"/></svg></span>
        <span><strong>Connect an existing machine</strong><small>It already has agents or skills. Connect its terminal, then choose what to sync.</small></span>
      </button>
      <button type="button" class:active={mode === 'scratch'} aria-pressed={mode === 'scratch'} onclick={() => chooseMode('scratch')}>
        <span class="mode-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg></span>
        <span><strong>Start from scratch</strong><small>A new machine. Restore a saved setup, then scan and sync.</small></span>
      </button>
    </div>

    <nav class="steps" aria-label="Setup steps">
      {#each steps as [number, label]}
        <button type="button" aria-current={step === number ? 'step' : undefined} onclick={() => go(Number(number))}>
          <span class="marker" class:done={step > Number(number)}>{step > Number(number) ? '✓' : number}</span>{label}
        </button>
      {/each}
    </nav>

    <div class="step-panel">
      {#if step === 1}
        <header>
          <p class="eyebrow">Step 1 · Install</p>
          <h3 tabindex="-1" bind:this={panelHeading}>Put the CLI on the machine.</h3>
          <p>Install the reet binary once. It includes the scanner and the machine agent. No separate server is needed on this machine.</p>
        </header>
        <div class="fields">
          <label>Machine name<input bind:this={nameInput} bind:value={host} list="known-machine-names" maxlength="200" placeholder="e.g. studio-laptop" autocomplete="off" aria-invalid={nameError ? 'true' : undefined} aria-describedby={nameError ? 'machine-name-error' : undefined} oninput={() => (nameError = '')} /></label>
          <label>Release<select bind:value={channel} onchange={() => (channelTouched = true)}><option value="stable">Latest stable</option><option value="dev">Dev — latest main</option></select></label>
        </div>
        <datalist id="known-machine-names">{#each machines as machine}<option value={machine.host}></option>{/each}</datalist>
        <div class="command">
          <div class="command-copy"><span>Install and check</span><CopyButton text={install} label="Copy install command" compact /></div>
          <pre>{install}</pre>
        </div>
        {#if nameError}<p class="name-error" id="machine-name-error" role="alert">{nameError}</p>{/if}
        <p class="hint">Required. The machine name is how this machine appears here. Keep it stable between scans.</p>
        <div class="step-actions"><button class="btn primary" type="button" onclick={() => continueTo(2)}>Continue to {mode === 'scratch' ? 'setup restore' : 'authentication'} →</button></div>
      {:else if step === 2 && mode === 'connect'}
        <header>
          <p class="eyebrow">Step 2 · Authenticate</p>
          <h3 tabindex="-1" bind:this={panelHeading}>Give this machine its own key.</h3>
          <p>Create a machine token here, then save the server connection on the machine. The token is shown once and is never stored in the browser.</p>
        </header>
        {#if tokenSetup}{@render tokenSetup()}{/if}
        {#if token}
          <details class="one-time"><summary>Show the one-time machine token</summary><code>{token}</code><CopyButton text={token} label="Copy machine token" compact /></details>
          <p class="hint">When the CLI prompts for a token, paste this value. It is not embedded in the command.</p>
        {/if}
        <div class="command">
          <div class="command-copy"><span>Save the connection</span><CopyButton text={configure} label="Copy connection command" compact /></div>
          <pre>{configure}</pre>
        </div>
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(1)}>Back</button><button class="btn primary" type="button" onclick={() => continueTo(3)}>Continue to terminal connection →</button></div>
      {:else if step === 2}
        <header>
          <p class="eyebrow">Step 2 · Restore setup</p>
          <h3 tabindex="-1" bind:this={panelHeading}>Bring the machine's setup with you.</h3>
          <p>Pull a setup profile from this server, fetch its encrypted snapshot, then restore it. Reet restores settings, skills and supported memory; it does not install the agent applications themselves.</p>
        </header>
        {#if profiles.length === 0}
          <div class="empty-choice">
            <strong>No setup profiles yet.</strong>
            <p>Create one on a machine that is already set up, then come back here.</p>
            <a class="btn" href="/profiles">Open Setups →</a>
          </div>
        {:else}
          <label>Setup profile<select bind:value={profileName}>{#each profiles as profile}<option value={profile.name}>{profile.name} · revision {profile.revision}</option>{/each}</select></label>
          <div class="command">
            <div class="command-copy"><span>Pull the profile metadata</span><CopyButton text={pull} label="Copy profile pull command" compact /></div>
            <pre>{pull}</pre>
          </div>
          <div class="command">
            <div class="command-copy"><span>Fetch the encrypted snapshot</span><CopyButton text={fetch} label="Copy snapshot fetch command" compact /></div>
            <pre>{fetch}</pre>
          </div>
          <p class="hint">Syncing a profile does not upload the snapshot. Use your configured S3 destination, or copy the snapshot securely from the source machine.</p>
          <div class="command">
            <div class="command-copy"><span>Preview the restore</span><CopyButton text={preview} label="Copy restore preview" compact /></div>
            <pre>{preview}</pre>
          </div>
          <div class="command">
            <div class="command-copy"><span>Apply after reviewing</span><CopyButton text={apply} label="Copy restore apply command" compact /></div>
            <pre>{apply}</pre>
          </div>
          <details class="advanced"><summary>Verify the restored files</summary><div class="command"><div class="command-copy"><span>Verify</span><CopyButton text={verify} label="Copy verify command" compact /></div><pre>{verify}</pre></div><p class="hint">Keep the journal for rollback. A file match does not prove the agents work.</p></details>
        {/if}
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(1)}>Back</button><button class="btn primary" type="button" onclick={() => continueTo(3)}>Continue to terminal connection →</button></div>
      {:else if step === 3}
        <header>
          <p class="eyebrow">Step 3 · Machine &amp; terminal connection</p>
          <h3 tabindex="-1" bind:this={panelHeading}>Bring this machine into your workspace.</h3>
          <p>Enroll once. The CLI offers a terminal wizard for local shell consent, tmux 3.2+, independent services and connection checks.</p>
        </header>
        {#if mode === 'scratch' && tokenSetup}{@render tokenSetup()}{/if}
        {#if terminalAccess}
          <div class="server-terminal" class:ready={serverTerminal?.available}>
            <div>
              <strong>Terminal access on this server</strong>
              <p>{serverTerminal === undefined ? 'Reading the server’s terminal status…' : controllerDetail(serverTerminal)}</p>
            </div>
            {#if serverTerminal && !serverTerminal.available}
              <div class="command"><div class="command-copy"><span>Allow terminals on this server</span><CopyButton text="reet terminal start" label="Copy terminal start" compact /></div><pre>reet terminal start</pre></div>
            {/if}
          </div>
        {/if}
        <label class="check"><input type="checkbox" bind:checked={allowApply} /> Allow this machine to apply sync plans I approve here</label>
        <div class="command">
          <div class="command-copy"><span>Enroll and connect</span><CopyButton text={enroll} label="Copy enrollment command" compact /></div>
          <pre>{enroll}</pre>
        </div>
        <p class="hint">Run these commands in this machine’s terminal. When prompted, use its personal API token. Shell access stays disabled until you consent locally; declining still allows inventory reporting.</p>

        <div class="watch" class:ready={terminalReady} class:problem={!!terminalError || stage === 'needs-setup'}>
          <p class="vh" role="status" aria-live="polite">Terminal connection: {watchTitle}{terminalError ? ` ${terminalError}` : ''}</p>
          <div class="watch-head">
            <span class="pulse" class:on={terminalReady} class:warn={!!terminalError || stage === 'needs-setup'} aria-hidden="true"></span>
            <div class="watch-copy">
              <strong>{watchTitle}</strong>
              <p>{watchDetail}</p>
            </div>
            <span class="watch-badge">{terminalReady ? 'Ready' : stage === 'blocked' ? 'Blocked' : stage === 'needs-setup' ? 'Setup needed' : stage === 'reconnecting' ? 'Reconnecting' : 'Watching'}</span>
          </div>
          <dl class="watch-facts">
            <div><dt>Enrollment</dt><dd>{terminalHost ? 'Detected' : 'Not yet'}</dd></div>
            <div><dt>Agent</dt><dd>{enrolledAgent ? `reet ${agentBuild(enrolledAgent)}` : 'Waiting'}</dd></div>
            <div><dt>Terminal</dt><dd>{terminalReady ? 'Ready' : stage === 'needs-setup' ? 'Needs local setup' : stage === 'blocked' ? 'Unavailable' : stage === 'reconnecting' ? 'Reconnecting' : 'Waiting'}</dd></div>
            <div><dt>Last check</dt><dd>{ago(lastCheckedAt)}{checkFailures ? ` · retry ${checkFailures}` : ''}</dd></div>
          </dl>
          {#if terminalError}<p class="watch-error" role="alert">{terminalError} {checkFailures > 1 ? 'Still trying automatically.' : 'Trying again shortly.'}</p>{/if}
          <div class="watch-actions">
            <button class="btn" onclick={check} disabled={checking || !host.trim()}>{checking ? 'Checking…' : 'Check terminal connection'}</button>
            {#if terminalReady && terminalHost}<a class="btn primary" href={`/sessions?agent=${encodeURIComponent(terminalHost.agentId)}`}>Open terminals for {terminalHost.host} →</a>{/if}
          </div>
        </div>

        {#if enrolledAgent}
          <div class="agent-update" aria-live="polite">
            <div class="agent-update-copy">
              <span class="eyebrow">Enrolled agent</span>
              <strong>reet {agentBuild(enrolledAgent)}</strong>
              <p>{updateReady ? 'A newer build is available for this machine.' : latest?.error ? 'Release check failed — the machine is still connected.' : latest ? `Latest ${latest.label}. This machine is current.` : 'Release information is loading from the controller.'}</p>
            </div>
            <button class="btn" class:primary={updateReady} type="button" onclick={updateAgent} disabled={updateBusy || enrolledAgent.paused}>{updateBusy ? 'Queueing update…' : updateReady ? `Update to ${agentUpdateTarget(enrolledAgent)}` : 'Update reet'}</button>
          </div>
          {#if updateJob}<p class="hint update-state" class:bad={updateJob.status === 'failed'} class:good={updateJob.status === 'succeeded'}>{updateJobLabel(updateJob)}</p>{/if}
          {#if updateNotice}<p class="hint" role="status">{updateNotice}</p>{/if}
          {#if updateError}<p class="inline-warning" role="alert">{updateError}</p>{/if}
        {/if}

        <details class="advanced"><summary>Already enrolled, or setup was interrupted?</summary><div class="command"><div class="command-copy"><span>Resume terminal setup</span><CopyButton text="reet agent terminal setup" label="Copy terminal setup wizard" compact /></div><pre>reet agent terminal setup</pre></div><p class="hint">This reuses the enrollment. The wizard diagnoses missing tmux and offers service repair. Enable browser terminals on the controller with <code>reet terminal start</code> if needed.</p></details>
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(2)}>Back</button><button class="btn primary" type="button" onclick={() => go(4)}>Continue to optional inventory &amp; sync →</button></div>
      {:else}
        <header>
          <p class="eyebrow">Step 4 · Inventory &amp; sync</p>
          <h3 tabindex="-1" bind:this={panelHeading}>Tell reet.sh what is on this machine.</h3>
          <p>Run the scan on the machine. It reads the agent skill folders and creates the machine inventory here.</p>
        </header>
        {#if !host.trim()}<p class="inline-warning">Add a machine name in step 1 first, so the scan and future reports use a stable name.</p>{/if}
        <div class="command">
          <div class="command-copy"><span>Scan this machine</span><CopyButton text={scan} label="Copy scan command" compact /></div>
          <pre>{scan}</pre>
        </div>

        {#if connected}
          <div class="joined">
            <div class="joined-head">
              <span class="joined-mark" aria-hidden="true">✓</span>
              <div><strong>{connected.host} is connected.</strong><p>A recent, complete inventory is here. This page keeps checking while it stays open.</p></div>
              <span class="watch-badge ready">Connected</span>
            </div>
            <dl class="watch-facts">
              <div><dt>Agent scopes</dt><dd>{agentGroups(connected).length}</dd></div>
              <div><dt>Skills found</dt><dd>{connected.skillCount}</dd></div>
              <div><dt>Last report</dt><dd>{ago(lastCheckedAt)}</dd></div>
              <div><dt>Agent</dt><dd>{enrolledAgent ? `reet ${agentBuild(enrolledAgent)}` : 'Reported'}</dd></div>
            </dl>
            <div class="joined-actions">
              <a class="btn primary" href={syncLink(connected.host)}>Open compare &amp; sync for {connected.host} →</a>
              {#if terminalReady && terminalHost}<a class="btn" href={`/sessions?agent=${encodeURIComponent(terminalHost.agentId)}`}>Open terminals →</a>{/if}
            </div>
          </div>
        {:else}
          <div class="watch" aria-live="polite">
            <div class="watch-head">
              <span class="pulse" aria-hidden="true"></span>
              <div class="watch-copy">
                <strong>Waiting for the first scan</strong>
                <p>Run the scan above. This page checks automatically and fills the machine in the moment its report lands.</p>
              </div>
              <span class="watch-badge">Watching</span>
            </div>
            <dl class="watch-facts">
              <div><dt>Enrollment</dt><dd>{terminalHost || enrolledAgent ? 'Detected' : 'Not yet'}</dd></div>
              <div><dt>Inventory</dt><dd>Waiting</dd></div>
              <div><dt>Last check</dt><dd>{ago(lastCheckedAt)}</dd></div>
              <div><dt>Agent</dt><dd>{enrolledAgent ? `reet ${agentBuild(enrolledAgent)}` : 'Waiting'}</dd></div>
            </dl>
            <div class="watch-actions">
              <button class="btn" type="button" onclick={check} disabled={checking || !host.trim()}>{checking ? 'Checking…' : 'Check for this machine'}</button>
            </div>
          </div>
        {/if}
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(3)}>Back</button><a class="btn quiet" href="/profiles">Setups</a></div>
      {/if}
    </div>

    <details class="advanced more">
      <summary>More commands and diagnostics</summary>
      <div class="command"><div class="command-copy"><span>Upgrade and diagnose</span><CopyButton text={upgrade} label="Copy upgrade commands" compact /></div><pre>{upgrade}</pre></div>
      {#if serverBuild}<p class="hint">This site is running {serverBuild.version} ({serverBuild.commit.slice(0, 12)}). Your CLI has its own version; check it with <code>reet --version</code>.</p>{/if}
    </details>
  </div>
</details>

<style>
  .setup { border: 1px solid var(--line); border-radius: 16px; background: var(--card); box-shadow: var(--shadow); margin: 18px 0 26px; }
  .setup > summary { list-style: none; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px 22px; cursor: pointer; min-height: 64px; }
  .setup > summary::-webkit-details-marker { display: none; }
  .setup > summary > span:first-child { display: grid; gap: 3px; }
  .setup > summary strong { font: 500 1.35rem/1.2 var(--serif); }
  .setup > summary small { color: var(--muted); font-size: 0.82rem; }
  .summary-state { flex: none; padding: 5px 10px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.74rem; }
  .chevron { flex: none; width: 9px; height: 9px; margin-right: 4px; border-right: 2px solid var(--muted); border-bottom: 2px solid var(--muted); transform: rotate(-45deg); transition: transform 160ms ease; }
  .setup[open] .chevron { transform: rotate(45deg); }
  .setup[open] > summary { border-bottom: 1px solid var(--line); }
  .setup-body { padding: 20px 22px 24px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.66rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .modes { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .modes button { display: flex; gap: 12px; align-items: flex-start; text-align: left; padding: 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); color: var(--ink); cursor: pointer; }
  .modes button:hover { border-color: var(--muted); }
  .modes button.active { border-color: var(--accent); background: var(--tint); box-shadow: inset 0 0 0 1px var(--accent); }
  .modes button > span:last-child { display: grid; gap: 3px; }
  .modes strong { font-size: 0.88rem; }
  .modes small { color: var(--muted); font-size: 0.78rem; line-height: 1.45; }
  .mode-icon { display: grid; place-items: center; width: 30px; height: 30px; flex: none; border-radius: 50%; background: var(--card); color: var(--accent-text); }
  .mode-icon svg { width: 16px; height: 16px; }
  .steps { display: flex; flex-wrap: wrap; gap: 4px; margin: 18px 0; padding: 5px; border: 1px solid var(--line); border-radius: 14px; background: var(--bg); }
  .steps button { display: inline-flex; align-items: center; gap: 7px; min-height: 38px; padding: 6px 12px; border: 1px solid transparent; border-radius: 10px; background: none; color: var(--muted); font: inherit; font-size: 0.8rem; cursor: pointer; }
  .steps .marker { display: grid; place-items: center; width: 21px; height: 21px; border: 1px solid var(--line); border-radius: 50%; font: 0.66rem var(--mono); }
  .steps .marker.done { background: color-mix(in srgb, var(--good) 16%, var(--card)); border-color: color-mix(in srgb, var(--good) 45%, var(--line)); color: var(--good); }
  .steps button:hover { color: var(--ink); background: var(--card); }
  .steps button[aria-current='step'] { color: var(--ink); background: var(--card); border-color: var(--line); font-weight: 650; box-shadow: 0 1px 2px rgb(0 0 0 / 0.04); }
  .steps button[aria-current='step'] .marker { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); font-weight: 700; }
  .step-panel { display: grid; gap: 14px; }
  .step-panel header { max-width: 70ch; }
  h3 { margin: 6px 0 0; font: 500 clamp(1.3rem, 2.4vw, 1.65rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  h3:focus-visible { outline: 2px solid var(--accent); outline-offset: 5px; border-radius: 4px; }
  .step-panel header p:last-child { margin: 8px 0 0; color: var(--muted); font-size: 0.86rem; line-height: 1.6; }
  .fields { display: grid; grid-template-columns: minmax(0, 1fr) 190px; gap: 12px; }
  label { display: grid; gap: 6px; color: var(--ink); font-size: 0.82rem; font-weight: 600; }
  /* No overflow clip on the setup card or its command boxes: they hold buttons, and a clip would cut them off instead of letting them wrap. */
  .command { display: grid; gap: 0; min-width: 0; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .command-copy { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 9px 12px; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.72rem; font-weight: 650; letter-spacing: 0.04em; text-transform: uppercase; }
  .command pre { margin: 0; padding: 13px 14px; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.78rem/1.6 var(--mono); }
  .hint { margin: 0; color: var(--muted); font-size: 0.78rem; line-height: 1.55; }
  .name-error { margin: 6px 0 0; color: var(--bad); font-size: 0.84rem; }
  .inline-warning { margin: 0; padding: 9px 11px; border: 1px solid color-mix(in srgb, var(--bad) 35%, var(--line)); border-radius: 8px; color: var(--bad); background: color-mix(in srgb, var(--bad) 6%, var(--card)); font-size: 0.8rem; }
  .step-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding-top: 4px; }
  .step-actions .primary { margin-left: auto; }
  .one-time { border: 1px solid var(--line); border-radius: 10px; padding: 12px; background: var(--bg); }
  .one-time code { display: block; margin: 10px 0; overflow-wrap: anywhere; }
  .empty-choice { padding: 18px; border: 1px dashed var(--line); border-radius: 12px; background: var(--bg); }
  .empty-choice p { margin: 6px 0 12px; color: var(--muted); font-size: 0.84rem; }
  .advanced { border: 1px solid var(--line); border-radius: 10px; padding: 12px 14px; background: var(--bg); }
  .advanced > summary { cursor: pointer; font-size: 0.84rem; font-weight: 600; }
  .advanced > * + * { margin-top: 10px; }
  .check { display: flex; align-items: center; gap: 9px; font-weight: 500; }
  .server-terminal { display: grid; gap: 10px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .server-terminal strong { font-size: 0.84rem; }
  .server-terminal p { margin: 3px 0 0; color: var(--muted); font-size: 0.8rem; line-height: 1.5; }
  .server-terminal.ready { border-color: color-mix(in srgb, var(--good) 40%, var(--line)); }
  .check input { flex: none; }

  .watch, .joined { display: grid; gap: 12px; padding: 15px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  .watch.ready, .joined { border-color: color-mix(in srgb, var(--good) 45%, var(--line)); background: color-mix(in srgb, var(--good) 6%, var(--card)); }
  .watch.problem { border-color: color-mix(in srgb, var(--warn) 42%, var(--line)); background: color-mix(in srgb, var(--warn) 5%, var(--card)); }
  .watch-head, .joined-head { display: flex; align-items: flex-start; gap: 11px; }
  .watch-copy { display: grid; gap: 4px; min-width: 0; flex: 1; }
  .watch-copy strong, .joined-head strong { font-size: 0.9rem; }
  .watch-copy p, .joined-head p { margin: 0; color: var(--muted); font-size: 0.8rem; line-height: 1.55; }
  .watch-badge { flex: none; padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.68rem; }
  .watch.ready .watch-badge, .watch-badge.ready { color: var(--good); border-color: color-mix(in srgb, var(--good) 38%, var(--line)); }
  .watch.problem .watch-badge { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 38%, var(--line)); }
  .pulse { width: 9px; height: 9px; margin-top: 5px; flex: none; border-radius: 50%; background: var(--muted); }
  .pulse.on { background: var(--good); box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 18%, transparent); }
  .pulse.warn { background: var(--warn); box-shadow: 0 0 0 3px color-mix(in srgb, var(--warn) 18%, transparent); }
  @media (prefers-reduced-motion: no-preference) {
    .pulse:not(.on):not(.warn) { animation: breathe 2.4s var(--ease-out) infinite; }
    @keyframes breathe { 0%, 100% { opacity: 0.45; } 50% { opacity: 1; } }
  }
  .watch-facts { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin: 0; padding: 11px 0 0; border-top: 1px solid color-mix(in srgb, var(--line) 75%, transparent); }
  .watch-facts div { display: grid; gap: 2px; min-width: 0; }
  .watch-facts dt { color: var(--muted); font-size: 0.66rem; letter-spacing: 0.04em; text-transform: uppercase; }
  .watch-facts dd { margin: 0; font-size: 0.79rem; overflow-wrap: anywhere; }
  .watch-error { margin: 0; color: var(--bad); font-size: 0.78rem; line-height: 1.5; }
  .watch-actions, .joined-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
  .joined-head > div { min-width: 0; flex: 1; }
  .joined-mark { display: grid; place-items: center; width: 26px; height: 26px; flex: none; border-radius: 50%; background: var(--good); color: var(--card); font-size: 0.8rem; }
  .agent-update { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 13px 15px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .agent-update-copy { display: grid; gap: 3px; min-width: 0; }
  .agent-update-copy strong { font-size: 0.86rem; }
  .agent-update-copy p { margin: 0; color: var(--muted); font-size: 0.77rem; line-height: 1.5; }
  .agent-update .btn { flex: none; }
  .update-state.bad { color: var(--bad); }
  .update-state.good { color: var(--good); }
  .more { margin-top: 6px; }

  @media (max-width: 760px) {
    .watch-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @media (max-width: 700px) {
    .setup > summary { padding: 16px; align-items: flex-start; }
    .summary-state { display: none; }
    .chevron { margin-left: auto; }
    .setup-body { padding: 16px; }
    .modes { grid-template-columns: 1fr; }
    .fields { grid-template-columns: 1fr; }
    /* Two columns when each step fits, one on the narrowest phones; a plain 1fr column grew past the card instead. */
    .steps { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 136px), 1fr)); }
    .steps button { justify-content: flex-start; min-width: 0; text-align: left; }
    .command-copy { flex-direction: column; align-items: stretch; }
    .command-copy :global(button) { width: 100%; white-space: normal; }
    .step-actions .primary { margin-left: 0; flex: 1 1 100%; }
    .watch-head, .agent-update { align-items: stretch; flex-direction: column; }
    .watch-badge { align-self: flex-start; }
    .watch-actions .btn, .agent-update .btn, .joined-actions .btn { width: 100%; }
  }
</style>
