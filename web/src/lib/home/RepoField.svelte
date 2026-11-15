<script lang="ts">
  import { githubRepos, refusalSentence, TerminalError, type GitHubRepo, type TerminalMachine } from '../terminal/api'
  import { timedOut } from '../githubRepo'
  import { elapsed } from '../time'

  // The Repository field wherever a GitHub repository is named (Add a project, Start an agent, a Factory outcome): owner/name
  // or a github.com link, typed, or chosen from the repositories the machine's gh sign-in reaches, read once per machine
  // while the field shows and offered under it as it is typed in. Anything can still be typed: a repository that is not
  // listed clones the same way. A machine that cannot list keeps the plain field. The parent checks what was typed.
  let { id, machine, value = $bindable(''), problem = '', disabled = false, hidden = false, onleave, onsignedout, oninput }: {
    /** The field's id; the list, its options and the hint take ids from it. */
    id: string
    machine: TerminalMachine | undefined
    value?: string
    /** What the parent says about what was typed (an unreadable repository), shown under the field. */
    problem?: string
    disabled?: boolean
    /** Whether the field is out of sight; the list is read only while it shows. */
    hidden?: boolean
    /** The person left the field, or pressed Enter on something not chosen from the list: say what is wrong now. */
    onleave?: () => void
    onsignedout: (e: unknown) => void
    oninput?: () => void
  } = $props()

  type Listing = { state: 'loading' } | { state: 'ready'; repos: GitHubRepo[]; more: boolean } | { state: 'failed'; sentence: string }
  let listings = $state<Record<string, Listing>>({})
  let input = $state<HTMLInputElement>(), listbox = $state<HTMLUListElement>()
  let open = $state(false), active = $state(-1)
  const life = new AbortController()
  $effect(() => () => life.abort())

  const canList = $derived(!!machine?.capabilities?.repoList)
  const listing = $derived(canList && machine ? listings[machine.agentId] : undefined)
  const query = $derived(value.trim().toLowerCase())
  const matches = $derived(listing?.state === 'ready' ? listing.repos.filter(r => r.name.toLowerCase().includes(query)) : [])
  const showList = $derived(open && !disabled && listing?.state === 'ready' && matches.length > 0)
  const optionId = (i: number) => `${id}-option-${i}`

  /** Puts the keyboard in the field. */
  export function focus() { input?.focus() }

  async function list(agentId: string, label: string) {
    listings[agentId] = { state: 'loading' }
    try {
      const answer = await githubRepos(agentId, life.signal)
      if (life.signal.aborted) return
      listings[agentId] = { state: 'ready', repos: answer.repos ?? [], more: !!answer.more }
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      listings[agentId] = { state: 'failed', sentence: timedOut(e) ? `${label} didn't list its repositories in time. Type one instead.` : refusalSentence(e) }
    }
  }
  $effect(() => {
    if (hidden || !canList || !machine || listings[machine.agentId]) return
    void list(machine.agentId, machine.host)
  })
  // The highlighted option stays in view as the arrows move it.
  $effect(() => { if (showList && active >= 0) listbox?.querySelector(`#${CSS.escape(optionId(active))}`)?.scrollIntoView({ block: 'nearest' }) })

  function choose(r: GitHubRepo) {
    value = r.name; open = false; active = -1
    oninput?.()
    onleave?.()
  }
  function keys(event: KeyboardEvent) {
    if (canList && listing?.state === 'ready') {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault()
        if (!open) { open = true; active = event.key === 'ArrowDown' ? 0 : matches.length - 1; return }
        active = event.key === 'ArrowDown' ? Math.min(active + 1, matches.length - 1) : Math.max(active - 1, 0)
        return
      }
      if (event.key === 'Enter' && showList && active >= 0 && matches[active]) { event.preventDefault(); choose(matches[active]); return }
      // Escape closes the list, not the dialog.
      if (event.key === 'Escape' && showList) { event.preventDefault(); event.stopPropagation(); open = false; active = -1; return }
    }
    if (event.key === 'Enter') onleave?.()
  }
  const pushed = (r: GitHubRepo) => {
    if (r.pushedAt <= 0) return ''
    const ago = elapsed(r.pushedAt * 1000)
    return ago === 'just now' ? 'pushed just now' : `pushed ${ago} ago`
  }
