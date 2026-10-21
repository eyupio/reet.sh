<script lang="ts">
  import { groupLabel, installationStatus, type InstallationReport } from './machines'
  import { recordInstallCopy, type Environment, type Event, type Flags, type ListRef, type Skill, type Tag } from './api'
  import CopyButton from './CopyButton.svelte'
  import { eventLabel, eventTitle } from './events'
  import TrendSparkline from './TrendSparkline.svelte'
  import { installCommand, installPrompt, sourceURL, readInstallKind, writeInstallKind, type InstallKind } from './install'
  import { fmt, moves, stageTitle, type Stage } from './shelf'
  import { elapsed, isOldEnvironment, isStale, sinceLabel } from './time'

  let {
    skill, stage, tagOptions, selecting, selectedIDs, onselect, canWrite, compact = false, expanded = false, onexpand, installationReports = [],
    onpatch, onrefresh, onlink, onremove, onfiltertag, onfilterenv, ontags, onhistory, onopenlist,
  }: {
    skill: Skill
    stage: Stage
    tagOptions: string[]
    selecting: boolean
    selectedIDs: string[]
    canWrite: boolean
    compact?: boolean
    expanded?: boolean
    onexpand: (expanded: boolean) => void
    installationReports?: InstallationReport[]
    onselect: (s: Skill, e: MouseEvent) => void
    onpatch: (s: Skill, flags: Flags, act: string) => Promise<boolean>
    onrefresh: (s: Skill) => void
    onlink: (s: Skill, input: string) => Promise<boolean>
    onremove: (s: Skill) => void
    onfiltertag: (tag: Tag) => void
    onfilterenv: (env: Environment) => void
    ontags: (s: Skill, add: string[], remove: number[]) => Promise<boolean>
    onhistory: (s: Skill) => Promise<Event[]>
    onopenlist: (list: ListRef) => void
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
  let tagging = $state(false)
  let tagDraft = $state('')
  let tagBusy = $state(false)
  let tagStatus = $state('')
  let tagError = $state('')
  let tagInput = $state<HTMLInputElement>()
  let historyOpen = $state(false)
  let historyLoading = $state(false)
  let history = $state<Event[]>([])
  const isSelected = $derived(selectedIDs.includes(skill.id))
  const tags = $derived(skill.tags ?? [])
  const tagName = $derived(tagDraft.trim())
  const tagExists = $derived(tags.some((tag) => tag.name.toLowerCase() === tagName.toLowerCase()))
  const availableTags = $derived(tagOptions.filter((name) => !tags.some((tag) => tag.name.toLowerCase() === name.toLowerCase())))
  const environments = $derived(skill.environments ?? [])
  const machines = $derived([...new Set(environments.map(e => e.host))].map(host => ({ host, groups: environments.filter(e => e.host === host) })))
  const lists = $derived(skill.lists ?? [])
  const trend = $derived(skill.trend ?? [])
  const weeklyChange = $derived.by(() => {
    if (trend.length < 2) return null
    const prev = trend[trend.length - 2]
    const last = trend[trend.length - 1]
    if (!prev || !last) return null
    const pct = Math.round(((last - prev) / prev) * 100)
    return pct === 0 ? null : pct
  })

  const command = $derived(installCommand(skill))
  const prompt = $derived(installPrompt(skill))
  const source = $derived(sourceURL(skill))
  const installText = $derived(installKind === 'prompt' ? prompt : command)
  const installations = $derived(installationReports.map(report => ({ ...report, status: installationStatus(report, skill.id) })).sort((a, b) => {
    const rank = { installed: 0, missing: 1, unknown: 2 }
    return rank[a.status] - rank[b.status] || a.host.localeCompare(b.host)
  }))
  const visibleInstallations = $derived(expanded ? installations : installations.slice(0, 3))
  const installationLabels = { installed: 'Installed', missing: 'Missing', unknown: 'Not confirmed' }

  $effect(() => {
    if (!descEl || descOpen) return
    descLong = descEl.scrollHeight > descEl.clientHeight + 1
  })

  $effect(() => {
    if (tagging) tagInput?.focus()
  })

  function toggleTagging() {
    tagging = !tagging
  }

  async function updateTags(add: string[], remove: number[], success: string) {
    if (tagBusy) return
    tagBusy = true
    tagError = ''
    tagStatus = ''
    try {
      if (await ontags(skill, add, remove)) {
        if (add.length) tagDraft = ''
        tagStatus = success
      } else {
        tagError = 'Could not save tags. Try again.'
      }
    } catch {
      tagError = 'Could not save tags. Try again.'
    } finally {
      tagBusy = false
      tagInput?.focus()
    }
  }

  async function addTag(e: SubmitEvent) {
    e.preventDefault()
    if (!tagName || tagExists || tagBusy) return
    await updateTags([tagName], [], `Added ${tagName}.`)
  }

  async function removeTag(tag: Tag) {
    await updateTags([], [tag.id], `Removed ${tag.name} from this skill.`)
  }

  async function toggleHistory() {
    historyOpen = !historyOpen
    if (!historyOpen) return
    historyLoading = true
    history = await onhistory(skill)
    historyLoading = false
  }

  function envTitle(env: Environment): string {
    const when = new Date(env.lastSeen).toLocaleString()
    return `${env.path ? env.path + ' · ' : ''}last seen ${when}`
  }

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

<li class="skill {stage}" class:compact class:collapsed={!expanded} data-skill={skill.id}>
  <div class="row">
    {#if selecting}
      <button
        type="button" class="pickbox" role="checkbox" aria-checked={isSelected}
        aria-label="Select {skill.name}" onclick={(e) => onselect(skill, e)}
      >
        <span class="box" aria-hidden="true">
          {#if isSelected}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <path d="m5 12 4 4L19 6"></path>
            </svg>
          {/if}
        </span>
      </button>
    {/if}
    <div class="head">
      <h3 title={!expanded ? skill.name : undefined}>
        {#if skill.url.startsWith('https://www.skills.sh/')}
          <a href={skill.url} target="_blank" rel="noopener noreferrer">{skill.name}<span class="vh"> (opens skills.sh)</span></a>
        {:else}
          {skill.name}
        {/if}
      </h3>
      {#if source}
        <a class="source source-link" href={source} target="_blank" rel="noopener noreferrer" title={source} aria-label={`Source: ${skill.source} (opens in a new tab)`}>
          <code>{skill.source}</code><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 3h7v7M21 3 10 14M10 3H3v18h18v-7" /></svg>
        </a>
      {:else}<code class="source">Unlinked source</code>{/if}
    </div>
    <button type="button" class="disclosure" aria-expanded={expanded} aria-controls={`skill-details-${skill.id}`}
      aria-label={`${expanded ? 'Collapse' : 'Expand'} ${skill.name}`} onclick={() => onexpand(!expanded)}>
      <span>{expanded ? 'Collapse' : 'Expand'}</span>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
    </button>
  </div>
  <div class="core-data" role="group" aria-label={`Overview for ${skill.name}`}>
    <span class="stage-label">{stageTitle(stage)}</span>
    {#if skill.installs > 0}<span class="metric" title="Total installs reported by skills.sh">{fmt(skill.installs)} installs</span>{/if}
    {#if skill.installsWeek > 0}<span class="metric">{fmt(skill.installsWeek)}/week{#if weeklyChange !== null}<span class="change" class:down={weeklyChange < 0}>{weeklyChange > 0 ? '↑' : '↓'} {Math.abs(weeklyChange)}%</span>{/if}</span>{/if}
    {#if trend.length > 1}<TrendSparkline values={trend} name={skill.name} />{/if}
    {#if machines.length}<span class="metric" title="Machines in your reported inventory"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8M12 17v4" /></svg>{machines.length} {machines.length === 1 ? 'machine' : 'machines'}</span>{/if}
    {#if tags.length}<span class="metric">{tags.length} {tags.length === 1 ? 'tag' : 'tags'}</span>{/if}
    {#if lists.length}<span class="metric">{lists.length} {lists.length === 1 ? 'list' : 'lists'}</span>{/if}
    {#if skill.notes}<span class="metric" title="You have a personal note on this skill"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M14 3H5v18h14V8zM14 3v5h5M8 12h8M8 16h6" /></svg>Note</span>{/if}
    {#if stage === 'try' && isStale(skill.updatedAt)}<span class="stale">still to test?</span>{/if}
  </div>
  {#if installations.length}
    <ul class="installation-badges" aria-label={`Reported installation by environment for ${skill.name}`}>
      {#each visibleInstallations as report (report.host)}
        <li class="installation-badge {report.status}" title={report.status === 'installed' ? 'Present in at least one complete agent scan from the last 14 days.' : report.status === 'missing' ? 'Absent from all reported agent groups. All of this machine’s agent scans are complete and less than 14 days old.' : report.groupCount ? 'Rescan this machine: one or more agent inventories are incomplete or out of date.' : 'No agent inventory reported. A server heartbeat cannot confirm installed skills.'}>
          <span class="status-dot" aria-hidden="true"></span><span class="host">{report.host}</span><strong>{installationLabels[report.status]}</strong>
        </li>
      {/each}
      {#if !expanded && installations.length > visibleInstallations.length}
        <li><button type="button" class="environment-more" onclick={() => onexpand(true)} aria-label={`Show all ${installations.length} environments for ${skill.name}`}>+{installations.length - visibleInstallations.length} more</button></li>
      {/if}
    </ul>
  {/if}
  <div class="actions quick-actions" role="group" aria-label={`Quick actions for ${skill.name}`}>
    {#if canWrite}
      {#each moves(stage) as m, i}
        <button type="button" class="chip" class:move={m.primary} data-act={'m' + i} onclick={() => onpatch(skill, m.flags, 'm' + i)}>{m.label}</button>
      {/each}
      <button type="button" class="chip heart" data-act="like" aria-label={`Like ${skill.name}`} aria-pressed={skill.liked} onclick={() => onpatch(skill, { liked: !skill.liked }, 'like')}><span aria-hidden="true">{skill.liked ? '♥' : '♡'}</span>Like</button>
    {/if}
    {#if command}<CopyButton text={command} label="Copy install" ariaLabel={`Copy install command for ${skill.name}`} compact oncopy={() => void recordInstallCopy(skill.id).catch(() => {})} />{/if}
    {#if prompt}<CopyButton text={prompt} label="Copy prompt" ariaLabel={`Copy prompt for ${skill.name}`} compact oncopy={() => void recordInstallCopy(skill.id).catch(() => {})} />{/if}
  </div>

  <div id={`skill-details-${skill.id}`} class="card-details" hidden={!expanded}>
    {#if expanded}
      <p class="age" title={new Date(skill.updatedAt).toLocaleString()}>{sinceLabel(stage, skill.updatedAt)}</p>
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

      {#if canWrite || tags.length}
        <div class="tagrow" aria-label="Tags for {skill.name}">
          {#each tags as tag (tag.id)}
            {#if tagging && canWrite}
              <button type="button" class="tag remove" onclick={() => removeTag(tag)} disabled={tagBusy}
                aria-label="Remove tag {tag.name} from {skill.name}" title="Remove from this skill">
                {tag.name}<span aria-hidden="true">×</span>
              </button>
            {:else}
              <button type="button" class="tag" onclick={() => onfiltertag(tag)} title="Filter by {tag.name}">{tag.name}</button>
            {/if}
          {/each}
          {#if canWrite}
            <button type="button" class="tag add" aria-expanded={tagging} aria-controls="tag-editor-{skill.id}"
              onclick={toggleTagging} disabled={tagBusy}>{tagging ? 'Done' : tags.length ? 'Edit tags' : '+ Add tags'}</button>
          {/if}
        </div>
      {/if}

      {#if canWrite && tagging}
        <div class="tag-edit" id="tag-editor-{skill.id}" aria-busy={tagBusy}>
          <p class="tag-help" id="tag-help-{skill.id}">Tags save automatically. Use × to remove a tag from this skill; topic sync will not add it back.</p>
          {#if tags.length === 0}<p class="tag-help">No tags on this skill yet.</p>{/if}
          <form onsubmit={addTag}>
            <label class="vh" for="tag-{skill.id}">Add a tag to {skill.name}</label>
            <input id="tag-{skill.id}" bind:this={tagInput} list="tag-options-{skill.id}" bind:value={tagDraft}
              placeholder="Choose or create a tag" autocomplete="off" disabled={tagBusy} aria-describedby="tag-help-{skill.id}" />
            <datalist id="tag-options-{skill.id}">
              {#each availableTags as name}<option value={name}></option>{/each}
            </datalist>
            <button type="submit" class="btn primary" disabled={tagBusy || !tagName || tagExists}>{tagBusy ? 'Saving…' : 'Add tag'}</button>
          </form>
          {#if tagExists}<p class="tag-help">This skill already has that tag.</p>{/if}
          {#if tagError}<p class="tag-error" role="alert">{tagError}</p>{/if}
        </div>
      {/if}
      {#if canWrite}<p class="tag-status vh" role="status">{tagStatus}</p>{/if}
      {#if environments.length}
        <div class="envrow">
          {#each machines as machine (machine.host)}
            <div class="envmachine"><strong>{machine.host}</strong>
              {#each machine.groups as env (env.id)}
                <button type="button" class="envchip" class:old={isOldEnvironment(env.lastSeen)} onclick={() => onfilterenv(env)} title={envTitle(env)}>
                  <span class="envdot" aria-hidden="true"></span>{groupLabel(env)}
                </button>
              {/each}
            </div>
          {/each}
        </div>
      {/if}
      {#if lists.length}
        <div class="listrow">
          {#each lists as list (list.id)}
            <button type="button" class="listchip" onclick={() => onopenlist(list)} title="Open shared list {list.name}">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"></path>
              </svg>
              {list.name}
            </button>
          {/each}
        </div>
      {/if}

      <div class="actions detail-actions">
        {#if canWrite}<button type="button" class="btn quiet" data-act="notes" aria-expanded={editing} aria-label="Notes for {skill.name}" onclick={toggleNotes}>Notes</button>{/if}
        <button type="button" class="btn quiet" aria-expanded={historyOpen} aria-label="History for {skill.name}" onclick={toggleHistory}>History</button>
        {#if canWrite}
          {#if skill.sourceType === 'local'}
            <button type="button" class="btn quiet" aria-expanded={linking} aria-label="Link source for {skill.name}" onclick={() => (linking = !linking)}>Link source</button>
          {:else}
            <button type="button" class="btn quiet" aria-label="Refresh {skill.name}" onclick={() => onrefresh(skill)}>Refresh</button>
          {/if}
          <button type="button" class="btn quiet danger" aria-label="Remove {skill.name}" onclick={() => onremove(skill)}>Remove</button>
        {/if}
      </div>

      {#if historyOpen}
        <div class="history">
          {#if historyLoading}
            <p>Loading…</p>
          {:else if history.length === 0}
            <p>Nothing recorded yet.</p>
          {:else}
            <ul>
              {#each history as event (event.id)}
                <li title={eventTitle(event)}>
                  <span>{eventLabel(event)}</span>
                  <time datetime={new Date(event.at).toISOString()}>{elapsed(event.at)}</time>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

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
                  <CopyButton text={installText} compact oncopy={() => void recordInstallCopy(skill.id).catch(() => {})} />
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      {#if canWrite && editing}
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

      {#if canWrite && linking}
        <form class="edit link" onsubmit={link}>
          <label class="vh" for="link-{skill.id}">skills.sh URL for {skill.name}</label>
          <div>
            <input id="link-{skill.id}" bind:value={linkInput} placeholder="skills.sh URL for {skill.name}" />
            <button type="submit" class="btn primary">Link</button>
          </div>
        </form>
      {/if}
    {/if}
  </div>
</li>

<style>
  .envmachine { display:flex; flex-wrap:wrap; align-items:center; gap:5px; padding:5px 8px; border:1px solid var(--line); border-radius:8px; min-width:0; } .envmachine>strong { font-size:.7rem; font-weight:600; overflow-wrap:anywhere; }
  .skill { position: relative; padding: 20px 22px; border: 1px solid var(--line); border-left: 3px solid var(--line); border-radius: var(--radius); background: var(--card); min-width: 0; }
  .skill.use { border-left-color: var(--accent); }
  .skill.try { border-left-color: var(--accent); border-left-style: dashed; }
  .row { display: flex; align-items: center; gap: 10px; }
  .head { flex: 1; min-width: 0; display: grid; gap: 4px; }
  h3 { flex: 0 1 auto; min-width: 0; margin: 0; font: 500 1.4rem/1.25 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .pickbox {
    display: grid; place-items: center; width: 32px; height: 32px; padding: 0;
    background: none; border: 0; cursor: pointer; flex: none;
  }
  .box {
    display: grid; place-items: center; width: 18px; height: 18px;
    border: 1px solid var(--muted); border-radius: 4px; background: transparent;
  }
  .pickbox[aria-checked='true'] .box { background: var(--accent); border-color: var(--accent); }
  .box svg { width: 12px; height: 12px; color: var(--accent-ink); }
  h3 a { color: inherit; text-decoration-color: var(--line); text-underline-offset: 4px; }
  h3 a:hover { color: var(--accent); text-decoration-color: currentColor; }
  .age { margin: 0 0 10px; color: var(--muted); font-size: 0.75rem; }
  .source { color: var(--muted); font-size: 0.75rem; overflow-wrap: anywhere; }
  .source-link { display: inline-flex; align-items: center; gap: 5px; min-height: 24px; max-width: 100%; justify-self: start; text-decoration-color: var(--line); }
  .head h3 a { text-decoration: none; }
  .head h3 a:hover { text-decoration: underline; }
  .source-link:hover { color: var(--accent); text-decoration-color: currentColor; }
  .source-link code { min-width: 0; }
  .source-link svg { width: 12px; height: 12px; flex: none; }
  .disclosure { display: inline-flex; align-items: center; gap: 5px; min-height: 44px; padding: 6px 8px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--muted); font-size: 0.78rem; cursor: pointer; flex: none; }
  .disclosure:hover { border-color: var(--muted); color: var(--ink); }
  .disclosure svg { width: 14px; height: 14px; }
  .disclosure[aria-expanded='true'] svg { transform: rotate(180deg); }
  .core-data { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; margin-top: 10px; color: var(--muted); font-size: 0.75rem; font-variant-numeric: tabular-nums; }
  .stage-label { color: var(--ink); font-weight: 600; }
  .metric { display: inline-flex; align-items: center; gap: 4px; }
  .metric > svg { width: 14px; height: 14px; flex: none; }
  .card-details:not([hidden]) { margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--line); }
  .quick-actions { margin-top: 10px; }
  .environment-more { min-height: 32px; padding: 4px 8px; background: transparent; border: 1px solid var(--line); border-radius: 6px; color: var(--ink); cursor: pointer; font-size: 0.75rem; }
  .installation-badges { list-style: none; display: flex; flex-wrap: wrap; gap: 6px; padding: 0; margin: 10px 0 0; }
  .installation-badge { display: inline-flex; align-items: center; gap: 5px; max-width: 100%; min-width: 0; padding: 4px 7px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); color: var(--muted); font-size: 0.75rem; }
  .installation-badge .host { overflow-wrap: anywhere; min-width: 0; }
  .installation-badge strong { font-weight: 600; flex: none; }
  .status-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; flex: none; }
  .installation-badge.installed { color: var(--good); border-color: color-mix(in srgb, var(--good) 40%, var(--line)); background: color-mix(in srgb, var(--good) 5%, var(--card)); }
  .installation-badge.missing { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); background: color-mix(in srgb, var(--warn) 5%, var(--card)); }
  .collapsed { padding-top: 14px; padding-bottom: 14px; }
  .collapsed h3 { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .collapsed .source { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .collapsed .source-link code { overflow: hidden; text-overflow: ellipsis; }
  .change { font-variant-numeric: tabular-nums; color: var(--good); }
  .change.down { color: var(--bad); }
  .stale { font-style: italic; }
  code { overflow-wrap: anywhere; }
  .desc { margin: 6px 0 0; color: var(--muted); max-width: 70ch; font-size: 0.86rem; }
  .desc.clamp { display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .more {
    display: inline-flex; align-items: center; margin-top: 4px; padding: 2px 0; min-height: 32px;
    background: none; border: 0; color: var(--muted); font-size: 0.85rem; font-weight: 600; cursor: pointer;
  }
  .more:hover { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .tagrow { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; align-items: center; }
  .tag {
    display: inline-flex; align-items: center; gap: 4px; padding: 3px 9px; min-height: 28px;
    border: 1px solid var(--line); border-radius: 999px; background: transparent;
    color: var(--muted); font-size: 0.78rem; cursor: pointer;
  }
  .tag:hover { color: var(--ink); border-color: var(--muted); }
  .tag.add { border-style: dashed; border-color: var(--accent); color: var(--accent); }
  .tag.add[aria-expanded='true'] { background: var(--tint); color: var(--ink); }
  .tag.remove span { color: var(--bad); font-weight: 700; }
  .tag-edit { margin-top: 8px; padding: 12px; display: grid; gap: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .tag-help, .tag-error { margin: 0; font-size: 0.8rem; color: var(--muted); }
  .tag-error { color: var(--bad); }
  .tag:disabled { opacity: 0.55; cursor: wait; }
  .tag-edit form { display: flex; gap: 8px; }
  .tag-edit input { flex: 1; min-width: 0; }
  .envrow { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
  .envchip {
    display: inline-flex; align-items: center; gap: 6px; padding: 3px 9px; min-height: 28px;
    border: 1px solid var(--line); border-radius: 6px; background: color-mix(in srgb, var(--bg) 55%, var(--card));
    color: var(--muted); font-size: 0.75rem; cursor: pointer;
  }
  .envchip:hover { color: var(--ink); border-color: var(--muted); }
  .envchip.old { opacity: 0.5; }
  .envdot { width: 6px; height: 6px; border-radius: 999px; background: var(--accent); opacity: 0.75; flex: none; }
  .listrow { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
  .listchip {
    display: inline-flex; align-items: center; gap: 5px; padding: 3px 9px; min-height: 28px;
    border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--line)); border-radius: 6px;
    background: color-mix(in srgb, var(--tint) 28%, var(--card)); color: var(--accent);
    font-size: 0.75rem; font-weight: 600; cursor: pointer;
  }
  .listchip:hover { border-color: var(--accent); background: color-mix(in srgb, var(--tint) 55%, var(--card)); }
  .listchip svg { width: 13px; height: 13px; flex: none; }
  .history { margin-top: 10px; padding-top: 8px; border-top: 1px dashed var(--line); }
  .history p { margin: 0; color: var(--muted); font-size: 0.85rem; }
  .history ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; }
  .history li { display: flex; justify-content: space-between; gap: 10px; color: var(--muted); font-size: 0.82rem; }
  .history li span { min-width: 0; overflow-wrap: anywhere; }
  .history time { flex: none; font-variant-numeric: tabular-nums; }
  .notes { margin: 12px 0 0; padding: 4px 0 4px 14px; border-left: 2px solid var(--accent); font: italic 1rem/1.5 var(--serif); white-space: pre-wrap; }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 14px; align-items: center; }
  .chip { display: inline-flex; gap: 6px; align-items: center; border: 1px solid var(--line); background: transparent; border-radius: 999px; padding: 4px 14px; cursor: pointer; min-height: 36px; color: var(--ink); }
  .chip:hover { border-color: var(--muted); }
  .chip.move { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .chip.move:hover { background: color-mix(in srgb, var(--tint) 55%, var(--card)); }
  .chip.heart { color: var(--muted); padding: 4px 12px; }
  .chip.heart[aria-pressed='true'] { color: var(--heart); border-color: var(--heart); font-weight: 600; }
  .actions .btn.quiet { padding-left: 7px; padding-right: 7px; }
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
  .compact { padding: 16px 14px; }
  .compact .head { display: grid; gap: 6px; }
  .compact h3 { font-size: 1.2rem; }
  .compact .desc { font-size: 0.88rem; }
  .compact .actions { gap: 6px; }
  .compact .actions .chip { padding: 5px 10px; font-size: 0.82rem; }
  .compact .install-line { flex-direction: column; align-items: stretch; }
  .compact .tag-edit form { flex-direction: column; }
  .compact .edit div { flex-wrap: wrap; }
  .compact .edit.link input { width: 100%; flex-basis: 100%; }
  @media (min-width: 900px) {
    .skill.collapsed:not(.compact) { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: center; gap: 8px 20px; padding: 12px 16px; }
    .collapsed:not(.compact) .row { grid-column: 1; grid-row: 1 / 4; }
    .collapsed:not(.compact) .core-data { grid-column: 2; grid-row: 1; margin: 0; }
    .collapsed:not(.compact) .installation-badges { grid-column: 2; grid-row: 2; margin: 0; }
    .collapsed:not(.compact) .quick-actions { grid-column: 2; grid-row: 3; margin: 0; }
  }
  @media (max-width: 560px) {
    .skill { padding: 18px 16px; }
    .edit.link div { flex-direction: column; }
    .install-line { flex-direction: column; align-items: stretch; }
    .install-line :global(.copy) { align-self: flex-start; }
    .tag-edit form { flex-direction: column; align-items: stretch; }
  }
  @media (pointer: coarse) {
    .source-link { min-height: 44px; }
    .chip, .install-toggle, .seg button, .more, .tag, .envchip, .listchip { min-height: 44px; }
    .pickbox { width: 44px; height: 44px; }
    .box { width: 22px; height: 22px; }
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
