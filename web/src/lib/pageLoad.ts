import { inflight } from './inflight'

// Chunk downloads participate in the same delayed progress bar as API reads.
export async function loadPage<T>(load: () => Promise<T>): Promise<T> {
  const end = inflight.begin()
  try { return await load() }
  finally { end() }
}
