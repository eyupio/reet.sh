<script lang="ts">
 import { onMount, tick, untrack } from 'svelte'
 import { UnauthorizedError } from './api'
 import { getRun, retryMember, TerminalError, type TerminalMachine, type TerminalSession } from './terminal/api'
 import { toolName } from './agentLaunch'
 import WorktreeDialog from './WorktreeDialog.svelte'
 import { STARTED_MEANS, memberLine, projectRefusalText, removeOffer, runPending, runSummary, type ProjectRun, type RunMember } from './projects'

 // One run: each repository's state, read again every 2 seconds while any is still waiting to start (and only while the
 // tab is visible). Open goes to the existing terminal session; Retry tries a failed repository again; Remove worktree
 // looks at the worktree first. Closing the page does not stop a run: the server carries on without it.
 let { run, machines, sessions, sessionsFailed = false, onchange, onsignedout }: {
  run: ProjectRun
  machines: TerminalMachine[]
  /** The account's sessions, or undefined while they are not known: Remove worktree is then never offered. */
  sessions: TerminalSession[] | undefined
  /** The last sessions read failed. */
  sessionsFailed?: boolean
  /** The run as last read, so the page can keep its list current. */
  onchange?: (run: ProjectRun) => void
  onsignedout?: (e: unknown) => void
 } = $props()

 const id = $props.id()
 // svelte-ignore state_referenced_locally
 let current = $state<ProjectRun>(run)
 // Reading the run: a passing failure retries on the 2-second pace; a refusal, or five failures in a row, stops it and says so.
 let readFailed = $state(false), stopped = $state(''), message = $state(''), failed = $state(false)
 let failures = 0
 // The member a Retry was sent for: its status line goes once it is no longer waiting.
 let retried: number | null = null
 // Bumped by every applied Retry result: a read that began before it is older, and is dropped.
 let generation = 0
 let retrying = $state<number | null>(null)
 let removing = $state<{ member: RunMember; opener: HTMLElement | null } | null>(null)
 // A newer copy from the page (after a start or a reload) replaces this one.
 $effect(() => { const next = run; untrack(() => { if (next !== current && next.id === current.id) { current = next; schedule() } }) })

 const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host ?? 'A machine no longer on this account'
 const started = $derived(current.members.filter(m => m.state === 'started' && m.sessionId))
 const pending = $derived(runPending(current))
 const openURL = (m: RunMember) => `/terminals?agent=${encodeURIComponent(m.host)}&session=${encodeURIComponent(m.sessionId)}`

 const life = new AbortController()
 let timer: ReturnType<typeof setTimeout> | undefined
 let reading = false
 function handOff(e: unknown): boolean {
  if (!(e instanceof TerminalError) || e.status !== 401 || !onsignedout) return false
  life.abort(); clearTimeout(timer); onsignedout(new UnauthorizedError())
  return true
 }
 async function read() {
  if (reading || life.signal.aborted) return
  reading = true
  const began = generation
  try {
   const next = await getRun(current.id, life.signal)
   if (life.signal.aborted || began !== generation) return
   current = next; readFailed = false; failures = 0
   if (retried !== null && next.members.find(m => m.position === retried)?.state !== 'pending') { retried = null; message = ''; failed = false }
   onchange?.(next)
  } catch (e) {
   if (life.signal.aborted || handOff(e)) return
   readFailed = true
   failures++
   if (e instanceof TerminalError && [400, 403, 404, 409, 413, 422].includes(e.status)) stopped = `${projectRefusalText(e.status, e.code, e.message)} This run is no longer being read.`
   else if (failures >= 5) stopped = "Couldn't read this run after several tries, so it is no longer being read."
  } finally {
   reading = false
   schedule()
  }
 }
 // Only while something is waiting, and only while the tab is visible; coming back to the tab reads at once.
 function schedule() {
  clearTimeout(timer)
  if (life.signal.aborted || stopped || !runPending(current) || document.visibilityState !== 'visible') return
  timer = setTimeout(() => void read(), 2000)
 }
 function visibility() {
  if (document.visibilityState !== 'visible') { clearTimeout(timer); return }
  if (runPending(current) && !stopped) void read()
 }
 function checkAgain() { stopped = ''; readFailed = false; failures = 0; void read() }

 async function retry(m: RunMember, button: HTMLElement) {
  if (retrying !== null || life.signal.aborted) return
  retrying = m.position; failed = false
  message = `Trying ${hostOf(m.host)} ${m.dir} again…`
  try {
   const next = await retryMember(current.id, m.position, life.signal)
   if (life.signal.aborted) return
   generation++
   current = next; onchange?.(next)
   retried = m.position
   message = `Trying ${hostOf(m.host)} ${m.dir} again. Its state shows below.`
  } catch (e) {
   if (life.signal.aborted || handOff(e)) return
   failed = true
   const definite = e instanceof TerminalError && [400, 403, 404, 409, 413, 422].includes(e.status)
   message = definite ? projectRefusalText((e as TerminalError).status, (e as TerminalError).code, (e as TerminalError).message) : "We didn't hear back, so it may be starting. Reading the run again."
   if (!definite) { retried = m.position; void read() }
  } finally {
   if (!life.signal.aborted) {
    retrying = null
    schedule()
    await tick()
    // The member's Retry button is gone once it is waiting again; its row keeps focus nearby.
    if (button.isConnected) button.focus(); else document.getElementById(`${id}-m-${m.position}`)?.focus()
   }
  }
 }

 onMount(() => {
  schedule()
  document.addEventListener('visibilitychange', visibility)
  return () => { life.abort(); clearTimeout(timer); document.removeEventListener('visibilitychange', visibility) }
 })
