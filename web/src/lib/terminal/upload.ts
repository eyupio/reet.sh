export const MAX_IMAGE_BYTES = 5 << 20
export const ACCEPTED_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']
export const MAX_IMAGES = 20
/** ACP prompts carry at most eight image content blocks (the same ceiling the
 *  reference agent UI uses); more than that fall back to readable paths. */
export const MAX_EMBEDDED_IMAGES = 8
export const MAX_IMAGE_TOTAL_BYTES = 50 << 20

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

/** The opening prompt with uploaded image paths appended. The words keep a
 *  one-word prompt from being read as a subcommand, and the agent still sees
 *  each path as plain text it can open. */
export function imagePrompt(prompt: string, references: string[]): string {
  const refs = references.filter(Boolean)
  if (!refs.length) return prompt
  const line = refs.length === 1 ? `Image: ${refs[0]}` : `Images: ${refs.join(' ')}`
  const text = String(prompt ?? '').replace(/\s+$/, '')
  return text ? `${text}\n\n${line}` : line
}

const IMAGE_REF_LINE = /(?:^|\n+)[ \t]*Images?: ((?:\S+\.(?:png|jpe?g|gif|webp)(?:[ \t]+|$))+)$/i

/** The inverse of imagePrompt: a sent message that ends in an `Image:` / `Images:` line is the words plus the uploaded
 *  paths. The chat shows the paths as thumbnails rather than as text. */
export function splitImageRefs(text: string): { text: string; refs: string[] } {
  const source = String(text ?? '')
  const match = IMAGE_REF_LINE.exec(source)
  if (!match) return { text: source, refs: [] }
  const refs = match[1].trim().split(/\s+/).filter(Boolean)
  return { text: source.slice(0, match.index).replace(/\s+$/, ''), refs }
}

/** The last path segment, for a label where the whole upload path would be noise. */
export function baseName(path: string): string {
  const parts = String(path ?? '').split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] ?? ''
}

export interface UploadedImage {
  path: string
  reference: string
}

/** The prompt body sent to a structured chat. A machine that negotiated
 *  acp-image-v1 receives the upload paths and embeds the images in the prompt;
 *  an older machine gets the path as text, as before. */
export function chatPromptParts(text: string, uploads: UploadedImage[], embedImages: boolean): { text: string; images: string[] } {
  if (embedImages && uploads.length && uploads.length <= MAX_EMBEDDED_IMAGES) return { text, images: uploads.map(upload => upload.path) }
  return { text: imagePrompt(text, uploads.map(upload => upload.reference)), images: [] }
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
