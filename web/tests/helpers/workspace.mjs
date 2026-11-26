// A browser page with the workspace API answered from fixtures, for UI contract tests. Unmocked private paths answer 404 and are recorded.
export const owner = {id:1,name:'Sam',email:'sam@example.test',role:'owner'}

const now = Date.now()
export const skill = id => ({id:`demo/toolkit/${id}`,name:id,slug:id,source:'demo/toolkit',sourceType:'github',url:`https://www.skills.sh/demo/toolkit/${id}`,description:'Example skill for UI verification.',notes:'',installs:0,installsWeek:0,trend:[],liked:false,bookmarked:false,used:false,createdAt:now,updatedAt:now})

// A row of the Skills page (GET /api/v2/skills), keyed by library item, on all 3 machines unless `over` says otherwise.
export const skillRow = (name, over = {}) => ({id:`item-${name}`,shelfId:`demo/toolkit/${name}`,name,description:`${name} does one job well.`,status:'active',originKind:'imported',origin:`https://github.com/demo/toolkit/${name}`,yours:false,published:false,hasCopy:true,liked:false,notes:'',tags:[],lists:[],health:{state:'ok',pending:0,total:3,hosts:[],machines:[]},...over})
// One tab of the Skills page as the server answers it: that tab's rows, both tabs' counts and the rest of the page.
// `work` is what the machines are doing over every skill, deleted ones still coming off included; nothing by default.
export const noWork = {failed:0,installing:0,removing:0,updating:0}
export const skillsPageOf = (rows, status = 'active', {newOnMachines = [], machines = 3, notice = null, work = noWork} = {}) => ({skills:rows.filter(r=>r.status===status),counts:{active:rows.filter(r=>r.status==='active').length,archived:rows.filter(r=>r.status==='archived').length},newOnMachines,machines,notice,work})
// The shelf fixtures a test passes as `skills`, as the Skills page lists them: Active, on no machine yet.
const shelfRow = s => skillRow(s.name, {id:s.id,shelfId:s.id,description:s.description ?? '',liked:!!s.liked,notes:s.notes ?? '',tags:s.tags ?? [],health:{state:'off',pending:0,total:0,hosts:[],machines:[]}})

