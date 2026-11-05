<script lang="ts">
  import { onMount } from 'svelte'
  import { actOnGafferProvider, getGafferModels, getGafferSettings, saveGafferProvider, UnauthorizedError, type GafferInput, type GafferProvider, type GafferSettings } from './api'
  import CollectionPanel from './CollectionPanel.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import GafferAvatar from './terminal/GafferAvatar.svelte'
  import { shell } from './dialogShell'

  let { onerror }: { onerror?: (e: unknown) => void } = $props()
  let data = $state<GafferSettings | null>(null), loading = $state(true), error = $state(''), notice = $state('')
  let editing = $state<GafferProvider | null>(null), formOpen = $state(false), removing = $state<GafferProvider | null>(null)
  let busy = $state(''), saving = $state(false), formError = $state(''), preset = $state('openai')
  let name = $state(''), kind = $state('openai'), baseUrl = $state(''), model = $state(''), effort = $state('')
  let key = $state(''), clearKey = $state(false), localOnly = $state(false), enabled = $state(true), makeDefault = $state(false)
  let allowStatus = $state(false), allowSessions = $state(false), allowCode = $state(false)
  let models = $state<string[]>([]), modelsLoading = $state(false), modelsError = $state(''), modelFilter = $state(''), manual = $state(false)
  let dialog = $state<HTMLDialogElement>(), nameInput = $state<HTMLInputElement>()
  const life = new AbortController()
  let modelRequest: AbortController | undefined, actionRequest: AbortController | undefined, epoch = 0, loadEpoch = 0
  const selectedPreset = $derived(data?.presets.find(p => p.id === preset))
  const filteredModels = $derived(models.filter(id => id.toLowerCase().includes(modelFilter.toLowerCase())))
  const savedKey = $derived(!!editing?.hasKey && editing.kind === kind && editing.baseUrl === baseUrl && !clearKey)
  const active = $derived(data?.providers.filter(p => p.enabled).length ?? 0)

  onMount(() => {
    void load()
    return () => { life.abort(); modelRequest?.abort(); actionRequest?.abort(); key = '' }
  })
  $effect(() => {
    if (formOpen && dialog && !dialog.open) { dialog.showModal(); nameInput?.focus() }
  })
  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) onerror?.(e)
    return e instanceof Error ? e.message : 'That could not be done just now.'
  }
  async function load() {
    const mine = ++loadEpoch; loading = true; error = ''
    try { const answer = await getGafferSettings(life.signal); if (!life.signal.aborted && mine === loadEpoch) data = answer }
    catch (e) { if (!life.signal.aborted && mine === loadEpoch) error = fail(e) }
    finally { if (!life.signal.aborted && mine === loadEpoch) loading = false }
  }
  function invalidateModels() { epoch++; modelRequest?.abort(); modelsLoading = false; models = []; modelsError = ''; modelFilter = '' }
  function choosePreset(id: string) {
    const p = data?.presets.find(p => p.id === id); if (!p) return
    invalidateModels(); preset = id; kind = p.kind; baseUrl = p.baseUrl; name = p.name; localOnly = p.localOnly
    model = ''; manual = false; key = ''; clearKey = false; if (kind === 'anthropic') effort = ''
  }
  function openForm(p: GafferProvider | null) {
    editing = p; formError = ''; notice = ''; invalidateModels(); key = ''; clearKey = false
    if (p) {
      name = p.name; kind = p.kind; baseUrl = p.baseUrl; model = p.model; effort = p.reasoningEffort
      localOnly = p.localOnly; enabled = p.enabled; allowStatus = p.allowStatus; allowSessions = p.allowSessions; allowCode = p.allowCode
      preset = data?.presets.find(x => x.kind === p.kind && x.baseUrl === p.baseUrl)?.id ?? (p.kind === 'anthropic' ? 'anthropic' : p.kind === 'openai' ? 'openai' : 'compatible')
    } else {
      choosePreset('openai'); effort = ''; enabled = true; allowStatus = false; allowSessions = false; allowCode = false
    }
    makeDefault = !data?.providers.some(x => x.isDefault); manual = false; formOpen = true
  }
  function closeForm() {
    if (saving) return
    invalidateModels(); dialog?.close(); formOpen = false; editing = null; key = ''; clearKey = false
  }
  function input(): GafferInput {
    return { ...(editing ? { id: editing.id, revision: editing.revision } : {}), name, kind, baseUrl, model, reasoningEffort: kind === 'anthropic' ? '' : effort,
      enabled, localOnly, allowStatus, allowSessions, allowCode: allowSessions && allowCode, ...(key ? { apiKey: key } : {}), clearKey, ...(!editing ? { makeDefault } : {}) }
  }
  async function fetchModels() {
    modelRequest?.abort(); const request = new AbortController(); modelRequest = request
    const mine = ++epoch; modelsLoading = true; modelsError = ''
    try {
      const answer = await getGafferModels(input(), request.signal)
      if (request.signal.aborted || life.signal.aborted || mine !== epoch) return
      models = answer.models
      if (!models.length) modelsError = 'This provider returned an empty list. You can enter its model name below.'
    } catch (e) { if (!request.signal.aborted && !life.signal.aborted && mine === epoch) modelsError = fail(e) }
    finally { if (!life.signal.aborted && mine === epoch) modelsLoading = false }
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); saving = true; formError = ''; modelRequest?.abort(); epoch++; modelsLoading = false
    try {
      const answer = await saveGafferProvider(input(), life.signal)
      if (life.signal.aborted) return
      loadEpoch++; data = answer; saving = false; closeForm(); notice = 'Reet, provider saved. Your choice of brains is in place.'
    } catch (e) { if (!life.signal.aborted) formError = `${fail(e)} If the save was interrupted, reload your providers before trying again.` }
    finally { if (!life.signal.aborted) saving = false }
  }
  async function act(p: GafferProvider, action: 'test' | 'default' | 'enabled' | 'delete') {
    actionRequest?.abort(); const request = new AbortController(); actionRequest = request; busy = `${p.id}:${action}`; error = ''; notice = ''
    try {
      const answer = await actOnGafferProvider(p, action, action === 'enabled' ? !p.enabled : undefined, request.signal)
      if (request.signal.aborted || life.signal.aborted) return
      loadEpoch++; data = answer
      const tested = answer.providers.find(x => x.id === p.id)?.lastCheck
      notice = action === 'test' ? tested?.ok ? 'Connection answered. Kettle on.' : 'The provider needs attention. Its test result is shown below.' : action === 'default' ? 'Gaffer’s default provider is set.' : action === 'delete' ? 'Provider and saved key removed.' : p.enabled ? 'Provider paused.' : 'Provider enabled.'
    } catch (e) { if (!request.signal.aborted && !life.signal.aborted) error = fail(e) }
    finally { if (!life.signal.aborted && actionRequest === request) busy = '' }
  }
  function cancelTest() { actionRequest?.abort(); busy = ''; notice = 'Connection test cancelled.' }
