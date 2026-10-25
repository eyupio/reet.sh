<script lang="ts">
  import type { ReleaseInfo, SyncAgent, SyncJob } from './api'
  import { agentBuild, agentUpdateState, channelBuild, updateAction, updateAvailable, updateProgress, updateStateLabel, updateStepsShown } from './machines'

  let {
    agent,
    job,
    releases,
    canWrite,
    busy,
    onreview,
    oncancel,
  }: {
    agent: SyncAgent
    job?: SyncJob
    releases?: ReleaseInfo
    canWrite: boolean
    busy: boolean
    onreview: () => void
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
  const steps = $derived(updateStepsShown(relevant))
  const progress = $derived(updateProgress(relevant))
  const onDev = $derived(agent.version === 'dev')
  const stableLabel = $derived(channelBuild(releases, 'latest'))
  const devLabel = $derived(channelBuild(releases, 'dev'))
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
  const action = $derived(updateAction(phase, { autoUpdate: agent.autoUpdate, updateWaiting }))

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

  // The policy left the card for the machine's settings, so the card says
  // which way it is set; otherwise a quiet machine would not say why it waits.
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
    return `Running the newest build on the ${onDev ? 'development' : 'release'} channel. Automatic updates are ${agent.autoUpdate ? 'on' : 'off'}.`
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

{#snippet stageList()}
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
{/snippet}

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

  {#if relevant && steps === 'live'}
    <div
      class="bar"
      role="progressbar"
      aria-label="Update progress"
      aria-valuemin="0"
      aria-valuemax={progress.total}
      aria-valuenow={progress.done}
    >
      <span style={`transform: scaleX(${progress.value})`}></span>
    </div>
    {@render stageList()}
  {/if}

  {#if relevant && (relevant.status === 'failed' || relevant.status === 'rolled_back' || relevant.status === 'cancelled' || relevant.status === 'succeeded')}
    <div class="outcome {tone}" role={relevant.status === 'succeeded' || relevant.status === 'cancelled' ? 'status' : 'alert'}>
      <p class="outcome-title">{outcomeTitle()}</p>
      {#if relevant.message}<p class="outcome-body">{relevant.message}</p>{/if}
      {#if (relevant.status === 'failed' || relevant.status === 'rolled_back') && agent.autoUpdate}
        <p class="outcome-note">Automatic updates wait an hour after an attempt before trying again.</p>
      {/if}
      {#if steps === 'folded'}
        <details class="steps">
          <summary>Show the steps</summary>
          {@render stageList()}
        </details>
      {/if}
    </div>
  {/if}

  {#if canWrite && action}
    <footer class="actions">
      {#if action.intent === 'cancel'}
        <button class="btn" type="button" disabled={busy} onclick={oncancel}>{action.label}</button>
      {:else}
        <button class="btn" class:primary={action.emphasis === 'primary'} type="button" disabled={!canReview} onclick={onreview}>{action.label}</button>
      {/if}
    </footer>
  {/if}
</section>

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
  .steps { margin-top: 4px; }
  .steps > summary { cursor: pointer; color: var(--muted); font-size: 0.69rem; }
  .steps > summary:hover { color: var(--ink); }
  .steps[open] > summary { margin-bottom: 6px; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
  .actions .btn { min-height: 34px; padding: 6px 11px; font-size: 0.76rem; }
  @media (max-width: 560px) {
    .updater { padding: 10px; }
    .actions .btn.primary { flex: 1 1 auto; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .stages li.running .dot { animation: pulse 1.4s ease-in-out infinite; }
    @keyframes pulse { 50% { opacity: 0.35; } }
  }
</style>
