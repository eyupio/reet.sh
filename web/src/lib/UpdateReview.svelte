<script lang="ts">
  import type { ReleaseInfo, SyncAgent } from './api'
  import { agentBuild, channelBuild } from './machines'
  import ConfirmDialog from './ConfirmDialog.svelte'

  // The review is the one place a build is chosen, whether a person arrives from
  // the update card or from the machine's settings; both describe the move alike.
  let {
    agent,
    releases,
    initial,
    onconfirm,
    oncancel,
  }: {
    agent: SyncAgent
    releases?: ReleaseInfo
    initial: 'dev' | 'latest'
    onconfirm: (version: string) => void
    oncancel: () => void
  } = $props()

  const onDev = $derived(agent.version === 'dev')
  const stableLabel = $derived(channelBuild(releases, 'latest'))
  const devLabel = $derived(channelBuild(releases, 'dev'))
  const currentTarget = $derived(onDev ? devLabel : stableLabel)
  const otherTarget = $derived(onDev ? stableLabel : devLabel)
  const own = $derived<'dev' | 'latest'>(onDev ? 'dev' : 'latest')
  const other = $derived<'dev' | 'latest'>(onDev ? 'latest' : 'dev')

  // The review is created afresh each time it opens, so the opening choice is read once on purpose.
  // svelte-ignore state_referenced_locally
  let target = $state<'dev' | 'latest'>(initial)

  function targetLabel(value: string): string {
    return value === 'dev' ? devLabel || 'the development build' : stableLabel || 'the latest release'
  }
</script>

<ConfirmDialog
  title={`Update reet on ${agent.host}`}
  body="The agent downloads the published release and verifies its checksum and build before changing anything. It keeps the current binary and restarts into the new one; if the new build cannot reach the controller, the previous binary is put back automatically."
  confirmLabel={`Update to ${targetLabel(target)}`}
  onconfirm={() => onconfirm(target)}
  {oncancel}
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
      <label class="target" class:chosen={target === own}>
        <input type="radio" name="target" value={own} bind:group={target} />
        <span>
          <strong>{currentTarget || (onDev ? 'Development (dev)' : 'Latest release')}</strong>
          <small>{currentTarget
            ? onDev ? 'Stay on the development channel.' : 'Stay on the release channel.'
            : 'The controller has not checked this channel; the agent resolves it from GitHub.'}</small>
        </span>
      </label>
      <label class="target" class:chosen={target === other}>
        <input type="radio" name="target" value={other} bind:group={target} />
        <span>
          <strong>{otherTarget || (onDev ? 'Latest release' : 'Development (dev)')}</strong>
          <small>{onDev ? 'Switch to the latest stable release.' : 'Switch to the development channel, built from main.'}</small>
        </span>
      </label>
    </fieldset>
    {#if target !== own}
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

<style>
  .review { display: grid; gap: 14px; margin-top: 16px; }
  .diff { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .diff > div { display: grid; gap: 1px; }
  .lab { color: var(--muted); font-size: 0.62rem; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
  .diff code { font-size: 0.78rem; }
  .diff code.to { color: var(--accent); }
  .arrow { color: var(--muted); }
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
</style>
