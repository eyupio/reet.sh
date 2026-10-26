<script lang="ts">
  import { onMount } from 'svelte'
  import {
    deleteSetupProfile, getSetupProfile, listSetupProfiles, renameSetupProfile, updateSetupProfileOverrides,
    type SetupProfileOverrides, type SetupProfileRecord, type SetupProfileRevision, type SetupProfileSummary,
  } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PageHeader from './PageHeader.svelte'
  import CopyButton from './CopyButton.svelte'
  import { setupContents, compareSetups, shellQuote } from './setupProfiles'

  let {
    canWrite = true,
    onerror,
  }: { canWrite?: boolean; onerror?: (e: unknown) => void } = $props()

  let profiles = $state<SetupProfileSummary[]>([])
  let selected = $state<SetupProfileRecord | null>(null)
  let loading = $state(true)
  let busy = $state('')
  let error = $state('')
  let deleteFor = $state<SetupProfileSummary | null>(null)
  let renaming = $state(false)
  let renameName = $state('')
  let editingOverrides = $state(false)
  let excludedKinds = $state<string[]>([])
  let excludedSources = $state('')
  const overrideKinds = ['settings', 'mcp', 'instructions', 'skills', 'memory']

  let selectedRevision = $state(0)
  let scopeMachine = $state(''), scopeProject = $state(''), creating = $state(false)
  let showAllRevisions = $state(false)
  const revisionLimit = 6
  const newest = $derived(selected?.revisions.at(-1) ?? null)
  const visibleRevisions = $derived(selected ? (showAllRevisions ? selected.revisions : selected.revisions.slice(-revisionLimit)) : [])
  const latest = $derived(selected?.revisions.find(revision => revision.revision === selectedRevision) ?? newest)
  const contents = $derived(setupContents(latest, scopeMachine, scopeProject))
  const previous = $derived(selected?.revisions.find(revision => revision.revision === (latest?.revision ?? 0) - 1))
  const changes = $derived(latest ? compareSetups(previous, latest, scopeMachine, scopeProject) : null)
  const profileArg = $derived(selected ? shellQuote(selected.name) : "'work'")
  const restoreCommand = $derived(`reet setup restore --profile ${profileArg} --revision ${latest?.revision ?? 1}${scopeMachine ? ` --machine ${shellQuote(scopeMachine)}` : ''}${scopeProject ? ` --project ${shellQuote(scopeProject)}` : ''}`)
  const verifyCommand = $derived(restoreCommand.replace(' setup restore ', ' setup verify '))
  const pullCommand = $derived(`reet setup profile pull ${profileArg} --server ${shellQuote(location.origin)}`)
  const fetchCommand = $derived(`reet setup fetch --s3 --profile ${profileArg} --revision ${latest?.revision ?? 1}`)
  const hasPreview = $derived((latest?.preview?.schema_version ?? 0) > 0)
  const componentCount = $derived.by(() => {
    if (!latest?.preview?.agents) return 0
    return latest.preview.agents.reduce((n, agent) => n + (agent.components?.length ?? 0), 0)
  })
  const omissionCount = $derived.by(() => {
    if (!latest?.preview?.agents) return 0
    return latest.preview.agents.reduce((n, agent) => n + (agent.omitted?.length ?? 0), 0)
  })
  const unavailable = $derived((latest?.preview?.classes ?? []).filter((item) => item.selection !== 'include'))
  const skillCount = $derived(latest?.preview?.skills?.length ?? 0)
  const memoryCount = $derived(latest?.preview?.memories?.length ?? 0)
  const storedAgents = $derived((latest?.preview?.agents ?? []).filter((agent) => (agent.components?.length ?? 0) > 0))
  const emptyAgents = $derived((latest?.preview?.agents ?? []).filter((agent) => (agent.components?.length ?? 0) === 0))

  onMount(() => {
    void load()
  })

  async function load() {
    loading = true
    try {
      const result = await listSetupProfiles()
      profiles = result.profiles ?? []
      error = ''
      if (selected && !profiles.some((profile) => profile.name === selected?.name)) selected = null
      if (!selected && profiles.length > 0) await choose(profiles[0].name)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      loading = false
    }
  }

  async function choose(name: string) {
    busy = name
    renaming = false
    editingOverrides = false
    showAllRevisions = false
    selectedRevision = 0; scopeMachine = ''; scopeProject = ''
    try {
      selected = await getSetupProfile(name)
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  function startRename() {
    if (!selected) return
    renameName = selected.name
    renaming = true
  }

  function startOverrides() {
    if (!newest) return
    const overrides = newest.overrides ?? {}
    excludedKinds = [...(overrides.exclude_kinds ?? [])]
    excludedSources = (overrides.exclude_sources ?? []).join('\n')
    editingOverrides = true
  }

  async function saveOverrides() {
    if (!selected || !latest) return
    const previous = newest?.overrides ?? {}
    const sources = excludedSources.split('\n').map((value) => value.trim()).filter(Boolean)
    const overrides: SetupProfileOverrides = { ...previous, exclude_kinds: excludedKinds, exclude_sources: sources }
    if (excludedKinds.length === 0) delete overrides.exclude_kinds
    if (sources.length === 0) delete overrides.exclude_sources
    busy = selected.name
    try {
      const updated = await updateSetupProfileOverrides(selected.name, overrides, newest?.revision)
      const result = await listSetupProfiles()
      profiles = result.profiles ?? []
      selected = updated
      selectedRevision = 0
      showAllRevisions = false
      editingOverrides = false
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function renameProfile() {
    if (!selected) return
    const oldName = selected.name
    const newName = renameName.trim()
    if (!newName || newName === oldName) {
      renaming = false
      return
    }
    busy = oldName
    try {
      const updated = await renameSetupProfile(oldName, newName, newest?.revision)
      const result = await listSetupProfiles()
      profiles = result.profiles ?? []
      selected = updated
      selectedRevision = 0
      showAllRevisions = false
      renaming = false
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  async function remove() {
    if (!deleteFor) return
    const name = deleteFor.name
    deleteFor = null
    busy = name
    try {
      await deleteSetupProfile(name)
      deleteFor = null
      selected = null
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      onerror?.(e)
    } finally {
      busy = ''
    }
  }

  function toggleRevisionHistory() {
    if (!selected) return
    showAllRevisions = !showAllRevisions
    if (!showAllRevisions && !selected.revisions.slice(-revisionLimit).some((revision) => revision.revision === selectedRevision)) selectedRevision = 0
  }
  function stamp(value: number): string {
    if (!value) return ''
    return new Date(value).toLocaleString()
  }

  function when(value: string): string {
    if (!value) return ''
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? '' : parsed.toLocaleString()
  }

  function kindLabel(kind: string): string {
    return ({ settings: 'Agent settings', mcp: 'MCP servers', instructions: 'Instructions', skills: 'Skills', credentials: 'Credential references', behavior: 'Rules and commands', memory: 'Memory', history: 'History', hooks: 'Hooks', caches: 'Caches' } as Record<string, string>)[kind] ?? kind
  }

  function revisionSummary(revision: SetupProfileRevision): string {
    const components = revision.preview?.agents?.reduce((n, agent) => n + (agent.components?.length ?? 0), 0) ?? 0
    const skills = revision.preview?.skills?.length ?? 0
    return skills > 0 ? `${components} components, ${skills} skills` : `${components} components`
  }
</script>

<PageHeader
  eyebrow="Agent setups"
  title="Setups."
  lede="Keep a portable record of your agent settings, skills and supported memory. Review coverage before restoring on another machine."
>
 {#snippet actions()}{#if canWrite}<button class="btn primary" onclick={() => creating = !creating} aria-expanded={creating}>Create a setup</button>{/if}{/snippet}
</PageHeader>
{#if creating}
 <section class="empty-state"><h2>Create a setup locally</h2><p>Run the wizard on the machine you want to capture. It creates an encrypted snapshot, saves a named profile and offers to sync its metadata here.</p><pre><code>reet setup profile</code></pre><CopyButton text="reet setup profile" label="Copy wizard command" /><p>The snapshot stays local unless you explicitly upload it to S3. Keep its passphrase somewhere you can recover independently.</p><a class="btn quiet" href="/machines#connect-machine">Install or connect the CLI →</a></section>
{/if}

{#if error}<p class="notice bad" role="alert">{error} <button class="btn quiet" onclick={() => selected ? choose(selected.name) : load()} disabled={busy !== ''}>Reload setup</button></p>{/if}

{#if loading}
  <p class="empty" role="status">Loading setups…</p>
{:else if profiles.length === 0}
  <section class="empty-state">
    <h2>No setups yet.</h2>
    <p>Run the interactive wizard; it can create or choose a snapshot, save a profile and push it here:</p>
    <pre><code>reet setup profile</code></pre>
    <p>Or save and push in steps:</p>
    <pre><code>reet setup profile save work
reet setup profile push work</code></pre>
    <details class="stored"><summary>How setups are stored</summary><p>The synced profile is metadata, not the encrypted snapshot. On another machine, pull the profile first, then copy the snapshot or fetch it from your configured S3 destination.</p></details><a class="btn quiet" href="/machines#connect-machine">Install or connect the CLI →</a>
  </section>
{:else}
  <div class="profiles">
    <section class="profile-list" aria-label="Setup profiles">
      {#each profiles as profile (profile.name)}
        <button
          type="button"
          class="profile-row"
          class:active={selected?.name === profile.name}
          aria-pressed={selected?.name === profile.name}
          onclick={() => choose(profile.name)}
          disabled={busy !== ''}
        >
          <span class="name">{profile.name}</span>
          <span class="revision">revision {profile.revision}</span>

          <span class="updated">{stamp(profile.updatedAt)}</span>
        </button>
      {/each}
    </section>

    <section class="profile-detail" aria-live="polite">
      {#if !selected}
        <p class="empty">Choose a profile to inspect it.</p>
      {:else if latest}
        <header class="detail-head">
          <div>
            <p class="eyebrow">Setup profile</p>
            <h2>{selected.name}</h2>
            <p class="muted">{latest.revision === newest?.revision ? 'Latest revision' : 'Viewing revision'} {latest.revision} of {selected.revisions.length}</p>
          </div>
          {#if canWrite}
            <div class="detail-actions">
              <button type="button" class="btn quiet" onclick={startOverrides} disabled={busy !== '' || latest.revision !== newest?.revision}>Edit exclusions</button>
              <button type="button" class="btn quiet" onclick={startRename} disabled={busy !== ''}>Rename</button>
              <button type="button" class="btn quiet danger" onclick={() => (deleteFor = profiles.find((profile) => profile.name === selected?.name) ?? null)} disabled={busy !== ''}>Delete</button>
            </div>
          {/if}
        </header>

        {#if editingOverrides}
          <form class="rename-form" onsubmit={(event) => { event.preventDefault(); void saveOverrides() }}>
            <span class="rename-label">New server revision exclusions</span>
            <div class="override-kinds">
              {#each overrideKinds as kind}
                <label><input type="checkbox" value={kind} bind:group={excludedKinds} /> {kindLabel(kind)}</label>
              {/each}
            </div>
            <label for="profile-exclude-sources">Excluded source paths, one per line</label>
            <textarea id="profile-exclude-sources" bind:value={excludedSources} rows="3" placeholder="/path/to/source"></textarea>
            <div class="rename-row">
              <button type="submit" class="btn" disabled={busy !== ''}>Save revision</button>
              <button type="button" class="btn quiet" onclick={() => (editingOverrides = false)} disabled={busy !== ''}>Cancel</button>
            </div>
            <p class="muted small">Adds a server revision with the same snapshot. Machine/project scopes are preserved; local profiles change only after a pull.</p>
          </form>
        {/if}

        {#if renaming}
          <form class="rename-form" onsubmit={(event) => { event.preventDefault(); void renameProfile() }}>
            <label for="profile-rename">New server copy name</label>
            <div class="rename-row">
              <input id="profile-rename" bind:value={renameName} maxlength="64" pattern="[A-Za-z0-9_-]+" required autocomplete="off" />
              <button type="submit" class="btn" disabled={busy !== '' || !renameName.trim() || renameName.trim() === selected.name}>Save</button>
              <button type="button" class="btn quiet" onclick={() => (renaming = false)} disabled={busy !== ''}>Cancel</button>
            </div>
            <p class="muted small">Renames the server copy only. Your local profile keeps its name until you run <code>reet setup profile rename</code>.</p>
          </form>
        {/if}

        <section class="coverage" aria-label="Restore coverage">
          <p class="eyebrow">Restore coverage</p>
          {#if hasPreview}
            <p class="coverage-count">{contents.count.components} components · {contents.count.skills} skills · {contents.count.memories} memory databases</p>
            <p class="muted small">Selected after exclusions for the scope below; the local plan determines which files can be mapped. Captured: {componentCount} components, {skillCount} skills and {memoryCount} memory databases. {contents.excluded} excluded.</p>
          {:else}<p>Coverage not recorded</p>{/if}
          <div class="scope-controls">
            <label>Machine scope<select bind:value={scopeMachine}><option value="">Global exclusions only</option>{#each Object.keys(latest.overrides?.machines ?? {}) as machine}<option value={machine}>{machine}</option>{/each}</select></label>
            <label>Destination project<input bind:value={scopeProject} list="setup-project-scopes" placeholder="/absolute/path (optional)" autocomplete="off" /></label><datalist id="setup-project-scopes">{#each Object.keys(latest.overrides?.projects ?? {}) as project}<option value={project}></option>{/each}</datalist>
          </div>
          <p class="muted small">Snapshot location and destination health are not checked by this page. File restore is supported for selected components; cross-OS recovery and runtime behaviour remain unverified.</p>
        </section>

        <details class="prepare">
          <summary>Prepare this machine</summary>
          <ol>
            <li><strong>Install and authenticate.</strong> <a href="/machines#connect-machine">Get the CLI</a>, then check <code>reet setup support</code>. Use a personal token from this site when prompted.</li>
            <li><strong>Pull the profile.</strong><pre>{pullCommand}</pre><CopyButton text={pullCommand} label="Copy pull command" /><p>A divergent local copy is preserved. Rename it locally before pulling if you need to keep both histories.</p></li>
            <li><strong>Obtain the encrypted snapshot.</strong> Copy it securely from the source machine, or use your configured S3 destination:<pre>{fetchCommand}</pre><CopyButton text={fetchCommand} label="Copy fetch command" /><p>Syncing a profile does not upload the snapshot. S3 must already be configured on this machine.</p></li>
            <li><strong>Stop destination agents and preview.</strong><pre>{restoreCommand}</pre><CopyButton text={restoreCommand} label="Copy restore preview" /><p>Enter the snapshot passphrase locally. Review paths, omissions and compatibility warnings. Without <code>--project</code>, project files are omitted.</p></li>
            <li><strong>Apply and verify locally.</strong> Re-run the reviewed command with <code>--apply</code>. Conflicts require explicit per-path skip/replace choices. Then verify the selected files:<pre>{verifyCommand}</pre><CopyButton text={verifyCommand} label="Copy verification command" /><p>Keep the journal for rollback; a file match does not establish working agents.</p></li>
          </ol>
        </details>

        <details class="technical"><summary>Snapshot and capture details</summary>
        <dl class="facts">
          <div><dt>Snapshot ID</dt><dd><code>{latest.snapshot_id}</code></dd></div>
          <div><dt>Snapshot captured</dt><dd>{when(latest.snapshot_taken_at)}</dd></div>
          <div><dt>Revision recorded</dt><dd>{when(latest.created_at)}</dd></div>
          <div><dt>Source platform</dt><dd>{latest.preview?.os || 'Not recorded'} / {latest.preview?.architecture || 'Not recorded'}</dd></div>
          <div><dt>Capture omissions</dt><dd>{hasPreview ? `${omissionCount} records` : 'Not recorded'}</dd></div>
          <div><dt>Software evidence</dt><dd>{latest.software?.length ? `${latest.software.length} agents` : 'None recorded'}</dd></div>
        </dl></details>

        {#if !hasPreview}
          <p class="notice">Preview not recorded. Run <code>reet setup profile</code> or <code>reet setup profile push</code> and open the snapshot, or save with <code>--password-file</code>, to include selected/omitted counts.</p>
        {/if}
        {#if latest.preview?.credentials_included}
          <p class="notice good">Credential references and any literal credentials are inside the encrypted snapshot. Values are never shown here.</p>
        {/if}

        {#if hasPreview}
          <details class="stored">
            <summary class="stored-summary">
              <span>Captured contents before exclusions</span>
              <span class="muted small">{componentCount} components · {skillCount} skills</span>
            </summary>
            <div class="stored-body">
            {#if storedAgents.length === 0}
              <p class="muted small">No components were selected in this revision.</p>
            {/if}
            {#each storedAgents as agent}
              <article class="agent-card">
                <header class="agent-head">
                  <strong>{agent.name}</strong>
                  <span class="tag">{agent.state ?? 'unknown'}</span>
                </header>
                {#each agent.components ?? [] as component}
                  <div class="component">
                    <div class="component-head">
                      <strong>{kindLabel(component.kind)}</strong>
                      <span class="tag">{component.scope || 'global'}</span>
                    </div>
                    <code>{component.source}</code>
                    {#if component.kind === 'mcp'}
                      {#if component.declaration_names?.length}
                        <div class="chips">
                          {#each component.declaration_names as name}
                            <span class="chip mono">{name}</span>
                          {/each}
                        </div>
                      {:else if !component.declarations}
                        <p class="muted small">No MCP declarations selected.</p>
                      {:else}
                        <p class="muted small">{(component.selected_declarations ?? 0)} of {component.declarations} declarations selected; server names were not recorded in this revision.</p>
                      {/if}
                    {/if}
                    {#if component.included_fields?.length}
                      <p class="fieldline"><span class="tag">fields</span> {component.included_fields.join(', ')}</p>
                    {/if}
                    {#if component.encrypted_fields?.length}
                      <p class="fieldline"><span class="tag warn">encrypted values</span> {component.encrypted_fields.join(', ')} <span class="muted">(names only; values are not shown)</span></p>
                    {/if}
                    {#if component.reference_fields?.length}
                      <p class="fieldline"><span class="tag">references only</span> {component.reference_fields.join(', ')}</p>
                    {/if}
                    {#if component.excluded_fields?.length}
                      <p class="fieldline muted small">Not copied: {component.excluded_fields.join(', ')}</p>
                    {/if}
                    {#if component.unknown_fields}
                      <p class="fieldline muted small">{component.unknown_fields} unrecognised field{component.unknown_fields === 1 ? '' : 's'} not copied.</p>
                    {/if}
                    {#if component.kind === 'mcp' && component.declarations}
                      <p class="fieldline muted small">{component.selected_declarations ?? 0} of {component.declarations} declarations selected.</p>
                    {/if}
                  </div>
                {/each}
                {#if agent.omitted?.length}
                  <details class="omitted">
                    <summary>{agent.omitted.length} omitted record{agent.omitted.length === 1 ? '' : 's'}</summary>
                    <ul>
                      {#each agent.omitted as item}
                        <li><strong>{kindLabel(item.kind)}</strong>: {item.reason}{item.count ? ` (${item.count})` : ''}</li>
                      {/each}
                    </ul>
                  </details>
                {/if}
              </article>
            {/each}
            {#if emptyAgents.length}
              <p class="muted small">No selected components for {emptyAgents.map((agent) => agent.name).join(', ')}.</p>
            {/if}
            {#if latest.preview?.memories?.length}
              <div class="skills">
                <h3>Memory databases</h3>
                <ul>
                  {#each latest.preview.memories as memory}
                    <li>
                      <strong>{memory.name}</strong>
                      <span class="tag">{memory.agent_id} · {memory.scope}</span>
                      <span class="muted small">{memory.bytes} bytes</span>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
            <div class="skills">
              <h3>Skills</h3>
              {#if latest.preview?.skills?.length}
                <ul>
                  {#each latest.preview.skills as skill}
                    <li>
                      <strong>{skill.name}</strong>
                      <span class="tag">{skill.agent_id} · {skill.scope}</span>
                      {#if skill.origin}<span class="tag">{skill.origin}</span>{/if}
                      <span class="muted small">{skill.files} file{skill.files === 1 ? '' : 's'}, {skill.bytes} bytes</span>
                    </li>
                  {/each}
                </ul>
              {:else}
                <p class="muted small">No installed skill folders were recorded in this revision.</p>
              {/if}
            </div>
            {#if unavailable.length}
              <div class="not-stored">
                <h3>Not stored here</h3>
                <ul>
                  {#each unavailable as item}
                    <li><strong>{kindLabel(item.kind)}.</strong> {item.reason}</li>
                  {/each}
                </ul>
              </div>
            {/if}
            <p class="notice small">Skill and memory contents stay inside the encrypted snapshot; this page shows their names, sizes and digests only. Credential values are never displayed; encrypted fields are listed by name only.</p>
            </div>
          </details>
        {/if}

        {#if latest.software?.length}
          <div class="software">
            <h3>Software evidence</h3>
            <ul>
              {#each latest.software as item}
                <li><strong>{item.name}</strong>{#if item.application_version} <span class="muted">{item.application_version}</span>{/if}{#if item.installation_package} <span class="muted">({item.installation_package})</span>{/if}</li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if latest.overrides && ((latest.overrides.exclude_kinds?.length ?? 0) > 0 || (latest.overrides.exclude_sources?.length ?? 0) > 0 || Object.keys(latest.overrides.machines ?? {}).length > 0 || Object.keys(latest.overrides.projects ?? {}).length > 0)}
          <div class="overrides">
            <h3>Overrides</h3>
            {#if latest.overrides.exclude_kinds?.length}
              <p><span class="tag">excluded kinds</span> {latest.overrides.exclude_kinds.join(', ')}</p>
            {/if}
            {#if latest.overrides.exclude_sources?.length}
              <p><span class="tag">excluded sources</span> {latest.overrides.exclude_sources.join(', ')}</p>
            {/if}
            {#each Object.entries(latest.overrides.machines ?? {}) as [machine, scope] (machine)}
              <p><span class="tag">machine {machine}</span> kinds {scope.exclude_kinds?.join(', ') || '—'}; sources {scope.exclude_sources?.join(', ') || '—'}</p>
            {/each}
            {#each Object.entries(latest.overrides.projects ?? {}) as [project, scope] (project)}
              <p><span class="tag">project {project}</span> kinds {scope.exclude_kinds?.join(', ') || '—'}; sources {scope.exclude_sources?.join(', ') || '—'}</p>
            {/each}
          </div>
        {/if}

        <div class="history">
          <div class="history-head"><h3>Revision history</h3>{#if selected.revisions.length > revisionLimit}<span class="muted small">{showAllRevisions ? `All ${selected.revisions.length} revisions` : `Latest ${revisionLimit} of ${selected.revisions.length}`}</span>{/if}</div>
          <ol>
            {#each visibleRevisions as revision (revision.revision)}
              <li class:current={revision.revision === latest.revision}>
                <button type="button" class="btn quiet rev" aria-pressed={revision.revision === latest.revision} onclick={() => { selectedRevision = revision.revision; editingOverrides = false; renaming = false; scopeMachine = ''; scopeProject = '' }}>View r{revision.revision}</button>
                <span>{when(revision.created_at)}</span>
                <span class="muted">{revisionSummary(revision)}</span>
              </li>
            {/each}
          </ol>
          {#if selected.revisions.length > revisionLimit}
            <button type="button" class="btn quiet" onclick={toggleRevisionHistory}>{showAllRevisions ? 'Show latest only' : `Show all ${selected.revisions.length} revisions`}</button>
          {/if}
        </div>

        {#if previous && changes}
          <details class="revision-diff"><summary>Changes from r{previous.revision} to r{latest.revision}</summary>
            <p class="muted small">{changes.added.length} added · {changes.removed.length} removed · {changes.changed.length} changed in the selected scope. Exclusions {changes.exclusionsChanged ? 'changed' : 'unchanged'}.</p>
            {#each [['Added', changes.added], ['Removed', changes.removed], ['Changed', changes.changed]] as [label, items]}
              {#if Array.isArray(items) && items.length}<h3>{label}</h3><ul>{#each items as item}<li class="muted small">{item}</li>{/each}</ul>{/if}
            {/each}
            <p class="muted small">Metadata comparison only; encrypted values and file contents are not available here.</p>
          </details>
        {/if}
      {:else}
        <p class="empty">This profile has no revisions.</p>
      {/if}
    </section>
  </div>
{/if}

{#if deleteFor}
  <ConfirmDialog
    title={`Delete ${deleteFor.name}?`}
    body="This removes the server copy of the profile. Local snapshots and local profiles are not deleted."
    confirmLabel="Delete profile"
    danger
    onconfirm={remove}
    oncancel={() => (deleteFor = null)}
  />
{/if}

<style>
  .coverage {margin:20px 0;padding:16px;border:1px solid var(--line);border-radius:10px;background:var(--tint)}
  .coverage-count {font-size:1.05rem;font-weight:600;margin:8px 0}
  .scope-controls {display:flex;gap:12px;flex-wrap:wrap;margin:14px 0}.scope-controls label {display:grid;gap:6px;font-size:.8rem;min-width:0;flex:1}.scope-controls select,.scope-controls input {max-width:100%;min-width:0}
  .prepare,.technical,.revision-diff {border-top:1px solid var(--line);padding:14px 0;margin-top:12px} .prepare summary,.technical summary,.revision-diff summary {cursor:pointer;font-weight:600;font-size:.9rem}.prepare ol {padding-left:22px}.prepare li {margin:16px 0;font-size:.85rem}.prepare p {color:var(--muted)}
  .profiles { display: grid; grid-template-columns: minmax(200px, 0.55fr) minmax(0, 1.6fr); gap: 18px; align-items: start; }
  .profile-list { display: grid; gap: 8px; }
  .profile-row {
    display: grid; grid-template-columns: 1fr auto; gap: 2px 12px; text-align: left; padding: 12px 14px;
    border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--ink); cursor: pointer;
  }
  .profile-row:hover { border-color: var(--muted); }
  .profile-row.active { border-color: var(--accent); background: var(--tint); }
  .profile-row:disabled { opacity: 0.65; cursor: default; }
  .name { font-weight: 650; overflow-wrap: anywhere; }
  .revision { color: var(--accent-text); font-size: 0.76rem; font-weight: 600; }
  .updated { grid-column: 1 / -1; color: var(--muted); font-size: 0.75rem; overflow-wrap: anywhere; }
  .profile-detail { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  .detail-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
  .detail-actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
  .rename-form { margin-top: 14px; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .rename-form label { display: block; font-size: 0.75rem; color: var(--muted); margin-bottom: 6px; }
  .rename-label { display: block; font-size: 0.75rem; color: var(--muted); margin-bottom: 6px; }
  .rename-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .rename-row input, .rename-form textarea { flex: 1 1 180px; min-width: 0; }
  .rename-form p { margin: 8px 0 0; }
  .rename-form textarea { width: 100%; box-sizing: border-box; margin: 4px 0 8px; font: inherit; }
  .override-kinds { display: flex; flex-wrap: wrap; gap: 8px 14px; margin: 4px 0 10px; font-size: 0.8rem; }
  .override-kinds label { display: flex; align-items: center; gap: 4px; margin: 0; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 4px 0 6px; font: 500 1.7rem/1.1 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  h3 { margin: 22px 0 8px; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
  .muted { color: var(--muted); overflow-wrap: anywhere; }
  .small { font-size: 0.8rem; }
  .facts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px 16px; margin: 18px 0 0; }
  .facts div { min-width: 0; }
  dt { color: var(--muted); font-size: 0.72rem; margin-bottom: 2px; }
  dd { margin: 0; overflow-wrap: anywhere; font-size: 0.84rem; }
  code { font-size: 0.8em; }
  .notice { margin-top: 16px; }
  .overrides p { margin: 6px 0; font-size: 0.82rem; overflow-wrap: anywhere; }
  .history-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
  .history-head h3 { margin-bottom: 8px; }
  .stored { margin-top: 18px; }
  .stored-summary { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; cursor: pointer; padding: 8px 0; color: var(--ink); font-size: 0.8rem; font-weight: 650; text-transform: uppercase; letter-spacing: 0.08em; }
  .stored-summary::before { content: ""; flex: 0 0 auto; width: 0; height: 0; border-left: 5px solid currentColor; border-top: 4px solid transparent; border-bottom: 4px solid transparent; transition: transform 150ms ease; }
  .stored[open] .stored-summary::before { transform: rotate(90deg); }
  .stored-summary .muted { text-transform: none; letter-spacing: 0; font-weight: 400; }
  .stored-body { margin-top: 4px; }
  .agent-card { border: 1px solid var(--line); border-radius: 10px; padding: 12px; margin-top: 10px; }
  .agent-head, .component-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
  .component { border-top: 1px solid var(--line); margin-top: 10px; padding-top: 10px; min-width: 0; }
  .component code { display: block; margin-top: 4px; overflow-wrap: anywhere; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
  .chip { border: 1px solid var(--line); border-radius: 999px; padding: 2px 8px; background: var(--bg); font-size: 0.75rem; overflow-wrap: anywhere; }
  .mono { font-family: monospace; }
  .fieldline { margin: 6px 0 0; font-size: 0.78rem; overflow-wrap: anywhere; }
  .tag.warn { color: var(--muted); border-color: var(--line); }
  .omitted { margin-top: 10px; font-size: 0.8rem; }
  .omitted summary { cursor: pointer; color: var(--muted); }
  .omitted ul, .not-stored ul, .software ul { margin: 6px 0 0; padding-left: 18px; font-size: 0.8rem; }
  .omitted li, .not-stored li, .software li { margin: 4px 0; overflow-wrap: anywhere; }
  .skills { margin-top: 16px; }
  .skills h3 { margin-bottom: 4px; }
  .skills ul { margin: 6px 0 0; padding-left: 18px; font-size: 0.8rem; }
  .skills li { margin: 4px 0; overflow-wrap: anywhere; }
  .not-stored { margin-top: 16px; }
  .not-stored h3, .software h3 { margin-bottom: 4px; }
  .software { margin-top: 16px; }
  .history ol { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .history li { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 10px; align-items: baseline; padding: 6px 8px; border-radius: 6px; font-size: 0.8rem; }
  .history li > .muted {grid-column:2}.history button {font-size:.8rem}.history li.current { background: var(--tint); }
  .rev { color: var(--accent-text); font-weight: 650; }
  .empty-state { border: 1px dashed var(--line); border-radius: 12px; padding: 24px; }
  .empty-state h2 { margin-top: 0; }
  .empty-state p { color: var(--muted); max-width: 60ch; }
  .stored { margin: 6px 0 12px; }
  .stored summary { display: inline-flex; align-items: center; min-height: 32px; color: var(--muted); font-weight: 600; cursor: pointer; }
  .stored summary:hover { color: var(--ink); }
  pre { overflow: auto; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  @media (max-width: 900px) {
    .profiles { grid-template-columns: 1fr; }
    .facts { grid-template-columns: 1fr; }
  }
  @media (max-width: 560px) {
    .profile-detail { padding: 16px; }
    .detail-head { flex-direction: column; }
  }
</style>
