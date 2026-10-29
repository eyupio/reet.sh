<script lang="ts">
  import { onMount, tick } from 'svelte'
  import {
    addModelFavourite, ApiError, getProfile, listModelFavourites, removeModelFavourite,
    reorderModelFavourites, saveProfile, setModelFavouriteDefault, UnauthorizedError,
    type ModelFavourites,
  } from './api'
  import { CATALOGUE } from './catalogue'
  import { cleanModel, modelSuggestions, toolName, validateModel } from './agentLaunch'
  import {
    dedupeModels, effortChoices, favouriteKey, modelOptions, starLabel,
    type MachineModels, type ModelFavourite,
  } from './models'
  import SlotPicker, { type SlotOption } from './launch/SlotPicker.svelte'
  import { allModels, type AllModels } from './terminal/api'

  let { canWrite, onerror }: { canWrite: boolean; onerror?: (e: unknown) => void } = $props()

  const agents = CATALOGUE.filter(agent => agent.enabled)
  const life = new AbortController()
  let loading = $state(true), busy = $state('')
  let defaultTool = $state(''), defaultModel = $state(''), defaultEffort = $state('')
  let defaultBaseBranch = $state(''), worktreesByDefault = $state(false)
  let formMessage = $state(''), notice = $state('')
  let modelButton = $state<HTMLButtonElement | null>(null)
  let agentSelect = $state<HTMLSelectElement | null>(null)
  let pickerOpen = $state(false), touch = $state(false)

  let favourites = $state<ModelFavourite[]>([])
  let favouritesVersion = $state(0)
  let favouritesLoading = $state(true)
  let favBusy = $state(''), favMessage = $state(''), favNotice = $state('')

  let addTool = $state(agents[0]?.id ?? '')
  let addMachines = $state<AllModels['machines']>([])
  let addLoading = $state(false), addError = $state('')
  let addButton = $state<HTMLButtonElement | null>(null)
  let addPickerOpen = $state(false)

  const takesEffort = $derived(effortChoices(defaultTool).length > 1)
  const modelLabel = $derived(cleanModel(defaultModel) || 'Agent default')
  const addName = $derived(toolName(addTool))
  const addModelList = $derived.by(() => {
    const models = dedupeModels(addMachines.flatMap(machine => machine.models ?? []))
    const list: MachineModels = {
      tool: addTool, state: models.length ? 'listed' : 'unsupported', models,
      more: addMachines.some(machine => machine.more), source: '', version: '',
      fetchedAt: null, checkedAt: null, reason: '', sentence: '',
    }
    return list
  })
  const addHost = $derived(addMachines.length === 1 ? addMachines[0].host : addMachines.length ? 'any machine' : '')
  const addStatus = $derived.by(() => {
    if (addLoading) return 'Reading your machines’ saved model lists…'
    if (addError) return addError
    if (!addMachines.length) return 'No machine has a saved model list for this agent yet; favourites, suggestions and Other still work.'
    if (!addModelList.models.length) return 'The saved lists have no models for this agent; favourites, suggestions and Other still work.'
    return `Read from ${addMachines.length === 1 ? addHost : `${addMachines.length} machines`}.`
  })

  type Group = { tool: string; name: string; items: { fav: ModelFavourite; inGroup: number; count: number }[] }
  const groups = $derived.by(() => {
    const out: Group[] = []
    const byTool = new Map<string, Group>()
    for (const fav of favourites) {
      let group = byTool.get(fav.tool)
      if (!group) {
        group = { tool: fav.tool, name: toolName(fav.tool), items: [] }
        byTool.set(fav.tool, group)
        out.push(group)
      }
      group.items.push({ fav, inGroup: 0, count: 0 })
    }
    for (const group of out) for (let i = 0; i < group.items.length; i++) {
      group.items[i].inGroup = i
      group.items[i].count = group.items.length
    }
    return out
  })

  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) onerror?.(e)
    else if (e instanceof ApiError && e.status === 401) onerror?.(new UnauthorizedError())
  }

  function fillProfile(profile: Awaited<ReturnType<typeof getProfile>>['profile']) {
    defaultTool = profile.defaultTool ?? ''
    defaultModel = profile.defaultModel ?? ''
    defaultEffort = profile.defaultEffort ?? ''
    defaultBaseBranch = profile.defaultBaseBranch ?? ''
    worktreesByDefault = profile.worktreesByDefault ?? false
  }

  async function loadProfile() {
    try {
      const answer = await getProfile()
      if (life.signal.aborted) return
      fillProfile(answer.profile)
    } catch (e) {
      if (life.signal.aborted) return
      fail(e)
      formMessage = "Couldn't read your defaults just now. Reload the page to try again."
    } finally {
      if (!life.signal.aborted) loading = false
    }
  }

  function applyFavourites(answer: ModelFavourites) {
    favourites = answer.favourites ?? []
    favouritesVersion = typeof answer.version === 'number' ? answer.version : 0
  }

  async function reloadFavourites() {
    const answer = await listModelFavourites(life.signal)
    if (!life.signal.aborted) applyFavourites(answer)
  }

  async function loadFavourites() {
    try {
      await reloadFavourites()
      if (!life.signal.aborted) favMessage = ''
    } catch (e) {
      if (life.signal.aborted) return
      fail(e)
      favMessage = "Couldn't read your favourite models just now."
    } finally {
      if (!life.signal.aborted) favouritesLoading = false
    }
  }

  function snapshotFavourites() {
    return { favourites: [...favourites], version: favouritesVersion }
  }
  function restoreFavourites(previous: { favourites: ModelFavourite[]; version: number }) {
    favourites = previous.favourites
    favouritesVersion = previous.version
  }
  async function favouriteProblem(e: unknown, previous: { favourites: ModelFavourite[]; version: number }) {
    if (e instanceof ApiError && e.status === 409) {
      try { await reloadFavourites() } catch { restoreFavourites(previous) }
      favMessage = 'Your favourites changed. Reload them, then try again.'
      return
    }
    restoreFavourites(previous)
    fail(e)
    favMessage = e instanceof ApiError && e.message ? e.message : "Couldn't save your favourites just now. Try again."
  }

  async function addFavourite(tool: string, model: string) {
    const value = cleanModel(model)
    if (!canWrite || favBusy || !tool || !value) return
    const previous = snapshotFavourites()
    const key = favouriteKey(tool, value)
    if (!favourites.some(favourite => favouriteKey(favourite.tool, favourite.model) === key)) {
      favourites = [...favourites, { tool, model: value, agentDefault: false }]
    }
    favBusy = 'add'; favMessage = ''; favNotice = ''
    try {
      const answer = await addModelFavourite(tool, value)
      if (life.signal.aborted) return
      applyFavourites(answer)
      favNotice = `Starred ${value} for ${toolName(tool)}.`
    } catch (e) {
      if (!life.signal.aborted) await favouriteProblem(e, previous)
    } finally {
      if (!life.signal.aborted) favBusy = ''
    }
  }

  async function removeFavourite(favourite: ModelFavourite) {
    if (!canWrite || favBusy) return
    const previous = snapshotFavourites()
    const key = favouriteKey(favourite.tool, favourite.model)
    favourites = favourites.filter(item => favouriteKey(item.tool, item.model) !== key)
    favBusy = 'remove'; favMessage = ''; favNotice = ''
    try {
      await removeModelFavourite(favourite.tool, favourite.model)
      if (life.signal.aborted) return
      // Removing answers with no body, so read the list again for its new version; the optimistic row stays if that read fails.
      try { await reloadFavourites() } catch { /* the next change reloads it after a 409 */ }
      favNotice = `Removed ${favourite.model} for ${toolName(favourite.tool)}.`
    } catch (e) {
      if (!life.signal.aborted) await favouriteProblem(e, previous)
    } finally {
      if (!life.signal.aborted) favBusy = ''
    }
  }

  async function setDefault(favourite: ModelFavourite) {
    if (!canWrite || favBusy) return
    const previous = snapshotFavourites()
    const clearing = favourite.agentDefault
    // At most one default per agent: marking one clears the others for that agent first, as the server does.
    favourites = favourites.map(item => item.tool === favourite.tool
      ? { ...item, agentDefault: !clearing && item.model === favourite.model }
      : item)
    favBusy = 'default'; favMessage = ''; favNotice = ''
    try {
      const answer = await setModelFavouriteDefault(favourite.tool, clearing ? '' : favourite.model)
      if (life.signal.aborted) return
      applyFavourites(answer)
      favNotice = clearing
        ? `Cleared the default for ${toolName(favourite.tool)}.`
        : `New sessions that pick ${toolName(favourite.tool)} use ${favourite.model}.`
    } catch (e) {
      if (!life.signal.aborted) await favouriteProblem(e, previous)
    } finally {
      if (!life.signal.aborted) favBusy = ''
    }
  }

  function moveWithinGroup(favourite: ModelFavourite, delta: number): ModelFavourite[] | null {
    const same = favourites.filter(item => item.tool === favourite.tool)
    const at = same.findIndex(item => item.model === favourite.model)
    const to = at + delta
    if (at < 0 || to < 0 || to >= same.length) return null
    const reordered = [...same]
    ;[reordered[at], reordered[to]] = [reordered[to], reordered[at]]
    let next = 0
    return favourites.map(item => item.tool === favourite.tool ? reordered[next++] : item)
  }

  async function moveFavourite(favourite: ModelFavourite, delta: number) {
    if (!canWrite || favBusy) return
    const next = moveWithinGroup(favourite, delta)
    if (!next) return
    const previous = snapshotFavourites()
    favourites = next
    favBusy = 'order'; favMessage = ''; favNotice = ''
    try {
      const answer = await reorderModelFavourites(previous.version, next.map(item => ({ tool: item.tool, model: item.model })))
      if (life.signal.aborted) return
      applyFavourites(answer)
      favNotice = 'Favourites reordered.'
    } catch (e) {
      if (!life.signal.aborted) await favouriteProblem(e, previous)
    } finally {
      if (!life.signal.aborted) favBusy = ''
    }
  }

  async function toggleStar(tool: string, model: string, starred: boolean) {
    if (!canWrite || !model) return
    if (starred) {
      const key = favouriteKey(tool, model)
      const favourite = favourites.find(item => favouriteKey(item.tool, item.model) === key)
      if (favourite) await removeFavourite(favourite)
    } else {
      await addFavourite(tool, model)
    }
  }

  function withStars(tool: string, options: ReturnType<typeof modelOptions>): SlotOption[] {
    return options.map(option => option.value === '' ? option : {
      ...option,
      starLabel: starLabel(tool, option.value, !!option.starred),
      onstar: () => { void toggleStar(tool, option.value, !!option.starred) },
    })
  }

  function optionsForDefault(): SlotOption[] {
    if (!defaultTool) return []
    return withStars(defaultTool, modelOptions({
      tool: defaultTool, current: defaultModel, machine: null, favourites, recent: [],
      suggestions: modelSuggestions(defaultTool), selectedModel: defaultModel,
      agentName: toolName(defaultTool),
    }))
  }

  function optionsForAdd(): SlotOption[] {
    if (!addTool) return []
    return withStars(addTool, modelOptions({
      tool: addTool, current: '', machine: addModelList.models.length ? addModelList : null,
      favourites, recent: [], suggestions: modelSuggestions(addTool),
      host: addHost, agentName: toolName(addTool),
    }))
  }

  function openModelPicker() {
    if (!canWrite || busy || !defaultTool) return
    pickerOpen = true
  }
  function pickModel(value: string) {
    defaultModel = value
    pickerOpen = false
    formMessage = ''
  }

  async function readAddModels(tool: string) {
    if (!tool) return
    addLoading = true; addError = ''
    try {
      const answer = await allModels(tool, life.signal)
      if (life.signal.aborted) return
      addMachines = answer.machines ?? []
    } catch (e) {
      if (life.signal.aborted) return
      addMachines = []
      addError = "Couldn't read your machines' saved models just now. Favourites, suggestions and Other still work."
      fail(e)
    } finally {
      if (!life.signal.aborted) addLoading = false
    }
  }

  async function openAddPicker() {
    if (!canWrite || favBusy || !addTool) return
    addPickerOpen = true
    await readAddModels(addTool)
  }
  function addToolChanged() {
    addPickerOpen = false
    addMachines = []
    addError = ''
  }
  async function pickAddModel(value: string) {
    addPickerOpen = false
    const model = cleanModel(value)
    if (model) await addFavourite(addTool, model)
  }

  function toolChanged() {
    if (!takesEffort) defaultEffort = ''
    formMessage = ''
  }

  function validation(): string {
    const model = cleanModel(defaultModel)
    if (model !== '' && defaultTool === '') return 'Choose an agent before setting a model.'
    const badModel = validateModel(model)
    if (badModel) return badModel
    if (defaultEffort !== '' && (!defaultTool || !takesEffort)) return 'A thinking level needs an agent that takes one.'
    if (defaultEffort !== '' && !['low', 'medium', 'high'].includes(defaultEffort)) return "Choose low, medium or high thinking, or the agent's default."
    return ''
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!canWrite || busy) return
    const problem = validation()
    if (problem) {
      formMessage = problem
      notice = ''
      await tick()
      if (defaultTool === '') agentSelect?.focus()
      else modelButton?.focus()
      return
    }
    busy = 'profile'; formMessage = ''; notice = ''
    try {
      const answer = await saveProfile({
        defaultTool,
        defaultModel: cleanModel(defaultModel),
        defaultEffort,
        defaultBaseBranch: defaultBaseBranch.trim(),
        worktreesByDefault,
      })
      if (life.signal.aborted) return
      fillProfile(answer.profile)
      notice = 'Reet, defaults saved. New sessions without a project use them unless a project says otherwise.'
    } catch (err) {
      if (life.signal.aborted) return
      fail(err)
      formMessage = err instanceof ApiError && err.message ? err.message : "Couldn't save your defaults just now. Try again."
    } finally {
      if (!life.signal.aborted) busy = ''
    }
  }

  onMount(() => {
    const coarse = matchMedia('(pointer: coarse)')
    const readPointer = () => { touch = coarse.matches }
    readPointer()
    coarse.addEventListener?.('change', readPointer)
    void loadProfile()
    void loadFavourites()
    return () => { life.abort(); coarse.removeEventListener?.('change', readPointer) }
  })
