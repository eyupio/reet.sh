import type { Project, ProjectInput, ProjectRun, WorktreeStatus, WorktreeTarget } from '../projects'

export type SessionState = 'creating' | 'running' | 'end_requested' | 'ended' | 'exited' | 'lost'
export type TerminalSession = { id: string; agentId: string; title: string; state: SessionState; createdAt: number; updatedAt: number; exitCode: number | null; removeHistory?:boolean }
export type Capabilities = { commandHistory?:boolean; launch?: boolean; worktree?: boolean; tools?: string[]; protocolVersion: number; os: string; osUser: string; terminalEnabled: boolean; terminalControl?: boolean; healthEnabled: boolean; readiness: string }
export type TerminalMachine = { agentId: string; host: string; ready: boolean; serverEnabled?:boolean; capabilities?: Capabilities }
export type Reading = { value: number | null; reason?: string }
export type Capacity = { totalBytes: Reading; availableBytes: Reading }
export type HealthReport = { cpuPercent: Reading; memory: Capacity; disk: Capacity; hostUptimeSeconds: Reading; scope: 'host' | 'container' }
export type ConnectionHealth = { connected: boolean; lastKnown?: boolean; reportFresh: boolean; contactAt?: number; reportAt?: number; rttMillis?: number; rttP50Millis?: number; rttP95Millis?: number; rttVariabilityMillis?: number; missedProbes: number; reconnects: number; path: string; report?: HealthReport }
export type MachineHealth = { agentId: string; host: string; state: string; connection?: ConnectionHealth; capabilities?: Capabilities }
export type HealthResponse = { serverNow: number; machines: MachineHealth[] }

export class TerminalError extends Error {
  status: number
  /** The server's machine-readable reason, sent only for some refusals (for example `folder_unavailable`). */
  code?: string
  /** Git's own first line, sent with some worktree refusals. Plain text, shown as detail under the sentence. */
  detail?: string
  constructor(status: number, message: string, code?: string, detail?: string) { super(message); this.status = status; if (code) this.code = code; if (detail) this.detail = detail }
}
/** What to start instead of a plain shell. Only the fields that are set are sent; the machine decides the program and flags. */
export type LaunchRequest = { tool?: string; model?: string; dir?: string; prompt?: string }
export function validTerminalTitle(title:string):boolean {return !!title.trim()&&Array.from(title).length<=80&&!/[\p{Cc}\p{Cf}\u2028\u2029]/u.test(title)}
// Interactive terminals intentionally use only the browser cookie, never the
// generic API helper's localStorage API token or a capability in a URL.
export async function terminalRequest(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin', cache: 'no-store' })
  if (!response.ok) {
    let message = 'The terminal connection is unavailable.', code: string | undefined, detail: string | undefined
    try {
      const body = await response.json()
      if (typeof body?.error === 'string' && body.error) message = body.error
      if (typeof body?.code === 'string' && body.code) code = body.code
      if (typeof body?.detail === 'string' && body.detail) detail = body.detail
    } catch { /* fixed fallback */ }
    throw new TerminalError(response.status, message, code, detail)
  }
  return response
}
export async function terminalMachines(signal?: AbortSignal): Promise<TerminalMachine[]> { return (await (await terminalRequest('/api/v2/terminal/machines', { signal })).json()).machines }
export type TerminalControllerStatus={configured:boolean;available:boolean;reason:string}
export async function terminalControllerStatus(signal?:AbortSignal):Promise<TerminalControllerStatus>{return (await terminalRequest('/api/v2/terminal/status',{signal})).json()}
export async function terminalSessions(signal?: AbortSignal): Promise<TerminalSession[]> { return (await (await terminalRequest('/api/v2/terminal/sessions', { signal })).json()).sessions }
export async function createTerminal(agentId: string, requestId: string, title: string, cols = 100, rows = 30, signal?: AbortSignal, launch?: LaunchRequest): Promise<TerminalSession> {
  // A plain create stays byte-identical to before: the launch keys are added only when they have a value.
  const body: Record<string, string | number> = { agentId, requestId, title, cols, rows }
  for (const key of ['tool', 'model', 'dir', 'prompt'] as const) if (launch?.[key]) body[key] = launch[key]
  return (await terminalRequest('/api/v2/terminal/sessions', { method: 'POST', signal, body: JSON.stringify(body) })).json()
}
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
  if (caps.readiness === 'tmux_unavailable') return 'Run reet agent terminal setup on this machine to install or repair tmux 3.2+.'
  if (caps.readiness === 'supervisor_unavailable') return 'Install or start the independent terminal service on this machine.'
  if (!machine.ready) return 'The machine is reconnecting. Existing shells will be kept.'
  return ''
}

/** True when the machine has done its own part: a current agent, local consent given, and tmux and the terminal service in place.
 *  Says nothing about the server's terminal access or whether the machine is connected right now, which `ready` also needs. */
export function machineSetUp(machine: TerminalMachine): boolean {
  const caps = machine.capabilities
  return !!caps && !!caps.terminalControl && !!caps.terminalEnabled && caps.readiness !== 'tmux_unavailable' && caps.readiness !== 'supervisor_unavailable'
}

/** A machine card's sentence: an enrolled machine the terminal list does not know yet has not connected for terminals. */
export function cardReadiness(machine?: TerminalMachine): string {
  return machine ? readiness(machine) : "This machine hasn't connected for terminals yet."
}

