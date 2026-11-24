<script lang="ts">
  // A calm, optional guide for the first session on a connected machine: which chat agents are there and how ready each is
  // (the words stage 3 uses), one read-only question to ask the one chosen, and the way to reet on a phone. It blocks
  // nothing on Home. It leaves when the person says Not now (Home offers a way back), or once the first reply has been seen.
  import { onMount, tick, untrack } from 'svelte'
  import Sentence from '../Sentence.svelte'
  import { UnauthorizedError } from '../api'
  import { toolChoices, notStartedText, refusalText } from '../agentLaunch'
  import { CATALOGUE } from '../catalogue'
  import { launchReadiness } from '../factory/launchReadiness'
  import type { MachineAccounts } from '../agentAccounts'
  import { modelStatus, type MachineModels } from '../models'
  import { chatNeedsTrust, chatTrustText, createTerminal, machineAccounts, machineModels, readiness, refreshMachineAccounts, sessionHref, structuredAgent, TerminalError,
    type FolderProject, type LaunchRequest, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { FIRST_SESSION_DOC_URL } from '../links'
  import { FIRST_PROMPT, guideBack, guideShown, nameList, replyOf, type FirstRunMemory } from './firstRun'

  let { machine, sessions, projects, userId, showing = $bindable(false), back = $bindable(false), onchanged, onterminal, onsignedout }: {
    /** The first connected machine, as the terminal list reports it. */
    machine: TerminalMachine | undefined
    sessions: TerminalSession[] | undefined
    projects: FolderProject[]
    userId: number
    /** Whether the guide is on Home, for the page to quieten its own first step while it is. */
    showing?: boolean
    /** Whether Not now was said and the guide could still come back, for Home to offer the way back. */
    back?: boolean
    /** Read Home again: a session was started. */
    onchanged: () => void
    /** The existing way to open a terminal, given the button it came from. */
    onterminal: (opener: HTMLElement) => void
    onsignedout: () => void
  } = $props()

  const id = $props.id()
  const POLL_MS = 15_000, SLOW_MS = 60_000
  const memoryKey = $derived(`reet.firstrun.${userId}`)
  const sessionKey = $derived(`reet.firstrun.session.${userId}`)
  const store = () => { try { return localStorage } catch { return undefined } }
  const remembered = (key: string) => { try { return store()?.getItem(key) ?? '' } catch { return '' } }
  const remember = (key: string, value: string) => { try { if (value) store()?.setItem(key, value); else store()?.removeItem(key) } catch { /* The guide simply shows again next time. */ } }

  let memory = $state<FirstRunMemory>('')
  // The sessions the guide started, kept so asking again after one ended does not hide the guide; the last is the one it follows.
  let startedIds = $state<string[]>([])
  let trackedId = $state('')
  // Once the first reply is seen the guide is put away for good, but stays on this page until the person is done with it.
  let celebrated = $state(false)
  onMount(() => {
    const saved = remembered(memoryKey)
    memory = saved === 'dismissed' || saved === 'done' ? saved : ''
    startedIds = remembered(sessionKey).split(',').filter(Boolean)
    trackedId = startedIds.at(-1) ?? ''
  })

  const mine = $derived(sessions?.find(s => s.id === trackedId))
  const reply = $derived(replyOf(mine))
  const shown = $derived(celebrated || guideShown(memory, !!machine, sessions, startedIds))
  $effect(() => { back = guideBack(memory, !!machine, sessions, startedIds) })
  let heading = $state<HTMLElement | null>(null)
  export async function restore() {
    memory = ''; remember(memoryKey, '')
    await tick()
    heading?.focus()
  }
  $effect(() => {
    if (reply === 'replied' && !celebrated && !memory) { celebrated = true; remember(memoryKey, 'done') }
  })

  // What the machine says of its agents: sign-ins (read again every 15 seconds while one is missing) and model lists (read once each).
  let accounts = $state<MachineAccounts | null | undefined>()
  let models = $state<Record<string, MachineModels | null | 'reading'>>({})
  let checking = $state(false)
  const installed = $derived(machine ? toolChoices(machine).filter(choice => choice.available) : [])
  // Only an agent that runs as a chat lets reet see the turn end; the rest are started from Start an agent session.
  const chats = $derived(installed.filter(choice => structuredAgent(machine?.capabilities, choice.id)))
  const terminalOnly = $derived(installed.filter(choice => !chats.includes(choice)).map(choice => choice.name))
  const supported = CATALOGUE.filter(agent => agent.enabled).map(agent => agent.name)
  const host = $derived(machine?.host ?? 'this machine')
  const stuck = $derived(machine ? readiness(machine) : '')
  const rows = $derived(chats.map(choice => {
    const ready = launchReadiness({ machine, tools: installed.map(item => item.id), efforts: () => [''], models: tool => models[tool] ?? 'reading', accounts, now: Date.now(),
      member: { tool: choice.id, model: '' }, spares: [], saved: null, edited: false })
    const list = models[choice.id]
    // A model list is said only when it is not fine; an agent that is ready is one line, as a stage in the plan is.
    const modelLine = list === undefined || list === 'reading' ? '' : list === null ? `reet could not read ${choice.name}’s models from ${host}.` : list.state === 'listed' ? '' : modelStatus(list, host).sentence
    const mark = ready.signIn.pending ? '…' : ready.signIn.state === 'missing' ? '!' : ready.signIn.quiet ? '·' : ready.signIn.state === 'ready' ? '✓' : '!'
    return { ...choice, signIn: ready.signIn, mark, modelLine, usable: ready.signIn.state !== 'missing' && !ready.signIn.pending }
  }))

  let picked = $state('')
  // The agent asked: the person's choice if it is there, else a ready one that needs no folder trust, else any ready one, else the first.
  const current = $derived(rows.find(row => row.id === picked) ?? rows.find(row => row.usable && !chatNeedsTrust(row.id)) ?? rows.find(row => row.usable) ?? rows[0])
  const other = $derived(rows.find(row => row.usable && row.id !== current?.id))
  const askOther = $derived(rows.find(row => row.usable && row.id !== current?.id && !chatNeedsTrust(row.id)))
  // With only terminal agents here the guide has nothing to ask, so Start an agent session is the step again.
  $effect(() => { showing = shown && !(installed.length && !rows.length && !stuck) })
  const signedOut = $derived(current?.signIn.state === 'missing')

  async function readAccounts(refresh: boolean) {
    const target = machine
    if (!target) return
    try {
      const answer = await (refresh ? refreshMachineAccounts(target.agentId) : machineAccounts(target.agentId))
      if (machine?.agentId === target.agentId) accounts = answer
    } catch (e) {
      if (e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)) return onsignedout()
      if (machine?.agentId === target.agentId && accounts === undefined) accounts = null
    }
  }
  async function readModels(agentId: string, tool: string) {
    models = { ...models, [tool]: 'reading' }
    let next: MachineModels | null = null
    try { next = await machineModels(agentId, tool) } catch { /* said as not read */ }
    if (machine?.agentId === agentId) models = { ...models, [tool]: next }
  }
  async function checkAgain() { checking = true; await readAccounts(true); checking = false }

  // Reading starts when the guide shows. While a sign-in is missing the machine is asked again every 15 seconds, so the
  // guide moves on by itself once the sign-in is there; with nothing missing it stops asking.
  const agentKey = $derived(shown && machine?.ready ? machine.agentId : '')
  const missing = $derived(rows.some(row => row.signIn.state === 'missing'))
  $effect(() => {
    if (!agentKey) return
    untrack(() => { accounts = undefined; models = {}; void readAccounts(false) })
  })
  $effect(() => {
    if (!agentKey || !missing) return
    const timer = setInterval(() => { if (document.visibilityState === 'visible') void readAccounts(true) }, POLL_MS)
    return () => clearInterval(timer)
  })
  $effect(() => {
    if (!agentKey) return
    for (const choice of chats) if (untrack(() => models[choice.id]) === undefined) void readModels(agentKey, choice.id)
  })

  // The question: where it looks, and for an agent that needs it, the same trust the Start an agent dialog asks for.
  let folder = $state(''), trust = $state(false)
  const folders = $derived(machine ? projects.filter(project => project.host === machine.agentId) : [])
  const needsTrust = $derived(!!current && chatNeedsTrust(current.id))
  const where = $derived(folder || `the home folder on ${host}`)
  const trustId = `${id}-trust`
  let openLink = $state<HTMLAnchorElement | null>(null)
  let wantFocus = false
  // The Ask button is gone once it works; the link that replaces it takes the focus when it arrives.
  $effect(() => { if (openLink && wantFocus) { wantFocus = false; openLink.focus() } })
  // A reply that is slow is said once, quietly, with no new motion.
  let slow = $state(false)
  $effect(() => {
    slow = false
    if (reply !== 'asking') return
    const timer = setTimeout(() => { slow = true }, SLOW_MS)
    return () => clearTimeout(timer)
  })
  let starting = $state(false), problem = $state(''), request = ''
  async function ask() {
    if (!machine || !current || starting || signedOut) return
    const launch: LaunchRequest = { tool: current.id, prompt: FIRST_PROMPT, structured: true }
    if (needsTrust) launch.trust = true
    if (folder) launch.dir = folder
    // A request whose answer was lost is asked again with the same id, so asking twice never starts two.
    request ||= crypto.randomUUID()
    starting = true; problem = ''
    try {
      const session = await createTerminal(machine.agentId, request, 'First look', 100, 30, AbortSignal.timeout(60_000), launch)
      const refused = notStartedText(session.state)
      request = ''
      if (refused) { problem = refused; return }
      trackedId = session.id
      startedIds = [...startedIds, session.id]
      remember(sessionKey, startedIds.join(','))
      wantFocus = true
      onchanged()
    } catch (e) {
      if (e instanceof UnauthorizedError || (e instanceof TerminalError && e.status === 401)) return onsignedout()
      if (e instanceof TerminalError && [400, 403, 404, 409, 422].includes(e.status)) { request = ''; problem = refusalText(e.status, e.code, e.message) }
      else problem = "We didn't hear back, so the session may have started. Asking again uses the same request."
    } finally { starting = false }
  }
  function again() { trackedId = ''; problem = '' }
  function askInstead(tool: string) { picked = tool; again(); void ask() }

  function close(next: FirstRunMemory) {
    memory = next; celebrated = false
    remember(memoryKey, next)
    queueMicrotask(() => document.querySelector<HTMLElement>('.home-start, .workbench button.btn')?.focus())
  }

  const name = $derived(current?.name ?? '')
  // One sentence for the status region: it changes only when the state does, so a poll that finds the same thing says nothing.
  const status = $derived(reply === 'replied' ? `${name} answered. It was asked to change nothing and ran without YOLO.`
    : reply === 'ended' ? `The session ended before ${name} answered. It was only asked to look, not to change anything.${mine?.exitCode ? ` It exited with code ${mine.exitCode}.` : ''}`
    : reply === 'asking' ? `Asking ${name}. Waiting for its first reply.${slow ? ' This is taking longer than usual. Open the session to look.' : ''}`
    : starting ? `Starting ${name} on ${host}…`
    : problem ? problem
    : signedOut ? `Waiting for ${name} to be signed in on ${host}. Sign in in ${name}’s own program on ${host}; this asks the machine again every 15 seconds.`
    : '')
  // The shared sign-in sentence also warns the first message may be refused; the guide already says what it is waiting for.
  const signInText = (text: string) => text.replace(', or its first message may be refused.', '.')
  const stuckHelp = $derived(!!stuck || !installed.length || signedOut || reply === 'ended')
  const terminalLine = (names: string[]) => `${nameList(names)} ${names.length === 1 ? 'runs' : 'run'} in a terminal here. Start ${names.length === 1 ? 'it' : 'one'} from Start an agent session.`
  const local = ['localhost', '127.0.0.1', '[::1]'].includes(location.hostname)
