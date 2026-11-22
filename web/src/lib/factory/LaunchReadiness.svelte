<script lang="ts">
  // One stage's readiness in the plan, as two grid children of the row that holds it: a single line saying what the
  // stage runs on, with its mark in front and its source at the end, and under it the backups, folded. A mark with a
  // word carries the level, so colour is never the only signal. What is wrong, and the machine, are said once at the
  // head of the pipelines, not under every stage.
  import type { StageReadiness } from './launchReadiness'

  let { ready, label }: { ready: StageReadiness; /** The stage and pipeline in words: "Review in pipeline 1". */ label: string } = $props()
  const MARK = { ready: ['✓', ''], look: ['!', 'Look at'], blocked: ['✕', 'Cannot start'] } as const
  const reading = $derived(ready.pending && ready.level === 'ready')
  const mark = $derived(reading ? ['…', 'Reading'] : MARK[ready.level])
  const attention = $derived(ready.backups.filter(item => !item.pending && !item.eligible).length)
  // Agents in the order tried; the model joins only where two backups share an agent.
  const names = $derived(ready.backups.map(item => {
    const shared = ready.backups.filter(other => other.agent === item.agent).length > 1
    return shared ? `${item.agent} (${item.model || 'default model'})` : item.agent
  }))
</script>

<span class="ready-line {reading ? 'reading' : ready.level}"><span class="mark" aria-hidden="true">{mark[0]}</span><span class="text">{#if mark[1]}<strong>{mark[1]}</strong>{' · '}{:else}<span class="sr-only">Ready, </span>{/if}{ready.line} · {ready.sourceFact.label}</span></span>
{#if ready.backups.length}<div class="notes">
  <details class="backups" aria-label={`Backups for ${label}, in the order tried`}>
    <summary>Backups: {names.join(', then ')}{#if attention}{' · '}{attention} to look at{/if}</summary>
    <ol>{#each ready.backups as spare, at (at)}<li class:good={!spare.pending && spare.eligible} class:bad={!spare.pending && !spare.eligible}>
      <span class="mark" aria-hidden="true">{spare.pending ? '…' : spare.eligible ? '✓' : '✕'}</span><span class="text"><strong>Backup {at + 1}: {spare.agent}{spare.model ? ` · ${spare.model}` : ''}{spare.effort ? ` · ${spare.effort} effort` : ''}</strong> · {spare.verdict}. {spare.reason}</span>
    </li>{/each}</ol>
  </details>
</div>{/if}

<style>
  /* Text columns start after a fixed-width mark, so wrapped lines and the lines under a row align under the text. */
  .ready-line,.backups li{display:grid;grid-template-columns:1.5em minmax(0,1fr);min-width:0;margin:0;font-size:0.875rem;color:var(--muted);overflow-wrap:anywhere}
  .mark{font-weight:700}.text{min-width:0}.ready-line strong{font-weight:600}
  .ready-line.ready .mark{color:var(--good)}.ready-line.look .mark{color:var(--warn)}.ready-line.blocked .mark,.ready-line.blocked strong{color:var(--bad)}
  .notes{grid-column:1/-1;display:grid;gap:2px;min-width:0}
  /* Quieter than the stage's own line above it: backups are detail, opened when wanted. */
  .backups summary{min-height:28px;display:list-item;list-style-position:outside;margin-left:1.5em;align-content:center;cursor:pointer;font-size:0.875rem;color:var(--muted);overflow-wrap:anywhere}
  .backups summary:hover{color:var(--ink-soft)}
  .backups summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:4px}
  .backups ol{list-style:none;margin:0;padding:0;display:grid;gap:2px}
  .backups li strong{font-weight:600;color:var(--ink-soft)}
  .backups li.good .mark{color:var(--good)}.backups li.bad .mark{color:var(--bad)}
  .sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap;border:0}
  @media(pointer:coarse){.backups summary{min-height:44px}}
</style>
