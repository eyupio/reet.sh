<script lang="ts">
  import { checkTone, elapsedWord, safeInsightURL, type InsightPull } from '../sessionInsights'
  import { conflictFact, reviewsLine, reviewWord, threadsFact } from './pullFacts'
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
  const review = $derived(pull ? reviewWord(pull) : undefined)
  const checksName = $derived(total ? `Checks: ${split.failed.length} failing, ${split.running.length} running, ${split.passed.length} passed` : 'Checks: none reported')
  const threads = $derived(pull ? threadsFact(pull) : undefined)
  const conflict = $derived(pull ? conflictFact(pull) : undefined)
  const prURL = $derived(safeInsightURL(pull?.url))
  const failingURL = $derived(split.failed.map(c => safeInsightURL(c.url)).find(Boolean))
  // A review link is worth showing only when there is something to review; otherwise it would repeat the first button.
  const reviewURL = $derived(pull && pull.state === 'OPEN' && (pull.review === 'CHANGES_REQUESTED' || (threadsFact(pull).tone === 'warn')) ? prURL : undefined)
  // The fold is the person's to open and close: it lives here, not in the markup a poll redraws, so a refresh never shuts it.
  let open = $state(false)
</script>

<div class="pull-card" role="group" aria-labelledby={`${id}-pull`}>
  <h4 id={`${id}-pull`}>Pull request</h4>
  {#if pull && verdict && threads && conflict}
    <div class="pull">
      <!-- The summary: what a person scans for, in rows of fixed height, so the folded card is one height whatever the pull request says. -->
      <div class="row ident"><span class="grow"><b>#{pull.number}</b> · {pull.state.toLowerCase()}{#if pull.draft} · draft{/if}</span>{#if stale}<span class="muted stale" title={stale}>{stale}</span>{/if}</div>
      <div class="row title" title={pull.title}>{pull.title}</div>
      <div class="row verdict" data-tone={verdict.tone} title={verdict.detail}><InsightIcon name={verdict.icon} /><strong>{verdict.title}</strong></div>
      <div class="summary-facts">
        <!-- Each count has its own shape as well as its colour; the words are its name and its title, and stay as chips in the fold. -->
        <span class="checks" role="img" aria-label={checksName} title={checksName}>
          {#if split.failed.length}<span class="count" data-tone="bad"><InsightIcon name="conflict" />{split.failed.length}</span>{/if}
          {#if split.running.length}<span class="count" data-tone="warn"><InsightIcon name="clock" />{split.running.length}</span>{/if}
          {#if split.passed.length}<span class="count" data-tone="good"><InsightIcon name="check" />{split.passed.length}</span>{/if}
          {#if !total}<span class="none">No checks reported</span>{/if}
        </span>
        {#if review}<span class="review" data-tone={review.tone} title={reviews}>{review.text}</span>{/if}
        {#if conflict.tone === 'bad'}<span class="conflicts" data-tone="bad" title={conflict.text}>Conflicts</span>{/if}
      </div>
      <div class="actions">
        {#if prURL}<a class="btn open-pull" href={prURL} target="_blank" rel="noopener noreferrer">Open pull request</a>{/if}
        <details class="more" bind:open>
          <summary class="btn">Details<svg class="chevron" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg></summary>
          <!-- The secondary facts and actions, in fixed rows too, so a poll that changes them does not move the card while it is open. -->
          <div class="fold">
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
            <div class="row pushed">{#if pull.lastPushAt}<span class="grow" title={`pushed ${elapsedWord(pull.lastPushAt, now)}`}>pushed {elapsedWord(pull.lastPushAt, now)}</span>{/if}</div>
            <div class="buttons">
              {#if failingURL}<a class="btn" href={failingURL} target="_blank" rel="noopener noreferrer">Open failing check</a>{:else}<span></span>{/if}
              {#if reviewURL}<a class="btn" href={reviewURL} target="_blank" rel="noopener noreferrer">Open review</a>{:else}<span></span>{/if}
              {#if prURL}<span class="copy-link"><CopyButton text={prURL} label="Copy link" ariaLabel="Copy pull request link" /></span>{:else}<span></span>{/if}
              <button type="button" class="btn" onclick={() => onfull()}>Full detail</button>
            </div>
          </div>
        </details>
      </div>
    </div>
  {:else}
    <div class="placeholder" role={placeholder.startsWith('Checking') ? 'status' : undefined}><p>{placeholder}</p></div>
  {/if}
</div>

<style>
  /* Folded, the card is a summary of rows with fixed heights: the number and state (and the Last known marker), the title, the
     verdict, one row of the facts a person scans for (checks, review, conflicts) and a row holding "Open pull request" and the
     Details fold. Measured at 14px: the summary row needs 308 px inside with two-digit counts of all three kinds of check,
     "Changes requested" and "Conflicts" (a card of 330 px), so under 340 px of card width (a panel under about 388 px, the
     default 360 px included) it takes two lines and no word in it is cut. Under 300 px of card width the fold's fact rows take
     two lines too, and its buttons go one per row.
     Everything else is behind Details, which only the person opens, so it is the one thing allowed to make the card taller. */
  .pull-card { min-width: 0; --btn: 32px; --summary: 24px; container-type: inline-size; }
  h4 { margin: 0 0 10px; font: 600 14px var(--sans); letter-spacing: 0.01em; color: var(--muted); }
  .placeholder { box-sizing: border-box; height: 76px; display: flex; align-items: center; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); }
  .placeholder p { margin: 0; font-size: 14px; line-height: 20px; color: var(--muted); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  .pull { box-sizing: border-box; min-width: 0; padding: 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); }
  .row { display: flex; align-items: center; gap: 8px; height: 20px; font-size: 14px; line-height: 20px; color: var(--ink-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .row.ident { color: var(--muted); } .ident b { color: var(--ink); font-weight: 600; }
  .grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  /* The number and state are the card's name and never shrink; the marker gives way, with its words in a title. */
  .ident .grow { flex: 1 0 auto; }
  .muted { flex: none; color: var(--muted); }
  .stale { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
  .title { display: block; margin-top: 2px; font-weight: 600; color: var(--ink); }
  .verdict { margin-top: 4px; color: var(--ink); } .verdict strong { font-size: 14px; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .verdict :global(svg), .count :global(svg) { flex: none; }
  .verdict[data-tone='bad'] { color: var(--bad); } .verdict[data-tone='warn'] { color: var(--warn); } .verdict[data-tone='good'] { color: var(--good); }
  /* The checks keep their counts whole and the conflicts their word. Only a count beyond two digits could still crowd the review word
     on one line; it would then give way with its words in a title, and the reviews line is whole in the fold. */
  .summary-facts { margin-top: 6px; height: var(--summary); display: grid; grid-template-columns: auto minmax(0, auto) auto; grid-template-areas: 'checks review conflicts'; justify-content: start; align-items: center; column-gap: 10px; overflow: hidden; font-size: 14px; line-height: 20px; white-space: nowrap; }
  .checks { grid-area: checks; display: inline-flex; align-items: center; gap: 6px; }
  .count { display: inline-flex; align-items: center; gap: 2px; font-variant-numeric: tabular-nums; font-weight: 600; }
  .count :global(svg) { width: 14px; height: 14px; }
  .none { color: var(--muted); }
  .review { grid-area: review; min-width: 0; overflow: hidden; text-overflow: ellipsis; color: var(--muted); }
  .conflicts { grid-area: conflicts; font-weight: 500; }
  .actions { margin-top: 8px; }
  /* "Open pull request" floats beside the fold's summary, so the two share one row while the fold's contents open below both. The
     link keeps its own width (130 px) and the summary takes the rest, which still leaves "Details" 84 px in a 300 px panel. */
  .open-pull { float: left; margin-right: 6px; }
  .btn { box-sizing: border-box; min-width: 0; height: var(--btn); min-height: var(--btn); display: inline-flex; align-items: center; justify-content: center; padding: 0 8px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; text-decoration: none; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .btn:hover { background: var(--tint); }
  .more > summary { display: flex; gap: 6px; list-style: none; user-select: none; }
  .more > summary::-webkit-details-marker { display: none; }
  .chevron { flex: none; transition: transform 160ms var(--ease-out, ease-out); }
  .more[open] .chevron { transform: rotate(180deg); }
  @media (prefers-reduced-motion: reduce) { .chevron { transition: none; } }
  .fold { clear: both; padding-top: 10px; --chips: 28px; --fact: 20px; }
  .chips { display: flex; flex-wrap: wrap; align-content: center; align-items: center; gap: 4px; height: var(--chips); overflow: hidden; }
  .chip { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; border-radius: 6px; padding: 2px 7px; background: var(--tint); color: var(--muted); font: 500 14px/20px var(--sans); white-space: nowrap; }
  .chip.bad { color: var(--bad); background: color-mix(in srgb, var(--bad) 10%, transparent); } .chip.warn { color: var(--warn); background: color-mix(in srgb, var(--warn) 10%, transparent); } .chip.good { color: var(--good); background: color-mix(in srgb, var(--good) 10%, transparent); }
  .meter-slot { height: 13px; display: flex; align-items: center; }
  .meter { display: flex; width: 100%; height: 5px; border-radius: 3px; overflow: hidden; background: var(--line-soft); }
  .meter i { display: block; height: 100%; } .meter .bad { background: var(--bad); } .meter .warn { background: var(--warn); } .meter .good { background: var(--good); }
  [data-tone='bad'] { color: var(--bad); } [data-tone='warn'] { color: var(--warn); } [data-tone='good'] { color: var(--good); }
  .fact { height: var(--fact); } .split { gap: 10px; } .totals { flex: none; font: 14px var(--mono); font-variant-numeric: tabular-nums; } .totals .good { color: var(--good); } .totals .bad { color: var(--bad); }
  .pushed { color: var(--muted); }
  .buttons { margin-top: 8px; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: var(--btn); gap: 6px; }
  .copy-link { display: flex; min-width: 0; }
  .copy-link :global(.copy) { box-sizing: border-box; flex: 1; min-width: 0; height: var(--btn); min-height: var(--btn); justify-content: center; gap: 6px; padding: 0 8px; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--ink); font: inherit; font-size: 14px; line-height: 20px; cursor: pointer; }
  .copy-link :global(.copy:hover) { background: var(--tint); }
  .copy-link :global(.copy svg) { width: 14px; height: 14px; }
  @container (max-width: 339px) {
    /* The summary row takes two lines: the counts and the conflicts word on the first, the review word on the second. */
    .pull { --summary: 44px; }
    .summary-facts { grid-template-columns: auto minmax(0, 1fr); grid-template-areas: 'checks conflicts' 'review review'; row-gap: 4px; }
  }
  @container (max-width: 299px) {
    /* In the fold: one button per row, so "Open failing check" keeps its words, and two lines for each fact row. */
    .fold { --chips: 52px; --fact: 40px; } .buttons { grid-template-columns: minmax(0, 1fr); }
    .fact { align-items: flex-start; } .fact .grow { white-space: normal; overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
  }
  :is(a, button, summary):focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .pull-card { --btn: 44px; } }
</style>
