<script lang="ts">
  import { stateWord, type FleetCard } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import { pendingPermissions, pendingQuestions, toolLabel, type ChatState } from '../chat'
  import { decisionsLine } from '../sessions'
  import type { TerminalSession } from '../terminal/api'
  import { chatInsight, elapsedWord, insightReason, type SessionInsight } from '../sessionInsights'
  import InsightIcon from './InsightIcon.svelte'
  import PromptCard from './PromptCard.svelte'
  import PullCard from './PullCard.svelte'
  import StageTracker from './StageTracker.svelte'
  import WhereCard from './WhereCard.svelte'
  import type { FixKind } from './fixMessages'
  import { deriveStages } from './overviewStages'
  import { placeholderLine } from './pullFacts'
  import { runLine } from './runClock'

  // The panel owns reading and polling; the Overview only shows what it is handed, so it can grow cards without touching either.
  let { id, session, host, status, project, skills, now, chat, supported, workspace, github, base, remote, pending, errors, baseline, newEvents, terminalAdvanced, onterminal, onpick, onfull, onfix }: {
    id: string; session: TerminalSession; host: string; status: FleetCard['status'] | undefined
    project: string; skills: FleetCard['skills'] | null | undefined; now: number; chat?: ChatState; supported: boolean
    workspace: SessionInsight | null; github: SessionInsight | null; base?: string; remote?: string; pending: Record<string, boolean>; errors: Record<string, string>
    baseline: { seq: number; at: number; activityAt?: number } | null; newEvents: number; terminalAdvanced: boolean | null
    onterminal?: () => void; onpick: (tab: 'changes') => void; onfull: () => void; onfix?: (kind: FixKind) => void
  } = $props()
  const chatAttention = $derived(chat?.connection === 'open' ? ({ working: 'working', waiting: 'waiting', starting: 'starting', idle: 'quiet', error: 'failed', ended: 'finished', cancelled: 'quiet' } as const)[chat.status.state as 'working' | 'waiting' | 'starting' | 'idle' | 'error' | 'ended' | 'cancelled'] : undefined)
  const sessionState = $derived(stateWord(chatAttention ? { ...session, attention: chatAttention, attentionSource: 'chat' } : session))
  const activity = $derived(chatInsight(chat))
  const attention = $derived(chat && chat.connection === 'open' ? pendingQuestions(chat)[0]?.question.message || pendingPermissions(chat)[0]?.title || (chat.status.state === 'waiting' ? 'The agent is waiting for your answer.' : '') : session.pendingQuestion?.message || session.pendingPermission?.title || (session.attention === 'waiting' ? 'The agent is waiting for your answer.' : ''))
  const branch = $derived(workspace?.branch || session.worktreeBranch || '')
  // The overview never changes height when the agent moves on: each of these fills a slot of fixed size (one line, or a box that scrolls), so the sections after them stay where they are.
  const headline = $derived(attention ? 'Your input is needed' : activity.active[0] ? activity.active[0].title || toolLabel(activity.active[0].toolKind) : session.attention === 'working' ? 'The agent is working' : 'Session at a glance')
  const subline = $derived(`${chat?.connection === 'reconnecting' ? 'Showing the last known activity' : sessionState.source}. Last activity ${elapsedWord(session.activityAt, now)}.`)
  const validStart = $derived(Number.isFinite(session.createdAt) && session.createdAt > 0)
  const startedIso = $derived(validStart ? new Date(session.createdAt).toISOString() : undefined)
  const startedFull = $derived(validStart ? new Date(session.createdAt).toLocaleString(undefined, { dateStyle: 'full', timeStyle: 'short' }) : undefined)
  const countsLine = $derived(!chat || chat.connection === 'connecting' ? 'Loading conversation activity…' : chat.truncated ? 'Counts exclude earlier activity that is not loaded.' : 'Counts cover the loaded conversation.')
  const running = $derived(activity.active[activity.active.length - 1])
  // The stage derivation takes facts only. Whether the pull request was read at all is decided here: no read yet, no support, or an error is "not read", never "none".
  const live = $derived(session.state === 'creating' || session.state === 'running' || session.state === 'end_requested')
  const planning = $derived(/plan/i.test(session.agentMode ?? ''))
  const worked = $derived(activity.active.length > 0 || (workspace?.changed ?? 0) > 0)
  const pull = $derived(github?.pull)
  const pullReason = $derived(github?.reason || (errors.github ? 'error' : undefined))
  const reading = $derived(supported && !github && !errors.github)
  const stages = $derived.by(() => {
    const derived = deriveStages({ live, planning, needsInput: attention, active: worked, pull, pullReason })
    if (pull || attention || (supported && !reading)) return derived
    return { ...derived, next: { text: placeholderLine({ supported, loading: reading }), tone: 'muted' as const } }
  })
  const placeholder = $derived(placeholderLine({ supported, loading: reading, reason: pullReason }))
  const errorDetail = $derived(chat?.status.state === 'error' ? chat.status.detail : '')
