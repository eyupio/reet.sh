// A project's colour is an identity, not a state: it never means running,
// warning or offline. The palette is shared with the session rail so a
// colour chosen for a project can also mark its sessions. "" is no colour.

export type ProjectColour = '' | 'red' | 'amber' | 'green' | 'blue' | 'violet'

export const PROJECT_COLOURS: readonly { id: ProjectColour; name: string; hex: string }[] = [
  { id: '', name: 'No colour', hex: 'transparent' },
  { id: 'red', name: 'Red', hex: '#d95757' },
  { id: 'amber', name: 'Amber', hex: '#c48b2f' },
  { id: 'green', name: 'Green', hex: '#3f9c72' },
  { id: 'blue', name: 'Blue', hex: '#598ed7' },
  { id: 'violet', name: 'Violet', hex: '#a17ed1' },
]

const known = new Set<string>(PROJECT_COLOURS.map(c => c.id))

/** A stored value the palette still knows, else no colour. */
export function projectColour(value: string | undefined): ProjectColour {
  return value && known.has(value) ? value as ProjectColour : ''
}

/** The swatch for a colour id; no colour is nothing to draw, not black. */
export function projectColourHex(value: string | undefined): string {
  return PROJECT_COLOURS.find(c => c.id === projectColour(value))?.hex ?? 'transparent'
}
