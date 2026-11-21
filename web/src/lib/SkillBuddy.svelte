<script lang="ts">
  import { untrack } from 'svelte'
  import { chooseSkill, recommendSkills, sourceName, receiptName, type BuddyAnswer, type BuddyContext, type BuddySkill } from './skillBuddy'
  let { context, oninsert, disabled = false, sessionHref = '', compact = false }: { context: BuddyContext; oninsert?: (text: string) => boolean | Promise<boolean>; disabled?: boolean; sessionHref?: string; compact?: boolean } = $props()
  const uid = $props.id()
  let answer = $state<BuddyAnswer | null>(null), error = $state(''), note = $state(''), loading = $state(false), busy = $state(false)
  let expanded = $state(false), paused = $state(false), preview = $state<{ skill: BuddySkill; text: string } | null>(null)
  let sequence = 0
  const shown = $derived((answer?.skills ?? []).filter(s => s.state !== 'dismissed'))
  const selected = (s: BuddySkill) => ['selected', 'prepared', 'drafted'].includes(s.state ?? '')
  const contextKey = $derived(JSON.stringify([context.sessionId, context.outcomeId, context.pipeline, context.stage]))
  $effect(() => { const key = contextKey; untrack(() => { preview = null; answer = null; error = ''; note = ''; try { paused = localStorage.getItem(`reet-skill-buddy:${key}`) === 'paused' } catch { paused = false } }) })
  $effect(() => {
    const next = { ...context }, stop = paused
    if (stop) return
    const life = new AbortController(), mine = ++sequence
    const timer = setTimeout(() => { void load(next, life, mine) }, 650)
    return () => { clearTimeout(timer); life.abort() }
  })
  async function load(next = { ...context }, life = new AbortController(), mine = ++sequence) {
    loading = true
    try { const out = await recommendSkills(next, life.signal); if (!life.signal.aborted && mine === sequence) { answer = out; error = '' } }
    catch (e) { if (!life.signal.aborted && mine === sequence) error = e instanceof Error ? e.message : 'Skills could not be read.' }
    finally { if (mine === sequence) loading = false }
  }
  function pause() { paused = !paused; try { if (paused) localStorage.setItem(`reet-skill-buddy:${contextKey}`, 'paused'); else localStorage.removeItem(`reet-skill-buddy:${contextKey}`) } catch { /* optional preference */ } }
  async function inspect(skill: BuddySkill) {
    busy = true; error = ''; note = ''
    const key = contextKey
    try { const out = await chooseSkill(context, skill.id, 'inspect'); if (key === contextKey) preview = { skill: { ...out.skill, reason: skill.reason }, text: out.text } }
    catch (e) { if (key === contextKey) error = e instanceof Error ? e.message : 'This skill could not be opened.' }
    finally { busy = false }
  }
  async function choose(skill: BuddySkill, remove = false) {
    if (busy) return
    busy = true; error = ''; note = ''
    const key = contextKey, target = { ...context }
    try {
      const out = await chooseSkill(target, skill.id, remove ? 'dismissed' : target.outcomeId ? 'selected' : 'drafted', skill.revision)
      if (key !== contextKey) return
      if (!remove && !target.outcomeId && !(await oninsert?.(out.text))) { error = 'There is not enough room in this message. Shorten it, then add the skill again.'; return }
      preview = null
      note = remove ? 'Suggestion dismissed.' : target.outcomeId ? 'Selected. These instructions will travel with this stage when it starts.' : 'Added to your message. Read it, then send when ready.'
      await load(target)
    } catch (e) { if (key === contextKey) error = e instanceof Error ? e.message : 'That choice could not be saved.' }
    finally { busy = false }
  }
</script>

