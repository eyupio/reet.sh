<script lang="ts">
  import { checkTone, checkWord, safeInsightURL, type SessionInsight } from '../sessionInsights'
  import InsightIcon from './InsightIcon.svelte'
  let { data }: { data: SessionInsight } = $props()
  const pr = $derived(data.pull!)
  const checks = $derived(pr.checks ?? [])
  const failed = $derived(checks.filter(c => checkTone(c.state) === 'bad'))
  const running = $derived(checks.filter(c => checkTone(c.state) === 'warn'))
  const passed = $derived(checks.filter(c => checkTone(c.state) === 'good'))
  const conflict = $derived(pr.mergeable === 'CONFLICTING')
  const review = $derived(pr.review === 'CHANGES_REQUESTED' ? 'Changes requested' : pr.review === 'APPROVED' ? 'Approved' : pr.review === 'REVIEW_REQUIRED' ? 'Review required' : 'No review decision')
  const verdict = $derived(pr.state === 'MERGED' ? 'Pull request merged' : pr.state === 'CLOSED' ? 'Pull request closed' : conflict ? 'Resolve merge conflicts' : failed.length ? 'Checks need attention' : pr.draft ? 'Draft pull request' : running.length ? 'Checks in progress' : pr.review === 'CHANGES_REQUESTED' ? 'Review needs changes' : 'Review pull request on GitHub')
</script>
<article class="pull">
  <div class="pull-heading"><InsightIcon name="github" /><span>#{pr.number} · {pr.state.toLowerCase()}{pr.draft ? ' · draft' : ''}</span>{#if safeInsightURL(pr.url)}<a href={safeInsightURL(pr.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open pull request ${pr.number} on GitHub`}><InsightIcon name="arrow" /></a>{/if}</div>
  <h3>{pr.title}</h3>
  <p class="verdict" class:bad={conflict || failed.length > 0 || pr.review === 'CHANGES_REQUESTED'}><InsightIcon name={conflict ? 'conflict' : failed.length ? 'conflict' : running.length ? 'clock' : 'github'} />{verdict}</p>
  <dl><div><dt>Review</dt><dd class:bad={pr.review === 'CHANGES_REQUESTED'}>{review}</dd></div><div><dt>Merge conflicts</dt><dd class:bad={conflict}>{conflict ? 'Conflicts with base' : pr.mergeable === 'MERGEABLE' ? 'None reported' : 'Not yet known'}</dd></div></dl>
  <div class="diff-totals"><span class="good">+{pr.added.toLocaleString()}</span><span class="bad">−{pr.removed.toLocaleString()}</span><span class="bar" aria-hidden="true"><i style={`width:${pr.added + pr.removed ? pr.added / (pr.added + pr.removed) * 100 : 50}%`}></i></span></div>
  <section aria-label="CI checks">
    <h4>Checks <span>{failed.length} failing · {running.length} pending · {passed.length} passed</span></h4>
    {#if !checks.length}<p class="quiet">No checks were reported. This does not confirm that CI passed.</p>{/if}
    {#each [...failed, ...running] as check}<div class="check {checkTone(check.state)}"><InsightIcon name={checkTone(check.state) === 'bad' ? 'conflict' : 'clock'} /><span><span>{check.name}</span><small>{checkWord(check.state)}</small></span>{#if safeInsightURL(check.url)}<a href={safeInsightURL(check.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open ${check.name} check details`}><InsightIcon name="arrow" /></a>{/if}</div>{/each}
    {#if passed.length}<details><summary><InsightIcon name="check" />{passed.length} passed or skipped</summary>{#each passed as check}<div class="check good"><InsightIcon name="check" /><span><span>{check.name}</span><small>{checkWord(check.state)}</small></span>{#if safeInsightURL(check.url)}<a href={safeInsightURL(check.url)} target="_blank" rel="noopener noreferrer" aria-label={`Open ${check.name} check details`}><InsightIcon name="arrow" /></a>{/if}</div>{/each}</details>{/if}
    {#if pr.more}<p class="quiet">Showing a limited set of checks. Open GitHub for the full list.</p>{/if}
  </section>
  {#if safeInsightURL(pr.url)}<a class="open-pr" href={safeInsightURL(pr.url)} target="_blank" rel="noopener noreferrer">Open pull request <InsightIcon name="arrow" /></a>{/if}
  <p class="quiet">GitHub decides whether this PR can merge. Required checks and branch rules may still block it.</p>
</article>
<style>
  .pull{min-width:0}.pull-heading{display:flex;align-items:center;gap:7px;color:var(--muted);font-size:12px}.pull-heading span{flex:1}.pull-heading a,.check a{display:grid;place-items:center;width:28px;min-height:28px;border-radius:5px;color:var(--accent-text)}h3{margin:8px 0 14px;font:600 16px/1.5 var(--sans);overflow-wrap:anywhere}.verdict{display:flex;align-items:center;gap:8px;border:1px solid var(--line);background:var(--card);border-radius:9px;padding:12px;font-size:13px}.bad{color:var(--bad)}.good{color:var(--good)}.warn{color:var(--warn)}dl{display:grid;gap:10px;font-size:12px;margin:16px 0}dl div{display:flex;gap:10px;justify-content:space-between}dt{color:var(--muted)}dd{margin:0;text-align:right}.diff-totals{display:flex;align-items:center;gap:10px;font:12px var(--mono);padding:12px 0;border-top:1px solid var(--line-soft)}.bar{display:flex;flex:1;height:5px;border-radius:3px;overflow:hidden;background:var(--bad)}.bar i{background:var(--good)}h4{font-size:13px;margin:20px 0 9px}h4 span{display:block;margin-top:5px;color:var(--muted);font-size:11px;font-weight:400}.check{display:flex;align-items:center;gap:9px;padding:9px 0;border-bottom:1px solid var(--line-soft);font-size:12px}.check>span{flex:1;min-width:0;overflow-wrap:anywhere}.check small{display:block;margin-top:3px;color:var(--muted);font-size:10px}details{margin-top:12px}summary{display:flex;align-items:center;gap:7px;cursor:pointer;min-height:32px;font-size:12px;color:var(--good)}.quiet{font-size:11px;line-height:1.6;color:var(--muted)}.open-pr{background:var(--card);display:flex;align-items:center;justify-content:center;gap:8px;margin-top:20px;min-height:36px;border:1px solid var(--line-strong);border-radius:8px;text-decoration:none;color:var(--accent-text);font-size:12px}:is(a,summary):focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.pull-heading a,.check a,summary,.open-pr{min-height:44px;min-width:44px}}
</style>
