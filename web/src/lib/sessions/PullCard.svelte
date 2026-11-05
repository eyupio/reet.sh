<script lang="ts">
  import { checkTone, elapsedWord, safeInsightURL, type InsightPull } from '../sessionInsights'
  import { conflictFact, reviewsLine, threadsFact } from './pullFacts'
  import { pullVerdict, splitChecks } from './pullVerdict'
  import InsightIcon from './InsightIcon.svelte'

  // `pull` is absent while GitHub is loading, not read, or has nothing for this branch; `placeholder` then says which, in the 69 px row the changes card also uses.
  let { id, pull, placeholder, now, onfull }: { id: string; pull?: InsightPull; placeholder: string; now: number; onfull: () => void } = $props()
  const checks = $derived(pull?.checks ?? [])
  const split = $derived(splitChecks(checks))
  const total = $derived(checks.length)
  const share = (n: number) => (total ? `${(n / total) * 100}%` : '0%')
  const verdict = $derived(pull ? pullVerdict(pull) : undefined)
  const reviews = $derived(pull ? reviewsLine(pull) : '')
  const threads = $derived(pull ? threadsFact(pull) : undefined)
  const conflict = $derived(pull ? conflictFact(pull) : undefined)
  const prURL = $derived(safeInsightURL(pull?.url))
  const failingURL = $derived(split.failed.map(c => safeInsightURL(c.url)).find(Boolean))
  // A review link is worth showing only when there is something to review; otherwise it would repeat the first button.
  const reviewURL = $derived(pull && pull.state === 'OPEN' && (pull.review === 'CHANGES_REQUESTED' || (threadsFact(pull).tone === 'warn')) ? prURL : undefined)
</script>

<div class="pull-card" role="group" aria-labelledby={`${id}-pull`}>
  <h4 id={`${id}-pull`}>Pull request</h4>
  {#if pull && verdict && threads && conflict}
    <div class="pull">
      <div class="row ident"><span class="grow"><b>#{pull.number}</b> · {pull.state.toLowerCase()}{#if pull.draft} · draft{/if}</span>{#if pull.lastPushAt}<span class="muted">pushed {elapsedWord(pull.lastPushAt, now)}</span>{/if}</div>
      <div class="row title" title={pull.title}>{pull.title}</div>
      <div class="row verdict" data-tone={verdict.tone} title={verdict.detail}><InsightIcon name={verdict.icon} /><strong>{verdict.title}</strong></div>
      <div class="chips">
        {#if split.failed.length}<span class="chip bad">{split.failed.length} failing</span>{/if}
        {#if split.running.length}<span class="chip warn">{split.running.length} running</span>{/if}
        {#if split.passed.length}<span class="chip good">{split.passed.length} passed</span>{/if}
        {#if !total}<span class="chip">None reported</span>{/if}
      </div>
      <div class="meter-slot">
        {#if total}
          <div class="meter" role="img" aria-label={`${split.failed.length} failing, ${split.running.length} running, ${split.passed.length} passed`}>
            <i class="bad" style={`width:${share(split.failed.length)}`}></i><i class="warn" style={`width:${share(split.running.length)}`}></i><i class="good" style={`width:${share(split.passed.length)}`}></i>
          </div>
        {:else}<div class="meter"></div>{/if}
      </div>
      <div class="row" title={reviews}>{reviews}</div>
      <div class="row" data-tone={threads.tone} title={threads.text}>{threads.text}</div>
      <div class="row split"><span class="grow" data-tone={conflict.tone} title={conflict.text}>{conflict.text}</span><span class="totals" aria-label={`${pull.added.toLocaleString()} lines added and ${pull.removed.toLocaleString()} removed`}><span class="good">+{pull.added.toLocaleString()}</span> <span class="bad">−{pull.removed.toLocaleString()}</span></span></div>
      <div class="buttons">
        {#if prURL}<a class="btn" href={prURL} target="_blank" rel="noopener noreferrer">Open pull request</a>{:else}<span></span>{/if}
        {#if failingURL}<a class="btn" href={failingURL} target="_blank" rel="noopener noreferrer">Open failing check</a>{:else}<span></span>{/if}
        {#if reviewURL}<a class="btn" href={reviewURL} target="_blank" rel="noopener noreferrer">Open review</a>{:else}<span></span>{/if}
        <button type="button" class="btn" onclick={() => onfull()}>Full detail</button>
      </div>
    </div>
  {:else}
    <div class="placeholder" role={placeholder.startsWith('Checking') ? 'status' : undefined}><p>{placeholder}</p></div>
  {/if}
</div>

<style>
  .pull-card { min-width: 0; --btn: 32px; }
  h4 { margin: 0 0 10px; font: 600 11px var(--sans); letter-spacing: 0.07em; text-transform: uppercase; color: var(--muted); }
  .placeholder { box-sizing: border-box; height: 69px; display: flex; align-items: center; padding: 0 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .placeholder p { margin: 0; font-size: 12px; line-height: 1.6; color: var(--muted); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  /* Every row has a fixed height, whether or not it has anything to say, so a pull request that changes never moves what is below it. */
  .pull { box-sizing: border-box; overflow: hidden; min-width: 0; padding: 11px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); color: var(--ink); height: calc(24px + 147px + 8px + 6px + 2 * var(--btn)); }
  .row { display: flex; align-items: center; gap: 8px; height: 18px; font-size: 11px; line-height: 18px; color: var(--ink-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .row.ident { color: var(--muted); } .ident b { color: var(--ink); font-weight: 600; }
  .grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .muted { flex: none; color: var(--muted); }
  .title { display: block; height: 20px; line-height: 20px; font-size: 13px; font-weight: 600; color: var(--ink); }
  .verdict { height: 20px; color: var(--ink); } .verdict strong { font-size: 12px; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .verdict :global(svg) { flex: none; }
  .verdict[data-tone='bad'] { color: var(--bad); } .verdict[data-tone='warn'] { color: var(--warn); } .verdict[data-tone='good'] { color: var(--good); }
  .chips { display: flex; align-items: center; gap: 5px; height: 22px; overflow: hidden; }
  .chip { flex: none; border-radius: 999px; padding: 2px 7px; background: var(--tint); color: var(--muted); font: 500 10px var(--mono); white-space: nowrap; }
  .chip.bad { color: var(--bad); background: color-mix(in srgb, var(--bad) 10%, transparent); } .chip.warn { color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, transparent); } .chip.good { color: var(--good); background: color-mix(in srgb, var(--good) 10%, transparent); }
  .meter-slot { height: 13px; display: flex; align-items: center; }
  .meter { display: flex; width: 100%; height: 5px; border-radius: 3px; overflow: hidden; background: var(--line-soft); }
  .meter i { display: block; height: 100%; } .meter .bad { background: var(--bad); } .meter .warn { background: var(--warn); } .meter .good { background: var(--good); }
  [data-tone='bad'] { color: var(--bad); } [data-tone='warn'] { color: var(--warn); } [data-tone='good'] { color: var(--good); }
  .split { gap: 10px; } .totals { flex: none; font: 11px var(--mono); } .totals .good { color: var(--good); } .totals .bad { color: var(--bad); }
  .buttons { margin-top: 8px; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: var(--btn); gap: 6px; }
  .btn { box-sizing: border-box; min-width: 0; display: inline-flex; align-items: center; justify-content: center; padding: 0 8px; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--ink); font: inherit; font-size: 11px; text-decoration: none; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .btn:hover { background: var(--tint); }
  :is(a, button):focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .pull-card { --btn: 44px; } }
</style>
