// A browser page with the workspace API answered from fixtures, for UI contract tests. Unmocked private paths answer 404 and are recorded.
export const owner = {id:1,name:'Sam',email:'sam@example.test',role:'owner'}

const now = Date.now()
export const skill = id => ({id:`demo/toolkit/${id}`,name:id,slug:id,source:'demo/toolkit',sourceType:'github',url:`https://www.skills.sh/demo/toolkit/${id}`,description:'Example skill for UI verification.',notes:'',installs:0,installsWeek:0,trend:[],liked:false,bookmarked:false,used:false,createdAt:now,updatedAt:now})

export async function openWorkspace(browser, {width, height=812, theme='light', status, skills=[], unread=0, terminalStatus={configured:false,available:false,reason:'disabled'}, terminalMachines=[], hosting={username:'',prefix:'reet-',skills:[]}, sync={environments:[],machines:[],agents:[],targets:[],jobs:[],runs:[],updateJobs:[]}, inventory={environments:[],machines:[]}, routes={}, touch=false, gates={}} = {}) {
 // `touch` is a phone: a coarse pointer, so the touch-size rules apply.
 const context = await browser.newContext({viewport:{width,height},hasTouch:touch,isMobile:touch})
 const page = await context.newPage()
 const state = {patches:[],status:{mode:'accounts',setupRequired:false,bootstrapRequired:false,signupEnabled:false,user:owner,...status}, theme, unexpected:[]}
 const errors = []
 page.on('pageerror',e=>errors.push(e.message))
 // `gates` maps an API path to a promise the answer waits for, so a test can look at the loading state.
 await page.route('**/api/**',async route=>{
  const path = new URL(route.request().url()).pathname
  if(gates[path])await gates[path]
  const json = data => route.fulfill({json:data})
  // A test's own answers, ahead of the defaults below: a function gets the request and the state and returns (or resolves to) {status?, json}, or just the JSON to send.
  if(routes[path]!==undefined){const answer=typeof routes[path]==='function'?await routes[path](route.request(),state):routes[path];return route.fulfill({status:answer?.status||200,json:answer?.json!==undefined?answer.json:answer})}
  if(path==='/api/v2/auth/status')return json(state.status)
  if(path==='/api/v2/auth/setup'){state.status={...state.status,setupRequired:false,user:owner};return json(owner)}
  if(path==='/api/v2/profile'){
   if(route.request().method()==='PATCH'){const body=route.request().postDataJSON();state.patches.push(body);if(body.theme)state.theme=body.theme}
   return json({user:owner,sessionCount:1,accounts:true,profile:{theme:state.theme,timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',updatedAt:now}})
  }
  if(path==='/api/version')return json({version:'test',commit:'fixture'})
  if(path==='/api/skills')return json({skills,counts:{all:skills.length,used:skills.filter(s=>s.used).length,liked:0,bookmarked:0}})
  if(path==='/api/environments')return json(inventory)
  if(path==='/api/tags')return json({tags:[]})
  if(path==='/api/v2/lists')return json({lists:[],linkEnabled:false})
  if(path==='/api/v2/sync')return json(sync)
  if(path==='/api/v2/setup/profiles')return json({profiles:[]})
  if(path==='/api/v2/hosting')return json(hosting)
  // As the server answers it: the health read uses the browser cookie only, so a single-token sign-in (which sends its token
  // in a header the read never carries) is refused with 401, and an open instance with 403.
  if(path==='/api/v2/machines/health'){
   if(state.status.mode==='token'&&!route.request().headers()['x-reet-token'])return route.fulfill({status:401,json:{error:'missing or wrong token'}})
   if(state.status.mode==='none')return route.fulfill({status:403,json:{error:'machine health requires an authenticated instance'}})
   return json({serverNow:Date.now(),machines:[]})
  }
  if(path==='/api/v2/terminal/status')return json(terminalStatus)
  if(path==='/api/v2/terminal/machines')return json({machines:terminalMachines})
  if(path==='/api/v2/terminal/sessions')return json({sessions:[]})
  if(path==='/api/v2/notifications/count')return json({unread})
  state.unexpected.push(path)
  return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 })
 return {context, page, state, errors}
}

// `consented` is the machine's own local consent and set-up; `ready` is the server's overall verdict, which also needs the server's terminal access on.
export const terminalMachine = (id, host, ready, consented = ready) => ({agentId:id,host,ready,serverEnabled:true,capabilities:{protocolVersion:1,terminalControl:true,os:'linux',osUser:'dev',terminalEnabled:consented,healthEnabled:false,readiness:consented?'ready':'terminal_disabled'}})
export const active = {configured:true,available:true,reason:''}

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
