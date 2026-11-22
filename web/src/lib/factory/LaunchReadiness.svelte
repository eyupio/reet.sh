<script lang="ts">
  // One stage's readiness in the plan, as two grid children of the row that holds it: a single line saying what the
  // stage runs on, with its mark in front and its source at the end, and under it only the exceptions (things to look
  // at, notes) and the backups, folded. A ready stage is the one line. A mark with a word carries the level, so colour
  // is never the only signal. The machine is said once, in the summary above Approve.
  import type { StageReadiness } from './launchReadiness'

  let { ready, label }: { ready: StageReadiness; /** The stage and pipeline in words: "Review in pipeline 1". */ label: string } = $props()
  const MARK = { ready: ['✓', ''], look: ['!', 'Look at'], blocked: ['✕', 'Cannot start'] } as const
  const reading = $derived(ready.pending && ready.level === 'ready')
  const mark = $derived(reading ? ['…', 'Reading'] : MARK[ready.level])
  const things = $derived([ready.connected, ready.agent, ready.signIn, ready.model, ready.effort, ready.sourceFact].filter(item => !item.pending && item.state !== 'ready' && item.state !== 'default'))
  const later = $derived([...new Set([ready.model.later, ready.effort.later].filter(Boolean))])
  // A backup that is not usable by balancing, or has old figures, is something to look at.
  const attention = $derived(ready.backups.filter(item => !item.pending && (!item.eligible || item.stale)).length)
  // Agents in the order tried; the model joins only where two backups share an agent.
  const names = $derived(ready.backups.map(item => {
    const shared = ready.backups.filter(other => other.agent === item.agent).length > 1
    return shared ? `${item.agent} (${item.model || 'default model'})` : item.agent
  }))
  // The person's own open or close is kept; until then it is open only for what needs a look.
  let chosen = $state<boolean | null>(null)
  const open = $derived(chosen ?? attention > 0)
</script>

<span class="ready-line {reading ? 'reading' : ready.level}"><span class="mark" aria-hidden="true">{mark[0]}</span><span class="text">{#if mark[1]}<strong>{mark[1]}</strong>{' · '}{:else}<span class="sr-only">Ready, </span>{/if}{ready.line} · {ready.sourceFact.label}</span></span>
{#if things.length || later.length || ready.backups.length}<div class="notes">
  {#if things.length || later.length}<div class="things" role="group" aria-label={`Launch readiness for ${label}`}>
    {#each things as item (item.label)}<p class:bad={item === ready.connected || item === ready.agent} class:warn={item !== ready.connected && item !== ready.agent && !item.quiet}><span class="mark" aria-hidden="true">{item === ready.connected || item === ready.agent ? '✕' : item.quiet ? '·' : '!'}</span><span class="text">{item.text}</span></p>{/each}
    {#each later as sentence (sentence)}<p class="note"><span class="mark" aria-hidden="true"></span><span class="text">{sentence}</span></p>{/each}
  </div>{/if}
  {#if ready.backups.length}<details class="backups" aria-label={`Backups for ${label}, in the order tried`} open={open}>
    <summary onclick={event => { chosen = !(event.currentTarget.parentElement as HTMLDetailsElement).open }}>Backups: {names.join(', then ')}{#if attention}{' · '}{attention} to look at{/if}</summary>
    <ol>{#each ready.backups as spare, at (at)}<li class:good={!spare.pending && spare.eligible && !spare.stale} class:warn={!spare.pending && (!spare.eligible || spare.stale)}>
      <span class="mark" aria-hidden="true">{spare.pending ? '…' : spare.eligible ? (spare.stale ? '!' : '✓') : '✕'}</span><span class="text"><strong>Backup {at + 1}: {spare.agent}{spare.model ? ` · ${spare.model}` : ''}{spare.effort ? ` · ${spare.effort} effort` : ''}</strong> · {spare.verdict}. {spare.reason}</span>
    </li>{/each}</ol>
  </details>{/if}
</div>{/if}

<style>
  /* Text columns start after a fixed-width mark, so wrapped lines and the lines under a row align under the text. */
  .ready-line,.things p,.backups li{display:grid;grid-template-columns:1.5em minmax(0,1fr);min-width:0;margin:0;font-size:0.875rem;color:var(--muted);overflow-wrap:anywhere}
  .mark{font-weight:700}.text{min-width:0}.ready-line strong{font-weight:600}
  .ready-line.ready .mark{color:var(--good)}.ready-line.look .mark{color:var(--warn)}.ready-line.blocked .mark,.ready-line.blocked strong{color:var(--bad)}
  .things p.warn .mark{color:var(--warn)}.things p.bad .mark{color:var(--bad)}
  .notes{grid-column:1/-1;display:grid;gap:2px;min-width:0}
  .things{display:grid;gap:2px}.things p.note{padding:0}
  /* Quieter than the stage's own line above it: backups are detail, opened when wanted or when one needs a look. */
  .backups summary{min-height:28px;display:list-item;list-style-position:outside;margin-left:1.5em;align-content:center;cursor:pointer;font-size:0.875rem;color:var(--muted);overflow-wrap:anywhere}
  .backups summary:hover{color:var(--ink-soft)}
  .backups summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:4px}
  .backups ol{list-style:none;margin:0;padding:0;display:grid;gap:2px}
  .backups li strong{font-weight:600;color:var(--ink-soft)}
  .backups li.good .mark{color:var(--good)}.backups li.warn .mark{color:var(--warn)}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap;border:0}
  @media(pointer:coarse){.backups summary{min-height:44px}}
</style>
