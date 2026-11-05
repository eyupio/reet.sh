<script lang="ts">
  import { checkTone, elapsedWord, safeInsightURL, type InsightPull } from '../sessionInsights'
  import { conflictFact, reviewsLine, threadsFact } from './pullFacts'
  import { pullVerdict, splitChecks } from './pullVerdict'
  import InsightIcon from './InsightIcon.svelte'
  import CopyButton from '../CopyButton.svelte'

  // `pull` is absent while GitHub is loading, not read, or has nothing for this branch; `placeholder` then says which, in the 76 px row the changes card also uses.
  // `stale` is the "Last known" marker for a read the machine can no longer vouch for; empty while the read is current.
  let { id, pull, placeholder, now, onfull, stale = '' }: { id: string; pull?: InsightPull; placeholder: string; now: number; onfull: () => void; stale?: string } = $props()
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
      <!-- A stale read takes the push time's place: how old the whole card is matters more than when the branch last moved. -->
      <div class="row ident"><span class="grow"><b>#{pull.number}</b> · {pull.state.toLowerCase()}{#if pull.draft} · draft{/if}</span>{#if stale}<span class="muted stale" title={stale}>{stale}</span>{:else if pull.lastPushAt}<span class="muted pushed" title={`pushed ${elapsedWord(pull.lastPushAt, now)}`}>pushed {elapsedWord(pull.lastPushAt, now)}</span>{/if}{#if prURL}<span class="copy-link"><CopyButton text={prURL} label="Link" ariaLabel="Copy pull request link" compact /></span>{/if}</div>
      <div class="row title" title={pull.title}>{pull.title}</div>
      <div class="row verdict" data-tone={verdict.tone} title={verdict.detail}><InsightIcon name={verdict.icon} /><strong>{verdict.title}</strong></div>
      <div class="chips">
        {#if split.failed.length}<span class="chip bad" title={`${split.failed.length} failing`}>{split.failed.length} failing</span>{/if}
        {#if split.running.length}<span class="chip warn" title={`${split.running.length} running`}>{split.running.length} running</span>{/if}
        {#if split.passed.length}<span class="chip good" title={`${split.passed.length} passed`}>{split.passed.length} passed</span>{/if}
        {#if !total}<span class="chip">None reported</span>{/if}
      </div>
      <div class="meter-slot">
        {#if total}
          <div class="meter" role="img" aria-label={`${split.failed.length} failing, ${split.running.length} running, ${split.passed.length} passed`}>
            <i class="bad" style={`width:${share(split.failed.length)}`}></i><i class="warn" style={`width:${share(split.running.length)}`}></i><i class="good" style={`width:${share(split.passed.length)}`}></i>
          </div>
        {:else}<div class="meter"></div>{/if}
      </div>
      <div class="row fact" title={reviews}><span class="grow">{reviews}</span></div>
      <div class="row fact" data-tone={threads.tone} title={threads.text}><span class="grow">{threads.text}</span></div>
      <div class="row fact split"><span class="grow" data-tone={conflict.tone} title={conflict.text}>{conflict.text}</span><span class="totals" aria-label={`${pull.added.toLocaleString()} lines added and ${pull.removed.toLocaleString()} removed`}><span class="good">+{pull.added.toLocaleString()}</span> <span class="bad">−{pull.removed.toLocaleString()}</span></span></div>
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
  /* 14px type does not fit a narrow panel the way 11px did. Under about 380 px the buttons go one per row so "Open failing check"
     keeps its words, the chips may take a second line, and the three fact rows two lines each; the card is still one height. */
  .pull-card { min-width: 0; --btn: 32px; --ident: 24px; container-type: inline-size; }
  h4 { margin: 0 0 10px; font: 600 14px var(--sans); letter-spacing: 0.07em; text-transform: uppercase; color: var(--muted); }
  .placeholder { box-sizing: border-box; height: 76px; display: flex; align-items: center; padding: 0 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .placeholder p { margin: 0; font-size: 14px; line-height: 20px; color: var(--muted); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  /* Every row has a fixed height, whether or not it has anything to say, so a pull request that changes never moves what is below it. */
  .pull { --rows: 2; --chips: 28px; --fact: 20px; box-sizing: border-box; overflow: hidden; min-width: 0; padding: 11px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); color: var(--ink); height: calc(24px + 53px + var(--chips) + 3 * var(--fact) + var(--ident) + 8px + (var(--rows) - 1) * 6px + var(--rows) * var(--btn)); }
  .row { display: flex; align-items: center; gap: 8px; height: 20px; font-size: 14px; line-height: 20px; color: var(--ink-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  /* The ident row is as tall as the copy control in it (24 px, 44 px on touch), in every state, so the card keeps one height. */
  .row.ident { height: var(--ident); color: var(--muted); } .ident b { color: var(--ink); font-weight: 600; }
  .grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  /* The number and state are the card's name and never shrink; the push time and the marker give way, with their words in a title. */
  .ident .grow { flex: 1 0 auto; }
  .muted { flex: none; color: var(--muted); }
  .pushed, .stale { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .stale { font-weight: 600; }
  .copy-link { flex: none; display: inline-flex; align-items: center; }
  .copy-link :global(.copy.compact) { box-sizing: border-box; min-height: var(--ident); min-width: 24px; padding: 0 7px; gap: 4px; border: 1px solid var(--line); border-radius: 6px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; line-height: 20px; cursor: pointer; }
  .copy-link :global(.copy svg) { width: 14px; height: 14px; }
  .title { display: block; height: 20px; line-height: 20px; font-size: 14px; font-weight: 600; color: var(--ink); }
  .verdict { height: 20px; color: var(--ink); } .verdict strong { font-size: 14px; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .verdict :global(svg) { flex: none; }
  .verdict[data-tone='bad'] { color: var(--bad); } .verdict[data-tone='warn'] { color: var(--warn); } .verdict[data-tone='good'] { color: var(--good); }
  .chips { display: flex; flex-wrap: wrap; align-content: center; align-items: center; gap: 4px; height: var(--chips); overflow: hidden; }
  .chip { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; border-radius: 999px; padding: 2px 7px; background: var(--tint); color: var(--muted); font: 500 14px/20px var(--sans); white-space: nowrap; }
  .chip.bad { color: var(--bad); background: color-mix(in srgb, var(--bad) 10%, transparent); } .chip.warn { color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, transparent); } .chip.good { color: var(--good); background: color-mix(in srgb, var(--good) 10%, transparent); }
  .meter-slot { height: 13px; display: flex; align-items: center; }
  .meter { display: flex; width: 100%; height: 5px; border-radius: 3px; overflow: hidden; background: var(--line-soft); }
  .meter i { display: block; height: 100%; } .meter .bad { background: var(--bad); } .meter .warn { background: var(--warn); } .meter .good { background: var(--good); }
  [data-tone='bad'] { color: var(--bad); } [data-tone='warn'] { color: var(--warn); } [data-tone='good'] { color: var(--good); }
  .fact { height: var(--fact); } .split { gap: 10px; } .totals { flex: none; font: 14px var(--mono); } .totals .good { color: var(--good); } .totals .bad { color: var(--bad); }
  .buttons { margin-top: 8px; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: var(--btn); gap: 6px; }
  .btn { box-sizing: border-box; min-width: 0; display: inline-flex; align-items: center; justify-content: center; padding: 0 8px; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; text-decoration: none; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .btn:hover { background: var(--tint); }
  /* Empty places keep their row, so the card is one height whichever buttons a pull request has. */
  @container (max-width: 329px) {
    .pull { --rows: 4; --chips: 52px; --fact: 40px; } .buttons { grid-template-columns: minmax(0, 1fr); }
    .fact { align-items: flex-start; } .fact .grow { white-space: normal; overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
  }
  :is(a, button):focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .pull-card { --btn: 44px; --ident: 44px; } .copy-link :global(.copy.compact) { min-width: 44px; } }
</style>