export async function openWorkspace(browser, {width, height=812, theme='light', status, skills=[], unread=0, terminalStatus={configured:false,available:false,reason:'disabled'}, terminalMachines=[], hosting={username:'',prefix:'reet-',skills:[]}, sync={environments:[],machines:[],agents:[],targets:[],jobs:[],runs:[],updateJobs:[]}, inventory={environments:[],machines:[]}, routes={}, touch=false, gates={}, sessionFocus, scale} = {}) {
 // `touch` is a phone: a coarse pointer, so the touch-size rules apply.
 const context = await browser.newContext({viewport:{width,height},hasTouch:touch,isMobile:touch,...(scale?{deviceScaleFactor:scale}:{})})
 const page = await context.newPage()
 // A phone opens a session in focus mode. `sessionFocus:false` starts as Exit focus leaves it, for a test about the full session chrome.
 if(sessionFocus===false)await page.addInitScript(()=>{if(localStorage.getItem('reet-terminal-layout:1:context:focus')===null)localStorage.setItem('reet-terminal-layout:1:context:focus','0')})
 const state = {patches:[],status:{mode:'accounts',setupRequired:false,bootstrapRequired:false,signupEnabled:false,supportDesk:true,user:owner,...status}, theme, unexpected:[]}
 const errors = []
 page.on('pageerror',e=>errors.push(e.message))
 // `gates` maps an API path to a promise the answer waits for, so a test can look at the loading state.
 await page.route('**/api/**',async route=>{
  const path = new URL(route.request().url()).pathname
  if(gates[path])await gates[path]
  const json = data => route.fulfill({json:data})
  // A test's own answers, ahead of the defaults below: a function gets the request and the state and returns (or resolves to) {status?, json}, or just the JSON to send.
  if(routes[path]!==undefined){const answer=typeof routes[path]==='function'?await routes[path](route.request(),state):routes[path];return route.fulfill({status:answer?.status||200,headers:answer?.headers,json:answer?.json!==undefined?answer.json:answer})}
  if(path==='/api/v2/auth/status')return json(state.status)
  if(path==='/api/v2/auth/setup'){state.status={...state.status,setupRequired:false,user:owner};return json(owner)}
  if(path==='/api/v2/profile'){
   if(route.request().method()==='PATCH'){const body=route.request().postDataJSON();state.patches.push(body);if(body.theme)state.theme=body.theme}
   return json({user:owner,sessionCount:1,accounts:true,profile:{theme:state.theme,timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',updatedAt:now}})
  }
  if(path==='/api/version')return json({version:'test',commit:'fixture'})
  if(path==='/api/skills')return json({skills,counts:{all:skills.length,used:skills.filter(s=>s.used).length,liked:0,bookmarked:0}})
  // The Skills page: the `skills` fixtures, all Active and on no machine. A test about the page answers the route itself.
  if(path==='/api/v2/skills'&&route.request().method()==='GET')return json(skillsPageOf(skills.map(shelfRow),new URL(route.request().url()).searchParams.get('status')||'active',{machines:0}))
  // Skills New on a machine, for Home's Waiting for you and the bell: none unless a test answers the route itself.
  if(path==='/api/v2/skills/new'&&route.request().method()==='GET')return json({newOnMachines:[]})
  if(path==='/api/environments')return json(inventory)
  // The ignore list went with the old sync: the server answers its routes 410 Gone.
  if(path==='/api/ignored'||path==='/api/skill/ignore')return route.fulfill({status:410,json:{error:'This was part of the old sync, which is gone: every machine follows your library. Make a skill Active or Archived on the Skills page, and see each machine on the Sync page.'}})
  if(path==='/api/tags')return json({tags:[]})
  if(path==='/api/v2/lists')return json({lists:[],linkEnabled:false})
  if(path==='/api/v2/library')return json({schemaVersion:1,items:[]})
  if(path==='/api/v2/sync')return json(sync)
  // The Sync page's read: no machine follows the library unless a test answers the route itself.
  if(path==='/api/v2/sync/skills')return json({machines:[],activity:[],work:noWork,active:0})
  if(path==='/api/v2/sync/candidates')return json({skills})
  if(path==='/api/v2/setup/fleet/batches/active')return json(null)
  if(path==='/api/v2/setup/profiles')return json({profiles:[]})
  if(path==='/api/v2/hosting')return json(hosting)
  if(path==='/api/v2/skill-imports')return json({requests:[],capabilities:[]})
  // As the server answers it: the health read uses the browser cookie only, so a single-token sign-in (which sends its token
  // in a header the read never carries) is refused with 401, and an open instance with 403.
  if(path==='/api/v2/machines/health'){
   if(state.status.mode==='token'&&!route.request().headers()['x-reet-token'])return route.fulfill({status:401,json:{error:'missing or wrong token'}})
   if(state.status.mode==='none')return route.fulfill({status:403,json:{error:'machine health requires an authenticated instance'}})
   return json({serverNow:Date.now(),machines:[]})
  }
  if(path==='/api/v2/profile/model-favourites')return json(modelFavourites())
  if(/^\/api\/v2\/machines\/[^/]+\/models$/.test(path))return json(modelList({state:'unsupported', models:[]}))
  if(/^\/api\/v2\/machines\/[^/]+\/models\/refresh$/.test(path))return json(modelList({state:'unsupported', models:[]}))
  // Every machine's kept model list for one agent (Gaffer settings' crew): none unless a test answers the route itself.
  if(path==='/api/v2/models')return json({machines:[]})
  if(path==='/api/v2/terminal/status')return json(terminalStatus)
  if(path==='/api/v2/terminal/machines')return json({machines:terminalMachines})
  if(path==='/api/v2/terminal/sessions')return json({sessions:[]})
  // A chat whose agent has reported no usage: the status line then shows nothing. A test that wants figures answers the route itself.
  if(/^\/api\/v2\/terminal\/sessions\/[^/]+\/chat\/usage$/.test(path))return json({reported:false,inputTokens:0,outputTokens:0,cachedTokens:0,contextUsed:0,contextSize:0,updatedAt:0})
  // The agent's slash commands: none announced unless a test answers the route itself.
  if(/^\/api\/v2\/terminal\/sessions\/[^/]+\/chat\/commands$/.test(path))return json({commands:[]})
  if(path==='/api/v2/session-backlog')return json({drafts:[]})
  // The Usage page's read: nothing counted yet, and no agents listed. A test that wants figures answers the route itself.
  if(path==='/api/v2/skill-buddy/recommendations')return json({skills:[],stage:'',editable:true,scope:'fixture'})
  if(path==='/api/v2/usage'){const q=new URL(route.request().url()).searchParams;return json({from:Number(q.get('from')),to:Number(q.get('to')),since:0,agents:[],gaffer:[],sessions:[],more:false})}
  if(path==='/api/v2/gaffer/goals'&&route.request().method()==='GET')return json({goals:[],readAt:Date.now()})
  if(path==='/api/v2/gaffer/outcomes'&&route.request().method()==='GET')return json({outcomes:[]})
  // Factory pathways, read by the dialogs that start work: none unless a test answers the route itself.
  if(path==='/api/v2/gaffer/pathways'&&route.request().method()==='GET')return json({pathways:[]})
  // An account that has saved no crew: the outcome dialog then starts every tier on the outcome's own agent.
  if(path==='/api/v2/gaffer/crew'&&route.request().method()==='GET')return json({crew:{light:{},standard:{},heavy:{}},saved:false,tiers:['light','standard','heavy'],defaults:{}})
  if(path==='/api/v2/folder-projects')return json({projects:[],savedWaiting:0})
  if(path==='/api/v2/project-groups')return json({groups:[]})
  // Desks, read by the Desks pages and by Jump once it is opened. None unless a test answers the route itself.
  if(path==='/api/v2/desks'&&route.request().method()==='GET')return json({desks:[]})
  if(path==='/api/v2/notifications/count')return json({unread})
  // Notifications on a device: off unless a test answers the route itself (device-push.e2e.mjs), so the Notifications page has no device card.
  if(path==='/api/v2/push')return json({available:false,devices:[]})
  // A page that inspects a session's workspace reads GitHub by itself: no pull request unless a test answers this route.
  if (/^\/api\/v2\/terminal\/sessions\/[^/]+\/inspect$/.test(path))return json({checkedAt:Date.now(),reason:'no_pr'})
  // What a machine reports when it has nothing to say, read by Home's first-session guide for a connected machine: no sign-ins, no model list. A test that cares answers the route itself.
  if(/^\/api\/v2\/machines\/[^/]+\/accounts(\/refresh)?$/.test(path))return json({state:'read',sentence:'',planLimits:false,checkedAt:Date.now(),accounts:[]})
  if(/^\/api\/v2\/machines\/[^/]+\/models$/.test(path))return json({tool:new URL(route.request().url()).searchParams.get('tool')||'',state:'unsupported',models:[],more:false,source:'',version:'',fetchedAt:null,checkedAt:null,reason:'',sentence:''})
  state.unexpected.push(path)
  return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 })
 return {context, page, state, errors}
}

// `consented` is the machine's own local consent and set-up; `ready` is the server's overall verdict, which also needs the server's terminal access on.
export const terminalMachine = (id, host, ready, consented = ready) => ({agentId:id,host,ready,serverEnabled:true,capabilities:{protocolVersion:1,terminalControl:true,os:'linux',osUser:'dev',terminalEnabled:consented,healthEnabled:false,readiness:consented?'ready':'terminal_disabled'}})
export const active = {configured:true,available:true,reason:''}

// A machine's model list as /api/v2/machines/{id}/models answers it; `models` defaults to two providers so a picker has a
// group to draw. `state` drives the status line: listed, unsupported, failed, off, offline or update.
export const modelEntry = (id, provider = id.includes('/') ? id.slice(0, id.indexOf('/')) : '') => ({id, provider})
export const modelList = (over = {}) => ({tool:'opencode', state:'listed', models:[modelEntry('anthropic/claude-sonnet-4-5'), modelEntry('openai/gpt-5')],
 more:false, source:'opencode models', version:'', fetchedAt:Date.now(), checkedAt:Date.now(), reason:'', sentence:'', ...over})
export const modelFavourites = (favourites = []) => ({version:1, favourites})

// What a fresh instance reports before any machine connects: the server scans itself, which is not a machine the person connected.
export const freshInstanceSync = () => {
  const scope = {id:'srv',host:'controller',agent:'reet-server',name:'controller',skills:[],skillCount:0,complete:true,scannedAt:Date.now(),lastSeen:Date.now(),os:'linux',arch:'amd64',version:'test'}
  return {environments:[scope],machines:[{host:'controller',groups:[scope],skillCount:0}],agents:[],targets:[],jobs:[],runs:[],updateJobs:[]}
}

// A machine whose agent scan found these skills, as the shelf reads it from /api/environments.
export const machineWith = (host, skills) => {
  const scope = {id:`${host}-agents`,host,agent:'agents',name:host,skills,skillCount:skills.length,complete:true,scannedAt:Date.now(),lastSeen:Date.now(),os:'linux',arch:'amd64',version:'test'}
  return {environments:[scope],machines:[{host,groups:[scope],skillCount:skills.length}]}
}
