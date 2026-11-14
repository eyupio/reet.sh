<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { cloneFolder, parseGitHubRepo, startUnheardLine, timedOut, type CloneEnd, type OutstandingClone } from '../githubRepo'
  import { addFromGitHub, githubRepos, refusalSentence, TerminalError, type GitHubRepo, type TerminalMachine } from '../terminal/api'
  import { elapsed } from '../time'

  // Add a project From GitHub, the Add project dialog's second choice: a repository, a machine that can clone it and an
  // optional name. Clone and add asks the controller to start the clone; while it runs the form gives way to one line and
  // Close, and the page reads the clone until it ends (ProjectRailHost). Closing never stops the clone.
  let { machines, clone, initialHost, id, hidden = false, onstart, onsignedout, onclose }: {
    machines: TerminalMachine[]
    /** The page's outstanding clone, shown instead of the form while it runs. */
    clone: OutstandingClone | null
    initialHost?: string
    /** The dialog's id prefix, so this panel's fields have ids of their own. */
    id: string
    hidden?: boolean
    onstart: (clone: OutstandingClone) => void
    onsignedout: (e: unknown) => void
    onclose: () => void
  } = $props()

  const INVALID = 'Write the repository as owner/name, or paste its github.com link.'
  // Only machines that say they can clone from GitHub (a current reet with git and the GitHub CLI there).
  const able = $derived(machines.filter(m => m.capabilities?.repoClone))
  // svelte-ignore state_referenced_locally
  let host = $state(able.find(m => m.agentId === initialHost)?.agentId ?? (able.length === 1 ? able[0].agentId : ''))
  let repo = $state(''), name = $state('')
  // The repository field says it cannot read what was typed only once it is left, or on Enter.
  let left = $state(false)
  let starting = $state(false), message = $state(''), failed = $state(false)
  let repoInput = $state<HTMLInputElement>(), submitButton = $state<HTMLButtonElement>(), closeButton = $state<HTMLButtonElement>()
  const life = new AbortController()

  const parsed = $derived(parseGitHubRepo(repo))
  const machine = $derived(able.find(m => m.agentId === host))
  const ready = $derived(!!parsed && !!machine)
  const repoProblem = $derived(left && repo.trim() !== '' && !parsed ? INVALID : '')

  // The repositories the chosen machine's gh sign-in reaches, read once per machine while this panel shows, offered under
  // the field as it is typed in. Anything can still be typed: a repository that is not listed (older than the 200 most
  // recently pushed, or a public one of someone else's) clones the same way. A machine that cannot list keeps the plain field.
  type Listing = { state: 'loading' } | { state: 'ready'; repos: GitHubRepo[]; more: boolean } | { state: 'failed'; sentence: string }
  let listings = $state<Record<string, Listing>>({})
  const canList = $derived(!!machine?.capabilities?.repoList)
  const listing = $derived(canList && machine ? listings[machine.agentId] : undefined)
  let open = $state(false), active = $state(-1)
  let listbox = $state<HTMLUListElement>()
  const query = $derived(repo.trim().toLowerCase())
  const matches = $derived(listing?.state === 'ready' ? listing.repos.filter(r => r.name.toLowerCase().includes(query)) : [])
  const showList = $derived(open && listing?.state === 'ready' && matches.length > 0)
  const optionId = (i: number) => `${id}-gh-repo-option-${i}`

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
    if (hidden || clone || !canList || !machine || listings[machine.agentId]) return
    void list(machine.agentId, machine.host)
  })
  // The highlighted option stays in view as the arrows move it.
  $effect(() => { if (showList && active >= 0) listbox?.querySelector(`#${CSS.escape(optionId(active))}`)?.scrollIntoView({ block: 'nearest' }) })

  function choose(r: GitHubRepo) {
    repo = r.name; open = false; active = -1; left = true
    edited()
  }
  function repoKeys(event: KeyboardEvent) {
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
    if (event.key === 'Enter' && !ready) left = true
  }
  const pushed = (r: GitHubRepo) => (r.pushedAt > 0 ? (elapsed(r.pushedAt * 1000) === 'just now' ? 'pushed just now' : `pushed ${elapsed(r.pushedAt * 1000)} ago`) : '')

  /** Whether the start is on its way, so the dialog holds its tabs until the answer. */
  export function busy(): boolean { return starting }
  /** The clone this page started has ended without a project: the controller's sentence goes in the error slot and the
   *  form comes back as it was (filled with that clone when it had been emptied by a close). */
  export function ended(end: CloneEnd): boolean {
    if (end.state !== 'failed') return false
    if (repo.trim() === '' && clone) { repo = clone.repo; if (able.some(m => m.agentId === clone?.host)) host = clone.host }
    failed = true
    message = end.sentence
    void tick().then(() => submitButton?.focus())
    return true
  }

  function edited() { if (!starting && message) { message = ''; failed = false } }

  async function start() {
    left = true
    if (starting || !ready || !machine || !parsed) return
    starting = true; failed = false; message = ''
    try {
      const answer = await addFromGitHub(host, repo.trim(), name.trim(), life.signal)
      if (life.signal.aborted) return
      onstart({ id: answer.cloneId, host, machine: machine.host, repo: `${parsed.owner}/${parsed.repo}` })
      void tick().then(() => closeButton?.focus())
    } catch (e) {
      if (life.signal.aborted) return
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      failed = true
      // No answer inside the request's 15 seconds: the controller may have started the clone, and this page has no id to
      // follow it with, so it says that and not that the request never arrived.
      message = timedOut(e) ? startUnheardLine : refusalSentence(e)
      const field = e instanceof TerminalError ? e.field : undefined
      void tick().then(() => (field === 'repo' ? repoInput : field === 'host' ? document.getElementById(`${id}-gh-machine`) : field === 'name' ? document.getElementById(`${id}-gh-name`) : submitButton)?.focus())
    } finally {
      if (!life.signal.aborted) starting = false
    }
  }

  onMount(() => () => life.abort())
