<script lang="ts">
  import { onMount } from 'svelte'
  import { syncCandidates, syncLibraryMachines, controlSyncAgent, getImports, type ImportRequest, type SyncDuplicate, type SyncState, type SyncSkill } from './api'
  import MachineSkillSync from './MachineSkillSync.svelte'
  import { agentOnline } from './machines'
  import { copiesSignature, copyingLine, duplicatesNote, leftOutHeadline, libraryNotice } from './librarySync'
  import { fleetSyncStatus } from './fleetSync'
  import { machineSyncOverview } from './syncOverview'
  import Skeleton from './Skeleton.svelte'

  let { snapshot, canWrite, onupdate }: { snapshot: SyncState; canWrite: boolean; onupdate: (state: SyncState) => void } = $props()
  let skills = $state<SyncSkill[]>([]), duplicates = $state<SyncDuplicate[]>([])
  let loading = $state(true), busy = $state(false), error = $state(''), query = $state('')
  let imports = $state<ImportRequest[]>([]), importError = $state(''), copying = $state<ImportRequest | null>(null)
  const agents = $derived(snapshot.agents ?? [])
  // One sentence for the whole fleet, and whether pressing the button is the right next step.
  const status = $derived(fleetSyncStatus(snapshot))
  const showButton = $derived(canWrite && (status.canSync || status.state === 'syncing'))
  const filtered = $derived(skills.filter(skill => `${skill.name} ${skill.source}`.toLowerCase().includes(query.toLowerCase())))
  const library = $derived(libraryNotice(skills, imports, snapshot.runs ?? []))
  const privateSkills = $derived(skills.filter(skill => skill.sourceType === 'private').length)

  async function readLibrary() {
    loading = true
    try { const read = await syncCandidates(0, 'library'); skills = read.skills; duplicates = read.duplicates ?? [] }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { loading = false }
  }
  // Reads the library again without the loading state, so what is on screen stays put until the answer arrives.
  // A failed read keeps what is shown; the next change tries again.
  async function refreshLibrary() {
    try { const read = await syncCandidates(0, 'library'); skills = read.skills; duplicates = read.duplicates ?? [] }
    catch { /* keep what is shown */ }
  }
  async function sync() {
    if (busy || !canWrite || !status.canSync) return
    busy = true; error = ''
    try {
      // The headline answers from the state this returns, so there is no success message to read: it flips to
      // "Syncing your machines…" and then to "Everything is in sync."
      onupdate(await syncLibraryMachines())
      await refreshLibrary()
    } catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
  async function automatic(id: string, enabled: boolean) {
    busy = true; error = ''
    try { onupdate(await controlSyncAgent(id, enabled ? 'keep-sync-on' : 'keep-sync-off')) }
    catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { busy = false }
  }
  onMount(() => {
    void readLibrary()
    let alive = true, reading = false
    // null until the first read, which the mount already covers.
    let copies: string | null = null
    const refresh = async () => {
      if (!canWrite || reading || document.hidden) return
      reading = true
      try {
        const data = await getImports()
        if (alive) {
          imports = data.requests; importError = ''
          // A copy that finishes or fails changes what the library says about its skill, and the library list is
          // not polled. Read it again when any copy changes state, so a failure shows up now and not after a reload.
          const now = copiesSignature(data.requests)
          if (copies !== null && now !== copies) void refreshLibrary()
          copies = now
        }
      }
      catch (e) { if (alive) importError = e instanceof Error ? e.message : String(e) }
      finally { reading = false }
    }
    void refresh()
    const timer = setInterval(() => void refresh(), 5000)
    return () => { alive = false; clearInterval(timer) }
  })
</script>

<section class="library-sync" aria-labelledby="library-sync-title" aria-busy={busy}>
  <header>
    <div><p class="eyebrow">One library. Every machine.</p><h2 id="library-sync-title">Your skills, everywhere.</h2></div>
    {#if canWrite}<a class="btn" class:primary={!agents.length} href="/machines#add-machine">Add a machine</a>{/if}
  </header>
  {#if agents.length}
  <!-- The page answers one question, "am I in sync?", and offers one button. Everything else is detail below. -->
  <div class="fleet" data-state={status.state} role="status" aria-live="polite" aria-busy={busy || status.state === 'syncing'}>
    <span class="fleet-dot" aria-hidden="true"></span>
    <div class="fleet-text">
      <strong>{status.headline}</strong>
      {#if status.detail}<span>{status.detail}</span>{/if}
      <small>{loading ? 'Reading your library…' : `${skills.length} ${skills.length === 1 ? 'skill' : 'skills'} · ${agents.length} ${agents.length === 1 ? 'machine' : 'machines'}`}</small>
    </div>
    {#if showButton}<button type="button" class="btn" class:primary={status.state !== 'in-sync' && status.state !== 'waiting'} disabled={busy || loading || !status.canSync} onclick={sync}>{busy ? 'Starting…' : status.state === 'syncing' ? 'Syncing…' : 'Sync all machines'}</button>{/if}
  </div>
  <p class="promise">After this, new skills and edits to your own skills follow by themselves, even when this page is closed.</p>
  {/if}
  {#if error}<p class="message error" role="alert">{error}<button type="button" class="btn" disabled={busy} onclick={() => { error = ''; void readLibrary() }}>Reload library</button></p>{/if}
  {#if importError}<p class="message error" role="status">Private copy progress could not be read: {importError}</p>{/if}
  {#each library.review as item (item.id)}
    <p class="message" role="status">{item.name}: Some files need your review before they can be copied.<button class="btn" onclick={() => copying = item}>Review files</button></p>
  {/each}
  {#if library.copying.length}<p class="message calm" role="status">{copyingLine(library.copying)}</p>{/if}
  {#if library.leftOutCount}
    <!-- A skill that cannot sync is a note, not an alarm: everything else still syncs, and Sync all machines tries these again. -->
    <div class="left-out" role="status">
      <details open={library.leftOut.length <= 2}>
        <summary>{leftOutHeadline(library.leftOutCount)}</summary>
        <ul>{#each library.leftOut as group (group.key)}<li><strong>{group.names.join(', ')}</strong>{group.reason}</li>{/each}</ul>
        <p>Sync all machines tries them again.</p>
      </details>
    </div>
  {/if}
  <div class="machine-list" aria-label="Machine sync status">
    {#each agents as agent (agent.id)}
      {@const machine = snapshot.machines?.find(item => item.host.toLowerCase() === agent.host.toLowerCase())}
      {@const overview = machine ? machineSyncOverview(machine, snapshot) : undefined}
      <article>
        <span class="machine-dot" class:online={agentOnline(agent)} aria-hidden="true"></span>
        <div class="machine-name"><strong>{agent.host}</strong><span>{agent.paused ? 'Paused · resume in machine settings' : !agent.keepSynced ? 'Automatic sync is off · Sync all machines to enable' : !agentOnline(agent) ? 'Offline · library sync resumes when this machine reconnects' : !machine ? 'Connected · waiting for the first scan' : overview?.detail}</span></div>
        <span class="status" data-tone={agent.paused || !agent.keepSynced ? 'attention' : overview?.tone ?? 'working'}>{agent.paused ? 'Paused' : !agent.keepSynced ? 'Sync off' : !agentOnline(agent) ? 'Waiting for machine' : overview?.label ?? 'Setting up'}</span>
        <a class="machine-link" href={`/machines?machine=${encodeURIComponent(agent.host)}`} aria-label={`Open ${agent.host}`}>Manage</a>
      </article>
    {:else}<div class="empty"><strong>Start with your first machine.</strong><p>Connect it once and Reet will bring your skills with you.</p></div>{/each}
  </div>
  <details class="library-content">
    <summary>In your library{#if !loading} · {skills.length} skills{#if privateSkills} · {privateSkills} private{/if}{/if}</summary>
    {#if loading}<Skeleton label="Loading your library…" shape="rows" count={3} />
    {:else if !skills.length}<p>Your library is ready for its first skill. <a href="/skills">Add a skill</a> or <a href="/hosting">write your own</a>.</p>
    {:else}
      <label class="search">Find a skill<input type="search" bind:value={query} placeholder="Search your library" /></label>
      <ul>{#each filtered as skill (skill.id)}<li><strong>{skill.name}</strong><span>{skill.problem ? 'Left out for now' : skill.sourceType === 'private' ? 'Private · only your machines' : skill.sourceType === 'local' || !skill.source ? 'Copying privately from its machine' : skill.source}</span></li>{:else}<li>No skills match your search.</li>{/each}</ul>
      {#if duplicates.length}<p class="footnote">{duplicatesNote(duplicates)}</p>{/if}
    {/if}
  </details>
  {#if agents.length}<details class="library-content"><summary>Automatic sync settings</summary><p class="footnote">Turn automatic sync off to manage a machine manually. Sync all machines restores the shared library for every machine.</p>
    {#each agents as agent (agent.id)}<label class="automatic"><span>{agent.host}</span><input type="checkbox" role="switch" aria-label={`Keep ${agent.host} synced`} checked={!!agent.keepSynced} disabled={busy || !canWrite} onchange={async event => { const input = event.currentTarget; await automatic(agent.id, input.checked); input.checked = !!snapshot.agents?.find(item => item.id === agent.id)?.keepSynced }} /></label>{/each}
  </details>{/if}
  <p class="footnote">Reet checks each change and backs up replaced copies. Other local skills are kept. Project folders and removal rules are available in Advanced sync.</p>
</section>
{#if copying}
  {@const destination = snapshot.environments.find(env => env.id === copying?.destinationId)?.host ?? copying.host}
  <MachineSkillSync skill={{ id: copying.syncSkillId!, slug: copying.name, name: copying.name }} {destination} initialRequestId={copying.id} onclose={() => { copying = null; void readLibrary() }} />
{/if}

<style>
  .library-sync { border: 1px solid var(--line); border-radius: 16px; background: var(--card); padding: clamp(18px, 3vw, 30px); margin: 20px 0; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
  header > div { min-width: 0; }
  .eyebrow { margin: 0 0 8px; color: var(--accent-text); font-size: 0.875rem; text-transform: uppercase; letter-spacing: .1em; }
  h2 { margin: 0; font: 500 clamp(1.7rem, 3vw, 2.25rem)/1.15 var(--serif); }
  .promise { max-width: 65ch; color: var(--muted); font-size: .9rem; line-height: 1.65; }
  header .btn { flex: none; }
  .fleet { display: flex; align-items: center; gap: 16px; padding: 20px 22px; margin-top: 20px; background: var(--bg); border: 1px solid var(--line); border-radius: 12px; }
  .fleet-dot { flex: none; width: 12px; height: 12px; border-radius: 50%; background: var(--muted); }
  .fleet[data-state='in-sync'] .fleet-dot { background: var(--good); }
  .fleet[data-state='behind'] .fleet-dot { background: var(--accent-text); }
  .fleet[data-state='attention'] .fleet-dot { background: var(--warn); }
  .fleet[data-state='syncing'] .fleet-dot { background: var(--accent-text); animation: fleet-pulse 1.4s ease-in-out infinite; }
  @keyframes fleet-pulse { 50% { opacity: .35; } }
  @media (prefers-reduced-motion: reduce) { .fleet[data-state='syncing'] .fleet-dot { animation: none; } }
  .fleet-text { flex: 1; min-width: 0; display: grid; gap: 4px; overflow-wrap: anywhere; }
  .fleet-text strong { font: 500 clamp(1.25rem, 2.6vw, 1.6rem)/1.25 var(--serif); }
  .fleet-text span { font-size: 0.875rem; line-height: 1.5; }
  .fleet-text small { color: var(--muted); font-size: 0.875rem; }
  .fleet .btn { flex: none; min-height: 44px; }
  .promise { margin: 14px 0 22px; }
  .machine-list { border-top: 1px solid var(--line); }
  article { display: flex; align-items: center; gap: 14px; padding: 18px 0; border-bottom: 1px solid var(--line); }
  .machine-dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--muted); }
  .machine-dot.online { background: var(--good); }
  .machine-name { flex: 1; min-width: 0; display: grid; gap: 5px; overflow-wrap: anywhere; }
  .machine-name strong { font-size: .92rem; }
  .machine-name span, .machine-link { font-size: 0.875rem; color: var(--muted); }
  .machine-link { min-height: 44px; display: inline-flex; align-items: center; gap: 6px; }
  .status { flex: none; max-width: 35%; border-radius: 20px; padding: 6px 10px; font-size: 0.875rem; background: var(--bg); overflow-wrap: anywhere; }
  .status[data-tone='synced'] { color: var(--good); }
  .status[data-tone='attention'], .status[data-tone='review'] { color: var(--warn); }
  .status[data-tone='working'] { color: var(--accent-text); }
  .library-content { margin-top: 22px; }
  summary { cursor: pointer; padding: 8px 0; font-size: 0.875rem; }
  .search { display: grid; gap: 6px; margin: 12px 0; font-size: 0.875rem; }
  input { min-height: 44px; width: 100%; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: inherit; padding: 10px; }
  ul { list-style: none; padding: 0; max-height: 320px; overflow: auto; }
  li { display: flex; justify-content: space-between; gap: 12px; border-bottom: 1px solid var(--line); padding: 12px 0; font-size: 0.875rem; overflow-wrap: anywhere; }
  li span { color: var(--muted); text-align: right; }
  .footnote { margin: 20px 0 0; font-size: 0.875rem; line-height: 1.6; color: var(--muted); }
  .message { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; border: 1px solid var(--line); border-radius: 10px; padding: 14px; font-size: 0.875rem; line-height: 1.6; }
  .error { color: var(--bad); }
  .message.calm { color: var(--muted); }
  .left-out { border: 1px solid var(--line); border-radius: 10px; padding: 2px 14px; margin: 12px 0; font-size: 0.875rem; line-height: 1.6; }
  .left-out summary { padding: 12px 0; }
  .left-out ul { max-height: none; overflow: visible; margin: 0; padding: 0 0 4px; }
  .left-out li { display: block; border-bottom: 0; padding: 6px 0; color: var(--muted); }
  .left-out li strong { color: var(--ink); font-weight: 600; }
  .left-out p { margin: 0 0 12px; color: var(--muted); }
  .empty { padding: 24px 0; font-size: .9rem; }
  .empty p { color: var(--muted); }
  .automatic { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 44px; padding: 10px 0; font-size: 0.875rem; overflow-wrap: anywhere; }
  .automatic input { width: 20px; min-height: 20px; flex: none; }
  @media (max-width: 650px) {
    header { flex-direction: column; }
    .fleet { flex-wrap: wrap; padding: 16px; gap: 12px; }
    .fleet-text { flex-basis: calc(100% - 28px); }
    .fleet .btn { width: 100%; }
    article { flex-wrap: wrap; gap: 10px; }
    .machine-name { flex-basis: calc(100% - 24px); }
    .status { margin-left: 18px; max-width: none; }
    .machine-link { margin-left: auto; }
    li { flex-direction: column; gap: 5px; }
    li span { text-align: left; }
  }
</style>
