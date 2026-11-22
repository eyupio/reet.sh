// The Factory page's list: every pipeline of every outcome, sorted into what waits for the person, what is working and
// what is finished. Pure, so the page only draws it.
import { track, waitingDecisions } from '../gafferOutcome'
import type { Need, TrackNode } from '../gafferOutcome'

export type FactorySection = 'waiting' | 'working' | 'finished'
export type ItemKind = 'decision' | 'approval' | 'delivery' | 'merge' | 'stopped' | 'failed' | 'running' | 'checking' | 'queued' | 'done' | 'halted'
export type FactoryItem = {
  id: string
  /** The session the pipeline has in hand, '' when none. */
  sessionId: string
  outcomeId: string; key: string; title: string; objective: string; projectId: string
  index: number; of: number
  section: FactorySection; kind: ItemKind; chip: string
  why: string
  need?: Need
  since: number
  nodes: TrackNode[]; at: number
  archived: boolean
}

/** The parts of an outcome record the list reads. A `GafferOutcome` is one. */
export type ListRecord = {
  id: string; objective: string; projectId: string; archivedAt?: number
  sessions: {
    key: string; title: string; state: string; sessionState?: string; sessionId?: string; conversationId?: string
    stage?: number; updatedAt: number; note?: string
    /** What the outcome list carries of a pipeline's delivery review, enough to say its status without reading the machine. */
    criteria?: readonly unknown[]; deliveries?: readonly { decision: string }[]
    stages?: { kind: string; part?: string; tool: string; model?: string; effort?: string; effortNotOffered?: string; provider?: string; tier?: string; tierWhy?: string; balanced?: string; nextUsageBackup?: string; appliedModel?: string; agentState?: string; appliedEffort?: string; choiceCheck?: string; choiceNote?: string; state: string; summary?: string; closedBy?: string }[]
  }[]
}
/** The parts of a goal overview the list reads. A `GafferGoalOverview` is one. */
export type ListGoal = {
  attention: boolean; reason?: string; workerUsageLimited?: boolean
  goal: { state: string; note: string; waitingOnWorker?: boolean; reviewFailed?: boolean; noStep?: boolean; advice?: { choice: string; why: string; text?: string }; updatedAt: number }
}

const CHIPS: Record<ItemKind, string> = { decision: 'Decision', approval: 'Approval', delivery: 'Delivery review', merge: 'Ready to merge', stopped: 'Agent stopped', failed: 'Failed', running: 'Running', checking: 'Checking', queued: 'Waiting its turn', done: 'Finished', halted: 'Stopped' }
const SECTIONS: Record<ItemKind, FactorySection> = { decision: 'waiting', approval: 'waiting', delivery: 'waiting', merge: 'waiting', stopped: 'waiting', failed: 'waiting', running: 'working', checking: 'working', queued: 'working', done: 'finished', halted: 'finished' }

