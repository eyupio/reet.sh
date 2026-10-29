export const MAX_PASTE_BYTES = 240 * 1024
export const MAX_REVIEW_BYTES = 64 * 1024
export const MAX_REVIEW_CHARS = 2000

export type PasteReviewReason = 'execute' | 'control' | 'large' | 'newline' | 'long' | ''
export interface PasteReviewOptions { bracketed?: boolean; execute?: boolean; force?: boolean }
export interface PasteReview { review: boolean; reason: PasteReviewReason }

export function utf8ByteLength(text: string): number {
  return new TextEncoder().encode(text).length
}

export function pasteTooLarge(text: string): boolean {
  return utf8ByteLength(text) > MAX_PASTE_BYTES
}

// A pasted payload must never terminate its own bracket or carry terminal
// escape sequences. Strip the two markers first, then every remaining ESC.
export function sanitizePaste(text: string): string {
  return text.replace(/\x1b\[20[01]~/g, '').replace(/\x1b/g, '')
}

// Bracketed paste makes newlines inert, so an ordinary multi-line block can go
// straight in. Everything else keeps the explicit review.
export function pasteNeedsReview(text: string, options: PasteReviewOptions = {}): PasteReview {
  const review = (reason: PasteReviewReason): PasteReview => ({ review: true, reason })
  if (options.execute || options.force) return review('execute')
  if (/[\x00-\x08\x0b-\x1f\x7f-\x9f]/.test(text)) return review('control')
  if (utf8ByteLength(text) > MAX_REVIEW_BYTES) return review('large')
  if (!options.bracketed) {
    if (/[\r\n]/.test(text)) return review('newline')
    if (text.length > MAX_REVIEW_CHARS) return review('long')
  }
  return { review: false, reason: '' }
}
