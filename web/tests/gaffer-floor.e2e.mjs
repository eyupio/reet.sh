import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

// The factory floor: running chats nobody asked Gaffer to look after sit beside the followed ones, several can be
// ticked, and one review starts each through the same reviewed setup a single session uses.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-floor'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),generalId='f'.repeat(32)
const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:0,workerRevision:0}
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
const session=(id,title,extra={})=>({id,agentId:'a1',title,state:'running',structured:true,tool:'claude',dir:`/work/${id}`,createdAt:now,updatedAt:now,exitCode:null,...extra})
const goalFor=(conversationId,sessionId,text,maxReviews,hours)=>({...pins,id:conversationId,conversationId,sessionId,revision:1,text,state:'active',note:'Waiting for new worker progress.',maxReviews,usedReviews:0,expiresAt:now+hours*3600000,createdAt:now,updatedAt:now,nextAt:now,lastSeq:0,summary:'',usageReported:false,inputTokens:0,outputTokens:0})

function fixture({active=1}={}){
  const box={writes:[],created:[],setups:[],controls:[],goals:[],conversations:new Map(),sessionsFail:false}
  const sessions=[
    session('floor-ready','Billing export'),
    session('floor-blocked','Flaky importer'),
    session('floor-plain','Plain shell',{structured:false}),
    session('floor-ended','Finished spike',{state:'exited'}),
  ]
  for(let n=0;n<active;n++){
    const sid=`floor-followed-${n}`,cid=(n+1).toString(16).padStart(32,'c')
    sessions.push(session(sid,`Followed work ${n+1}`))
    box.goals.push({goal:goalFor(cid,sid,'Raise the release PR. Do not merge.',12,24),session:{id:sid,agentId:'a1',title:`Followed work ${n+1}`,state:'running'},available:true,attention:false})
  }
  // Each followed goal answers its own fresh read and control, as its card would use.
  const followedRoutes={}
  for(const row of box.goals){
    const path=`/api/v2/gaffer/conversations/${row.goal.conversationId}/goal`
    followedRoutes[path]=()=>({goal:row.goal,pins,allowed:true,setupAllowed:true})
    followedRoutes[path+'/control']=request=>{
      const input=request.postDataJSON();box.controls.push({sid:row.goal.sessionId,...input})
      if(row.goal.sessionId==='floor-followed-2')return {status:409,json:{error:'That goal changed. Reload it.'}}
      row.goal.state=input.action==='pause'?'paused':'stopped';row.goal.revision++
      return {goal:row.goal,pins,allowed:true,setupAllowed:true}
    }
  }
  // The blocked session already has a Gaffer chat; the ready one has none until the review creates it.
  box.conversations.set('floor-blocked',{id:'b'.repeat(32),sessionId:'floor-blocked',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now})
  const general={conversation:{id:generalId,sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
  const routes={
    ...followedRoutes,
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/terminal/sessions':()=>box.sessionsFail?{status:503,json:{error:'The machines could not be reached.'}}:{sessions},
    '/api/v2/gaffer/goals':request=>{expect(request.method()).toBe('GET');return {goals:box.goals,readAt:now}},
    '/api/v2/gaffer/conversations':request=>{
      const query=new URL(request.url()).searchParams
      if(request.method()==='POST'){
        const input=request.postDataJSON();box.created.push(input)
        const conversation={id:'d'.repeat(32),sessionId:input.sessionId,providerId:input.providerId,model:input.model,revision:1,createdAt:now,updatedAt:now}
        box.conversations.set(input.sessionId,conversation);return {conversation,turns:[]}
      }
      if(query.get('all')==='true')return {conversations:[general.conversation,...box.conversations.values()]}
      const sid=query.get('sessionId')||''
      if(!sid)return general
      const found=box.conversations.get(sid)
      return found?{conversation:found,turns:[]}:{status:404,json:{error:'No conversation yet.'}}
    },
  }
  for(const [cid,sid,title] of [['d'.repeat(32),'floor-ready','Billing export'],['b'.repeat(32),'floor-blocked','Flaky importer']]){
    const path=`/api/v2/gaffer/conversations/${cid}/goal`
    routes[path]=request=>{expect(request.method()).toBe('GET');return {goal:null,pins,allowed:false,setupAllowed:true,providerName:'Workshop Gaffer',requiresProviderAccess:true,conversationRevision:1}}
    routes[path+'/setup']=request=>{
      const input=request.postDataJSON();box.setups.push({sid,...input})
      if(sid==='floor-blocked')return {status:409,json:{error:'The worker’s message channel is unavailable.'}}
      const goal=goalFor(cid,sid,input.text,input.maxReviews,input.hours)
      box.goals.push({goal,session:{id:sid,agentId:'a1',title,state:'running'},available:true,attention:false})
      return {goal,pins,allowed:true,setupAllowed:true}
    }
  }
  return {box,routes}
}
// The floor's sessions are part of the Factory page.
async function openFloor(w){
  await w.page.goto(base+'/factory')
  await expect(w.page.getByRole('heading',{name:'Factory',exact:true,level:1})).toBeVisible()
  return w.page.locator('[data-floor]')
}
const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})

