import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// The Factory page's pieces, one labelled block each: the stage pipeline, a waiting item, the pipeline detail, and the
// page that puts them together (the list beside the detail on a wide screen, the detail as its own screen on a narrow one).
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-pieces'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32),now=Date.now()
// Three pipelines of one outcome: Export API waits on a decision Gaffer brought back (its advice is to move on), Export
// button is working by itself, and Export docs waits for its merge to be approved.
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Export API · code',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={moved:[],posted:[]}
 const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:sid,revision:3,text:'Outcome: x',state:'active',note:'Gaffer brought a question back.',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,summary:'Tests pass locally.',inputTokens:0,outputTokens:0,usageReported:true,advice:{choice:'move',why:'The review stage will catch the rest.'}}
 const row={goal,session:{id:sid,agentId:'a1',title:session.title,state:session.state},available:true,attention:true,reason:'Gaffer brought a question back.'}
 const stage=(kind,state,s=sid,c=cid)=>({kind,tool:'opencode',model:'',state,requestId:'r',...(state==='running'?{sessionId:s,conversationId:c}:{})})
 const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:sid,conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running'),stage('review','pending'),stage('merge','pending')]},
  {key:'b',title:'Export button',prompt:'p',state:'following',sessionState:'running',note:'The review stage is under way.',sessionId:'s-b',conversationId:'b'.repeat(32),updatedAt:now,stage:1,stages:[stage('code','done'),stage('review','running','s-b','b'.repeat(32)),stage('merge','pending')]},
  {key:'c',title:'Export docs',prompt:'p',state:'ready_to_merge',sessionState:'running',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s-c',conversationId:'d'.repeat(32),updatedAt:now,stage:2,stages:[stage('code','done'),stage('review','done'),stage('merge','pending')]}]}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal,pins,allowed:true,setupAllowed:true,providerName:'Workshop Gaffer',conversationRevision:1,requiresProviderAccess:false})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  '/api/v2/gaffer/outcomes':()=>({outcomes:[outcome]}),
  '/api/v2/gaffer/goals':()=>({goals:[row],readAt:now}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  [`/api/v2/gaffer/outcomes/${oid}/automation`]:request=>{const input=request.postDataJSON();box.posted.push(input);outcome.automation=input.on;return outcome},
  // Moving Export API on: its code stage is done, review is in hand, and Gaffer is no longer waiting on the person.
  [`/api/v2/gaffer/outcomes/${oid}/sessions/a/advance`]:request=>{box.moved.push(request.postDataJSON());const piece=outcome.sessions[0];piece.stages[0].state='done';piece.stage=1;piece.stages[1]={...piece.stages[1],state:'running',sessionId:sid,conversationId:cid};piece.note='The review stage is under way.';row.attention=false;delete row.reason;delete goal.advice;outcome.revision++;return outcome},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[{id:'p1',host:'a1',machine:'Workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:0,version:1,createdAt:now,updatedAt:now}],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
 }
 return{routes,outcome,box,row}
}
async function open(width,{theme='light',height,hash='',tweak,scale}={}){
 const f=fixture();tweak?.(f)
 const w=await openWorkspace(browser,{width,height:height??(width<500?844:1000),theme,touch:width<1000,routes:f.routes,scale})
 await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 await w.page.goto(`${base}/factory${hash}`)
 return{f,w,page:w.page}
}
const sideways=page=>page.evaluate(()=>document.documentElement.scrollWidth)
try{
 // The stage pipeline: three stages, the first in hand, and no sideways scroll on a phone.
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  const {w,page}=await open(width,{theme})
  try{
   const stages=page.getByRole('list',{name:'Stages of Export API',exact:true})
   await expect(stages).toBeVisible()
   await expect(stages.getByRole('listitem')).toHaveCount(3)
   await expect(stages.getByRole('listitem').first()).toHaveAttribute('aria-current','step')
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/stage-pipeline-${width}-${theme}.png`})
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - stage pipeline')
 // A waiting item: its filled action and the way to the detail; a working one has no filled button.
 {
  const {w,page}=await open(1000,{height:900})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting.getByRole('button',{name:'Move to next stage',exact:true})).toBeVisible()
   await expect(waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true})).toBeVisible()
   await page.getByRole('button',{name:/^Working/}).click()
   const working=page.getByRole('region',{name:'Working',exact:true})
   await expect(working.locator(`#pipeline-${oid}-b`)).toContainText('Export button')
   await expect(working.locator('.btn.primary')).toHaveCount(0)
  }finally{await w.context.close()}
  console.log('ok - waiting item')
 }
 // The pipeline detail: one bar of what can be done, the settings in a line, who decides, and the facts of a picked stage.
 {
  const {f,w,page}=await open(1440)
  try{
   // On a wide screen the list's item is one button that opens its pipeline beside the list.
   const item=page.getByRole('region',{name:'Waiting for you',exact:true}).getByRole('button',{name:'Decision: Export API',exact:true})
   await item.click();await expect(item).toHaveAttribute('aria-pressed','true')
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
   await expect(bar.getByRole('button').first()).toHaveAccessibleName(/^Move to next stage/)
   await expect(bar.getByRole('button',{name:'Tell the agent…',exact:true})).toHaveCount(1)
   await expect(bar.getByRole('button',{name:/^Move to next stage/})).toHaveCount(1)
   await expect(bar.getByRole('button',{name:'Goal and limits',exact:true})).toHaveCount(0)
   await page.getByText('Usage and budget',{exact:true}).click()
   await expect(page.getByRole('button',{name:'Goal and limits',exact:true})).toBeVisible()
   await expect(page.getByText('You decide · Ask me · merge waits for you',{exact:true})).toBeVisible()
   await page.getByText('Settings for this outcome',{exact:true}).click()
   await page.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
   await expect.poll(()=>f.box.posted).toEqual([{revision:5,on:true}])
   await page.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('button',{name:/Review/}).click()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toBeVisible()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toContainText('will do this stage')
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - pipeline detail')
 }
 // The page on a wide screen: the list and the detail side by side, the first waiting pipeline open on arrival.
 for(const [width,theme] of [[1280,'light'],[1440,'dark']]){
  const {f,w,page}=await open(width,{theme})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   await expect(waiting.getByRole('button',{name:'Decision: Export API',exact:true})).toHaveAttribute('aria-pressed','true')
   // One line of counts replaces the tiles.
   await expect(page.getByText('2 waiting for you · 1 working · 0 finished',{exact:true})).toBeVisible()
   await expect(page.getByRole('list',{name:'Factory at a glance',exact:true})).toHaveCount(0)
   // Working starts closed and still says what is in it; Finished or stopped and Other sessions are closed too.
   const workingHead=page.getByRole('button',{name:/^Working/})
   await expect(workingHead).toHaveAttribute('aria-expanded','false');await expect(workingHead).toContainText('Export button')
   await expect(page.getByRole('button',{name:/^Finished or stopped/})).toHaveAttribute('aria-expanded','false')
   await expect(page.getByRole('button',{name:/^Other sessions/})).toHaveAttribute('aria-expanded','false')
   // The detail's bar stays at the foot of the visible detail.
   const pane=page.locator('.pane'),barBox=await page.getByRole('group',{name:'What do you want to do?',exact:true}).boundingBox(),paneBox=await pane.boundingBox()
   expect(barBox.y+barBox.height).toBeLessThanOrEqual(paneBox.y+paneBox.height+1);expect(barBox.y+barBox.height).toBeGreaterThan(paneBox.y+paneBox.height-40)
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-page-${width}-${theme}.png`})
   if(width===1280){
    // Choosing another item opens it, and the address names it so a reload reopens it.
    await waiting.getByRole('button',{name:'Ready to merge: Export docs',exact:true}).click()
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await expect(page).toHaveURL(new RegExp(`[?&]pipeline=${oid}\\.c(&|$)`))
    await page.reload()
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await waiting.getByRole('button',{name:'Decision: Export API',exact:true}).click()
    // Taking the decision moves the pipeline to its next stage and out of Waiting for you; the detail stays on it.
    await page.getByRole('group',{name:'What do you want to do?',exact:true}).getByRole('button',{name:/^Move to next stage/}).click()
    await page.getByRole('dialog').getByRole('button',{name:'Move on',exact:true}).click()
    await expect.poll(()=>f.box.moved).toEqual([{confirmed:true,stage:'code'}])
    await expect(waiting.getByRole('button',{name:'Decision: Export API',exact:true})).toHaveCount(0)
    await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem').nth(1)).toHaveAttribute('aria-current','step')
    // A section the person opened stays open after a reload.
    await workingHead.click();await expect(workingHead).toHaveAttribute('aria-expanded','true')
    await page.reload()
    await expect(page.getByRole('button',{name:/^Working/})).toHaveAttribute('aria-expanded','true')
    await expect(page.getByRole('region',{name:'Working',exact:true}).getByRole('button',{name:'Running: Export API',exact:true})).toBeVisible()
   }
   expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 // A link to a pipeline (Home's, or an old bookmark) opens that pipeline.
 for(const width of [1280,834]){
  const {w,page}=await open(width,{hash:`#pipeline-${oid}-b`})
  try{
   const title=page.getByRole('heading',{name:'Export button',level:2,exact:true})
   await expect(title).toBeVisible();await expect(title).toBeFocused()
   if(width<1100)await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0)
   // Its section opens, though Working starts closed, so the chosen item is in view in the list.
   else{await expect(page.getByRole('button',{name:/^Working/})).toHaveAttribute('aria-expanded','true');await expect(page.getByRole('button',{name:'Running: Export button',exact:true})).toHaveAttribute('aria-pressed','true')}
  }finally{await w.context.close()}
 }
 console.log('ok - the page on a wide screen')
 // The page on a tablet and a phone: the list is the page, and Details opens one pipeline as a screen of its own.
 for(const [width,theme] of [[834,'light'],[390,'dark']]){
  const {w,page}=await open(width,{theme})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toHaveCount(0)
   await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toHaveCount(0)
   // From 600 px the cards name each stage; below it they are circles with a caption.
   const stages=waiting.getByRole('list',{name:'Stages of Export API',exact:true})
   if(width>=600)await expect(stages.locator('.name')).toHaveText(['Code','Review','Merge'])
   else await expect(waiting.getByText('Code in hand · the first stage · next Review',{exact:true})).toBeVisible()
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-list-${width}-${theme}.png`,fullPage:true})
   await waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   // The focus moves to the detail's title, not to the top of the document.
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeFocused()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0)
   await expect(page).toHaveURL(new RegExp(`[?&]pipeline=${oid}\\.a(&|$)`))
   const nav=page.getByRole('navigation',{name:'Waiting items',exact:true})
   await expect(nav).toContainText('1 of 2 waiting for you')
   // The bar sits at the foot of the screen while the detail scrolls.
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true}),box=await bar.boundingBox(),inner=await page.evaluate(()=>innerHeight)
   expect(box.y+box.height).toBeLessThanOrEqual(inner+1)
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-detail-${width}-${theme}.png`})
   // Previous and next step through what is waiting, in order.
   await nav.getByRole('button',{name:'Next waiting item',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await expect(nav).toContainText('2 of 2 waiting for you');await expect(nav.getByRole('button',{name:'Next waiting item',exact:true})).toBeDisabled()
   await nav.getByRole('button',{name:'Previous waiting item',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   // Back to the list, and the address no longer names a pipeline.
   await nav.getByRole('button',{name:'‹ Factory',exact:true}).click()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toHaveCount(0)
   expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull()
   // The focus is back on the Details button of the item that was open.
   await expect(waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true})).toBeFocused()
   // The browser's Back closes a detail opened from the list, and Forward opens it again.
   await waiting.getByRole('button',{name:'Details and other actions for Export docs',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await page.goBack()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toHaveCount(0)
   expect(new URL(page.url()).pathname).toBe('/factory');expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull()
   await page.goForward()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - the page on a tablet and a phone')

 // The Projects picker: a second project with one running pipeline, so the counts and the narrowing have something to say.
 const twoProjects=f=>{
  const stage=(kind,state)=>({kind,tool:'opencode',model:'',state,requestId:'r'})
  const other={id:'f'.repeat(32),revision:1,objective:'Tidy the rail.',state:'settled',note:'',agentId:'a1',projectId:'p2',projectName:'reet',dir:'/work/reet',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
   {key:'z',title:'Rail tidy',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:'s-z',conversationId:'9'.repeat(32),updatedAt:now,stage:0,stages:[stage('code','running'),stage('merge','pending')]}]}
  f.routes['/api/v2/gaffer/outcomes']=()=>({outcomes:[f.outcome,other]})
  const folder=f.routes['/api/v2/folder-projects'];f.routes['/api/v2/folder-projects']={projects:[folder.projects[0],{...folder.projects[0],id:'p2',dir:'/work/reet',name:'reet',position:1}],savedWaiting:0}
 }
 for(const width of [834,390]){
  const {w,page}=await open(width,{tweak:twoProjects})
  try{
   const pick=page.getByRole('button',{name:'Projects: all',exact:true})
   await expect(pick).toBeVisible()
   await expect(page.getByText('Export API',{exact:true}).first()).toBeVisible()
   await page.screenshot({path:`${shots}/picker-closed-${width}.png`})
   await pick.click()
   const dialog=page.getByRole('dialog')
   await expect(dialog).toBeVisible()
   await expect(dialog.getByRole('button',{name:/^All projects/})).toHaveAttribute('aria-pressed','true')
   await expect(dialog.getByRole('button',{name:/^billing/})).toContainText('2 waiting · 1 working')
   await expect(dialog.getByRole('button',{name:/^reet/})).toContainText('0 waiting · 1 working')
   await page.screenshot({path:`${shots}/picker-open-${width}.png`})
   await dialog.getByRole('button',{name:/^billing/}).click()
   await expect(dialog).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Projects: billing',exact:true})).toBeVisible()
   await expect(page.getByText('Rail tidy',{exact:true})).toHaveCount(0)
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await page.getByRole('button',{name:'Projects: billing',exact:true}).click()
   await expect(page.getByRole('dialog').getByRole('button',{name:/^billing/})).toHaveAttribute('aria-pressed','true')
   await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
   await page.getByRole('button',{name:'Projects: billing',exact:true}).click()
   await page.getByRole('dialog').getByRole('button',{name:/^All projects/}).click()
   await expect(page.getByRole('button',{name:'Projects: all',exact:true})).toBeVisible()
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - projects picker on a tablet and a phone')
 // The wide Projects panel says how many of each project's pipelines wait and work, and has no picker button.
 {
  const {w,page}=await open(1440,{tweak:twoProjects})
  try{
   await expect(page.getByRole('button',{name:/^Projects: /})).toHaveCount(0)
   await expect(page.getByRole('button',{name:/^billing.*2 waiting · 1 working/})).toBeVisible()
   await expect(page.getByRole('button',{name:/^reet.*0 waiting · 1 working/})).toBeVisible()
   await expect(page.locator('aside.rail')).toContainText('2 waiting · 1 working')
   await page.screenshot({path:`${shots}/projects-panel-1440.png`})
  }finally{await w.context.close()}
  console.log('ok - projects panel counts')
 }
 // A phone with five pipelines of eight stages: no sideways scroll, big touch targets, and the caption under the circles.
 {
  const eight=['plan','design','code','review','test','pr','babysit','merge']
  const {w,page}=await open(390,{tweak:f=>{
   f.outcome.sessions=['a','b','c','d','e'].map((key,i)=>({key,title:`Pipeline ${i+1}`,prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:`s-${key}`,conversationId:String(i+1).repeat(32),updatedAt:now,stage:2,stages:eight.map((kind,n)=>({kind,tool:'opencode',model:'',state:n<2?'done':n===2?'running':'pending',requestId:'r'}))}))
  }})
  try{
   await page.getByRole('button',{name:/^Working/}).click()
   const card=page.locator(`#pipeline-${oid}-a`)
   await expect(card).toContainText('Code in hand · Plan, Design done · next Review')
   const heights=await card.getByRole('button').evaluateAll(list=>list.map(b=>b.getBoundingClientRect().height))
   expect(heights.length).toBeGreaterThan(0);for(const h of heights)expect(h).toBeGreaterThanOrEqual(44)
   await page.screenshot({path:`${shots}/five-pipelines-390.png`})
   expect(await sideways(page)).toBeLessThanOrEqual(390)
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - five pipelines of eight stages on a phone')
 }
 // The floating Gaffer button stays clear of the detail's action bar and its closed rows.
 {
  const {w,page}=await open(1440)
  try{
   await expect(page.locator('.gaffer-launcher')).toBeVisible()
   await expect(page.getByText('Settings for this outcome',{exact:true})).toBeVisible()
   await page.waitForTimeout(400)
   await page.screenshot({path:`${shots}/launcher-1440.png`})
   const lb=await page.evaluate(()=>{const r=document.querySelector('.gaffer-launcher')?.getBoundingClientRect();return r?{l:r.left,t:r.top,r:r.right,b:r.bottom}:null})
   expect(lb).not.toBeNull()
   const overlaps=await page.evaluate(box=>{const hit=[];for(const e of document.querySelectorAll('.detail .row summary, .detail .row .label, .detail .row .headline, .detail .bar button, .detail .bar .ask')){const r=e.getBoundingClientRect();if(r.width&&r.height&&r.left<box.r&&r.right>box.l&&r.top<box.b&&r.bottom>box.t)hit.push(e.textContent.trim().slice(0,40))}return hit},lb)
   expect(overlaps).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - the launcher stays clear of the detail')
 }
 // The five captures kept in docs/images/ui, at twice the pixel density and as the person sees the screen (not full page,
 // so the fixed bars sit where they are). Saved only when REET_UI_SCREENSHOTS names a folder.
 if(process.env.REET_UI_SCREENSHOTS){
  for(const [width,theme] of [[1440,'light'],[1440,'dark']]){
   const {w,page}=await open(width,{theme,scale:2})
   try{
    await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toBeVisible()
    await page.waitForTimeout(400)
    await page.screenshot({path:`${shots}/factory-rework-${width}-${theme}.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  {
   const {w,page}=await open(834,{scale:2,height:1112})
   try{
    const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
    await expect(waiting).toBeVisible()
    await page.waitForTimeout(400)
    await page.screenshot({path:`${shots}/factory-rework-834-light-list.png`,fullPage:false})
    await waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true}).click()
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toBeVisible()
    await page.waitForTimeout(400)
    await page.screenshot({path:`${shots}/factory-rework-834-light-detail.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  {
   const {w,page}=await open(390,{theme:'dark',scale:2})
   try{
    await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
    await page.waitForTimeout(400)
    await page.screenshot({path:`${shots}/factory-rework-390-dark.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  console.log('ok - captures saved')
 }
}finally{await browser.close()}
