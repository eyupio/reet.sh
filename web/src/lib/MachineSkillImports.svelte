<script lang="ts">
 import { onMount } from 'svelte'
 import { shell } from './dialogShell'
 import { getImports, getImport, requestImport, approveImport, cancelImport, stopImport, getPrivateBundles, getPrivateRevisions, type ImportRequest, type ImportState, type PrivateBundle, type PrivateRevision, type SyncState } from './api'
 import CopyButton from './CopyButton.svelte'
 let { snapshot, source, onclose }: { snapshot: SyncState; source?: { environmentId: string; name: string }; onclose: () => void } = $props()
 let dialog = $state<HTMLDialogElement>()
 let importState = $state<ImportState>({requests: [], capabilities: []}), bundles = $state<PrivateBundle[]>([])
 let selected = $state<ImportRequest | null>(null), selectedID = $state(''), error = $state(''), busy = $state(false), loading = $state(true)
 let environmentId = $state(''), name = $state(''), bundleId = $state(''), exclusions = $state<string[]>([])
 let historyID = $state(''), revisions = $state<PrivateRevision[]>([])
 let requestKey = crypto.randomUUID(), alive = true, generation = 0, syncing = false
 let refreshAbort: AbortController | undefined
 const environments = $derived(snapshot.environments.filter(e => ['agents','claude','cursor','codex','opencode'].includes(e.agent)))
 const environment = $derived(environments.find(e => e.id === environmentId))
 const enrollment = $derived(snapshot.agents?.find(a => a.host.toLowerCase() === environment?.host.toLowerCase()))
 const ready = $derived(!!importState.capabilities.find(c => c.agentId === enrollment?.id && c.ready))
 const names = $derived(environment?.content?.skills.map(s => s.name) ?? [])
 const selectedFiles = $derived(selected?.preview?.files.filter(f => !exclusions.includes(f.path)) ?? [])
 const blocked = $derived(selectedFiles.some(f => !!f.blocked))
 function active(phase: string) { return ['queued','previewing','reviewing','approved','uploading'].includes(phase) }
 const pending = $derived(importState.requests.find(r => r.agentId === enrollment?.id && active(r.state)))
 const agentLabels: Record<string,string> = { agents: 'Shared agents / Codex', claude: 'Claude Code', cursor: 'Cursor', codex: 'Legacy Codex', opencode: 'OpenCode' }
 const stateLabels: Record<string,string> = {queued:'Queued',previewing:'Reading files',reviewing:'Ready for review',approved:'Approved',uploading:'Saving private copy',completed:'Saved',cancelled:'Cancelled',expired:'Expired',failed:'Needs attention'}
 const messages: Record<string,string> = { reviewing: 'Ready for your review. No file contents have been uploaded.', queued: 'Waiting for the machine to prepare a preview.', previewing: 'Reading file metadata on the machine. No file contents have been uploaded.', approved: 'Approved. Waiting for the machine to recheck and transfer your selection.', uploading: 'Verifying and saving the approved files privately.', cancelled: 'Cancelled. No further transfer is authorised.', expired: 'This preview expired or the connector restarted. Request a fresh preview.', failed: 'The import could not complete. Review the reason and request a fresh preview.', completed: 'Saved privately in reet.' }
 const reasons: Record<string,string> = { capture_failed: 'The source contains unsafe, unavailable or changing files.', source_changed: 'Files changed after preview. Nothing from this attempt was saved.', scope_unavailable: 'The source is no longer enrolled locally.', transfer_failed: 'The transfer was rejected. Check the destination revision, content validation and private storage capacity.', preview_expired: 'The preview or connector generation expired.', invalid_skill_manifest: 'SKILL.md needs valid matching metadata and instructions.' }
 async function refresh(force = false) {
  if (!alive || (!force && (syncing || busy))) return
  if (force) refreshAbort?.abort()
  const abort = new AbortController(); refreshAbort = abort
  syncing = true; const token = generation
  try {
   const [next, library] = await Promise.all([getImports(abort.signal),getPrivateBundles(abort.signal)])
   const current = selectedID ? await getImport(selectedID,abort.signal) : null
   if (!alive || token !== generation) return
   importState = next; bundles = library.bundles
   if (current) { const first = selected?.id !== current.id; selected = current; if (first) exclusions = [...current.excluded] }
   error = ''
  } catch (e) { if (alive && token === generation) error = String(e) }
  finally { if (refreshAbort === abort) { syncing = false; if (alive) loading = false } }
 }
 onMount(() => {
  environmentId = source?.environmentId ?? ''; name = source?.name ?? ''; dialog?.showModal(); void refresh()
  const timer = setInterval(() => { if (document.visibilityState === 'visible') void refresh() },3000)
  return () => { alive = false; generation++; refreshAbort?.abort(); clearInterval(timer) }
 })
 async function preview() {
  if (!enrollment || !environment || !name || !ready) return
  busy = true; error = ''; generation++
  try {
   const destination = bundles.find(b => b.id === bundleId)
   const created = await requestImport({ requestKey, agentId: enrollment.id, environmentId, name, bundleId, expectedBase: destination?.head ?? '' })
   if (!alive) return
   selectedID = created.id; selected = created; exclusions = [...created.excluded]
  } catch (e) { if (alive) error = `${String(e)} You can retry this same preview request.` }
  finally { if (alive) busy = false }
 }
 async function approve() {
  if (!selected?.previewHash || blocked || !selected.preview?.skillValid) return
  busy = true; error = ''; generation++
  try { const next = await approveImport(selected.id, selected.previewHash, [...exclusions]); if (alive) selected = next }
  catch (e) { if (alive) error = `${String(e)} Refresh to reconcile the original request before starting another.` }
  finally { if (alive) busy = false }
 }
 async function remove(r: ImportRequest) {
  busy = true; generation++
  try { if (active(r.state)) { const outcome = await stopImport(r.id); if (!alive) return; selectedID = outcome.id; selected = outcome } else { await cancelImport(r.id); if (!alive) return; if (r.id === selectedID) { selected = null; selectedID = ''; requestKey = crypto.randomUUID(); exclusions = [] } }; await refresh(true) }
  catch (e) { if (alive) error = String(e) }
  finally { if (alive) busy = false }
 }
 async function choose(r: ImportRequest) { generation++; selectedID = r.id; selected = null; error = ''; await refresh(true) }
 async function history(id: string) { historyID = id; revisions = []; try { const result = await getPrivateRevisions(id); if (alive && historyID === id) revisions = result.revisions } catch (e) { if (alive) error = String(e) } }
 function toggle(path: string) { exclusions = exclusions.includes(path) ? exclusions.filter(v => v !== path) : [...exclusions,path] }
