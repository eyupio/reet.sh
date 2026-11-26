<script lang="ts">
  import { tick } from 'svelte'
  import { ApiError, createGafferPathway, deleteGafferPathway, saveGafferPathway, UnauthorizedError } from '../../api'
  import ConfirmDialog from '../../ConfirmDialog.svelte'
  import DeskMenu from '../../desks/DeskMenu.svelte'
  import { MAX_PATHWAY_NAME, stageTitle, type Pathway } from '../pathways'

  // The person's pathways, one card each: the name (into the editor), the stage line, the description and anything that
  // stops work starting on it, with a switch that turns it on or off at once and a menu with Edit, Duplicate and Delete.
  let { pathways, onopen, onnew, onchange, onsaved, onremoved }: {
    pathways: Pathway[]
    onopen: (id: string) => void
    onnew: (start: 'blank' | 'standard') => void
    /** Read the list again: after someone else's save, or to meet a sign-out. */
    onchange: () => void | Promise<void>
    /** The server's answer to a save here, to take that pathway's place (its new updatedAt with it). */
    onsaved: (p: Pathway) => void
    /** A pathway deleted here, to leave the list. */
    onremoved: (id: string) => void
  } = $props()

  let notice = $state<{ text: string; bad: boolean } | null>(null)
  let noticeEl = $state<HTMLElement>()
  // A switch's new value while it saves; it goes back to the saved value if the save fails.
  let flipping = $state<Record<string, boolean>>({})
  let working = $state(false)
  const enabled = (p: Pathway) => flipping[p.id] ?? p.enabled
  const quoted = (name: string) => `“${name}”`
  // Signed out meanwhile: reading the list again meets the same answer, and the page hands it to the shell.
  const signedOut = async (e: unknown) => { if (!(e instanceof UnauthorizedError)) return false; await onchange(); return true }
  const say = (text: string, bad = false) => { notice = { text, bad } }

  async function toggle(p: Pathway) {
    if (p.id in flipping) return
    const next = !p.enabled
    flipping[p.id] = next
    notice = null
    try {
      onsaved(await saveGafferPathway({ ...p, enabled: next }))
    } catch (e) {
      if (await signedOut(e)) return
      if (e instanceof ApiError && e.code === 'pathway_changed') {
        say(`${quoted(p.name)} was changed somewhere else, so the list now shows it as it is saved. Switch it again if you still want it ${next ? 'on' : 'off'}.`, true)
        await onchange()
      } else say(`${quoted(p.name)} could not be switched ${next ? 'on' : 'off'} just now. Try again.`, true)
    } finally { delete flipping[p.id] }
  }

  /** "<name> copy", then "<name> copy 2"…, cut so the whole name fits; names are unique ignoring case, as the server holds them. */
  function copyName(name: string): string {
    const taken = new Set(pathways.map(p => p.name.toLocaleLowerCase()))
    for (let n = 1; ; n++) {
      const tail = n === 1 ? ' copy' : ` copy ${n}`
      const candidate = [...name].slice(0, MAX_PATHWAY_NAME - tail.length).join('').trimEnd() + tail
      if (!taken.has(candidate.toLocaleLowerCase())) return candidate
    }
  }
  async function duplicate(p: Pathway) {
    if (working) return
    working = true; notice = null
    try {
      const made = await createGafferPathway({ name: copyName(p.name), description: p.description, enabled: p.enabled, stages: p.stages })
      onopen(made.id)
    } catch (e) {
      if (await signedOut(e)) return
      // The limit and a name taken meanwhile are refusals the server words; anything else is a failure to try again.
      say(e instanceof ApiError && e.code ? e.message : `${quoted(p.name)} could not be duplicated just now. Try again.`, true)
      opener?.focus()
    } finally { working = false }
  }

  let deleting = $state<Pathway | null>(null)
  async function remove() {
    const p = deleting
    deleting = null
    if (!p || working) return
    working = true; notice = null
    try {
      await deleteGafferPathway(p.id)
      onremoved(p.id)
      say(`Sorted. ${quoted(p.name)} is deleted.`)
      await tick()
      noticeEl?.focus()
    } catch (e) {
      if (await signedOut(e)) return
      say(`${quoted(p.name)} could not be deleted just now. Try again.`, true)
      opener?.focus()
    } finally { working = false }
  }

  let menu = $state<{ pathway: Pathway; x: number; y: number } | null>(null)
  let opener = $state<HTMLElement | null>(null)
  function openMenu(event: MouseEvent, pathway: Pathway) {
    if (menu?.pathway.id === pathway.id) { closeMenu(false); return }
    opener = event.currentTarget as HTMLElement
    const box = opener.getBoundingClientRect()
    // The menu keeps itself 320 px clear of the window's foot (DeskMenu), which would pull it over this button near the
    // bottom; there it opens above instead. Three items of 34 px (44 px on touch) and the menu's padding.
    const tall = 3 * (matchMedia('(pointer: coarse)').matches ? 44 : 34) + 10
    menu = { pathway, x: box.right - 210, y: box.bottom + 4 > innerHeight - 320 ? box.top - 4 - tall : box.bottom + 4 }
  }
  function closeMenu(focus: boolean) { menu = null; if (focus) opener?.focus() }
  function pick(action: (p: Pathway) => void) { const p = menu?.pathway; closeMenu(false); if (p) action(p) }

  /** The name opens the editor in this tab; a modified click is left to the browser. */
  function follow(event: MouseEvent, id: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onopen(id)
  }
