export type SessionState = 'creating' | 'running' | 'end_requested' | 'ended' | 'exited' | 'lost'
export type TerminalSession = { id: string; agentId: string; title: string; state: SessionState; createdAt: number; updatedAt: number; exitCode: number | null; removeHistory?:boolean }
export type Capabilities = { protocolVersion: number; os: string; osUser: string; terminalEnabled: boolean; terminalControl?: boolean; healthEnabled: boolean; readiness: string }
export type TerminalMachine = { agentId: string; host: string; ready: boolean; serverEnabled?:boolean; capabilities?: Capabilities }
export type Reading = { value: number | null; reason?: string }
export type Capacity = { totalBytes: Reading; availableBytes: Reading }
export type HealthReport = { cpuPercent: Reading; memory: Capacity; disk: Capacity; hostUptimeSeconds: Reading; scope: 'host' | 'container' }
export type ConnectionHealth = { connected: boolean; lastKnown?: boolean; reportFresh: boolean; contactAt?: number; reportAt?: number; rttMillis?: number; rttP50Millis?: number; rttP95Millis?: number; rttVariabilityMillis?: number; missedProbes: number; reconnects: number; path: string; report?: HealthReport }
export type MachineHealth = { agentId: string; host: string; state: string; connection?: ConnectionHealth; capabilities?: Capabilities }
export type HealthResponse = { serverNow: number; machines: MachineHealth[] }

export class TerminalError extends Error {
  status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}
export function validTerminalTitle(title:string):boolean {return !!title.trim()&&Array.from(title).length<=80&&!/[\p{Cc}\p{Cf}\u2028\u2029]/u.test(title)}
// Interactive terminals intentionally use only the browser cookie, never the
// generic API helper's localStorage API token or a capability in a URL.
export async function terminalRequest(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin', cache: 'no-store' })
  if (!response.ok) {
    let message = 'The terminal connection is unavailable.'
    try { message = (await response.json()).error || message } catch { /* fixed fallback */ }
    throw new TerminalError(response.status, message)
  }
  return response
}
export async function terminalMachines(signal?: AbortSignal): Promise<TerminalMachine[]> { return (await (await terminalRequest('/api/v2/terminal/machines', { signal })).json()).machines }
export type TerminalControllerStatus={configured:boolean;available:boolean;reason:string}
export async function terminalControllerStatus(signal?:AbortSignal):Promise<TerminalControllerStatus>{return (await terminalRequest('/api/v2/terminal/status',{signal})).json()}
export async function terminalSessions(signal?: AbortSignal): Promise<TerminalSession[]> { return (await (await terminalRequest('/api/v2/terminal/sessions', { signal })).json()).sessions }
export async function createTerminal(agentId: string, requestId: string, title: string, cols = 100, rows = 30, signal?: AbortSignal): Promise<TerminalSession> { return (await terminalRequest('/api/v2/terminal/sessions', { method: 'POST', signal, body: JSON.stringify({ agentId, requestId, title, cols, rows }) })).json() }
export async function terminalTicket(session: string, cols: number, rows: number, takeover: boolean, signal?: AbortSignal): Promise<string> { return (await (await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(session)}/attachments`, { method: 'POST', body: JSON.stringify({ cols, rows, takeover }), signal })).json()).ticket }
export async function machineHealth(signal?: AbortSignal): Promise<HealthResponse> { return (await terminalRequest('/api/v2/machines/health', { signal })).json() }
export async function renameTerminal(id: string, title: string): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}`, { method:'PATCH', body:JSON.stringify({title}) }) }
export async function endTerminal(id: string, requestId: string, removeHistory=false): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/end`, { method:'POST', body:JSON.stringify({requestId,removeHistory}) }) }
export function terminalClosed(state: SessionState): boolean { return ['ended','exited','lost'].includes(state) }
export async function archiveTerminal(id:string):Promise<void>{await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/archive`,{method:'POST',body:'{}'})}
export async function terminalHistory(id: string, signal?: AbortSignal): Promise<string> { return (await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/history?lines=2000`, {signal})).text() }

export function readiness(machine?: TerminalMachine): string {
  if (!machine) return 'Choose an enrolled machine.'
  if (machine.serverEnabled===false) return 'Remote terminals are disabled on this server. Run reet terminal start on the controller, with accounts mode and an HTTPS public URL configured. Monitoring and local shells remain available.'
  const caps = machine.capabilities
  if (!caps) return 'Start the background agent on this machine to connect.'
  if (!caps.terminalControl) return 'Update the reet agent to enable terminal support.'
  if (!caps.terminalEnabled) return 'Enable terminal access locally on this machine.'
  if (caps.readiness === 'tmux_unavailable') return 'Install tmux on this machine.'
  if (caps.readiness === 'supervisor_unavailable') return 'Install or start the independent terminal service on this machine.'
  if (!machine.ready) return 'The machine is reconnecting. Existing shells will be kept.'
  return ''
}

export type TerminalCommand={id:string;name:string;command:string;description:string;revision:number;createdAt:number;updatedAt:number}
export async function savedCommands(signal?:AbortSignal):Promise<TerminalCommand[]>{return(await(await terminalRequest('/api/v2/terminal/commands',{signal:signal?AbortSignal.any([signal,AbortSignal.timeout(15000)]):AbortSignal.timeout(15000)})).json()).commands}
export async function saveCommand(item:Pick<TerminalCommand,'name'|'command'|'description'>,current?:TerminalCommand):Promise<TerminalCommand>{return(await terminalRequest('/api/v2/terminal/commands'+(current?`/${encodeURIComponent(current.id)}`:''),{method:current?'PATCH':'POST',signal:AbortSignal.timeout(15000),body:JSON.stringify({...item,revision:current?.revision||0})})).json()}
export async function removeCommand(item:TerminalCommand):Promise<void>{await terminalRequest(`/api/v2/terminal/commands/${encodeURIComponent(item.id)}`,{method:'DELETE',signal:AbortSignal.timeout(15000),body:JSON.stringify({revision:item.revision})})}
