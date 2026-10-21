import type { Flags, Skill } from './api'

/** Where a skill sits on the shelf, derived from its flags so the data model is unchanged. */
export type Stage = 'use' | 'try' | 'shelf'
export type Move = { label: string; flags: Flags; primary?: boolean }

export const stages: { id: Stage; title: string; hint: string; empty: string }[] = [
  { id: 'use', title: 'In use', hint: 'Earning their keep.', empty: 'Nowt in use yet. Mark a skill In use when it earns its place.' },
  { id: 'try', title: 'To test', hint: 'Give these a proper go.', empty: 'Nowt waiting for a go. Save a skill To test when it catches your eye.' },
  { id: 'shelf', title: 'On the shelf', hint: 'Good finds, saved for later.', empty: 'Nowt on the shelf yet. Paste a skill link above to get cracking.' },
]

export const stageOf = (s: Skill): Stage => (s.used ? 'use' : s.bookmarked ? 'try' : 'shelf')
export const stageTitle = (id: Stage): string => stages.find((s) => s.id === id)!.title

/** The buttons that move a skill along from the given stage. */
export function moves(stage: Stage): Move[] {
  switch (stage) {
    case 'shelf':
      return [
        { label: 'Save to test', flags: { bookmarked: true }, primary: true },
        { label: 'Start using', flags: { used: true } },
      ]
    case 'try':
      return [
        { label: 'Start using', flags: { used: true }, primary: true },
        { label: 'Back to shelf', flags: { bookmarked: false } },
      ]
    case 'use':
      return [{ label: 'Stop using', flags: { used: false } }]
  }
}

export const fmt = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1e3 ? (n / 1e3).toFixed(1) + 'K' : String(n))
