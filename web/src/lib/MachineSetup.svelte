<script lang="ts">
  import type { Snippet } from 'svelte'
  import {terminalMachines,readiness,type TerminalMachine} from './terminal/api'
  import type { Machine, SetupProfileSummary } from './api'
  import { agentGroups } from './machines'
  import {
    configureCommand, enrollCommands, installCommands, profileFetchCommand, profilePullCommand,
    profileRestoreCommand, profileVerifyCommand, scanCommand, syncLink, upgradeCommands,
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
    open = $bindable(false),
    initialMode = 'connect',
    terminalAccess=false,
    onrefresh,
  }: {
    site: string
    machines: Machine[]
    profiles?: SetupProfileSummary[]
    token?: string
    tokenSetup?: Snippet
    serverBuild?: { version: string; commit: string } | null
    open?: boolean
    initialMode?: 'connect' | 'scratch'
    terminalAccess?:boolean
    onrefresh: () => Promise<void> | void
  } = $props()

  type Mode = 'connect' | 'scratch'
  // svelte-ignore state_referenced_locally
  let mode = $state<Mode>(initialMode)
  let step = $state(1)
  let host = $state('')
  let channel = $state<ReleaseChannel>('stable')
  let channelTouched = $state(false)
  // svelte-ignore state_referenced_locally
  let profileName = $state(profiles[0]?.name ?? '')
  let allowApply = $state(false)
  let checking = $state(false),terminalError=$state(''),terminalHosts=$state<TerminalMachine[]>([])
  const terminalHost=$derived.by(()=>{const matches=terminalHosts.filter(item=>item.host.toLowerCase()===host.trim().toLowerCase());return matches.length===1?matches[0]:undefined})
  const terminalReady=$derived(!!terminalHost?.ready&&!terminalError)

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
    machine.host.toLowerCase() === host.trim().toLowerCase() &&
    agentGroups(machine).some((group) => group.complete && group.scannedAt > Date.now() - 14 * 86400000)))
  const steps = $derived(mode === 'connect'
    ? [[1, 'Install'], [2, 'Authenticate'], [3, 'Connect terminal'], [4, 'Inventory & sync']]
    : [[1, 'Install'], [2, 'Restore setup'], [3, 'Connect terminal'], [4, 'Inventory & sync']])

  $effect(() => {
    if (!profileName && profiles.length > 0) profileName = profiles[0].name
  })
  $effect(() => {
    if (!channelTouched && serverBuild?.version === 'dev' && channel !== 'dev') channel = 'dev'
  })

  async function checkTerminal(signal?:AbortSignal){
    try{const next=await terminalMachines(signal?AbortSignal.any([signal,AbortSignal.timeout(10000)]):AbortSignal.timeout(10000));if(!signal?.aborted){terminalHosts=next;terminalError=''}}catch(e){if(!signal?.aborted){terminalError=e instanceof Error?e.message:'Terminal connection status is unavailable.';terminalHosts=[]}}
  }
  $effect(()=>{if(open&&step===3&&terminalAccess){const request=new AbortController();void checkTerminal(request.signal);const timer=setInterval(()=>{if(document.visibilityState==='visible')void checkTerminal(request.signal)},15000);return()=>{request.abort();clearInterval(timer)}}})

  function chooseMode(next: Mode) {
    mode = next
    step = 1
  }

  function go(next: number) {
    step = next
  }

  async function check() {
    checking = true
    try {
      await onrefresh()
      if(terminalAccess)await checkTerminal()
    } finally {
      checking = false
    }
  }
</script>

