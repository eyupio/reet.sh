// The optional image attached in Gaffer's goal and outcome modals. The limits match the
// existing machine upload path, but the modal accepts only the three types people expect
// for a reference picture; the terminal uploader's GIF support is not carried over.
export const MAX_ATTACHMENT_BYTES = 5 << 20
export const ACCEPTED_ATTACHMENT_TYPES = ['image/png', 'image/jpeg', 'image/webp'] as const
export const ATTACHMENT_ACCEPT = ACCEPTED_ATTACHMENT_TYPES.join(',')

export type AttachmentReason = 'type' | 'size' | 'empty' | 'read'
export type AttachmentCheck = { ok: true } | { ok: false; reason: AttachmentReason }

export interface AttachmentFacts {
  type: string
  size: number
}

export function validateAttachment(file: AttachmentFacts | null | undefined): AttachmentCheck {
  if (!file || !Number.isFinite(file.size) || file.size <= 0) return { ok: false, reason: 'empty' }
  if (!ACCEPTED_ATTACHMENT_TYPES.includes(file.type as (typeof ACCEPTED_ATTACHMENT_TYPES)[number])) return { ok: false, reason: 'type' }
  if (file.size > MAX_ATTACHMENT_BYTES) return { ok: false, reason: 'size' }
  return { ok: true }
}

export function attachmentMessage(reason: AttachmentReason): string {
  if (reason === 'empty') return 'That image is empty.'
  if (reason === 'size') return 'That image is larger than 5 MiB.'
  if (reason === 'read') return 'That image could not be read.'
  return 'Attach a PNG, JPEG or WebP image.'
}
