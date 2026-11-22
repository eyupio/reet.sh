<script lang="ts">
  // One stage's readiness under its row in the plan: where it runs and where the choice came from, what to look at, and
  // every backup with whether it can take over. A stage that is ready and all default is one line. A mark with a word
  // carries the level, so colour is never the only signal.
  import { CHECKED_AT_START, type StageReadiness } from './launchReadiness'

  let { ready }: { ready: StageReadiness } = $props()
  const MARK = { ready: ['✓', 'Ready'], look: ['!', 'Look at'], blocked: ['✕', 'Cannot start'] } as const
  const things = $derived([ready.connected, ready.agent, ready.signIn, ready.model, ready.effort, ready.sourceFact].filter(item => !item.pending && item.state !== 'ready' && item.state !== 'default'))
  // A backup's mark is the one the stage's own level is judged by: one that cannot take over, one with old figures, or neither.
  const chosen = $derived([ready.model, ready.effort].filter(item => item.state === 'default').map(item => item.label))
</script>

<div class="readiness {ready.level}" role="group" aria-label="Launch readiness">
  <p class="line"><span class="mark" aria-hidden="true">{MARK[ready.level][0]}</span><strong>{MARK[ready.level][1]}</strong> · {ready.machine || 'No machine'} · {ready.sourceFact.label}{#if chosen.length}<span class="chosen">{' · '}Your choice: {chosen.join(', ')}</span>{/if}</p>
  {#if things.length}<div class="things">{#each things as item (item.label)}<p><span class="mark" aria-hidden="true">{item === ready.connected || item === ready.agent ? '✕' : '!'}</span>{item.text}</p>{/each}</div>{/if}
  {#if ready.checkedAtStart}<p class="note">{CHECKED_AT_START}</p>{/if}
  {#if ready.backups.length}<div class="backups" role="group" aria-label="Backups, in the order tried">{#each ready.backups as spare, at (at)}<p>
    <span class="mark" aria-hidden="true">{spare.eligible ? (spare.stale ? '!' : '✓') : '✕'}</span><strong>Backup {at + 1}: {spare.agent}{spare.model ? ` · ${spare.model}` : ''}{spare.effort ? ` · ${spare.effort} effort` : ''}</strong> · {spare.eligible ? 'can take over' : 'cannot take over'}{#if spare.reason}. {spare.reason}{:else}. Plan figures are current.{/if}
  </p>{/each}</div>{/if}
</div>

<style>
  .readiness{grid-column:1/-1;display:grid;gap:2px;min-width:0;font-size:0.875rem;color:var(--muted)}
  p{margin:0;overflow-wrap:anywhere}.things,.backups{display:grid;gap:2px}
  .mark{display:inline-block;width:1.25em;font-weight:700}
  .look .line .mark,.things .mark{color:var(--warn)}.blocked .line,.blocked .line .mark{color:var(--bad)}.ready .line .mark{color:var(--good)}
  .backups strong{font-weight:600;color:var(--ink-soft)}.note{padding-left:1.25em}
</style>
