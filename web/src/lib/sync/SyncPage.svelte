<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from '../PageHeader.svelte'
  import Sentence from '../Sentence.svelte'
  import { LOCKED_SENTENCE } from '../terminal/api'
  import type { SectionItem } from '../SectionNav.svelte'
  import UpdateReview from '../UpdateReview.svelte'
  import FleetSoftware from '../FleetSoftware.svelte'
  import { controlSyncAgent, getSyncState, loadSyncSkills, retryMachineSkills, UnauthorizedError, updateSyncAgent, type ReleaseInfo, type SyncAgent, type SyncMachine, type SyncSkills } from '../api'
  import { describeError } from '../contentReview'
  import { formatWhen } from '../time'
  import { activityText, ago, andList, groupActivity, ledeText, machineLine, pageSentence } from './syncPage'

  // Sync: one sentence for every machine, a row per machine and what the machines did lately. Each machine follows the
  // library unless it is paused; the switch is the only choice a machine has. A machine that failed a skill offers Retry,
  // which asks it to try again on its next poll. A machine below the skills protocol is offered the update it needs;
  // one newer than this controller can only wait for the controller. All skill work follows the shared library.
  let { canWrite, accounts = false, timeZone = '', dateFormat = 'locale', onsignedout }: { canWrite: boolean; accounts?: boolean; timeZone?: string; dateFormat?: string; onsignedout?: (e: unknown) => void } = $props()

  // How often the page reads the machines again.
  const POLL_MS = 15000

  let data = $state<SyncSkills | null>(null)
  let locked = $state<ReadonlySet<string>>(new Set())
  let loadError = $state('')
  let message = $state('')
  let error = $state('')
  let busy = $state<Record<string, boolean>>({})
  let now = $state(Date.now())
  let review = $state<{ agent: SyncAgent; releases?: ReleaseInfo } | null>(null)
  let destroyed = false
  let softwareOpen = $state(false)

  const sections: SectionItem[] = [
    { label: 'Inventory & connection', href: '/machines', current: false },
    { label: 'Sync', href: '/sync', current: true },
  ]

  const machines = $derived(data?.machines ?? [])
  const title = $derived(data ? pageSentence(data.work, machines.length) : 'Sync')
  const lede = $derived(data ? (machines.length ? ledeText(data.active, data.work) : 'Connect a machine and your Active skills go on it. Archived ones stay off.') : '')
  const activity = $derived(groupActivity(data?.activity ?? []))

  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) {
      onsignedout?.(e)
      return
    }
    error = describeError(e)
  }

  async function read(quiet = false) {
    try {
      // A blocked machine is either not allowed to write skills or locked by its owner; only the agent list says which.
      const [next, state] = await Promise.all([loadSyncSkills(), getSyncState().catch(() => null)])
      if (destroyed) return
      data = next
      locked = new Set((state?.agents ?? []).filter(a => a.locked).map(a => a.id))
      now = Date.now()
      loadError = ''
    } catch (e) {
      if (e instanceof UnauthorizedError) return fail(e)
      if (!quiet || !data) loadError = describeError(e)
    }
  }

  // One action at a time per machine: a second press while one is on its way sends nothing.
  async function act(m: SyncMachine, key: string, fn: () => Promise<string>) {
    const id = `${key}:${m.agentId}`
    if (busy[id]) return
    busy = { ...busy, [id]: true }
    error = ''
    try {
      message = await fn()
      await read(true)
    } catch (e) {
      fail(e)
    } finally {
      const { [id]: _, ...rest } = busy
      busy = rest
    }
  }

  const toggle = (m: SyncMachine) => act(m, 'pause', async () => {
    await controlSyncAgent(m.agentId, m.paused ? 'resume' : 'pause')
    return m.paused ? `Sync is back on for ${m.host}.` : `Paused sync on ${m.host}. reet leaves its skills as they are.`
  })

  const retry = (m: SyncMachine) => act(m, 'retry', async () => {
    const names = await retryMachineSkills(m.agentId, m.failed.map(f => f.name))
    return `Trying ${andList(names)} again on ${m.host}.`
  })

  // The update is the one Machines offers: its review chooses the build, then the update is queued.
  const askUpdate = (m: SyncMachine) => act(m, 'update', async () => {
    const sync = await getSyncState()
    const agent = sync.agents?.find(a => a.id === m.agentId)
    if (!agent) throw new Error(`${m.host} is no longer connected.`)
    review = { agent, releases: sync.releases }
    return ''
  })
  function confirmUpdate(version: string) {
    const agent = review?.agent
    review = null
    if (!agent) return
    const m = machines.find(x => x.agentId === agent.id)
    if (m) void act(m, 'update', async () => {
      await updateSyncAgent(agent.id, version)
      return `Update queued for ${agent.host}. Its skills follow once reet there is updated.`
    })
  }

  onMount(() => {
    void read()
    const poll = setInterval(() => {
      if (document.visibilityState === 'visible' && !review) void read(true)
    }, POLL_MS)
    return () => {
      destroyed = true
      clearInterval(poll)
    }
  })
