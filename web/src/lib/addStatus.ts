// Community's Save and a list's Add all to my library add skills as Active, preselected, or Archived. Active goes on
// every machine; Archived stays in the library, off the machines.
export type AddStatus = 'active' | 'archived'

const skills = (n: number) => `${n} skill${n === 1 ? '' : 's'}`
const where = (status: AddStatus) => (status === 'active' ? 'as Active · going on every machine' : 'as Archived · kept off your machines')

/** What Add all asks before it sends: how many skills, and on how many machines. `machines` is null when unknown. */
export function addAllTitle(count: number, machines: number | null, status: AddStatus): string {
  if (status === 'archived') return `Add ${skills(count)} to your library as Archived?`
  if (machines === null) return `Put ${skills(count)} on every machine?`
  if (machines === 0) return `Add ${skills(count)} to your library?`
  if (machines === 1) return `Put ${skills(count)} on your machine?`
  return `Put ${skills(count)} on all ${machines} machines?`
}

/** What a choice of status means, under the choice. */
export const statusHint: Record<AddStatus, string> = { active: 'On every machine', archived: 'In your library, off your machines' }

export function savedNotice(name: string, status: AddStatus): string {
  return `Saved ${name} to your library ${where(status)}`
}

export function addedNotice(count: number, status: AddStatus): string {
  if (count === 0) return 'Every skill was already in your library. Nothing changed.'
  return `Added ${skills(count)} to your library ${where(status)}`
}
