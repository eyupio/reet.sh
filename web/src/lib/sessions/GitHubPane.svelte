<script lang="ts">
  import { checkLabel, checkTone, safeInsightURL, type SessionInsight } from '../sessionInsights'
  import InsightIcon from './InsightIcon.svelte'
  import { pullVerdict, splitChecks } from './pullVerdict'
  let { data }: { data: SessionInsight } = $props()
  const pr = $derived(data.pull!)
  const checks = $derived(pr.checks ?? [])
  const split = $derived(splitChecks(checks))
  const failed = $derived(split.failed)
  const running = $derived(split.running)
  const passed = $derived(split.passed)
  const conflict = $derived(pr.mergeable === 'CONFLICTING')
  const changesRequested = $derived(pr.review === 'CHANGES_REQUESTED')
  const approved = $derived(pr.review === 'APPROVED')
  const reviewRequired = $derived(pr.review === 'REVIEW_REQUIRED')
  const review = $derived(changesRequested ? 'Changes requested' : approved ? 'Approved' : reviewRequired ? 'Review required' : 'Awaiting review')
  const merge = $derived(conflict ? 'Conflicts with base' : pr.mergeable === 'MERGEABLE' ? 'No conflicts reported' : 'Not reported yet')
  const total = $derived(checks.length)
  const share = (n: number) => total ? `${n / total * 100}%` : '0%'

  const verdict = $derived(pullVerdict(pr))

  const actionable = $derived(failed[0] ?? running[0])
  const statusHref = $derived(safeInsightURL(actionable?.url || (changesRequested ? pr.url : '')))
  const statusAction = $derived(failed.length ? 'Open failing check' : running.length ? 'View running check' : changesRequested ? 'View review on GitHub' : '')
  const diffLabel = $derived(`${pr.added.toLocaleString()} lines added and ${pr.removed.toLocaleString()} removed`)
