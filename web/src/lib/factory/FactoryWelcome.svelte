<script lang="ts">
  import BoardIcon from '../home/BoardIcon.svelte'
  import GafferCompanionAvatar from '../GafferCompanionAvatar.svelte'
  let { onstart }: { onstart: (objective: string) => void } = $props()
  let objective = $state('')
  let startButton = $state<HTMLButtonElement>()
  export function focus() { startButton?.focus({ preventScroll: true }) }
  const id = $props.id()
  const examples = [
    { label: 'Fix failing CI', text: 'Fix the failing CI workflow and explain what caused it.' },
    { label: 'Polish an empty state', text: 'Add a clear and useful empty state to the home page.' },
    { label: 'Improve the docs', text: 'Improve the getting-started documentation for new users.' },
  ]
  const steps = [
    { title: 'Describe the outcome', copy: 'Tell Gaffer what you want to achieve.' },
    { title: 'Review the plan', copy: 'Gaffer breaks it down. You approve the approach.' },
    { title: 'Follow the work', copy: 'See coding, reviews and tests as they happen.' },
    { title: 'See it through', copy: 'Track the pull request, follow-up and merge.' },
  ]
</script>

<div class="wrap"><section class="welcome" aria-labelledby={`${id}-heading`}>
  <div class="starter">
    <p class="eyebrow">Your AI foreman · No pipelines yet</p>
    <h2 id={`${id}-heading`}>Put Gaffer on the job.</h2>
    <p>Start with what you want done. Gaffer helps work out the how.</p>
    <form onsubmit={event => { event.preventDefault(); onstart(objective.trim()) }}>
      <label for={`${id}-objective`}>What would you like done?</label>
      <textarea id={`${id}-objective`} bind:value={objective} placeholder="Describe the outcome you have in mind…"></textarea>
      <div class="examples" role="group" aria-label="Example outcomes">{#each examples as example}<button type="button" onclick={() => objective = example.text}>{example.label}</button>{/each}</div>
      <button class="btn primary" type="submit" bind:this={startButton}>Give Gaffer an outcome<BoardIcon name="arrow" /></button>
    </form>
  </div>
  <aside class="sheet" aria-labelledby={`${id}-sheet`}>
    <div class="sheet-heading"><GafferCompanionAvatar size={36} /><h3 id={`${id}-sheet`}>Gaffer’s job sheet</h3></div>
    <ol>{#each steps as step, index}<li><span class="number" aria-hidden="true">{String(index + 1).padStart(2, '0')}</span><div><h4>{step.title}</h4><p>{step.copy}</p></div></li>{/each}</ol>
    <p class="explanation">Gaffer is your AI foreman: it coordinates the work and flags when it needs you.</p>
  </aside>
</section></div>

<style>
  .welcome{display:grid;grid-template-columns:minmax(0,1.1fr) minmax(0,1fr);gap:20px;min-width:0;color:var(--ink)}
  .starter,.sheet{min-width:0;padding:26px;border:1px solid var(--line);border-radius:14px;background:var(--card)}.sheet{background:var(--bg-sunk)}
  h2,h3,h4{color:var(--ink);margin:0;overflow-wrap:anywhere}h2{font:600 28px/1.2 var(--serif);margin:10px 0 12px}h3,h4{font:500 16px/1.4 var(--sans)}
  p{margin:0;color:var(--muted);line-height:1.55}.eyebrow{color:var(--accent-text);font-size:14px;letter-spacing:.05em;text-transform:uppercase}
  form{display:grid;gap:10px;margin-top:24px}label{color:var(--ink);font-weight:500}textarea{min-height:112px;width:100%;font-size:16px}.examples{display:flex;gap:6px;flex-wrap:wrap}.examples button{font:inherit;color:var(--muted);background:var(--bg);border:1px solid var(--line);padding:8px 10px;min-height:44px;border-radius:7px;cursor:pointer}.examples button:hover{color:var(--ink);border-color:var(--accent-text)}
  form>.btn{justify-self:start;max-width:100%;min-height:44px;margin-top:6px}form>.btn :global(svg){flex:none}
  .sheet-heading{display:flex;align-items:center;gap:10px;padding-bottom:16px;border-bottom:1px solid var(--line)}
  ol{display:grid;gap:24px;list-style:none;margin:24px 0;padding:0}li{display:flex;align-items:flex-start;gap:13px}li>div{min-width:0}.number{display:grid;place-items:center;flex:none;width:31px;height:31px;font:14px var(--mono);color:var(--accent-text);background:var(--card);border:1px solid var(--line);border-radius:8px}li p{margin-top:4px}.explanation{border-top:1px solid var(--line);padding-top:16px}
  /* Side by side only where each panel gets a readable width: the page's own width decides, with or without the rail. */
  .wrap{container:factory-welcome/inline-size;min-width:0}
  @container factory-welcome (max-width:760px){.welcome{grid-template-columns:minmax(0,1fr)}}
  @media(max-width:560px){.starter,.sheet{padding:20px}h2{font-size:24px}}
</style>
