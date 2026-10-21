<script lang="ts">
  import type { Flags, Skill } from './api'
  import { fmt, moves, type Stage } from './shelf'

  let { skill, stage, onpatch, onrefresh, onlink, onremove }: {
    skill: Skill
    stage: Stage
    onpatch: (s: Skill, flags: Flags, act: string) => Promise<boolean>
    onrefresh: (s: Skill) => void
    onlink: (s: Skill, input: string) => Promise<boolean>
    onremove: (s: Skill) => void
  } = $props()

  let editing = $state(false)
  let draft = $state('')
  let linking = $state(false)
  let linkInput = $state('')

  function toggleNotes() {
    editing = !editing
    if (editing) draft = skill.notes
  }
  async function saveNotes() {
    if (await onpatch(skill, { notes: draft }, 'notes')) editing = false
  }
  async function link(e: SubmitEvent) {
    e.preventDefault()
    if (await onlink(skill, linkInput.trim())) {
      linking = false
      linkInput = ''
    }
  }
</script>

<li class="skill {stage}" data-skill={skill.id}>
  <div class="row">
    <h3>
      {#if skill.url.startsWith('https://www.skills.sh/')}
        <a href={skill.url} target="_blank" rel="noopener noreferrer">{skill.name}<span class="vh"> (opens skills.sh)</span></a>
      {:else}
        {skill.name}
      {/if}
    </h3>
    <div class="meta">
      <code>{skill.sourceType === 'local' ? 'unlinked' : skill.source}</code>
      {#if skill.installs > 0}<span>{fmt(skill.installs)} installs</span>{/if}
    </div>
  </div>
  {#if skill.description}<p class="desc">{skill.description}</p>{/if}

  <div class="actions">
    {#each moves(stage) as m, i}
      <button type="button" class="chip" class:move={m.primary} data-act={'m' + i} onclick={() => onpatch(skill, m.flags, 'm' + i)}>
        {m.label}
      </button>
    {/each}
    <button
      type="button" class="chip heart" data-act="like" aria-pressed={skill.liked}
      onclick={() => onpatch(skill, { liked: !skill.liked }, 'like')}
    >
      <span aria-hidden="true">{skill.liked ? '♥' : '♡'}</span>Like
    </button>
    <span class="spacer"></span>
    <button type="button" class="btn quiet" data-act="notes" aria-expanded={editing} aria-label="Notes for {skill.name}" onclick={toggleNotes}>Notes</button>
    {#if skill.sourceType === 'local'}
      <button type="button" class="btn quiet" aria-expanded={linking} aria-label="Link {skill.name} to skills.sh" onclick={() => (linking = !linking)}>Link</button>
    {:else}
      <button type="button" class="btn quiet" aria-label="Refresh {skill.name}" onclick={() => onrefresh(skill)}>Refresh</button>
    {/if}
    <button type="button" class="btn quiet danger" aria-label="Remove {skill.name}" onclick={() => onremove(skill)}>Remove</button>
  </div>

  {#if editing}
    <div class="edit">
      <label class="vh" for="notes-{skill.id}">Notes for {skill.name}</label>
      <textarea id="notes-{skill.id}" rows="3" bind:value={draft} placeholder="Why you like it, how you use it…"></textarea>
      <div>
        <button type="button" class="btn primary" onclick={saveNotes}>Save notes</button>
        <button type="button" class="btn quiet" onclick={() => (editing = false)}>Cancel</button>
      </div>
    </div>
  {:else if skill.notes}
    <p class="notes">{skill.notes}</p>
  {/if}

  {#if linking}
    <form class="edit link" onsubmit={link}>
      <label class="vh" for="link-{skill.id}">skills.sh URL for {skill.name}</label>
      <div>
        <input id="link-{skill.id}" bind:value={linkInput} placeholder="skills.sh URL for {skill.name}" />
        <button type="submit" class="btn primary">Link</button>
      </div>
    </form>
  {/if}
</li>

<style>
  .skill { position: relative; padding: 18px 0 16px 22px; border-bottom: 1px solid var(--line); }
  .skill::before { content: ''; position: absolute; left: 0; top: 22px; bottom: 20px; width: 4px; border-radius: 4px; background: var(--line); }
  .skill.use::before { background: var(--accent); }
  .skill.try::before { background: repeating-linear-gradient(to bottom, var(--accent) 0 6px, transparent 6px 10px); }
  .row { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; flex-wrap: wrap; }
  h3 { margin: 0; font: 600 1.25rem/1.25 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  h3 a { color: inherit; text-decoration-color: var(--line); text-underline-offset: 4px; }
  h3 a:hover { color: var(--accent); text-decoration-color: currentColor; }
  .meta { color: var(--muted); font-size: 0.85rem; display: flex; gap: 12px; flex-wrap: wrap; }
  code { overflow-wrap: anywhere; }
  .desc { margin: 6px 0 0; color: var(--muted); max-width: 62ch; }
  .notes { margin: 12px 0 0; padding: 4px 0 4px 14px; border-left: 2px solid var(--accent); font: italic 1rem/1.5 var(--serif); white-space: pre-wrap; }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 14px; align-items: center; }
  .chip { display: inline-flex; gap: 6px; align-items: center; border: 1px solid var(--line); background: transparent; border-radius: 999px; padding: 4px 14px; cursor: pointer; min-height: 36px; color: var(--ink); }
  .chip:hover { border-color: var(--muted); }
  .chip.move { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .chip.move:hover { background: color-mix(in srgb, var(--tint) 55%, var(--card)); }
  .chip.heart { color: var(--muted); padding: 4px 12px; }
  .chip.heart[aria-pressed='true'] { color: var(--heart); border-color: var(--heart); font-weight: 600; }
  .spacer { flex: 1; }
  .edit { margin-top: 12px; display: grid; gap: 8px; }
  .edit div { display: flex; gap: 8px; }
  .edit.link input { flex: 1; }
  @media (max-width: 560px) { .spacer { display: none; } .edit.link div { flex-direction: column; } }
  @media (prefers-reduced-motion: no-preference) { .chip { transition: background-color 0.15s, border-color 0.15s, color 0.15s; } }
</style>