</script>
<article class="pull">
  <div class="pull-heading">
    <InsightIcon name="github" />
    <span class="ident"><b>#{pr.number}</b> · {pr.state.toLowerCase()}{#if pr.draft} · draft{/if}</span>
    {#if safeInsightURL(pr.url)}<a class="github-link" href={safeInsightURL(pr.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open pull request ${pr.number} on GitHub`}>GitHub <InsightIcon name="arrow" /></a>{/if}
  </div>
  <h3>{pr.title}</h3>

  <div class="status {verdict.tone}">
    <span class="status-icon"><InsightIcon name={verdict.icon} /></span>
    <div>
      <strong>{verdict.title}</strong>
      <p>{verdict.detail}</p>
      {#if statusHref && statusAction}<a class="status-link" href={statusHref} target="_blank" rel="noopener noreferrer">{statusAction} <InsightIcon name="arrow" /></a>{/if}
    </div>
  </div>

  <section class="checks" aria-label="CI checks">
    <div class="section-head">
      <h4>Checks</h4>
      <span class="counts">
        {#if failed.length}<span class="chip bad">{failed.length} failing</span>{/if}
        {#if running.length}<span class="chip warn">{running.length} running</span>{/if}
        {#if passed.length}<span class="chip good">{passed.length} passed</span>{/if}
        {#if !total}<span class="chip">None reported</span>{/if}
      </span>
    </div>
    {#if total}
      <div class="meter" role="img" aria-label={`${failed.length} failing, ${running.length} running, ${passed.length} passed`}>
        <i class="bad" style={`width:${share(failed.length)}`}></i>
        <i class="warn" style={`width:${share(running.length)}`}></i>
        <i class="good" style={`width:${share(passed.length)}`}></i>
      </div>
      {#each [...failed, ...running] as check}<div class="check {checkTone(check.state)}"><span class="check-icon"><InsightIcon name={checkTone(check.state) === 'bad' ? 'conflict' : 'clock'} /></span><span class="check-text"><span>{check.name}</span><small>{checkLabel(check.state)}</small></span>{#if safeInsightURL(check.url)}<a href={safeInsightURL(check.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open ${check.name} check details`}><InsightIcon name="arrow" /></a>{/if}</div>{/each}
      {#if passed.length}<details><summary><InsightIcon name="check" />{passed.length} passed or skipped</summary>{#each passed as check}<div class="check good"><span class="check-icon"><InsightIcon name="check" /></span><span class="check-text"><span>{check.name}</span><small>{checkLabel(check.state)}</small></span>{#if safeInsightURL(check.url)}<a href={safeInsightURL(check.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open ${check.name} check details`}><InsightIcon name="arrow" /></a>{/if}</div>{/each}</details>{/if}
      {#if pr.more}<p class="note">Showing a limited set of checks. Open GitHub for the full list.</p>{/if}
    {:else}
      <p class="note warn"><span class="note-icon"><InsightIcon name="conflict" /></span>No checks were reported. This does not confirm that CI passed.</p>
    {/if}
  </section>

  <dl class="facts">
    <div><dt>Review</dt><dd class:bad={changesRequested} class:warn={reviewRequired} class:good={approved}>{review}</dd></div>
    <div><dt>Merge</dt><dd class:bad={conflict} class:good={pr.mergeable === 'MERGEABLE'}>{merge}</dd></div>
  </dl>

  <div class="diff-totals" aria-label={diffLabel}>
    <span class="label">Changes</span>
    <span class="good">+{pr.added.toLocaleString()}</span>
    <span class="bad">−{pr.removed.toLocaleString()}</span>
    <span class="bar" aria-hidden="true"><i style={`width:${pr.added + pr.removed ? pr.added / (pr.added + pr.removed) * 100 : 50}%`}></i></span>
  </div>

  {#if safeInsightURL(pr.url)}<a class="open-pr" href={safeInsightURL(pr.url)} target="_blank" rel="noopener noreferrer">Open pull request <InsightIcon name="arrow" /></a>{/if}
  <p class="note">GitHub decides whether this PR can merge. Required checks and branch rules may still block it.</p>
</article>
<style>
  .pull{min-width:0}.pull-heading{display:flex;align-items:center;gap:7px;color:var(--muted);font-size:14px}.ident{flex:1;min-width:0;overflow-wrap:anywhere}.ident b{color:var(--ink);font-weight:600}.github-link{display:inline-flex;align-items:center;gap:4px;min-height:28px;padding:0 4px;border-radius:5px;color:var(--accent-text);font-size:14px;text-decoration:none}.github-link:hover{background:var(--tint)}h3{margin:8px 0 14px;font:600 16px/1.5 var(--sans);overflow-wrap:anywhere}
  .status{display:flex;align-items:flex-start;gap:10px;box-sizing:border-box;padding:12px;border:1px solid var(--line);border-radius:10px;background:var(--card)}.status-icon{flex:none;margin-top:1px;display:inline-flex}.status div{min-width:0;flex:1}.status strong{display:block;font:600 14px/1.4 var(--sans);color:var(--ink)}.status p{margin:4px 0 0;font-size:14px;line-height:1.55;color:var(--ink-soft)}.status.bad{border-color:color-mix(in srgb,var(--bad) 38%,transparent);background:color-mix(in srgb,var(--bad) 7%,transparent)}.status.bad .status-icon,.status.bad strong{color:var(--bad)}.status.warn{border-color:color-mix(in srgb,var(--warn) 38%,transparent);background:color-mix(in srgb,var(--warn) 7%,transparent)}.status.warn .status-icon,.status.warn strong{color:var(--warn)}.status.good{border-color:color-mix(in srgb,var(--good) 38%,transparent);background:color-mix(in srgb,var(--good) 7%,transparent)}.status.good .status-icon,.status.good strong{color:var(--good)}.status.muted{background:var(--tint)}.status-link{display:inline-flex;align-items:center;gap:4px;margin-top:8px;font:500 14px var(--sans);color:var(--accent-text);text-decoration:none}.status-link:hover{text-decoration:underline}
  .checks{margin-top:20px}.section-head{display:flex;align-items:baseline;justify-content:space-between;gap:8px;flex-wrap:wrap}.section-head h4{margin:0}.counts{display:flex;align-items:center;gap:5px;flex-wrap:wrap}.chip{border-radius:999px;padding:2px 7px;background:var(--tint);color:var(--muted);font:500 14px var(--mono);white-space:nowrap}.chip.bad{color:var(--bad);background:color-mix(in srgb,var(--bad) 10%,transparent)}.chip.warn{color:var(--warn);background:color-mix(in srgb,var(--warn) 10%,transparent)}.chip.good{color:var(--good);background:color-mix(in srgb,var(--good) 10%,transparent)}
  .meter{display:flex;height:5px;margin:9px 0 4px;border-radius:3px;overflow:hidden;background:var(--line-soft)}.meter i{display:block;height:100%}.meter .bad{background:var(--bad)}.meter .warn{background:var(--warn)}.meter .good{background:var(--good)}
  h4{font-size:14px;margin:0 0 9px}.check{display:flex;align-items:center;gap:9px;padding:9px 0;border-bottom:1px solid var(--line-soft);font-size:14px}.check-icon{flex:none;color:var(--muted);display:inline-flex}.check-text{flex:1;min-width:0;overflow-wrap:anywhere}.check-text>span{color:var(--ink)}.check small{display:block;margin-top:3px;color:var(--muted);font-size:14px}.check.bad .check-icon,.check.bad small{color:var(--bad)}.check.warn .check-icon,.check.warn small{color:var(--warn)}.check a{display:grid;place-items:center;flex:none;width:28px;min-height:28px;border-radius:5px;color:var(--accent-text)}.check a:hover{background:var(--tint)}details{margin-top:12px}summary{display:flex;align-items:center;gap:7px;cursor:pointer;min-height:32px;font-size:14px;color:var(--good)}.note{font-size:14px;line-height:1.6;color:var(--muted);margin:10px 0 0}.note.warn{display:flex;align-items:flex-start;gap:7px;color:var(--warn);padding:10px;border:1px solid color-mix(in srgb,var(--warn) 35%,transparent);border-radius:8px;background:color-mix(in srgb,var(--warn) 7%,transparent)}.note-icon{flex:none;margin-top:1px;display:inline-flex}
  .facts{display:grid;gap:0;font-size:14px;margin:16px 0;padding:4px 0;border-top:1px solid var(--line-soft);border-bottom:1px solid var(--line-soft)}.facts div{display:flex;gap:10px;justify-content:space-between;padding:7px 0}dt{color:var(--muted)}dd{margin:0;text-align:right;min-width:0;overflow-wrap:anywhere}.bad{color:var(--bad)}.good{color:var(--good)}.warn{color:var(--warn)}
  .diff-totals{display:flex;align-items:center;gap:10px;font:14px var(--mono);padding:4px 0 12px}.diff-totals .label{flex:1;color:var(--muted);font-family:var(--sans);font-size:14px}.bar{display:flex;flex:0 0 78px;height:5px;border-radius:3px;overflow:hidden;background:var(--bad)}.bar i{background:var(--good)}
  .open-pr{background:var(--accent);display:flex;align-items:center;justify-content:center;gap:8px;margin-top:18px;min-height:38px;border:1px solid var(--accent);border-radius:8px;text-decoration:none;color:var(--accent-ink);font-size:14px;font-weight:600}.open-pr:hover{filter:brightness(1.06)}
  :is(a,summary):focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(pointer:coarse){.github-link,.check a,summary,.open-pr{min-height:44px}.github-link{min-width:44px}.check a{min-width:44px}}
</style>
