import type { Project, ProjectRun, WorktreeStatus, WorktreeTarget } from '../projects'
import type { MachineModels } from '../models'

export type SessionState = 'creating' | 'running' | 'end_requested' | 'ended' | 'exited' | 'lost'
export type TerminalSession = { id: string; agentId: string; title: string; state: SessionState; createdAt: number; updatedAt: number; exitCode: number | null; removeHistory?:boolean; tool?: string; model?: string; effort?: string; yolo?: boolean; projectId?: string | null; dir?: string; worktreePath?: string; worktreeRepo?: string; worktreeBranch?: string; structured?: boolean }
export type Capabilities = { agentsV2?: boolean; osVersion?: string; yolo?: boolean; commandHistory?:boolean; launch?: boolean; resume?: boolean; worktree?: boolean; tools?: string[]; browse?: boolean; browseOff?: boolean; branches?: boolean; uploads?: boolean; uploadsOff?: boolean; models?: boolean; modelsOff?: boolean; acp?: boolean; protocolVersion: number; os: string; osUser: string; terminalEnabled: boolean; terminalControl?: boolean; healthEnabled: boolean; readiness: string }
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
  /** The field a Projects refusal is about ("name", "members.2.dir"), so a form can move focus there. */
  field?: string
  constructor(status: number, message: string, code?: string, detail?: string, field?: string) { super(message); this.status = status; if (code) this.code = code; if (detail) this.detail = detail; if (field) this.field = field }
}
/** What to start instead of a plain shell. Only the fields that are set are sent; the machine decides the program and flags. */
export type LaunchRequest = { tool?: string; model?: string; dir?: string; prompt?: string; base?: string; effort?: 'low' | 'medium' | 'high'; yolo?: boolean; resume?: boolean; worktree?: string; project?: string; structured?: boolean; trust?: boolean
  /** The folder project the session belongs to (checked by the controller, never sent to the machine). */
  projectId?: string }
export function validTerminalTitle(title:string):boolean {return !!title.trim()&&Array.from(title).length<=80&&!/[\p{Cc}\p{Cf}\u2028\u2029]/u.test(title)}
// Interactive terminals intentionally use only the browser cookie, never the
// generic API helper's localStorage API token or a capability in a URL.
export async function terminalRequest(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin', cache: 'no-store' })
  if (!response.ok) {
    let message = 'The terminal connection is unavailable.', code: string | undefined, detail: string | undefined, field: string | undefined
    try {
      const body = await response.json()
      if (typeof body?.error === 'string' && body.error) message = body.error
      if (typeof body?.code === 'string' && body.code) code = body.code
      if (typeof body?.detail === 'string' && body.detail) detail = body.detail
      if (typeof body?.field === 'string' && body.field) field = body.field
    } catch { /* fixed fallback */ }
    throw new TerminalError(response.status, message, code, detail, field)
  }
  return response
}
/** Upload one image body as sent by the browser. It never goes through
 *  terminalRequest, which forces a JSON Content-Type on every body. */
export async function terminalUpload(session: string, image: Blob, signal?: AbortSignal): Promise<{ path: string; reference: string }> {
  const response = await fetch(`/api/v2/terminal/sessions/${encodeURIComponent(session)}/uploads`, {
    method: 'POST',
    body: image,
    headers: new Headers({ 'Content-Type': image.type }),
    credentials: 'same-origin',
    cache: 'no-store',
    signal,
  })
  if (!response.ok) {
    let message = 'The terminal connection is unavailable.', code: string | undefined, detail: string | undefined, field: string | undefined
    try {
      const body = await response.json()
      if (typeof body?.error === 'string' && body.error) message = body.error
      if (typeof body?.code === 'string' && body.code) code = body.code
      if (typeof body?.detail === 'string' && body.detail) detail = body.detail
      if (typeof body?.field === 'string' && body.field) field = body.field
    } catch { /* fixed fallback */ }
    throw new TerminalError(response.status, message, code, detail, field)
  }
  const path = decodeURIComponent(response.headers.get('X-Reet-Upload-Path') ?? '')
  const reference = decodeURIComponent(response.headers.get('X-Reet-Upload-Reference') ?? '')
  if (!reference) throw new TerminalError(response.status, 'The upload did not return a path.')
  return { path, reference }
}
/** Upload one image for a session that is about to start on a machine. The
 *  reply is the path the opening prompt should name. Same body and headers as
 *  a session upload; there is no session yet. */
