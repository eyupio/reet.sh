// Where a hover preview sits beside the icon that opened it. It prefers the space below, flips above when the card
// would run off the bottom, and is kept a margin inside the viewport on every edge.
export type PreviewAnchor = { left: number; top: number; bottom: number; width: number; height: number }
export type PreviewSize = { width: number; height: number }
export type PreviewViewport = { width: number; height: number }
export type PreviewPlace = { left: number; top: number; placement: 'above' | 'below' }

const GAP = 10
const MARGIN = 8

export function placePreview(anchor: PreviewAnchor, size: PreviewSize, viewport: PreviewViewport, gap = GAP, margin = MARGIN): PreviewPlace {
  const roomBelow = viewport.height - anchor.bottom - gap - margin
  const roomAbove = anchor.top - gap - margin
  const placement = size.height > roomBelow && roomAbove > roomBelow ? 'above' : 'below'
  const wantedTop = placement === 'above' ? anchor.top - gap - size.height : anchor.bottom + gap
  const maxLeft = Math.max(margin, viewport.width - size.width - margin)
  const maxTop = Math.max(margin, viewport.height - size.height - margin)
  return {
    left: Math.min(Math.max(margin, anchor.left + anchor.width / 2 - size.width / 2), maxLeft),
    top: Math.min(Math.max(margin, wantedTop), maxTop),
    placement,
  }
}
