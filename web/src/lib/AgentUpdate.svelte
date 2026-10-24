<script lang="ts">
  import type { ReleaseInfo, SyncAgent, SyncJob } from './api'
  import { agentBuild, agentUpdateState, shortCommit, updateAvailable, updateProgress, updateStateLabel } from './machines'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let {
    agent,
    job,
    releases,
    canWrite,
    busy,
    onupdate,
    onpolicy,
    oncancel,
  }: {
    agent: SyncAgent
    job?: SyncJob
    releases?: ReleaseInfo
    canWrite: boolean
    busy: boolean
    onupdate: (version: string) => void
    onpolicy: (enabled: boolean) => void
    oncancel: () => void
  } = $props()

  // An old failed attempt is history, not the machine's current state. The
  // panel speaks about the machine as it is now; a day-old outcome would
  // read as if it had just happened.
  const relevant = $derived(job && (job.status === 'queued' || job.status === 'running' || Date.now() - job.updatedAt < 86400000) ? job : undefined)
  const phase = $derived(agentUpdateState(agent, releases, relevant))
  const stages = $derived((relevant?.stages ?? []).map((stage) =>
    relevant?.status === 'failed' && stage.status === 'running' ? { ...stage, status: 'failed' as const } : stage,
  ))
  const progress = $derived(updateProgress(relevant))
  const onDev = $derived(agent.version === 'dev')
  const stableLabel = $derived(releases?.latest?.version ?? '')
  const devLabel = $derived(releases?.dev?.commit ? `dev · ${shortCommit(releases.dev.commit)}` : releases?.dev?.version === 'dev' ? 'dev' : '')
  const checkError = $derived(releases?.latest?.error || releases?.dev?.error || '')
  const currentTarget = $derived(onDev ? devLabel : stableLabel)
  const otherTarget = $derived(onDev ? stableLabel : devLabel)
  const targetBuild = $derived(currentTarget || otherTarget || (onDev ? 'dev' : 'latest'))
  // The last job names the channel it was for. A person may have switched
  // channels, so the attempt's target is not always the running channel's.
  const jobTarget = $derived(
    relevant?.revision === 'dev'
      ? devLabel || 'the development build'
      : relevant?.revision === 'latest'
        ? stableLabel || 'the latest release'
        : targetBuild,
  )
  const shownTarget = $derived(phase === 'available' || phase === 'current' ? targetBuild : jobTarget)
  // A move to the other channel is a switch, as the dialog says; it is not
  // the ordinary update the panel otherwise describes.
  const switching = $derived(!!relevant && (relevant.revision === 'dev') !== onDev)
  const updateWaiting = $derived(updateAvailable(agent, releases))
  // The controller only names a channel; the agent resolves it from GitHub. A
  // failed release check therefore blocks automatic updates, not a person's
  // deliberate one.
  const canReview = $derived(canWrite && !busy && !agent.paused)

  let choosing = $state(false)
  let target = $state('')

  function openReview() {
    target = onDev ? 'dev' : 'latest'
    choosing = true
  }

  function confirm() {
    const chosen = target
    choosing = false
    onupdate(chosen)
  }

  function targetLabel(value: string): string {
    return value === 'dev' ? devLabel || 'the development build' : stableLabel || 'the latest release'
  }

  const tone = $derived(
    phase === 'failed'
      ? 'bad'
      : phase === 'rolled_back'
        ? 'warn'
        : phase === 'queued' || phase === 'running'
          ? 'busy'
          : phase === 'available'
            ? 'accent'
            : phase === 'current'
              ? 'good'
              : 'quiet',
  )

  const explanation = $derived.by(() => {
    if (agent.paused) return 'Sync is paused for this machine, so updates wait until it is resumed.'
    if (phase === 'running') return `The agent is ${switching ? 'switching to' : 'installing'} ${jobTarget}. It keeps the current binary and restarts into the verified one.`
    if (phase === 'queued') return 'The update is queued. The machine will collect it on its next check and verify the release itself.'
    if (phase === 'rolled_back') return `The new build did not report back, so the previous binary is running again. ${switching ? 'The channel is unchanged. ' : ''}Nothing else on the machine was changed.`
    if (phase === 'cancelled') return `The update was cancelled. The machine stays on ${agentBuild(agent)}.`
    if (phase === 'failed') return `The last attempt did not finish. The machine is still on ${agentBuild(agent)}.`
    if (phase === 'unknown') return checkError ? `The release channel could not be checked: ${checkError}. Automatic updates wait until it is reachable.` : 'The release channel has not been checked yet.'
    if (phase === 'available' && agent.autoUpdate) return `Automatic updates are on. While this machine is online, the controller will install ${targetBuild} and verify it before reporting back.`
    if (phase === 'available') return `Automatic updates are off. Review ${targetBuild} when you are ready; the agent verifies the release and keeps the current binary.`
    return `Running the newest build on the ${onDev ? 'development' : 'release'} channel.`
  })

  function pillLabel(): string {
    if (!switching) return updateStateLabel(phase)
    if (phase === 'failed') return 'Switch failed'
    if (phase === 'rolled_back') return 'Switch rolled back'
    if (phase === 'cancelled') return 'Switch cancelled'
    if (phase === 'queued' || phase === 'running') return 'Switching'
    return updateStateLabel(phase)
  }

  function outcomeTitle(): string {
    if (!relevant) return ''
    if (relevant.status === 'rolled_back') return `${switching ? 'Switch rolled back' : 'Rolled back'} to ${agentBuild(agent)}`
    if (relevant.status === 'failed') return switching ? 'Switch did not finish' : 'Update did not finish'
    if (relevant.status === 'cancelled') return switching ? 'Switch cancelled' : 'Update cancelled'
    if (relevant.status === 'succeeded') return `${switching ? 'Switched' : 'Updated'} to ${agentBuild(agent)}`
    return ''
  }
