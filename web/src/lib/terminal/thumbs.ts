// Small copies of the images a person sent in a chat, kept in this browser so the conversation shows pictures, not upload
// paths, even after the page reloads. The machine holds the real file; this is only a preview and is always optional.
const KEY = 'reet.chat.thumbs.v1'
const MAX_ENTRIES = 40
const MAX_ENTRY_CHARS = 160_000

interface Stored { order: string[]; map: Record<string, { n: string; u: string }> }
let cache: Stored | undefined

function read(): Stored {
  if (cache) return cache
  try {
    const parsed = JSON.parse(localStorage.getItem(KEY) ?? '') as Stored
    if (parsed && Array.isArray(parsed.order) && parsed.map && typeof parsed.map === 'object') return (cache = parsed)
  } catch { /* nothing stored, or storage is unavailable */ }
  return (cache = { order: [], map: {} })
}

function write(next: Stored) {
  cache = next
  for (;;) {
    try { localStorage.setItem(KEY, JSON.stringify(next)); return } catch { /* full or blocked */ }
    const oldest = next.order.shift()
    if (oldest === undefined) return
    delete next.map[oldest]
  }
}

/** A downscaled JPEG data URL of the image, or '' when the browser cannot draw it. */
export async function makeThumb(file: Blob, max = 720): Promise<string> {
  try {
    const bitmap = await createImageBitmap(file)
    const scale = Math.min(1, max / Math.max(bitmap.width, bitmap.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(bitmap.width * scale)); canvas.height = Math.max(1, Math.round(bitmap.height * scale))
    const context = canvas.getContext('2d')
    if (!context) return ''
    context.fillStyle = '#fff'; context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    bitmap.close?.()
    return canvas.toDataURL('image/jpeg', 0.8)
  } catch { return '' }
}

export function saveThumb(path: string, name: string, url: string) {
  if (!path || !url || url.length > MAX_ENTRY_CHARS) return
  const state = read()
  const order = state.order.filter(item => item !== path)
  order.push(path)
  const map = { ...state.map, [path]: { n: name, u: url } }
  while (order.length > MAX_ENTRIES) delete map[order.shift() as string]
  write({ order, map })
}

export function loadThumb(path: string): { url: string; name: string } | null {
  const hit = read().map[path]
  return hit ? { url: hit.u, name: hit.n } : null
}
