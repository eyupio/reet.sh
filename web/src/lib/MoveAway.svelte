<script lang="ts">
  // The old controller's side of a move: fence it, take the final copy, and say what happens next on the new
  // controller. Machines follow on their own; the page shows each one as it goes. The owner only, and only on a
  // controller that holds the fence.
  //
  // The order is the point. A copy taken before the fence can already be out of date, and a second live controller
  // working from it would split one set of data, so the copy step stays shut until the fence is up, and the page says
  // whether the copy it took really was taken fenced rather than assuming so.
  import { backupDownloadUrl, getFenceMachines, liftFence, raiseFence, runBackupNow, setFenceAddress, type BackupItem } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import PasswordInput from './PasswordInput.svelte'
  import {
    COPY_PASSWORD_MIN, addressProblem, cleanAddress, copyPasswordProblem, copySource, finalCopy, followProblem, machineMoveState,
    moveProgress, moveStateLine, type Fence, type FenceMachine, type MoveFeatures,
  } from './controllerMove'

  let { fence, features, items, enabled = true, when, size, onfence, ontaken }: {
    fence: Fence
    features: MoveFeatures
    items: BackupItem[]
    /** Whether backups can be taken at all; without a usable key there is no copy to take. */
    enabled?: boolean
    when: (ms?: number) => string
    size: (bytes: number) => string
    onfence: (next: Fence) => void
    ontaken: (item: BackupItem) => void
  } = $props()

  let address = $state('')
  let password = $state('')
  let busy = $state('')
  let error = $state('')
  let asking = $state<'' | 'raise' | 'lift'>('')
  let taken = $state<BackupItem | null>(null)
  let again = $state(false)

  let editing = $state(false)
  let machines = $state<FenceMachine[] | null>(null)
  let machinesError = $state('')
  // The controller's clock, so "still asking" is judged against the times it recorded.
  let skew = $state(0)
  let tick = $state(Date.now())

  const addressBad = $derived(addressProblem(address))
  const addressNote = $derived(addressBad ? '' : followProblem(address))
  const passwordBad = $derived(password !== '' ? copyPasswordProblem(password) : '')
  // What was just taken is shown even when it turned out not to be fenced, so the page can say so; a reload falls back
  // to the list, which only ever offers a copy that says it was.
  // Once the list has caught up it is the list's row that is shown, so the size reads the same here as in the copies below.
  const copy = $derived(fence.fenced ? ((taken && (items.find(item => item.id === taken?.id) ?? taken)) ?? finalCopy(items, fence)) : null)
  const copyState = $derived(copy ? copySource(copy) : 'unknown')
  const copyGood = $derived(!!copy && (copyState === 'fenced' || !features.copyOrigin))
  const now = $derived(tick + skew)
  const progress = $derived(machines ? moveProgress(machines, now) : null)

  // While the fence stands the machines are asked after every ten seconds, so the list moves as they do.
  $effect(() => {
    if (!fence.fenced || !features.fenceMachines) {
      machines = null
      return
    }
    let stopped = false
    const load = async () => {
      try {
        const got = await getFenceMachines()
        if (stopped) return
        skew = got.now - Date.now()
        tick = Date.now()
        machines = got.machines
        machinesError = ''
      } catch (e) {
        if (!stopped) machinesError = e instanceof Error ? e.message : String(e)
      }
    }
    void load()
    const timer = setInterval(load, 10_000)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  })

  function say(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
  }

  async function raise() {
    asking = ''
    busy = 'fence'
    error = ''
    try {
      onfence(await raiseFence(cleanAddress(address)))
      address = ''
    } catch (e) {
      say(e)
    } finally {
      busy = ''
    }
  }

  async function readdress(e: SubmitEvent) {
    e.preventDefault()
    if (addressBad) return
    busy = 'address'
    error = ''
    try {
      onfence(await setFenceAddress(cleanAddress(address)))
      address = ''
      editing = false
    } catch (e) {
      say(e)
    } finally {
      busy = ''
    }
  }

  async function lift() {
    asking = ''
    busy = 'lift'
    error = ''
    try {
      onfence(await liftFence())
      taken = null
      again = false
      password = ''
    } catch (e) {
      say(e)
    } finally {
      busy = ''
    }
  }

  async function take(e: SubmitEvent) {
    e.preventDefault()
    if (features.copyPassword) {
      const problem = copyPasswordProblem(password)
      if (problem) {
        error = problem
        return
      }
    }
    busy = 'copy'
    error = ''
    try {
      taken = await runBackupNow(true, features.copyPassword ? password : undefined)
      // The password is not kept by the controller, and not by the page either once the copy exists.
      password = ''
      again = false
      ontaken(taken)
    } catch (e) {
      say(e)
    } finally {
      busy = ''
    }
  }