export async function launchUpload(agentId: string, image: Blob, signal?: AbortSignal): Promise<{ path: string; reference: string }> {
  const response = await fetch(`/api/v2/machines/${encodeURIComponent(agentId)}/uploads`, {
    method: 'POST',
    body: image,
    headers: new Headers({ 'Content-Type': image.type }),
    credentials: 'same-origin',
    cache: 'no-store',
    signal,
  })
  if (!response.ok) {
    let message = 'The terminal connection is unavailable.', code: string | undefined, detail: string | undefined, field: string | undefined
    try {
      const body = await response.json()
      if (typeof body?.error === 'string' && body.error) message = body.error
      if (typeof body?.code === 'string' && body.code) code = body.code
      if (typeof body?.detail === 'string' && body.detail) detail = body.detail
      if (typeof body?.field === 'string' && body.field) field = body.field
    } catch { /* fixed fallback */ }
    throw new TerminalError(response.status, message, code, detail, field)
  }
  const reference = decodeURIComponent(response.headers.get('X-Reet-Upload-Reference') ?? '')
  if (!reference) throw new TerminalError(response.status, 'The upload did not return a path.')
  return { path: decodeURIComponent(response.headers.get('X-Reet-Upload-Path') ?? ''), reference }
}
export async function terminalMachines(signal?: AbortSignal): Promise<TerminalMachine[]> { return (await (await terminalRequest('/api/v2/terminal/machines', { signal })).json()).machines }
const modelsQuery = (tool: string): string => new URLSearchParams({ tool }).toString()
export async function machineModels(agentId: string, tool: string, signal?: AbortSignal): Promise<MachineModels> {
  return (await terminalRequest(`/api/v2/machines/${encodeURIComponent(agentId)}/models?${modelsQuery(tool)}`, { signal })).json()
}
export async function refreshMachineModels(agentId: string, tool: string, signal?: AbortSignal): Promise<MachineModels> {
  return (await terminalRequest(`/api/v2/machines/${encodeURIComponent(agentId)}/models/refresh`, { method: 'POST', signal, body: JSON.stringify({ tool }) })).json()
}
export type AllModels = { machines: (Omit<MachineModels, 'tool'> & { agentId: string; host: string; tool?: string })[] }
export async function allModels(tool: string, signal?: AbortSignal): Promise<AllModels> {
  return (await terminalRequest(`/api/v2/models?${modelsQuery(tool)}`, { signal })).json()
}
export type TerminalControllerStatus={configured:boolean;available:boolean;reason:string}
export async function terminalControllerStatus(signal?:AbortSignal):Promise<TerminalControllerStatus>{return (await terminalRequest('/api/v2/terminal/status',{signal})).json()}
export async function terminalSessions(signal?: AbortSignal): Promise<TerminalSession[]> { return (await (await terminalRequest('/api/v2/terminal/sessions', { signal })).json()).sessions }
export async function createTerminal(agentId: string, requestId: string, title: string, cols = 100, rows = 30, signal?: AbortSignal, launch?: LaunchRequest): Promise<TerminalSession> {
  // A plain create stays byte-identical to before: the launch keys are added only when they have a value.
  const body: Record<string, string | number | boolean> = { agentId, requestId, title, cols, rows }
  for (const key of ['tool', 'model', 'dir', 'prompt', 'projectId', 'base', 'effort', 'worktree', 'project'] as const) if (launch?.[key]) body[key] = launch[key]
  if (launch?.yolo) body.yolo = true
  if (launch?.resume) body.resume = true
  if (launch?.structured) body.structured = true
  if (launch?.trust) body.trust = true
  return (await terminalRequest('/api/v2/terminal/sessions', { method: 'POST', signal, body: JSON.stringify(body) })).json()
}
export async function terminalTicket(session: string, cols: number, rows: number, takeover: boolean, signal?: AbortSignal): Promise<string> { return (await (await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(session)}/attachments`, { method: 'POST', body: JSON.stringify({ cols, rows, takeover }), signal })).json()).ticket }
export async function machineHealth(signal?: AbortSignal): Promise<HealthResponse> { return (await terminalRequest('/api/v2/machines/health', { signal })).json() }
export async function renameTerminal(id: string, title: string): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}`, { method:'PATCH', body:JSON.stringify({title}) }) }
export async function endTerminal(id: string, requestId: string, removeHistory=false): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/end`, { method:'POST', body:JSON.stringify({requestId,removeHistory}) }) }
/** The address that opens one session (R17). Only ids go in the address. */
export const sessionHref = (session: Pick<TerminalSession, 'agentId' | 'id'>): string => `/sessions?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`
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

// Saved Projects, kept to read: the list, their runs and a run, and the worktree status and remove a run's worktree
// takes. Same cookie-only requests as terminals; every call gives up after 15 seconds. Nothing here changes a Project.
const within = (signal?: AbortSignal): AbortSignal => (signal ? AbortSignal.any([signal, AbortSignal.timeout(15000)]) : AbortSignal.timeout(15000))
const projectPath = (id: string): string => `/api/v2/projects/${encodeURIComponent(id)}`
const ifMatch = (version: number): Record<string, string> => ({ 'If-Match': `"${version}"` })

export async function listProjects(signal?: AbortSignal): Promise<Project[]> { return (await (await terminalRequest('/api/v2/projects', { signal: within(signal) })).json()).projects }
export async function projectRuns(projectId: string, signal?: AbortSignal): Promise<ProjectRun[]> { return (await (await terminalRequest(`${projectPath(projectId)}/runs`, { signal: within(signal) })).json()).runs }
export async function getRun(runId: string, signal?: AbortSignal): Promise<ProjectRun> { return (await terminalRequest(`/api/v2/project-runs/${encodeURIComponent(runId)}`, { signal: within(signal) })).json() }
// Start in several projects: one run of the folder projects chosen in the rail (POST /api/v2/folder-projects/runs), read and
// retried under the folder projects' routes. The start is idempotent by its request id; a member's retry asks the controller,
// which keeps the same request id while the earlier create may have started an agent.
export type SelectionRunBody = { requestId: string; projectIds: string[]; tool: string; model: string; effort: string; prompt: string; worktree: boolean }
const selectionRunPath = (runId: string): string => `/api/v2/folder-projects/runs/${encodeURIComponent(runId)}`
export async function startSelectionRun(body: SelectionRunBody, signal?: AbortSignal): Promise<ProjectRun> {
  return (await terminalRequest('/api/v2/folder-projects/runs', { method: 'POST', signal: within(signal), body: JSON.stringify(body) })).json()
}
export async function getSelectionRun(runId: string, signal?: AbortSignal): Promise<ProjectRun> { return (await terminalRequest(selectionRunPath(runId), { signal: within(signal) })).json() }
export async function retrySelectionMember(runId: string, position: number, signal?: AbortSignal): Promise<ProjectRun> {
  return (await terminalRequest(`${selectionRunPath(runId)}/members/${position}/retry`, { method: 'POST', signal: within(signal), body: '{}' })).json()
}
export async function worktreeStatus(target: WorktreeTarget, signal?: AbortSignal): Promise<WorktreeStatus> {
  return (await terminalRequest('/api/v2/terminal/worktrees/status', { method: 'POST', signal: within(signal), body: JSON.stringify(target) })).json()
}
export type RemovedWorktree = { ok: boolean; path?: string; branch: string; confirmedBy?: string }
/** Removes a worktree only if the machine's counts still equal the ones shown; the branch is kept. */
export async function removeWorktree(target: WorktreeTarget, shown: { changed: number; unpushed: number }, signal?: AbortSignal): Promise<RemovedWorktree> {
  return (await terminalRequest('/api/v2/terminal/worktrees/remove', { method: 'POST', signal: within(signal), body: JSON.stringify({ ...target, changed: shown.changed, unpushed: shown.unpushed }) })).json()
}

// Folder projects: one folder on one machine (GET/POST /api/v2/folder-projects, GET/PATCH/DELETE …/{id}, POST …/reorder).
// The same cookie-only requests as terminals and Projects; an update or delete names the version it read (If-Match), and
// a project changed since answers 409 `project_changed`. The single read and every write give the version (the ETag).
export type WorktreeDefault = 'inherit' | 'yes' | 'no'
export type FolderProject = {
  id: string
  /** The machine's enrollment id: pass it as it is as the agentId of `browseFolder`, `listBranches` and `createTerminal`. */
  host: string
  /** The machine's current name for display; "" once the machine is no longer enrolled. */
  machine: string
  dir: string; name: string; isRepo: boolean; baseBranch: string; worktreeDefault: WorktreeDefault
  /** The rail colour's id from projectColour.ts, "" when none. */
  colour: string
  defaultTool: string; defaultModel: string; defaultEffort: string; hasStarterPrompt: boolean
  /** Only in the single read (`getFolderProject`), for the edit form. */
  starterPrompt?: string
  /** The one-time notice from moving saved Projects here: the names of the other Projects with this folder whose shared
   * prompts were not kept ("A, B"), or "". Show it once; an update with `note: ''` clears it. */
  note: string
  position: number; createdAt: number; updatedAt: number; version: number
}
/** A create sends host (the machine's enrollment id) and dir at least; an update sends only what changed (absent keeps, "" clears a text field).
 * `note` can only be cleared (anything else is 422 `invalid_note`). */
export type FolderProjectInput = Partial<Pick<FolderProject, 'host' | 'dir' | 'name' | 'isRepo' | 'baseBranch' | 'worktreeDefault' | 'colour' | 'defaultTool' | 'defaultModel' | 'defaultEffort' | 'starterPrompt'>> & { note?: '' }
export type RemovedFolderProject = { ok: boolean; message: string }
/** The rail's list; `savedWaiting` counts saved Project repositories still waiting for room (offer `importSavedProjects`). */
export type FolderProjectList = { projects: FolderProject[]; savedWaiting: number }
/** What bringing in the waiting saved repositories did, with the list as it now is and one sentence to show. */
export type ImportedSavedProjects = FolderProjectList & { created: number; skipped: number; message: string }

export type BrowseRepo = 'root' | 'inside' | 'none'
export type BrowseEntry = { name: string; dir: boolean; repoRoot: boolean; hidden: boolean }
export type BrowseResult = { path: string; home: string; parent: string; repo: BrowseRepo; repoTop: string; entries: BrowseEntry[]; more: boolean; partial: boolean; scanned: number; omitted: number }
/** The machine's reasons for not listing a folder; `off` (folder browsing switched off there) reaches the page as `browse_off`. */
export type BrowseReason = 'not_found' | 'not_dir' | 'outside_home' | 'denied' | 'unreadable' | 'conflict' | 'off'
/** `session` names one opened browser (a fresh id per dialog), so its pages share one audit row. */
export type BrowseQuery = { path?: string; after?: string; filter?: string; hidden?: boolean; session: string }
export type BranchesResult = { current: string; default: string; branches: string[]; more: boolean }

export const FOLDER_PROJECT_ERROR_CODES = ['project_exists', 'project_limit', 'project_changed', 'project_rate_limited', 'name_taken', 'unknown_machine', 'project_not_found', 'invalid_version', 'invalid_order'] as const
export const BROWSE_ERROR_CODES = ['browse_unsupported', 'browse_off', 'browse_busy', 'browse_rate_limited', 'not_found', 'not_dir', 'outside_home', 'denied', 'unreadable', 'machine_timeout', 'machine_error', 'machine_unavailable'] as const
export const BRANCHES_ERROR_CODES = ['branches_unsupported', 'branches_denied', 'branches_busy', 'branches_rate_limited', 'not_repo_root', 'outside_home', 'git_failed', 'machine_timeout', 'machine_error', 'machine_unavailable'] as const
export type FolderProjectErrorCode = typeof FOLDER_PROJECT_ERROR_CODES[number]
export type BrowseErrorCode = typeof BROWSE_ERROR_CODES[number]
export type BranchesErrorCode = typeof BRANCHES_ERROR_CODES[number]
/** The refusal's code when it is one the caller handles, else undefined (show the sentence, `error.message`, as it is). */
export function refusalCode<C extends string>(error: unknown, codes: readonly C[]): C | undefined {
  return error instanceof TerminalError && error.code && (codes as readonly string[]).includes(error.code) ? error.code as C : undefined
}

/** The sentence to show for a failed request: the server's own refusal as sent, or one plain sentence when no answer came. */
export function refusalSentence(error: unknown): string {
  return error instanceof TerminalError ? error.message : "That didn't reach reet. Check your connection, then try again."
}

export const folderProjectPath = (id: string): string => `/api/v2/folder-projects/${encodeURIComponent(id)}`
function versionHeader(version: number): Record<string, string> {
  if (!Number.isSafeInteger(version) || version < 1) throw new Error('A change needs the version of the project it was made from.')
  return ifMatch(version)
}
/** The version a response names: its ETag when it has one, else the body's. */
async function withVersion(response: Response): Promise<FolderProject> {
  const project: FolderProject = await response.json()
  const tag = /^"(\d+)"$/.exec(response.headers.get('ETag') ?? '')
  if (tag) project.version = Number(tag[1])
  return project
}

export async function listFolderProjects(signal?: AbortSignal): Promise<FolderProject[]> { return (await (await terminalRequest('/api/v2/folder-projects', { signal: within(signal) })).json()).projects }
export async function folderProjectList(signal?: AbortSignal): Promise<FolderProjectList> { return (await terminalRequest('/api/v2/folder-projects', { signal: within(signal) })).json() }
/** Brings in the saved Project repositories that waited for room; with nothing waiting it changes nothing. */
export async function importSavedProjects(signal?: AbortSignal): Promise<ImportedSavedProjects> {
  return (await terminalRequest('/api/v2/folder-projects/import-saved', { method: 'POST', signal: within(signal), body: '{}' })).json()
}
/** The folder project made from a saved Project (for an old /projects/<id> address), or null when none was. */
export async function folderProjectForLegacy(projectId: string, signal?: AbortSignal): Promise<string | null> {
  try {
    const answer = await (await terminalRequest(`/api/v2/folder-projects?${new URLSearchParams({ legacy: projectId })}`, { signal: within(signal) })).json()
    return typeof answer?.id === 'string' ? answer.id : null
  } catch (e) {
    if (e instanceof TerminalError && e.status === 404) return null
    throw e
  }
}
export async function getFolderProject(id: string, signal?: AbortSignal): Promise<FolderProject> { return withVersion(await terminalRequest(folderProjectPath(id), { signal: within(signal) })) }
export async function createFolderProject(input: FolderProjectInput, signal?: AbortSignal): Promise<FolderProject> {
  return withVersion(await terminalRequest('/api/v2/folder-projects', { method: 'POST', signal: within(signal), body: JSON.stringify(input) }))
}
export async function updateFolderProject(id: string, input: FolderProjectInput, version: number, signal?: AbortSignal): Promise<FolderProject> {
  const headers = versionHeader(version)
  return withVersion(await terminalRequest(folderProjectPath(id), { method: 'PATCH', signal: within(signal), body: JSON.stringify(input), headers }))
}
export async function deleteFolderProject(id: string, version: number, signal?: AbortSignal): Promise<RemovedFolderProject> {
  const headers = versionHeader(version)
  return (await terminalRequest(folderProjectPath(id), { method: 'DELETE', signal: within(signal), headers })).json()
}
/** Every project once, each with the version read; answers the list in its new order with the new versions. */
export async function reorderFolderProjects(ids: string[], versions: number[], signal?: AbortSignal): Promise<FolderProject[]> {
  if (ids.length !== versions.length) throw new Error('Every project needs the version that was read.')
  return (await (await terminalRequest('/api/v2/folder-projects/reorder', { method: 'POST', signal: within(signal), body: JSON.stringify({ ids, versions }) })).json()).projects
}

/** Only the parameters that are set, each once; `hidden` only when true. */
export function browseURL(agentId: string, query: BrowseQuery): string {
  const params = new URLSearchParams()
  for (const key of ['path', 'after', 'filter'] as const) if (query[key]) params.set(key, query[key] as string)
  if (query.hidden) params.set('hidden', 'true')
  params.set('session', query.session)
  return `/api/v2/machines/${encodeURIComponent(agentId)}/browse?${params}`
}
export function branchesURL(agentId: string, dir: string, filter?: string): string {
  const params = new URLSearchParams({ dir })
  if (filter) params.set('filter', filter)
  return `/api/v2/machines/${encodeURIComponent(agentId)}/branches?${params}`
}
export async function browseFolder(agentId: string, query: BrowseQuery, signal?: AbortSignal): Promise<BrowseResult> { return (await terminalRequest(browseURL(agentId, query), { signal: within(signal) })).json() }
export async function listBranches(agentId: string, dir: string, filter?: string, signal?: AbortSignal): Promise<BranchesResult> { return (await terminalRequest(branchesURL(agentId, dir, filter), { signal: within(signal) })).json() }
export function chatURL(id: string): string { return `/api/v2/terminal/sessions/${encodeURIComponent(id)}/chat` }
export async function chatPrompt(id: string, text: string): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/chat/prompt`, { method: 'POST', body: JSON.stringify({ text }) }) }
export async function chatCancel(id: string): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/chat/cancel`, { method: 'POST', body: '{}' }) }
export async function chatAnswer(id: string, permissionId: string, allow: boolean): Promise<void> { await terminalRequest(`/api/v2/terminal/sessions/${encodeURIComponent(id)}/chat/permissions/${encodeURIComponent(permissionId)}`, { method: 'POST', body: JSON.stringify({ allow }) }) }