</script>

<section class="block" aria-labelledby="agents-title">
  <h2 id="agents-title">Agents</h2>
  <p class="muted">Choose what new sessions start with when they don't belong to a project, and keep the models you reach for.</p>
  {#if !canWrite}<p class="muted readonly">Viewer accounts can read these defaults but can't change them.</p>{/if}

  {#if formMessage}<p class="notice bad" role="alert">{formMessage}</p>{/if}
  {#if notice}<p class="notice good" role="status">{notice}</p>{/if}

  <h3>New sessions without a project</h3>
  <form onsubmit={save} aria-busy={busy === 'profile'}>
    <div class="grid2">
      <label><span>Agent</span>
        <select id="agents-tool" aria-label="Default agent" bind:this={agentSelect} bind:value={defaultTool} onchange={toolChanged} disabled={!canWrite || loading || busy === 'profile'}>
          <option value="">The first available agent</option>
          {#each agents as agent (agent.id)}<option value={agent.id}>{agent.name}</option>{/each}
        </select>
        <small>Cline is offered here too. A model or thinking level only applies with its agent.</small>
      </label>

      <div class="field">
        <span>Model</span>
        <button id="agents-model" type="button" class="picker" bind:this={modelButton} disabled={!canWrite || !defaultTool || busy === 'profile'}
          aria-haspopup="dialog" aria-label={`Model: ${modelLabel}`} onclick={openModelPicker}>
          <span class="value">{modelLabel}</span><span class="chev" aria-hidden="true">⌄</span>
        </button>
        {#if !defaultTool}<small>Choose an agent before setting a model.</small>{:else}<small>Favourites and reet's suggestions. The agent decides whether a model exists.</small>{/if}
      </div>

      {#if takesEffort}
        <label><span>Thinking</span>
          <select id="agents-effort" aria-label="Thinking" bind:value={defaultEffort} disabled={!canWrite || busy === 'profile'}>
            {#each effortChoices(defaultTool) as value (value || 'default')}<option value={value}>{value || 'The agent’s default'}</option>{/each}
          </select>
          <small>Only agents that take an effort level show this.</small>
        </label>
      {/if}

      <label><span>Base branch for fresh worktrees</span>
        <input id="agents-branch" aria-label="Base branch for fresh worktrees" bind:value={defaultBaseBranch} placeholder="The repository's default" autocomplete="off" spellcheck="false" maxlength="255" disabled={!canWrite || busy === 'profile'} />
        <small>Used when a session starts in a new worktree. Empty follows the repository.</small>
      </label>

      <label><span>Worktrees by default</span>
        <select id="agents-worktrees" aria-label="Worktrees by default" bind:value={worktreesByDefault} disabled={!canWrite || busy === 'profile'}>
          <option value={false}>No — start in the folder itself</option>
          <option value={true}>Yes — start in a fresh worktree</option>
        </select>
        <small>Projects can override this.</small>
      </label>
    </div>
    <div class="formacts">
      <button type="submit" class="btn primary" disabled={!canWrite || loading || busy === 'profile'}>{busy === 'profile' ? 'Saving…' : 'Save defaults'}</button>
    </div>
  </form>

  <h3>Favourite models</h3>
  {#if favMessage}<p class="notice bad" role="alert">{favMessage}</p>{/if}
  {#if favNotice}<p class="notice good" role="status">{favNotice}</p>{/if}

  {#if favouritesLoading}
    <p class="muted">Reading your favourite models…</p>
  {:else if groups.length === 0}
    <p class="muted">No favourite models yet. Add one below, or star one from a model picker.</p>
  {:else}
    {#each groups as group (group.tool)}
      <section class="favgroup" aria-labelledby={`agents-fav-${group.tool}`}>
        <h4 id={`agents-fav-${group.tool}`}>{group.name}</h4>
        <ul class="favs">
          {#each group.items as item (favouriteKey(item.fav.tool, item.fav.model))}
            <li>
              <div class="favmain">
                <div class="favname">
                  <strong>{item.fav.model}</strong>
                  {#if item.fav.agentDefault}<span class="tag">Default for {group.name}</span>{/if}
                </div>
                <label class="default">
                  <input type="checkbox" checked={item.fav.agentDefault} disabled={!canWrite || !!favBusy}
                    aria-label={`Use ${item.fav.model} as the default when I pick ${group.name}`}
                    onchange={() => void setDefault(item.fav)} />
                  <span>Use as default when I pick this agent</span>
                </label>
              </div>
              <div class="favactions">
                <button type="button" class="btn quiet" disabled={!canWrite || !!favBusy || item.inGroup === 0}
                  aria-label={`Move ${item.fav.model} up for ${group.name}`} onclick={() => void moveFavourite(item.fav, -1)}>Move up</button>
                <button type="button" class="btn quiet" disabled={!canWrite || !!favBusy || item.inGroup === item.count - 1}
                  aria-label={`Move ${item.fav.model} down for ${group.name}`} onclick={() => void moveFavourite(item.fav, 1)}>Move down</button>
                <button type="button" class="btn quiet danger" disabled={!canWrite || !!favBusy}
                  aria-label={`Remove ${item.fav.model} for ${group.name}`} onclick={() => void removeFavourite(item.fav)}>Remove</button>
              </div>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  {/if}

  <div class="add">
    <h4>Add a model</h4>
    <div class="addrow">
      <label><span>Agent for the new favourite</span>
        <select id="agents-add-tool" aria-label="Agent for the new favourite" bind:value={addTool} onchange={addToolChanged} disabled={!canWrite || !!favBusy}>
          {#each agents as agent (agent.id)}<option value={agent.id}>{agent.name}</option>{/each}
        </select>
      </label>
      <button id="agents-add-model" type="button" class="picker addpicker" bind:this={addButton} disabled={!canWrite || !addTool || !!favBusy}
        aria-haspopup="dialog" aria-label={`Add a model for ${addName}`} onclick={() => void openAddPicker()}>
        <span class="value">Choose a model</span><span class="chev" aria-hidden="true">⌄</span>
      </button>
    </div>
    <p class="muted small">The picker reads the saved lists from all your machines, then reet's suggestions; Other takes any name the agent accepts.</p>
  </div>
</section>

{#if pickerOpen}
  <SlotPicker label="Model" options={optionsForDefault()} value={defaultModel} sheet={touch} opener={modelButton}
    filter="Filter models"
    status="Favourites and reet's suggestions; the agent decides whether a model exists."
    other={{ label: 'Other…', field: 'Model name', validate: validateModel }}
    onpick={pickModel} onclose={() => { pickerOpen = false }} />
{/if}

{#if addPickerOpen}
  <SlotPicker label="Add a favourite model" options={optionsForAdd()} value="" sheet={touch} opener={addButton}
    filter="Filter models" status={addStatus}
    other={{ label: 'Other…', field: 'Model name', validate: validateModel }}
    onpick={(value) => void pickAddModel(value)} onclose={() => { addPickerOpen = false }} />
{/if}

<style>
  .block { margin-top: 24px; padding: clamp(18px, 3vw, 28px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); min-width: 0; }
  h2 { margin: 0; font: 500 1.35rem/1.25 var(--serif); }
  h3 { margin: 26px 0 0; font: 600 1.02rem/1.3 var(--sans); }
  h4 { margin: 0 0 8px; font: 600 .9rem/1.3 var(--sans); color: var(--ink); }
  .muted { margin: 8px 0 0; color: var(--muted); font-size: .86rem; line-height: 1.5; }
  .muted.small { font-size: .78rem; }
  .readonly { color: var(--ink-soft); }
  .notice { margin: 12px 0 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); font-size: .86rem; }
  .notice.bad { border-color: var(--bad); }
  .notice.good { border-color: var(--good); }
  form { display: grid; grid-template-columns: minmax(0, 1fr); gap: 14px; margin-top: 12px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; min-width: 0; }
  .grid2 label, .field, .addrow label { display: grid; gap: 4px; font-size: .84rem; color: var(--muted); min-width: 0; }
  .grid2 small { color: var(--muted); font-size: .76rem; line-height: 1.45; }
  select, input { min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
  select:disabled, input:disabled { opacity: .6; }
  .picker { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); text-align: left; background: var(--bg); border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
  .picker:disabled { opacity: .55; cursor: default; }
  .picker .value { min-width: 0; overflow-wrap: anywhere; }
  .picker .chev { flex: none; color: var(--muted); }
  .formacts { display: flex; justify-content: flex-end; }
  .favgroup { margin-top: 16px; min-width: 0; }
  .favs { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .favs li { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px 14px; flex-wrap: wrap; padding: 10px 0; border-bottom: 1px solid var(--line); min-width: 0; }
  .favmain { display: grid; gap: 6px; min-width: 0; flex: 1 1 240px; }
  .favname { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; min-width: 0; }
  .favname strong { overflow-wrap: anywhere; }
  .default { display: flex; align-items: center; gap: 8px; color: var(--ink-soft); font-size: .8rem; line-height: 1.4; }
  .default input { flex: none; width: auto; min-height: 0; }
  .favactions { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .tag { padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: .65rem; }
  .add { margin-top: 22px; padding-top: 16px; border-top: 1px solid var(--line); min-width: 0; }
  .addrow { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; align-items: end; }
  .addpicker { align-self: end; }
  /* On a phone a field under 16 px makes the browser zoom in when it is focused. */
  @media (max-width: 650px) { select, input { font-size: 16px; } }
  @media (max-width: 560px) {
    .grid2, .addrow { grid-template-columns: 1fr; }
    .formacts { justify-content: stretch; }
    .formacts button { width: 100%; }
  }
  /* Touch targets are thumb-sized: 44 px for each control. */
  @media (pointer: coarse) {
    select, input, .picker, button { min-height: 44px; }
    .favactions button { min-width: 44px; }
    .default { min-height: 44px; }
    .default input { width: 22px; height: 22px; }
  }
</style>