</script>

<section class="gaffer-settings" aria-label="Gaffer AI settings">
  <header class="intro">
    <div class="portrait" aria-hidden="true"><GafferAvatar phase="idle" pose="tea" /></div>
    <div><p class="eyebrow">Your critical buddy</p><h2>Gaffer AI</h2><p class="quip">Pick the brains. I’ll put the kettle on.</p>
      <p class="intro-copy">Choose who Gaffer talks to. His model is separate from the agent doing your work, and these providers belong to your account.</p></div>
  </header>
  <p class="note">Choose a provider, then use Ask Gaffer for a natter. Use Context in the chat to preview and share retained reet or session data. Worker instructions and offline follow-up are coming next.</p>
  {#if notice}<p class="feedback good" role="status">{notice}</p>{/if}
  {#if error}<div class="feedback bad" role="alert"><p>{error}</p><button class="btn quiet" onclick={() => void load()} disabled={!!busy}>Reload providers</button></div>{/if}
  {#if data && !data.available}<p class="feedback bad" role="alert">The controller’s provider-key storage is unavailable. Ask an administrator to check the instance key configuration.</p>{/if}
  <CollectionPanel label="Your Gaffer providers" count={loading ? 'Reading your providers…' : `${data?.providers.length ?? 0} providers · ${active} enabled`}>
    {#snippet actions()}<button class="btn primary" disabled={loading || !data?.available || !!busy || (data?.providers.length ?? 0) >= (data?.maxProviders ?? 12)} onclick={() => openForm(null)}>Add provider</button>{/snippet}
    {#snippet toolbar()}<p class="note">Use a hosted API or a local model. Gaffer’s default provider stays separate from the agent working in each session.</p>{/snippet}
    {#if loading && !data}<div class="empty" role="status"><span class="loading-line"></span><p>Finding your usuals…</p></div>
    {:else if data && !data.providers.length}<div class="empty"><span class="empty-mark" aria-hidden="true">G</span><h3>Nowt on the list yet.</h3><p>Add a provider, fetch its models and choose one for Gaffer.</p><p class="note">Keys stay on the controller. Saving a provider makes no model request.</p></div>
    {:else if data}
      <ul class="providers">
        {#each data.providers as p (p.id)}
          <li class:paused={!p.enabled}>
            <div class="provider-head"><div><p class="provider-kind">{p.localOnly ? 'Local network' : p.kind === 'anthropic' ? 'Anthropic API' : p.kind === 'openai' ? 'OpenAI API' : 'Compatible API'}</p><h3>{p.name}</h3></div>
              <div class="badges">{#if p.isDefault}<span class="badge default">Default</span>{/if}<span class="badge" class:off={!p.enabled}>{p.enabled ? 'Enabled' : 'Paused'}</span></div>
            </div>
            <p class="model-name">{p.model}</p><p class="endpoint">{p.baseUrl}</p>
            <div class="metadata"><span>{p.hasKey ? 'Key saved securely' : 'No key saved'}</span><span>{p.allowSessions ? 'Followed-session context allowed' : p.allowStatus ? 'Reet status allowed' : 'Reet context off'}</span></div>
            {#if busy === `${p.id}:test`}<div class="test-result" role="status"><span>Testing the connection…</span><button class="btn quiet" onclick={cancelTest}>Cancel test</button></div>
            {:else if p.lastCheck}<div class="test-result" class:failed={!p.lastCheck.ok}><span class="result-dot" aria-hidden="true"></span><div><strong>{p.lastCheck.ok ? 'Connection answered' : 'Needs attention'}</strong><p>{p.lastCheck.ok ? `${p.lastCheck.latencyMs} ms · ${p.lastCheck.usageReported ? 'usage reported' : 'usage not reported'}` : p.lastCheck.error}</p></div></div>
            {:else}<p class="untested">Connection not tested yet</p>{/if}
            <div class="provider-actions">
              <button class="btn" disabled={!!busy || !data.available} onclick={() => void act(p, 'test')}>Test connection</button>
              {#if !p.isDefault}<button class="btn quiet" disabled={!!busy || !p.enabled} onclick={() => void act(p, 'default')}>Make default</button>{/if}
              <button class="btn quiet" disabled={!!busy} onclick={() => openForm(p)}>Edit</button>
              <button class="btn quiet" disabled={!!busy} onclick={() => void act(p, 'enabled')}>{p.enabled ? 'Pause' : 'Enable'}</button>
              <button class="btn quiet remove" disabled={!!busy} onclick={() => removing = p}>Remove</button>
            </div>
          </li>
        {/each}
      </ul>
      <p class="footnote">Test connection sends a short request to your selected model and may use provider credits. It shares no reet or session context.</p>
    {/if}
  </CollectionPanel>
</section>

{#if formOpen}
  <dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby="gaffer-provider-title" oncancel={event => { event.preventDefault(); closeForm() }}>
    <div class="provider-form" data-shell-card>
      <header><p class="eyebrow">Gaffer’s choice of brains</p><h2 id="gaffer-provider-title">{editing ? 'Edit provider' : 'Add a provider'}</h2><p class="note">Your key is saved securely and is never read back into this form.</p></header>
      <form onsubmit={save}>
        {#if formError}<p class="feedback bad" role="alert">{formError}</p>{/if}
        <fieldset disabled={saving}>
          <div class="grid2">
            <label>Provider<select aria-label="Provider" value={preset} onchange={event => choosePreset(event.currentTarget.value)}>{#each data?.presets ?? [] as p}<option value={p.id} disabled={p.localOnly && !data?.canUseLocal}>{p.name}{p.localOnly && !data?.canUseLocal ? ' · administrator needed' : ''}</option>{/each}</select></label>
            <label>Name<input bind:this={nameInput} bind:value={name} required maxlength="80" autocomplete="off" /></label>
          </div>
          {#if selectedPreset}<p class="field-help">{selectedPreset.help}</p>{/if}
          <label>API address<input bind:value={baseUrl} oninput={invalidateModels} type="url" required placeholder="https://your-provider.example/v1" spellcheck="false" autocomplete="off" /></label>
          <label>API key<input type="password" bind:value={key} oninput={() => { clearKey = false; invalidateModels() }} autocomplete="new-password" spellcheck="false" placeholder={savedKey ? 'Saved key kept when left blank' : 'Your provider’s key, if required'} /></label>
          {#if editing?.hasKey}<label class="check"><input type="checkbox" bind:checked={clearKey} onchange={event => { if (event.currentTarget.checked) key = ''; invalidateModels() }} />Remove the saved key</label>{/if}
          {#if editing?.hasKey && !savedKey && !key && !clearKey}<p class="field-help">For a changed address, enter its key or choose Remove the saved key before fetching models.</p>{/if}
          <label class="check"><input type="checkbox" bind:checked={localOnly} disabled={!data?.canUseLocal} onchange={invalidateModels} />Local network only</label>
          <p class="field-help">{data?.canUseLocal ? 'For local servers and private networks reachable from the controller. Hosted providers use HTTPS.' : 'An owner or administrator can configure providers on the controller’s local network.'}</p>
          <div class="model-section"><div class="field-head"><label for="gaffer-model">Model</label><button type="button" class="btn quiet" onclick={() => void fetchModels()} disabled={modelsLoading || !name || !baseUrl}>{modelsLoading ? 'Reading models…' : models.length ? 'Refresh models' : 'Fetch models'}</button></div>
            {#if models.length}<input class="model-filter" aria-label="Filter provider models" type="search" bind:value={modelFilter} onkeydown={event => { if (event.key === 'Enter') event.preventDefault() }} placeholder={`Search ${models.length} models…`} />{/if}
            <select id="gaffer-model" value={manual ? '__manual__' : model} onchange={event => { const value = event.currentTarget.value; manual = value === '__manual__'; if (!manual) model = value }}>
              <option value="" disabled>Choose a model</option>
              {#if model && !filteredModels.includes(model)}<option value={model}>{model} · current selection</option>{/if}
              {#each filteredModels as id}<option value={id}>{id}</option>{/each}
              <option value="__manual__">Enter a model name…</option>
            </select>
            {#if manual}<label>Exact model name<input bind:value={model} required maxlength="256" autocomplete="off" spellcheck="false" /></label>{/if}
            {#if modelsError}<p class="field-help bad" role="status">{modelsError} You can also enter the model name.</p>{:else if models.length}<p class="field-help" role="status">Read {models.length} models from this provider. Test your choice after saving.</p>{:else}<p class="field-help">Fetch the provider’s live list, or enter a model name.</p>{/if}
          </div>
          {#if kind !== 'anthropic'}<label>Reasoning effort<select bind:value={effort}><option value="">Provider default</option><option value="none">None</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option></select><small>Supported values depend on the model. Test connection checks your choice.</small></label>{/if}
          <section class="context-permissions" aria-label="Provider context permissions"><h3>What may Gaffer share?</h3><p class="field-help">These set the maximum data this provider may receive. Choose and preview sharing separately in each Gaffer conversation. Saving them sends no context.</p>
            <label class="check"><input type="checkbox" bind:checked={allowStatus} />Reet status and activity</label>
            <label class="check"><input type="checkbox" bind:checked={allowSessions} onchange={event => { if (!event.currentTarget.checked) allowCode = false }} />Followed-session conversations and tool results</label>
            <label class="check"><input type="checkbox" bind:checked={allowCode} disabled={!allowSessions} />Code and logs from followed sessions</label>
          </section>
          <label class="check"><input type="checkbox" bind:checked={enabled} onchange={event => { if (!event.currentTarget.checked) makeDefault = false }} />Enable this provider</label>
          {#if !editing}<label class="check"><input type="checkbox" bind:checked={makeDefault} disabled={!enabled} />Make this Gaffer’s default</label>{/if}
        </fieldset>
        <div class="form-actions" data-shell-foot><button class="btn quiet" type="button" disabled={saving} onclick={closeForm}>Cancel</button><button class="btn primary" disabled={saving || !name || !model || !baseUrl}>{saving ? 'Saving…' : 'Save provider'}</button></div>
      </form>
    </div>
  </dialog>
{/if}
{#if removing}<ConfirmDialog title={`Remove ${removing.name}?`} body="This removes the provider settings and its saved key. You can add it again later. Your working agents keep their own settings." confirmLabel="Remove provider" danger oncancel={() => removing = null} onconfirm={() => { const p = removing; removing = null; if (p) void act(p, 'delete') }} />{/if}

<style>
  .gaffer-settings { display: grid; gap: 20px; margin-top: 24px; }
  .intro { display: flex; align-items: center; gap: 24px; padding: 26px; border: 1px solid var(--line); border-radius: 14px; background: linear-gradient(120deg, var(--card), color-mix(in srgb, var(--tint) 55%, var(--card))); }
  .portrait { width: 100px; min-width: 100px; align-self: stretch; display: grid; align-items: center; }
  .eyebrow, .provider-kind { margin: 0 0 6px; font-size: 14px; color: var(--muted); letter-spacing: .06em; text-transform: uppercase; }
  h2 { margin: 0; font: 600 1.85rem/1.15 var(--serif); letter-spacing: -.025em; }
  h3 { margin: 0; font: 600 1.15rem/1.3 var(--sans); overflow-wrap: anywhere; }
  .quip { margin: 8px 0; color: var(--accent-text); font-size: 1rem; }
  .intro-copy, .note, .field-help, small { color: var(--muted); font-size: 14px; line-height: 1.5; }
  .intro-copy { margin: 0; max-width: 58ch; }
  .note { margin: 0; }
  .feedback { border: 1px solid var(--line); border-radius: 10px; padding: 12px 16px; margin: 0; background: var(--card); overflow-wrap: anywhere; }
  .feedback p { margin: 0 0 8px; }
  .feedback.good { border-color: color-mix(in srgb, var(--good) 45%, var(--line)); }
  .feedback.bad, .bad { color: var(--bad); }
  .feedback.bad { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); }
  .empty { text-align: center; padding: 40px 16px 24px; color: var(--muted); }
  .empty h3 { color: var(--ink); margin-top: 14px; }
  .empty p { margin: 10px auto 0; max-width: 44ch; }
  .empty-mark { display: inline-grid; place-items: center; width: 48px; height: 48px; border: 1px solid var(--line); border-radius: 13px; font: 600 1.7rem var(--serif); background: var(--bg-sunk); color: var(--accent-text); }
  .providers { margin: 0; padding: 0; list-style: none; display: grid; gap: 16px; }
  .providers li { margin-top: 18px; border: 1px solid var(--line); border-radius: 12px; padding: 20px; min-width: 0; background: var(--card-raised); }
  .providers li + li { margin-top: 0; }
  .provider-head { display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 12px; }
  .provider-kind { font-size: 14px; letter-spacing: .035em; }
  .badges { display: flex; gap: 6px; flex-wrap: wrap; }
  .badge { padding: 4px 9px; border: 1px solid var(--line); border-radius: 999px; font-size: 14px; color: var(--muted); background: var(--bg); }
  .badge.default { background: var(--tint); color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 30%, var(--line)); }
  .badge.off { background: var(--bg-sunk); }
  .model-name { font-family: var(--mono); font-size: 14px; margin: 15px 0 5px; overflow-wrap: anywhere; }
  .endpoint { margin: 0; font-size: 14px; color: var(--muted); overflow-wrap: anywhere; }
  .metadata { display: flex; flex-wrap: wrap; gap: 6px 20px; margin-top: 12px; font-size: 14px; color: var(--muted); }
  .test-result { display: flex; align-items: center; gap: 10px; margin-top: 16px; padding: 12px; border-radius: 8px; background: color-mix(in srgb, var(--good) 6%, var(--bg)); font-size: 14px; }
  .test-result.failed { background: color-mix(in srgb, var(--bad) 6%, var(--bg)); }
  .test-result p { margin: 3px 0 0; color: var(--muted); overflow-wrap: anywhere; }
  .result-dot { width: 7px; height: 7px; flex: none; background: var(--good); border-radius: 50%; }
  .failed .result-dot { background: var(--bad); }
  .untested { color: var(--muted); margin: 16px 0 0; font-size: 14px; }
  .provider-actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--line-soft); }
  .remove { margin-left: auto; color: var(--bad); }
  .footnote { margin: 16px 0 0; color: var(--muted); font-size: 14px; line-height: 1.5; }
  dialog { border: 0; padding: 0; width: min(620px, calc(100vw - 24px)); max-height: calc(100dvh - 24px); overflow: auto; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); backdrop-filter: blur(3px); }
  .provider-form { padding: 26px; background: var(--card); border: 1px solid var(--line); border-radius: 14px; }
  .provider-form header .note { margin-top: 10px; }
  form { display: grid; gap: 18px; margin-top: 20px; }
  fieldset { border: 0; padding: 0; margin: 0; min-width: 0; display: grid; gap: 14px; }
  label { display: grid; gap: 7px; font-size: 14px; font-weight: 600; min-width: 0; }
  label small { font-weight: 400; }
  input:not([type='checkbox']), select { min-width: 0; width: 100%; box-sizing: border-box; min-height: 44px; border: 1px solid var(--line-strong); border-radius: 8px; background: var(--bg); color: var(--ink); padding: 10px 12px; font: 400 14px/1.4 var(--sans); }
  input:focus-visible, select:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .check { display: flex; align-items: center; gap: 10px; min-height: 36px; font-weight: 400; }
  .check input { width: 18px; height: 18px; flex: none; accent-color: var(--accent); }
  .field-help { margin: -4px 0 0; }
  .model-section { border-top: 1px solid var(--line); padding-top: 16px; display: grid; gap: 10px; }
  .field-head { display: flex; justify-content: space-between; gap: 10px; align-items: center; }
  .context-permissions { border: 1px solid var(--line); padding: 14px; border-radius: 10px; display: grid; gap: 8px; background: var(--bg-sunk); }
  .context-permissions h3 { font-size: 14px; }
  .context-permissions .field-help { margin: 0; }
  .form-actions { display: flex; justify-content: flex-end; gap: 8px; padding-top: 16px; border-top: 1px solid var(--line); }
  .btn { min-height: 40px; }
  @media (max-width: 650px) {
    .intro { gap: 14px; padding: 18px 14px; align-items: flex-start; }
    .portrait { width: 62px; min-width: 62px; align-self: flex-start; }
    h2 { font-size: 1.6rem; }
    .providers li { padding: 14px; }
    .btn, .check { min-height: 44px; }
    .provider-actions { gap: 8px; }
    .provider-actions .btn { flex: 1 1 auto; }
    .remove { margin-left: 0; }
    .provider-form { padding: 20px 16px; }
    .grid2 { grid-template-columns: 1fr; }
    .form-actions .btn { flex: 1; }
  }
</style>
