// Type in all panes, the gate: whether input from the focused pane may go to the other panes receiving, and sending it.
// Pure apart from the receivers it is handed, so the unit test drives it with fake panes. Nothing here imports at run
// time: the paste review rule is passed in (paste.ts's pasteNeedsReview), so the unit test can load this file alone.

/** One pane of the tab as the gate sees it, read at the moment of sending. */
export type Receiver = {
  /** The connection is live and its socket open: bytes handed over now are sent now. */
  canSend(): boolean
  /** Earlier output is open: the pane takes no input. */
  scrollback(): boolean
  /** The program in the pane takes a paste as one block. */
  bracketed(): boolean
  /** Sends typed text, or a paste through the pane's own paste; true only once the bytes were handed to the socket. */
  send(data: string, paste: boolean): boolean
}
/** The mode as it was turned on: the tab, its shape and the panes receiving. */
export type Mode = { tab: string; shape: string; set: string[] }
/** The tab as it is now: its id, its shape, the panes that could receive and each pane's terminal (when mounted). */
export type Now = {
  tab: string; shape: string; receivers: string[]
  /** The pane is still on the tab. */
  present: (id: string) => boolean
  /** The pane's terminal, while it is mounted. */
  pane: (id: string) => Receiver | undefined
  name: (id: string) => string
}
/** Whether a paste of `text` would be held for review in a program with or without bracketed paste. */
export type Review = (text: string, bracketed: boolean) => boolean

export const STOP_CHANGED = 'Stopped typing in all panes: the panes on this tab changed.'
export const stopDropped = (name: string) => `Stopped typing in all panes: ${name} is no longer connected.`
export const stopLineByLine = (name: string) => `Stopped typing in all panes: ${name} would run this paste line by line.`
export const stopPartial = (names: string[]) => `Stopped typing in all panes: only ${listOf(names)} received the last input.`
function listOf(names: string[]): string {
  return names.length <= 1 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`
}

/** Why the mode cannot go on, or '' when it can. A pane of the set still on the tab, in the same place and not reading
 *  earlier output, that cannot be sent to is named; anything else is the tab changing. */
export function stopReason(mode: Mode, now: Now): string {
  if (now.tab !== mode.tab || now.shape !== mode.shape) return STOP_CHANGED
  for (const id of mode.set) {
    const r = now.pane(id)
    if (r?.scrollback()) return STOP_CHANGED
    if (now.present(id) && (!r || !r.canSend())) return stopDropped(now.name(id))
  }
  const same = now.receivers.length === mode.set.length && mode.set.every(id => now.receivers.includes(id) && now.present(id))
  return same ? '' : STOP_CHANGED
}

/** The outcome of one input: the panes it reached besides the source, and the notice when the mode must stop. */
export type Outcome = { sent: string[]; stop: string }

/** Sends input the source has already sent to its own shell on to every other pane of the set. Everything is checked
 *  first, as it is at this moment: the tab, every pane's connection and, for a paste, every other pane's own paste rule.
 *  Anything wrong sends nothing to any other pane. A send that still fails partway stops the mode and names the panes
 *  that did get the input (the source first). */
export function sendToAll(mode: Mode, now: Now, from: string, data: string, paste: boolean, review: Review): Outcome {
  if (!mode.set.includes(from)) return { sent: [], stop: '' }
  const stop = stopReason(mode, now)
  if (stop) return { sent: [], stop }
  const others = mode.set.filter(id => id !== from)
  if (paste) {
    for (const id of others) {
      const r = now.pane(id)!
      if (review(data, r.bracketed())) return { sent: [], stop: stopLineByLine(now.name(id)) }
    }
  }
  const sent: string[] = []
  for (const id of others) {
    if (now.pane(id)!.send(data, paste)) { sent.push(id); continue }
    return { sent, stop: stopPartial([from, ...sent].map(now.name)) }
  }
  return { sent, stop: '' }
}
