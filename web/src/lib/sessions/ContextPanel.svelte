<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { get } from 'svelte/store'
  import type { FleetCard } from '../home'
  import type { ChatState } from '../chat'
  import type { WorktreeStatus } from '../projects'
  import { changedList, worktreeTargetOf } from '../sessions'
  import { inspectSession, worktreeStatus, type TerminalSession } from '../terminal/api'
  import { elapsedWord, insightReason, type InsightFile, type InsightView, type SessionInsight } from '../sessionInsights'
  import { insightsAuto, pullReadAge, pullReadBusy, pullReadingIds, rememberInsight, sessionInsights, trackPullRead } from './insightStore'
  import { githubReadDue } from './pullFacts'
  import { PANEL_TAB_KEY, readPanelTab, type PanelTab } from './panelTab'
  import InsightIcon from './InsightIcon.svelte'
  import { pullVerdict } from './pullVerdict'
  import OverviewTab from './OverviewTab.svelte'
  import WorkspaceFiles from './WorkspaceFiles.svelte'
  import GitHubPane from './GitHubPane.svelte'
  import TerminalIcon from '../terminal/TerminalIcon.svelte'
  import GafferProgress from '../terminal/GafferProgress.svelte'
  import GafferCompanionAvatar from '../GafferCompanionAvatar.svelte'
  import { gafferChatEntry } from '../gafferContext'
  import { chatToolName, terminalClosed } from '../terminal/api'
  import type { ChatProgressState } from '../gafferProgress'

  let { session, host, status, project, skills, now, chat, chatProgress, supported = false, docked = false, onterminal, onkeydown, onclose, id }: {
    session: TerminalSession; host: string; status: FleetCard['status'] | undefined
    project: string; skills: FleetCard['skills'] | null | undefined; now: number; chat?: ChatState; chatProgress?: ChatProgressState; supported?: boolean
    /** The panel sits beside the terminal: it has a resize separator and its own collapse control. */
    docked?: boolean
    onterminal?: () => void; onkeydown?: (event: KeyboardEvent) => void; onclose?: () => void; id: string
  } = $props()
  const tabs: { key: PanelTab; label: string }[] = [{ key: 'overview', label: 'Overview' }, { key: 'changes', label: 'Changes' }, { key: 'files', label: 'Files' }, { key: 'github', label: 'GitHub' }]
  // The chosen view is a browser preference, so a new session opens on the same tab.
  function savedTab(): PanelTab {
    try { return readPanelTab(localStorage.getItem(PANEL_TAB_KEY)) } catch { return 'overview' }
  }
  let tab = $state<PanelTab>(savedTab()), auto = $state(true)
  let workspace = $state<SessionInsight | null>(null), github = $state<SessionInsight | null>(null), listing = $state<SessionInsight | null>(null), preview = $state<SessionInsight | null>(null)
  let folder = $state(''), selected = $state(''), previewView = $state<'file' | 'diff'>('file')
  let pending = $state<Record<string, boolean>>({}), errors = $state<Record<string, string>>({})
  const requests = new Map<string, AbortController>()
  let alive = true, lastLocal = 0, lastGitHub = 0
  const canRead = $derived(supported && status !== 'offline' && status !== 'paused')
  const view = $derived(tab === 'github' ? 'github' : tab === 'files' ? 'files' : 'status')
  const current = $derived(tab === 'github' ? github : tab === 'files' ? listing : workspace)
  const target = $derived(worktreeTargetOf(session))
  const targetKey = $derived(target ? `${target.agentId}:${target.project}:${target.repo}:${target.run}` : '')
  let legacyReadFor = ''
  $effect(() => { const key = targetKey; if (!supported && key && key !== legacyReadFor) { legacyReadFor = key; untrack(() => { legacy = null; void readLegacy() }) } })
  let legacy = $state<WorktreeStatus | null>(null), legacyBusy = $state(false), legacyFailed = $state(false)
  const legacyList = $derived(legacy ? changedList(legacy) : null)
  const freshAt = $derived(current?.checkedAt)
  const stale = $derived(!!freshAt && (now - freshAt > (tab === 'github' ? 180_000 : 90_000) || status === 'offline' || !!errors[view]))

  // The tab says what the pull request needs in words as well as colour. A muted verdict (a closed pull request) says nothing, and "good" is honest: merged, or checks passed, never "ready to merge".
  const githubTone = $derived(github?.pull ? pullVerdict(github.pull).tone : 'muted')
  const githubState = $derived(githubTone === 'bad' ? 'needs attention' : githubTone === 'warn' ? 'in progress' : githubTone === 'good' ? (github?.pull?.state === 'MERGED' ? 'merged' : 'checks passed') : '')

  function sentence(text: string) { return /[.!?]$/.test(text) ? text : `${text}.` }
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
      const call = inspectSession(session.id, which, path, life.signal)
      if (which === 'github') trackPullRead(session.id, call)
      const answer = await call
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
  // Every automatic GitHub read goes through here: not while Live updates is off, not while the navigation's read for this session is in flight, and not within a cadence of the last read by either.
  function backgroundGitHub() {
    // The shared store decides, not the local copy: on a warm store `github` may not be seeded yet when the mount effect runs.
    const hasStored = !!github || !!get(sessionInsights)[session.id]?.github
    if (githubReadDue({ auto, canRead, hidden: document.hidden, pending: !!pending.github, busy: pullReadBusy(session.id), hasStored, ageMs: Math.min(Date.now() - lastGitHub, pullReadAge(session.id)), cadenceMs: 120_000 })) void read('github')
  }
  // The Overview shows the pull request too, so its Refresh reads both; a manual refresh is never held back by the cadence.
  function refresh() { if (!supported) void readLegacy(); else if (selected) void read(previewView, selected); else { void read(view, tab === 'files' ? folder : ''); if (tab === 'overview') void read('github') } }
  function pick(next: PanelTab) {
    tab = next; try { localStorage.setItem(PANEL_TAB_KEY, next) } catch { /* optional preference */ }
    selected = ''; preview = null
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
  $effect(() => { if (canRead) untrack(() => { if (!workspace && !pending.status) void read('status'); if (tab === 'overview' || tab === 'github') backgroundGitHub() }) })
  // A read the navigation made for this session lands in the store; the panel takes it up rather than reading again.
  const stored = $derived($sessionInsights[session.id]?.github)
  $effect(() => { const next = stored; if (next && (!github || (next.checkedAt ?? 0) > (github.checkedAt ?? 0))) untrack(() => { github = next; errors.github = '' }) })
  // Live updates is the one switch for automatic reads: the navigation's background PR reads follow it too.
  $effect(() => { insightsAuto.set(auto) })
  onMount(() => {
    const cached = get(sessionInsights)[session.id]; workspace = cached?.status ?? null; github = cached?.github ?? null
    if (supported) void read('status'); else if (target) void readLegacy()
    if (tab === 'files') void read('files', '')
    const visibility = () => {
      if (document.hidden) return
      if (auto && canRead) { void read('status'); if (tab === 'github' || tab === 'overview') backgroundGitHub() }
    }
    const timer = setInterval(() => {
      if (!auto || document.hidden || !canRead) return
      if (!pending.status && Date.now() - lastLocal >= (session.attention === 'working' ? 15_000 : 60_000)) void read('status')
      if (tab === 'github' || tab === 'overview') backgroundGitHub()
    }, 5000)
    document.addEventListener('visibilitychange', visibility)
    return () => { alive = false; clearInterval(timer); document.removeEventListener('visibilitychange', visibility); requests.forEach(r => r.abort()) }
  })
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (the parent owns sheet focus and Escape) -->
<aside class="context-panel" aria-label="Session context" data-keeps-focus {id} {onkeydown}>
  <header class="panel-head">
    <div class="panel-title"><strong>Session workspace</strong>{#if onclose && !docked}<button type="button" class="close-details" onclick={onclose}>Close details</button>{/if}</div>
    <div class="panel-tools"><span class="connection" class:warn={status !== 'online'}><i></i>{status === 'online' ? 'Machine online' : status === 'offline' ? 'Machine offline' : status === 'paused' ? 'Machine paused' : 'Checking connection'}</span>{#if $gafferChatEntry}<button class="icon-button gaffer-entry" aria-label="Ask Gaffer" title="Ask Gaffer about this session" onclick={event => $gafferChatEntry?.(event.currentTarget)}><GafferCompanionAvatar size={20} /></button>{/if}<button class="icon-button" aria-label="Refresh session workspace" title="Refresh session workspace" disabled={!!pending[view] || (supported && !canRead)} aria-busy={!!pending[view] || legacyBusy} onclick={refresh}><InsightIcon name="refresh" /></button>{#if onclose && docked}<button class="icon-button collapse-panel" aria-label="Collapse session workspace" title="Collapse session workspace" onclick={onclose}><TerminalIcon name="panelRight" /></button>{/if}</div>
    <div class="tabs" role="tablist" aria-label="Session workspace views">{#each tabs as item, i}<button id={`${id}-tab-${item.key}`} role="tab" aria-selected={tab === item.key} aria-controls={`${id}-view`} aria-label={item.key === 'github' && githubState ? `GitHub, ${githubState}` : undefined} tabindex={tab === item.key ? 0 : -1} onclick={() => pick(item.key)} onkeydown={e => tabKeys(e, i)}><InsightIcon name={item.key} /><span>{item.label}</span>{#if item.key === 'changes' && workspace?.changed}<small>{workspace.changed}</small>{/if}{#if item.key === 'github' && githubState}<i class="tab-dot" data-tone={githubTone} title={githubState} aria-hidden="true"></i>{/if}</button>{/each}</div>
  </header>
  <div class="panel-body" id={`${id}-view`} role="tabpanel" aria-labelledby={`${id}-tab-${tab}`} tabindex="0">
    {#if tab === 'overview'}
      {#if session.structured}
        <div class="overview-gaffer">
          {#if chat}
          <GafferProgress {chat} sessionId={session.id} dir={session.dir} agent={chatToolName(session.tool)} placement="overview"
            replayed={chatProgress?.replayed ?? false}
            ended={chatProgress?.ended ?? terminalClosed(session.state)}
            sessionFailed={chatProgress?.sessionFailed ?? false}
            reattaching={chatProgress?.reattaching ?? false} />
          {/if}
          {#if !chat || (chat.connection === 'connecting' && !chatProgress?.ended)}<p class="gaffer-loading">Loading session updates…</p>{/if}
        </div>
      {/if}
      <OverviewTab {id} {session} {host} {status} {project} {skills} {now} {chat} {supported} {workspace} {github} checking={!!pending.github || !!$pullReadingIds[session.id]} base={github?.pull?.base} remote={workspace?.remote || github?.remote} {pending} {errors} {docked} {onterminal} onpick={pick} onfull={() => pick('github')} />
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
  .overview-gaffer { min-height: 84px; margin-bottom: 20px; }
  @media (pointer: coarse) { .overview-gaffer { min-height: 88px; } }
  .gaffer-loading { display: flex; align-items: center; min-height: 84px; margin: 0; padding: 12px; border: 1px solid var(--line); border-radius: 9px; color: var(--muted); font-size: 14px; }
  .context-panel{width:100%;flex:none;box-sizing:border-box;display:flex;flex-direction:column;min-height:0;min-width:0;border-left:1px solid var(--line-soft);background:var(--bg-sunk);overflow:hidden;color:var(--ink)}.panel-head{flex:none;padding:16px 16px 0;border-bottom:1px solid var(--line)}.panel-title,.panel-tools{display:flex;align-items:center;justify-content:space-between;gap:8px}.panel-title>strong{font-size:14px;font-weight:600}.panel-tools{margin:9px 0}.connection{display:flex;align-items:center;gap:6px;min-width:0;overflow:hidden;white-space:nowrap;font-size:14px;color:var(--muted)}.connection i{width:6px;height:6px;border-radius:50%;background:var(--good)}.connection.warn i{background:var(--warn)}button{font:inherit;cursor:pointer}button:disabled{opacity:.5;cursor:default}.icon-button{display:grid;place-items:center;width:28px;height:28px;background:none;color:var(--muted);border:0;border-radius:6px}.icon-button:hover{background:var(--tint);color:var(--ink)}.tabs{display:flex;margin:0 -5px}.tabs button{position:relative;display:flex;flex:1;min-width:0;justify-content:center;align-items:center;flex-wrap:nowrap;gap:4px;padding:12px 3px;border:0;border-bottom:2px solid transparent;background:none;color:var(--muted);font-size:14px}.tabs button[aria-selected=true]{border-bottom-color:var(--accent);color:var(--ink)}.tabs button:hover{background:var(--tint)}.tabs .tab-dot{position:absolute;top:7px;right:5px;width:7px;height:7px;border-radius:50%;background:var(--muted);box-shadow:0 0 0 2px var(--bg-sunk)}.tabs .tab-dot[data-tone=bad]{background:var(--bad)}.tabs .tab-dot[data-tone=warn]{background:var(--warn)}.tabs .tab-dot[data-tone=good]{background:var(--good)}.tabs small{position:absolute;top:2px;right:3px;font:14px var(--mono);padding:1px 4px;border-radius:4px;background:var(--tint)}.panel-body{flex:1;min-height:0;overflow:auto;padding:18px 16px;scrollbar-gutter:stable;outline-offset:-3px}.panel-body>section{margin-bottom:20px}h3{font:600 16px/1.5 var(--sans);margin:10px 0 8px;overflow-wrap:anywhere}.bad{color:var(--bad)}.quiet{font-size:14px;line-height:1.65;color:var(--muted);margin:8px 0}.text-action{display:inline-flex;align-items:center;gap:7px;border:0;background:none;padding:5px 0;min-height:30px;color:var(--ink);font-size:14px}footer{display:flex;align-items:center;gap:8px;padding:8px 12px;min-height:34px;border-top:1px solid var(--line);font-size:14px;color:var(--muted)}footer>span{flex:1;min-width:0}footer label{display:flex;align-items:center;gap:4px;white-space:nowrap}footer input{accent-color:var(--accent);width:12px;height:12px;margin:0}.terminal-action{display:flex;align-items:center;gap:4px;padding:5px;border:0;border-radius:5px;background:none;color:var(--muted);font-size:14px}.terminal-action:hover{background:var(--tint);color:var(--ink)}.close-details{display:none;min-height:32px;padding:5px 8px;border:1px solid var(--line);border-radius:7px;background:var(--card);color:var(--ink);font-size:14px}.empty{font-size:14px;line-height:1.8;color:var(--muted);padding:14px 0;margin:0}.callout{font-size:14px;line-height:1.7;padding:11px;border:1px solid var(--line);border-radius:8px;color:var(--warn);margin:0 0 14px}.callout.bad{color:var(--bad)}.folder-head{display:flex;align-items:center;gap:12px;min-width:0;font-size:14px}.folder-head strong{overflow:hidden;white-space:nowrap;text-overflow:ellipsis;font:500 14px var(--mono)}.preview-head{display:grid;gap:8px;min-width:0}.preview-head>strong{font:500 14px var(--mono);overflow-wrap:anywhere}.preview-modes{display:flex;gap:4px}.preview-modes button,.secondary{min-height:30px;font-size:14px;border:1px solid var(--line);border-radius:6px;padding:5px 10px;color:var(--muted);background:var(--card)}.preview-modes button[aria-pressed=true]{color:var(--accent-text);background:var(--tint)}.preview{max-height:65vh;overflow:auto;border:1px solid var(--line);border-radius:7px;background:var(--card);margin-top:12px}.preview pre{font:14px/1.7 var(--mono);padding:10px;margin:0;tab-size:2;white-space:pre}.preview pre span{display:block;min-height:1.7em;white-space:pre}.preview pre span.addition{color:var(--good);background:color-mix(in srgb,var(--good) 8%,transparent)}.preview pre span.deletion{color:var(--bad);background:color-mix(in srgb,var(--bad) 8%,transparent)}.legacy-files{list-style:none;padding:0}.legacy-files li{font:14px/1.7 var(--mono);overflow-wrap:anywhere;padding:6px 0;border-bottom:1px solid var(--line-soft)}:is(button,input):focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.icon-button,.tabs button,.text-action,.terminal-action,.preview-modes button,.secondary,.close-details{min-height:44px}.icon-button,.terminal-action,.text-action,.preview-modes button{min-width:44px}footer label{min-height:44px}}@media(max-width:400px){.context-panel{width:100%}.tabs button{font-size:14px}.tabs :global(svg){width:13px}.panel-body{padding:14px}.terminal-action span{display:none}}
</style>
