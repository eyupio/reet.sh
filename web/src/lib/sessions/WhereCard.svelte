<script lang="ts">
  import type { FleetCard } from '../home'
  import type { TerminalSession } from '../terminal/api'
  import { safeInsightURL } from '../sessionInsights'
  import { baseSentence } from '../baseNotice'
  import CopyButton from '../CopyButton.svelte'
  import { runLine, startLabel } from './runClock'
  import { middleEllipsis } from './middleEllipsis'

  // Facts only: the Overview owns the section and its heading, so this stays a plain block that cannot pick up that component's section rules.
  let { session, host, status, project, branch, base, remote, now }: {
    session: TerminalSession; host: string; status: FleetCard['status'] | undefined; project: string
    branch: string; base?: string; remote?: string; now: number
  } = $props()
  // A repo name comes from a git remote, which anyone can set: only a plain owner/name becomes a link, and only to github.com.
  const repoLink = $derived(remote && /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(remote) && remote.split('/').every(part => part !== '.' && part !== '..') ? safeInsightURL(`https://github.com/${remote}`) : undefined)
  const path = $derived(session.worktreePath || session.dir || '')
  const agent = $derived([session.tool, session.model, session.effort].filter(Boolean).join(' · '))
  const line = $derived(runLine(session, now))
  // "Started 14:02 · running 1h 12m" drops its label here; a finished session's "Ran 1h 12m" gains the start it lacks.
  const started = $derived(line.startsWith('Started ') ? line.slice(8) : `${startLabel(session.createdAt, now)} · ${line.charAt(0).toLowerCase()}${line.slice(1)}`)
  // What this session's worktree was cut from, as the machine reported it at the start. Only a base that differs from
  // GitHub's, or one GitHub was never reached for, says anything (baseNotice.ts).
  const startedFrom = $derived(baseSentence({ base: session.baseName, relation: session.baseRelation, start: session.baseStart }, host))
</script>

<dl class="where">
  <div><dt>Repo</dt><dd>{#if repoLink}<a href={repoLink} target="_blank" rel="noopener noreferrer" title={remote}>{remote}</a>{:else}<span title={session.worktreeRepo || ''}>{session.worktreeRepo || 'Not recorded'}</span>{/if}</dd></div>
  <div><dt>Machine</dt><dd>{host} <span class="state" class:warn={status === 'offline' || status === 'paused'} class:good={status === 'online'}>● {status || 'no report yet'}</span></dd></div>
  <div><dt>Project</dt><dd>{project || 'None'}</dd></div>
  <div class="copyable"><dt>Branch</dt><dd><span class="mono" title={branch}>{branch || 'Not recorded'}</span>{#if base}<span class="base" title={`Pull request base: ${base}`}>Base → {base}</span>{/if}{#if branch}<CopyButton compact text={branch} ariaLabel="Copy branch name" />{/if}</dd></div>
  <div class="wrap"><dt>Worktree</dt><dd>{session.worktreePath ? 'Own worktree' : 'Shared checkout'}</dd>{#if path}<dd class="full"><span class="mono path" title={path}>{middleEllipsis(path)}</span><CopyButton compact text={path} ariaLabel="Copy worktree path" /></dd>{/if}</div>
  <div><dt>Agent</dt><dd title={agent}>{agent || 'Not recorded'}</dd></div>
  <div><dt>Started</dt><dd>{started}</dd></div>
  {#if startedFrom}<div class="wrap from"><dt>Started from</dt><dd class="full">{startedFrom}</dd></div>{/if}
</dl>

<style>
  .where{display:grid;gap:8px;margin:0;font-size:14px;font-variant-numeric:tabular-nums}
  .where>div{display:flex;justify-content:space-between;align-items:center;gap:12px;min-width:0;min-height:24px}
  dt{color:var(--muted);flex:none}
  dd{margin:0;min-width:0;display:flex;align-items:center;justify-content:flex-end;gap:8px;overflow:hidden;white-space:nowrap;text-align:right}
  dd>span:not(.state),dd>a{min-width:0;overflow:hidden;text-overflow:ellipsis}
  dd>:global(button){flex:none}
  /* The copy button arrives with the branch name; the row is already its height (34 px, the compact copy button at 14px), so the read finishing moves nothing below. */
  .where>.copyable{min-height:34px}
  .mono{font-family:var(--mono)}
  .path{color:var(--ink-soft)}
  .base{flex:0 0 auto;max-width:50%;color:var(--muted)}
  .state{color:var(--muted)}.state.good{color:var(--good)}.state.warn{color:var(--warn)}
  a{color:var(--ink);text-decoration:underline;text-underline-offset:2px}
  /* The path is the worktree's second line, so the two read as one fact. */
  .wrap{flex-wrap:wrap}
  .full{flex:1 0 100%;justify-content:space-between}
  /* The notice is a sentence, so it wraps and reads left to right under its own label. */
  .from>.full{justify-content:flex-start;white-space:normal;text-align:left;line-height:1.5;color:var(--ink-soft);overflow:visible}
  a:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:3px}
  @media(pointer:coarse){.where>div,.where>.copyable{min-height:44px}a{display:inline-flex;align-items:center;min-height:44px}}
</style>