<section class="buddy" aria-labelledby={`${uid}-title`} data-skill-buddy>
  <header>
    <div class="heading"><h3 id={`${uid}-title`}>{context.outcomeId && answer?.stage ? `Skills for ${answer.stage}` : 'Skills for this task'}</h3><p>{paused ? 'Suggestions paused for this work' : compact && shown[0] ? `Suggested: ${shown[0].name}` : 'Suggested from the task and stage'}</p></div>
    {#if compact}<button class="text-button" aria-expanded={expanded} onclick={() => expanded = !expanded}>{expanded ? 'Hide' : 'See skills'}</button>{/if}
    <button class="text-button" onclick={pause} aria-pressed={paused}>{paused ? 'Resume' : 'Pause'}</button>
  </header>
  {#if !paused && (!compact || expanded)}
    {#if error}<p class="problem" role="alert">{error} <button class="text-button" onclick={() => void load()}>Try again</button></p>{/if}
    {#if !answer && loading}<p class="hint" role="status">Finding a useful match…</p>
    {:else if answer && !shown.length}<p class="hint">No strong match yet. Keep working, or <a href="/community">explore community skills</a>.</p>
    {:else}
      <div class="matches" aria-busy={loading}>
        {#each (expanded ? shown : shown.slice(0, 2)) as skill (skill.id)}
          <article class:chosen={selected(skill)}>
            <div class="skill-info"><span class="origin">{sourceName(skill.source)}{#if selected(skill)} · {receiptName(skill.state)}{/if}</span><h4>{skill.name}</h4><p>{skill.reason}</p></div>
            {#if skill.source === 'community' || skill.source === 'curated'}<a class="btn quiet small" href={skill.url || '/community'}>Explore</a>
            {:else if skill.state === 'selected' && answer?.editable}<button class="text-button" disabled={busy || disabled} onclick={() => void inspect(skill)}>Read guidance</button><button class="text-button" disabled={busy || disabled} onclick={() => void choose(skill, true)}>Remove</button>
            {:else}<button class="btn quiet small" disabled={busy || disabled} onclick={() => void inspect(skill)}>{selected(skill) ? 'Read guidance' : 'Review skill'}</button>{/if}
          </article>
        {/each}
      </div>
      {#if shown.length > 2}<button class="text-button more" aria-expanded={expanded} onclick={() => expanded = !expanded}>{expanded ? 'Show fewer' : `See ${shown.length - 2} more`}</button>{/if}
    {/if}
    {#if preview}<div class="preview" role="region" aria-label={`Review ${preview.skill.name}`} tabindex="-1">
      <div class="preview-head"><strong>{preview.skill.name}</strong><button class="text-button" onclick={() => preview = null}>Close</button></div>
      <pre>{preview.text}</pre>
      {#if preview.skill.state === 'selected' && context.outcomeId}<p class="hint">These are the instructions selected for this stage</p>
      {:else if answer?.editable && !disabled}<button class="btn" disabled={busy} onclick={() => preview && void choose(preview.skill)}>{busy ? 'Saving…' : context.outcomeId ? 'Use for this stage' : 'Add to message'}</button>
      {:else}<p class="hint">{context.outcomeId ? 'This stage has already started.' : 'This session is not running.'}{#if sessionHref} <a href={sessionHref}>Open its session to add guidance.</a>{/if}</p>{/if}
    </div>{/if}
    {#if note}<p class="success" role="status">{note}</p>{/if}
    <footer><span>{context.outcomeId ? 'Suggestions adapt to each stage.' : 'Only selected instructions enter your message.'}</span><a href="/skill-insights">Skill insights →</a></footer>
  {/if}
</section>

<style>
  /* UX polish informed by Unslop, Damir Sokolovsky (@heydamir), CC BY 4.0: https://github.com/heydamir/unslop */
  .buddy{border:1px solid var(--line);border-radius:12px;background:var(--card);overflow:hidden;color:var(--ink);font:14px/1.5 var(--sans)}
  header{display:flex;align-items:center;gap:10px;padding:12px 16px;background:var(--card)}.heading{flex:1;min-width:0}h3,h4,p{margin:0}h3{font-size:14px;font-weight:500}.heading p,.hint,footer{color:var(--muted);font-size:14px}.text-button{min-height:44px;min-width:44px;border:0;background:transparent;color:var(--accent);font:inherit;padding:8px;cursor:pointer}.text-button:focus-visible,a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}.matches{padding:0 16px}article{display:flex;gap:12px;align-items:center;padding:12px 0;border-top:1px solid var(--line)}article:first-child{border:0}.skill-info{flex:1;min-width:0}.origin{font-size:14px;color:var(--muted)}h4{font-size:14px;margin:2px 0}.skill-info p{color:var(--muted);font-size:14px}.small{min-height:44px;display:inline-flex;align-items:center;justify-content:center;font-size:14px;white-space:nowrap;padding:8px 12px}.chosen h4{color:var(--accent)}.hint,.problem,.success{padding:12px 16px}.problem{color:var(--bad)}.success{color:var(--accent)}footer{padding:12px 16px;border-top:1px solid var(--line);display:flex;gap:10px;justify-content:space-between;flex-wrap:wrap}footer a{min-height:44px;display:inline-flex;align-items:center;color:var(--accent)}.more{margin:0 8px 5px}.preview{margin:0 12px 12px;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--bg)}.preview-head{display:flex;justify-content:space-between;align-items:center}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:240px;overflow:auto;font:14px/1.6 var(--sans)}button:disabled{opacity:.5;cursor:default}@media(max-width:420px){header{padding:12px}.matches{padding:0 12px}article{gap:6px}.small{padding:6px}.origin{font-size:14px}}
</style>
