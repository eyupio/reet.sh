// Reads the chat pane makes in the background: what is left of the agent's plan, and the models and favourites its
// picker offers. Each returns a function that retires it, so a late answer can never land after the pane moved on.
import type { MachineModels, ModelFavourite } from '../models'

/** Reads the plan now and again every `every` ms, handing each answer on while the poll is live. */
export function pollPlans<T>(read: () => Promise<T>, every: number, onAnswer: (answer: T) => void): () => void {
  let live = true
  const poll = () => void read().then(answer => { if (live) onAnswer(answer) })
  poll()
  const timer = setInterval(poll, every)
  return () => { live = false; clearInterval(timer) }
}

export const MODEL_LIST_UNAVAILABLE = 'Model list unavailable. Enter a model ID configured on this machine.'

export type ModelLoadOptions = {
  machineModels: (signal: AbortSignal) => Promise<MachineModels>
  favourites: (signal: AbortSignal) => Promise<{ favourites: ModelFavourite[] }>
  onMachine: (machine: MachineModels) => void
  onFavourites: (favourites: ModelFavourite[]) => void
  onNote: (note: string) => void
}

/** Reads the machine's models and the person's favourites together; a failed models read leaves a note, a failed favourites read nothing. */
export function loadModels(options: ModelLoadOptions): () => void {
  const life = new AbortController()
  void options.machineModels(life.signal).then(result => {
    if (!life.signal.aborted) options.onMachine(result)
  }).catch(() => { if (!life.signal.aborted) options.onNote(MODEL_LIST_UNAVAILABLE) })
  void options.favourites(life.signal).then(answer => { if (!life.signal.aborted) options.onFavourites(answer.favourites) }).catch(() => {})
  return () => life.abort()
}
