export type JsonObject = Record<string, unknown>;

const MAX_API_BYTES = 8 * 1024 * 1024;

export class ReetClient {
  private readonly base: URL;

  constructor(url: string, private readonly token?: string, private readonly timeoutMs = 30_000) {
    try {
      this.base = new URL(url);
    } catch {
      throw new Error("REET_URL must be an absolute http:// or https:// instance URL.");
    }
    if (!['http:', 'https:'].includes(this.base.protocol) || this.base.username || this.base.password ||
        this.base.search || this.base.hash) {
      throw new Error("REET_URL must use HTTP(S), without credentials, query parameters or a fragment. Use REET_API_TOKEN for authentication.");
    }
    this.base.pathname = this.base.pathname.replace(/\/$/, '') + '/';
  }

  async request(path: string, method = 'GET', body?: JsonObject, query?: URLSearchParams): Promise<JsonObject> {
    const url = new URL(path.replace(/^\//, ''), this.base);
    if (query) url.search = query.toString();
    const headers: Record<string, string> = { Accept: 'application/json', 'X-Reet-Source': 'mcp' };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    if (body) headers['Content-Type'] = 'application/json';
    let response: Response;
    try {
      response = await fetch(url, {
        method, headers, body: body ? JSON.stringify(body) : undefined,
        redirect: 'error', signal: AbortSignal.timeout(this.timeoutMs),
      });
    } catch {
      throw new Error('Cannot reach reet.sh. Check REET_URL, TLS and connectivity; requests time out after 30 seconds. Redirects are rejected: configure the final instance URL.');
    }
    if (!response.ok) {
      const raw = (await response.text().catch(() => '')).slice(0, 64_000);
      const advice: Record<number, string> = {
        400: 'Check the tool arguments and use IDs returned by the read tools.',
        401: 'Set REET_API_TOKEN to a personal API token from Account → API tokens (or the legacy instance token in token mode). Bootstrap tokens cannot access shelf APIs.',
        403: 'This account cannot perform that operation. Check its role and list permissions.',
        404: 'Resource not found. Use the list tools to find an accessible ID.',
        429: 'Rate limited. Wait before retrying.',
      };
      try {
        const body: unknown = JSON.parse(raw);
        if (body && typeof body === 'object') {
          const error = body as { code?: unknown; field?: unknown; message?: unknown; hint?: unknown; error?: unknown };
          if (typeof error.code === 'string' && error.code) {
            const message = typeof error.message === 'string' ? error.message : (typeof error.error === 'string' ? error.error : 'request rejected');
            const field = typeof error.field === 'string' && error.field ? ` field=${error.field}` : '';
            const hint = typeof error.hint === 'string' && error.hint ? `. ${error.hint}` : '';
            throw new Error(`reet.sh returned HTTP ${response.status} [${error.code}]${field}: ${message}${hint}`);
          }
        }
      } catch (error) {
        if (error instanceof Error && error.message.startsWith('reet.sh returned HTTP')) throw error;
      }
      throw new Error(`reet.sh returned HTTP ${response.status}. ${advice[response.status] ?? 'Check instance health and server logs before retrying.'}`);
    }
    if (response.status === 204) return { ok: true };
    // Bound the upstream response even though the existing API has no pagination for most collections.
    const reader = response.body?.getReader();
    if (!reader) throw new Error('reet.sh returned an empty response; check the instance URL.');
    const chunks: Uint8Array[] = [];
    let bytes = 0;
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        bytes += value.byteLength;
        if (bytes > MAX_API_BYTES) {
          throw new Error('reet.sh response exceeds 8 MiB. Narrow the search, stage, tag or environment filters.');
        }
        chunks.push(value);
      }
    } catch (error) {
      await reader.cancel().catch(() => undefined);
      if (error instanceof Error && error.message.includes('8 MiB')) throw error;
      throw new Error('reet.sh response was interrupted or timed out. Check instance health before retrying.');
    } finally {
      reader.releaseLock();
    }
    try {
      const data: unknown = JSON.parse(Buffer.concat(chunks).toString('utf8'));
      return object(data);
    } catch {
      throw new Error('reet.sh returned invalid JSON. Check REET_URL points to the instance, not a sign-in page.');
    }
  }
}

export function object(value: unknown): JsonObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Unexpected reet.sh API response: expected an object. Check server compatibility.');
  }
  return value as JsonObject;
}

export function collection(data: JsonObject, key: string): JsonObject[] {
  const value = data[key];
  // Go encodes some empty slices as null.
  if (value === null) return [];
  if (!Array.isArray(value)) throw new Error(`Unexpected reet.sh API response: missing ${key} array.`);
  return value.map(object);
}
