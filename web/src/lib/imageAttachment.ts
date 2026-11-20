// The images attached in Gaffer's goal and outcome modals. The limits match the
// existing machine upload path, but the modal accepts only the three types people
// expect for a reference picture; the terminal uploader's GIF support is not
// carried over. An outcome holds up to the same eight images one prompt can.
export const MAX_ATTACHMENT_BYTES = 5 << 20
export const MAX_ATTACHMENTS = 8
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

export type PastedAttachment = { ok: true; file: File } | { ok: false; reason: AttachmentReason } | null

/** The first image in a paste, if any. A paste without an image is left to the field itself. */
export function attachmentFromPaste(files: ArrayLike<File> | null | undefined): PastedAttachment {
  const found = attachmentsFromPaste(files)
  if (!found) return null
  if (!found.files.length) return { ok: false, reason: found.rejected[0] }
  return { ok: true, file: found.files[0] }
}

export type PastedAttachments = { files: File[]; rejected: AttachmentReason[] } | null

/**
 * Every image in a paste, in clipboard order, with the ones that cannot be
 * attached kept as reasons. A paste without an image is left to the field
 * itself, so text and files paste normally.
 */
export function attachmentsFromPaste(files: ArrayLike<File> | null | undefined): PastedAttachments {
  const images = Array.from(files ?? []).filter(file => file.type.startsWith('image/'))
  if (!images.length) return null
  const accepted: File[] = []
  const rejected: AttachmentReason[] = []
  for (const image of images) {
    const check = validateAttachment({ type: image.type, size: image.size })
    if (check.ok) accepted.push(image)
    else rejected.push(check.reason)
  }
  return { files: accepted, rejected }
}
