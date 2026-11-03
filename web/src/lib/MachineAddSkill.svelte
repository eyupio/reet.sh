<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import Skeleton from './Skeleton.svelte'
  import { syncCandidates, type Machine, type SyncSkill } from './api'
  import { installSkillOnMachine } from './syncActions'

  let {
    machine, onclose, onadded,
  }: {
    machine: Machine
    onclose: () => void
    onadded: (result: Awaited<ReturnType<typeof installSkillOnMachine>>, skill: SyncSkill) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let skills = $state<SyncSkill[]>([])
  let loading = $state(true)
  let query = $state('')
  let busy = $state('')
  let error = $state('')
  let installed = $derived(new Set(machine.groups.flatMap((group) => (group.skills ?? []).map((skill) => skill.id))))
  let available = $derived(skills.filter((skill) => !!skill.source && skill.sourceType !== 'local' && !skill.problem))
  let shown = $derived.by(() => {
    const term = query.trim().toLowerCase()
    return available.filter((skill) => !term || `${skill.name} ${skill.source}`.toLowerCase().includes(term))
  })

  async function load() {
    loading = true
    error = ''
    try {
      skills = (await syncCandidates()).skills
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  async function add(skill: SyncSkill) {
    if (busy) return
    busy = skill.id
    error = ''
    try {
      const result = await installSkillOnMachine(skill, machine.host)
      onadded(result, skill)
      onclose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = ''
    }
  }

  onMount(() => {
    dialog?.showModal()
    void load()
    return () => {
      if (dialog?.open) dialog.close()
    }
  })
</script>

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby="machine-add-skill-title">
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Add a skill</p>
        <h2 id="machine-add-skill-title">Add a skill to {machine.host}</h2>
        <p class="lede" data-shell-aside>Reet saves the intent, then the machine previews the exact plan. Additive installs may apply on their own; removals wait for review.</p>
      </div>
      <button type="button" class="icon" onclick={() => dialog?.close()} aria-label="Close add a skill" data-shell-close>×</button>
    </header>

    <label class="search">Search skills
      <input type="search" bind:value={query} placeholder="Name or source" />
    </label>

    {#if error}
      <p class="notice bad" role="alert">{error} <button type="button" class="btn quiet" onclick={load} disabled={loading}>Try again</button></p>
    {/if}

    {#if loading}
      <Skeleton label="Reading the shelf…" shape="lines" count={4} />
    {:else if shown.length === 0}
      <div class="empty">
        <h3>{available.length === 0 ? 'No shelf skill can be installed here yet.' : 'No skill matches that search.'}</h3>
        <p>{available.length === 0 ? 'A skill needs a linked source before it can be added to a machine.' : 'Try another name or source.'}</p>
      </div>
    {:else}
      <ul class="skills">
        {#each shown as skill (skill.id)}
          <li>
            <div><strong>{skill.name}</strong><small>{skill.source}</small></div>
            {#if installed.has(skill.id)}
              <span class="tag">Installed</span>
            {:else}
              <button type="button" class="btn primary" disabled={!!busy} aria-busy={busy === skill.id} aria-label={`Add ${skill.name} to ${machine.host}`} onclick={() => add(skill)}>{busy === skill.id ? 'Adding…' : 'Add'}</button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    <footer>
      <button type="button" class="btn quiet" onclick={() => dialog?.close()}>Close</button>
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(620px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; border: 1px solid var(--line); border-radius: 14px; background: var(--card); color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .sheet { display: grid; gap: 16px; padding: 22px; overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.4rem, 3vw, 1.8rem)/1.15 var(--serif); letter-spacing: -0.025em; overflow-wrap: anywhere; }
  .lede { margin: 8px 0 0; color: var(--muted); font-size: 0.84rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  .search { display: grid; gap: 5px; color: var(--muted); font-size: 0.8rem; }
  .search input { width: 100%; }
  .skills { list-style: none; margin: 0; padding: 0; display: grid; border-top: 1px solid var(--line-soft); }
  .skills li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 2px; border-bottom: 1px solid var(--line-soft); }
  .skills li > div { min-width: 0; display: grid; gap: 2px; }
  .skills strong { overflow-wrap: anywhere; }
  .skills small { color: var(--muted); font-size: 0.76rem; overflow-wrap: anywhere; }
  .tag { flex: none; padding: 2px 8px; border: 1px solid color-mix(in srgb, var(--good) 35%, var(--line)); border-radius: 999px; color: var(--good); font-size: 0.72rem; }
  .empty { padding: 18px 0; max-width: 48ch; }
  .empty h3 { margin: 0; font: 500 1.2rem/1.25 var(--serif); }
  .empty p { margin: 6px 0 0; color: var(--muted); font-size: 0.84rem; }
  .notice { margin: 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; font-size: 0.84rem; }
  .notice.bad { border-color: var(--bad); }
  footer { display: flex; justify-content: flex-end; padding-top: 12px; border-top: 1px solid var(--line); }
  @media (max-width: 560px) {
    .sheet { padding: 18px; }
    .skills li { align-items: flex-start; }
  }
  @media (pointer: coarse) {
    .search input, .skills .btn, .icon { min-height: 44px; }
    .icon { min-width: 44px; }
  }
</style>
