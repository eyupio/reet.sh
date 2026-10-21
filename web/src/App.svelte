<script lang="ts">
  import { tick } from 'svelte'
  import {
    addSkill, listSkills, patchSkill, refreshSkill, removeSkill, resolveSkill,
    type Counts, type Flags, type Skill, type Sort,
  } from './lib/api'
  import Footer from './lib/Footer.svelte'
  import Logo from './lib/Logo.svelte'
  import SkillCard from './lib/SkillCard.svelte'
  import Tools from './lib/Tools.svelte'
  import { stageOf, stages, stageTitle, type Stage } from './lib/shelf'

  let skills = $state<Skill[]>([])
  let counts = $state<Counts>({ all: 0, liked: 0, bookmarked: 0, used: 0 })
  let show = $state<'all' | Stage>('all')
  let onlyLiked = $state(false)
  let q = $state('')
  let sort = $state<Sort>('recent')
  let addInput = $state('')
  let message = $state('')
  let error = $state('')
  let loading = $state(true)
  let search = $state<HTMLInputElement>()

  const filtering = $derived(q.trim() !== '' || onlyLiked)
  const grouped = $derived(stages.map((st) => ({ ...st, items: skills.filter((k) => stageOf(k) === st.id) })))
  // While searching or filtering, hide sections with nothing to show; otherwise keep them so the shelf reads as a pipeline.
  const visible = $derived(grouped.filter((g) => (show === 'all' || show === g.id) && (g.items.length > 0 || !filtering)))
  const total = $derived(visible.reduce((n, g) => n + g.items.length, 0))

  let clearTimer: ReturnType<typeof setTimeout> | undefined
  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    message = ''
  }
  function ok(m: string) {
    message = m
    error = ''
    clearTimeout(clearTimer)
    clearTimer = setTimeout(() => (message = ''), 6000)
  }

  async function load() {
    try {
      const r = await listSkills(onlyLiked ? 'liked' : 'all', q, sort)
      skills = r.skills
      counts = r.counts
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void [onlyLiked, q, sort]
    const t = setTimeout(load, 150)
    return () => clearTimeout(t)
  })

  // A moved skill is re-rendered in another section, so put keyboard focus back on the control that was used.
  async function refocus(id: string, act: string) {
    await tick()
    const card = document.querySelector(`[data-skill="${CSS.escape(id)}"]`)
    const el = card?.querySelector(`[data-act="${act}"]`) ?? card?.querySelector('[data-act="m0"]')
    ;(el as HTMLElement | null)?.focus()
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (!addInput.trim()) return
    try {
      const s = await addSkill(addInput.trim())
      addInput = ''
      ok(`Reet, added ${s.name} to the shelf.`)
      await load()
      setTimeout(load, 2500) // pick up fetched description
    } catch (e) {
      fail(e)
    }
  }
  async function patch(s: Skill, flags: Flags, act: string): Promise<boolean> {
    try {
      const updated = await patchSkill(s.id, flags)
      const to = stageOf(updated)
      if (to !== stageOf(s)) ok(`Moved ${s.name} to ${stageTitle(to)}.`)
      await load()
      await refocus(s.id, act)
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
  async function refresh(s: Skill) {
    try {
      await refreshSkill(s.id)
      ok(`Refreshed ${s.name}`)
      await load()
    } catch (e) {
      fail(e)
    }
  }
  async function remove(s: Skill) {
    if (!confirm(`Remove ${s.name} from your list?`)) return
    try {
      await removeSkill(s.id)
      await load()
      search?.focus()
    } catch (e) {
      fail(e)
    }
  }
  async function link(s: Skill, input: string): Promise<boolean> {
    try {
      const r = await resolveSkill(s.id, input)
      ok(`Linked to ${r.id}`)
      await load()
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
</script>

<a class="skip" href="#sections">Skip to your skills</a>
<main>
  <header>
    <h1><Logo /></h1>
    <p class="tag">Keep the reet good AI skills from skills.sh, all in one place.</p>
  </header>

  <form class="add" onsubmit={add}>
    <label class="vh" for="add-input">Skill URL or slug</label>
    <input id="add-input" placeholder="Paste a skills.sh URL or owner/repo/skill" autocomplete="off" bind:value={addInput} />
    <button class="btn primary" type="submit">Add to shelf</button>
  </form>

  <div class="notices">
    {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
    <p class="notice good" role="status" hidden={!message}>{message}</p>
  </div>

  <div class="toolbar" role="group" aria-label="Filter and sort">
    <div class="filters">
      <button type="button" aria-pressed={show === 'all'} onclick={() => (show = 'all')}>All<small>{skills.length}</small></button>
      {#each grouped as g}
        <button type="button" aria-pressed={show === g.id} onclick={() => (show = g.id)}>{g.title}<small>{g.items.length}</small></button>
      {/each}
      <span class="sep" aria-hidden="true"></span>
      <button type="button" class="liked" aria-pressed={onlyLiked} onclick={() => (onlyLiked = !onlyLiked)}>♥ Liked<small>{counts.liked}</small></button>
    </div>
    <div class="controls">
      <label class="vh" for="search">Search skills</label>
      <input id="search" type="search" placeholder="Search" bind:value={q} bind:this={search} />
      <label class="vh" for="sort">Sort by</label>
      <span class="select">
        <select id="sort" bind:value={sort}>
          <option value="recent">Recent</option>
          <option value="name">Name</option>
          <option value="installs">Installs</option>
        </select>
      </span>
    </div>
  </div>

  {#if !loading}<p class="count" role="status">{total === 1 ? '1 skill' : `${total} skills`}</p>{/if}

  <div id="sections">
    {#if loading}
      <p class="empty">Loading…</p>
    {:else if visible.length === 0}
      <p class="empty">
        {counts.all === 0 ? 'Nowt on the shelf yet. Paste a skills.sh link above, or import what you have installed.' : 'Nowt matches that.'}
      </p>
    {:else}
      {#each visible as g (g.id)}
        <section class={g.id} aria-labelledby="h-{g.id}">
          <h2 id="h-{g.id}">{g.title}<small>{g.items.length}</small></h2>
          <p class="hint">{g.hint}</p>
          <ul>
            {#each g.items as s (s.id)}
              <SkillCard skill={s} stage={g.id} onpatch={patch} onrefresh={refresh} onlink={link} onremove={remove} />
            {:else}
              <li class="none">{g.empty}</li>
            {/each}
          </ul>
        </section>
      {/each}
    {/if}
  </div>

  <Tools onchange={load} {ok} {fail} />
</main>
<Footer />

<style>
  .skip { position: absolute; left: 8px; top: -48px; background: var(--accent); color: var(--accent-ink); padding: 8px 12px; border-radius: 8px; z-index: 10; }
  .skip:focus { top: 8px; }
  main { max-width: 760px; margin: 0 auto; padding: clamp(24px, 6vw, 56px) 16px 48px; }

  h1 { margin: 0; line-height: 1; }
  .tag { margin: 12px 0 0; color: var(--muted); font: italic 1.1rem/1.4 var(--serif); max-width: 36ch; }

  .add { display: flex; gap: 8px; margin: 28px 0 8px; }
  .add input { flex: 1; }

  .notices { display: grid; gap: 8px; }
  .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice[hidden] { display: none; }
  .good { border-color: var(--good); }
  .bad { border-color: var(--bad); }

  .toolbar {
    position: sticky; top: 0; z-index: 5; margin-top: 16px; padding: 14px 0; border-bottom: 1px solid var(--line);
    background: color-mix(in srgb, var(--bg) 92%, transparent); backdrop-filter: blur(8px);
    display: flex; flex-wrap: wrap; gap: 10px 16px; justify-content: space-between; align-items: center;
  }
  .filters { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }
  .filters button { background: none; border: 0; padding: 6px 12px; border-radius: 999px; cursor: pointer; color: var(--muted); font-weight: 500; min-height: 36px; }
  .filters button:hover { color: var(--ink); }
  .filters button[aria-pressed='true'] { background: var(--tint); color: var(--ink); font-weight: 650; }
  .filters .liked[aria-pressed='true'] { background: var(--heart); color: #fff; }
  .filters small { margin-left: 4px; font-variant-numeric: tabular-nums; opacity: 0.8; }
  .sep { width: 1px; height: 20px; background: var(--line); margin: 0 6px; }
  .controls { display: flex; gap: 8px; }
  .controls input { width: 11rem; }

  .select { position: relative; display: inline-flex; }
  .select select { appearance: none; padding-right: 38px; cursor: pointer; }
  .select select:hover { border-color: var(--muted); }
  .select::after {
    content: ''; position: absolute; right: 16px; top: 50%; width: 7px; height: 7px; pointer-events: none;
    border: solid var(--muted); border-width: 0 2px 2px 0; transform: translateY(-70%) rotate(45deg);
  }
  .select:has(select:open)::after { transform: translateY(-30%) rotate(225deg); }
  .select option { background: var(--card); color: var(--ink); }
  @media (prefers-reduced-motion: no-preference) {
    .select::after { transition: transform 0.15s; }
  }
  /* Chromium can draw the list itself, so it can match the cards instead of the OS menu. */
  @supports (appearance: base-select) {
    .select select, .select select::picker(select) { appearance: base-select; }
    .select select::picker-icon { display: none; }
    .select select::picker(select) {
      margin-top: 6px; padding: 6px; border: 1px solid var(--line); border-radius: 12px; background: var(--card);
      box-shadow: 0 16px 40px -12px rgb(0 0 0 / 0.28);
    }
    .select option { padding: 8px 12px; border-radius: 8px; cursor: pointer; }
    .select option:hover, .select option:focus { background: var(--tint); outline: none; }
    .select option:checked { font-weight: 650; }
    .select option::checkmark { color: var(--accent); }
  }

  .count { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  section { margin-top: 28px; }
  h2 { margin: 0; display: flex; align-items: baseline; gap: 10px; font: 600 1.7rem/1.1 var(--serif); letter-spacing: -0.02em; }
  h2 small { font: 500 0.9rem var(--sans); color: var(--muted); font-variant-numeric: tabular-nums; }
  .hint { margin: 4px 0 0; color: var(--muted); font: italic 0.98rem var(--serif); }
  ul { list-style: none; margin: 8px 0 0; padding: 0; }
  .none { color: var(--muted); font: italic 1.05rem/1.5 var(--serif); padding: 18px 0 6px 22px; }
  .empty { text-align: center; color: var(--muted); font: italic 1.15rem/1.5 var(--serif); padding: 56px 0; }

  @media (prefers-reduced-motion: no-preference) {
    .filters button { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
  }
  @media (max-width: 560px) {
    .add { flex-direction: column; }
    .controls { width: 100%; }
    .controls input { flex: 1; width: auto; }
    .sep { display: none; }
  }
</style>