</script>

<div class="panel" id={`${id}-panel-github`} role="tabpanel" aria-labelledby={`${id}-tab-github`} {hidden}>
<form novalidate onsubmit={(event) => { event.preventDefault(); void start() }} oninput={edited}>
  <div class="body" data-shell-body>
    {#if clone}
      <!-- The line is in the live region below. -->
    {:else if able.length === 0}
      <p class="lead">No machine here can clone from GitHub yet. It needs a current reet, git and the GitHub CLI.</p>
    {:else}
      <fieldset disabled={starting}>
        <div class="field">
          <label class="label" for={`${id}-gh-repo`}>Repository</label>
          <div class="combo">
            <input id={`${id}-gh-repo`} class="mono" bind:this={repoInput} bind:value={repo} onchange={() => (left = true)}
              onkeydown={repoKeys} oninput={() => { open = true; active = -1 }} onclick={() => (open = true)} onblur={() => { open = false; active = -1 }}
              role={canList ? 'combobox' : undefined} aria-autocomplete={canList ? 'list' : undefined}
              aria-expanded={canList ? showList : undefined} aria-controls={canList ? `${id}-gh-repo-list` : undefined}
              aria-activedescendant={showList && active >= 0 ? optionId(active) : undefined}
              placeholder={listing?.state === 'ready' && listing.repos.length ? 'Search or type owner/name' : undefined}
              autocomplete="off" autocapitalize="off" spellcheck="false" maxlength="300"
              aria-invalid={repoProblem ? 'true' : undefined} aria-describedby={`${id}-gh-repo-hint${repoProblem ? ` ${id}-gh-repo-note` : ''}`} />
            {#if canList}
              <!-- Always in the page, so aria-controls names it; shown while there is something to choose. -->
              <ul class="options" id={`${id}-gh-repo-list`} role="listbox" aria-label={`${machine?.host ?? 'The machine'}'s repositories`} bind:this={listbox} hidden={!showList}>
                {#each showList ? matches : [] as r, i (r.name)}
                  <!-- The keyboard stays in the field (arrows, Enter, Escape) and names the option with aria-activedescendant, as
                       a combobox does, so an option takes no keys of its own. -->
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
          <p class="hint" id={`${id}-gh-repo-hint`}>{#if listing?.state === 'loading'}Reading {machine?.host}'s repositories from GitHub…{:else if listing?.state === 'failed'}{listing.sentence}{:else if listing?.state === 'ready' && listing.repos.length}Choose from {machine?.host}'s {listing.more ? `${listing.repos.length} most recently pushed ` : ''}GitHub repositories, or type owner/name or a github.com link{:else if listing?.state === 'ready'}{machine?.host}'s GitHub sign-in reaches no repositories. Type owner/name or a github.com link{:else}owner/name, or a github.com link{/if}</p>
          {#if repoProblem}<p class="field-note bad" id={`${id}-gh-repo-note`}>{repoProblem}</p>{/if}
        </div>
        <div class="pair">
          <div class="field">
            <label class="label" for={`${id}-gh-machine`}>Machine</label>
            <div class="select-wrap">
              <select id={`${id}-gh-machine`} bind:value={host} onchange={edited}>
                {#if host === ''}<option value="" disabled>Choose a machine</option>{/if}
                {#each able as m (m.agentId)}<option value={m.agentId}>{m.host}</option>{/each}
              </select>
              <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M6 9l6 6 6-6" /></svg>
            </div>
          </div>
          <div class="field">
            <label class="label" for={`${id}-gh-name`}>Name <span class="optional">optional</span></label>
            <input id={`${id}-gh-name`} bind:value={name} placeholder="Defaults to the repository name" autocomplete="off" maxlength="200" />
          </div>
        </div>
        {#if parsed && machine}<p class="where">reet will clone it to <span class="path">{cloneFolder(parsed)}</span> on {machine.host}.</p>{/if}
      </fieldset>
    {/if}
    <div class="live" role="status" aria-live="polite" aria-atomic="true">
      {#if clone}<p class="lead cloning">Cloning {clone.repo} on {clone.machine}… You can close this; the project appears when it's done.</p>{/if}
    </div>
  </div>
  <footer data-shell-foot>
    <p class="status" class:bad={failed} class:quiet={!message} role="status" aria-live="polite" aria-atomic="true">{message}</p>
    <div class="actions">
      {#if clone || able.length === 0}
        <button type="button" class="cancel" bind:this={closeButton} onclick={onclose}>Close</button>
      {:else}
        <button type="button" class="cancel" onclick={onclose} disabled={starting}>Cancel</button>
        <button type="submit" class="submit" bind:this={submitButton} disabled={starting || !ready} aria-busy={starting}>{starting ? 'Starting…' : 'Clone and add'}</button>
      {/if}
    </div>
  </footer>
</form>
</div>

<style>
  .panel { display: grid; min-height: 0; min-width: 0; }
  form { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; min-width: 0; }
  .panel[hidden] { display: none; }
  .body { min-height: 0; overflow: auto; padding: 16px 24px; }
  fieldset { margin: 0; padding: 0; border: 0; min-width: 0; display: flex; flex-direction: column; gap: 14px; }
  .field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  /* The list sits in the form's flow under the field, not over it: the dialog body scrolls and would clip a floating list. */
  .combo { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .options { max-height: min(280px, 45vh); overflow: auto; margin: 0; padding: 4px;
    list-style: none; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--card-raised, var(--card)); box-shadow: var(--shadow); }
  .options[hidden] { display: none; }
  .options li { display: flex; flex-direction: column; gap: 2px; min-height: 44px; justify-content: center; padding: 6px 10px; border-radius: 6px; cursor: pointer; }
  .options li.active, .options li:hover { background: var(--bg-sunk, var(--bg)); }
  .option-name { font-family: var(--mono); font-size: 14px; color: var(--ink); overflow-wrap: anywhere; }
  .option-meta { display: flex; gap: 8px; font-size: 14px; color: var(--muted); }
  .private { padding: 0 6px; border: 1px solid var(--line-strong); border-radius: 5px; }
  .label { color: var(--muted); font-size: 14px; }
  .optional { color: var(--faint); }
  input, select { width: 100%; min-width: 0; height: 42px; min-height: 42px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: 14px var(--sans); }
  .mono { font-family: var(--mono); }
  input::placeholder { color: var(--faint); }
  .select-wrap { position: relative; }
  .select-wrap select { appearance: none; padding-right: 34px; cursor: pointer; }
  .chev { position: absolute; right: 12px; top: 14px; width: 14px; height: 14px; color: var(--muted); pointer-events: none; }
  .hint, .field-note, .where, .lead { margin: 0; font-size: 14px; line-height: 1.45; color: var(--muted); overflow-wrap: anywhere; }
  .field-note.bad { color: var(--bad); }
  .where { color: var(--ink); }
  .path { font-family: var(--mono); }
  .lead { color: var(--ink); }
  /* The live regions stay in the accessibility tree from the start, empty and taking no room, so what is put in them is
     announced (a region that appears together with its words often is not). */
  .live:empty { margin: 0; padding: 0; }
  .pair { display: flex; gap: 12px; }
  .pair > .field { flex: 1; }
  footer { display: flex; align-items: center; gap: 10px; padding: 16px 24px; border-top: 1px solid var(--line-soft); }
  .status { flex-grow: 1; min-width: 0; margin: 0; font-size: 14px; color: var(--ink); overflow-wrap: anywhere; }
  .status.bad { color: var(--bad); }
  .actions { display: flex; gap: 10px; flex: none; margin-left: auto; }
  .cancel, .submit { height: 44px; border-radius: 8px; cursor: pointer; font: inherit; }
  .cancel { padding: 0 20px; border: 1px solid var(--line); background: transparent; color: var(--ink); font-weight: 500; }
  .cancel:hover:not(:disabled) { border-color: var(--line-strong); }
  .submit { padding: 0 24px; border: 1px solid var(--accent); background: var(--accent); color: var(--accent-ink); font-weight: 600; }
  .submit:hover:not(:disabled) { background: color-mix(in srgb, var(--accent) 90%, var(--ink)); }
  .submit:disabled, .cancel:disabled { opacity: .5; cursor: default; }
  @media (pointer: coarse) {
    input, select { height: 44px; min-height: 44px; }
    .chev { top: 15px; }
  }
  /* Phone: the dialog is a full-screen sheet; fields are 16 px so focusing one never zooms the page. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    form { background: var(--bg); }
    .body { padding: 14px 16px 16px; font-size: 15px; }
    fieldset { gap: 12px; }
    input, select { height: 48px; min-height: 48px; border-radius: 10px; background: var(--card); font-size: 16px; }
    .mono { font-size: 16px; }
    .chev { top: 17px; }
    .pair { flex-direction: column; }
    footer { flex-direction: column; align-items: stretch; gap: 10px; padding: 12px 16px 16px; background: var(--bg-sunk); }
    .status.quiet { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
    .cancel { width: 110px; height: 48px; padding: 0; border-color: var(--line-strong); border-radius: 10px; }
    .submit { flex: 1; height: 48px; border-radius: 10px; padding: 0 12px; }
    /* A short screen (sideways, or the keyboard open): the buttons keep the 44 px a touch needs and no more, so the
       repository field has room above the footer under the dialog's header and tabs. */
    :global(dialog[data-shell-short]) .cancel, :global(dialog[data-shell-short]) .submit { height: 44px; }
  }
</style>
