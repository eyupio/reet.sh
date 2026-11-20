<script lang="ts">
  // One decision a pipeline is waiting on the person for: why, what Gaffer would do, and the choices as buttons with
  // the recommended one first. The same block is shown at the head of an outcome's card and in the pipeline's session.
  import type { Need, NeedChoice } from '../gafferOutcome'

  let { title, stage = '', href = '', need, choices = need.choices, note = '', review = '', busy = false, onchoose }: {
    /** The pipeline the decision is about. */
    title: string; stage?: string; /** Where the pipeline's own card is, when this block is not on it. */ href?: string
    need: Need; /** The choices that can be taken here; the caller leaves out any that cannot. */ choices?: NeedChoice[]
    /** Anything else worth saying about who is deciding. */ note?: string
    /** Gaffer's latest review of the stage, made readable here when the note points the person at it. */
    review?: string
    busy?: boolean; onchoose: (choice: NeedChoice) => void
  } = $props()
  // The instruction Gaffer would give is only worth showing when telling the agent is what it recommends.
  const advice = $derived(need.recommend?.by === 'gaffer' && choices[0]?.recommended ? need.recommend : undefined)
</script>

<section class="decision" aria-label={`Decision for ${title}`}>
  <p class="about">{#if href}<a href={href}>{title}</a>{:else}<strong>{title}</strong>{/if}{#if stage}<span class="stage">{stage} stage</span>{/if}</p>
  <p class="why">{need.why}</p>
  {#if advice}<p class="advice"><strong>Gaffer recommends: {advice.label.replace(/…$/, '')}.</strong> {advice.why}{#if advice.text}<span class="text">It would say: “{advice.text}”</span>{/if}</p>{/if}
  {#if review}<details class="review"><summary>Gaffer’s latest review</summary><p>{review}</p></details>{/if}
  {#if note}<p class="note">{note}</p>{/if}
  <div class="choices" role="group" aria-label={`Choices for ${title}`} data-floating-clearance>
    {#each choices as choice (choice.id)}<button class="btn" class:primary={choice.recommended} type="button" disabled={busy} onclick={() => onchoose(choice)}>{choice.label}{#if choice.recommended}{' '}<span class="rec">Recommended</span>{/if}</button>{/each}
  </div>
</section>

<style>
  .decision{display:grid;gap:8px;padding:12px 14px;border-radius:10px;background:var(--card);border:1px solid var(--accent);min-width:0}
  .about{margin:0;display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 10px;font-weight:600}.about a{color:var(--ink);text-decoration:underline;text-underline-offset:3px}.about a:hover{color:var(--accent-text)}
  .stage{font-weight:500;color:var(--muted)}
  .why{margin:0;color:var(--ink);overflow-wrap:anywhere}
  .advice{margin:0;padding:8px 10px;border-radius:8px;background:var(--tint);color:var(--ink);overflow-wrap:anywhere}.advice .text{display:block;margin-top:4px;color:var(--muted)}
  .review{margin:0;color:var(--muted)}.review summary{cursor:pointer;color:var(--ink-soft);font-weight:500}.review p{margin:6px 0 0;color:var(--ink);overflow-wrap:anywhere;white-space:pre-wrap}
  .note{margin:0;color:var(--muted)}
  .choices{display:flex;flex-wrap:wrap;gap:8px}.choices .btn{min-height:44px;display:inline-flex;align-items:center;gap:8px}
  .rec{font-size:0.875rem;font-weight:600;padding:1px 8px;border-radius:999px;border:1px solid currentColor;opacity:.9}
  @media(max-width:560px){.choices .btn{flex:1 1 100%;justify-content:center}}
</style>