</script>

<PageHeader group="Machines" {title} {lede} {sections} sectionsLabel="Machines">
  <!-- Software is an action beside the title, as on Machines. -->
  {#snippet actions()}{#if accounts && canWrite}<button type="button" class="btn software" onclick={() => (softwareOpen = true)}>Software</button>{/if}{/snippet}
</PageHeader>

<div class="sync-page">
  <p class="notice" role="status">{message}</p>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}

  {#if loadError && !data}
    <p class="notice bad" role="alert">Couldn't read your machines: {loadError}</p>
  {:else if data}
    <div class="grid">
      <section aria-labelledby="sync-machines-title">
        <h2 id="sync-machines-title" class="label">Machines</h2>
        {#if machines.length}
          <ul class="machines" aria-label="Machines">
            {#each machines as m (m.agentId)}
              {@const line = machineLine(m, data.active, now)}
              <li class="machine">
                <div class="top">
                  <h3>{m.host}</h3>
                  {#if m.state !== 'needs-newer-controller'}
                    {#if canWrite}
                      <button type="button" class="switch" role="switch" aria-checked={!m.paused} aria-label={`Sync on ${m.host}`} aria-busy={!!busy[`pause:${m.agentId}`]} onclick={() => void toggle(m)}>
                        <span class="track" class:on={!m.paused} aria-hidden="true"><span></span></span>
                        <span aria-hidden="true">{m.paused ? 'Paused' : 'Sync on'}</span>
                      </button>
                    {:else}
                      <span class="switch-text">{m.paused ? 'Paused' : 'Sync on'}</span>
                    {/if}
                  {/if}
                </div>
                <p class="health" data-tone={line.tone}><span class="dot" aria-hidden="true"></span>{line.text}</p>
                {#if m.state === 'failed'}
                  <ul class="why">
                    {#each m.failed as f (f.name)}<li><strong>{f.name}</strong>{f.detail ? `: ${f.detail}` : ''}</li>{/each}
                  </ul>
                {:else if m.state === 'blocked'}
                  <p class="why">{#if locked.has(m.agentId)}<Sentence text={LOCKED_SENTENCE} />{:else}reet isn't allowed to change skills here. Run <code>reet agent allow-apply on</code> on {m.host}.{/if}</p>
                {/if}
                {#if (m.state === 'offline' || m.state === 'blocked') && m.waiting.length}
                  <p class="why">Waiting: {andList(m.waiting)}.</p>
                {/if}
                {#if canWrite && m.state === 'failed'}
                  <div><button type="button" class="btn primary" aria-label={`Retry ${andList(m.failed.map(f => f.name))} on ${m.host}`} disabled={!!busy[`retry:${m.agentId}`]} onclick={() => void retry(m)}>Retry</button></div>
                {:else if canWrite && m.state === 'needs-update'}
                  <div><button type="button" class="btn" aria-label={`Update reet on ${m.host}`} disabled={!!busy[`update:${m.agentId}`]} onclick={() => void askUpdate(m)}>Update reet</button></div>
                {/if}
              </li>
            {/each}
          </ul>
        {:else}
          <div class="first">
            <p class="first-title">Start with your first machine.</p>
            <p class="empty">Connect one and your Active skills go on it, and come off it when you archive them.</p>
            {#if canWrite}<a class="btn primary" href="/machines#add-machine">Add a machine</a>{/if}
          </div>
        {/if}
      </section>

      <section class="activity" aria-labelledby="sync-activity-title">
        <h2 id="sync-activity-title" class="label">Recent activity</h2>
        {#if activity.length}
          <ol aria-label="Recent activity">
            {#each activity as g, i (`${g.at}:${g.name}:${g.action}:${i}`)}
              {@const t = activityText(g)}
              <li><time datetime={new Date(g.at).toISOString()} title={formatWhen(g.at, timeZone, dateFormat)}>{ago(g.at, now)}</time><span>{t.lead}<strong>{t.name}</strong>{t.rest}</span></li>
            {/each}
          </ol>
        {:else}
          <p class="empty">Nowt yet. What your machines do with your skills shows here.</p>
        {/if}
      </section>
    </div>

  {/if}
</div>

{#if softwareOpen}<FleetSoftware onclose={() => (softwareOpen = false)} />{/if}
{#if review}
  <UpdateReview agent={review.agent} releases={review.releases} initial={review.agent.version === 'dev' ? 'dev' : 'latest'} onconfirm={confirmUpdate} oncancel={() => (review = null)} />
{/if}

<style>
  .sync-page { display: flex; flex-direction: column; gap: 20px; }
  .notice { margin: 0; font-size: 0.9375rem; color: var(--ink-soft); }
  .notice:empty { display: none; }
  .notice.bad { color: var(--bad); }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr)); gap: 24px; align-items: start; }
  section { min-width: 0; border: 1px solid var(--line); border-radius: 14px; background: var(--card); padding: 16px 20px; }
  .label { margin: 0 0 4px; font: 500 0.875rem/1.4 var(--mono); letter-spacing: 0.1em; text-transform: uppercase; color: var(--muted); }
  .machines, .why, ol { list-style: none; margin: 0; padding: 0; }
  .machine { display: flex; flex-direction: column; gap: 6px; padding: 14px 0; border-top: 1px solid var(--line-soft, var(--line)); }
  .machine:first-child { border-top: 0; }
  .top { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
  h3 { margin: 0; font-size: 1.0625rem; font-weight: 600; overflow-wrap: anywhere; color: var(--ink); }
  .health { display: flex; align-items: baseline; gap: 8px; margin: 0; font-size: 0.875rem; font-weight: 500; color: var(--muted); overflow-wrap: anywhere; }
  .health[data-tone='ok'] { color: var(--good); }
  .health[data-tone='bad'] { color: var(--bad); }
  .health[data-tone='work'] { color: var(--info); }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; flex: none; transform: translateY(-1px); }
  .why, .why li { margin: 0; font-size: 0.875rem; color: var(--ink-soft); overflow-wrap: anywhere; line-height: 1.45; }
  code { font-family: var(--mono); font-size: 0.875rem; }
  .btn, :global(.page-head .software) { min-height: 44px; }
  .switch { display: inline-flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 4px; border: 0; background: none; color: var(--ink-soft); font: inherit; font-size: 0.875rem; cursor: pointer; border-radius: 8px; }
  .switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .switch[aria-busy='true'] { cursor: progress; }
  .switch-text { font-size: 0.875rem; color: var(--ink-soft); min-height: 44px; display: inline-flex; align-items: center; }
  .track { width: 40px; height: 24px; border-radius: 999px; background: var(--line-strong); position: relative; flex: none; transition: background-color 0.15s; }
  .track span { position: absolute; top: 3px; left: 3px; width: 18px; height: 18px; border-radius: 50%; background: var(--card); transition: left 0.15s var(--ease-out); }
  .track.on { background: var(--accent); }
  .track.on span { left: 19px; }
  ol li { display: flex; gap: 14px; padding: 10px 0; border-top: 1px solid var(--line-soft, var(--line)); font-size: 0.875rem; line-height: 1.45; color: var(--ink); }
  ol li:first-child { border-top: 0; }
  ol li span { min-width: 0; overflow-wrap: anywhere; }
  time { width: 76px; flex: none; color: var(--muted); font-family: var(--mono); font-size: 0.875rem; }
  .first { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; padding: 8px 0 4px; }
  .first-title { margin: 0; font-weight: 600; color: var(--ink); }
  .first .btn { min-height: 44px; }
  .empty { margin: 8px 0; font-size: 0.875rem; color: var(--muted); }
  @media (prefers-reduced-motion: reduce) { .track, .track span { transition: none; } }
</style>
