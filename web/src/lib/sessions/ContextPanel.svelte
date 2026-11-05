<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { get } from 'svelte/store'
  import { stateWord, type FleetCard } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import { pendingPermissions, pendingQuestions, toolLabel, type ChatState } from '../chat'
  import type { WorktreeStatus } from '../projects'
  import { changedList, decisionsLine, worktreeTargetOf } from '../sessions'
  import { inspectSession, worktreeStatus, type TerminalSession } from '../terminal/api'
  import { chatInsight, elapsedWord, insightReason, type InsightFile, type InsightView, type SessionInsight } from '../sessionInsights'
  import { insightsAuto, rememberInsight, sessionInsights } from './insightStore'
  import InsightIcon from './InsightIcon.svelte'
  import WorkspaceFiles from './WorkspaceFiles.svelte'
  import GitHubPane from './GitHubPane.svelte'
  import TerminalIcon from '../terminal/TerminalIcon.svelte'

  let { session, host, status, project, skills, now, chat, supported = false, docked = false, onterminal, onkeydown, onclose, id }: {
    session: TerminalSession; host: string; status: FleetCard['status'] | undefined
    project: string; skills: FleetCard['skills'] | null | undefined; now: number; chat?: ChatState; supported?: boolean
    /** The panel sits beside the terminal: it has a resize separator and its own collapse control. */
    docked?: boolean
    onterminal?: () => void; onkeydown?: (event: KeyboardEvent) => void; onclose?: () => void; id: string
  } = $props()
  type Tab = 'overview' | 'changes' | 'files' | 'github'
  const tabs: { key: Tab; label: string }[] = [{ key: 'overview', label: 'Overview' }, { key: 'changes', label: 'Changes' }, { key: 'files', label: 'Files' }, { key: 'github', label: 'GitHub' }]
  let tab = $state<Tab>('overview'), auto = $state(true)
  let workspace = $state<SessionInsight | null>(null), github = $state<SessionInsight | null>(null), listing = $state<SessionInsight | null>(null), preview = $state<SessionInsight | null>(null)
  let folder = $state(''), selected = $state(''), previewView = $state<'file' | 'diff'>('file')
  let pending = $state<Record<string, boolean>>({}), errors = $state<Record<string, string>>({})
  const requests = new Map<string, AbortController>()
  let alive = true, lastLocal = 0, lastGitHub = 0
  const canRead = $derived(supported && status !== 'offline' && status !== 'paused')
  const view = $derived(tab === 'github' ? 'github' : tab === 'files' ? 'files' : 'status')
  const current = $derived(tab === 'github' ? github : tab === 'files' ? listing : workspace)
  const chatAttention = $derived(chat?.connection === 'open' ? ({ working: 'working', waiting: 'waiting', starting: 'starting', idle: 'quiet', error: 'failed', ended: 'finished', cancelled: 'quiet' } as const)[chat.status.state as 'working' | 'waiting' | 'starting' | 'idle' | 'error' | 'ended' | 'cancelled'] : undefined)
  const sessionState = $derived(stateWord(chatAttention ? { ...session, attention: chatAttention, attentionSource: 'chat' } : session))
  const activity = $derived(chatInsight(chat))
  const target = $derived(worktreeTargetOf(session))
  const targetKey = $derived(target ? `${target.agentId}:${target.project}:${target.repo}:${target.run}` : '')
  let legacyReadFor = ''
  $effect(() => { const key = targetKey; if (!supported && key && key !== legacyReadFor) { legacyReadFor = key; untrack(() => { legacy = null; void readLegacy() }) } })
  let legacy = $state<WorktreeStatus | null>(null), legacyBusy = $state(false), legacyFailed = $state(false)
  const legacyList = $derived(legacy ? changedList(legacy) : null)
  // Only sequence and time are stored, scoped to this tab and opaque session id.
  const visitKey = $derived(`reet-session-visit:${session.id}`)
  let baseline = $state<{ seq: number; at: number; activityAt?: number } | null>(null), visitReady = false
  const newEvents = $derived(baseline && chat && chat.lastSeq >= baseline.seq ? chat.lastSeq - baseline.seq : 0)
  const terminalAdvanced = $derived(!session.structured && baseline && (session.activityAt ?? 0) > (baseline.activityAt ?? baseline.at))
  const attention = $derived(chat && chat.connection === 'open' ? pendingQuestions(chat)[0]?.question.message || pendingPermissions(chat)[0]?.title || (chat.status.state === 'waiting' ? 'The agent is waiting for your answer.' : '') : session.pendingQuestion?.message || session.pendingPermission?.title || (session.attention === 'waiting' ? 'The agent is waiting for your answer.' : ''))
  const branch = $derived(workspace?.branch || session.worktreeBranch || '')
  const freshAt = $derived(current?.checkedAt)
  const stale = $derived(!!freshAt && (now - freshAt > (tab === 'github' ? 180_000 : 90_000) || status === 'offline' || !!errors[view]))
  // The overview never changes height when the agent moves on: each of these fills a slot of fixed size (one line, or a box that scrolls), so the sections after them stay where they are.
  const headline = $derived(attention ? 'Your input is needed' : activity.active[0] ? activity.active[0].title || toolLabel(activity.active[0].toolKind) : session.attention === 'working' ? 'The agent is working' : 'Session at a glance')
  const subline = $derived(`${chat?.connection === 'reconnecting' ? 'Showing the last known activity' : sessionState.source}. Last activity ${elapsedWord(session.activityAt, now)}.`)
  const countsLine = $derived(!chat || chat.connection === 'connecting' ? 'Loading conversation activity…' : chat.truncated ? 'Counts exclude earlier activity that is not loaded.' : 'Counts cover the loaded conversation.')
  const running = $derived(activity.active[activity.active.length - 1])
  const errorDetail = $derived(chat?.status.state === 'error' ? chat.status.detail : '')

  function sentence(text: string) { return /[.!?]$/.test(text) ? text : `${text}.` }
  function readVisit() {
    try { const v = JSON.parse(sessionStorage.getItem(visitKey) || 'null'); if (v && Number.isSafeInteger(v.seq) && v.seq >= 0 && Number.isFinite(v.at)) baseline = v } catch { /* optional */ }
  }
  function saveVisit() {
    if (!visitReady || document.hidden || (session.structured && (!chat || chat.connection !== 'open'))) return
    try { sessionStorage.setItem(visitKey, JSON.stringify({ seq: chat?.lastSeq ?? 0, at: Date.now(), activityAt: session.activityAt ?? 0 })) } catch { /* optional */ }
  }
  $effect(() => { const seq = chat?.lastSeq, at = session.activityAt; if (!session.structured || chat?.connection === 'open') untrack(saveVisit) })
  async function read(which: InsightView, path = '') {
    if (!canRead || !alive) return
    const key = which === 'file' || which === 'diff' ? 'preview' : which
    requests.get(key)?.abort()
    const life = new AbortController(); requests.set(key, life)
    pending[key] = true; errors[key] = ''
    if (which === 'status') lastLocal = Date.now()
    if (which === 'github') lastGitHub = Date.now()
    if (key === 'preview') preview = null
    try {
      const answer = await inspectSession(session.id, which, path, life.signal)
      if (!alive || life.signal.aborted) return
      if (answer.reason && (which === 'status' ? workspace && !workspace.reason : which === 'github' ? github?.pull : false)) { errors[key] = insightReason(answer.reason); return }
      if (which === 'status') { workspace = answer; lastLocal = Date.now(); rememberInsight(session.id, 'status', answer) }
      else if (which === 'github') { github = answer; lastGitHub = Date.now(); rememberInsight(session.id, 'github', answer) }
      else if (which === 'files') listing = answer
      else preview = answer
    } catch (e) { if (alive && !life.signal.aborted) errors[key] = e instanceof Error ? e.message : 'Could not read this view. Try again.' }
    finally { if (alive && !life.signal.aborted) pending[key] = false }
  }
  async function readLegacy() {
    if (!target || legacyBusy) return
    legacyBusy = true; legacyFailed = false
    const life = new AbortController(); requests.set('legacy', life)
    try { const data = await worktreeStatus(target, life.signal); if (alive) legacy = data }
    catch { if (alive) legacyFailed = true }
    finally { if (alive) legacyBusy = false }
  }
  function refresh() { if (!supported) void readLegacy(); else if (selected) void read(previewView, selected); else void read(view, tab === 'files' ? folder : '') }
  function pick(next: Tab) {
    tab = next; selected = ''; preview = null
    requests.get('preview')?.abort(); pending.preview = false
    if (next === 'github' && (!github || Date.now() - lastGitHub > 120_000)) void read('github')
    if (next === 'files') void read('files', folder)
    if ((next === 'changes' || next === 'overview') && (!workspace || Date.now() - lastLocal > 15_000)) void read('status')
    if (!supported && next === 'changes' && !legacy) void readLegacy()
  }
  function openFile(file: InsightFile) {
    if (file.directory) { folder = file.path; void read('files', folder); return }
    selected = file.path; previewView = tab === 'changes' && file.status !== '??' ? 'diff' : 'file'
    void read(previewView, selected)
  }
  function parentFolder() { folder = folder.split('/').slice(0, -1).join('/'); selected = ''; preview = null; void read('files', folder) }
  function tabKeys(event: KeyboardEvent, index: number) {
    let next = index
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length
    else if (event.key === 'ArrowLeft') next = (index + tabs.length - 1) % tabs.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = tabs.length - 1
    else return
    event.preventDefault(); pick(tabs[next].key); document.getElementById(`${id}-tab-${tabs[next].key}`)?.focus()
  }
  $effect(() => { if (canRead) untrack(() => { if (!workspace && !pending.status) void read('status') }) })
  // Live updates is the one switch for automatic reads: the navigation's background PR reads follow it too.
  $effect(() => { insightsAuto.set(auto) })
  onMount(() => {
    const cached = get(sessionInsights)[session.id]; workspace = cached?.status ?? null; github = cached?.github ?? null
    readVisit(); visitReady = true; saveVisit()
    if (supported) void read('status'); else if (target) void readLegacy()
    const visibility = () => {
      if (document.hidden) return
      readVisit(); saveVisit()
      if (auto && canRead) { void read('status'); if (tab === 'github') void read('github') }
    }
    const timer = setInterval(() => {
      if (!auto || document.hidden || !canRead) return
      if (!pending.status && Date.now() - lastLocal >= (session.attention === 'working' ? 15_000 : 60_000)) void read('status')
      if (tab === 'github' && !pending.github && Date.now() - lastGitHub >= 120_000) void read('github')
    }, 5000)
    document.addEventListener('visibilitychange', visibility)
    return () => { saveVisit(); alive = false; clearInterval(timer); document.removeEventListener('visibilitychange', visibility); requests.forEach(r => r.abort()) }
  })
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (the parent owns sheet focus and Escape) -->
<aside class="context-panel" aria-label="Session context" data-keeps-focus {id} {onkeydown}>
  <header class="panel-head">
    <div class="panel-title"><strong>Session workspace</strong>{#if onclose && !docked}<button type="button" class="close-details" onclick={onclose}>Close details</button>{/if}</div>
    <div class="panel-tools"><span class="connection" class:warn={status !== 'online'}><i></i>{status === 'online' ? 'Machine online' : status === 'offline' ? 'Machine offline' : status === 'paused' ? 'Machine paused' : 'Checking connection'}</span><button class="icon-button" aria-label="Refresh session workspace" title="Refresh session workspace" disabled={!!pending[view] || (supported && !canRead)} aria-busy={!!pending[view] || legacyBusy} onclick={refresh}><InsightIcon name="refresh" /></button>{#if onclose && docked}<button class="icon-button collapse-panel" aria-label="Collapse session workspace" title="Collapse session workspace" onclick={onclose}><TerminalIcon name="panelRight" /></button>{/if}</div>
    <div class="tabs" role="tablist" aria-label="Session workspace views">{#each tabs as item, i}<button id={`${id}-tab-${item.key}`} role="tab" aria-selected={tab === item.key} aria-controls={`${id}-view`} tabindex={tab === item.key ? 0 : -1} onclick={() => pick(item.key)} onkeydown={e => tabKeys(e, i)}><InsightIcon name={item.key} /><span>{item.label}</span>{#if item.key === 'changes' && workspace?.changed}<small>{workspace.changed}</small>{/if}</button>{/each}</div>
  </header>
  <div class="panel-body" id={`${id}-view`} role="tabpanel" aria-labelledby={`${id}-tab-${tab}`} tabindex="0">
    {#if tab === 'overview'}
      <section class="session-summary" aria-label="Current session status">
        <div class="eyebrow">Right now <span class="state" data-status={sessionStatus(chatAttention ? { ...session, attention: chatAttention, attentionSource: 'chat' } : session)}>{chat?.connection === 'reconnecting' ? 'Reconnecting' : sessionState.word}</span></div>
        <h3 title={headline}>{headline}</h3>
        <p class="quiet one-line" title={subline}>{subline}</p>
        {#if attention}<div class="notice warn"><p title={attention}>{attention}</p><button class="notice-action" aria-label="Review and answer in session" title="Review and answer in session" onclick={() => onterminal?.()}>Review <InsightIcon name="arrow" /></button></div>
        {:else if errorDetail}<div class="notice bad"><p title={errorDetail}>{errorDetail}</p></div>
        {:else if baseline && ((chat && newEvents > 0) || terminalAdvanced)}<div class="notice"><InsightIcon name="clock" /><span><strong>Since your last visit</strong>{terminalAdvanced ? 'New terminal output' : `${newEvents} new event${newEvents === 1 ? '' : 's'}`} · last here {elapsedWord(baseline.at, now)}{#if chat?.truncated}<small>Earlier activity is outside the loaded history.</small>{/if}</span></div>
        {:else}<div class="notice calm"><p>Nothing is waiting for you.</p></div>{/if}
        {#if session.structured}<div class="facts"><span><b>{activity.done.length}</b> tools done</span><span class:bad={activity.failed.length > 0}><b>{activity.failed.length}</b> failed</span><span><b>{activity.active.length}</b> active</span></div><p class="quiet one-line" title={countsLine}>{countsLine}</p>{:else}<p class="quiet">Terminal state comes from process and output activity. Agent steps are available in Gaffer chats.</p>{/if}
      </section>
      {#if session.structured}
        <section class="live" aria-label="Agent activity">
          <div class="slot"><div class="slot-head"><h4>In progress</h4>{#if activity.active.length > 1}<span class="count">{activity.active.length} running</span>{/if}</div><div class="slot-box tool-row">{#if running}<div class="activity-row"><InsightIcon name="clock" /><span><span class="tool-title">{running.title || toolLabel(running.toolKind)}</span><small>{running.command || running.path || toolLabel(running.toolKind)}</small></span></div>{:else}<p class="slot-empty">Nothing is running right now.</p>{/if}</div></div>
          <div class="slot fails">{#if activity.failed.length}<details class="failed-tools"><summary>{activity.failed.length} tool{activity.failed.length === 1 ? '' : 's'} reported failure</summary><div class="failed-list"><p class="quiet">These are reported tool results; the agent may have recovered later.</p>{#each activity.failed.slice(-5) as tool}<p>{tool.title || toolLabel(tool.toolKind)}</p>{/each}</div></details>{:else}<p class="slot-empty">No tool failures reported.</p>{/if}</div>
          <div class="slot"><div class="slot-head"><h4 id={`${id}-latest`}>Latest agent update</h4><button class="text-action" onclick={() => onterminal?.()}>Read in conversation <InsightIcon name="arrow" /></button></div><!-- svelte-ignore a11y_no_noninteractive_tabindex (the read-only update scrolls with the keyboard) --><div class="slot-box latest-text" role="region" aria-labelledby={`${id}-latest`} tabindex="0">{#if activity.latest}{activity.latest.slice(0, 1800)}{activity.latest.length > 1800 ? '…' : ''}{:else}<span class="slot-empty">The agent has not posted an update yet.</span>{/if}</div></div>
        </section>
      {/if}
      <section class="change-summary"><h4>Workspace changes</h4>{#if workspace && !workspace.reason}<button onclick={() => pick('changes')}><InsightIcon name={workspace.conflicts ? 'conflict' : 'changes'} /><span><strong>{workspace.changed}{workspace.changedMore ? "+" : ""} changed files</strong><small>{workspace.conflicts ? `${workspace.conflicts} conflict${workspace.conflicts === 1 ? ' needs' : 's need'} attention` : workspace.upstream ? `${workspace.ahead} ahead · ${workspace.behind} behind upstream` : 'No upstream comparison reported'}</small></span><InsightIcon name="arrow" /></button>{:else}<p class="quiet">{pending.status ? 'Checking the workspace…' : workspace?.reason ? insightReason(workspace.reason) : supported ? errors.status || 'Workspace status has not been read.' : 'Update reet on this machine for checkout changes, files and GitHub status.'}</p>{/if}</section>
      <section aria-labelledby={`${id}-where`}><h4 id={`${id}-where`}>Where it's working</h4><dl class="where"><div><dt>Machine</dt><dd>{host} <span class="state" class:warn={status === 'offline' || status === 'paused'} class:good={status === 'online'}>● {status || 'no report yet'}</span></dd></div><div><dt>Project</dt><dd>{project || 'None'}</dd></div><div><dt>Branch</dt><dd class="mono" title={branch}>{branch || 'Not recorded'}</dd></div><div><dt>Worktree</dt><dd>{session.worktreePath ? 'own copy' : 'the checkout'}</dd></div><div><dt>Started</dt><dd>{elapsedWord(session.createdAt, now)}</dd></div></dl>{#if session.worktreePath || session.dir}<p class="workspace-path" title={session.worktreePath || session.dir}>{session.worktreePath || session.dir}</p>{/if}</section>
      <details class="session-details"><summary>Skills & session decisions</summary><h4>Skills on this machine</h4><div class="skills"><span class="skills-line">{skills === undefined ? 'Checking…' : skills === null ? 'Not checked' : skills.text}</span><a aria-label={`Manage skills on ${host}`} href={`/machines?view=compare&machine=${encodeURIComponent(host)}`}>Manage</a></div><h4>Decisions in this session</h4><p class="decisions">{decisionsLine(session)}{#if session.yolo} <a href="/activity">Activity</a>{/if}</p></details>
    {:else if !supported}
      {#if tab === 'changes'}<section aria-label="Changed files"><h3>Changed files</h3>{#if !session.worktreePath}<p class="empty">Update reet on this machine to inspect changes in the checkout.</p>{:else if !target}<p class="empty">reet can't name this worktree, so its changes aren't listed here.</p>{:else if legacyList}<p><span class="total">{legacyList.total}</span> changed files</p><ul class="legacy-files">{#each legacyList.shown as path}<li>{path}</li>{/each}</ul>{#if legacyList.more}<p class="quiet">and {legacyList.more} more not listed.</p>{/if}{:else}<p class="empty">{legacyFailed ? "Couldn't read the changed files just now." : 'Checking…'}</p>{/if}{#if target}<button class="secondary" disabled={legacyBusy} aria-busy={legacyBusy} onclick={() => void readLegacy()}>Refresh changed files</button>{/if}</section>{:else}<p class="empty">Update reet on this machine to use {tab === 'github' ? 'GitHub status' : 'read-only file browsing'}.</p>{/if}
    {:else}
      {#if !canRead}<p class="callout">The machine is offline or unavailable. Showing the last known view. Refresh when it reconnects.</p>{/if}
      {#if errors[view]}<p class="callout bad" role="status">{sentence(errors[view])}{current ? ' The previous result is shown below.' : ''}</p>{/if}
      {#if selected}
        <div class="preview-head"><button class="text-action" onclick={() => { selected = ''; requests.get('preview')?.abort(); pending.preview = false }}>← Back to {tab === 'changes' ? 'changes' : 'files'}</button><strong>{selected}</strong>{#if tab === 'changes'}<div class="preview-modes"><button aria-pressed={previewView === 'diff'} onclick={() => { previewView = 'diff'; void read('diff', selected) }}>Diff</button><button aria-pressed={previewView === 'file'} onclick={() => { previewView = 'file'; void read('file', selected) }}>File</button></div>{/if}</div>
        {#if pending.preview}<p class="empty" role="status">Reading {previewView}…</p>{:else if errors.preview}<p class="callout bad" role="status">{errors.preview}</p>{:else if preview?.reason}<p class="empty">{insightReason(preview.reason)}</p>{:else if preview?.binary}<p class="empty">Binary file. Preview is unavailable.</p>{:else if preview}<!-- svelte-ignore a11y_no_noninteractive_tabindex (the read-only preview scrolls with the keyboard) --><div class="preview" role="region" aria-label={`${previewView === 'diff' ? 'Diff' : 'File'} preview for ${selected}`} tabindex="0"><pre>{#if previewView === 'diff'}{#each (preview.text || '').split('\n') as line}<span class:addition={line.startsWith('+')} class:deletion={line.startsWith('-')}>{line || ' '}</span>{/each}{:else}{preview.text || '(empty file)'}{/if}</pre></div>{#if preview.more}<p class="quiet">Preview truncated. Open the file on the machine for the full content.</p>{/if}{#if previewView === 'diff' && !preview.text}<p class="quiet">No diff against HEAD was reported. Untracked files can be viewed using File.</p>{/if}{/if}
      {:else if pending[view] && !current}<p class="empty" role="status">Checking {tab === 'github' ? 'GitHub' : 'the workspace'}…</p>
      {:else if current?.reason}<p class="empty">{insightReason(current.reason)}</p>
      {:else if tab === 'github' && github?.pull}<GitHubPane data={github} />
      {:else if tab === 'changes' && workspace}<h3>Changed files</h3><WorkspaceFiles data={workspace} changes onopen={openFile} />
      {:else if tab === 'files'}<div class="folder-head">{#if folder}<button class="text-action" onclick={parentFolder}>← Up</button>{/if}<strong title={folder || '/'}>{folder || 'Workspace files'}</strong></div>{#if listing}<WorkspaceFiles data={listing} onopen={openFile} />{/if}
      {:else if !errors[view]}<p class="empty">Refresh to read this view.</p>{/if}
    {/if}
  </div>
  <footer><span title={freshAt ? new Date(freshAt).toLocaleString() : ''}>{pending[view] ? 'Refreshing…' : freshAt ? `${stale ? 'Last known · ' : 'Checked '}${elapsedWord(freshAt, now)}` : supported ? 'No workspace snapshot yet' : 'Machine update needed'}</span>{#if supported}<label><input type="checkbox" bind:checked={auto} />Live updates</label>{/if}<button class="terminal-action" aria-label="Session" onclick={() => onterminal?.()} title="Focus the active session"><InsightIcon name="terminal" /><span>Session</span></button></footer>
</aside>

<style>
  .context-panel{width:100%;flex:none;box-sizing:border-box;display:flex;flex-direction:column;min-height:0;min-width:0;border-left:1px solid var(--line-soft);background:var(--bg-sunk);overflow:hidden;color:var(--ink)}.panel-head{flex:none;padding:16px 16px 0;border-bottom:1px solid var(--line)}.panel-title,.panel-tools{display:flex;align-items:center;justify-content:space-between;gap:8px}.panel-title>strong{font-size:14px;font-weight:600}.panel-tools{margin:9px 0}.connection{display:flex;align-items:center;gap:6px;font-size:14px;color:var(--muted)}.connection i{width:6px;height:6px;border-radius:50%;background:var(--good)}.connection.warn i{background:var(--warn)}button{font:inherit;cursor:pointer}button:disabled{opacity:.5;cursor:default}.icon-button{display:grid;place-items:center;width:28px;height:28px;background:none;color:var(--muted);border:0;border-radius:6px}.icon-button:hover{background:var(--tint);color:var(--ink)}.tabs{display:flex;margin:0 -5px}.tabs button{position:relative;display:flex;flex:1;min-width:0;justify-content:center;align-items:center;flex-wrap:nowrap;gap:4px;padding:12px 3px;border:0;border-bottom:2px solid transparent;background:none;color:var(--muted);font-size:14px}.tabs button[aria-selected=true]{border-bottom-color:var(--accent);color:var(--ink)}.tabs button:hover{background:var(--tint)}.tabs small{position:absolute;top:2px;right:3px;font:14px var(--mono);padding:1px 4px;border-radius:4px;background:var(--tint)}.panel-body{flex:1;min-height:0;overflow:auto;padding:18px 16px;scrollbar-gutter:stable;outline-offset:-3px}.panel-body>section{margin-bottom:20px}.panel-body>section+section{padding-top:17px;border-top:1px solid var(--line-soft)}h3{font:600 16px/1.5 var(--sans);margin:10px 0 8px;overflow-wrap:anywhere}h4{margin:0 0 10px;font:600 14px var(--sans);letter-spacing:.07em;text-transform:uppercase;color:var(--muted)}.eyebrow{display:flex;justify-content:space-between;align-items:center;font-size:14px;color:var(--muted)}.state{border:1px solid var(--status-edge);border-radius:99px;padding:3px 8px;font-size:14px;font-weight:600;color:var(--status);background:var(--status-wash)}.bad{color:var(--bad)}.quiet{font-size:14px;line-height:1.65;color:var(--muted);margin:8px 0}.session-summary h3,.one-line{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.notice{display:flex;align-items:center;gap:10px;box-sizing:border-box;height:60px;margin-top:14px;padding:0 12px;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--tint);color:var(--ink-soft);font-size:14px;line-height:1.5}.notice>p{flex:1;min-width:0;margin:0;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;overflow:hidden;overflow-wrap:anywhere}.notice>span{flex:1;min-width:0}.notice strong{display:block;color:var(--ink)}.notice small{display:block}.notice.warn{border-color:color-mix(in srgb,var(--warn) 40%,transparent);background:color-mix(in srgb,var(--warn) 7%,transparent);color:var(--ink);font-size:14px}.notice.bad{border-color:color-mix(in srgb,var(--bad) 40%,transparent);background:none;color:var(--bad);font-size:14px}.notice.calm{border-color:transparent;background:none;color:var(--muted)}.notice-action{flex:none;display:inline-flex;align-items:center;gap:5px;min-height:34px;padding:0 10px;background:var(--accent);color:var(--accent-ink);border:0;border-radius:7px;font-size:14px}.facts{display:flex;gap:6px;margin-top:14px}.facts>span{flex:1;border:1px solid var(--line);background:var(--card);border-radius:8px;padding:9px 6px;text-align:center;font-size:14px;color:var(--muted)}.facts>span.bad{color:var(--bad)}.facts b{display:block;font:500 20px var(--mono);color:var(--ink);margin-bottom:3px}.live{display:grid;grid-template-columns:minmax(0,1fr);gap:12px}.slot-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-height:30px}.slot-head h4{margin:0}.count{font-size:14px;color:var(--muted)}.slot-box{display:flex;align-items:center;box-sizing:border-box;min-width:0}.tool-row{height:44px}.slot-box>.activity-row{width:100%;min-width:0;padding:0}.tool-title{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.slot-empty{margin:0;font-size:14px;color:var(--muted)}.fails{min-height:32px;display:flex;align-items:center}.fails>.failed-tools{width:100%;margin:0}.failed-list{max-height:120px;overflow:auto}.latest-text{display:block;height:84px;overflow:auto;white-space:pre-wrap;font-size:14px;line-height:1.75;overflow-wrap:anywhere}.text-action{display:inline-flex;align-items:center;gap:7px;border:0;background:none;padding:5px 0;min-height:30px;color:var(--ink);font-size:14px}.activity-row{display:flex;align-items:flex-start;gap:8px;padding:7px 0;font-size:14px}.activity-row>span{min-width:0;flex:1;overflow-wrap:anywhere}.activity-row small{display:block;margin-top:4px;font:14px var(--mono);color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.change-summary>button{display:flex;align-items:center;gap:10px;width:100%;text-align:left;padding:12px;border:1px solid var(--line);border-radius:9px;background:var(--card);color:var(--ink)}.change-summary>button>span{flex:1;min-width:0}.change-summary>button,.change-summary>.quiet{box-sizing:border-box;min-height:76px}.change-summary>.quiet{display:flex;align-items:center;margin:0}.change-summary strong{font-size:14px;font-weight:500}.change-summary small{display:block;color:var(--muted);font-size:14px;margin-top:5px}.where{display:grid;gap:8px;margin:0;font-size:14px}.where>div{display:flex;justify-content:space-between;gap:12px;min-width:0}dt{color:var(--muted);flex:none}dd{margin:0;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:right}.mono,.workspace-path{font-family:var(--mono)}.workspace-path{font-size:14px;color:var(--muted);overflow-wrap:anywhere;line-height:1.6}.session-details{border-top:1px solid var(--line-soft);padding-top:10px;font-size:14px;line-height:1.7}.session-details summary,.failed-tools summary{min-height:32px;cursor:pointer;color:var(--muted);font-size:14px}.session-details h4{margin-top:16px}.skills{display:flex;justify-content:space-between;gap:10px}.session-details a{color:var(--ink);text-decoration:underline}.failed-tools{margin-bottom:16px;color:var(--bad);font-size:14px}.failed-tools summary{color:var(--bad)}footer{display:flex;align-items:center;gap:8px;padding:8px 12px;min-height:34px;border-top:1px solid var(--line);font-size:14px;color:var(--muted)}footer>span{flex:1;min-width:0}footer label{display:flex;align-items:center;gap:4px;white-space:nowrap}footer input{accent-color:var(--accent);width:12px;height:12px;margin:0}.terminal-action{display:flex;align-items:center;gap:4px;padding:5px;border:0;border-radius:5px;background:none;color:var(--muted);font-size:14px}.terminal-action:hover{background:var(--tint);color:var(--ink)}.close-details{display:none;min-height:32px;padding:5px 8px;border:1px solid var(--line);border-radius:7px;background:var(--card);color:var(--ink);font-size:14px}.empty{font-size:14px;line-height:1.8;color:var(--muted);padding:14px 0;margin:0}.callout{font-size:14px;line-height:1.7;padding:11px;border:1px solid var(--line);border-radius:8px;color:var(--warn);margin:0 0 14px}.callout.bad{color:var(--bad)}.folder-head{display:flex;align-items:center;gap:12px;min-width:0;font-size:14px}.folder-head strong{overflow:hidden;white-space:nowrap;text-overflow:ellipsis;font:500 14px var(--mono)}.preview-head{display:grid;gap:8px;min-width:0}.preview-head>strong{font:500 14px var(--mono);overflow-wrap:anywhere}.preview-modes{display:flex;gap:4px}.preview-modes button,.secondary{min-height:30px;font-size:14px;border:1px solid var(--line);border-radius:6px;padding:5px 10px;color:var(--muted);background:var(--card)}.preview-modes button[aria-pressed=true]{color:var(--accent-text);background:var(--tint)}.preview{max-height:65vh;overflow:auto;border:1px solid var(--line);border-radius:7px;background:var(--card);margin-top:12px}.preview pre{font:14px/1.7 var(--mono);padding:10px;margin:0;tab-size:2;white-space:pre}.preview pre span{display:block;min-height:1.7em;white-space:pre}.preview pre span.addition{color:var(--good);background:color-mix(in srgb,var(--good) 8%,transparent)}.preview pre span.deletion{color:var(--bad);background:color-mix(in srgb,var(--bad) 8%,transparent)}.legacy-files{list-style:none;padding:0}.legacy-files li{font:14px/1.7 var(--mono);overflow-wrap:anywhere;padding:6px 0;border-bottom:1px solid var(--line-soft)}:is(button,a,input,summary):focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.icon-button,.tabs button,.text-action,.terminal-action,.preview-modes button,.secondary,.close-details,.session-details summary,.failed-tools summary,.notice-action{min-height:44px}.icon-button,.terminal-action,.text-action,.preview-modes button,.notice-action{min-width:44px}.fails{min-height:44px}.session-details a{display:inline-flex;align-items:center;min-width:44px;min-height:44px}footer label{min-height:44px}}@media(max-width:400px){.context-panel{width:100%}.tabs button{font-size:14px}.tabs :global(svg){width:13px}.panel-body{padding:14px}.terminal-action span{display:none}}
</style>
