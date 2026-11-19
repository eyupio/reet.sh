<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { shell } from '../dialogShell'
  import { ApiError, decideNewSkills, UnauthorizedError, type NewSkill } from '../api'
  import { toastText } from './skillsPage'

  // The review of skills found on a machine that reet did not put there. Each is asked about with Keep (it goes on every
  // machine) and Archive (it comes off the machines it was found on and reet remembers it), plus Keep all. Nothing
  // spreads until the person presses one. When the machines' copies differ, Keep asks which machine's copy to take.
  let { items, ondecided, onclose, onerror }: {
    items: NewSkill[]
    /** After each decision, so the page reads its rows, counts and banner again. */
    ondecided: () => void
    onclose: () => void
    /** A refusal the dialog cannot answer itself, such as a sign-in that has ended. */
    onerror: (e: unknown) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let list = $state<HTMLElement>()
  let foot = $state<HTMLElement>()
  const titleId = $props.id()
  // The skills as they were when the review opened. It keeps its own list and title while it is open, so a decision
  // never reorders it or renames it, whatever the page reads in the meantime.
  const asked = untrack(() => items.map(item => ({ ...item, copies: [...item.copies] })))
  // The skills still to decide, and the machine whose copy Keep takes for each.
  let remaining = $state<NewSkill[]>(asked)
  let chosen = $state<Record<string, string>>(Object.fromEntries(asked.map(item => [item.name, item.copies[0]?.agentId ?? ''])))
  let busy = $state('')
  let said = $state('')
  let error = $state('')

  const hostsOf = (skills: NewSkill[]) => [...new Set(skills.flatMap(s => s.copies.map(c => c.host)))]
  const phrase = (hosts: string[]) => hosts.length === 1 ? hosts[0] : hosts.length === 2 ? `${hosts[0]} and ${hosts[1]}` : 'your machines'
  const where = phrase(hostsOf(asked))
  const folders = (skill: NewSkill) => [...new Set(skill.copies.map(c => c.folder))].join(', ')
  // A machine with the skill in two folders is named with the folder.
  const copyLabel = (skill: NewSkill, copy: NewSkill['copies'][number]) => skill.copies.filter(c => c.host === copy.host).length > 1 ? `${copy.host} · ${copy.folder}` : copy.host

  onMount(() => {
    dialog?.showModal()
    foot?.querySelector<HTMLButtonElement>('[data-first]')?.focus()
    return () => dialog?.close()
  })

  function close() {
    if (busy) return
    dialog?.close()
    onclose()
  }

  // `failed` keeps what a later decision in the same press said went wrong (Keep all that kept some, then failed).
  async function settle(names: string[], action: 'keep' | 'archive', failed = false) {
    const decided = remaining.filter(item => names.includes(item.name))
    const index = remaining.findIndex(item => names.includes(item.name))
    remaining = remaining.filter(item => !names.includes(item.name))
    said = action === 'keep' ? toastText('kept', names, 0) : `Archived ${names.length === 1 ? names[0] : `${names.length} skills`} · coming off ${phrase(hostsOf(decided))}`
    if (!failed) error = ''
    ondecided()
    await tick()
    // Focus stays in the review: the next skill's Keep, or Close once all are sorted.
    const keeps = list?.querySelectorAll<HTMLButtonElement>('[data-keep]') ?? []
    const next = keeps[Math.min(Math.max(index, 0), keeps.length - 1)] ?? foot?.querySelector<HTMLButtonElement>('[data-first]')
    next?.focus()
  }

  function refused(e: unknown) {
    if (e instanceof ApiError && e.code === 'choose-copy') {
      // The copies differ after all: ask which machine's copy to keep.
      remaining = remaining.map(item => e.names.includes(item.name) ? { ...item, digestsDiffer: true } : item)
      error = e.message
      return
    }
    if (e instanceof ApiError && e.code === 'not-new') {
      remaining = remaining.filter(item => !e.names.includes(item.name))
      error = e.message
      ondecided()
      return
    }
    if (e instanceof UnauthorizedError) { onerror(e); return }
    error = e instanceof Error ? e.message : String(e)
  }

  async function keep(item: NewSkill) {
    if (busy) return
    busy = item.name
    try {
      await decideNewSkills([item.name], 'keep', item.digestsDiffer ? chosen[item.name] : undefined)
      await settle([item.name], 'keep')
    } catch (e) {
      refused(e)
    } finally {
      busy = ''
    }
  }

  async function archive(item: NewSkill) {
    if (busy) return
    busy = item.name
    try {
      await decideNewSkills([item.name], 'archive')
      await settle([item.name], 'archive')
    } catch (e) {
      refused(e)
    } finally {
      busy = ''
    }
  }

  // Keep all: the skills whose copies agree in one request, and each one whose copies differ with its chosen machine.
  async function keepAll() {
    if (busy || !remaining.length) return
    busy = '*'
    const same = remaining.filter(item => !item.digestsDiffer).map(item => item.name)
    const differ = remaining.filter(item => item.digestsDiffer)
    const kept: string[] = []
    let failed = false
    try {
      if (same.length) {
        await decideNewSkills(same, 'keep')
        kept.push(...same)
      }
      for (const item of differ) {
        await decideNewSkills([item.name], 'keep', chosen[item.name])
        kept.push(item.name)
      }
    } catch (e) {
      failed = true
      refused(e)
    } finally {
      busy = ''
      if (kept.length) await settle(kept, 'keep', failed)
    }
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'full' }} aria-labelledby={titleId} oncancel={e => { e.preventDefault(); close() }}>
  <div class="box" data-shell-card>
    <header class="head" data-shell-head>
      <p class="eyebrow" data-shell-aside>FOUND ON A MACHINE</p>
      <h2 id={titleId}>New on {where}</h2>
      <p class="lede" data-shell-aside>These turned up on {where} without coming from reet. Nothing spreads until you choose. Keep puts a skill on every machine. Archive takes it off {hostsOf(asked).length > 2 ? 'those machines' : where} and remembers it.</p>
    </header>
    <div class="body" data-shell-body>
      <ul class="found" bind:this={list} aria-label="New skills">
        {#each remaining as item (item.name)}
          <li>
            <div class="what">
              <strong>{item.name}</strong>
              <span class="src">{item.copies.map(c => c.host).filter((h, i, all) => all.indexOf(h) === i).join(', ')} · {folders(item)}</span>
              {#if item.digestsDiffer}
                <div class="choose" role="radiogroup" aria-label={`Which copy of ${item.name} to keep`}>
                  <span class="choose-label">The copies differ. Keep the one from</span>
                  {#each item.copies as copy (copy.agentId + copy.folder)}
                    <label class="option">
                      <input type="radio" name={`copy-${item.name}`} value={copy.agentId} checked={chosen[item.name] === copy.agentId} onchange={() => (chosen[item.name] = copy.agentId)} />
                      {copyLabel(item, copy)}
                    </label>
                  {/each}
                </div>
              {/if}
            </div>
            <div class="acts">
              <button type="button" class="btn primary" data-keep aria-label={`Keep ${item.name}`} disabled={!!busy} aria-busy={busy === item.name} onclick={() => keep(item)}>Keep</button>
              <button type="button" class="btn" aria-label={`Archive ${item.name}`} disabled={!!busy} onclick={() => archive(item)}>Archive</button>
            </div>
          </li>
        {/each}
      </ul>
      {#if !remaining.length}<p class="sorted">All sorted.</p>{/if}
      <p class="said" role="status">{said}</p>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>
    <footer class="foot" bind:this={foot} data-shell-foot>
      {#if remaining.length}
        <span class="hint" data-shell-aside>Skipping leaves them on {where} only.</span>
        <div class="buttons">
          <button type="button" class="btn quiet" data-first disabled={busy === '*'} onclick={close}>Not now</button>
          <button type="button" class="btn" disabled={!!busy} aria-busy={busy === '*'} onclick={keepAll}>Keep all</button>
        </div>
      {:else}
        <div class="buttons">
          <button type="button" class="btn" data-first onclick={close}>Close</button>
        </div>
      {/if}
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(560px, calc(100vw - 32px)); max-width: none; max-height: calc(100dvh - 32px); overflow: auto; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); overflow-wrap: anywhere; }
  .head { padding: 24px 24px 8px; display: flex; flex-direction: column; gap: 8px; }
  .eyebrow { margin: 0; color: var(--accent-text); font: 500 0.875rem/1.4 var(--mono); letter-spacing: 0.1em; }
  h2 { margin: 0; font: 600 1.75rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 0; color: var(--ink-soft); font-size: 0.9375rem; line-height: 1.5; }
  .body { padding: 8px 24px; }
  .found { list-style: none; margin: 0; padding: 0; }
  .found li { display: flex; align-items: center; gap: 12px; padding: 14px 0; border-top: 1px solid var(--line-soft); flex-wrap: wrap; }
  .what { flex: 1 1 220px; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  strong { font: 600 1.1875rem/1.25 var(--serif); }
  .src { color: var(--muted); font: 0.875rem/1.4 var(--mono); }
  .choose { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; margin-top: 6px; font-size: 0.875rem; }
  .choose-label { color: var(--ink-soft); }
  .option { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; cursor: pointer; font-weight: 500; }
  .option input { width: 18px; height: 18px; margin: 0; }
  .acts { display: flex; gap: 8px; }
  .acts .btn { min-height: 44px; border-radius: 10px; }
  .sorted { margin: 0; padding: 18px 0; color: var(--good); font-weight: 600; }
  .said { margin: 0; color: var(--ink-soft); font-size: 0.875rem; }
  .error { margin: 8px 0 0; color: var(--bad); font-size: 0.875rem; }
  .foot { display: flex; justify-content: space-between; align-items: center; gap: 8px 12px; flex-wrap: wrap; padding: 12px 24px 24px; border-top: 1px solid var(--line-soft); margin-top: 8px; }
  .hint { color: var(--muted); font-size: 0.875rem; }
  .buttons { display: flex; gap: 8px; margin-left: auto; }
  .buttons .btn { min-height: 44px; border-radius: 10px; }
  @media (max-width: 560px) {
    .head { padding: 20px 20px 8px; }
    .body { padding: 8px 20px; }
    .foot { padding: 12px 20px 20px; }
    .acts, .buttons { width: 100%; }
    .acts .btn, .buttons .btn { flex: 1; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
