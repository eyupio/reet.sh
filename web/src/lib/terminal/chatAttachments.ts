// What the chat composer does with pictures and with messages that wait to be sent: checking and holding attached images
// (and every object URL made for them), uploading them, matching a sent message to its pictures, and sending the queue one
// message at a time. The pane keeps the reactive state and passes in what each function needs.
import { dropQueuedPrompt, type QueuedPrompt } from '../chat'
import { MAX_IMAGE_TOTAL_BYTES, splitImageRefs, validateImage, type UploadedImage } from './upload'

export const MAX_CHAT_BYTES = 2000
/** The broker shows an image-only message as this placeholder; the thumbnails say it better. */
export const IMAGE_ONLY_ECHO = 'Image attached.'

export type Picture = { id: number; file: File; preview: string }
export type Preview = { url: string; name: string }

export const QUEUE_NOTES = {
  queued: 'Queued. It sends when the agent finishes this turn.',
  halted: 'The queued message was not sent. Retry, edit or remove it.',
  retry: 'Trying that message again…',
  full: 'The queue is full. Send or remove one of the messages first.',
}

/** What the person is told when a message is too long to send, or '' when it fits. */
export function messageProblem(text: string): string {
  if (new TextEncoder().encode(text).length > MAX_CHAT_BYTES) return 'That message is too long. Keep it under 2,000 bytes.'
  return ''
}

/** Why pictures cannot go with a message on this machine. */
export function imagesRefusal(uploadsOff: boolean): string {
  return uploadsOff ? 'Image uploads are off on that machine.' : 'That machine does not accept images.'
}

export function sentNote(remaining: number): string {
  return remaining ? 'Sent one queued message. The next waits for this turn.' : 'Queued message sent.'
}

const revokeUrl = (url: string) => URL.revokeObjectURL(url)

/** Check the files in order and keep each acceptable one, with an object URL for its preview. The first limit reached stops
 *  the rest; `refused` is the last thing the person should be told, or '' when everything was taken. */
export function admitImages(current: Picture[], files: File[], limit: number, lastId: number, makeUrl: (file: File) => string = file => URL.createObjectURL(file)): { images: Picture[]; lastId: number; refused: string } {
  let images = current
  let total = images.reduce((sum, item) => sum + item.file.size, 0)
  let refused = ''
  for (const file of files) {
    const check = validateImage(file)
    if (!check.ok) { refused = check.reason === 'type' ? 'Use PNG, JPEG, GIF or WebP images.' : check.reason === 'size' ? 'Each image must be 5 MB or smaller.' : 'That image is empty.'; continue }
    if (images.length >= limit) { refused = `Attach at most ${limit} images.`; break }
    if (total + file.size > MAX_IMAGE_TOTAL_BYTES) { refused = 'Those images are more than 50 MB together.'; break }
    images = [...images, { id: ++lastId, file, preview: makeUrl(file) }]
    total += file.size
  }
  return { images, lastId, refused }
}

export function revokePictures(pictures: readonly Picture[], revoke: (url: string) => void = revokeUrl) {
  for (const picture of pictures) revoke(picture.preview)
}

/** The pictures without the one removed; its object URL is revoked. */
export function removePicture(images: Picture[], id: number, revoke: (url: string) => void = revokeUrl): Picture[] {
  const item = images.find(image => image.id === id)
  if (item) revoke(item.preview)
  return images.filter(image => image.id !== id)
}

/** Pictures that were sent keep their object URLs until the pane closes: the history and the lightbox still show them. */
export function createPictureHold(revoke: (url: string) => void = revokeUrl) {
  const sent: string[] = []
  return {
    keep(pictures: readonly Picture[]) { for (const picture of pictures) sent.push(picture.preview) },
    /** Revokes every object URL still alive: attached, sent and queued pictures. */
    releaseAll(images: readonly Picture[], queued: readonly QueuedPrompt<Picture[]>[]) {
      revokePictures(images, revoke)
      for (const url of sent) revoke(url)
      for (const item of queued) revokePictures(item.payload, revoke)
    },
  }
}

