<script lang="ts">
  import type { Flags, Skill } from './api'
  import CopyButton from './CopyButton.svelte'
  import { installCommand, installPrompt, readInstallKind, writeInstallKind, type InstallKind } from './install'
  import { fmt, moves, type Stage } from './shelf'
  import { isStale, sinceLabel } from './time'

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
  let installOpen = $state(false)
  let installKind = $state<InstallKind>('command')
  let descOpen = $state(false)
  let descEl = $state<HTMLParagraphElement>()
  let descLong = $state(false)

  const command = $derived(installCommand(skill))
  const prompt = $derived(installPrompt(skill))
  const installText = $derived(installKind === 'prompt' ? prompt : command)

  $effect(() => {
    if (!descEl || descOpen) return
    descLong = descEl.scrollHeight > descEl.clientHeight + 1
  })

  function toggleInstall() {
    installOpen = !installOpen
    if (installOpen) installKind = readInstallKind()
  }
  function chooseInstall(kind: InstallKind) {
    installKind = kind
    writeInstallKind(kind)
  }

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
      {#if skill.installsWeek > 0}<span>{fmt(skill.installsWeek)} this week</span>{/if}
      <span class="age" title={new Date(skill.updatedAt).toLocaleString()}>{sinceLabel(stage, skill.updatedAt)}</span>
      {#if stage === 'try' && isStale(skill.updatedAt)}<span class="stale">still to test?</span>{/if}
    </div>
  </div>
  {#if skill.description}
    <p
      bind:this={descEl}
      class="desc"
      class:clamp={!descOpen}
      title={descLong && !descOpen ? skill.description : undefined}
    >{skill.description}</p>
    {#if descLong}
      <button type="button" class="more" aria-expanded={descOpen} onclick={() => (descOpen = !descOpen)}>
        {descOpen ? 'Less' : 'More'}
      </button>
    {/if}
  {/if}

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

  {#if command}
    <div class="install">
      <button type="button" class="install-toggle" aria-expanded={installOpen} onclick={toggleInstall}>
        <svg class="term" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="m5 7 4 4-4 4M12 15h7"></path>
        </svg>
        <span>Install</span>
        <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="m8 10 4 4 4-4"></path>
        </svg>
      </button>
      {#if installOpen}
        <div class="install-box">
          <div class="seg" role="group" aria-label="Install format">
            <button type="button" aria-pressed={installKind === 'command'} onclick={() => chooseInstall('command')}>Command</button>
            <button type="button" aria-pressed={installKind === 'prompt'} onclick={() => chooseInstall('prompt')}>Prompt</button>
          </div>
          {#if installText}
            <div class="install-line">
              <code>{installText}</code>
              <CopyButton text={installText} compact />
            </div>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

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
  .age, .stale { white-space: nowrap; }
  .stale { font-style: italic; }
  code { overflow-wrap: anywhere; }
  .desc { margin: 6px 0 0; color: var(--muted); max-width: 62ch; }
  .desc.clamp { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .more {
    display: inline-flex; align-items: center; margin-top: 4px; padding: 2px 0; min-height: 32px;
    background: none; border: 0; color: var(--muted); font-size: 0.85rem; font-weight: 600; cursor: pointer;
  }
  .more:hover { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .notes { margin: 12px 0 0; padding: 4px 0 4px 14px; border-left: 2px solid var(--accent); font: italic 1rem/1.5 var(--serif); white-space: pre-wrap; }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 14px; align-items: center; }
  .chip { display: inline-flex; gap: 6px; align-items: center; border: 1px solid var(--line); background: transparent; border-radius: 999px; padding: 4px 14px; cursor: pointer; min-height: 36px; color: var(--ink); }
  .chip:hover { border-color: var(--muted); }
  .chip.move { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .chip.move:hover { background: color-mix(in srgb, var(--tint) 55%, var(--card)); }
  .chip.heart { color: var(--muted); padding: 4px 12px; }
  .chip.heart[aria-pressed='true'] { color: var(--heart); border-color: var(--heart); font-weight: 600; }
  .spacer { flex: 1; }
  .install { margin-top: 6px; }
  .install-toggle {
    display: inline-flex; align-items: center; gap: 6px; padding: 6px 0; min-height: 36px;
    background: none; border: 0; color: var(--muted); font-weight: 600; cursor: pointer;
  }
  .install-toggle:hover { color: var(--ink); }
  .install-toggle svg { width: 15px; height: 15px; flex: none; }
  .install-toggle .chev { margin-left: 1px; }
  .install-box {
    margin-top: 4px; padding: 10px; border: 1px solid var(--line); border-radius: 10px;
    background: color-mix(in srgb, var(--bg) 55%, var(--card));
  }
  .seg {
    display: inline-flex; padding: 2px; border: 1px solid var(--line);
    border-radius: 8px; background: var(--card);
  }
  .seg button {
    background: none; border: 0; border-radius: 6px; padding: 4px 10px; min-height: 28px;
    color: var(--muted); font-size: 0.8rem; font-weight: 600; cursor: pointer;
  }
  .seg button:hover { color: var(--ink); }
  .seg button[aria-pressed='true'] { background: var(--tint); color: var(--ink); }
  .install-line { display: flex; gap: 8px; align-items: flex-start; margin-top: 8px; }
  .install-line code { flex: 1; min-width: 0; font-size: 0.8rem; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  .edit { margin-top: 12px; display: grid; gap: 8px; }
  .edit div { display: flex; gap: 8px; }
  .edit.link input { flex: 1; }
  @media (max-width: 560px) {
    .spacer { display: none; }
    .edit.link div { flex-direction: column; }
    .install-line { flex-direction: column; align-items: stretch; }
    .install-line :global(.copy) { align-self: flex-start; }
  }
  @media (pointer: coarse) {
    .chip, .install-toggle, .seg button, .more { min-height: 44px; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .chip { transition: background-color 0.15s, border-color 0.15s, color 0.15s, transform 160ms var(--ease-out); }
    .chip:active { transform: scale(0.97); }
    .install-toggle .chev { transition: transform 150ms var(--ease-out); }
    .install-toggle[aria-expanded='true'] .chev { transform: rotate(180deg); }
    .install-box {
      transition: opacity 160ms var(--ease-out), transform 160ms var(--ease-out);
      @starting-style { opacity: 0; transform: translateY(-4px); }
    }
  }
</style>