</script>

{#if shown}
  <section class="guide" aria-labelledby={`${id}-h`}>
    <header>
      <h2 id={`${id}-h`} tabindex="-1" bind:this={heading}>Your first session</h2>
      {#if reply !== 'replied'}<button type="button" class="btn quiet" onclick={() => close('dismissed')}>Not now</button>{/if}
    </header>
    {#if !stuck}<p class="lede">{host} is connected.{#if rows.length}{' '}Ask an agent one question that changes nothing.{/if}</p>{/if}

    {#if stuck}
      <p class="note">{host} cannot take a session yet. <Sentence text={stuck} /></p>
      <p class="help"><a href="/machines">Open Machines</a></p>
    {:else if !installed.length}
      <p class="note">No agent is installed on {host} yet. Install {nameList(supported, 'or')} there; this looks again by itself.</p>
    {:else if !rows.length}
      <p class="note">None of the agents on {host} runs as a chat here, so reet cannot see when one answers. {terminalLine(terminalOnly)}</p>
    {:else}
      <fieldset class="agents" disabled={starting || !!trackedId}>
        <legend>Agent</legend>
        {#each rows as row (row.id)}
          <label class="agent" class:on={row.id === current?.id}>
            <input type="radio" name={`${id}-agent`} value={row.id} checked={row.id === current?.id} onchange={() => (picked = row.id)} />
            <span class="what"><strong>{row.name}</strong>
              <span class="line" class:warn={row.mark === '!'}><span class="mark" aria-hidden="true">{row.mark}</span><span>{signInText(row.signIn.text)}</span></span>
              {#if row.modelLine}<span class="line warn"><span class="mark" aria-hidden="true">!</span><span>{row.modelLine}</span></span>{/if}
            </span>
          </label>
        {/each}
      </fieldset>
      {#if terminalOnly.length}<p class="quiet-line">{terminalLine(terminalOnly)}</p>{/if}

      <div class="ask">
        <p class="prompt"><span class="vh">The question: </span>{FIRST_PROMPT}</p>
        {#if !trackedId}
          {#if folders.length}
            <div class="field"><label for={`${id}-folder`}>Where it looks</label>
              <select id={`${id}-folder`} bind:value={folder} disabled={starting}>
                <option value="">The home folder on {host}</option>
                {#each folders as project (project.id)}<option value={project.dir}>{project.name}</option>{/each}
              </select></div>
          {/if}
          {#if needsTrust}<label class="trust" id={trustId}><input type="checkbox" bind:checked={trust} /> {chatTrustText(current?.id).replace('this folder', where)}</label>{/if}
          {#if signedOut}
            <div class="actions">
              <button type="button" class="btn primary" onclick={event => onterminal(event.currentTarget)}>Open terminal</button>
              {#if other}<button type="button" class="btn" onclick={() => (picked = other.id)}>Use {other.name} instead</button>{/if}
              <button type="button" class="btn" aria-busy={checking} onclick={checkAgain}>{checking ? 'Checking…' : 'Check again'}</button>
            </div>
          {:else}
            <div class="actions">
              <button type="button" class="btn primary" aria-busy={starting} aria-describedby={needsTrust && !trust ? trustId : undefined} disabled={starting || !current?.usable || (needsTrust && !trust)} onclick={ask}>{starting ? `Starting ${name}…` : `Ask ${name}`}</button>
            </div>
          {/if}
        {:else}
          <div class="actions">
            {#if mine}<a class="btn" class:primary={reply === 'replied'} href={sessionHref(mine)} bind:this={openLink}>{reply === 'replied' ? 'Read the reply' : 'Open the session'}</a>{/if}
            {#if reply === 'replied'}<button type="button" class="btn" onclick={() => close('done')}>Done</button>
            {:else if reply === 'ended'}
              <button type="button" class="btn" onclick={again}>Try again</button>
              {#if askOther}<button type="button" class="btn" onclick={() => askInstead(askOther.id)}>Ask {askOther.name} instead</button>{/if}
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    <p class="status" role="status" class:good={reply === 'replied'}>{status}</p>

    {#if stuckHelp}<p class="help"><a href={FIRST_SESSION_DOC_URL} target="_blank" rel="noopener noreferrer">How the first session works<span class="vh"> (opens in a new tab)</span></a></p>{/if}

    <details class="phone">
      <summary>Use reet on your phone</summary>
      <p>On your phone, open {#if local}the address you use to reach this controller from other devices{:else}<strong>{location.origin}</strong>{/if} and sign in. Then use your browser’s <strong>Add to Home Screen</strong> to keep reet to hand.{#if local}{' '}A phone cannot open <strong>localhost</strong>.{/if}</p>
      <p>To be told when a session needs you, turn on notifications on the phone; that needs https. <a href="/notifications">Open Notifications</a></p>
    </details>
    <p class="phone-here"><a href="/notifications">Turn on notifications</a></p>
  </section>
{/if}

<style>
  .guide { position: relative; display: grid; gap: 12px; min-width: 0; padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  header { display: flex; justify-content: space-between; gap: 8px 16px; align-items: center; min-width: 0; }
  h2 { margin: 0; font: 500 1.25rem/1.3 var(--serif); }
  header .btn { flex: none; margin-right: -8px; }
  .lede { margin: -8px 0 0; color: var(--muted); font-size: 0.9375rem; max-width: 62ch; }
  /* Short lines do not sit in a full-width box: the card is full width, what it says is not. */
  .guide > :not(header) { max-width: 720px; }
  h2:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 6px; }
  .note, .status, .prompt { margin: 0; font-size: 0.9375rem; overflow-wrap: anywhere; }
  /* Empty, it stays in the accessibility tree so its first message is announced, and takes no room. */
  .status:empty { position: absolute; }
  .status.good { color: var(--good); font-weight: 600; }
  .agents { display: grid; gap: 8px; margin: 0; padding: 0; border: 0; min-width: 0; }
  .agents legend { padding: 0; margin-bottom: 8px; font-weight: 600; font-size: 0.9375rem; }
  .agent { display: flex; align-items: flex-start; gap: 12px; min-height: 44px; box-sizing: border-box; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; min-width: 0; }
  .agent.on { border-color: var(--accent); background: var(--accent-wash); }
  .agent input { margin-top: 3px; flex: none; accent-color: var(--accent); }
  .agent:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
  .what { display: grid; gap: 2px; min-width: 0; }
  .line { display: grid; grid-template-columns: 1.5em minmax(0, 1fr); color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
  .line .mark { font-weight: 700; color: var(--good); }
  .line.warn, .line.warn .mark { color: var(--warn); }
  .ask { display: grid; gap: 10px; justify-items: start; min-width: 0; }
  .prompt { padding: 10px 12px; border: 1px dashed var(--line-strong); border-radius: 8px; font-family: var(--mono); font-size: 0.875rem; }
  .field { display: grid; gap: 4px; min-width: 0; max-width: 100%; }
  .field label { font-size: 0.875rem; font-weight: 600; }
  .field select { min-height: 40px; max-width: 100%; }
  .trust { display: flex; gap: 8px; align-items: flex-start; font-size: 0.875rem; }
  .quiet-line { margin: 0; color: var(--muted); font-size: 0.875rem; }
  .help a { color: var(--accent-text); font-weight: 600; }
  .help { margin: 0; font-size: 0.875rem; }
  .help a:focus-visible, .phone-here a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 6px; }
  .phone-here { display: none; margin: 0; font-size: 0.9375rem; }
  .phone-here a { color: var(--accent-text); font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .actions .btn { min-height: 40px; }
  .phone { border-top: 1px solid var(--line-soft); padding-top: 4px; font-size: 0.9375rem; }
  .phone summary { display: list-item; list-style-position: inside; align-content: center; min-height: 40px; cursor: pointer; color: var(--ink); }
  .phone summary:hover { text-decoration: underline; }
  .phone summary:focus-visible, .phone a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 6px; }
  .phone p { margin: 4px 0 8px; max-width: 62ch; }
  .phone a { color: var(--accent-text); font-weight: 600; }
  @media (pointer: coarse) {
    .actions .btn, .field select, .phone summary { min-height: 44px; }
    .phone a, .help a, .phone-here a { display: inline-flex; align-items: center; min-height: 44px; }
    .agent input, .trust input { width: 22px; height: 22px; }
    .trust { align-items: center; min-height: 44px; }
  }
  /* A phone is already the place the phone step points to; only the notifications link is of use there. */
  @media (max-width: 650px), (pointer: coarse) and (hover: none) { .phone { display: none; } .phone-here { display: block; } }
  @media (max-width: 650px) { .guide { padding: 12px; } header .btn { margin-right: -4px; } }
</style>
