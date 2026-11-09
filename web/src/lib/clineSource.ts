// Cline's billing sources. Cline reports its provider as an id; reet labels the ones it knows and shows any other id
// as Cline reports it. Everything here is pure so the live chat, Start an agent and Settings share one set of rules.

export type SourceChoice = { id: string; label: string }

export const KNOWN_SOURCES: readonly SourceChoice[] = [
  { id: 'cline', label: 'Usage-Billing' },
  { id: 'cline-pass', label: 'ClinePass' },
]

export function sourceLabel(id: string): string {
  return KNOWN_SOURCES.find(source => source.id === id)?.label ?? id
}

/** The sources a person can choose between. One value is not a choice, so it offers nothing. */
export function sourceChoices(values: readonly string[]): SourceChoice[] {
  if (values.length < 2) return []
  return values.map(id => ({ id, label: sourceLabel(id) }))
}

export type StartSourceInput = { explicit?: string; saved?: string; offered: readonly string[] }

/** The source Start an agent opens on: the person's own choice (an empty one is Cline's own), then the saved default
 *  while this machine still offers it, then Cline's own. A saved source that is gone says so and never blocks Start. */
export function resolveStartSource({ explicit, saved, offered }: StartSourceInput): { source: string; note: string } {
  if (explicit !== undefined) return { source: explicit, note: '' }
  if (!saved) return { source: '', note: '' }
  if (offered.includes(saved)) return { source: saved, note: '' }
  return { source: '', note: `Your saved source, ${sourceLabel(saved)}, isn’t available on this machine, so Cline’s own choice is used.` }
}

export type CreditGuidanceInput = { acpModel: boolean; offered: readonly string[]; current: string }

const names = (labels: string[]): string => labels.length < 2 ? (labels[0] ?? '') : `${labels.slice(0, -1).join(', ')} or ${labels[labels.length - 1]}`

/** What to do when a chat stops for lack of credit: switch to another source when the session offers one, otherwise
 *  choose another model or add credits. */
export function creditGuidance({ acpModel, offered, current }: CreditGuidanceInput): string {
  const others = offered.filter(id => id !== current).map(sourceLabel)
  if (others.length > 0) return `This source has no credit. Switch to ${names(others)} above, or add credits with Cline, then send a message to continue.`
  const head = 'This model has reached its usage or credit limit. '
  return acpModel
    ? `${head}Choose another model below, or add credits with your provider, then send a message to continue.`
    : `${head}Add credits with your provider, then send a message to continue.`
}
