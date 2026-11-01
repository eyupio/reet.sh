<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { shell } from '../dialogShell'
  import SkillInstallPanel from './SkillInstallPanel.svelte'
  import { groupSkills, seenLine, type SessionSkill, type SessionSkills } from './skills'

  // The per-session skill picker: a modal sheet listing the skills a session could use, grouped by where they live.
  // Names and descriptions are untrusted text from a machine and are only ever rendered as text.
  // Arrow keys, Home and End move through the rows, Enter picks one, Escape closes; the caller gives focus back.
  let { data, loading, error, tool, initialQuery = '', sessionId, onpick, onclose, onretry, onrefresh }: {
    data: SessionSkills | null
    loading: boolean
    error: string
    tool: string
    initialQuery?: string
    sessionId: string
    onpick: (skill: SessionSkill) => void
    onclose: () => void
    onretry: () => void
    onrefresh: () => void
  } = $props()

  const id = $props.id()
  let query = $state(untrack(() => initialQuery))
  let installing = $state<SessionSkill | null>(null)
  let dialog = $state<HTMLDialogElement>(), list = $state<HTMLDivElement>(), input = $state<HTMLInputElement>()
  const now = Date.now()

  const groups = $derived(data ? groupSkills(data.skills, query, tool) : [])
  const unknownMachine = $derived(!!data && data.host === '')
  const empty = $derived(!!data && data.skills.length === 0)
  const noMatch = $derived(!!data && !empty && groups.length === 0)
  const hasAction = (s: SessionSkill) => s.source === 'reet' && !s.onMachine
  // Consecutive rows without an action stay together; a row with an action stands alone.
  function runsOf(skills: SessionSkill[]): SessionSkill[][] {
    const out: SessionSkill[][] = []
    for (const s of skills) {
      const last = out[out.length - 1]
      if (last && !hasAction(s) && !hasAction(last[0])) last.push(s)
      else out.push([s])
    }
    return out
  }
  const reason = (s: SessionSkill) => (s.invocation ? '' : s.source === 'reet' ? 'not on this machine' : `not native to ${tool}`)

  onMount(() => {
    dialog?.showModal()
    input?.focus()
    return () => { if (dialog?.open) dialog.close() }
  })
  function close() { if (dialog?.open) dialog.close(); onclose() }
  // The install panel takes the sheet over; when it closes the list comes back, reloaded if something was installed.
  async function installed(done: boolean) {
    installing = null
    if (done) onrefresh()
    await tick()
    input?.focus()
  }
  function pick(skill: SessionSkill) {
    if (!skill.invocation) return
    onpick(skill)
  }
  const rows = () => [...(list?.querySelectorAll<HTMLElement>('[role=option]') ?? [])]
  function move(event: KeyboardEvent, to: (at: number, last: number) => number) {
    const all = rows()
    if (!all.length) return
    event.preventDefault()
    const at = all.indexOf(document.activeElement as HTMLElement)
    all[Math.max(0, Math.min(all.length - 1, to(at, all.length - 1)))].focus()
  }
  function keys(event: KeyboardEvent) {
    const onRow = (event.target as HTMLElement)?.getAttribute('role') === 'option'
    if (event.key === 'ArrowDown') { if (onRow || event.target === input) move(event, at => at + 1) }
    else if (event.key === 'ArrowUp') {
      if (onRow && rows()[0] === event.target) { event.preventDefault(); input?.focus() }
      else if (onRow) move(event, at => at - 1)
    }
    else if (event.key === 'Home' && onRow) move(event, () => 0)
    else if (event.key === 'End' && onRow) move(event, (_, last) => last)
    else if (event.key === 'Enter' && onRow) {
      event.preventDefault()
      ;(event.target as HTMLElement).click()
    }
    else if (event.key === 'Enter' && event.target === input) {
      event.preventDefault()
      const first = rows().find(el => el.getAttribute('aria-disabled') !== 'true')
      first?.click()
    }
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={`${id}-title`}
  oncancel={(event) => { event.preventDefault(); close() }}
  onclick={(event) => { if (event.target === dialog) close() }}>
  <div class="card" data-shell-card>
    <header data-shell-head>
      <h2 id={`${id}-title`}>Skills</h2>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    </header>
    <div class="body" data-shell-body onkeydown={keys} role="presentation">
      {#if installing && !unknownMachine}
        <SkillInstallPanel {sessionId} skill={installing} host={data?.host ?? ''} {tool} onclose={installed} />
      {:else}
      <input class="filter" type="text" role="searchbox" bind:this={input} bind:value={query} aria-label="Filter skills" placeholder="Filter skills" autocomplete="off" spellcheck="false" />
      {#if loading}
        <p class="note" role="status">Reading skills…</p>
      {:else if error}
        <div class="note" role="alert">
          <p>{error}</p>
          <button type="button" class="retry" onclick={onretry}>Retry</button>
        </div>
      {:else if data}
        <div class="groups" bind:this={list}>
          {#each groups as g (g.key)}
            <div class="group">
              <div class="heading" id={`${id}-${g.key}`}>{g.label}</div>
              <div class="list">
                {#each runsOf(g.skills) as run (run[0].id)}
                  <!-- A listbox may own only options: rows without an action share the group's listbox; a row with an action is its own
                       one-option listbox with the action beside it, outside. -->
                  <div class="item">
                    <div class="single" role="listbox" aria-labelledby={`${id}-${g.key}`}>
                      {#each run as s (s.id)}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <div class="option" id={`${id}-o-${s.id}`} role="option" tabindex="-1" aria-selected="false" aria-disabled={s.invocation ? undefined : 'true'} onclick={() => pick(s)}>
                          <span class="name">{s.name}</span>
                          {#if s.description}<span class="description">{s.description}</span>{/if}
                          <span class="meta">
                            <span class="source">{s.source === 'reet' ? 'reet-hosted' : `On this machine · ${s.agent}`}</span>
                            {#if !s.invocation}<span class="reason">{reason(s)}</span>{/if}
                          </span>
                        </div>
                      {/each}
                    </div>
                    {#if hasAction(run[0])}
                      {#if !run[0].published}
                        <a class="publish" href="/hosting">Publish this skill to install it.</a>
                      {:else if !unknownMachine}
                        <!-- An unknown machine has no name to install on, so there is nothing to confirm. -->
                        <button type="button" class="install" aria-label={`Install ${run[0].name}`} onclick={() => (installing = run[0])}>Install</button>
                      {/if}
                    {/if}
                  </div>
                {/each}
              </div>
            </div>
          {/each}
        </div>
        {#if noMatch}<p class="note" role="status">No skills match.</p>{/if}
        {#if unknownMachine}<p class="note" role="status">This session's machine is not known to reet, so its skills cannot be listed.</p>
        {:else}
          {#if empty}<p class="note" role="status">No skills found on {data.host} yet.</p>{/if}
          <p class="seen">{seenLine(data.machineSeen, data.host, now)}</p>
        {/if}
      {/if}
      {/if}
    </div>
  </div>
</dialog>

<style>
  dialog { width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; overflow: hidden; background: transparent; color: var(--ink); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); }
  .card { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 32px); overflow: hidden; padding: 8px 8px 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); overflow-wrap: anywhere; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 4px 4px 4px 12px; }
  h2 { margin: 0; font: 600 1.15rem/1.3 var(--serif); }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 10px; background: none; color: var(--muted); cursor: pointer; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 20px; height: 20px; }
  .body { display: flex; flex-direction: column; gap: 8px; min-height: 0; flex: 1; overflow-y: auto; overflow-x: hidden; padding: 4px; }
  .filter { width: 100%; min-width: 0; min-height: 36px; padding: 6px 10px; font: inherit; font-size: 16px; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; box-sizing: border-box; }
  .groups { display: flex; flex-direction: column; gap: 10px; }
  .heading { padding: 4px 10px 2px; font-size: 11px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .list { display: flex; flex-direction: column; gap: 2px; }
  .item { display: flex; flex-direction: column; align-items: stretch; }
  .install { align-self: flex-start; min-height: 36px; margin: 0 0 6px 10px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--accent); font: inherit; font-weight: 600; cursor: pointer; }
  .publish { margin: 0 0 6px 10px; padding: 2px 0; font-size: 12px; color: var(--accent); }
  .option { display: flex; flex-direction: column; gap: 2px; min-height: 40px; justify-content: center; padding: 6px 10px; border-radius: 8px; color: var(--ink); cursor: pointer; }
  .option:hover, .option:focus-visible { background: var(--tint); }
  .option:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .option[aria-disabled='true'] { cursor: default; opacity: 0.6; }
  .option[aria-disabled='true']:hover { background: none; }
  .name { font-weight: 600; }
  .description { font-size: 12px; line-height: 1.35; color: var(--muted); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 10px; font-size: 12px; color: var(--muted); }
  .reason { font-weight: 600; }
  .note, .seen { margin: 0; padding: 4px 10px; font-size: 13px; color: var(--muted); }
  .note p { margin: 0 0 8px; }
  .seen { font-size: 12px; }
  .retry { min-height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--accent); font: inherit; font-weight: 600; cursor: pointer; }
  @media (max-width: 560px) { .card { border-radius: 14px; } }
  @media (pointer: coarse) {
    .option { min-height: 44px; padding: 8px 12px; }
    .filter, .retry, .install { min-height: 44px; }
    .publish { display: inline-flex; align-items: center; min-height: 44px; }
  }
</style>
