// The Add project dialog's fixture page (tests/fixtures/add-project.html) with a small synthetic tree under /home/dev on
// studio; desk is offline. Shared by add-project.e2e.mjs and add-project-edit.e2e.mjs.
import { openWorkspace } from './workspace.mjs'

export const HOME = '/home/dev'
export const caps = over => ({protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,agentsV2:true,models:true,worktree:true,browse:true,branches:true,tools:['claude','codex','opencode','gemini','cline'],...over})
export const machines = [
 {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps()},
 {agentId:'a2',host:'desk',ready:false,serverEnabled:true,capabilities:caps({agentsV2:false,models:false})},
]
export const dir = (name, repoRoot=false) => ({name,dir:true,repoRoot,hidden:name.startsWith('.')})
// The tree: path -> {repo, repoTop, entries}. `big` has 250 folders, so it pages.
export const tree = {
 // browse-v1 lists folders only, but the field is kept for a later version: an entry with dir false is never shown.
 [HOME]: {repo:'none', entries:[dir('.config'), dir('archive'), dir('big'), dir('blog',true), {name:'commands.txt',dir:false,repoRoot:false,hidden:false}, dir('my work #1'), dir('reet',true), dir('scratch'), dir('site',true)]},
 [`${HOME}/archive`]: {repo:'none', entries:[]},
 [`${HOME}/big`]: {repo:'none', entries:Array.from({length:250}, (_, i) => dir(`f${String(i).padStart(3,'0')}`))},
 [`${HOME}/my work #1`]: {repo:'none', entries:[dir('café'), dir('notes')]},
 [`${HOME}/my work #1/café`]: {repo:'none', entries:[dir('日本語')]},
 [`${HOME}/my work #1/café/日本語`]: {repo:'none', entries:[]},
 [`${HOME}/reet`]: {repo:'root', entries:[dir('docs'), dir('web')]},
 [`${HOME}/reet/web`]: {repo:'inside', repoTop:`${HOME}/reet`, entries:[dir('src')]},
 [`${HOME}/blog`]: {repo:'root', entries:[]},
 [`${HOME}/site`]: {repo:'root', entries:[]},
 [`${HOME}/notes`]: {repo:'none', entries:[]},
}
export const refuse = (status, code, error) => ({status, json:{error, code}})
export function browseAnswer(url) {
 const q = new URL(url).searchParams
 const path = q.get('path') || HOME
 if (!path.startsWith('/')) return refuse(400, 'bad_request', 'bad path')
 if (path !== HOME && !path.startsWith(`${HOME}/`)) return refuse(422, 'outside_home', 'That folder is outside the home folder reet was set up with.')
 const node = tree[path]
 if (!node) return refuse(404, 'not_found', "That folder isn't on studio.")
 const filter = (q.get('filter') || '').toLowerCase(), hidden = q.get('hidden') === 'true', after = q.get('after') || ''
 const shown = node.entries.filter(e => (hidden || !e.hidden) && e.name.toLowerCase().includes(filter) && e.name > after)
 const page = shown.slice(0, 200)
 return {path, home:HOME, parent:path === HOME ? '' : path.slice(0, path.lastIndexOf('/')), repo:node.repo, repoTop:node.repoTop ?? '', entries:page, more:shown.length > 200, partial:false, scanned:node.entries.length, omitted:0}
}
export const saved = (over={}) => ({id:'fp1',host:'a1',machine:'studio',dir:`${HOME}/reet`,name:'reet',isRepo:true,baseBranch:'',worktreeDefault:'inherit',defaultTool:'',defaultModel:'',defaultEffort:'',hasStarterPrompt:true,note:'',position:0,createdAt:1,updatedAt:1,version:3,...over})
const modelsFor = tool => ({tool, state:'listed', models:[{id:'anthropic/claude-sonnet-4-5',provider:'anthropic'},{id:'openai/gpt-5',provider:'openai'}], more:false, source:'fixture', version:'', fetchedAt:Date.now(), checkedAt:Date.now(), reason:'', sentence:''})

// The fixture page with the API above. `box.fail` answers the next browse with a refusal; `box.browses` records each one.
// `options.routes` answers paths of its own, ahead of these.
export async function openFixture(browser, harness, options={}) {
 const box = {browses:[], writes:[], fail:null, branchReads:[], branchFail:null, modelReads:[], modelFail:null, favouriteReads:0, favouriteFail:null,
  favourites:[{tool:'cline', model:'anthropic/claude-opus-4-1', agentDefault:false}]}
 const routes = {
  '/api/v2/terminal/machines': {machines},
  '/api/v2/machines/a1/browse': request => {
   box.browses.push(new URL(request.url()))
   if (box.fail) { const f = box.fail; box.fail = null; return f }
   return browseAnswer(request.url())
  },
  '/api/v2/machines/a2/browse': request => { box.browses.push(new URL(request.url())); return refuse(503, 'machine_unavailable', 'That machine is reconnecting. Try again in a moment.') },
  // Each repository its own branches; site has none. `box.branchFail` answers the next listing with a refusal.
  '/api/v2/machines/a1/branches': request => {
   const dir = new URL(request.url()).searchParams.get('dir')
   box.branchReads.push(dir)
   if (box.branchFail) { const f = box.branchFail; box.branchFail = null; return f }
   const branches = {[`${HOME}/reet`]:['main','dev'], [`${HOME}/blog`]:['trunk'], [`${HOME}/site`]:[]}[dir]
   return branches ? {current:branches[0] ?? '', default:branches[0] ?? '', branches, more:false} : refuse(422, 'not_repo_root', "That folder isn't the top of a git repository.")
  },
  '/api/v2/profile/model-favourites': () => {
   box.favouriteReads++
   if (box.favouriteFail) { const f = box.favouriteFail; box.favouriteFail = null; return f }
   return {version:1, favourites:box.favourites}
  },
  '/api/v2/machines/a1/models': request => {
   const tool = new URL(request.url()).searchParams.get('tool') ?? ''
   box.modelReads.push(tool)
   if (box.modelFail) { const f = box.modelFail; box.modelFail = null; return f }
   return modelsFor(tool)
  },
  '/api/v2/machines/a2/models': request => {
   const tool = new URL(request.url()).searchParams.get('tool') ?? ''
   box.modelReads.push(tool)
   if (box.modelFail) { const f = box.modelFail; box.modelFail = null; return f }
   return modelsFor(tool)
  },
  '/api/v2/folder-projects': request => {
   const body = request.postDataJSON()
   box.writes.push({method:request.method(), body, headers:request.headers()})
   return {status:201, json:saved({id:'fp9', host:body.host, dir:body.dir, name:body.name || body.dir.split('/').pop(), isRepo:!!body.isRepo, version:1})}
  },
  '/api/v2/folder-projects/fp1': request => {
   if (request.method() === 'GET') return saved({starterPrompt:'Read the README first'})
   box.writes.push({method:request.method(), body:request.postDataJSON(), headers:request.headers()})
   return refuse(409, 'project_changed', 'Someone changed this project. Reload it, then try again.')
  },
 }
 const opened = await openWorkspace(browser, {width:1440, height:900, ...options, routes:{...routes, ...(options.routes ?? {})}})
 await opened.page.goto(`${harness.url}/tests/fixtures/add-project.html${options.query ?? ''}`)
 return {...opened, box}
}
