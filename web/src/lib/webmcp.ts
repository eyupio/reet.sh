// The browser WebMCP API is progressive enhancement; no polyfill exposes tools to unsupported browsers.
export interface BrowserModelContext {
  registerTool(tool: {
    name: string
    description: string
    inputSchema: Record<string, unknown>
    annotations?: { readOnlyHint: boolean }
    execute: (args: Record<string, unknown>, client?: { requestUserInteraction?: <T>(callback: () => Promise<T>) => Promise<T> }) => Promise<unknown>
  }): void
  unregisterTool(name: string): void
}

export function browserModelContext(): BrowserModelContext | undefined {
  return (navigator as Navigator & { modelContext?: BrowserModelContext }).modelContext
}
