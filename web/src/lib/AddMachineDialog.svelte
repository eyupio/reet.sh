<script lang="ts" module>
 // The last name typed, kept for this page's lifetime so a second open can offer it. In memory only: never in browser storage.
 let remembered = ''
</script>

<script lang="ts">
 import { onMount, untrack } from 'svelte'
 import type { Snippet } from 'svelte'
 import { UnauthorizedError, type Machine, type SyncState } from './api'
 import { SIGNED_OUT, isSignedOut } from './contentReview'
 import { JOIN_WATCH_CAP_MS, joinBaseline, joinCommands, joinDelay, joinState, validateMachineName, type JoinBaseline, type JoinContext } from './machineJoin'
 import type { ReleaseChannel } from './machineSetup'
 import CopyButton from './CopyButton.svelte'

 // Bring another machine in from where you are working. One block of commands to paste on that machine, and a status line that
 // watches the controller for it. It writes nothing to any machine and sends nothing but the existing token request (made by
 // the token creator it shows) and the refresh reads. Every state says only what the controller has been told.
 // opener is the element the person pressed to open this, which gets focus back on close.
 let { site, machines, snapshot, accounts, canWrite, token, tokenSetup, context, opener = null, channel = 'stable', onrefresh, onready, onstepbystep, onsignedout, onclose }: {
  site: string
  machines: Machine[]
  /** Undefined until the machines have been read once. */
  snapshot: SyncState | undefined
  accounts: boolean
  canWrite: boolean
  token: string
  tokenSetup?: Snippet
  context: JoinContext
  opener: HTMLElement | null
  channel?: ReleaseChannel
  onrefresh: () => Promise<void> | void
  onready: (r: { host: string; context: JoinContext }) => void
  onstepbystep: (name: string) => void
  onsignedout?: (e: unknown) => void
  onclose: () => void
 } = $props()

 const titleId = $props.id()
 let dialog = $state<HTMLDialogElement>()
 let nameInput = $state<HTMLInputElement>()
 let name = $state(remembered)
 let touched = $state(false)
 const checked = $derived(validateMachineName(name))
 const host = $derived(checked.ok ? checked.name : '')

 // What was already reporting under the name, captured from loaded data when the validated name changes and not again.
 let captured = $state<{ host: string; baseline: JoinBaseline } | undefined>()
 // Bumped after each read, so the state is asked again against the clock as it is then.
 let clock = $state(0)
 let capped = $state(false), signedOut = $state(false)
 // Aborted with the dialog: nothing may be started or applied after that.
 const life = new AbortController()
 let generation = 0, failures = 0, waitingSince = Date.now(), readingGen = -1
 let timer: ReturnType<typeof setTimeout> | undefined

 const loaded = $derived(snapshot !== undefined)
 const view = $derived.by(() => {
  void clock
  if (!host || !snapshot || captured?.host !== host) return undefined
  // A fresh clock reading every time the state is asked, never a value held from earlier.
  return joinState({ name: host, machines, agents: snapshot.agents ?? [], baseline: captured.baseline, now: Date.now() })
 })
 const status = $derived.by(() => {
  if (signedOut) return { stage: 'unknown', label: SIGNED_OUT, detail: '', canUse: false }
  if (!host) return { stage: 'idle', label: 'Choose a name to start watching.', detail: '', canUse: false }
  if (!loaded || !view) return { stage: 'idle', label: 'Machines are still loading.', detail: 'Nothing is claimed about this name until they have been read.', canUse: false }
  return view
 })
 const canUse = $derived(status.canUse && !signedOut)
 const waitingOut = $derived(capped && !signedOut && !canUse)
 const commands = $derived(host ? joinCommands(site, host, channel) : '')
 const shown = $derived(touched && !checked.ok ? checked.reason : '')

 $effect.pre(() => {
  const h = host, snap = snapshot
  untrack(() => {
   if (!h) { captured = undefined; return }
   if (!snap || captured?.host === h) return
   captured = { host: h, baseline: joinBaseline(h, snap.agents ?? [], machines) }
   // A new name is a new wait.
   waitingSince = Date.now(); failures = 0
   if (capped) { capped = false; restart() }
  })
 })

 const live = (g: number) => !life.signal.aborted && g === generation
 const lost = (e: unknown) => isSignedOut(e) || e instanceof UnauthorizedError
 function stop() { clearTimeout(timer); timer = undefined; generation++ }
 function arm(g: number) {
  clearTimeout(timer); timer = undefined
  if (!live(g) || signedOut || capped || document.hidden) return
  timer = setTimeout(() => void read(g), joinDelay(failures))
 }
 function restart() { stop(); failures = 0; waitingSince = Date.now(); capped = false; arm(generation) }
 // One read at a time per generation. A failure backs the next one off; a lost sign-in stops everything.
 async function read(g: number) {
  timer = undefined
  if (!live(g) || signedOut || capped || document.hidden || readingGen === g) return
  if (Date.now() - waitingSince >= JOIN_WATCH_CAP_MS) { capped = true; clock++; return }
  readingGen = g
  let stopped = false
  try {
   await onrefresh()
   if (!live(g)) return
   failures = 0
  } catch (e) {
   if (!live(g)) return
   if (lost(e)) { stopped = true; signOut(e); return }
   failures++
  } finally {
   if (readingGen === g) readingGen = -1
   if (live(g) && !stopped) clock++
  }
  arm(g)
 }
 function signOut(e: unknown) {
  if (signedOut) return
  signedOut = true; stop()
  onsignedout?.(e instanceof UnauthorizedError ? e : new UnauthorizedError())
 }
 function checkAgain() { restart(); void read(generation) }
 // Back in view (or back online): read at once, then carry on at the usual pace.
 function wake() {
  if (life.signal.aborted || signedOut || capped) return
  clearTimeout(timer); timer = undefined
  if (!document.hidden) void read(generation)
 }

 function shutdown(returnFocus: boolean) {
  life.abort(); stop()
  dialog?.close()
  if (returnFocus && opener?.isConnected) opener.focus()
 }
 function close() {
  remembered = host
  shutdown(true)
  onclose()
 }
 function use() {
  if (!canUse || !host) return
  const chosen = host
  remembered = ''
  shutdown(true)
  onready({ host: chosen, context })
  onclose()
 }
 function stepByStep() {
  touched = true
  if (!host) { nameInput?.focus(); return }
  const chosen = host
  remembered = ''
  shutdown(false)
  onstepbystep(chosen)
  onclose()
 }

 onMount(() => {
  dialog?.showModal()
  arm(generation)
  document.addEventListener('visibilitychange', wake)
  window.addEventListener('online', wake)
  return () => { life.abort(); stop(); clearTimeout(timer); document.removeEventListener('visibilitychange', wake); window.removeEventListener('online', wake) }
 })
