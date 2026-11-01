<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { installSessionSkill, cancelSessionSkillInstall } from './api'
  import { getSyncState, controlSyncJob, type SyncJob } from '../api'
  import { installStep, installStatusLine, planVerdict, type SessionSkill } from './skills'

  // Installs one reet-hosted skill on the session's machine: confirm, wait for the machine's plan, review it, approve it, watch it finish.
  // Everything shown (names, plan lines, the machine's message) is untrusted text and is only ever rendered as text.
  // The panel never cancels a job by itself, and closing it (or the picker) leaves a started job as it is.
  let { sessionId, skill, host, tool, onclose }: {
    sessionId: string
    skill: SessionSkill
    host: string
    tool: string
    onclose: (installed: boolean) => void
  } = $props()

  const slug = $derived(skill.id.replace(/^reet:/, ''))
  const POLL_MS = 2000

  let phase = $state<'confirm' | 'started' | 'cancelled'>('confirm')
  let jobId = $state('')
  let job = $state<SyncJob | undefined>()
  let startedAt = $state(Date.now()), now = $state(Date.now())
  let busy = $state(false)
  let problem = $state<{ text: string; phase: string; step: string } | null>(null)
  let note = $state(''), pollProblem = $state('')
  let root = $state<HTMLElement>()
  let alive = true
  let timer: ReturnType<typeof setTimeout> | undefined
  const lives = new Set<AbortController>()
  let pollLife: AbortController | undefined

  const step = $derived(phase === 'confirm' ? null : phase === 'cancelled' ? 'cancelled' : installStep(job))
  const verdict = $derived(step === 'review' ? planVerdict(job?.plan, slug) : null)
  const failure = $derived(step === 'failed' ? job?.message ?? '' : '')
  // A refusal belongs to the state it was shown in; once the state moves on it no longer applies.
  const shownProblem = $derived(problem && problem.phase === phase && problem.step === (step ?? '') ? problem.text : '')
  const status = $derived.by(() => {
    const line = phase === 'confirm' ? '' : installStatusLine(step ?? 'waiting', host, slug, Math.max(0, Math.floor((now - startedAt) / 1000)))
    return [shownProblem || pollProblem || line, note].filter(Boolean).join(' ')
  })
  const terminal = (s: string | null) => s === 'done' || s === 'failed' || s === 'cancelled'

  function track(): AbortController {
    const life = new AbortController()
    lives.add(life)
    return life
  }
  const message = (e: unknown, fallback: string) => (e instanceof Error && e.message ? e.message : fallback)
  const codeOf = (e: unknown) => (e as { code?: string } | null)?.code
  function fail(text: string) { problem = { text, phase, step: step ?? '' } }

  async function focusPrimary() {
    await tick()
    if (!alive) return
    const primary = root?.querySelector<HTMLElement>('[data-primary]')
    ;(primary ?? root)?.focus()
  }

  function stop() { clearTimeout(timer); timer = undefined; pollLife?.abort(); pollLife = undefined }

  async function read() {
    clearTimeout(timer); timer = undefined
    now = Date.now()
    pollLife?.abort()
    const life = pollLife = new AbortController()
    try {
      const state = await getSyncState(life.signal)
      if (life.signal.aborted || !alive) return
      job = state.jobs?.find(j => j.id === jobId)
      pollProblem = ''
    } catch (e) {
      if (life.signal.aborted || !alive) return
      pollProblem = 'Could not check on the install just now. Trying again.'
    }
    if (pollLife === life) pollLife = undefined
    if (!alive || phase !== 'started') return
    const s = installStep(job)
    if (s === 'done') { onclose(true); return }
    if (!terminal(s)) timer = setTimeout(() => void read(), POLL_MS)
  }

  async function start() {
    if (busy) return
    busy = true; problem = null; note = ''
    const life = track()
    try {
      const started = await installSessionSkill(sessionId, skill.id, life.signal)
      if (!alive) return
      jobId = started.job; job = undefined; startedAt = Date.now(); now = startedAt; phase = 'started'
      busy = false
      void read()
      void focusPrimary()
    } catch (e) {
      if (!alive || life.signal.aborted) return
      if (codeOf(e) === 'unpublished') { onclose(true); return }
      fail(message(e, 'The install could not be started. Try again.'))
      busy = false
      void focusPrimary()
    } finally {
      lives.delete(life)
      busy = false
    }
  }

  async function approve() {
    if (busy || phase !== 'started') return
    busy = true; problem = null
    const life = track()
    try {
      // Look again at the job as it is now: the plan and its hash may have changed since the review opened.
      const state = await getSyncState(life.signal)
      if (!alive) return
      const current = state.jobs?.find(j => j.id === jobId)
      job = current
      if (installStep(current) !== 'review') { fail('This install changed while you were reviewing it. Check it again.'); return }
      if (!planVerdict(current?.plan, slug).ok || !current) return
      await controlSyncJob(jobId, 'approve', current.hash)
      if (!alive) return
      void read()
    } catch (e) {
      if (!alive || life.signal.aborted) return
      fail(message(e, 'The plan could not be approved. Try again.'))
    } finally {
      lives.delete(life)
      busy = false
      if (alive) void focusPrimary()
    }
  }

  async function cancel() {
    if (busy || phase !== 'started') return
    busy = true; problem = null; note = ''
    const life = track()
    try {
      const result = await cancelSessionSkillInstall(sessionId, jobId, life.signal)
      if (!alive) return
      stop()
      if (result.reverted) { onclose(false); return }
      note = 'The saved skill list was changed elsewhere, so it was left as it is.'
      phase = 'cancelled'
    } catch (e) {
      if (!alive || life.signal.aborted) return
      // A running preview cannot be cancelled: the sentence is shown and the panel keeps polling.
      fail(message(e, 'The install could not be cancelled. Try again.'))
    } finally {
      lives.delete(life)
      busy = false
      if (alive) void focusPrimary()
    }
  }

  async function retry() {
    if (busy) return
    stop()
    phase = 'confirm'
    await start()
  }

  // Focus follows each state the person has to act on; a waiting state moves it to the panel, which announces it.
  let focused = ''
  $effect(() => {
    const key = `${phase}/${step}/${verdict ? verdict.ok : ''}`
    if (key === focused) return
    focused = key
    void focusPrimary()
  })

  onMount(() => {
    void focusPrimary()
    return () => {
      alive = false
      stop()
      for (const life of lives) life.abort()
      lives.clear()
    }
  })