<details class="setup" id="connect-machine" bind:open>
  <summary>
    <span>
      <span class="eyebrow">Machine setup</span>
      <strong>Set up a machine</strong>
      <small>Connect your machine and terminal, then bring its AI toolkit into the fleet.</small>
    </span>
    <span class="summary-state">{machines.length ? `${machines.length} ${machines.length === 1 ? 'machine' : 'machines'} connected` : 'Start here'}</span>
    <span class="chevron" aria-hidden="true">›</span>
  </summary>

  <div class="setup-body">
    <div class="modes" role="group" aria-label="Setup type">
      <button type="button" class:active={mode === 'connect'} aria-pressed={mode === 'connect'} onclick={() => chooseMode('connect')}>
        <span class="mode-icon" aria-hidden="true">↻</span>
        <span><strong>Connect an existing machine</strong><small>It already has agents or skills. Connect its terminal, then choose what to sync.</small></span>
      </button>
      <button type="button" class:active={mode === 'scratch'} aria-pressed={mode === 'scratch'} onclick={() => chooseMode('scratch')}>
        <span class="mode-icon" aria-hidden="true">＋</span>
        <span><strong>Start from scratch</strong><small>A new machine. Restore a saved setup, then scan and sync.</small></span>
      </button>
    </div>

    <nav class="steps" aria-label="Setup steps">
      {#each steps as [number, label]}
        <button type="button" aria-current={step === number ? 'step' : undefined} onclick={() => go(Number(number))}>
          <span>{number}</span>{label}
        </button>
      {/each}
    </nav>

    <div class="step-panel">
      {#if step === 1}
        <header>
          <p class="eyebrow">Step 1 · Install</p>
          <h3>Put the CLI on the machine.</h3>
          <p>Install the reet binary once. It includes the scanner and the machine agent. No separate server is needed on this machine.</p>
        </header>
        <div class="fields">
          <label>Machine name<input bind:value={host} list="known-machine-names" maxlength="200" placeholder="e.g. studio-laptop" autocomplete="off" /></label>
          <label>Release<select bind:value={channel} onchange={() => (channelTouched = true)}><option value="stable">Latest stable</option><option value="dev">Dev — latest main</option></select></label>
        </div>
        <datalist id="known-machine-names">{#each machines as machine}<option value={machine.host}></option>{/each}</datalist>
        <div class="command">
          <div class="command-copy"><span>Install and check</span><CopyButton text={install} label="Copy install command" /></div>
          <pre>{install}</pre>
        </div>
        <p class="hint">The machine name is how this machine appears here. Keep it stable between scans.</p>
        <div class="step-actions"><button class="btn primary" type="button" onclick={() => go(2)} disabled={!host.trim()}>Continue to {mode === 'scratch' ? 'setup restore' : 'authentication'} →</button></div>
      {:else if step === 2 && mode === 'connect'}
        <header>
          <p class="eyebrow">Step 2 · Authenticate</p>
          <h3>Give this machine its own key.</h3>
          <p>Create a machine token here, then save the server connection on the machine. The token is shown once and is never stored in the browser.</p>
        </header>
        {#if tokenSetup}{@render tokenSetup()}{/if}
        {#if token}
          <details class="one-time"><summary>Show the one-time machine token</summary><code>{token}</code><CopyButton text={token} label="Copy machine token" /></details>
          <p class="hint">When the CLI prompts for a token, paste this value. It is not embedded in the command.</p>
        {/if}
        <div class="command">
          <div class="command-copy"><span>Save the connection</span><CopyButton text={configure} label="Copy connection command" /></div>
          <pre>{configure}</pre>
        </div>
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(1)}>Back</button><button class="btn primary" type="button" onclick={() => go(3)} disabled={!host.trim()}>Continue to terminal connection →</button></div>
      {:else if step === 2}
        <header>
          <p class="eyebrow">Step 2 · Restore setup</p>
          <h3>Bring the machine's setup with you.</h3>
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
            <div class="command-copy"><span>Pull the profile metadata</span><CopyButton text={pull} label="Copy profile pull command" /></div>
            <pre>{pull}</pre>
          </div>
          <div class="command">
            <div class="command-copy"><span>Fetch the encrypted snapshot</span><CopyButton text={fetch} label="Copy snapshot fetch command" /></div>
            <pre>{fetch}</pre>
          </div>
          <p class="hint">Syncing a profile does not upload the snapshot. Use your configured S3 destination, or copy the snapshot securely from the source machine.</p>
          <div class="command">
            <div class="command-copy"><span>Preview the restore</span><CopyButton text={preview} label="Copy restore preview" /></div>
            <pre>{preview}</pre>
          </div>
          <div class="command">
            <div class="command-copy"><span>Apply after reviewing</span><CopyButton text={apply} label="Copy restore apply command" /></div>
            <pre>{apply}</pre>
          </div>
          <details class="advanced"><summary>Verify the restored files</summary><div class="command"><div class="command-copy"><span>Verify</span><CopyButton text={verify} label="Copy verify command" /></div><pre>{verify}</pre></div><p class="hint">Keep the journal for rollback. A file match does not prove the agents work.</p></details>
        {/if}
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(1)}>Back</button><button class="btn primary" type="button" onclick={() => go(3)} disabled={!host.trim()}>Continue to terminal connection →</button></div>
      {:else if step===3}
        <header><p class="eyebrow">Step 3 · Machine &amp; terminal connection</p><h3>Bring this machine into your workspace.</h3><p>Enroll once. The CLI offers a terminal wizard for local shell consent, tmux 3.2+, independent services and connection checks.</p></header>
        {#if mode==='scratch'&&tokenSetup}{@render tokenSetup()}{/if}
        <label class="check"><input type="checkbox" bind:checked={allowApply}/> Allow this machine to apply sync plans I approve here</label>
        <div class="command"><div class="command-copy"><span>Enroll and connect</span><CopyButton text={enroll} label="Copy enrollment command"/></div><pre>{enroll}</pre></div>
        <p class="hint">Run these commands in this machine’s terminal. When prompted, use its personal API token. Shell access stays disabled until you consent locally; declining still allows inventory reporting.</p>
        <details class="advanced"><summary>Already enrolled, or setup was interrupted?</summary><div class="command"><div class="command-copy"><span>Resume terminal setup</span><CopyButton text="reet agent terminal setup" label="Copy terminal setup wizard"/></div><pre>reet agent terminal setup</pre></div><p class="hint">This reuses the enrollment. The wizard diagnoses missing tmux and offers service repair. Enable browser terminals on the controller with <code>reet terminal start</code> if needed.</p></details>
        <div class="check-state" class:ready={terminalReady}><div><strong>{terminalReady?`${terminalHost?.host} is ready for terminals.`:'Waiting for terminal readiness'}</strong><p>{terminalError||(!terminalAccess?'Browser terminals require account sign-in with a write role.':terminalHost?readiness(terminalHost):'Readiness comes from this enrolled machine’s live connection; a completed scan alone does not establish it.')}</p></div><button class="btn" onclick={check} disabled={checking||!host.trim()}>{checking?'Checking…':'Check terminal connection'}</button></div>
        {#if terminalReady&&terminalHost}<a class="btn primary" href={`/terminals?agent=${encodeURIComponent(terminalHost.agentId)}`}>Open terminals for {terminalHost.host} →</a>{/if}
        <div class="step-actions"><button class="btn" type="button" onclick={()=>go(2)}>Back</button><button class="btn primary" type="button" onclick={()=>go(4)}>Continue to optional inventory &amp; sync →</button></div>
      {:else}
        <header>
          <p class="eyebrow">Step 4 · Inventory &amp; sync</p>
          <h3>Tell reet.sh what is on this machine.</h3>
          <p>Run the scan on the machine. It reads the agent skill folders and creates the machine inventory here.</p>
        </header>
        {#if !host.trim()}<p class="inline-warning">Add a machine name in step 1 first, so the scan and future reports use a stable name.</p>{/if}
        <div class="command">
          <div class="command-copy"><span>Scan this machine</span><CopyButton text={scan} label="Copy scan command" /></div>
          <pre>{scan}</pre>
        </div>
        <div class="check-state" class:ready={!!connected}>
          <div>
            <strong>{connected ? `${connected.host} is connected.` : 'Waiting for the first scan'}</strong>
            <p>{connected ? 'A recent, complete inventory is here. Choose what to sync next.' : 'Run the scan above, then check for this machine. You can also leave this page and check later.'}</p>
          </div>
          <button class="btn" type="button" onclick={check} disabled={checking || !host.trim()}>{checking ? 'Checking…' : 'Check for this machine'}</button>
        </div>
        {#if connected}
          <a class="btn primary sync-next" href={syncLink(connected.host)}>Open compare &amp; sync for {connected.host} →</a>
        {/if}
        <div class="step-actions"><button class="btn" type="button" onclick={() => go(3)}>Back</button><a class="btn quiet" href="/profiles">Setups</a></div>
      {/if}
    </div>

    <details class="advanced more">
      <summary>More commands and diagnostics</summary>
      <div class="command"><div class="command-copy"><span>Upgrade and diagnose</span><CopyButton text={upgrade} label="Copy upgrade commands" /></div><pre>{upgrade}</pre></div>
      {#if serverBuild}<p class="hint">This site is running {serverBuild.version} ({serverBuild.commit.slice(0, 12)}). Your CLI has its own version; check it with <code>reet --version</code>.</p>{/if}
    </details>
  </div>
</details>

<style>
  .setup { border: 1px solid var(--line); border-radius: 16px; background: var(--card); box-shadow: var(--shadow); margin: 18px 0 26px; overflow: clip; }
  .setup > summary { list-style: none; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px 22px; cursor: pointer; min-height: 64px; }
  .setup > summary::-webkit-details-marker { display: none; }
  .setup > summary > span:first-child { display: grid; gap: 3px; }
  .setup > summary strong { font: 500 1.35rem/1.2 var(--serif); }
  .setup > summary small { color: var(--muted); font-size: 0.82rem; }
  .summary-state { flex: none; padding: 5px 10px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.74rem; }
  .chevron { flex: none; color: var(--muted); font-size: 1.35rem; line-height: 1; transition: transform 160ms ease; }
  .setup[open] .chevron { transform: rotate(90deg); }
  .setup[open] > summary { border-bottom: 1px solid var(--line); }
  .setup-body { padding: 20px 22px 24px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.66rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .modes { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .modes button { display: flex; gap: 12px; align-items: flex-start; text-align: left; padding: 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); color: var(--ink); cursor: pointer; }
  .modes button:hover { border-color: var(--muted); }
  .modes button.active { border-color: var(--accent); background: var(--tint); box-shadow: inset 0 0 0 1px var(--accent); }
  .modes button > span:last-child { display: grid; gap: 3px; }
  .modes strong { font-size: 0.88rem; }
  .modes small { color: var(--muted); font-size: 0.78rem; line-height: 1.45; }
  .mode-icon { display: grid; place-items: center; width: 30px; height: 30px; flex: none; border-radius: 50%; background: var(--card); color: var(--accent); font-size: 1rem; }
  .steps { display: flex; flex-wrap: wrap; gap: 6px; margin: 18px 0; }
  .steps button { display: inline-flex; align-items: center; gap: 7px; min-height: 38px; padding: 6px 11px; border: 1px solid transparent; border-radius: 999px; background: none; color: var(--muted); font: inherit; font-size: 0.8rem; cursor: pointer; }
  .steps button span { display: grid; place-items: center; width: 20px; height: 20px; border: 1px solid var(--line); border-radius: 50%; font: 0.66rem var(--mono); }
  .steps button:hover { color: var(--ink); background: var(--bg); }
  .steps button[aria-current='step'] { color: var(--ink); background: var(--tint); border-color: var(--line); font-weight: 650; }
  .steps button[aria-current='step'] span { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); }
  .step-panel { display: grid; gap: 14px; }
  .step-panel header { max-width: 70ch; }
  h3 { margin: 6px 0 0; font: 500 clamp(1.3rem, 2.4vw, 1.65rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  .step-panel header p:last-child { margin: 8px 0 0; color: var(--muted); font-size: 0.86rem; line-height: 1.6; }
  .fields { display: grid; grid-template-columns: minmax(0, 1fr) 190px; gap: 12px; }
  label { display: grid; gap: 6px; color: var(--ink); font-size: 0.82rem; font-weight: 600; }
  .command { display: grid; gap: 0; min-width: 0; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); overflow: hidden; }
  .command-copy { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 9px 12px; border-bottom: 1px solid var(--line); color: var(--muted); font-size: 0.72rem; font-weight: 650; letter-spacing: 0.04em; text-transform: uppercase; }
  .command pre { margin: 0; padding: 13px 14px; white-space: pre-wrap; overflow-wrap: anywhere; font: 0.78rem/1.6 var(--mono); }
  .hint { margin: 0; color: var(--muted); font-size: 0.78rem; line-height: 1.55; }
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
  .check input { flex: none; }
  .check-state { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .check-state.ready { border-color: color-mix(in srgb, var(--good) 45%, var(--line)); background: color-mix(in srgb, var(--good) 7%, var(--card)); }
  .check-state strong { font-size: 0.88rem; }
  .check-state p { margin: 4px 0 0; color: var(--muted); font-size: 0.8rem; line-height: 1.5; }
  .sync-next { justify-self: start; }
  .more { margin-top: 6px; }
  @media (max-width: 700px) {
    .setup > summary { padding: 16px; align-items: flex-start; }
    .summary-state { display: none; }
    .chevron { margin-left: auto; }
    .setup-body { padding: 16px; }
    .modes { grid-template-columns: 1fr; }
    .fields { grid-template-columns: 1fr; }
    .command-copy { flex-direction: column; align-items: stretch; }
    .command-copy :global(button) { width: 100%; }
    .step-actions .primary { margin-left: 0; flex: 1 1 100%; }
    .check-state { align-items: stretch; flex-direction: column; }
    .check-state .btn { width: 100%; }
  }
</style>