try {
  for(const theme of ['light','dark'])for(const width of [1440,390]){
    const f=fixture(),w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,routes:f.routes})
    watch(w,f.box)
    try {
      const floor=await openFloor(w)
      // Followed work and running chats share the sheet; a plain terminal and an ended chat are not offered.
      await expect(floor.getByRole('listitem')).toHaveCount(1)
      const picks=floor.getByRole('group',{name:'Running chats not followed',exact:true})
      await expect(picks.getByRole('checkbox')).toHaveCount(2)
      await expect(picks.getByText('Plain shell',{exact:true})).toHaveCount(0)
      await expect(picks.getByText('Finished spike',{exact:true})).toHaveCount(0)
      await expect(picks.getByText('Followed work 1',{exact:true})).toHaveCount(0)
      await expect(floor.getByRole('button',{name:'Not looked after 2',exact:true})).toBeVisible()
      await expect(floor.getByRole('button',{name:'Following 1',exact:true})).toBeVisible()
      await expect(floor.getByRole('button',{name:'Needs you',exact:true})).toBeVisible()
      await expect(picks.getByRole('link',{name:'Open Billing export',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=floor-ready')
      const lookAfter=floor.getByRole('button',{name:/^Look after/}),reviewFirst=floor.getByRole('button',{name:'Review first',exact:true})
      await expect(lookAfter).toBeDisabled()
      expect(await floor.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await floor.getByRole('button',{name:/^Not looked after/}).click()
      await expect(floor.getByRole('listitem')).toHaveCount(0);await expect(picks.getByRole('checkbox')).toHaveCount(2)
      await floor.getByRole('button',{name:/^Following/}).click()
      await expect(floor.getByRole('listitem')).toHaveCount(1);await expect(picks).toHaveCount(0)
      await floor.getByRole('button',{name:'All',exact:true}).click()
      await floor.getByRole('button',{name:'Select all',exact:true}).click()
      await expect(lookAfter).toHaveText('Look after 2 selected')
      if(theme==='light'||width===1440)await floor.screenshot({path:`${shots}/gaffer-floor-${width}-${theme}.png`})
      // Ticking and opening the review share nothing and start nothing.
      await reviewFirst.click()
      const review=w.page.getByRole('dialog',{name:'Look after these sessions',exact:true})
      await expect(review.getByRole('list',{name:'Sessions to look after',exact:true}).getByRole('listitem')).toHaveCount(2)
      await expect(review.getByText('Workshop Gaffer · gaffer-model',{exact:false})).toBeVisible()
      const goal=review.getByRole('textbox',{name:'Goal for each session',exact:true})
      await expect(goal).toHaveValue(/^Ensure a pull request is raised/)
      await review.getByRole('combobox',{name:'Use a prompt',exact:true}).selectOption({label:'Get the checks green'})
      await expect(goal).toHaveValue(/^Watch the pull request’s checks\./)
      await review.getByText('Change limits',{exact:true}).click()
      await review.getByRole('spinbutton',{name:'Review budget',exact:true}).fill('6')
      await review.getByRole('spinbutton',{name:'Follow for (hours)',exact:true}).fill('48')
      await expect(review.getByText('Up to 12 Gaffer reviews in all',{exact:false})).toBeVisible()
      expect(await review.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      if(theme==='light'||width===1440)await review.screenshot({path:`${shots}/gaffer-floor-review-${width}-${theme}.png`})
      await review.getByRole('button',{name:'Cancel',exact:true}).click();await expect(review).toHaveCount(0)
      expect(f.box.writes).toEqual([])
      // Cancelling keeps the ticks. An empty goal cannot start.
      await expect(lookAfter).toHaveText('Look after 2 selected')
      await reviewFirst.click()
      await goal.fill('   ');await expect(review.getByRole('button',{name:'Look after 2 sessions',exact:true})).toBeDisabled()
      await review.getByRole('combobox',{name:'Use a prompt',exact:true}).selectOption({label:'Get the checks green'})
      await review.getByText('Change limits',{exact:true}).click()
      await review.getByRole('spinbutton',{name:'Review budget',exact:true}).fill('6')
      await review.getByRole('spinbutton',{name:'Follow for (hours)',exact:true}).fill('48')
      await review.getByRole('button',{name:'Look after 2 sessions',exact:true}).click()
      // One started through its own conversation and reviewed setup; the other failed alone and says why.
      await expect(review.getByRole('status')).toHaveText('1 started, 1 not started. The ones not started were left unchanged.')
      const results=review.getByRole('list',{name:'Results',exact:true}).getByRole('listitem')
      await expect(results).toHaveCount(2)
      await expect(results.nth(0)).toContainText('Billing export');await expect(results.nth(0)).toContainText('Following')
      await expect(results.nth(1)).toContainText('Flaky importer');await expect(results.nth(1)).toContainText('Not started · The worker’s message channel is unavailable.')
      expect(f.box.created).toEqual([{sessionId:'floor-ready',providerId:pid,model:''}])
      expect(f.box.setups.map(s=>s.sid)).toEqual(['floor-ready','floor-blocked'])
      for(const sent of f.box.setups){
        expect(sent).toMatchObject({...pins,revision:0,conversationRevision:1,confirmed:true,action:'start',maxReviews:6,hours:48})
        expect(sent.text).toMatch(/^Watch the pull request’s checks\..*Do not merge\.$/)
      }
      expect(f.box.writes).toEqual(['POST /api/v2/gaffer/conversations',`POST /api/v2/gaffer/conversations/${'d'.repeat(32)}/goal/setup`,`POST /api/v2/gaffer/conversations/${'b'.repeat(32)}/goal/setup`])
      await review.getByRole('button',{name:'Back to the floor',exact:true}).click();await expect(review).toHaveCount(0)
      // The started session joins the job sheet; the failed one stays ticked for another go.
      await expect(floor.getByRole('listitem')).toHaveCount(2)
      await expect(floor.getByRole('heading',{name:'Billing export',exact:true})).toBeVisible()
      await expect(picks.getByRole('checkbox')).toHaveCount(1);await expect(picks.getByRole('checkbox')).toBeChecked()
      await expect(lookAfter).toHaveText('Look after 1 selected')
      // The goal and limits are this browser's starting point next time.
      await reviewFirst.click()
      await expect(review.getByRole('textbox',{name:'Goal for this session',exact:true})).toHaveValue(/^Watch the pull request’s checks\./)
      await expect(review.getByText('6 reviews · 48 hours per session',{exact:false})).toBeVisible()
      await w.page.keyboard.press('Escape');await expect(review).toHaveCount(0)
      // A failed session read keeps the followed work and says what is missing.
      f.box.sessionsFail=true;await floor.getByRole('button',{name:'Refresh',exact:true}).click()
      await expect(floor.getByText('The machines could not be reached.',{exact:false})).toBeVisible()
      await expect(floor.getByRole('listitem')).toHaveCount(2)
      expect(f.box.writes).toHaveLength(3);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // The account's ten-goal limit is said before the review, not discovered one failure at a time.
  for(const [active,warning] of [[9,'Gaffer has room for 1 more active goal. Untick 1.'],[10,'Gaffer is already following ten sessions. Pause or stop one first.']]){
    const f=fixture({active}),w=await openWorkspace(browser,{width:1280,height:1000,routes:f.routes})
    watch(w,f.box)
    try {
      const floor=await openFloor(w)
      await floor.getByRole('button',{name:/^Not looked after/}).click()
      await floor.getByRole('button',{name:'Select all',exact:true}).click()
      await expect(floor.getByText(warning,{exact:true})).toBeVisible()
      await expect(floor.getByRole('button',{name:'Look after 2 selected',exact:true})).toBeDisabled()
      if(active===9){
        await floor.getByRole('checkbox',{name:/Flaky importer/}).uncheck()
        await expect(floor.getByRole('button',{name:'Look after 1 selected',exact:true})).toBeEnabled()
      }
      expect(f.box.writes).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // One action for the sessions shown: pause every followed one, or stop them all after a confirmation.
  {
    const f=fixture({active:3}),w=await openWorkspace(browser,{width:1280,height:1000,routes:f.routes})
    watch(w,f.box)
    try {
      const floor=await openFloor(w)
      await floor.getByRole('button',{name:/^Following/}).click()
      await floor.getByRole('button',{name:'Pause all 3 following',exact:true}).click()
      // Each goal is read afresh and paused on its own; one refusal is named and does not stop the others.
      await expect(floor.getByRole('status').filter({hasText:'Paused'})).toHaveText('Paused 2 of 3. The working models keep running. Not changed: Followed work 3: That goal changed. Reload it.')
      expect(f.box.controls.map(c=>[c.sid,c.action,c.revision,c.confirmed])).toEqual([['floor-followed-0','pause',1,false],['floor-followed-1','pause',1,false],['floor-followed-2','pause',1,false]])
      await floor.getByRole('button',{name:'All',exact:true}).click()
      await floor.getByRole('button',{name:'Stop all 3 goals',exact:true}).click()
      const confirm=w.page.getByRole('dialog',{name:'Stop 3 goals?',exact:true})
      await expect(confirm).toContainText('Resuming is done one session at a time')
      await confirm.getByRole('button',{name:'Stop goals',exact:true}).click()
      await expect(floor.getByRole('status').filter({hasText:'Stopped'})).toContainText('Stopped 2 of 3.')
      expect(f.box.controls.slice(3).map(c=>c.action)).toEqual(['stop','stop','stop'])
      expect(f.box.writes).toHaveLength(6);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // With nothing followed yet, Home still offers the floor in one click and says how many chats are waiting.
  for(const width of [1280,390]){
    const f=fixture({active:0}),w=await openWorkspace(browser,{width,height:900,touch:width<1000,routes:f.routes})
    watch(w,f.box)
    try {
      await w.page.goto(base+'/')
      const entry=w.page.getByRole('link',{name:'Gaffer: 2 running chats not followed',exact:true})
      await expect(entry).toBeVisible()
      expect((await entry.boundingBox()).height).toBeGreaterThanOrEqual(44)
      await entry.click()
      await expect(w.page).toHaveURL(/\/factory$/)
      const floor=w.page.locator('[data-floor]')
      await expect(floor.getByRole('group',{name:'Running chats not followed',exact:true}).getByRole('checkbox')).toHaveCount(2)
      await expect(floor.getByRole('list',{name:'Sessions Gaffer looks after',exact:true})).toHaveCount(0)
      // Looking after sessions is pre-approved by default: one click starts them with the usual goal and limits, and says so.
      await expect(floor.getByText('You pre-approved this in Gaffer settings.',{exact:false})).toBeVisible()
      await floor.getByRole('button',{name:'Select all',exact:true}).click()
      await floor.getByRole('button',{name:'Look after 2 selected',exact:true}).click()
      const quick=w.page.getByRole('dialog',{name:'Look after these sessions',exact:true})
      await expect(quick.getByRole('status')).toHaveText('1 started, 1 not started. The ones not started were left unchanged.')
      await expect(quick.getByRole('textbox')).toHaveCount(0)
      expect(f.box.setups.map(s=>s.sid)).toEqual(['floor-ready','floor-blocked'])
      expect(f.box.setups[0]).toMatchObject({confirmed:true,action:'start',maxReviews:12,hours:24})
      expect(f.box.setups[0].text).toMatch(/^Ensure a pull request is raised.*Do not merge\.$/)
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A person who turned pre-approval off is shown the review every time.
  {
    const f=fixture({active:0}),w=await openWorkspace(browser,{width:1280,height:900,routes:{...f.routes,'/api/v2/profile':{user:{id:1,name:'Sam',email:'sam@example.test',role:'owner'},sessionCount:1,accounts:true,profile:{theme:'light',timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,hostedPrefix:'reet-',gafferStanding:false,updatedAt:now}}}})
    watch(w,f.box)
    try {
      const floor=await openFloor(w)
      await floor.getByRole('button',{name:'Select all',exact:true}).click()
      await expect(floor.getByRole('button',{name:'Review first',exact:true})).toHaveCount(0)
      await floor.getByRole('button',{name:'Look after 2 selected',exact:true}).click()
      await expect(w.page.getByRole('dialog',{name:'Look after these sessions',exact:true}).getByRole('button',{name:'Look after 2 sessions',exact:true})).toBeVisible()
      expect(f.box.writes).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Factory floor: four theme/width views, running chats beside followed work, filters, multi-select, one combined review with prompt and limits, per-session setup and results, partial failure, saved starting point, failed session read, the ten-goal limit, bulk pause and stop, the Home entry to the Factory, one-click look-after when pre-approved and the review when not passed.')
} finally { await browser.close() }