</script>

<dialog bind:this={dialog} use:shell oncancel={(event) => { event.preventDefault(); onclose() }} aria-labelledby="machine-import-title">
 <header data-shell-head><div><p class="eyebrow" data-shell-aside>Your private toolkit</p><h2 id="machine-import-title">Bring a machine skill into reet</h2></div><button type="button" onclick={onclose} aria-label="Close skill imports" data-shell-close>Close ×</button></header>
 <div class="body" data-shell-body>
  <p class="intro">Review a skill’s files, choose what to keep, then save a private copy. Your machine files, shelf choices and published skills stay separate.</p>
  {#if error}<p role="alert" class="error">{error}</p>{/if}
  {#if loading}<p role="status">Loading imports and private copies…</p>{/if}
  {#if !selected && selectedID}<p role="status">Loading selected import…</p>
  {:else if !selected}
   <section aria-label="Choose import source" class="form">
    <label>Source scope<select aria-label="Source scope" bind:value={environmentId} onchange={() => { name = ''; bundleId = ''; requestKey = crypto.randomUUID() }}><option value="">Choose a machine and agent scope</option>{#each environments as env (env.id)}<option value={env.id}>{env.host} · {agentLabels[env.agent] ?? env.agent}{env.project ? ` · ${env.project}` : ' · Home'}</option>{/each}</select></label>
    <label>Skill folder<select aria-label="Skill folder" bind:value={name} onchange={() => { bundleId = ''; requestKey = crypto.randomUUID() }} disabled={!environment}><option value="">Choose a reported skill</option>{#each names as item}<option value={item}>{item}</option>{/each}</select></label>
    {#if environment && !names.length}<p class="hint">Request a machine scan to report its skill folders, then refresh the machine view.</p>{/if}
    <label>Save to<select aria-label="Save to" bind:value={bundleId} onchange={() => requestKey = crypto.randomUUID()}><option value="">New private copy</option>{#each bundles.filter(b => b.name === name) as b (b.id)}<option value={b.id}>Append to {b.name} · {b.id.slice(0,8)}</option>{/each}</select></label>
    {#if bundleId}<p class="hint">Expected revision: <code>{bundles.find(b => b.id === bundleId)?.head}</code>. A concurrent change will require a fresh review.</p>{/if}
    {#if environment && !ready}<p class="hint">This machine has not recently negotiated browser import. Update its reet connector and check that it is online and unpaused. Older connectors keep their scan and terminal features.</p>{/if}
    {#if pending}<p class="hint">This machine already has a pending import. <button type="button" class="link" onclick={() => choose(pending)}>Resume its review</button> or cancel it before another request.</p>{/if}

   </section>
  {:else}
   <section aria-label="Import review">
    <div class="review-head"><div><h3>{selected.name}</h3><p>{selected.host} · {agentLabels[selected.agent] ?? selected.agent} · {selected.project || 'Home scope'}</p><p>Destination: {selected.bundleId ? `private bundle ${selected.bundleId}` : 'new private copy'}</p></div><span class="status-pill">{stateLabels[selected.state] ?? selected.state}</span></div>
    <p role="status">{messages[selected.state] ?? 'Checking the import request.'}</p>
    {#if selected.error}<p class="error">{reasons[selected.error] ?? 'Request a fresh preview to continue.'}</p>{/if}
    {#if selected.preview && selected.state === 'reviewing'}
     <p class="hint">Only checked files will be imported. Exclude credential/configuration files and review supporting files. SKILL.md is required.</p>
     {#if !selected.preview.skillValid}<p class="error">SKILL.md needs a matching name, description and instructions. Fix it on the machine, then request a fresh preview.</p>{/if}
     <div class="file-table"><table><caption>Files on the machine</caption><thead><tr><th>Keep</th><th>File</th><th>Size</th><th>Mode</th></tr></thead><tbody>{#each selected.preview.files as file (file.path)}<tr class:blocked={!!file.blocked && !exclusions.includes(file.path)}><td><input type="checkbox" aria-label={`Include ${file.path}`} checked={!exclusions.includes(file.path)} disabled={busy || file.path === 'SKILL.md'} onchange={() => toggle(file.path)} /></td><td><strong>{file.path}</strong>{#if file.blocked}<small class="file-issue" class:excluded={exclusions.includes(file.path)}>{file.blocked.replaceAll('_',' ')}{exclusions.includes(file.path) ? ' · excluded' : ' · must be excluded'}</small>{/if}<code>{file.digest.slice(0,16)}…</code></td><td>{file.bytes.toLocaleString()} B</td><td>{file.executable ? 'Executable' : 'Regular'}</td></tr>{/each}</tbody></table></div>
     <p>{selectedFiles.length} files · {selectedFiles.reduce((total,f) => total + f.bytes,0).toLocaleString()} bytes selected</p>
     <details><summary>Source fingerprint</summary><code class="digest">{selected.preview.sourceDigest}</code></details>
     <p class="hint">The connector will recheck the complete source before upload. A changed file, expired preview or revoked enrollment stops the import.</p>

    {/if}
    {#if selected.receipt}
     <div class="receipt"><strong>Private copy saved: {selected.receipt.bundle.name}</strong><p>Bundle <code>{selected.receipt.bundle.id}</code></p><p>Revision <code>{selected.receipt.revision.id}</code> <CopyButton text={selected.receipt.revision.id} label="Copy revision ID" /></p><code class="digest">{selected.receipt.revision.manifestDigest}</code><p>Use the private copies below to inspect its revision history.</p></div>
    {/if}

   </section>
  {/if}
  <section class="library" aria-label="Private skill copies"><h3>Private copies</h3>{#if !bundles.length}<p class="hint">Your imported private copies will appear here.</p>{/if}<ul>{#each bundles as b (b.id)}<li><button type="button" class="link" onclick={() => history(b.id)}>{b.name}</button><code>{b.head.slice(0,12)}</code></li>{/each}</ul>{#if historyID}<ol>{#each revisions as revision (revision.id)}<li><strong>{new Date(revision.createdAt).toLocaleString()}</strong><code>{revision.id}</code><small>{revision.origin.kind} · {revision.manifestDigest.slice(0,16)}…</small></li>{/each}</ol>{/if}</section>
  <section aria-label="Recent import requests"><h3>Recent requests</h3><ul>{#each importState.requests as r (r.id)}<li><button type="button" class="link" onclick={() => choose(r)}>{r.name} · {r.host}</button><span>{r.state}</span>{#if !active(r.state)}<button type="button" disabled={busy} onclick={() => remove(r)} aria-label={`Dismiss ${r.name} request`}>Dismiss</button>{/if}</li>{/each}</ul></section>
 </div>
 <footer data-shell-foot>
  {#if selected}
   {#if selected.preview && selected.state === 'reviewing'}
    <button class="primary" type="button" disabled={busy || !!error || blocked || !selected.preview.skillValid} onclick={approve}>{busy ? 'Approving…' : `Import ${selectedFiles.length} files privately`}</button>
   {/if}
    <div class="actions"><button type="button" disabled={busy} onclick={() => void refresh()}>Refresh status</button><button type="button" disabled={busy} onclick={() => remove(selected!)}>{active(selected.state) ? 'Cancel import' : 'Dismiss import history'}</button><button type="button" disabled={busy} onclick={() => { generation++; selected = null; selectedID = ''; exclusions = []; requestKey = crypto.randomUUID() }}>Back to sources</button></div>
  {:else if !selectedID}
   <button class="primary" type="button" disabled={busy || !!pending || !ready || !name || loading} onclick={preview}>{busy ? 'Requesting…' : 'Request file preview'}</button>
   <small>No file contents are uploaded at this step.</small>
  {/if}
  <small data-shell-aside>Private import never publishes or runs a skill. Closing this panel keeps a pending request; use Cancel import to revoke it.</small>
 </footer>
</dialog>
<style>
 dialog { width: min(880px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--text); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0,1fr) auto; height: min(900px, calc(100dvh - 28px)); }
 dialog::backdrop { background: #0006; }
 header { position: sticky; top: 0; z-index: 1; background: var(--card); display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); }
 h2 { font: 500 1.65rem/1.2 var(--serif); margin: 4px 0 0; } h3 { margin: 0 0 8px; font-size: 1rem; }
 .eyebrow { color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; margin: 0; }
 .body { padding: 22px; min-height: 0; overflow: auto; } .intro,.hint,small,footer { color: var(--muted); font-size: .8rem; line-height: 1.5; }
 section { margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--line); }
 .form { display: grid; gap: 14px; } label { display: grid; gap: 5px; font-size: .82rem; font-weight: 600; } select { width: 100%; min-width: 0; }
 button,select { padding: 8px 12px; border: 1px solid var(--line); border-radius: 7px; background: var(--card); color: var(--text); } button { cursor: pointer; } header button { flex: none; white-space: nowrap; } .file-issue { color: var(--bad); } .file-issue.excluded { color: var(--muted); } button:disabled { cursor: default; opacity: .5; } .primary { background: var(--accent); color: white; border-color: var(--accent); font-weight: 600; } .link { border: 0; padding: 4px 0; text-align: left; color: var(--accent); }
 small { display: block; margin-top: 4px; } .error { color: var(--bad); overflow-wrap: anywhere; }
 .review-head { display: flex; justify-content: space-between; gap: 15px; } .review-head p { margin: 4px 0; font-size: .8rem; overflow-wrap: anywhere; } .status-pill { flex: none; align-self: flex-start; padding: 5px 8px; border: 1px solid var(--line); border-radius: 999px; font-size: .7rem; color: var(--muted); white-space: nowrap; }
 .file-table { overflow: auto; max-height: 340px; margin-top: 14px; border: 1px solid var(--line); border-radius: 8px; } table { width: 100%; border-collapse: collapse; font-size: .78rem; } caption { text-align: left; padding: 10px; font-weight: 600; } th,td { text-align: left; padding: 10px; border-top: 1px solid var(--line); } td:nth-child(2) { overflow-wrap: anywhere; } td:nth-child(3) { white-space: nowrap; } td code { display: block; color: var(--muted); font-size: .67rem; } input[type=checkbox] { width: 16px; height: 16px; } .blocked { background: color-mix(in srgb,var(--bad) 7%,transparent); }
 .digest,code { overflow-wrap: anywhere; font-size: .72rem; } .digest { display: block; margin-top: 8px; } .receipt { padding: 16px; border: 1px solid var(--line); border-radius: 10px; margin: 16px 0; }
 .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; } ul,ol { padding-left: 18px; } li { margin: 8px 0; font-size: .8rem; } li code,li span { margin-left: 12px; } ol code { display: block; margin-left: 0; } footer { padding: 16px 22px; border-top: 1px solid var(--line); }
 @media(max-width:500px) { header,.body { padding: 16px; } h2 { font-size: 1.35rem; } th,td { padding: 7px; } }
</style>
