import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-stage-follow'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='goal-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),now=Date.now()
function fixture(structured=true){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Prepare the release',tool:'opencode',model:'working-model',structured,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={allowed:false,starts:[],controls:[],workerWrites:0}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal:view.goal??null,pins,allowed:!!view.goal&&box.allowed,setupAllowed:box.allowed,providerName:'Workshop Gaffer',conversationRevision:view.conversation.revision,requiresProviderAccess:!view.goal})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  [`/api/v2/gaffer/conversations/${cid}/goal/setup`]:request=>{const in_=request.postDataJSON();expect(in_.confirmed).toBe(true);expect(in_.conversationRevision).toBe(view.conversation.revision);if(in_.action==='resume'){box.controls.push(in_);view.goal.state='active';view.goal.revision++;return goalView()}if(in_.action==='start'){expect(in_.revision).toBe(view.goal?.revision??0);for(const [key,value]of Object.entries(pins))expect(in_[key]).toBe(value);box.starts.push(in_);view.goal={...pins,id:'b'.repeat(32),conversationId:cid,sessionId:sid,revision:(view.goal?.revision??0)+1,text:in_.text,state:'active',note:'Waiting for new worker progress. No model request was made.',maxReviews:in_.maxReviews,usedReviews:0,createdAt:now,updatedAt:now,expiresAt:now+in_.hours*3600000,nextAt:now,lastSeq:-1,inputTokens:0,outputTokens:0,usageReported:true}}return goalView()},
  [`/api/v2/gaffer/conversations/${cid}/goal/control`]:request=>{const in_=request.postDataJSON();expect(in_.revision).toBe(view.goal.revision);if(in_.action==='resume')expect(in_.confirmed).toBe(true);box.controls.push(in_);view.goal.state={pause:'paused',resume:'active',complete:'completed',stop:'stopped'}[in_.action];view.goal.revision++;view.goal.note=in_.action==='complete'?'You marked this goal complete.':'Goal updated.';return goalView()},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[],savedWaiting:0},
  '/api/v2/yolo':{allowed:false},
  [`/api/v2/terminal/sessions/${sid}/skills`]:{skills:[],machineSeen:Date.now(),host:'Workshop'},
  [`/api/v2/terminal/sessions/${sid}/inspect`]:{status:503,json:{error:'Machine unavailable'}},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:request=>{if(request.method()==='POST')box.workerWrites++;return{queue:[]}},
 }
 return{routes,view,box}
}

