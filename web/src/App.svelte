<script lang="ts">
  import { groupLabel } from './lib/machines'
  import Community from './lib/Community.svelte'
  import SharedLists from './lib/SharedLists.svelte'
  import Spaces from './lib/Spaces.svelte'
  import { audienceLabel } from './lib/api'
  import { onMount, tick } from 'svelte'
  import {
    addSkill, addSkillTags, clearToken, getAuthStatus, getToken, listEnvironments, listEvents, listLists, listSkills, listTags,
    patchSkill, patchSkills, refreshSkill, removeSkill, removeSkills, resolveSkill, restoreAll, signOutAccount, tagSkills, UnauthorizedError,
    type Counts, type Environment, type Event, type Flags, type ListRef, type SharedList, type Skill, type Sort, type Tag, type User,
  } from './lib/api'
  import Account from './lib/Account.svelte'
  import AccountSignIn from './lib/AccountSignIn.svelte'
  import Admin from './lib/Admin.svelte'
  import Bookmarklets from './lib/Bookmarklets.svelte'
  import BulkBar from './lib/BulkBar.svelte'
  import ConfirmDialog from './lib/ConfirmDialog.svelte'
  import CopyButton from './lib/CopyButton.svelte'
  import Footer from './lib/Footer.svelte'
  import Join from './lib/Join.svelte'
  import ListView from './lib/ListView.svelte'
  import Logo from './lib/Logo.svelte'
  import NewList from './lib/NewList.svelte'
  import PublicList from './lib/PublicList.svelte'
  import PageHeader from './lib/PageHeader.svelte'
  import Setup from './lib/Setup.svelte'
  import SignIn from './lib/SignIn.svelte'
  import SkillCard from './lib/SkillCard.svelte'
  import Toast from './lib/Toast.svelte'
  import Tools from './lib/Tools.svelte'
  import { copyText } from './lib/clipboard'
  import { installCommand } from './lib/install'
  import { stageOf, stages, stageTitle, type Stage } from './lib/shelf'
  import { isOldEnvironment } from './lib/time'

  let skills = $state<Skill[]>([])
  let counts = $state<Counts>({ all: 0, liked: 0, bookmarked: 0, used: 0 })
  let show = $state<'all' | Stage>('all')
  let onlyLiked = $state(false)
  let q = $state('')
  let sort = $state<Sort>('recent')
  let addInput = $state('')
  let message = $state('')
  let error = $state('')
  let loading = $state(true)
  let needsAuth = $state(false)
  let token = $state(getToken())
  let authMode = $state<'accounts' | 'token' | 'none' | ''>('')
  let authChecked = $state(false)
  let setupRequired = $state(false)
  let bootstrapRequired = $state(false)
  let currentUser = $state<User | null>(null)
  let search = $state<HTMLInputElement>()
  let addEl = $state<HTMLInputElement>()
  type ToastState = { id: number; message: string; actionLabel?: string; onaction?: () => void | Promise<void> }
  let toast = $state<ToastState | null>(null)
  let toastId = 0
  let environments = $state<Environment[]>([])
  let allTags = $state<Tag[]>([])
  let lists = $state<SharedList[]>([])
  let linkEnabled = $state(true)
  let newListOpen = $state(false)
  let selectedTags = $state<number[]>([])
  let selectedEnv = $state('')
  let selectedMachine = $state('')
  let machines = $state<import('./lib/api').Machine[]>([])
  let onlyOne = $state(false)
  let copying = $state(false)
  let removePending = $state<Skill | null>(null)
  let metaLoaded = $state(false)
  let selecting = $state(false)
  let selected = $state<string[]>([])
  let lastPicked = $state<number | null>(null)
  let bulkBusy = $state(false)
  let bulkRemoveOpen = $state(false)
  let bulkRemoving = $state<Skill[]>([])

  const origin = location.origin
  const isCommunity = location.pathname === '/community'
  const isShared = location.pathname === '/shared'
  const isSpaces = location.pathname === '/spaces'
  const isAdmin = location.pathname.startsWith('/admin')
  const isAccount = location.pathname.startsWith('/account')
  const isJoin = location.pathname.startsWith('/join')
  const listMatch = location.pathname.match(/^\/lists\/(\d+)\/?$/)
  const isList = listMatch !== null
  const listId = listMatch ? Number(listMatch[1]) : 0
  const publicMatch = location.pathname.match(/^\/l\/([^/]+)\/?$/)
  const isPublicList = publicMatch !== null
  const publicSlug = publicMatch ? decodeURIComponent(publicMatch[1]) : ''
  const joinToken = new URLSearchParams(location.search).get('token') ?? ''
  const signedIn = $derived(token !== '')
  const authenticated = $derived(authMode === 'none' || (authMode === 'token' && token !== '') || (authMode === 'accounts' && currentUser !== null))
  const canLoad = $derived(authChecked && authenticated && !needsAuth && !isCommunity && !isPublicList)
  const role = $derived(currentUser?.role ?? (authMode === 'accounts' ? 'viewer' : 'owner'))
  const canWrite = $derived(authMode !== 'accounts' || role !== 'viewer')
  const isAdminUser = $derived(authMode !== 'accounts' || role === 'owner' || role === 'admin')
  const scanCommand = $derived(`reet scan --prune --server ${origin}${authMode === 'none' ? '' : ' --token "$REET_TOKEN"'}`)

  const filtering = $derived(q.trim() !== '' || onlyLiked || selectedTags.length > 0 || selectedEnv !== '' || selectedMachine !== '' || onlyOne)
  const grouped = $derived(stages.map((st) => ({ ...st, items: skills.filter((k) => stageOf(k) === st.id) })))
  // While searching or filtering, hide sections with nothing to show; otherwise keep them so the shelf reads as a pipeline.
  const visible = $derived(grouped.filter((g) => (show === 'all' || show === g.id) && (g.items.length > 0 || !filtering)))
  const total = $derived(visible.reduce((n, g) => n + g.items.length, 0))
  const likedCount = $derived(skills.filter((s) => s.liked).length)
  const tagOptions = $derived(allTags.map((t) => t.name))
  const filterKey = $derived(`${q}|${sort}|${onlyLiked}|${selectedTags.join(',')}|${selectedMachine}|${selectedEnv}|${onlyOne}|${show}`)
  const tagCounts = $derived.by(() => {
    const map = new Map<number, { tag: Tag; count: number }>()
    for (const skill of skills) {
      for (const tag of skill.tags ?? []) {
        const current = map.get(tag.id)
        if (current) current.count++
        else map.set(tag.id, { tag, count: 1 })
      }
    }
    for (const id of selectedTags) {
      if (!map.has(id)) {
        const tag = allTags.find((t) => t.id === id)
        if (tag) map.set(id, { tag, count: 0 })
      }
    }
    return [...map.values()].sort((a, b) => a.tag.name.localeCompare(b.tag.name))
  })

  let clearTimer: ReturnType<typeof setTimeout> | undefined
  function fail(e: unknown) {
    if (e instanceof UnauthorizedError) {
      clearToken()
      token = ''
      if (authMode === 'accounts') currentUser = null
      needsAuth = true
      message = ''
      toast = null
      removePending = null
      return
    }
    error = e instanceof Error ? e.message : String(e)
    message = ''
  }
  function ok(m: string) {
    message = m
    error = ''
    clearTimeout(clearTimer)
    clearTimer = setTimeout(() => (message = ''), 6000)
  }
  function showToast(message: string, actionLabel?: string, onaction?: () => void | Promise<void>) {
    toast = { id: ++toastId, message, actionLabel, onaction }
  }
  function dismissToast() {
    toast = null
  }

  let loadVersion = 0

  async function load(): Promise<boolean> {
    const version = ++loadVersion
    try {
      const r = await listSkills({
        tab: onlyLiked ? 'liked' : 'all',
        q,
        sort,
        tags: selectedTags,
        env: selectedEnv, machine: selectedMachine,
        only: onlyOne ? 'one-machine' : '',
      })
      if (version !== loadVersion) return false
      skills = r.skills
      counts = r.counts
      error = ''
      needsAuth = false
      if (!metaLoaded) void loadMeta().then((ok) => (metaLoaded = ok))
      return true
    } catch (e) {
      if (version !== loadVersion) return false
      fail(e)
      return false
    } finally {
      if (version === loadVersion) loading = false
    }
  }

  async function loadMeta(): Promise<boolean> {
    try {
      const [envs, tags, shared] = await Promise.all([listEnvironments(), listTags(), listLists()])
      environments = envs.environments
      machines = envs.machines
      allTags = tags.tags
      lists = shared.lists
      linkEnabled = shared.linkEnabled
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  function openList(list: SharedList | ListRef) {
    location.assign(`/lists/${list.id}`)
  }

  async function reloadAll() {
    await load()
    metaLoaded = await loadMeta()
  }

  onMount(() => {
    void initAuth()
  })

  async function initAuth() {
    try {
      const status = await getAuthStatus()
      authMode = status.mode
      setupRequired = status.setupRequired
      bootstrapRequired = status.bootstrapRequired
      currentUser = status.user
      if (status.mode === 'token') {
        token = getToken()
        needsAuth = token === ''
      } else {
        clearToken()
        token = ''
        needsAuth = status.mode === 'accounts' ? !status.user : false
      }
    } catch {
      authMode = ''
    } finally {
      authChecked = true
    }
  }

  async function afterAuth(user: User) {
    authMode = 'accounts'
    currentUser = user
    setupRequired = false
    needsAuth = false
    token = ''
    clearToken()
    loading = true
    await load()
    await loadMeta()
    await tick()
    addEl?.focus()
  }

  async function afterJoin(user: User) {
    await afterAuth(user)
    location.assign('/')
  }

  async function attemptSignIn(): Promise<string | null> {
    loading = true
    const accepted = await load()
    if (accepted) {
      token = getToken()
      message = ''
      await tick()
      addEl?.focus()
      return null
    }
    if (needsAuth) return 'That token was not accepted.'
    return error || 'Could not reach the shelf.'
  }

  async function signOut() {
    if (authMode === 'accounts') {
      try {
        await signOutAccount()
      } catch {
        /* the local session is cleared even if the server call cannot be made */
      }
      currentUser = null
      needsAuth = true
      token = ''
      clearToken()
      message = ''
      error = ''
      toast = null
      removePending = null
      selecting = false
      selected = []
      lastPicked = null
      return
    }
    clearToken()
    token = ''
    message = ''
    error = ''
    toast = null
    removePending = null
    loading = true
    await load()
  }

  $effect(() => {
    void [onlyLiked, q, sort, selectedTags, selectedEnv, selectedMachine, onlyOne, canLoad]
    if (!canLoad) return
    const t = setTimeout(load, 150)
    return () => clearTimeout(t)
  })


  let lastFilterKey = ''
  $effect(() => {
    const key = filterKey
    if (key !== lastFilterKey) {
      lastFilterKey = key
      if (selecting) {
        selected = []
        lastPicked = null
      }
    }
  })

  // A moved skill is re-rendered in another section, so put keyboard focus back on the control that was used.
  async function refocus(id: string, act: string) {
    await tick()
    const card = document.querySelector(`[data-skill="${CSS.escape(id)}"]`)
    const el = card?.querySelector(`[data-act="${act}"]`) ?? card?.querySelector('[data-act="m0"]')
    ;(el as HTMLElement | null)?.focus()
  }

  async function flash(id: string) {
    await tick()
    const card = document.querySelector(`[data-skill="${CSS.escape(id)}"]`)
    if (!card) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    card.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'center' })
    card.classList.remove('flash')
    void (card as HTMLElement).offsetWidth
    card.classList.add('flash')
    setTimeout(() => card.classList.remove('flash'), 1600)
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (!addInput.trim()) return
    try {
      const s = await addSkill(addInput.trim())
      addInput = ''
      show = 'all'
      onlyLiked = false
      q = ''
      ok(s.created ? `Reet, added ${s.name} to the shelf.` : `Already on the shelf, in ${stageTitle(stageOf(s))}.`)
      await reloadAll()
      if (!s.created) await flash(s.id)
      setTimeout(load, 2500) // pick up fetched metadata
    } catch (e) {
      fail(e)
    }
  }
  async function patch(s: Skill, flags: Flags, act: string): Promise<boolean> {
    try {
      const updated = await patchSkill(s.id, flags)
      const to = stageOf(updated)
      if (to !== stageOf(s)) ok(`Moved ${s.name} to ${stageTitle(to)}.`)
      await reloadAll()
      await refocus(s.id, act)
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
  async function refreshOne(s: Skill) {
    try {
      const r = await refreshSkill(s.id)
      if (!r.refreshed) fail(new Error(`Could not refresh ${s.name}.`))
      else if (!r.changed) ok(`No change for ${s.name}.`)
      else ok(`Refreshed ${s.name}.`)
      await reloadAll()
    } catch (e) {
      fail(e)
    }
  }
  function envTitle(env: Environment): string {
    const when = new Date(env.lastSeen).toLocaleString()
    return `${env.path ? env.path + ' · ' : ''}last seen ${when}`
  }

  function toggleSelecting() {
    selecting = !selecting
    if (!selecting) {
      selected = []
      lastPicked = null
    }
  }

  function select(skill: Skill, e: MouseEvent) {
    const flat = visible.flatMap((g) => g.items)
    const index = flat.findIndex((s) => s.id === skill.id)
    if (index < 0) return
    if (e.shiftKey && lastPicked !== null) {
      const [from, to] = [lastPicked, index].sort((a, b) => a - b)
      const range = flat.slice(from, to + 1).map((s) => s.id)
      const all = range.every((id) => selected.includes(id))
      selected = all ? selected.filter((id) => !range.includes(id)) : [...new Set([...selected, ...range])]
    } else {
      selected = selected.includes(skill.id) ? selected.filter((id) => id !== skill.id) : [...selected, skill.id]
    }
    lastPicked = index
  }

  async function bulkPatch(flags: Flags) {
    if (!selected.length) return
    bulkBusy = true
    try {
      await patchSkills(selected, flags)
      selecting = false
      selected = []
      lastPicked = null
      await reloadAll()
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function bulkStage(stage: 'use' | 'try' | 'shelf') {
    void bulkPatch(stage === 'use' ? { used: true } : stage === 'try' ? { used: false, bookmarked: true } : { used: false, bookmarked: false })
  }

  async function bulkTag(name: string) {
    if (!selected.length) return
    bulkBusy = true
    try {
      await tagSkills(selected, [name], [])
      selecting = false
      selected = []
      lastPicked = null
      await reloadAll()
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function bulkRemove() {
    if (!selected.length) return
    bulkRemoving = skills.filter((s) => selected.includes(s.id))
    bulkRemoveOpen = true
  }

  async function confirmBulkRemove() {
    const removed = bulkRemoving
    bulkRemoveOpen = false
    if (!removed.length) return
    bulkBusy = true
    try {
      await removeSkills(removed.map((s) => s.id))
      selecting = false
      selected = []
      lastPicked = null
      bulkRemoving = []
      await reloadAll()
      showToast(removed.length === 1 ? `Removed ${removed[0].name}` : `Removed ${removed.length} skills`, 'Undo', async () => {
        try {
          await restoreAll({ version: 2, exportedAt: Date.now(), skills: removed })
          await reloadAll()
          ok(removed.length === 1 ? `Restored ${removed[0].name}.` : `Restored ${removed.length} skills.`)
        } catch (e) {
          fail(e)
        }
      })
    } catch (e) {
      fail(e)
    } finally {
      bulkBusy = false
    }
  }

  function toggleTag(id: number) {
    selectedTags = selectedTags.includes(id) ? selectedTags.filter((x) => x !== id) : [...selectedTags, id]
  }
  function toggleEnv(id: string) {
    selectedEnv = selectedEnv === id ? '' : id
  }
  function filterTag(tag: Tag) {
    toggleTag(tag.id)
  }
  function filterEnv(env: Environment) {
    selectedMachine = env.host; toggleEnv(env.id)
  }

  async function copyMissing() {
    if (!selectedEnv && !selectedMachine) return
    copying = true
    try {
      const all = await listSkills({ tab: 'all', q: '', sort: 'recent' })
      const missing = all.skills.filter((s) => {
        const command = installCommand(s)
        return command !== null && !(s.environments ?? []).some((e) => (selectedEnv ? e.id === selectedEnv : e.host === selectedMachine))
      })
      const text = missing.map((s) => installCommand(s)).filter((x): x is string => Boolean(x)).join('\n')
      if (!text) {
        ok('Everything on the shelf is already installed there.')
        return
      }
      if (await copyText(text)) {
        ok(`Copied ${missing.length} install ${missing.length === 1 ? 'command' : 'commands'}.`)
      } else {
        fail(new Error('Could not copy to the clipboard.'))
      }
    } catch (e) {
      fail(e)
    } finally {
      copying = false
    }
  }

  async function tags(s: Skill, add: string[], remove: number[]): Promise<boolean> {
    try {
      const updated = await addSkillTags(s.id, add, remove)
      // Older list requests must not overwrite the tag mutation response.
      ++loadVersion
      skills = skills.map((skill) => skill.id === updated.id ? updated : skill)
      await reloadAll()
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }

  async function history(s: Skill): Promise<Event[]> {
    try {
      return (await listEvents(20, s.id)).events
    } catch (e) {
      fail(e)
      return []
    }
  }

  function remove(s: Skill) {
    if (s.sourceType === 'local' || (s.environments?.length ?? 0) > 0) {
      removePending = s
      return
    }
    void doRemove(s, false)
  }

  async function doRemove(s: Skill, ignore: boolean) {
    try {
      await removeSkill(s.id, ignore)
      await reloadAll()
      search?.focus()
      showToast(ignore ? `Removed ${s.name} and ignored future scans` : `Removed ${s.name}`, 'Undo', async () => {
        try {
          await restoreAll({ version: 2, exportedAt: Date.now(), skills: [s] })
          await reloadAll()
          ok(`Restored ${s.name}.`)
        } catch (e) {
          fail(e)
        }
      })
    } catch (e) {
      fail(e)
    }
  }
  async function link(s: Skill, input: string): Promise<boolean> {
    try {
      const r = await resolveSkill(s.id, input)
      ok(`Linked to ${r.id}`)
      await reloadAll()
      return true
    } catch (e) {
      fail(e)
      return false
    }
  }
</script>

<svelte:head>
  {#if !isPublicList}
    <title>{isCommunity ? 'Public community' : isShared ? 'Your lists' : isSpaces ? 'Teams & organisations' : isJoin ? 'Join reet.sh' : authMode === 'accounts' && setupRequired ? 'Set up reet.sh' : authChecked && !authenticated ? 'Sign in' : isAdmin ? 'Admin' : isAccount ? 'Your account' : isList ? 'Shared list' : 'Your shelf'} · reet.sh</title>
  {/if}
</svelte:head>

<a class="skip" href="#main-content">Skip to content</a>
<header class="site-header">
  <div class="header-inner">
    <div>
      <a class="home" href="/" aria-label="reet.sh home"><Logo /></a>
    </div>
    {#if authenticated && !needsAuth && !isPublicList && !isJoin}
      <nav class="site-nav" aria-label="Main navigation">
        <a href="/" aria-current={!isAdmin && !isAccount && !isList && !isCommunity && !isShared && !isSpaces ? 'page' : undefined}>Personal shelf</a>
        <a href="/shared" aria-current={isShared ? 'page' : undefined}>Lists</a>
        <a href="/spaces" aria-current={isSpaces ? 'page' : undefined}>Teams</a>
        <a href="/community" aria-current={isCommunity ? 'page' : undefined}>Community</a>
        {#if authMode === 'accounts'}<a href="/account" aria-current={isAccount ? 'page' : undefined}>Account</a>{/if}
        {#if isAdminUser}<a href="/admin" aria-current={isAdmin ? 'page' : undefined}>Admin</a>{/if}
      </nav>
    {:else if isPublicList || isCommunity || isJoin}
      <nav class="site-nav" aria-label="Public navigation"><a href="/community">Public community</a><a class="entry-link" href="/">Open your shelf →</a></nav>
    {/if}
    {#if (!authenticated || needsAuth) && !isPublicList && !isCommunity && !isJoin}<span class="tag">Your shelf for AI skills</span>{/if}
  </div>
</header>
<main id="main-content" tabindex="-1" class:auth-page={!isPublicList && !isCommunity && (!authenticated || needsAuth || isJoin)}>
  {#if isCommunity}
    <Community canSave={authenticated && !needsAuth && canWrite} />
  {:else if isPublicList}
    <PublicList slug={publicSlug} />
  {:else if isJoin}
    <Join token={joinToken} onsuccess={afterJoin} />
  {:else if !authChecked}
    <p class="empty" role="status">Loading…</p>
  {:else if authMode === 'accounts' && setupRequired}
    <Setup {bootstrapRequired} onsuccess={afterAuth} />
  {:else if authMode === 'accounts' && !currentUser}
    <AccountSignIn onsuccess={afterAuth} />
  {:else if authMode === 'token' && needsAuth}
    <SignIn onattempt={attemptSignIn} />
  {:else if !authMode}
    <PageHeader eyebrow="Connection problem" title="The shelf is out of reach." />
    <p class="notice bad" role="alert">Could not check how this instance signs in. Reload the page to try again.</p>
  {:else if isShared}
    <SharedLists userId={currentUser?.id ?? 0} {canWrite} />
  {:else if isSpaces}
    <Spaces {canWrite} />
  {:else if isAccount && authMode === 'accounts'}
    <Account user={currentUser!} onsignout={signOut} onerror={fail} />
  {:else if isList}
    <ListView id={listId} {canWrite} userRole={role} onerror={fail} />
  {:else if isAccount}
    <PageHeader eyebrow="Your account" title="Accounts are not enabled." />
    <p class="notice bad" role="alert">Accounts are not enabled on this instance.</p>
  {:else if isAdmin && !isAdminUser}
    <PageHeader eyebrow="Instance administration" title="Admin access is needed." />
    <p class="notice bad" role="alert">Admins only. Your account does not have access to this page.</p>
  {:else if isAdmin}
    <Admin onerror={fail} {role} accounts={authMode === 'accounts'} currentEmail={currentUser?.email ?? ''} />
  {:else}
    <PageHeader eyebrow="Your personal library" title="Your shelf. Your toolkit." lede="The good finds, the next experiments, and the skills you use every day.">
      {#snippet actions()}<span class="badge">{counts.all} {counts.all === 1 ? 'skill' : 'skills'} · on this instance</span>{/snippet}
    </PageHeader>
    {#if canWrite}
      <form class="add" onsubmit={add}>
        <label class="vh" for="add-input">Skill URL or slug</label>
        <input id="add-input" bind:this={addEl} placeholder="Paste a skills.sh URL or owner/repo/skill" autocomplete="off" bind:value={addInput} />
        <button class="btn primary" type="submit">Add to shelf</button>
      </form>
    {/if}

    <div class="notices">
      {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
      <p class="notice good" role="status" hidden={!message}>{message}</p>
    </div>

    {#if counts.all > 0 || filtering}
      <div class="toolbar" role="group" aria-label="Filter and sort">
        <div class="filters">
          <button type="button" aria-pressed={show === 'all'} onclick={() => (show = 'all')}>All<small>{skills.length}</small></button>
          {#each grouped as g}
            <button type="button" aria-pressed={show === g.id} onclick={() => (show = g.id)}>{g.title}<small>{g.items.length}</small></button>
          {/each}
          <span class="sep" aria-hidden="true"></span>
          <button type="button" class="liked" aria-pressed={onlyLiked} onclick={() => (onlyLiked = !onlyLiked)}>♥ Liked<small>{likedCount}</small></button>
          {#if canWrite}
            <span class="sep" aria-hidden="true"></span>
            <button type="button" aria-pressed={selecting} onclick={toggleSelecting}>Select</button>
          {/if}
        </div>
        <div class="controls">
          <label class="vh" for="search">Search skills</label>
          <input id="search" type="search" placeholder="Search" bind:value={q} bind:this={search} />
          <label class="vh" for="sort">Sort by</label>
          <span class="select">
            <select id="sort" bind:value={sort}>
              <option value="recent">Recent</option>
              <option value="name">Name</option>
              <option value="installs">Installs</option>
            </select>
          </span>
        </div>
      </div>
    {/if}

    {#if lists.length > 0 || canWrite}
      <div class="filterbar" role="group" aria-label="Shared lists">
        <span class="bar-label">Your lists</span>
        <a class="chip" href="/shared">Browse & manage lists</a>
        <a class="chip" href="/community">Public community ↗</a>
        {#each lists.filter(l => l.ownerId === currentUser?.id || (authMode !== 'accounts' && l.role === 'owner')) as list (list.id)}
          <button type="button" class="chip" onclick={() => openList(list)} title={`${list.skillCount} skills · ${list.visibility}`}>
            {list.name}<small>{audienceLabel(list.visibility)} · {list.skillCount}</small>
          </button>
        {/each}
        {#if canWrite}
          <button type="button" class="chip action" onclick={() => (newListOpen = true)}>+ New list</button>
        {/if}
      </div>
    {/if}

    {#if tagCounts.length > 0 || selectedTags.length > 0}
      <div class="filterbar" role="group" aria-label="Tags">
        <span class="bar-label">Tags</span>
        {#each tagCounts as item (item.tag.id)}
          <button type="button" class="chip" aria-pressed={selectedTags.includes(item.tag.id)} onclick={() => toggleTag(item.tag.id)}>
            {item.tag.name}<small>{item.count}</small>
          </button>
        {/each}
        {#if selectedTags.length > 0}
          <button type="button" class="chip clear" onclick={() => (selectedTags = [])}>Clear</button>
        {/if}
      </div>
    {/if}

    {#if environments.length > 0 || selectedEnv !== '' || selectedMachine !== '' || onlyOne}
      <div class="filterbar" role="group" aria-label="Environments">
        <span class="bar-label">Environments</span>
        {#each machines as machine (machine.host)}
          <button type="button" class="chip" aria-pressed={selectedMachine === machine.host}
            onclick={() => { selectedMachine = selectedMachine === machine.host ? '' : machine.host; selectedEnv = '' }}
            title={`${machine.groups.filter(g => !g.agent.startsWith('reet')).length} agent groups · ${machine.skillCount} unique skills`}>
            {machine.host}<small>{machine.skillCount}</small>
          </button>
        {/each}
        <span class="sep" aria-hidden="true"></span>
        <button type="button" class="chip" aria-pressed={onlyOne} onclick={() => (onlyOne = !onlyOne)}>Only on one machine</button>
        {#if selectedMachine}
          <button type="button" class="chip action" onclick={copyMissing} disabled={copying}>
            {copying ? 'Copying…' : 'Copy missing installs'}
          </button>
        {/if}
      </div>
      {#if selectedMachine}
        <div class="filterbar" role="group" aria-label="Agent groups on selected machine">
          <span class="bar-label">Agent groups</span>
          <button type="button" class="chip" aria-pressed={!selectedEnv} onclick={() => selectedEnv = ''}>All agents</button>
          {#each environments.filter(e => e.host === selectedMachine && !e.agent.startsWith('reet')) as env (env.id)}
            <button type="button" class="chip" class:old={isOldEnvironment(env.lastSeen)} aria-pressed={selectedEnv === env.id} onclick={() => toggleEnv(env.id)} title={envTitle(env)}>{groupLabel(env)}<small>{env.skillCount ?? 0}</small></button>
          {/each}
        </div>
      {/if}
    {/if}

    {#if !loading && (counts.all > 0 || filtering)}<p class="count" role="status">{total === 1 ? '1 skill' : `${total} skills`}</p>{/if}

    <div id="sections">
      {#if loading}
        <p class="empty" role="status">Loading…</p>
      {:else if counts.all === 0 && !filtering && !canWrite}
        <section class="start" aria-labelledby="start-title">
          <p class="eyebrow">Read-only</p>
          <h2 id="start-title">Your account can browse, but not edit the shelf.</h2>
          <p class="lede">Ask the owner for a shared list or a member role.</p>
        </section>
      {:else if counts.all === 0 && !filtering}
        <section class="start" aria-labelledby="start-title">
          <p class="eyebrow">Start here</p>
          <h2 id="start-title">Put the first skill on your shelf.</h2>
          <p class="lede">Capture it now, sort it later. Everything stays on this instance.</p>
          <ol class="steps">
            <li>
              <span class="num" aria-hidden="true">1</span>
              <div>
                <h3>Paste a link</h3>
                <p>Drop a skills.sh URL or <code>owner/repo/skill</code> into the box above.</p>
              </div>
            </li>
            <li>
              <span class="num" aria-hidden="true">2</span>
              <div>
                <h3>Drag a bookmarklet</h3>
                <p>Put Like, To test or In use on your bookmarks bar. Click one on any skills.sh page.</p>
                <Bookmarklets {token} />
              </div>
            </li>
            <li>
              <span class="num" aria-hidden="true">3</span>
              <div>
                <h3>Scan a machine</h3>
                <p>Run this where your skills live. It marks what is installed as in use.</p>
                <div class="command">
                  <code>{scanCommand}</code>
                  <CopyButton text={scanCommand} compact />
                </div>
              </div>
            </li>
          </ol>
          <p class="browse"><a href="https://www.skills.sh/topic" target="_blank" rel="noopener noreferrer">Browse skills.sh topics<span class="vh"> (opens in a new tab)</span></a></p>
        </section>
      {:else if visible.length === 0}
        <div class="no-matches">
          <h2>No skills match that.</h2>
          <p>Try another search or clear your filters.</p>
          <button type="button" class="btn" onclick={() => { q = ''; show = 'all'; onlyLiked = false; selectedTags = []; selectedEnv = ''; selectedMachine = ''; onlyOne = false }}>Clear filters</button>
        </div>
      {:else}
        {#each visible as g (g.id)}
          <section class={g.id} aria-labelledby="h-{g.id}">
            <h2 id="h-{g.id}">{g.title}<small>{g.items.length}</small></h2>
            <p class="hint">{g.hint}</p>
            <ul>
              {#each g.items as s (s.id)}
                <SkillCard
                skill={s} stage={g.id} tagOptions={tagOptions}
                selecting={selecting} selectedIDs={selected} onselect={select} {canWrite}
                onpatch={patch} onrefresh={refreshOne} onlink={link} onremove={remove}
                onfiltertag={filterTag} onfilterenv={filterEnv} ontags={tags} onhistory={history}
                onopenlist={openList}
              />
              {:else}
                <li class="none">{g.empty}</li>
              {/each}
            </ul>
          </section>
        {/each}
      {/if}
    </div>

    {#if !loading && canWrite}
      {#key counts.all === 0}
        <Tools
          onchange={reloadAll} {ok} {fail} {token} canManageTags={isAdminUser}
          filters={{ tab: onlyLiked ? 'liked' : 'all', q, sort, tags: selectedTags, env: selectedEnv, machine: selectedMachine, only: onlyOne ? 'one-machine' : '', stage: show === 'all' ? '' : show }}
          startOpen={counts.all === 0} hideBookmarklets={counts.all === 0}
        />
      {/key}
    {/if}
  {/if}
</main>
<Footer
  signedIn={!isPublicList && authMode !== 'none' && authenticated && !needsAuth}
  showAdmin={!isPublicList && authenticated && !needsAuth && isAdminUser}
  userName={currentUser?.name ?? ''}
  onsignout={signOut}
/>
{#if newListOpen}
  <NewList
    {linkEnabled}
    oncreated={(list) => { newListOpen = false; location.assign(`/lists/${list.id}`) }}
    oncancel={() => (newListOpen = false)}
  />
{/if}

{#if removePending}
  <ConfirmDialog
    title={`Remove ${removePending.name}?`}
    body="This skill is on your scanner's list. It will come back on the next scan unless you ignore it."
    confirmLabel="Remove"
    secondaryLabel="Remove and ignore"
    danger
    onconfirm={() => { const skill = removePending!; removePending = null; void doRemove(skill, false) }}
    onsecondary={() => { const skill = removePending!; removePending = null; void doRemove(skill, true) }}
    oncancel={() => (removePending = null)}
  />
{/if}

{#if bulkRemoveOpen}
  <ConfirmDialog
    title={`Remove ${bulkRemoving.length} ${bulkRemoving.length === 1 ? 'skill' : 'skills'}?`}
    body="They will be removed from the shelf. Use Undo in the toast if you change your mind."
    confirmLabel="Remove"
    danger
    onconfirm={confirmBulkRemove}
    oncancel={() => (bulkRemoveOpen = false)}
  />
{/if}

{#if selecting && !needsAuth}
  <BulkBar
    count={selected.length} {tagOptions} busy={bulkBusy}
    onclear={toggleSelecting} ontag={bulkTag} onstage={bulkStage}
    onlike={() => bulkPatch({ liked: true })} onremove={bulkRemove}
  />
{/if}

{#if toast && !needsAuth}
  {#key toast.id}
    <Toast
      message={toast.message}
      actionLabel={toast.actionLabel}
      onaction={toast.onaction}
      ondismiss={dismissToast}
    />
  {/key}
{/if}

<style>
  .skip { position: absolute; left: 8px; top: -48px; background: var(--accent); color: var(--accent-ink); padding: 8px 12px; border-radius: 8px; z-index: 10; }
  .skip:focus { top: 8px; }
  main { width: 100%; max-width: var(--content-width); margin: 0 auto; padding: 36px 40px 56px; flex: 1; }
  main.auth-page { max-width: 960px; }
  main:focus { outline: none; }
  .site-header { border-bottom: 1px solid var(--line); }
  .header-inner { display: flex; align-items: center; justify-content: space-between; gap: 24px; max-width: var(--content-width); margin: auto; padding: 24px 40px; }
  .site-nav { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
  .site-nav a, .entry-link { display: inline-flex; align-items: center; min-height: 44px; padding: 8px 12px; color: var(--muted); font-size: 0.8rem; font-weight: 600; text-decoration: none; border-radius: 8px; }
  .site-nav a:hover, .entry-link:hover { color: var(--ink); background: var(--card); }
  .site-nav a[aria-current='page'] { color: var(--ink); background: var(--tint); }
  .home { display: inline-block; color: inherit; text-decoration: none; }
  .tag { color: var(--muted); font-size: 0.65rem; text-transform: uppercase; letter-spacing: 0.1em; }

  .add { display: flex; gap: 8px; margin: 24px 0 12px; }
  .add input { flex: 1; }

  .notices { display: grid; gap: 8px; }
  .notice { margin: 0; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .notice[hidden] { display: none; }
  .good { border-color: var(--good); }
  .bad { border-color: var(--bad); }

  .toolbar {
    position: sticky; top: 0; z-index: 5; margin-top: 16px; padding: 14px 0; border-bottom: 1px solid var(--line);
    background: color-mix(in srgb, var(--bg) 92%, transparent); backdrop-filter: blur(8px);
    display: flex; flex-wrap: wrap; gap: 10px 16px; justify-content: space-between; align-items: center;
  }
  .filters { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }
  .filters button { background: none; border: 0; padding: 6px 10px; border-radius: 999px; cursor: pointer; color: var(--muted); font-weight: 500; min-height: 36px; }
  .filters button:hover { color: var(--ink); }
  .filters button[aria-pressed='true'] { background: var(--tint); color: var(--ink); font-weight: 650; }
  .filters .liked[aria-pressed='true'] { background: var(--heart); color: #fff; }
  .filters small { margin-left: 4px; font-variant-numeric: tabular-nums; opacity: 0.8; }
  .sep { width: 1px; height: 20px; background: var(--line); margin: 0 6px; }
  .controls { display: flex; gap: 8px; }
  .controls input { width: 10rem; }

  .filterbar { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; padding: 4px 0 2px; }
  .bar-label {
    color: var(--muted); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.1em;
    text-transform: uppercase; margin-right: 4px;
  }
  .filterbar .chip {
    display: inline-flex; align-items: center; gap: 5px; max-width: 260px; min-height: 32px;
    padding: 4px 10px; border: 1px solid var(--line); border-radius: 999px;
    background: transparent; color: var(--muted); font-size: 0.8rem; text-align: left; cursor: pointer;
  }
  .filterbar .chip:hover { border-color: var(--muted); color: var(--ink); }
  .filterbar .chip[aria-pressed='true'] { background: var(--tint); color: var(--ink); border-color: var(--accent); }
  .filterbar .chip small { font-variant-numeric: tabular-nums; opacity: 0.8; }
  .filterbar .chip.old { opacity: 0.55; }
  .filterbar .chip.action { border-style: dashed; border-color: var(--accent); color: var(--accent); }
  .filterbar .chip.clear { border-style: dashed; }
  .filterbar .sep { height: 18px; }
  @media (pointer: coarse) { .filterbar .chip { min-height: 44px; } }

  .select { position: relative; display: inline-flex; }
  .select select { appearance: none; padding-right: 38px; cursor: pointer; }
  .select select:hover { border-color: var(--muted); }
  .select::after {
    content: ''; position: absolute; right: 16px; top: 50%; width: 7px; height: 7px; pointer-events: none;
    border: solid var(--muted); border-width: 0 2px 2px 0; transform: translateY(-70%) rotate(45deg);
  }
  .select:has(select:open)::after { transform: translateY(-30%) rotate(225deg); }
  .select option { background: var(--card); color: var(--ink); }
  @media (prefers-reduced-motion: no-preference) {
    .select::after { transition: transform 0.15s; }
  }
  /* Chromium can draw the list itself, so it can match the cards instead of the OS menu. */
  @supports (appearance: base-select) {
    .select select, .select select::picker(select) { appearance: base-select; }
    .select select::picker-icon { display: none; }
    .select select::picker(select) {
      margin-top: 6px; padding: 6px; border: 1px solid var(--line); border-radius: 12px; background: var(--card);
      box-shadow: 0 16px 40px -12px rgb(0 0 0 / 0.28);
    }
    .select option { padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
    .select option + option { margin-top: 4px; }
    .select option:hover, .select option:focus { background: var(--tint); border-color: var(--accent); outline: none; }
    .select option:checked { font-weight: 650; }
    .select option::checkmark { color: var(--accent); }
  }

  .count { margin: 14px 0 0; color: var(--muted); font-size: 0.9rem; }
  #sections { scroll-margin-top: 140px; }
  section { margin-top: 28px; }
  h2 { margin: 0; display: flex; align-items: baseline; gap: 10px; font: 500 1.65rem/1.2 var(--serif); letter-spacing: -0.02em; }
  h2 small { font: 500 0.9rem var(--sans); color: var(--muted); font-variant-numeric: tabular-nums; }
  .hint { margin: 4px 0 0; color: var(--muted); font: 0.78rem/1.5 var(--sans); }
  ul { list-style: none; margin: 14px 0 0; padding: 0; display: grid; gap: 12px; }
  .none { color: var(--muted); font: italic 1.05rem/1.5 var(--serif); padding: 18px 0 6px 22px; }
  .no-matches { padding: 48px 20px; text-align: center; }
  .no-matches h2 { display: block; }
  .no-matches p { color: var(--muted); }
  .empty { text-align: center; color: var(--muted); font: italic 1.15rem/1.5 var(--serif); padding: 56px 0; }

  .start {
    margin-top: 32px; padding: clamp(22px, 4vw, 30px);
    background: var(--card); border: 1px solid var(--line); border-radius: 16px;
    box-shadow: 0 28px 70px -56px rgb(0 0 0 / 0.35);
  }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.72rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  .start h2 { display: block; margin: 8px 0 0; font: 600 clamp(1.5rem, 4vw, 1.9rem)/1.1 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 0; color: var(--muted); max-width: 48ch; }
  .steps { list-style: none; margin: 24px 0 0; padding: 0; display: grid; gap: 22px; }
  .steps li { display: grid; grid-template-columns: 30px 1fr; gap: 14px; align-items: start; }
  .num {
    display: grid; place-items: center; width: 30px; height: 30px; border-radius: 999px;
    border: 1px solid var(--line); color: var(--muted); font-size: 0.82rem; font-weight: 650;
    font-variant-numeric: tabular-nums;
  }
  .steps h3 { margin: 2px 0 0; font: 650 1rem/1.3 var(--sans); }
  .steps p { margin: 4px 0 0; color: var(--muted); max-width: 52ch; }
  .steps code { font-size: 0.88em; color: var(--ink); }
  .command { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-top: 10px; }
  .command code {
    flex: 1 1 240px; min-width: 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px;
    background: var(--bg); font-size: 0.8rem; overflow-wrap: anywhere;
  }
  .browse { margin: 24px 0 0; padding-top: 18px; border-top: 1px solid var(--line); }
  .browse a { color: var(--accent); font-weight: 600; text-underline-offset: 3px; }
  .browse a:hover { text-decoration-thickness: 2px; }

  :global([data-skill]) { scroll-margin-top: 92px; }
  :global([data-skill].flash) { background: var(--tint); border-radius: 8px; }
  @media (prefers-reduced-motion: no-preference) {
    :global([data-skill].flash) { animation: flash 1.6s var(--ease-out); }
    @keyframes flash {
      0%, 60% { background: var(--tint); }
      100% { background: transparent; }
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    .filters button { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
    .start { animation: rise 340ms var(--ease-out) both; }
    @keyframes rise {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }
  }
  @media (max-width: 560px) {
    main { padding: 28px 16px 40px; }
    .header-inner { flex-wrap: wrap; gap: 14px; padding: 20px 16px; }
    .tag { margin-left: auto; max-width: 18ch; text-align: right; }
    .site-nav { width: 100%; }
    .site-nav a { flex: 1; justify-content: center; border: 1px solid var(--line); }
    .add { flex-direction: column; }
    .controls { width: 100%; }
    .controls input { flex: 1; width: auto; }
    .sep { display: none; }
    .start { margin-top: 24px; padding: 20px; border-radius: 14px; }
    .steps li { grid-template-columns: 26px 1fr; gap: 12px; }
    .num { width: 26px; height: 26px; }
  }
  @media (pointer: coarse) { .filters button { min-height: 44px; } }
</style>
