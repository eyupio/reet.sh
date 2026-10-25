import type { TerminalControllerStatus } from './api'

/** What the controller's terminal status means, in words. One place, so Control and the connection guide say the same thing. */
export function controllerDetail(status: TerminalControllerStatus | null): string {
  if (status?.available) return 'Controller access is enabled. Each host still needs local terminal consent.'
  if (status?.reason === 'https_required') return 'Started, but the controller has no usable HTTPS public URL. Start/restart reads your existing .env configuration.'
  if (status?.reason === 'accounts_required') return 'Started, but the running controller is not using accounts authentication.'
  return 'Remote access is stopped. Existing host shells continue running.'
}
