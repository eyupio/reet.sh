// A browser page with the workspace API answered from fixtures, for UI contract tests. Unmocked private paths answer 404 and are recorded.
export const owner = {id:1,name:'Sam',email:'sam@example.test',role:'owner'}

const now = Date.now()
export const skill = id => ({id:`demo/toolkit/${id}`,name:id,slug:id,source:'demo/toolkit',sourceType:'github',url:`https://www.skills.sh/demo/toolkit/${id}`,description:'Example skill for UI verification.',notes:'',installs:0,installsWeek:0,trend:[],liked:false,bookmarked:false,used:false,createdAt:now,updatedAt:now})

export async function openWorkspace(browser, {width, height=812, theme='light', status, skills=[], unread=0, terminalStatus={configured:false,available:false,reason:'disabled'}, terminalMachines=[], hosting={username:'',prefix:'reet-',skills:[]}} = {}) {
 const context = await browser.newContext({viewport:{width,height}})
 const page = await context.newPage()
 const state = {status:{mode:'accounts',setupRequired:false,bootstrapRequired:false,signupEnabled:false,user:owner,...status}, theme, unexpected:[]}
 const errors = []
 page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',route=>{
  const path = new URL(route.request().url()).pathname
  const json = data => route.fulfill({json:data})
  if(path==='/api/v2/auth/status')return json(state.status)
  if(path==='/api/v2/auth/setup'){state.status={...state.status,setupRequired:false,user:owner};return json(owner)}
  if(path==='/api/v2/profile')return json({user:owner,sessionCount:1,accounts:true,profile:{theme:state.theme,timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',updatedAt:now}})
  if(path==='/api/version')return json({version:'test',commit:'fixture'})
  if(path==='/api/skills')return json({skills,counts:{all:skills.length,used:skills.filter(s=>s.used).length,liked:0,bookmarked:0}})
  if(path==='/api/environments')return json({environments:[],machines:[]})
  if(path==='/api/tags')return json({tags:[]})
  if(path==='/api/v2/lists')return json({lists:[],linkEnabled:false})
  if(path==='/api/v2/sync')return json({environments:[],machines:[],agents:[]})
  if(path==='/api/v2/setup/profiles')return json({profiles:[]})
  if(path==='/api/v2/hosting')return json(hosting)
  if(path==='/api/v2/machines/health')return json({serverNow:Date.now(),machines:[]})
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
