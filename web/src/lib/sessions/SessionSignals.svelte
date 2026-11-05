<script lang="ts">
  import { ensurePullInsight, sessionInsights } from './insightStore'
  import { checkTone } from '../sessionInsights'
  import InsightIcon from './InsightIcon.svelte'
  let { sessionId, now, canInspect = false, active = false }: { sessionId: string; now: number; canInspect?: boolean; active?: boolean } = $props()
  const data = $derived($sessionInsights[sessionId])
  const status = $derived(data?.status)
  const github = $derived(data?.github)
  const stale = $derived(!!status?.checkedAt && now - status.checkedAt > 90_000)
  const pr = $derived(github?.pull)
  const failed = $derived(pr?.checks?.some(c => checkTone(c.state) === 'bad'))
  const pending = $derived(pr?.checks?.some(c => checkTone(c.state) === 'warn'))
  const blocked = $derived(failed || pr?.mergeable === 'CONFLICTING' || pr?.review === 'CHANGES_REQUESTED')
  const healthy = $derived(!blocked && !pending && !pr?.draft && !!pr?.checks?.length)
  const githubStale = $derived(!github?.checkedAt || now - github.checkedAt > 180_000)
  // Read the branch's pull request after the fact: a PR created in a session shows its number in the navigation
  // without the person opening that session's GitHub panel first. Live sessions are read often, quiet ones rarely.
  $effect(() => {
    if (!canInspect) return
    const read = () => { if (!document.hidden) void ensurePullInsight(sessionId, active) }
    read()
    const timer = setInterval(read, active ? 120_000 : 600_000)
    return () => clearInterval(timer)
  })
</script>
{#if status?.changed || pr}
  <span class="signals" class:stale>
    {#if status?.changed}<span class:bad={!!status.conflicts} title={`${status.changed} changed files${status.conflicts ? `; ${status.conflicts} conflicts` : ''}${stale ? ' · last known' : ''}`} aria-label={`${status.changed} changed files${status.conflicts ? `, ${status.conflicts} conflicts` : ''}${stale ? ', last known' : ''}`}><InsightIcon name="changes" />{status.changed}</span>{/if}
    {#if pr}<span class:bad={blocked} class:warn={!blocked && pending} class:good={healthy} class:stale={githubStale} title={`PR #${pr.number} · ${pr.mergeable === 'CONFLICTING' ? 'merge conflicts' : pr.review === 'CHANGES_REQUESTED' ? 'changes requested' : failed ? 'checks failing' : pending ? 'checks running' : pr.state.toLowerCase()} ${githubStale ? ' · last known' : ''} · last checked ${new Date(github!.checkedAt || 0).toLocaleTimeString()}`} aria-label={`PR ${pr.number}, ${pr.mergeable === 'CONFLICTING' ? 'merge conflicts' : pr.review === 'CHANGES_REQUESTED' ? 'changes requested' : failed ? 'checks failing' : pending ? 'checks running' : pr.state.toLowerCase()}`}><InsightIcon name="github" />{pr.number}</span>{/if}
  </span>
{/if}
<style>
  .signals{display:flex;align-items:center;gap:9px;font:500 10px var(--mono);color:var(--muted)}.signals>span{display:inline-flex;align-items:center;gap:4px}.signals.stale,.signals>span.stale{opacity:.65}.bad{color:var(--bad)}.warn{color:var(--warn)}.good{color:var(--good)}
</style>
