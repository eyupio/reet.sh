// How the session workspace's context panel sits beside the terminal: its
// width, the separator's drag and keys, and the choice saved in this browser.
// Kept free of Svelte so node:test can run it.

export const CONTEXT_MIN = 300, CONTEXT_MAX = 520, CONTEXT_DEFAULT = 360
/** The panel never takes more than this share of the workspace, so the terminal keeps at least 60 %. */
export const CONTEXT_SHARE = 0.4
/** Keyboard step for the separator's arrow keys. */
export const CONTEXT_KEY_STEP = 16

/** The width saved for the panel; each choice is left undefined until the person makes it. */
export type SavedContext = { open: boolean | undefined; width: number | undefined }

/** The widest the panel may be in a workspace this wide; an unmeasured (0) workspace only has the absolute limit. */
export function widestContext(containerWidth: number): number {
  return containerWidth > 0 ? Math.min(CONTEXT_MAX, Math.floor(containerWidth * CONTEXT_SHARE)) : CONTEXT_MAX
}

/** The panel width to draw: 300 to 520 px, and never more than 40 % of the workspace. */
export function clampContextWidth(requested: number, containerWidth: number): number {
  const wanted = Number.isFinite(requested) ? Math.round(requested) : CONTEXT_DEFAULT
  return Math.min(widestContext(containerWidth), Math.max(CONTEXT_MIN, wanted))
}

/**
 * The separator's keys, laid out the way the panel moves: Left widens the panel, Right narrows it, Home reaches the
 * narrowest width and End the widest. Anything else is not a resize and returns null.
 */
export function contextKeyWidth(key: string, width: number, containerWidth: number): number | null {
  const current = clampContextWidth(width, containerWidth)
  if (key === 'Home') return clampContextWidth(CONTEXT_MIN, containerWidth)
  if (key === 'End') return widestContext(containerWidth)
  if (key === 'ArrowLeft') return clampContextWidth(current + CONTEXT_KEY_STEP, containerWidth)
  if (key === 'ArrowRight') return clampContextWidth(current - CONTEXT_KEY_STEP, containerWidth)
  return null
}

/** Reads the saved layout; anything unreadable falls back to the defaults. */
export function readContextLayout(raw: string | null | undefined): SavedContext {
  const layout: SavedContext = { open: undefined, width: undefined }
  let saved: unknown
  try { saved = JSON.parse(raw || '{}') } catch { return layout }
  if (!saved || typeof saved !== 'object' || Array.isArray(saved)) return layout
  const record = saved as Record<string, unknown>
  if (typeof record.open === 'boolean') layout.open = record.open
  if (typeof record.width === 'number' && Number.isFinite(record.width)) layout.width = Math.round(Math.min(CONTEXT_MAX, Math.max(CONTEXT_MIN, record.width)))
  return layout
}

/** What is saved: whether the docked panel is open, and its width. */
export function contextLayoutRecord(open: boolean, width: number) {
  return { open, width: Math.round(Math.min(CONTEXT_MAX, Math.max(CONTEXT_MIN, width))) }
}