</script>

<section class="updater" aria-label={`reet updates for ${agent.host}`}>
  <header class="head">
    <div class="identity">
      <span class="mark" aria-hidden="true">↑</span>
      <div class="who">
        <p class="eyebrow">reet agent</p>
        <p class="builds">
          <code>{agentBuild(agent)}</code>
          {#if phase === 'available' || phase === 'queued' || phase === 'running' || phase === 'failed' || phase === 'rolled_back'}
            <span class="arrow" aria-hidden="true">→</span>
            <code class="to">{shownTarget}</code>
          {/if}
        </p>
      </div>
    </div>
    <span class="pill {tone}" aria-live="polite">{pillLabel()}</span>
  </header>

  <p class="explain">{explanation}</p>

  {#if relevant && stages.length && relevant.status !== 'cancelled'}
    <div
      class="bar"
      role="progressbar"
      aria-label="Update progress"
      aria-valuemin="0"
      aria-valuemax={progress.total}
      aria-valuenow={progress.done}
    >
      <span class:failed={relevant.status === 'failed'} class:rolled={relevant.status === 'rolled_back'} style={`transform: scaleX(${progress.value})`}></span>
    </div>
    <ol class="stages">
      {#each stages as stage (stage.key)}
        <li class={stage.status}>
          <span class="dot" aria-hidden="true"></span>
          <span class="stage-copy">
            <span class="stage-label">{stage.label}</span>
            {#if stage.detail}<span class="stage-detail">{stage.detail}</span>{/if}
          </span>
          <span class="vh">{stage.status}</span>
        </li>
      {/each}
    </ol>
  {/if}

  {#if relevant && (relevant.status === 'failed' || relevant.status === 'rolled_back' || relevant.status === 'cancelled' || relevant.status === 'succeeded')}
    <div class="outcome {tone}" role={relevant.status === 'succeeded' || relevant.status === 'cancelled' ? 'status' : 'alert'}>
      <p class="outcome-title">{outcomeTitle()}</p>
      {#if relevant.message}<p class="outcome-body">{relevant.message}</p>{/if}
      {#if (relevant.status === 'failed' || relevant.status === 'rolled_back') && agent.autoUpdate}
        <p class="outcome-note">Automatic updates wait an hour after an attempt before trying again.</p>
      {/if}
    </div>
  {/if}

  <footer class="actions">
    {#if canWrite}
      {#if relevant?.status === 'queued'}
        <button class="btn quiet" type="button" disabled={busy} onclick={oncancel}>Cancel update</button>
      {/if}
      {#if phase === 'running' || phase === 'queued'}
        <button class="btn primary" type="button" disabled>{phase === 'queued' ? 'Waiting for the machine…' : 'Updating…'}</button>
      {:else}
        <button class="btn primary" type="button" disabled={!canReview} onclick={openReview}>
          {phase === 'failed' || phase === 'rolled_back'
            ? 'Try again…'
            : updateWaiting
              ? `Review ${onDev ? 'dev update' : 'update'}…`
              : 'Update reet…'}
        </button>
      {/if}
      <label class="auto" title="Install new reet builds for this machine without a review">
        <input type="checkbox" checked={agent.autoUpdate} disabled={busy} onchange={(event) => onpolicy(event.currentTarget.checked)} />
        <span>Update automatically</span>
      </label>
    {:else}
      <span class="read-only">Automatic updates {agent.autoUpdate ? 'on' : 'off'}</span>
    {/if}
  </footer>
</section>

{#if choosing}
  <ConfirmDialog
    title={`Update reet on ${agent.host}`}
    body="The agent downloads the published release and verifies its checksum and build before changing anything. It keeps the current binary and restarts into the new one; if the new build cannot reach the controller, the previous binary is put back automatically."
    confirmLabel={`Update to ${targetLabel(target)}`}
    onconfirm={confirm}
    oncancel={() => (choosing = false)}
  >
    <div class="review">
      <div class="diff" aria-label={`From ${agentBuild(agent)} to ${targetLabel(target)}`}>
        <div><span class="lab">Runs now</span><code>{agentBuild(agent)}</code></div>
        <span class="arrow" aria-hidden="true">→</span>
        <div><span class="lab">After</span><code class="to">{targetLabel(target)}</code></div>
      </div>
      {#if target !== 'dev' && releases?.latest?.version}
        <a class="notes" href={`https://github.com/eyupio/reet.sh/releases/tag/${releases.latest.version}`} target="_blank" rel="noopener noreferrer">{releases.latest.version} release notes ↗</a>
      {/if}
      <fieldset class="targets">
        <legend>Which build</legend>
        <label class="target" class:chosen={target === (onDev ? 'dev' : 'latest')}>
          <input type="radio" name="target" value={onDev ? 'dev' : 'latest'} bind:group={target} />
          <span>
            <strong>{currentTarget || (onDev ? 'Development (dev)' : 'Latest release')}</strong>
            <small>{currentTarget
              ? onDev ? 'Stay on the development channel.' : 'Stay on the release channel.'
              : 'The controller has not checked this channel; the agent resolves it from GitHub.'}</small>
          </span>
        </label>
        {#if otherTarget}
          <label class="target" class:chosen={target === (onDev ? 'latest' : 'dev')}>
            <input type="radio" name="target" value={onDev ? 'latest' : 'dev'} bind:group={target} />
            <span>
              <strong>{otherTarget}</strong>
              <small>{onDev ? 'Switch to the latest stable release.' : 'Switch to the development channel, built from main.'}</small>
            </span>
          </label>
        {/if}
      </fieldset>
      {#if target && target !== (onDev ? 'dev' : 'latest')}
        <p class="warn">This changes channel. Development follows main and a release follows tagged builds, so the build you choose may be older than the one running now. Skills and enrollment are unaffected.</p>
      {/if}
      <div class="side">
        <div class="kept">
          <p>Kept</p>
          <ul>
            <li>This enrollment and its project scopes</li>
            <li>Installed skills and their files</li>
            <li>The current binary, as reet.previous</li>
          </ul>
        </div>
        <div class="interrupted">
          <p>Interrupted</p>
          <ul>
            <li>Sync pauses for the few seconds the agent restarts</li>
            <li>A running job on this machine must finish first</li>
          </ul>
        </div>
      </div>
    </div>
  </ConfirmDialog>
{/if}

<style>
  .updater { display: grid; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  .identity { display: flex; align-items: flex-start; gap: 9px; min-width: 0; }
  .mark { display: grid; place-items: center; flex: none; width: 24px; height: 24px; border-radius: 7px; background: var(--tint); color: var(--accent); font-weight: 700; font-size: 0.9rem; }
  .who { display: grid; gap: 1px; min-width: 0; }
  .eyebrow { margin: 0; color: var(--muted); font-size: 0.62rem; font-weight: 700; letter-spacing: 0.1em; text-transform: uppercase; }
  .builds { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; margin: 0; font-size: 0.76rem; }
  .builds code { color: var(--ink); }
  .builds code.to { color: var(--accent); font-weight: 600; }
  .arrow { color: var(--muted); }
  .pill { flex: none; padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.66rem; font-weight: 600; white-space: nowrap; }
  .pill.good { color: var(--good); border-color: color-mix(in srgb, var(--good) 35%, var(--line)); background: color-mix(in srgb, var(--good) 7%, transparent); }
  .pill.accent { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); background: var(--tint); }
  .pill.busy { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); }
  .pill.warn { color: var(--warn, #b45309); border-color: color-mix(in srgb, var(--warn, #b45309) 40%, var(--line)); }
  .pill.bad { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 40%, var(--line)); background: color-mix(in srgb, var(--bad) 6%, transparent); }
  .explain { margin: 0; color: var(--muted); font-size: 0.72rem; line-height: 1.5; }
  .bar { height: 4px; border-radius: 999px; background: var(--line); overflow: hidden; }
  .bar span { display: block; height: 100%; background: var(--accent); transform-origin: left; transition: transform 400ms var(--ease-out); }
  .bar span.failed { background: var(--bad); }
  .bar span.rolled { background: var(--warn, #b45309); }
  .stages { list-style: none; display: grid; gap: 5px; margin: 0; padding: 0; }
  .stages li { display: flex; align-items: flex-start; gap: 7px; color: var(--muted); font-size: 0.7rem; line-height: 1.4; }
  .dot { flex: none; width: 9px; height: 9px; margin-top: 3px; border-radius: 50%; border: 1px solid var(--line); background: var(--card); }
  .stages li.done .dot { background: var(--good); border-color: var(--good); }
  .stages li.running .dot { border-color: var(--accent); border-width: 2px; background: transparent; }
  .stages li.failed .dot { background: var(--bad); border-color: var(--bad); }
  .stage-copy { display: grid; gap: 0; min-width: 0; }
  .stage-label { color: var(--ink); }
  .stages li.pending .stage-label { color: var(--muted); }
  .stage-detail { color: var(--muted); font-size: 0.67rem; overflow-wrap: anywhere; }
  .outcome { display: grid; gap: 3px; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; }
  .outcome.good { border-color: color-mix(in srgb, var(--good) 35%, var(--line)); background: color-mix(in srgb, var(--good) 6%, transparent); }
  .outcome.warn { border-color: color-mix(in srgb, var(--warn, #b45309) 35%, var(--line)); background: color-mix(in srgb, var(--warn, #b45309) 6%, transparent); }
  .outcome.bad { border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); background: color-mix(in srgb, var(--bad) 6%, transparent); }
  .outcome-title { margin: 0; font-size: 0.74rem; font-weight: 650; }
  .outcome-body, .outcome-note { margin: 0; color: var(--muted); font-size: 0.69rem; line-height: 1.45; overflow-wrap: anywhere; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
  .actions .btn { min-height: 34px; padding: 6px 11px; font-size: 0.76rem; }
  .auto { display: inline-flex; align-items: center; gap: 5px; color: var(--muted); font-size: 0.72rem; cursor: pointer; }
  .read-only { color: var(--muted); font-size: 0.72rem; }
  .review { display: grid; gap: 14px; margin-top: 16px; }
  .diff { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .diff > div { display: grid; gap: 1px; }
  .lab { color: var(--muted); font-size: 0.62rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
  .diff code { font-size: 0.78rem; }
  .diff code.to { color: var(--accent); }
  .notes { justify-self: start; font-size: 0.72rem; }
  .targets { display: grid; gap: 7px; margin: 0; padding: 0; border: 0; }
  .targets legend { margin-bottom: 2px; color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
  .target { display: flex; align-items: flex-start; gap: 9px; padding: 9px 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); cursor: pointer; }
  .target.chosen { border-color: var(--accent); background: var(--tint); }
  .target > span { display: grid; gap: 1px; min-width: 0; }
  .target strong { font-size: 0.78rem; }
  .target small { color: var(--muted); font-size: 0.67rem; }
  .side { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 180px), 1fr)); gap: 10px; }
  .side > div { padding: 9px 11px; border: 1px solid var(--line); border-radius: 9px; }
  .side p { margin: 0 0 3px; font-size: 0.7rem; font-weight: 650; }
  .side ul { margin: 0; padding-left: 16px; color: var(--muted); font-size: 0.68rem; line-height: 1.45; }
  .interrupted { color: var(--muted); }
  .warn { margin: 0; padding: 8px 10px; border: 1px solid color-mix(in srgb, var(--warn, #b45309) 35%, var(--line)); border-radius: 8px; background: color-mix(in srgb, var(--warn, #b45309) 6%, transparent); color: var(--warn, #b45309); font-size: 0.7rem; line-height: 1.45; }
  @media (max-width: 560px) {
    .updater { padding: 10px; }
    .actions .btn.primary { flex: 1 1 auto; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .stages li.running .dot { animation: pulse 1.4s ease-in-out infinite; }
    @keyframes pulse { 50% { opacity: 0.35; } }
  }
</style>