// The Overview's stages offer follow-up items: in a chat they open Gaffer's follow-up editor with the goal written in (nothing starts),
// in a terminal session they copy the same words for the agent.
const PROGRESS='Progress'
// The Details panel holds the Overview; it is optional on every screen, so open it the way a person does.
async function overview(page){
 await page.locator('.session-screen header.head').waitFor({state:'visible',timeout:60000})
 const details=page.getByRole('button',{name:'Session details panel',exact:true})
 if(await details.getAttribute('aria-expanded')!=='true')await details.click()
 const panel=page.getByRole('complementary',{name:'Session context'});await expect(panel).toBeVisible()
 return panel.getByRole('group',{name:PROGRESS})
}
try{
 for(const theme of ['light','dark']){
  const f=fixture(),w=await openWorkspace(browser,{width:1600,height:1000,theme,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
   const tracker=await overview(w.page),goal=w.page.getByRole('dialog',{name:'Look after this session',exact:true})
   const cell=name=>tracker.getByRole('button',{name:`${name}: follow-up options`,exact:true})
   await expect(tracker).toBeVisible()
   const before=await tracker.evaluate(el=>({h:el.getBoundingClientRect().height,next:el.querySelector('.next').getBoundingClientRect().top}))
   // Click: the stage's menu, below the row, over the Next slot; nothing moves.
   await cell('PR').click()
   const menu=w.page.getByRole('menu',{name:'PR: follow-up options',exact:true})
   await expect(menu).toBeVisible();await expect(menu.getByRole('menuitem')).toHaveText(['Babysit the pull request','Babysit it, then merge when green'])
   await expect(menu.getByRole('menuitem').first()).toBeFocused();await expect(cell('PR')).toHaveAttribute('aria-expanded','true')
   expect(await tracker.evaluate(el=>({h:el.getBoundingClientRect().height,next:el.querySelector('.next').getBoundingClientRect().top}))).toEqual(before)
   if(theme==='dark')await tracker.screenshot({path:`${shots}/stage-menu-${theme}.png`})
   // Escape closes it and puts focus back on the stage.
   await w.page.keyboard.press('Escape');await expect(menu).toBeHidden();await expect(cell('PR')).toBeFocused();await expect(cell('PR')).toHaveAttribute('aria-expanded','false')
   // Right-click opens the same menu; a press elsewhere closes it.
   await cell('Review').click({button:'right'})
   await expect(w.page.getByRole('menu',{name:'Review: follow-up options',exact:true}).getByRole('menuitem')).toHaveText(['Have Gaffer review it','Address the review comments'])
   await w.page.mouse.click(5,5);await expect(w.page.getByRole('menu')).toHaveCount(0)
   // Keyboard: Enter opens it, arrows move, Enter picks. The editor opens with the goal written in, and nothing is started.
   await cell('Merge').focus();await w.page.keyboard.press('Enter')
   const merge=w.page.getByRole('menu',{name:'Merge: follow-up options',exact:true})
   await expect(merge.getByRole('menuitem',{name:'Merge when green',exact:true})).toBeFocused()
   await w.page.keyboard.press('ArrowDown');await expect(merge.getByRole('menuitem',{name:'Keep conflicts resolved',exact:true})).toBeFocused()
   await w.page.keyboard.press('ArrowUp');await w.page.keyboard.press('Enter')
   const field=goal.getByLabel('Agreed goal',{exact:true})
   await expect(goal).toBeVisible();f.box.allowed=true
   await expect(field).toHaveValue(/merged as soon as every check passes, no review thread is open and there are no conflicts, then tell me\. Bring decisions back to me\.$/)
   await expect(field).toBeFocused()
   expect(f.box.starts,'picking an item starts nothing').toHaveLength(0)
   await expect(goal.getByRole('button',{name:'Start follow-up',exact:true})).toBeVisible()
   await w.page.keyboard.press('Escape');await expect(goal).toBeHidden()
   // The ordinary entry is unchanged: no goal is carried over.
   await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).isVisible().catch(()=>{})
   await cell('Checks').click();await w.page.getByRole('menuitem',{name:'Fix failing checks',exact:true}).click()
   await expect(field).toHaveValue(/Ensure every failing check on the pull request is fixed and stays passing\. Bring decisions back to me\. Do not merge\.$/)
   expect(f.box.starts).toHaveLength(0)
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 {
  // A terminal session has no follow-up: the same words are copied for the agent, and the menu says so.
  const f=fixture(false),w=await openWorkspace(browser,{width:1600,height:1000,theme:'dark',routes:f.routes})
  try{
   await w.context.grantPermissions(['clipboard-read','clipboard-write'])
   await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
   const tracker=await overview(w.page)
   await tracker.getByRole('button',{name:'PR: follow-up options',exact:true}).click()
   const menu=w.page.getByRole('menu',{name:'PR: follow-up options',exact:true})
   await expect(menu.getByText('Copy a message for the agent',{exact:true})).toBeVisible()
   await menu.getByRole('menuitem',{name:'Copy message for the agent: Babysit the pull request',exact:true}).click()
   await expect(tracker.getByRole('status')).toHaveText('Copied')
   expect(await w.page.evaluate(()=>navigator.clipboard.readText())).toMatch(/^Look after the pull request: ensure failing checks are fixed.* Do not merge\.$/)
   await expect(w.page.getByRole('dialog',{name:'Look after this session',exact:true})).toHaveCount(0)
   expect(f.box.starts).toHaveLength(0);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Stage follow-ups: click, right-click and keyboard menus, focus return, no layout shift, goal editor prefilled and never started, terminal copy')
}finally{await browser.close()}
