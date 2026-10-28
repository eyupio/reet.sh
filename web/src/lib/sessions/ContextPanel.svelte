<script lang="ts">
  import { onMount } from 'svelte'
  import type { FleetCard } from '../home'
  import type { WorktreeStatus } from '../projects'
  import { changedList, decisionsLine, worktreeTargetOf } from '../sessions'
  import { worktreeStatus, type TerminalSession } from '../terminal/api'

  // The session screen's context panel: where the session is working, the files changed in its worktree, the skills line
  // of its machine and the decisions in it. Everything comes from records reet already has: the session listing, the
  // folder project, the machine's health report and sync state, and the worktree status, which is read once when the
  // panel opens on a session and again only from Refresh (never polled, R19). Nothing here is stored.
  let { session, host, status, project, skills, onkeydown, onclose, id }: {
    session: TerminalSession; host: string
    /** The machine's reporting state as the fleet strip words it, or undefined with no report yet. */
    status: FleetCard['status'] | undefined
    project: string; skills: FleetCard['skills'] | null | undefined
    onkeydown?: (event: KeyboardEvent) => void; onclose?: () => void; id: string
  } = $props()

  // svelte-ignore state_referenced_locally
  const target = worktreeTargetOf(session)
  let changes = $state<WorktreeStatus | null>(null), reading = $state(false), readFailed = $state(false)
  let generation = 0
  const life = new AbortController()
  async function readChanges() {
    if (!target || life.signal.aborted) return
    const mine = ++generation
    reading = true; readFailed = false
    try {
      const answer = await worktreeStatus(target, life.signal)
      if (mine === generation) changes = answer
    } catch {
      if (mine === generation && !life.signal.aborted) readFailed = true
    } finally {
      if (mine === generation) reading = false
    }
  }
  onMount(() => { void readChanges(); return () => life.abort() })

  const list = $derived(changes ? changedList(changes) : null)
  const statusWord = (s: FleetCard['status'] | undefined) => s === 'online' ? 'online' : s === 'paused' ? 'paused' : s === 'reconnecting' ? 'reconnecting' : s === 'offline' ? 'offline' : 'no report yet'
  // Only a machine reporting with its sync running reads as good; a paused one says so in the warn tone.
  const statusTone = (s: FleetCard['status'] | undefined) => s === 'online' ? 'good' : s === 'offline' || s === 'paused' ? 'warn' : 'muted'
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (Escape closes the panel when it is shown over the terminal) -->
<!-- data-keeps-focus: a reconnecting terminal behind the panel never takes focus out of it. -->
<aside class="context-panel" aria-label="Session context" data-keeps-focus {id} {onkeydown}>
  <!-- Shown only while the panel lies over the terminal (behind Details). -->
  {#if onclose}<button type="button" class="close-details" onclick={onclose}>Close details</button>{/if}
  <section aria-labelledby={`${id}-where`}>
    <h3 id={`${id}-where`}>Where it's working</h3>
    <dl class="card where">
      <div><dt>Machine</dt><dd>{host} <span class="state {statusTone(status)}"><span aria-hidden="true">●</span> {statusWord(status)}</span></dd></div>
      <div><dt>Project</dt><dd>{project || 'None'}</dd></div>
      <div><dt>Branch</dt>{#if session.worktreeBranch}<dd class="mono" title={session.worktreeBranch}>{session.worktreeBranch}</dd>{:else}<dd class="quiet">Not recorded</dd>{/if}</div>
      <div><dt>Worktree</dt><dd class="mono quiet">{session.worktreePath ? 'own copy' : 'the checkout'}</dd></div>
    </dl>
  </section>

  <section class="changed" aria-labelledby={`${id}-changed`}>
    <h3><span class="label" id={`${id}-changed`}>Changed files</span>{#if list}<span class="total">{list.total}</span>{/if}</h3>
    {#if !session.worktreePath}
      <p class="card note">Only sessions in their own worktree list changed files.</p>
    {:else if !target}
      <p class="card note">reet can't name this worktree, so its changes aren't listed here.</p>
    {:else}
      <div class="card files">
        {#if list && list.total === 0}<p class="note">No uncommitted changes.</p>
        {:else if list}
          <ul>{#each list.shown as path, i (i)}<li class="mono" title={path}>{path}</li>{/each}</ul>
          {#if list.more > 0}<p class="note more">and {list.more} more not listed.</p>{/if}
        {:else if readFailed}<p class="note bad">Couldn't read the changed files just now.</p>
        {:else}<p class="note">Checking…</p>{/if}
        <div class="foot">
          <button type="button" class="refresh" aria-label="Refresh changed files" disabled={reading} onclick={() => void readChanges()}>{reading ? 'Checking…' : 'Refresh'}</button>
        </div>
      </div>
    {/if}
  </section>

  <section aria-labelledby={`${id}-skills`}>
    <h3 id={`${id}-skills`}>Skills on this machine</h3>
    <div class="card skills">
      {#if skills?.tone === 'good'}<span class="tick good" aria-hidden="true">✓</span>{/if}
      <span class="skills-line {skills?.tone ?? 'muted'}">{skills === undefined ? 'Checking…' : skills === null ? 'Not checked' : skills.text}</span>
      <a class="manage" href={`/machines?view=compare&machine=${encodeURIComponent(host)}`} aria-label={`Manage skills on ${host}`}>Manage</a>
    </div>
  </section>

  <section aria-labelledby={`${id}-decisions`}>
    <h3 id={`${id}-decisions`}>Decisions in this session</h3>
    <p class="card decisions">{decisionsLine(session)}{#if session.yolo}{' '}<a href="/activity">Activity</a>{/if}</p>
  </section>
</aside>

<style>
  /* The mock-up's panel: 340 px on the sunk background, 20 px padding, each section a small uppercase label over a card. */
  .context-panel { width: 340px; flex: none; box-sizing: border-box; display: flex; flex-direction: column; gap: 18px; padding: 20px; border-left: 1px solid var(--line-soft); background: var(--bg-sunk); overflow-y: auto; min-height: 0; }
  section { min-width: 0; }
  h3 { display: flex; align-items: center; margin: 0 0 8px; font: 600 11px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--muted); }
  h3 .label { flex-grow: 1; }
  h3 .total { font-weight: 400; letter-spacing: 0; text-transform: none; }
  .card { margin: 0; border: 1px solid var(--line); border-radius: 11px; background: var(--card); font-size: 13px; }
  .where { display: flex; flex-direction: column; gap: 6px; padding: 12px 14px; }
  .where div { display: flex; justify-content: space-between; gap: 12px; min-width: 0; }
  dt { color: var(--muted); flex: none; }
  dd { margin: 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: right; }
  .mono { font-family: var(--mono); }
  .quiet { color: var(--muted); }
  .state.good { color: var(--good); }
  .state.warn { color: var(--warn); }
  .state.muted { color: var(--muted); }
  .note { margin: 0; padding: 12px 14px; color: var(--muted); }
  .note.bad { color: var(--bad); }
  .files { overflow: hidden; }
  .files ul { list-style: none; margin: 0; padding: 0; }
  .files li { padding: 9px 14px; border-bottom: 1px solid var(--line-soft); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .files .more { padding-top: 9px; padding-bottom: 9px; border-bottom: 1px solid var(--line-soft); }
  .foot { display: flex; justify-content: flex-end; padding: 4px 6px; }
  .refresh { min-height: 32px; padding: 0 10px; border: 0; border-radius: 7px; background: none; color: var(--muted); font: 500 12px var(--sans); cursor: pointer; }
  .refresh:hover:not(:disabled) { background: var(--tint); color: var(--ink); }
  .refresh:disabled { cursor: default; }
  .skills { display: flex; align-items: center; gap: 10px; padding: 12px 14px; }
  .skills-line { flex-grow: 1; min-width: 0; }
  .skills-line.warn { color: var(--warn); }
  .skills-line.muted { color: var(--muted); }
  .tick.good { color: var(--good); }
  .manage { color: var(--muted); text-decoration: none; }
  .manage:hover { color: var(--accent-text); }
  .decisions { padding: 12px 14px; color: var(--ink-soft); }
  .decisions a { color: var(--accent-text); }
  .refresh:focus-visible, .manage:focus-visible, .decisions a:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .close-details { display: none; align-self: flex-end; min-height: 44px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card); color: var(--ink); font: 500 14px var(--sans); cursor: pointer; }
  .close-details:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  @media (pointer: coarse) { .refresh, .manage { min-height: 44px; display: inline-flex; align-items: center; } .refresh { padding: 0 14px; } }
</style>