</script>

<div class="combo">
  <input {id} class="mono" bind:this={input} bind:value {disabled} onchange={() => onleave?.()}
    onkeydown={keys} oninput={() => { open = true; active = -1; oninput?.() }} onclick={() => (open = true)} onblur={() => { open = false; active = -1 }}
    role={canList ? 'combobox' : undefined} aria-autocomplete={canList ? 'list' : undefined}
    aria-expanded={canList ? showList : undefined} aria-controls={canList ? `${id}-list` : undefined}
    aria-activedescendant={showList && active >= 0 ? optionId(active) : undefined}
    placeholder={listing?.state === 'ready' && listing.repos.length ? 'Search or type owner/name' : undefined}
    autocomplete="off" autocapitalize="off" spellcheck="false" maxlength="300"
    aria-invalid={problem ? 'true' : undefined} aria-describedby={`${id}-hint${problem ? ` ${id}-note` : ''}`} />
  {#if canList}
    <!-- Always in the page, so aria-controls names it; shown while there is something to choose. -->
    <ul class="options" id={`${id}-list`} role="listbox" aria-label={`${machine?.host ?? 'The machine'}'s repositories`} bind:this={listbox} hidden={!showList}>
      {#each showList ? matches : [] as r, i (r.name)}
        <!-- The keyboard stays in the field (arrows, Enter, Escape) and names the option with aria-activedescendant, as a
             combobox does, so an option takes no keys of its own. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li id={optionId(i)} role="option" aria-selected={i === active} class:active={i === active}
          onpointerdown={(event) => event.preventDefault()} onclick={() => choose(r)}>
          <span class="option-name">{r.name}</span>
          <span class="option-meta">{#if r.private}<span class="private">private</span>{/if}{pushed(r)}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
<p class="hint" id={`${id}-hint`}>{#if listing?.state === 'loading'}Reading {machine?.host}'s repositories from GitHub…{:else if listing?.state === 'failed'}{listing.sentence}{:else if listing?.state === 'ready' && listing.repos.length}Choose from {machine?.host}'s {listing.more ? `${listing.repos.length} most recently pushed ` : ''}GitHub repositories, or type owner/name or a github.com link{:else if listing?.state === 'ready'}{machine?.host}'s GitHub sign-in reaches no repositories. Type owner/name or a github.com link{:else}owner/name, or a github.com link{/if}</p>
{#if problem}<p class="note" id={`${id}-note`}>{problem}</p>{/if}

<style>
  /* The list sits in the form's flow under the field, not over it: a dialog body scrolls and would clip a floating list. */
  .combo { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  input { width: 100%; min-width: 0; height: 42px; min-height: 42px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--mono); }
  input::placeholder { color: var(--faint); }
  .options { max-height: min(280px, 45vh); overflow: auto; margin: 0; padding: 4px;
    list-style: none; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card-raised, var(--card)); box-shadow: var(--shadow); }
  .options[hidden] { display: none; }
  .options li { display: flex; flex-direction: column; gap: 2px; min-height: 44px; justify-content: center; padding: 6px 10px; border-radius: 6px; cursor: pointer; }
  .options li.active, .options li:hover { background: var(--bg-sunk, var(--bg)); }
  .option-name { font-family: var(--mono); font-size: 14px; color: var(--ink); overflow-wrap: anywhere; }
  .option-meta { display: flex; gap: 8px; font-size: 14px; color: var(--muted); }
  .private { padding: 0 6px; border: 1px solid var(--line-strong); border-radius: 5px; }
  .hint, .note { margin: 0; font-size: 14px; line-height: 1.45; color: var(--muted); overflow-wrap: anywhere; }
  .note { color: var(--bad); }
  @media (pointer: coarse) { input { height: 44px; min-height: 44px; } }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    input { height: 48px; min-height: 48px; border-radius: 10px; background: var(--card); font-size: 16px; }
  }
</style>
