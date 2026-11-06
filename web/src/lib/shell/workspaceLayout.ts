// The widths and collapse rules for the two left-hand panels on Home: the
// workspace navigation sidebar (fixed to the window) and the project rail
// (a column of Home). No browser needed, so the rules are unit tested.

export const SIDEBAR_MIN = 192
export const SIDEBAR_MAX = 360
export const SIDEBAR_DEFAULT = 228
/** The sidebar's content margin on the far left, beside the 1 px rule. */
export const SIDEBAR_GUTTER = 36

export const RAIL_MIN = 240
export const RAIL_MAX = 460
export const RAIL_DEFAULT = 301
/** The rail folded to its strip of project initials. */
export const RAIL_STRIP = 56
/** The rail never takes more than this share of the window, so the board keeps the rest. */
export const RAIL_SHARE = 0.4

/** Keyboard step for either panel's separator. */
export const KEY_STEP = 16

/** A sidebar width: 192 to 360 px, rounded, with the default for a width that is missing or unreadable. */
export function clampSidebarWidth(requested: number): number {
  const wanted = Number.isFinite(requested) ? Math.round(requested) : SIDEBAR_DEFAULT
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, wanted))
}

/** A rail width: 240 to 460 px and never more than 40 % of the window, so the board keeps at least 60 %. */
export function clampRailWidth(requested: number, viewportWidth = 0): number {
  const wanted = Number.isFinite(requested) ? Math.round(requested) : RAIL_DEFAULT
  const widest = viewportWidth > 0 ? Math.min(RAIL_MAX, Math.floor(viewportWidth * RAIL_SHARE)) : RAIL_MAX
  return Math.max(RAIL_MIN, Math.min(widest, wanted))
}

/** The track the sidebar takes: nothing folded away, otherwise its clamped width. */
export function sidebarTrack(collapsed: boolean, width: number): number {
  return collapsed ? 0 : clampSidebarWidth(width)
}

/** The rail's grid track: its strip while folded, otherwise its clamped width. */
export function railTrack(collapsed: boolean, width: number, viewportWidth = 0): number {
  return collapsed ? RAIL_STRIP : clampRailWidth(width, viewportWidth)
}

/** A stored width, brought into range; a missing or unreadable value keeps the fallback. */
export function readWidth(raw: string | null | undefined, fallback: number, clamp: (width: number) => number): number {
  if (raw === null || raw === undefined || raw.trim() === '') return fallback
  const value = Number(raw)
  return Number.isFinite(value) ? clamp(value) : fallback
}

/** A stored fold: only "1" folds a panel away. */
export function readCollapsed(raw: string | null | undefined): boolean {
  return raw === '1'
}

/**
 * A separator's keyboard step. ArrowRight widens either panel, ArrowLeft narrows it, Home asks for the minimum and End
 * for the maximum. Null means the key is not the separator's.
 */
export function keyWidth(key: string, current: number, min: number, max: number, step = KEY_STEP): number | null {
  if (key === 'ArrowRight') return Math.min(max, current + step)
  if (key === 'ArrowLeft') return Math.max(min, current - step)
  if (key === 'Home') return min
  if (key === 'End') return max
  return null
}