/** Upload the pictures in order, saying which one is on its way when there are several. */
export async function uploadPictures(pictures: readonly Picture[], upload: (file: File) => Promise<UploadedImage>, progress: (note: string) => void): Promise<UploadedImage[]> {
  const uploads: UploadedImage[] = []
  progress('')
  for (let i = 0; i < pictures.length; i++) {
    if (pictures.length > 1) progress(`Uploading image ${i + 1} of ${pictures.length}…`)
    uploads.push(await upload(pictures[i].file))
  }
  return uploads
}

/** The picture held in this browser for each name its upload goes by (the machine's path and the prompt reference). */
export function previewsFor(uploads: readonly UploadedImage[], pictures: readonly Picture[]): Record<string, Preview> {
  const keep: Record<string, Preview> = {}
  uploads.forEach((upload, i) => {
    const image = pictures[i]
    for (const ref of [upload.path, upload.reference]) if (ref) keep[ref] = { url: image.preview, name: image.file.name }
  })
  return keep
}

/** What a sent message with pictures will look like when the stream echoes it back, or null when it carries none. */
export function echoOf(body: { text: string; images: string[] }, uploads: readonly UploadedImage[]): { text: string; refs: string[] } | null {
  return body.images.length ? { text: body.text.trim() || IMAGE_ONLY_ECHO, refs: uploads.map(upload => upload.path || upload.reference) } : null
}

/** With embedded images the echo carries no path; the text sent and its uploads wait here for the echo to arrive. */
export function createEchoes() {
  let waiting: { text: string; refs: string[] }[] = []
  const seen = new Set<string>()
  return {
    add(echo: { text: string; refs: string[] }) { waiting.push(echo) },
    remove(echo: { text: string; refs: string[] }) { waiting = waiting.filter(item => item !== echo) },
    /** The pictures of the message that arrived under this key, once; undefined when it was seen before or matches nothing. */
    claim(key: string, text: string): string[] | undefined {
      if (seen.has(key)) return undefined
      seen.add(key)
      const at = waiting.findIndex(echo => echo.text === text.trim())
      if (at < 0) return undefined
      return waiting.splice(at, 1)[0].refs
    },
  }
}

/** The text and pictures to show for a message the person sent: its bound pictures, else any the text itself names. */
export function shownAttachments(item: { text: string }, refs: string[] | null | undefined): { text: string; refs: string[] } {
  return refs ? { text: item.text.trim() === IMAGE_ONLY_ECHO ? '' : item.text, refs } : splitImageRefs(item.text)
}

/** A flag that clears itself after a while. */
export function createExpiry(ms: number, expire: () => void) {
  let timer: ReturnType<typeof setTimeout> | undefined
  return {
    arm() { clearTimeout(timer); timer = setTimeout(expire, ms) },
    clear() { clearTimeout(timer) },
  }
}

export type QueueDrainOptions = {
  queue: () => QueuedPrompt<Picture[]>[]
  /** Whether a send from the queue is in flight. */
  busy: () => boolean
  setBusy: (busy: boolean) => void
  /** One attempt at sending; true when the controller accepted it. */
  deliver: (text: string, pictures: Picture[]) => Promise<boolean>
  /** The first message was not sent: the queue stops until someone retries or changes it. */
  halted: (note: string) => void
  /** The first message went: what is left of the queue, and what to tell the person. */
  sent: (queue: QueuedPrompt<Picture[]>[], note: string, pictures: Picture[]) => void
  /** Follows the new message down the log; the queue is free again once it has. */
  follow: () => Promise<void>
}

/** Send the first queued message. One call sends one message; the pane decides when the agent is free to take it. */
export function createQueueDrain(options: QueueDrainOptions) {
  return async function dispatchQueued() {
    if (options.busy() || !options.queue().length) return
    const item = options.queue()[0]
    options.setBusy(true)
    try {
      const ok = await options.deliver(item.text, item.payload)
      if (!ok) { options.halted(QUEUE_NOTES.halted); return }
      const rest = dropQueuedPrompt(options.queue(), item.id)
      options.sent(rest, sentNote(rest.length), item.payload)
      await options.follow()
    } finally { options.setBusy(false) }
  }
}
