// What the Factory and the sessions are each waiting for, as one list on Home. Pure, so the page only draws it.
import { inSection, sectionSummary, waited } from '../factory/factoryList'
import type { FactoryItem } from '../factory/factoryList'
import type { NeedCard } from '../home'

/** A Gaffer goal that needs review on a session, as the list shows it: the session's title and place, and Gaffer's reason. */
export type GoalNeed = { id: string; sessionId: string; title: string; where: string; href: string; reason: string; since: number }

export type WaitingEntry =
  | { source: 'factory'; id: string; since: number; item: FactoryItem; approval?: NeedCard['approval'] }
  | { source: 'session'; id: string; since: number; card: NeedCard }
  | { source: 'gaffer'; id: string; since: number; goal: GoalNeed }

/** Pipelines waiting for the person, sessions needing them and Gaffer's goals that need review, oldest first. A finished
 *  session is not waiting. A session's card gives way to its pipeline's item only while that item waits and shows what
 *  the card asks: an approval the item does not carry keeps the card. A goal shows only on a session that has no card
 *  in the list and no pipeline (a card already says the goal needs the person; a pipeline's goal is its item). */
export function homeWaiting(items: FactoryItem[], cards: NeedCard[], goals: GoalNeed[] = []): WaitingEntry[] {
  const waiting = inSection(items, 'waiting').filter(item => !item.archived)
  const waitingBySession = new Map(waiting.filter(item => item.sessionId).map(item => [item.sessionId, item]))
  const shownByItem = (card: NeedCard) => { const item = waitingBySession.get(card.sessionId); return !!item && !(card.approval && item.kind !== 'approval') }
  const pipelineSessions = new Set(items.map(item => item.sessionId).filter(Boolean))
  // Only a card that is in the list can speak for its goal: a finished session's card is not.
  const carded = new Set(cards.filter(card => card.kind !== 'finished').map(card => card.sessionId))
  const entries: WaitingEntry[] = [
    ...waiting.map((item): WaitingEntry => ({ source: 'factory', id: item.id, since: item.since, item })),
    ...cards.filter(card => card.kind !== 'finished' && !shownByItem(card)).map((card): WaitingEntry => ({ source: 'session', id: `session:${card.sessionId}`, since: card.at, card })),
    ...goals.filter(goal => !carded.has(goal.sessionId) && !pipelineSessions.has(goal.sessionId)).map((goal): WaitingEntry => ({ source: 'gaffer', id: `gaffer:${goal.id}`, since: goal.since, goal })),
  ]
  return entries.sort((a, b) => a.since - b.since || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
}

type Label = 'decision' | 'approval' | 'merge' | 'question' | 'stopped' | 'failed' | 'review' | 'gaffer'
const ORDER: Label[] = ['decision', 'approval', 'merge', 'question', 'stopped', 'failed', 'review', 'gaffer']
const WORDS: Record<Label, [string, string]> = {
  decision: ['decision', 'decisions'], approval: ['approval', 'approvals'], merge: ['merge', 'merges'], question: ['question', 'questions'],
  stopped: ['stopped', 'stopped'], failed: ['failed', 'failed'], review: ['to review', 'to review'], gaffer: ['Gaffer goal', 'Gaffer goals'],
}

function labelOf(entry: WaitingEntry): Label {
  if (entry.source === 'gaffer') return 'gaffer'
  if (entry.source === 'factory') {
    const kind = entry.item.kind
    return kind === 'approval' || kind === 'merge' || kind === 'stopped' || kind === 'failed' ? kind : 'decision'
  }
  const { card } = entry
  if (card.kind === 'failed') return 'failed'
  if (card.kind === 'review') return 'review'
  return card.approval || card.reasons.includes('approval') ? 'approval' : 'question'
}

/** "1 decision, 2 approvals · oldest 1 h": how many of each kind wait, and for how long the oldest has. */
export function waitingSummary(entries: WaitingEntry[], now: number): string {
  if (!entries.length) return ''
  const counts = new Map<Label, number>()
  for (const entry of entries) counts.set(labelOf(entry), (counts.get(labelOf(entry)) ?? 0) + 1)
  const parts = ORDER.filter(label => counts.has(label)).map(label => `${counts.get(label)} ${WORDS[label][counts.get(label) === 1 ? 0 : 1]}`)
  return `${parts.join(', ')} · oldest ${waited(now - Math.min(...entries.map(entry => entry.since)))}`
}

/** The pipelines that are working, named as the Factory page names them. */
export function workingSummary(items: FactoryItem[]): string {
  return sectionSummary(items, 'working', 0)
}
