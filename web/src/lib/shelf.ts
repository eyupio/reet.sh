import type { Flags, Skill } from './api'

/** Where a skill sits on the shelf, derived from its flags so the data model is unchanged. */
export type Stage = 'use' | 'try' | 'shelf'
export type Move = { label: string; flags: Flags; primary?: boolean }

export const stages: { id: Stage; title: string; hint: string; empty: string }[] = [
  { id: 'use', title: 'In use', hint: 'The ones you actually reach for.', empty: 'Nowt in use yet.' },
  { id: 'try', title: 'To test', hint: 'Saved to give a go.', empty: 'Nowt waiting to be tested.' },
  { id: 'shelf', title: 'On the shelf', hint: 'Spotted, not sorted yet.', empty: 'Nowt else on the shelf.' },
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