</script>

<section class="block move" aria-labelledby="move-away-title">
  <h3 id="move-away-title">Move this controller away</h3>
  <p class="muted">
    For a controller that is going to a new machine or a new operator. Fence it so it stands still, take the final
    copy, and carry that copy to the new controller. Nothing here deletes anything, and lifting the fence puts this
    controller back as it was.
  </p>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}

  <ol class="steps">
    <li class:done={fence.fenced}>
      <h4><span class="marker" aria-hidden="true">{fence.fenced ? '✓' : '1'}</span>Fence this controller</h4>
      {#if fence.fenced}
        <p>
          Fenced since {when(fence.since)}.
          {#if fence.movedTo}Moved to <a href={fence.movedTo} rel="noopener noreferrer">{fence.movedTo}</a>.{:else}No new address was given yet, so machines do not know where to go.{/if}
        </p>
        {#if features.fenceAddress && (editing || !fence.movedTo)}
          <form class="readdress" onsubmit={readdress}>
            <label class="field">
              <span>{fence.movedTo ? 'Change the new address' : 'New address'}</span>
              <input type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://reet.new.example" bind:value={address}
                aria-invalid={addressBad ? 'true' : undefined} aria-describedby="move-readdress-hint" />
            </label>
            <small id="move-readdress-hint" class:bad={!!addressBad || !!addressNote}>
              {addressBad || addressNote || 'Machines are told it on their next request and follow on their own. The fence stays up.'}
            </small>
            <div class="acts">
              <button type="submit" class="btn primary" disabled={busy !== '' || !cleanAddress(address) || !!addressBad} aria-busy={busy === 'address'}>
                {busy === 'address' ? 'Saving…' : 'Save the address'}
              </button>
              {#if editing}<button type="button" class="btn" onclick={() => { editing = false; address = '' }}>Cancel</button>{/if}
            </div>
          </form>
        {/if}
        <p class="muted">Every change is refused and background work stands still. Signing in, reading, and taking, checking and downloading copies still work.</p>
        <div class="acts">
          {#if features.fenceAddress && fence.movedTo && !editing}
            <button type="button" class="btn" onclick={() => (editing = true)} disabled={busy !== ''}>Change the address</button>
          {/if}
          <button type="button" class="btn" onclick={() => (asking = 'lift')} disabled={busy !== ''} aria-busy={busy === 'lift'}>
            {busy === 'lift' ? 'Lifting…' : 'Lift the fence'}
          </button>
        </div>
      {:else}
        <!-- The hint sits outside the label so the field is named by its label alone. -->
        <label class="field">
          <span>New address (optional)</span>
          <input type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://reet.new.example" bind:value={address}
            aria-invalid={addressBad ? 'true' : undefined} aria-describedby="move-address-hint" />
        </label>
        <small id="move-address-hint" class:bad={!!addressBad || !!addressNote}>
          {addressBad || addressNote || 'Where the controller will answer after the move. Every machine is told it and follows on its own; you can also give it once fenced.'}
        </small>
        <div class="acts">
          <button type="button" class="btn primary" onclick={() => (asking = 'raise')} disabled={busy !== '' || !!addressBad} aria-busy={busy === 'fence'}>
            {busy === 'fence' ? 'Fencing…' : 'Fence this controller'}
          </button>
        </div>
      {/if}
    </li>

    <li class:done={copyGood} class:later={!fence.fenced}>
      <h4><span class="marker" aria-hidden="true">{copyGood ? '✓' : '2'}</span>Take the final copy</h4>
      {#if !fence.fenced}
        <p class="muted">Available once the controller is fenced, so that nothing changes after the copy is taken.</p>
      {:else}
        {#if copy}
          <div class="copy" role="status">
            <p class="copy-id">
              <code>{copy.id}</code>
              {#if copyState === 'fenced'}<span class="tag good">taken fenced</span>{:else if features.copyOrigin}<span class="tag bad">not marked fenced</span>{/if}
            </p>
            <p class="muted">{when(copy.takenAt)} · {size(copy.bytes)}</p>
            {#if copyGood}
              <p class="muted">
                {copy.protection === 'password'
                  ? 'It opens with its password alone, on any machine. This controller did not keep the password, so keep it with the copy.'
                  : 'It is sealed with this controller\'s backup key, so the new controller needs the same REET_BACKUP_KEY to open it.'}
                {#if !features.copyOrigin}This controller does not record that a copy was taken fenced, so the new controller will ask you to say the old one is stopped.{/if}
              </p>
              <div class="acts">
                <a class="btn primary" href={backupDownloadUrl(true, copy.id)} download>Download the final copy</a>
                {#if !again}<button type="button" class="btn" onclick={() => (again = true)}>Take another</button>{/if}
              </div>
            {:else}
              <p class="bad">The fence changed while this copy was being taken, so it does not count as the final one. Take it again.</p>
            {/if}
          </div>
        {/if}
        {#if !copyGood || again}
          <form class="take" onsubmit={take}>
            {#if features.copyPassword}
              <label class="field" for="move-copy-password"><span>Copy password</span></label>
              <PasswordInput id="move-copy-password" bind:value={password} autocomplete="new-password" required={false} minlength={COPY_PASSWORD_MIN} describedby="move-password-hint" />
              <small id="move-password-hint" class:bad={!!passwordBad}>
                {passwordBad || `At least ${COPY_PASSWORD_MIN} characters. It seals this one copy and is not stored here: whoever opens the copy on the new controller types it there.`}
              </small>
            {:else}
              <p class="muted">This controller cannot seal one copy with a password of its own, so the copy is sealed as its other copies are: with the stored copy password if there is one, otherwise with the backup key.</p>
            {/if}
            <div class="acts">
              <button type="submit" class="btn primary" disabled={busy !== '' || !enabled || (features.copyPassword && (password === '' || !!passwordBad))} aria-busy={busy === 'copy'}>
                {busy === 'copy' ? 'Taking the copy…' : 'Take the final copy'}
              </button>
            </div>
            {#if !enabled}<p class="bad">No copy can be taken until the backup key is set.</p>{/if}
          </form>
        {/if}
      {/if}
    </li>

    <li>
      <h4><span class="marker" aria-hidden="true">3</span>On the new controller</h4>
      <ul>
        <li>Open <strong>Admin → Backups</strong> there and find <strong>Move a controller in</strong>.</li>
        <li>Give it the downloaded copy, the copy password and the address it answers at.</li>
        <li>Restart it to put the copy in place, then set by hand what its report lists as not in the copy.</li>
      </ul>
    </li>

    <li class:done={!!progress?.done} class:later={!fence.fenced}>
      <h4><span class="marker" aria-hidden="true">{progress?.done ? '✓' : '4'}</span>Machines follow on their own</h4>
      <p class="muted">
        Nothing to run on them. Each machine is told the new address on its next request here, keeps it, and moves once
        the new controller proves it holds the machine's record; only then does it send a credential. It then tells this
        controller it has gone.
      </p>
      {#if fence.fenced && features.fenceMachines}
        {#if machinesError}<p class="bad" role="alert">{machinesError}</p>{/if}
        {#if progress && machines}
          {#if progress.total === 0}
            <p class="muted">No machines are enrolled here, so there is nothing to move.</p>
          {:else}
            <p class="tally" role="status">
              <strong>{progress.moved} of {progress.total}</strong> moved{#if progress.waiting}, {progress.waiting} {fence.movedTo ? 'waiting for the new controller' : 'waiting for the new address'}{/if}{#if progress.quiet + progress.unseen}, {progress.quiet + progress.unseen} not asking right now{/if}{#if progress.held}, {progress.held} need a hand{/if}.
            </p>
            <ul class="machines" aria-label="Machines">
              {#each machines as m (m.id)}
                {@const state = machineMoveState(m, now)}
                <li class="machine {state}">
                  <span class="dot" aria-hidden="true"></span>
                  <span class="host">{m.host}</span>
                  <span class="what">{moveStateLine(m, now, !!fence.movedTo)}</span>
                </li>
              {/each}
            </ul>
            {#if !fence.movedTo}<p class="bad">Give the new address in step 1; until then no machine knows where to go.</p>{/if}
          {/if}
        {:else if !machinesError}
          <p class="muted">Reading the machines…</p>
        {/if}
      {/if}
    </li>

    <li class:done={!!progress?.done}>
      <h4><span class="marker" aria-hidden="true">5</span>When everything works</h4>
      <p class="muted">
        Resume what the new controller held back; its report lists it. Keep this controller fenced and running until
        every machine shows moved: a machine switched off now learns the address from it when it is back. After that it
        can be retired.
      </p>
    </li>
  </ol>
</section>

{#if asking === 'raise'}
  <ConfirmDialog title="Fence this controller?"
    body="Every change is refused, here and from every machine, and background work stands still: scheduled copies, mail, project runs and Gaffer's reviews. Nothing is deleted. You can still sign in, take and download copies, and lift the fence to put this controller back as it was."
    confirmLabel="Fence this controller" oncancel={() => (asking = '')} onconfirm={() => void raise()} />
{/if}
{#if asking === 'lift'}
  <ConfirmDialog title="Lift the fence?"
    body="This controller takes changes again and its background work carries on from where it stood. If a copy of it is already running somewhere else, two controllers would work from one set of data, so lift the fence only when the move is off."
    confirmLabel="Lift the fence" oncancel={() => (asking = '')} onconfirm={() => void lift()} />
{/if}

<style>
  .block { margin-top: 24px; padding: clamp(16px, 3vw, 24px); border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  h3 { margin: 0; font: 500 1rem/1.3 var(--serif); letter-spacing: -0.01em; color: var(--ink); }
  .muted { margin: 6px 0 0; color: var(--muted); font-size: 0.88rem; max-width: 70ch; }
  .notice { margin: 16px 0 0; padding: 12px 16px; border: 1px solid var(--bad); border-radius: 8px; background: var(--card); overflow-wrap: anywhere; }
  .steps { list-style: none; margin: 16px 0 0; padding: 0; display: grid; }
  .steps > li { min-width: 0; padding: 16px 0; border-top: 1px solid var(--line-soft); }
  .steps > li:last-child { padding-bottom: 0; }
  .steps > li.later h4 { color: var(--muted); }
  h4 { display: flex; align-items: center; gap: 10px; margin: 0; font: 600 1rem/1.3 var(--sans, inherit); }
  .marker { display: grid; place-items: center; flex: none; width: 24px; height: 24px; border: 1px solid var(--line-strong); border-radius: 50%; font-size: 0.875rem; font-variant-numeric: tabular-nums; color: var(--muted); }
  li.done .marker { background: color-mix(in srgb, var(--good) 16%, var(--card)); border-color: color-mix(in srgb, var(--good) 45%, var(--line)); color: var(--good); }
  .steps p { margin: 8px 0 0; overflow-wrap: anywhere; }
  .steps ul { margin: 8px 0 0; padding-left: 20px; display: grid; gap: 4px; color: var(--muted); font-size: 0.88rem; }
  .steps ul strong { color: var(--ink); }
  .field { display: grid; gap: 4px; margin-top: 12px; max-width: 28rem; font-size: 0.875rem; color: var(--muted); }
  .field input { width: 100%; min-width: 0; }
  small { display: block; margin-top: 4px; max-width: 28rem; color: var(--muted); font-size: 0.875rem; }
  .take { max-width: 28rem; }
  .acts { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  .copy { margin-top: 12px; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .copy p:first-child { margin-top: 0; }
  .copy-id { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .copy-id code { font-size: 0.875rem; overflow-wrap: anywhere; }
  .tag { display: inline-flex; padding: 1px 7px; border: 1px solid var(--line); border-radius: 6px; color: var(--muted); font-size: 0.875rem; }
  .tag.good { border-color: color-mix(in srgb, var(--good) 45%, var(--line)); color: var(--good); }
  .tag.bad { border-color: var(--bad); color: var(--bad); }
  .bad { color: var(--bad); overflow-wrap: anywhere; }
  .readdress { max-width: 28rem; }
  .tally { font-size: 0.88rem; }
  .steps .machines { list-style: none; margin: 8px 0 0; padding: 0; display: grid; gap: 0; color: inherit; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .steps .machines li { display: grid; grid-template-columns: auto minmax(0, 10rem) minmax(0, 1fr); align-items: baseline; gap: 10px; padding: 8px 12px; border-top: 1px solid var(--line-soft); font-size: 0.875rem; }
  .steps .machines li:first-child { border-top: 0; }
  .machines .host { color: var(--ink); font-weight: 600; overflow-wrap: anywhere; }
  .machines .what { color: var(--muted); overflow-wrap: anywhere; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--line-strong); align-self: center; }
  .machine.moved .dot { background: var(--good); }
  .machine.waiting .dot { background: var(--accent, var(--muted)); }
  .machine.held .dot { background: var(--bad); }
  .machine.held .what { color: var(--bad); }
  @media (max-width: 560px) { .steps .machines li { grid-template-columns: auto minmax(0, 1fr); row-gap: 2px; } .machines .what { grid-column: 2; } }
  @media (max-width: 560px) { .acts .btn { width: 100%; } }
  @media (pointer: coarse) { .acts .btn, .field input { min-height: 44px; } }
</style>
