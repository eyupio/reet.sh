// What the Factory and the sessions are each waiting for, as one list on Home. Pure, so the page only draws it.
import { inSection, sectionSummary, waited } from '../factory/factoryList'
import type { FactoryItem } from '../factory/factoryList'
import type { NeedCard } from '../home'

export type WaitingEntry =
  | { source: 'factory'; id: string; since: number; item: FactoryItem }
  | { source: 'session'; id: string; since: number; card: NeedCard }

/** Pipelines waiting for the person and sessions needing them, oldest first. A finished session is not waiting, and a
 *  pipeline's own session shows once, as the pipeline. */
export function homeWaiting(items: FactoryItem[], cards: NeedCard[]): WaitingEntry[] {
  const pipelineSessions = new Set(items.map(item => item.sessionId).filter(Boolean))
  const entries: WaitingEntry[] = [
    ...inSection(items, 'waiting').filter(item => !item.archived).map((item): WaitingEntry => ({ source: 'factory', id: item.id, since: item.since, item })),
    ...cards.filter(card => card.kind !== 'finished' && !pipelineSessions.has(card.sessionId)).map((card): WaitingEntry => ({ source: 'session', id: `session:${card.sessionId}`, since: card.at, card })),
  ]
  return entries.sort((a, b) => a.since - b.since || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
}

type Label = 'decision' | 'approval' | 'merge' | 'question' | 'stopped' | 'failed' | 'review'
const ORDER: Label[] = ['decision', 'approval', 'merge', 'question', 'stopped', 'failed', 'review']
const WORDS: Record<Label, [string, string]> = {
  decision: ['decision', 'decisions'], approval: ['approval', 'approvals'], merge: ['merge', 'merges'], question: ['question', 'questions'],
  stopped: ['stopped', 'stopped'], failed: ['failed', 'failed'], review: ['to review', 'to review'],
}

function labelOf(entry: WaitingEntry): Label {
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