/** Every pipeline as one item, in record order. `permissions` is the set of session ids that hold a pending permission. */
export function factoryItems(records: ListRecord[], goals: ReadonlyMap<string, ListGoal>, permissions: ReadonlySet<string> = new Set()): FactoryItem[] {
  const items: FactoryItem[] = []
  for (const record of records) {
    const decisions = new Map(waitingDecisions(record, goals as Map<string, ListGoal>).map(d => [d.piece.key, d]))
    record.sessions.forEach((piece, index) => {
      const goal = piece.conversationId ? goals.get(piece.conversationId) : undefined
      const decision = decisions.get(piece.key)
      let kind: ItemKind
      if (decision) kind = decision.kind === 'merge' ? 'merge' : decision.kind === 'delivery' ? 'delivery' : decision.kind === 'ended' ? 'stopped' : goal?.goal.waitingOnWorker && piece.sessionId && permissions.has(piece.sessionId) ? 'approval' : 'decision'
      else if (piece.state === 'failed') kind = 'failed'
      else if (piece.state === 'done') kind = 'done'
      else if (piece.state === 'cancelled' || piece.state === 'unfollowed') kind = 'halted'
      else if (piece.state === 'waiting') kind = 'queued'
      else kind = 'running'
      const stages = piece.stages ?? []
      // The pipeline stays "following" between the agent ending its turn and Gaffer reviewing that turn, so its note
      // still says the stage is under way. The session itself already reads Finished everywhere else; say what is
      // actually happening instead of Running.
      const inHand = stages[Math.min(piece.stage ?? 0, stages.length - 1)]
      if (kind === 'running' && piece.state === 'following' && inHand?.agentState === 'finished') kind = 'checking'
      const why = decision ? decision.need.why
        : kind === 'checking' ? `The ${inHand.part ? `“${inHand.part}” part of the ` : ''}${inHand.kind} stage’s agent finished its turn. Gaffer is checking the work before it moves on.`
        : piece.note ?? ''
      items.push({
        id: `${record.id}.${piece.key}`, sessionId: piece.sessionId ?? '', outcomeId: record.id, key: piece.key, title: piece.title, objective: record.objective, projectId: record.projectId,
        index: index + 1, of: record.sessions.length, section: SECTIONS[kind], kind, chip: decision?.kind === 'usage_limit' ? 'Usage limit' : CHIPS[kind],
        why,
        ...(decision ? { need: decision.need } : {}),
        since: goal?.goal.updatedAt || piece.updatedAt,
        nodes: track(piece), at: stages.length ? Math.min(piece.stage ?? 0, stages.length - 1) : -1,
        archived: !!record.archivedAt,
      })
    })
  }
  return items
}

/** The items of one section. What waits comes oldest first; the others stay in record order. */
export function inSection(items: FactoryItem[], section: FactorySection): FactoryItem[] {
  const own = items.filter(item => item.section === section)
  return section === 'waiting' ? own.map((item, i) => ({ item, i })).sort((a, b) => a.item.since - b.item.since || a.i - b.i).map(x => x.item) : own
}

/** The stage in hand and the next, in words. `dots` is the short form for a row of dots, `circles` the longer one. */
export function stageCaption(nodes: TrackNode[], at: number, size: 'dots' | 'circles'): string {
  if (at < 0 || at >= nodes.length) return ''
  // A pipeline with every stage done has none in hand.
  if (nodes.every(node => node.state === 'done')) return nodes.length === 1 ? 'Its one stage done' : `All ${nodes.length} stages done`
  const here = nodes[at].label, last = at === nodes.length - 1
  if (size === 'dots') return `${here} · ${at + 1} of ${nodes.length} · ${last ? 'the last' : `next ${nodes[at + 1].label}`}`
  const done = nodes.slice(0, at).map(node => node.label)
  const place = at === 0 ? (last ? 'the only stage' : 'the first stage') : ''
  return [`${here} in hand`, done.length ? `${done.join(', ')} done` : place, last && nodes.length > 1 ? 'the last stage' : `next ${nodes[at + 1]?.label ?? ''}`].filter(part => part && part !== 'next ').join(' · ')
}

export const waited = (ms: number): string => {
  const minutes = Math.max(0, Math.floor(ms / 60000))
  if (minutes < 1) return 'under a minute'
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  return hours < 48 ? `${hours} h` : `${Math.floor(hours / 24)} days`
}

/** What a closed section says about what is in it: the oldest wait for what needs the person, else the titles. */
export function sectionSummary(items: FactoryItem[], section: FactorySection, now: number): string {
  const own = items.filter(item => item.section === section)
  if (!own.length) return ''
  if (section === 'waiting') return `oldest ${waited(now - Math.min(...own.map(item => item.since)))}`
  const titles = own.slice(0, 3).map(item => item.title).join(', ')
  return own.length > 3 ? `${titles} and ${own.length - 3} more` : titles
}
