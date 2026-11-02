import { chromium, expect as playwrightExpect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const expect=playwrightExpect.configure({timeout:20000})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:900},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage(),errors=[],inputs=[],tickets=[],ticketSizes=[],writes=[]
page.on('pageerror',error=>errors.push(error.message))
const now=Date.now(),agentA='connector-laptop000000001',agentB='connector-builder00000001',sessionA='session-build00000000001',sessionB='session-deploy0000000001'
let failHistory=false,historyReads=0,failSessions=false,denied=false,viewer=false,websockets=0
const sessions=[{id:sessionA,agentId:agentA,title:'Build workspace',state:'running',createdAt:now,updatedAt:now,exitCode:null},{id:sessionB,agentId:agentB,title:'Deployment checks',state:'running',createdAt:now,updatedAt:now,exitCode:null}]
const machines=[{agentId:agentA,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,readiness:'ready'}},{agentId:agentB,host:'build-server',ready:true,serverEnabled:true,capabilities:{osUser:'builder',terminalEnabled:true,terminalControl:true,readiness:'ready'}}]
await page.addInitScript(()=>{
 window.__nativeClipboard=navigator.clipboard
 let clipboard='from-clipboard'
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{readText:async()=>clipboard,writeText:async text=>{clipboard=text;window.__copied=text}}})
 window.__setClipboard=text=>clipboard=text
})
await page.route('**/api/**',async route=>{
 const request=route.request(),url=new URL(request.url()),path=url.pathname
 let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Workspace tester',email:'test@example.test',role:viewer?'viewer':'owner'}}
 else if(path==='/api/v2/profile')data={profile:{theme:'light',shelfView:'list',cardsExpanded:false}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}
 else if(path==='/api/environments')data={environments:[],machines:[]}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data={environments:[],machines:[],targets:[],agents:[],jobs:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'fixture',commit:'fixture'}
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[],history:[]}
 else if(path==='/api/v2/terminal/machines')data={machines}
 else if(path==='/api/v2/terminal/status')data={available:true,configured:true,reason:''}
 else if(path==='/api/v2/terminal/sessions'){
  if(request.method()!=='GET'){writes.push(request.postDataJSON());return route.fulfill({status:400,json:{error:'No implicit creation permitted'}})}
  if(failSessions||denied)return route.fulfill({status:denied?403:503,json:{error:'Unavailable'}})
  data={sessions}
 }
 else if(path.endsWith('/attachments')){tickets.push(path.split('/').at(-2));ticketSizes.push(request.postDataJSON());data={ticket:'fixture-ticket00000000000'}}
 else if(path.endsWith('/history')){historyReads++;return route.fulfill(failHistory?{status:503,json:{error:'History temporarily unavailable'}}:{body:Array.from({length:400},(_,i)=>`earlier output ${i}`).join('\n')})}
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
let releaseInitialRedraw,holdInitialRedraw=false
const resizes=[]
await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{
 websockets++
 socket.onMessage(message=>{
  if(typeof message==='string'){
   const data=JSON.parse(message)
   if(data.ticket){socket.send('{"type":"ready"}');const redraw=()=>socket.send(Buffer.from('\x1b[?2026h\x1b[2J\x1b[HExisting persistent shell\r\nready> \x1b[?2026l'));if(holdInitialRedraw){holdInitialRedraw=false;releaseInitialRedraw=redraw}else redraw()}
   if(data.type==='resize')resizes.push(data)
  }else{inputs.push(message.toString());socket.send(message)}
 })
})
const pane=page.locator('.pane'),screen=page.getByRole('region',{name:'Remote terminal screen',exact:true})
async function live(){await expect(pane).toHaveAttribute('data-connection-state','live')}
try {
 // Both empty-state actions open settings; cancelling never creates or switches sessions.
 await page.goto(`${base}/sessions?view=terminal`)
 await expect(page.getByRole('heading',{name:'Start a new session'})).toBeVisible()
 await expect(page.locator('.ended .create-form')).toHaveCount(0)
 await page.getByRole('button',{name:'✦ Start a new agent session',exact:true}).click()
 await expect(page.getByRole('dialog')).toBeVisible()
 await page.keyboard.press('Escape')
 const terminalDialog=page.getByRole('dialog',{name:'New terminal session'})
 for(const button of ['New session','✦ Start a new Terminal session']){
  await page.getByRole('button',{name:button,exact:true}).click()
  await expect(terminalDialog).toBeVisible()
  await expect(terminalDialog.getByLabel('Machine',{exact:true})).toHaveValue('')
  await expect(terminalDialog.getByRole('button',{name:'New terminal',exact:true})).toBeDisabled()
  await terminalDialog.getByLabel('Machine',{exact:true}).selectOption(agentA)
  await expect(terminalDialog.getByRole('button',{name:'New terminal',exact:true})).toBeEnabled()
  await page.getByRole('button',{name:'Close New terminal session'}).click()
 }
 await expect(page.getByRole('heading',{name:'Start a new session'})).toBeVisible()
 expect(writes).toHaveLength(0)
 // A missing/ended session gets the same two actions. Machine/folder drafts stay local until confirmation.
 await page.goto(`${base}/sessions?view=terminal&agent=${agentA}&session=removed-session`)
 await expect(page.getByRole('heading',{name:'This session has ended',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'✦ Start a new Terminal session',exact:true}).click()
 await terminalDialog.getByLabel('Machine',{exact:true}).selectOption(agentB)
 await terminalDialog.getByLabel('Folder',{exact:true}).fill('relative/path')
 await expect(terminalDialog.getByRole('button',{name:'New terminal',exact:true})).toBeDisabled()
 await expect(terminalDialog.getByText('Use a full path that starts with /.')).toBeVisible()
 await terminalDialog.getByLabel('Folder',{exact:true}).fill('/srv/project with spaces')
 await terminalDialog.getByLabel('New session name').fill('Configured shell')
 expect(new URL(page.url()).searchParams.get('agent')).toBe(agentA)
 expect(writes).toHaveLength(0)
 let uncertain=true
 const configuredCreates=[]
 const configuredRoute=async route=>{
  if(route.request().method()==='GET')return route.fallback()
  const body=route.request().postDataJSON();configuredCreates.push(body)
  if(uncertain){uncertain=false;return route.fulfill({status:503,json:{error:'Reply unavailable'}})}
  const created={id:'configured-terminal0000001',agentId:body.agentId,title:body.title,dir:body.dir,state:'running',createdAt:now,updatedAt:now,exitCode:null}
  sessions.push(created)
  return route.fulfill({json:created})
 }
 await page.route('**/api/v2/terminal/sessions',configuredRoute)
 await terminalDialog.getByRole('button',{name:'New terminal',exact:true}).click()
 await expect(terminalDialog.getByText(/The creation is unconfirmed/)).toBeVisible()
 await expect(terminalDialog.getByLabel('Folder',{exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 await page.getByRole('button',{name:'✦ Start a new Terminal session',exact:true}).click()
 await terminalDialog.getByLabel('Machine',{exact:true}).selectOption(agentB)
 await expect(terminalDialog.getByLabel('Folder',{exact:true})).toHaveValue('/srv/project with spaces')
 await terminalDialog.getByRole('button',{name:'Retry creation',exact:true}).click()
 await live()
 expect(configuredCreates).toHaveLength(2)
 expect(configuredCreates[1]).toEqual(configuredCreates[0])
 expect(configuredCreates[0]).toMatchObject({agentId:agentB,title:'Configured shell',dir:'/srv/project with spaces'})
 expect(new URL(page.url()).searchParams.get('agent')).toBe(agentB)
 // From a running session, New session also opens settings and preserves the live view when cancelled.
 await page.getByRole('button',{name:'New session',exact:true}).click()
 await terminalDialog.getByLabel('Machine',{exact:true}).selectOption(agentA)
 await expect(terminalDialog.getByLabel('Folder',{exact:true})).toHaveValue('')
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 await live()
 expect(new URL(page.url()).searchParams.get('session')).toBe('configured-terminal0000001')
 expect(configuredCreates).toHaveLength(2)
 await page.unroute('**/api/v2/terminal/sessions',configuredRoute)

 // An unavailable machine is visible but cannot start a terminal; another machine remains selectable.
 machines[0].ready=false
 await page.goto(`${base}/sessions?view=terminal&agent=${agentA}&session=removed-session`)
 await page.getByRole('button',{name:'✦ Start a new Terminal session',exact:true}).click()
 await expect(terminalDialog.getByLabel('Machine',{exact:true})).toHaveValue(agentA)
 await expect(terminalDialog.getByRole('button',{name:'New terminal',exact:true})).toBeDisabled()
 await terminalDialog.getByLabel('Machine',{exact:true}).selectOption(agentB)
 await expect(terminalDialog.getByRole('button',{name:'New terminal',exact:true})).toBeEnabled()
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 // The compact actions and settings stay within a phone viewport.
 await page.setViewportSize({width:390,height:844})
 await expect(page.getByRole('button',{name:'✦ Start a new Terminal session',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'✦ Start a new Terminal session',exact:true}).click()
 await expect(terminalDialog).toBeVisible()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 await page.keyboard.press('Escape')
 expect(errors).toEqual([])
 console.log('Terminal start passed: both modals, cancellation, machine and folder settings, validation, immutable retry, confirmed navigation, unavailable machines and phone layout.')
} finally { await browser.close() }