</script>

<dialog bind:this={dialog} oncancel={(event) => { event.preventDefault(); close() }} aria-labelledby={titleId}>
 <header>
  <div>
   <p class="eyebrow">Machines</p>
   <h2 id={titleId}>Add a machine</h2>
   <p class="lede">Right then: name it, give it a key, run one block of commands on it, and reet.sh will watch for it.</p>
  </div>
  <button type="button" class="btn" onclick={close} aria-label="Close">Close ×</button>
 </header>
 <div class="body">
  <section aria-labelledby={`${titleId}-name`}>
   <h3 id={`${titleId}-name`}><span class="num" aria-hidden="true">1</span>Name it</h3>
   <form onsubmit={(event) => { event.preventDefault(); touched = true }}>
    <label for={`${titleId}-field`}>Machine name</label>
    <input id={`${titleId}-field`} bind:this={nameInput} bind:value={name} list={`${titleId}-known`} autocomplete="off" spellcheck="false" placeholder="e.g. studio-laptop"
     aria-invalid={shown ? 'true' : undefined} aria-describedby={shown ? `${titleId}-hint ${titleId}-reason` : `${titleId}-hint`}
     oninput={() => (touched = true)} onblur={() => (touched = true)} />
    <datalist id={`${titleId}-known`}>{#each machines as machine (machine.host)}<option value={machine.host}></option>{/each}</datalist>
    <p class="hint" id={`${titleId}-hint`}>This is how the machine appears in reet.sh. Keep it the same between scans.</p>
    {#if shown}<p class="reason" id={`${titleId}-reason`}>{shown}</p>{/if}
   </form>
  </section>

  <section aria-labelledby={`${titleId}-key`}>
   <h3 id={`${titleId}-key`}><span class="num" aria-hidden="true">2</span>Give it a key</h3>
   {#if tokenSetup}
    <p class="hint">Create a machine token here. It is shown once and is never stored in the browser. It is not part of the commands: the machine asks for it and you paste it there.</p>
    {@render tokenSetup()}
    {#if token}
     <details class="one-time"><summary>Show the one-time machine token</summary><code>{token}</code><CopyButton text={token} label="Copy machine token" compact /></details>
     <p class="hint">When the CLI prompts for a token, paste this value. It is not embedded in the commands.</p>
    {/if}
   {:else}
    <p class="hint">Use the token configured for this instance when the CLI asks. Instances without authentication do not require one.</p>
   {/if}
  </section>

  <section aria-labelledby={`${titleId}-run`}>
   <h3 id={`${titleId}-run`}><span class="num" aria-hidden="true">3</span>Run it</h3>
   {#if host}
    <p class="hint">Paste this once in a terminal on the machine. It installs reet, saves the connection, then enrols and runs the agent.</p>
    <div class="command"><pre class="commands">{commands}</pre><CopyButton text={commands} label="Copy commands" /></div>
    <p class="hint">The machine will ask for a token. Paste the machine token from step 2 when it does; a missing or failed token does not change the commands.</p>
   {:else}
    <p class="hint">Enter a name above and the commands will appear here.</p>
   {/if}
   <p class="hint">Nothing happens on that machine until you run this there. reet.sh never logs in to it.</p>
   <button type="button" class="linklike" onclick={stepByStep}>Prefer the step-by-step setup?</button>
  </section>

  <div class="join-status" role="status" aria-live="polite" aria-atomic="true" data-stage={status.stage}>
   <span class="dot {status.stage}" aria-hidden="true"></span>
   <div>
    <strong>{status.label}</strong>
    {#if status.detail}<span class="detail">{status.detail}</span>{/if}
    {#if waitingOut}<span class="detail">Still waiting. Check again.</span>{/if}
   </div>
  </div>
  {#if waitingOut}<p class="again"><button type="button" class="btn" onclick={checkAgain}>Check again</button></p>{/if}
  <p class="hint">Connected and Ready only say what reet.sh has been told: that a connector reported in and a scan arrived. They do not show that the commands worked.</p>
 </div>
 <footer>
  <small>Nothing here changes a machine.</small>
  <div class="actions">
   {#if canUse}<button type="button" class="btn primary" onclick={use}>Use {host}</button>{/if}
  </div>
 </footer>
</dialog>

<style>
 dialog { width: min(720px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; color: var(--ink); background: var(--card); border: 1px solid var(--line); border-radius: 16px; box-shadow: var(--shadow); }
 dialog[open] { display: grid; grid-template-rows: auto minmax(0, 1fr) auto; }
 dialog::backdrop { background: rgb(20 18 15 / 0.45); }
 header { display: flex; gap: 20px; align-items: flex-start; justify-content: space-between; padding: 22px; border-bottom: 1px solid var(--line); border-radius: 16px 16px 0 0; }
 header > div { min-width: 0; }
 h2 { margin: 4px 0 0; font: 500 1.65rem/1.2 var(--serif); overflow-wrap: anywhere; }
 .eyebrow { margin: 0; color: var(--accent); text-transform: uppercase; font-size: .65rem; letter-spacing: .1em; }
 .lede { margin: 8px 0 0; color: var(--muted); font-size: .88rem; line-height: 1.5; }
 header .btn { flex: none; white-space: nowrap; }
 .body { display: grid; gap: 22px; padding: 22px; min-height: 0; overflow: auto; overflow-wrap: anywhere; align-content: start; }
 section { min-width: 0; }
 h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 8px; font: 600 1rem/1.3 var(--sans); }
 .num { display: inline-grid; place-items: center; flex: none; width: 24px; height: 24px; border-radius: 50%; background: var(--tint); color: var(--accent); font-size: .8rem; }
 form { display: grid; gap: 6px; }
 label { font-size: .85rem; font-weight: 600; }
 input { min-width: 0; width: 100%; min-height: 42px; padding: 8px 10px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 input[aria-invalid='true'] { border-color: var(--bad); }
 .hint, small { margin: 6px 0; color: var(--muted); font-size: .83rem; line-height: 1.5; }
 .reason { margin: 0; color: var(--bad); font-size: .85rem; }
 .command { display: flex; align-items: flex-start; gap: 12px; flex-wrap: wrap; margin: 10px 0; padding: 12px; background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
 pre { flex: 1 1 260px; min-width: 0; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: .78rem/1.6 var(--mono); }
 .one-time { border: 1px solid var(--line); border-radius: 10px; padding: 12px; background: var(--bg); }
 .one-time code { display: block; margin: 10px 0; overflow-wrap: anywhere; }
 .linklike { padding: 4px 0; min-height: 32px; font: inherit; font-size: .85rem; color: var(--accent); background: none; border: 0; text-decoration: underline; cursor: pointer; text-align: left; }
 .join-status { display: flex; align-items: flex-start; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
 .join-status > div { display: grid; gap: 4px; min-width: 0; }
 .detail { color: var(--muted); font-size: .85rem; line-height: 1.5; }
 .dot { flex: none; width: 12px; height: 12px; margin-top: 4px; border-radius: 50%; background: var(--muted); }
 .dot.waiting, .dot.first_scan, .dot.idle { background: var(--muted); }
 .dot.connected, .dot.already_here { background: var(--warn); }
 .dot.ready { background: var(--good); }
 .dot.unknown { background: var(--bad); }
 .again { margin: -12px 0 0; }
 footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; padding: 14px 22px; border-top: 1px solid var(--line); border-radius: 0 0 16px 16px; }
 footer small { margin: 0; }
 .actions { display: flex; flex-wrap: wrap; gap: 10px; }
 @media (max-width: 500px) { header, .body { padding: 16px; } footer { padding: 12px 16px; } h2 { font-size: 1.35rem; } .command :global(button) { width: 100%; } }
</style>
