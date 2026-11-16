<script lang="ts">
  import BoardIcon from './BoardIcon.svelte'
  import LaunchPreview from './LaunchPreview.svelte'
  import type { NewSessionKind } from '../terminal/NewSessionChoice.svelte'
  let { factory, onchoose, onplan }: {
    factory: boolean
    onchoose: (kind: NewSessionKind, opener: HTMLElement) => void
    onplan: () => void
  } = $props()
  const id = $props.id()
  const choices: { kind: NewSessionKind; eyebrow: string; title: string; copy: string; action: string }[] = [
    { kind: 'factory', eyebrow: 'Let Gaffer coordinate', title: 'Start Factory work', copy: 'Give Gaffer an outcome. Review the plan, then follow the work through.', action: 'Give Gaffer an outcome' },
    { kind: 'agent', eyebrow: 'Work together', title: 'Start an agent session', copy: 'Work directly with an AI agent. Guide the conversation and the next step.', action: 'Start a session' },
    { kind: 'terminal', eyebrow: 'Take the controls', title: 'Open a terminal', copy: 'Open a plain terminal on your machine and work directly.', action: 'Open terminal' },
  ]
</script>

<section class="workbench" aria-labelledby={`${id}-heading`}>
  <header class="intro">
    <p class="eyebrow">Nothing in flight</p>
    <h2 id={`${id}-heading`}>How would you like to work?</h2>
    <p>{factory ? 'Give Gaffer the outcome, work alongside an agent, or take the controls yourself.' : 'Work alongside an agent, or take the controls yourself.'}</p>
  </header>
  <div class="choices" style:--choices={factory ? 3 : 2}>
    {#each choices.filter(choice => factory || choice.kind !== 'factory') as choice (choice.kind)}
      <article class="choice" aria-labelledby={`${id}-${choice.kind}`}>
        <LaunchPreview kind={choice.kind} />
        <div class="copy">
          <p class="eyebrow">{choice.eyebrow}</p>
          <h3 id={`${id}-${choice.kind}`}>{choice.title}</h3>
          <p class="description">{choice.copy}</p>
          <button class="btn primary" type="button" onclick={event => onchoose(choice.kind, event.currentTarget)}>{choice.action}<BoardIcon name="arrow" /></button>
        </div>
      </article>
    {/each}
  </div>
  <section class="later" aria-labelledby={`${id}-later`}>
    <span class="later-icon" aria-hidden="true"><BoardIcon name="backlog" size={24} /></span>
    <div><h3 id={`${id}-later`}>A good idea for another day?</h3><p>Save a prompt on your planning board. Start it when you’re ready.</p></div>
    <button type="button" class="btn" onclick={onplan}>Plan a session<BoardIcon name="arrow" /></button>
  </section>
  {#if factory}<div class="context"><p>Gaffer is your AI foreman: it plans and coordinates Factory work.</p><a href="/factory">Explore the Factory<BoardIcon name="arrow" /></a></div>{/if}
</section>

<style>
  .workbench{container:workbench / inline-size;display:grid;gap:16px;min-width:0;color:var(--ink)}
  .intro{padding:24px 26px;border:1px solid var(--line);border-radius:14px;background:var(--card)}
  h2,h3{color:var(--ink);margin:0;overflow-wrap:anywhere}h2{font:600 28px/1.2 var(--serif);margin:10px 0}h3{font:600 16px/1.35 var(--sans)}
  p{margin:0;color:var(--muted);line-height:1.55}.eyebrow{font-size:14px;font-weight:500;letter-spacing:.06em;text-transform:uppercase;color:var(--accent-text)}
  .choices{display:grid;grid-template-columns:repeat(var(--choices),minmax(0,1fr));gap:16px;min-width:0}.choice{display:flex;flex-direction:column;min-width:0;border:1px solid var(--line);border-radius:12px;background:var(--card);overflow:hidden}
  .copy{display:flex;flex-direction:column;align-items:flex-start;gap:9px;padding:20px;min-width:0;flex:1}.description{flex:1}
  .copy .btn{align-self:flex-start;max-width:100%;min-height:44px;margin-top:7px;flex-shrink:0;white-space:normal}.copy .btn :global(svg){flex:none}
  .later{display:flex;align-items:center;gap:16px;padding:20px 22px;border:1px solid var(--line);border-radius:12px;background:var(--card)}.later-icon{color:var(--accent-text);flex:none}.later>div{min-width:0}.later p{margin-top:5px}.later .btn{flex:none;margin-left:auto;min-height:44px;max-width:100%}
  .context{display:flex;justify-content:space-between;align-items:center;gap:8px 20px;flex-wrap:wrap}.context a{display:inline-flex;align-items:center;gap:8px;min-height:44px;flex:none;color:var(--accent-text)}
  @container workbench (max-width:760px){.choices{grid-template-columns:minmax(0,1fr);gap:12px}.choice{flex-direction:row}.copy{padding:18px}.later{flex-wrap:wrap;padding:18px}.later .btn{margin-left:0}.later-icon{display:none}}
  @container workbench (max-width:380px){.intro{padding:20px}.copy{padding:14px}h2{font-size:24px}.eyebrow{letter-spacing:.02em}}
</style>