</script>

<div class="run">
 <p class="summary" role="status" aria-live="polite" aria-atomic="true">{runSummary(current)}{#if pending} <span class="muted">Checking every 2 seconds.</span>{/if}</p>
 {#if stopped}<p class="note">{stopped} <button type="button" class="btn small" onclick={checkAgain}>Check again</button></p>{:else if readFailed}<p class="note">Couldn't read this run just now. Trying again.</p>{/if}
 {#if !pending && started.length === 1}
  <p class="one"><a class="btn primary" href={openURL(started[0])}>Open the session on {hostOf(started[0].host)}</a></p>
 {/if}
 <ol class="members">
  {#each current.members as m (m.position)}
   {@const offer = removeOffer(m, sessions, sessionsFailed)}
   <li id={`${id}-m-${m.position}`} tabindex="-1" data-state={m.state}>
    <div class="what">
     <p class="where"><strong>{hostOf(m.host)}</strong> <span class="mono">{m.dir}</span></p>
     <p class="meta">{toolName(m.tool)} · {m.model || 'Agent default'}{m.attempt > 1 ? ` · attempt ${m.attempt}` : ''}</p>
     <p class="line" class:bad={m.state === 'failed'}><span class="dot" aria-hidden="true"></span>{memberLine(m)}</p>
     {#if m.worktreeBranch || m.worktreePath}<p class="meta">Worktree <span class="mono">{m.worktreePath || 'folder not reported'}</span>{#if m.worktreeBranch}{' on '}<span class="mono">{m.worktreeBranch}</span>{/if}</p>{/if}
     {#if m.sharesCheckout && m.state !== 'failed'}<p class="warn">No worktree: this agent works in the saved folder itself, so any other session there shares that checkout.</p>{/if}
     {#if offer.reason}<p class="meta">{offer.reason}</p>{/if}
    </div>
    <div class="actions">
     {#if (m.state === 'started' || m.state === 'removed') && m.sessionId}<a class="btn" href={openURL(m)} aria-label={`Open ${toolName(m.tool)} on ${hostOf(m.host)} in ${m.dir}`}>Open</a>{/if}
     {#if m.state === 'failed'}<button type="button" class="btn" onclick={(e) => retry(m, e.currentTarget)} disabled={retrying !== null} aria-busy={retrying === m.position} aria-label={`Retry ${hostOf(m.host)} ${m.dir}`}>{retrying === m.position ? 'Retrying…' : 'Retry'}</button>{/if}
     {#if offer.offer}<button type="button" class="btn danger" onclick={(e) => (removing = { member: m, opener: e.currentTarget })} aria-label={`Remove worktree for ${hostOf(m.host)} ${m.dir}`}>Remove worktree</button>{/if}
    </div>
   </li>
  {/each}
 </ol>
 <p class="status" class:bad={failed} role="status" aria-live="polite" aria-atomic="true">{message}</p>
 <p class="muted small">{STARTED_MEANS}</p>
</div>

{#if removing}
 <WorktreeDialog member={removing.member} host={hostOf(removing.member.host)} opener={removing.opener} onremoved={() => void read()} {onsignedout} onclose={() => (removing = null)} />
{/if}

<style>
 .run { display: grid; gap: 10px; min-width: 0; }
 .summary, .note, .status, .one { margin: 0; font-size: .88rem; line-height: 1.5; }
 .note { color: var(--warn); }
 .btn.small { min-height: 36px; padding: 5px 10px; font-size: .82rem; margin-left: 6px; }
 .status:empty { display: none; }
 .status.bad { color: var(--bad); }
 .muted { color: var(--muted); }
 .small { margin: 0; font-size: .78rem; }
 .members { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
 .members > li { display: flex; flex-wrap: wrap; gap: 10px 16px; justify-content: space-between; align-items: flex-start; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); min-width: 0; }
 .members > li[data-state='failed'] { border-left: 3px solid var(--bad); }
 .members > li[data-state='started'] { border-left: 3px solid var(--good); }
 .members > li[data-state='pending'] { border-left: 3px solid var(--line); }
 .what { display: grid; gap: 2px; min-width: 0; flex: 1 1 260px; }
 .what p { margin: 0; font-size: .85rem; line-height: 1.5; overflow-wrap: anywhere; }
 .meta { color: var(--muted); }
 .mono { font-family: var(--mono); font-size: .78rem; }
 .line { display: flex; gap: 8px; align-items: baseline; }
 .line.bad { color: var(--bad); }
 .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--muted); transform: translateY(-1px); }
 [data-state='started'] .dot { background: var(--good); }
 [data-state='failed'] .dot { background: var(--bad); }
 [data-state='removed'] .dot { background: var(--line); }
 .warn { color: var(--ink); padding-left: 8px; border-left: 2px solid var(--warn); }
 .actions { display: flex; flex-wrap: wrap; gap: 8px; }
 .actions .btn { min-height: 40px; padding: 7px 12px; font-size: .84rem; }
</style>
