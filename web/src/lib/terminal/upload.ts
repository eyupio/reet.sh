export const MAX_IMAGE_BYTES = 5 << 20
export const ACCEPTED_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']

export type ImageValidation = { ok: true } | { ok: false; reason: 'type' | 'size' | 'empty' }

export interface ImageFacts {
  type: string
  size: number
}

export function validateImage(image: ImageFacts): ImageValidation {
  if (!image || !Number.isFinite(image.size) || image.size <= 0) return { ok: false, reason: 'empty' }
  if (!ACCEPTED_IMAGE_TYPES.includes(image.type)) return { ok: false, reason: 'type' }
  if (image.size > MAX_IMAGE_BYTES) return { ok: false, reason: 'size' }
  return { ok: true }
}

export function formatImageSize(bytes: number): string {
  const count = Number.isFinite(bytes) ? Math.max(0, Math.floor(bytes)) : 0
  if (count < 1024) return `${count} B`
  if (count < 1 << 20) return `${(count / 1024).toFixed(1)} KB`
  return `${(count / (1 << 20)).toFixed(1)} MB`
}

export function uploadInsertionAllowed(started: number, current: number, live: boolean, alive: boolean): boolean {
  return alive && live && started === current
}

export function joinReferences(references: string[]): string {
  return references.length ? ` ${references.join(' ')} ` : ''
}

export interface ImageItemLike {
  kind?: string
  type?: string
  getAsFile?: () => File | null
}

// Clipboard and drop item lists carry strings and files together. Only file
// items that call themselves images are returned; SVG and other image types
// still reach validation, so they are refused rather than pasted as text.
export function imageFiles(items: ArrayLike<ImageItemLike> | null | undefined): File[] {
  const files: File[] = []
  if (!items) return files
  for (let index = 0; index < items.length; index++) {
    const item = items[index]
    if (!item || (item.kind && item.kind !== 'file')) continue
    if (!item.type || !item.type.startsWith('image/')) continue
    const file = item.getAsFile?.()
    if (file) files.push(file)
  }
  return files
}