</script>

{#if notice}
  <div class="notice" class:bad={notice.bad}>
    <p bind:this={noticeEl} tabindex="-1" role={notice.bad ? 'alert' : 'status'}>{notice.text}</p>
    <button class="dismiss" type="button" aria-label="Dismiss this message" onclick={() => (notice = null)}>✕</button>
  </div>
{/if}

{#if pathways.length}
  <ul class="cards" aria-label="Pathways">
    {#each pathways as p (p.id)}
      {@const on = enabled(p)}
      <li class="card" class:off={!on}>
        <div class="body">
          <h2><a href={`/factory/pathways/${encodeURIComponent(p.id)}`} onclick={event => follow(event, p.id)}>{p.name}</a></h2>
          <ol class="line" aria-label={`Stages of ${p.name}`}>
            {#each p.stages as stage, i (i)}<li>{#if i}<span class="sep" aria-hidden="true">›</span>{/if}<span class="chip" class:own={stage.kind === 'custom'}>{stageTitle(stage)}</span></li>{/each}
          </ol>
          {#if p.description || !on}<p class="desc">{p.description}{#if !on}{p.description ? ' ' : ''}<span class="off-note">Off: not offered to you or Gaffer.</span>{/if}</p>{/if}
          {#each p.problems ?? [] as problem, i (i)}<p class="problem"><span class="mark" aria-hidden="true">!</span> {problem}</p>{/each}
        </div>
        <div class="controls">
          <button type="button" role="switch" class="toggle" aria-checked={on} aria-label={`Offer ${p.name} when starting work`}
            aria-busy={p.id in flipping} onclick={() => void toggle(p)}>
            <span class="switch" aria-hidden="true"><span class="knob"></span></span><span class="state" aria-hidden="true">{on ? 'On' : 'Off'}</span>
          </button>
          <button type="button" class="more" aria-label={`Actions for ${p.name}`} aria-haspopup="menu" aria-expanded={menu?.pathway.id === p.id} onclick={event => openMenu(event, p)}>…</button>
        </div>
      </li>
    {/each}
  </ul>
{:else}
  <div class="empty">
    <h2>No pathways yet.</h2>
    <p>A pathway is a line of stages you choose once and reuse when you start work: which stages run, in what order, and who does each.</p>
    <div class="empty-actions">
      <button class="btn primary" type="button" onclick={() => onnew('blank')}>New pathway</button>
      <button class="btn" type="button" onclick={() => onnew('standard')}>Start from the standard stages</button>
    </div>
  </div>
{/if}

{#if menu}
  {@const p = menu.pathway}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${p.name}`} {opener} onclose={closeMenu}>
    <button type="button" role="menuitem" onclick={() => pick(t => onopen(t.id))}>Edit</button>
    <button type="button" role="menuitem" disabled={working} onclick={() => pick(t => void duplicate(t))}>Duplicate</button>
    <button type="button" role="menuitem" class="danger" disabled={working} onclick={() => pick(t => (deleting = t))}>Delete</button>
  </DeskMenu>
{/if}

{#if deleting}
  <ConfirmDialog title={`Delete ${quoted(deleting.name)}?`} body="It is no longer offered when you or Gaffer start work. Work already running on it is not affected."
    confirmLabel="Delete" danger onconfirm={() => void remove()} oncancel={() => { deleting = null; opener?.focus() }} />
{/if}

<style>
  .notice { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 16px; padding: 10px 8px 10px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice.bad { border-color: var(--bad); }
  .notice p { margin: 0; padding: 4px 0; min-width: 0; overflow-wrap: anywhere; }
  .notice p:focus { outline: none; }
  .notice.bad p { color: var(--bad); }
  .dismiss { flex: none; width: 32px; height: 32px; border: 0; border-radius: 6px; background: none; color: var(--muted); font: inherit; cursor: pointer; }
  .dismiss:hover { color: var(--ink); background: var(--line-soft); }

  .cards { display: grid; gap: 12px; margin: 0; padding: 0; list-style: none; }
  .card { display: flex; align-items: flex-start; gap: 16px; padding: 18px 16px 18px 20px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .body { flex: 1; min-width: 0; display: grid; gap: 10px; }
  h2 { margin: 0; font: 600 1.25rem/1.3 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  h2 a { color: var(--ink); text-decoration: none; border-radius: 4px; }
  h2 a:hover { text-decoration: underline; text-decoration-color: color-mix(in srgb, currentColor 45%, transparent); text-underline-offset: 3px; }
  h2 a:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
  .line { display: flex; flex-wrap: wrap; align-items: center; row-gap: 6px; margin: 0; padding: 0; list-style: none; }
  .line li { display: flex; align-items: center; }
  /* The joining mark follows its stage, so a wrapped line never starts with one; on a narrow screen, where the line
     wraps, the chips stand apart with no mark at all, so none is left hanging at a line's end. */
  .sep { padding: 0 6px; color: var(--faint); }
  .chip { padding: 1px 10px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--ink-soft); font-size: 14px; line-height: 20px; white-space: nowrap; }
  .chip.own { border-color: color-mix(in srgb, var(--accent) 40%, var(--line)); background: var(--accent-wash); color: var(--accent-text); }
  .desc, .problem { margin: 0; font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
  .desc { color: var(--muted); }
  .problem { color: var(--warn); }
  .mark { font-weight: 700; }
  /* Off: the pathway's own text steps back; its switch and menu stay at full strength. */
  .card.off :is(h2, .line) { opacity: 0.6; }

  .controls { display: flex; align-items: center; gap: 2px; flex: none; margin-top: -4px; }
  .toggle { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 14px; cursor: pointer; }
  .toggle:hover .switch { border-color: var(--muted); }
  .toggle[aria-checked='true']:hover .switch { border-color: var(--accent); }
  .toggle:focus-visible { outline: 2px solid var(--focus); outline-offset: 0; }
  .toggle[aria-busy='true'] { cursor: progress; }
  .state { min-width: 26px; text-align: left; }
  .toggle[aria-checked='true'] .state { color: var(--ink); }
  .switch { position: relative; flex: none; width: 42px; height: 24px; box-sizing: border-box; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  .toggle[aria-checked='true'] .switch { background: var(--accent); border-color: var(--accent); }
  .knob { position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--card); box-shadow: var(--shadow); }
  .toggle[aria-checked='false'] .knob { background: var(--line-strong); box-shadow: none; }
  .toggle[aria-checked='true'] .knob { transform: translateX(18px); }
  @media (prefers-reduced-motion: no-preference) { .knob { transition: transform 0.16s var(--ease-out), background-color 0.16s; } }
  .more { width: 36px; height: 36px; flex: none; border: 0; border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 20px; line-height: 1; padding: 0 0 8px; cursor: pointer; }
  .more:hover, .more[aria-expanded='true'] { color: var(--ink); background: var(--line-soft); }
  .more:focus-visible { outline: 2px solid var(--focus); outline-offset: 0; }

  /* DeskMenu draws every item in the ink colour; Delete keeps the danger colour. */
  :global(.desk-menu) [role='menuitem'].danger:not(:disabled) { color: var(--bad); }

  .empty { display: grid; justify-items: center; gap: 10px; padding: 40px 24px; border: 1px dashed var(--line-strong); border-radius: var(--radius); text-align: center; }
  .empty h2 { font-size: 1.375rem; }
  .empty p { margin: 0; max-width: 52ch; color: var(--muted); }
  .empty-actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin-top: 6px; }

  @media (pointer: coarse) {
    .toggle, .more, .dismiss { min-height: 44px; }
    .more, .dismiss { width: 44px; height: 44px; }
    h2 a { display: inline-flex; align-items: center; min-height: 44px; margin: -9px 0; }
  }
  /* A phone: the name and the controls share the top row, and the rest takes the card's whole width beneath them. */
  @media (max-width: 650px) {
    .line { gap: 6px 8px; }
    .sep { display: none; }
  }
  @media (max-width: 560px) {
    .card { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px 8px; padding: 14px 8px 16px 16px; }
    .body { display: contents; }
    h2 { grid-column: 1; grid-row: 1; align-self: center; }
    .controls { grid-column: 2; grid-row: 1; margin: -6px 0; }
    .body > :not(h2) { grid-column: 1 / -1; margin-right: 8px; }
    .empty { padding: 28px 16px; }
    .empty-actions { flex-direction: column; align-self: stretch; }
  }
</style>