</script>

<section class="install" bind:this={root} tabindex="-1" aria-label={`Install ${skill.name}`}>
  <h3>Install {skill.name}</h3>
  {#if phase === 'confirm'}
    <p>Add {slug} to the {tool} skills folder on {host}. Nothing runs until you approve the plan.</p>
  {:else if step === 'review' && verdict}
    <ul class="plan">
      {#each verdict.lines as line}<li>{line}</li>{/each}
    </ul>
    {#if !verdict.ok}
      <p>{verdict.reason}</p>
      <p><a href="/sync" target="_blank" rel="noopener">Open the Sync page</a></p>
    {/if}
  {:else if failure}
    <p class="failure">{failure}</p>
  {/if}
  <p class="status" role="status">{status}</p>
  <div class="actions">
    {#if phase === 'confirm'}
      <button type="button" class="primary" data-primary disabled={busy} onclick={start}>Install</button>
      <button type="button" disabled={busy} onclick={() => onclose(false)}>Cancel</button>
    {:else if step === 'waiting'}
      <button type="button" disabled={busy} onclick={cancel}>Cancel</button>
    {:else if step === 'review'}
      {#if verdict?.ok}<button type="button" class="primary" data-primary disabled={busy} onclick={approve}>Approve and install</button>{/if}
      <button type="button" data-primary={verdict?.ok ? undefined : ''} disabled={busy} onclick={cancel}>Cancel</button>
    {:else if step === 'failed'}
      <button type="button" class="primary" data-primary disabled={busy} onclick={retry}>Retry</button>
      <button type="button" disabled={busy} onclick={() => onclose(false)}>Close</button>
    {:else if step === 'cancelled'}
      <button type="button" class="primary" data-primary onclick={() => onclose(false)}>Close</button>
    {/if}
  </div>
</section>

<style>
  .install { display: flex; flex-direction: column; gap: 10px; padding: 8px 10px; min-width: 0; overflow-wrap: anywhere; }
  .install:focus { outline: none; }
  h3 { margin: 0; font: 600 1rem/1.3 var(--serif); }
  p { margin: 0; font-size: 14px; line-height: 1.45; }
  .plan { margin: 0; padding: 0 0 0 18px; font-size: 14px; line-height: 1.5; }
  .failure { color: var(--ink); }
  .status { min-height: 1.45em; color: var(--muted); font-size: 13px; }
  a { color: var(--accent); font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  button { min-height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--accent); font: inherit; font-weight: 600; cursor: pointer; }
  button.primary { background: var(--accent); border-color: var(--accent); color: var(--bg); }
  button:disabled { opacity: 0.55; cursor: default; }
  @media (pointer: coarse) {
    button { min-height: 44px; }
    a { display: inline-flex; align-items: center; min-height: 44px; }
  }
</style>