</script>

<section class="session-summary" aria-label="Current session status">
  <div class="eyebrow">Right now <span class="state" data-status={sessionStatus(chatAttention ? { ...session, attention: chatAttention, attentionSource: 'chat' } : session)}>{chat?.connection === 'reconnecting' ? 'Reconnecting' : sessionState.word}</span></div>
  <h3 title={headline}>{headline}</h3>
  <p class="quiet one-line" title={subline}>{subline}</p>
  <!-- Not live: a clock that a screen reader announced every minute would be noise. The person reads it when they look. -->
  <p class="run-line one-line"><time datetime={startedIso} title={startedFull}>{runLine(session, now)}</time></p>
  {#if attention}<div class="notice warn"><p title={attention}>{attention}</p><button class="notice-action" aria-label="Review and answer in session" title="Review and answer in session" onclick={() => onterminal?.()}>Review <InsightIcon name="arrow" /></button></div>
  {:else if errorDetail}<div class="notice bad"><p title={errorDetail}>{errorDetail}</p></div>
  {:else if baseline && ((chat && newEvents > 0) || terminalAdvanced)}<div class="notice"><InsightIcon name="clock" /><span><strong>Since your last visit</strong>{terminalAdvanced ? 'New terminal output' : `${newEvents} new event${newEvents === 1 ? '' : 's'}`} · last here {elapsedWord(baseline.at, now)}{#if chat?.truncated}<small>Earlier activity is outside the loaded history.</small>{/if}</span></div>
  {:else}<div class="notice calm"><p>Nothing is waiting for you.</p></div>{/if}
  {#if session.structured}<div class="facts"><span><b>{activity.done.length}</b> tools done</span><span class:bad={activity.failed.length > 0}><b>{activity.failed.length}</b> failed</span><span><b>{activity.active.length}</b> active</span></div><p class="quiet one-line" title={countsLine}>{countsLine}</p>{:else}<p class="quiet">Terminal state comes from process and output activity. Agent steps are available in Gaffer chats.</p>{/if}
</section>
<div class="prompt-slot"><PromptCard {session} {chat} {now} /></div>
<div class="stage-slot"><StageTracker {id} {stages} {now} onsession={() => onterminal?.()} onfix={kind => onfix?.(kind)} /></div>
<div class="pull-slot"><PullCard {id} {pull} {placeholder} {now} {onfull} /></div>
{#if session.structured}
  <section class="live" aria-label="Agent activity">
    <div class="slot"><div class="slot-head"><h4>In progress</h4>{#if activity.active.length > 1}<span class="count">{activity.active.length} running</span>{/if}</div><div class="slot-box tool-row">{#if running}<div class="activity-row"><InsightIcon name="clock" /><span><span class="tool-title">{running.title || toolLabel(running.toolKind)}</span><small>{running.command || running.path || toolLabel(running.toolKind)}</small></span></div>{:else}<p class="slot-empty">Nothing is running right now.</p>{/if}</div></div>
    <div class="slot fails">{#if activity.failed.length}<details class="failed-tools"><summary>{activity.failed.length} tool{activity.failed.length === 1 ? '' : 's'} reported failure</summary><div class="failed-list"><p class="quiet">These are reported tool results; the agent may have recovered later.</p>{#each activity.failed.slice(-5) as tool}<p>{tool.title || toolLabel(tool.toolKind)}</p>{/each}</div></details>{:else}<p class="slot-empty">No tool failures reported.</p>{/if}</div>
    <div class="slot"><div class="slot-head"><h4 id={`${id}-latest`}>Latest agent update</h4><button class="text-action" onclick={() => onterminal?.()}>Read in conversation <InsightIcon name="arrow" /></button></div><!-- svelte-ignore a11y_no_noninteractive_tabindex (the read-only update scrolls with the keyboard) --><div class="slot-box latest-text" role="region" aria-labelledby={`${id}-latest`} tabindex="0">{#if activity.latest}{activity.latest.slice(0, 1800)}{activity.latest.length > 1800 ? '…' : ''}{:else}<span class="slot-empty">The agent has not posted an update yet.</span>{/if}</div></div>
  </section>
{/if}
<section class="change-summary"><h4>Workspace changes</h4>{#if workspace && !workspace.reason}<button onclick={() => onpick('changes')}><InsightIcon name={workspace.conflicts ? 'conflict' : 'changes'} /><span><strong>{workspace.changed}{workspace.changedMore ? "+" : ""} changed files</strong><small>{workspace.conflicts ? `${workspace.conflicts} conflict${workspace.conflicts === 1 ? ' needs' : 's need'} attention` : workspace.upstream ? `${workspace.ahead} ahead · ${workspace.behind} behind upstream` : 'No upstream comparison reported'}</small></span><InsightIcon name="arrow" /></button>{:else}<p class="quiet">{pending.status ? 'Checking the workspace…' : workspace?.reason ? insightReason(workspace.reason) : supported ? errors.status || 'Workspace status has not been read.' : 'Update reet on this machine for checkout changes, files and GitHub status.'}</p>{/if}</section>
<section aria-labelledby={`${id}-where`}><h4 id={`${id}-where`}>Where it's working</h4><WhereCard {session} {host} {status} {project} {branch} {base} {remote} {now} /></section>
<details class="session-details"><summary>Skills & session decisions</summary><h4>Skills on this machine</h4><div class="skills"><span class="skills-line">{skills === undefined ? 'Checking…' : skills === null ? 'Not checked' : skills.text}</span><a aria-label={`Manage skills on ${host}`} href={`/machines?view=compare&machine=${encodeURIComponent(host)}`}>Manage</a></div><h4>Decisions in this session</h4><p class="decisions">{decisionsLine(session)}{#if session.yolo} <a href="/activity">Activity</a>{/if}</p></details>

<!-- Scoped styles do not reach a child component, so the rules the panel's other tabs also use (button, section, h3, .bad, .quiet, .text-action, focus) are declared again here with the panel's values; the rest moved from ContextPanel. -->
<style>
  button{font:inherit;cursor:pointer}section,.prompt-slot,.stage-slot,.pull-slot{margin-bottom:20px}section+section,.prompt-slot,.stage-slot,.pull-slot,.pull-slot+section{padding-top:17px;border-top:1px solid var(--line-soft)}h3{font:600 16px/1.5 var(--sans);margin:10px 0 8px;overflow-wrap:anywhere}h4{margin:0 0 10px;font:600 11px var(--sans);letter-spacing:.07em;text-transform:uppercase;color:var(--muted)}.eyebrow{display:flex;justify-content:space-between;align-items:center;font-size:11px;color:var(--muted)}.state{border:1px solid var(--status-edge);border-radius:99px;padding:3px 8px;font-size:11px;font-weight:600;color:var(--status);background:var(--status-wash)}.bad{color:var(--bad)}.run-line{height:18px;margin:0 0 8px;font-size:11px;line-height:18px;color:var(--muted)}.quiet{font-size:11px;line-height:1.65;color:var(--muted);margin:8px 0}.session-summary h3,.one-line{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.notice{display:flex;align-items:center;gap:10px;box-sizing:border-box;height:60px;margin-top:14px;padding:0 12px;border:1px solid var(--line);border-radius:8px;overflow:hidden;background:var(--tint);color:var(--ink-soft);font-size:11px;line-height:1.5}.notice>p{flex:1;min-width:0;margin:0;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;line-clamp:2;overflow:hidden;overflow-wrap:anywhere}.notice>span{flex:1;min-width:0}.notice strong{display:block;color:var(--ink)}.notice small{display:block}.notice.warn{border-color:color-mix(in srgb,var(--warn) 40%,transparent);background:color-mix(in srgb,var(--warn) 7%,transparent);color:var(--ink);font-size:12px}.notice.bad{border-color:color-mix(in srgb,var(--bad) 40%,transparent);background:none;color:var(--bad);font-size:12px}.notice.calm{border-color:transparent;background:none;color:var(--muted)}.notice-action{flex:none;display:inline-flex;align-items:center;gap:5px;min-height:34px;padding:0 10px;background:var(--accent);color:var(--accent-ink);border:0;border-radius:7px;font-size:12px}.facts{display:flex;gap:6px;margin-top:14px}.facts>span{flex:1;border:1px solid var(--line);background:var(--card);border-radius:8px;padding:9px 6px;text-align:center;font-size:10px;color:var(--muted)}.facts>span.bad{color:var(--bad)}.facts b{display:block;font:500 20px var(--mono);color:var(--ink);margin-bottom:3px}.live{display:grid;grid-template-columns:minmax(0,1fr);gap:12px}.slot-head{display:flex;align-items:center;justify-content:space-between;gap:8px;min-height:30px}.slot-head h4{margin:0}.count{font-size:11px;color:var(--muted)}.slot-box{display:flex;align-items:center;box-sizing:border-box;min-width:0}.tool-row{height:44px}.slot-box>.activity-row{width:100%;min-width:0;padding:0}.tool-title{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.slot-empty{margin:0;font-size:12px;color:var(--muted)}.fails{min-height:32px;display:flex;align-items:center}.fails>.failed-tools{width:100%;margin:0}.failed-list{max-height:120px;overflow:auto}.latest-text{display:block;height:84px;overflow:auto;white-space:pre-wrap;font-size:12px;line-height:1.75;overflow-wrap:anywhere}.text-action{display:inline-flex;align-items:center;gap:7px;border:0;background:none;padding:5px 0;min-height:30px;color:var(--ink);font-size:11px}.activity-row{display:flex;align-items:flex-start;gap:8px;padding:7px 0;font-size:12px}.activity-row>span{min-width:0;flex:1;overflow-wrap:anywhere}.activity-row small{display:block;margin-top:4px;font:10px var(--mono);color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.change-summary>button{display:flex;align-items:center;gap:10px;width:100%;text-align:left;padding:12px;border:1px solid var(--line);border-radius:9px;background:var(--card);color:var(--ink)}.change-summary>button>span{flex:1;min-width:0}.change-summary>button,.change-summary>.quiet{box-sizing:border-box;min-height:69px}.change-summary>.quiet{display:flex;align-items:center;margin:0}.change-summary strong{font-size:13px;font-weight:500}.change-summary small{display:block;color:var(--muted);font-size:10px;margin-top:5px}.session-details{border-top:1px solid var(--line-soft);padding-top:10px;font-size:12px;line-height:1.7}.session-details summary,.failed-tools summary{min-height:32px;cursor:pointer;color:var(--muted);font-size:12px}.session-details h4{margin-top:16px}.skills{display:flex;justify-content:space-between;gap:10px}.session-details a{color:var(--ink);text-decoration:underline}.failed-tools{margin-bottom:16px;color:var(--bad);font-size:12px}.failed-tools summary{color:var(--bad)}:is(button,a,summary):focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.text-action,.session-details summary,.failed-tools summary,.notice-action{min-height:44px}.text-action,.notice-action{min-width:44px}.fails{min-height:44px}.session-details a{display:inline-flex;align-items:center;min-width:44px;min-height:44px}}
</style>