export type TerminalCommand={id:string;name:string;command:string;description:string;revision:number;createdAt:number;updatedAt:number}
export async function savedCommands(signal?:AbortSignal):Promise<TerminalCommand[]>{return(await(await terminalRequest('/api/v2/terminal/commands',{signal:signal?AbortSignal.any([signal,AbortSignal.timeout(15000)]):AbortSignal.timeout(15000)})).json()).commands}
export async function saveCommand(item:Pick<TerminalCommand,'name'|'command'|'description'>,current?:TerminalCommand):Promise<TerminalCommand>{return(await terminalRequest('/api/v2/terminal/commands'+(current?`/${encodeURIComponent(current.id)}`:''),{method:current?'PATCH':'POST',signal:AbortSignal.timeout(15000),body:JSON.stringify({...item,revision:current?.revision||0})})).json()}
export async function removeCommand(item:TerminalCommand):Promise<void>{await terminalRequest(`/api/v2/terminal/commands/${encodeURIComponent(item.id)}`,{method:'DELETE',signal:AbortSignal.timeout(15000),body:JSON.stringify({revision:item.revision})})}

export type CommandHistoryEntry={command:string;source:'bash'|'zsh';timestamp?:number}
export type CommandHistorySnapshot={entries:CommandHistoryEntry[];sources:{source:'bash'|'zsh';available:boolean;reason?:string}[];truncated:boolean;skipped:number}
export async function commandHistory(id:string,signal?:AbortSignal):Promise<CommandHistorySnapshot>{return(await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/command-history`,{signal:signal?AbortSignal.any([signal,AbortSignal.timeout(15000)]):AbortSignal.timeout(15000)})).json()}

// Projects. Same cookie-only requests as terminals; every call gives up after 15 seconds. An update or delete always
// names the version it read (If-Match), and starting a run always carries a request id, so a lost reply can be resent.
const within = (signal?: AbortSignal): AbortSignal => (signal ? AbortSignal.any([signal, AbortSignal.timeout(15000)]) : AbortSignal.timeout(15000))
const projectPath = (id: string): string => `/api/v2/projects/${encodeURIComponent(id)}`
const ifMatch = (version: number): Record<string, string> => ({ 'If-Match': `"${version}"` })

export async function listProjects(signal?: AbortSignal): Promise<Project[]> { return (await (await terminalRequest('/api/v2/projects', { signal: within(signal) })).json()).projects }
/** One Project with its shared prompt (for the edit form) and the version to send back when saving. */
export async function getProject(id: string, signal?: AbortSignal): Promise<Project> { return (await terminalRequest(projectPath(id), { signal: within(signal) })).json() }
/** Create (no `current`) or update. An update sends If-Match with the version read; a changed Project answers 409 `project_changed`. */
export async function saveProject(input: ProjectInput, current?: { id: string; version: number }, signal?: AbortSignal): Promise<Project> {
  return (await terminalRequest(current ? projectPath(current.id) : '/api/v2/projects', {
    method: current ? 'PATCH' : 'POST', signal: within(signal), body: JSON.stringify(input), headers: current ? ifMatch(current.version) : undefined,
  })).json()
}
export type DeletedProject = { ok: boolean; mayHaveWorktrees: boolean; message: string }
export async function deleteProject(current: { id: string; version: number }, signal?: AbortSignal): Promise<DeletedProject> {
  return (await terminalRequest(projectPath(current.id), { method: 'DELETE', signal: within(signal), headers: ifMatch(current.version) })).json()
}
export async function projectRuns(projectId: string, signal?: AbortSignal): Promise<ProjectRun[]> { return (await (await terminalRequest(`${projectPath(projectId)}/runs`, { signal: within(signal) })).json()).runs }
export async function getRun(runId: string, signal?: AbortSignal): Promise<ProjectRun> { return (await terminalRequest(`/api/v2/project-runs/${encodeURIComponent(runId)}`, { signal: within(signal) })).json() }
/** Starts a run of the chosen member positions. 201 (new) and 200 (the same request resent) both give the run. */
export async function startRun(projectId: string, positions: number[], requestId: string, signal?: AbortSignal): Promise<ProjectRun> {
  if (!requestId) throw new Error('A run needs a request id.')
  if (positions.length === 0) throw new Error('Choose at least one repository to start.')
  return (await terminalRequest(`${projectPath(projectId)}/runs`, { method: 'POST', signal: within(signal), body: JSON.stringify({ requestId, positions }) })).json()
}
export async function retryMember(runId: string, position: number, signal?: AbortSignal): Promise<ProjectRun> {
  return (await terminalRequest(`/api/v2/project-runs/${encodeURIComponent(runId)}/members/${position}/retry`, { method: 'POST', signal: within(signal), body: '{}' })).json()
}
export async function worktreeStatus(target: WorktreeTarget, signal?: AbortSignal): Promise<WorktreeStatus> {
  return (await terminalRequest('/api/v2/terminal/worktrees/status', { method: 'POST', signal: within(signal), body: JSON.stringify(target) })).json()
}
export type RemovedWorktree = { ok: boolean; path?: string; branch: string; confirmedBy?: string }
/** Removes a worktree only if the machine's counts still equal the ones shown; the branch is kept. */
export async function removeWorktree(target: WorktreeTarget, shown: { changed: number; unpushed: number }, signal?: AbortSignal): Promise<RemovedWorktree> {
  return (await terminalRequest('/api/v2/terminal/worktrees/remove', { method: 'POST', signal: within(signal), body: JSON.stringify({ ...target, changed: shown.changed, unpushed: shown.unpushed }) })).json()
}
