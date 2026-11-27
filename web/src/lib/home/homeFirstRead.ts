export type HomeFirstRead = 'loading' | 'error' | 'ready'

export function homeFirstRead(input: { settled: boolean; failed: boolean; writer: boolean; hasSessions: boolean; hasHealth: boolean }): HomeFirstRead {
  if (!input.settled) return 'loading'
  if (input.failed && (input.writer ? !input.hasSessions : !input.hasHealth)) return 'error'
  return 'ready'
}
