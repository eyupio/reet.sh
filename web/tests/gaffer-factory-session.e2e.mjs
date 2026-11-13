import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

// A Factory pipeline whose session the person ended. The card says so and offers what can actually be done: run the
// stage again in a new session, move on, or stop following. It never offers to tell an agent that is no longer there.
// A pipeline whose session was removed has no session to open, and a failed first read is not shown as an empty floor.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-factory-session'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),oid='e'.repeat(32)
const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1}
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const general={conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
const kinds=['plan','design','code','review','test','pr','babysit','merge']
const line=at=>kinds.map((kind,i)=>({kind,tool:'claude',model:'',state:i<at?'done':i===at?'running':'pending',requestId:'r'}))
const fresh=()=>[{id:oid,revision:7,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'ended',note:'The code stage is under way.',sessionId:'s1',conversationId:'1'.repeat(32),updatedAt:now,stage:2,stages:line(2)},
  {key:'b',title:'Export docs',prompt:'p',state:'unfollowed',sessionState:'gone',note:'Its session no longer exists, so nothing is following this pipeline any more. Retry it to start a new session, or archive or delete the outcome.',sessionId:'s2',conversationId:'2'.repeat(32),updatedAt:now,stage:1,stages:line(1)},
  {key:'c',title:'Export button',prompt:'p',state:'ready_to_merge',sessionState:'running',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s3',conversationId:'3'.repeat(32),updatedAt:now,stage:7,stages:line(7).map((s,i)=>i===7?{...s,state:'pending'}:s)},
]}]
const goals=[{goal:{...pins,id:'1'.repeat(32),conversationId:'1'.repeat(32),sessionId:'s1',revision:1,text:'Outcome: x',state:'needs_user',note:'The working session has ended or is unavailable. Review its outcome before choosing further work.',maxReviews:12,usedReviews:2,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,lastObservedAt:now,lastReviewedAt:now-90000,summary:'The endpoint is committed; tests are half written.',usageReported:true,inputTokens:900,outputTokens:200},session:{id:'s1',agentId:'a1',title:'Export API',state:'ended'},available:true,attention:true,reason:'The working session has ended or is unavailable.'}]
const machines=[{agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'}]}}]
const projects=[{id:'p1',host:'a1',machine:'workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now}]
try {
  for(const [width,theme] of [[1440,'light'],[390,'dark']]){
    const box={retries:[],moved:[],fail:false},outcomes=fresh()
    const w=await openWorkspace(browser,{width,height:width<500?844:1100,theme,touch:width<1000,terminalMachines:machines,routes:{
      '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},'/api/v2/terminal/sessions':{sessions:[]},'/api/v2/gaffer/goals':{goals,readAt:now},'/api/v2/folder-projects':{projects,savedWaiting:0},
      '/api/v2/gaffer/conversations':r=>new URL(r.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
      '/api/v2/gaffer/outcomes':()=>box.fail?{status:503,json:{error:'The controller is restarting.'}}:{outcomes},
      [`/api/v2/gaffer/outcomes/${oid}/sessions/a/retry`]:request=>{box.retries.push(request.postDataJSON());const piece=outcomes[0].sessions[0];Object.assign(piece,{state:'starting',note:'Trying again…',sessionId:'',conversationId:''});delete piece.sessionState;outcomes[0].revision++;outcomes[0].state='starting';return outcomes[0]},
      [`/api/v2/gaffer/outcomes/${oid}/sessions/a/advance`]:request=>{box.moved.push(request.postDataJSON());return outcomes[0]},
    }})
    try {
      const page=w.page
      await page.goto(base+'/factory')
      const tiles=page.getByRole('list',{name:'Factory at a glance',exact:true}).getByRole('listitem')
      // An ended session wants the person, whatever its goal says.
      await expect(tiles.nth(0)).toContainText('1');await expect(tiles.nth(0)).toContainText('Need you')
      const pipes=page.getByRole('list',{name:'Pipelines of this outcome',exact:true}).locator(':scope > li')
      const ended=pipes.nth(0),gone=pipes.nth(1),ready=pipes.nth(2)
      await expect(ended).toContainText('Needs you')
      const decide=ended.getByRole('region',{name:'What Export API needs from you',exact:true})
      await expect(decide).toContainText('This stage’s session was ended, so nothing is working on it.')
      await expect(decide.getByRole('button')).toHaveText(['Run this stage again in a new session','Move to next stage','Stop following this outcome','Read the ended session'])
      // Nothing that needs a live agent is offered, and Gaffer's last review is still there to read.
      await expect(ended.getByRole('button',{name:'Tell the agent…',exact:true})).toHaveCount(0)
      await expect(ended.getByRole('button',{name:'Have Gaffer look again',exact:true})).toHaveCount(0)
      await expect(ended.getByRole('button',{name:'Send back…',exact:true})).toHaveCount(0)
      await expect(ended).toContainText('The endpoint is committed; tests are half written.')
      // A removed session cannot be opened or told; the pipeline can be tried again.
      if(!(await gone.getByRole('button',{name:/^Hide details/}).count()))await gone.getByRole('button',{name:/^Show details/}).click()
      await expect(gone.getByRole('link',{name:/session/i})).toHaveCount(0)
      await expect(gone.getByRole('button',{name:'Tell the agent…',exact:true})).toHaveCount(0)
      await expect(gone.getByRole('button',{name:'Try again',exact:true})).toBeVisible()
      // A pipeline waiting for the merge decision is asked for that decision, not for a setting that would do nothing now.
      await expect(ready.getByRole('button',{name:'Approve merge',exact:true})).toBeVisible()
      await expect(ready.getByRole('button',{name:/merge by itself/})).toHaveCount(0)
      await page.screenshot({path:`${shots}/gaffer-factory-session-${width}-${theme}.png`,fullPage:width<500})
      if(width<500){
        // A long line on a phone names the stage in hand and leaves the rest as their marks, so no two names collide.
        const names=ended.getByRole('list',{name:'Stages of Export API',exact:true}).locator('.name')
        const boxes=await names.evaluateAll(list=>list.map(el=>el.getBoundingClientRect()).filter(r=>r.width>2).map(r=>[r.left,r.right]))
        expect(boxes.length).toBe(1)
      }
      // Moving on names the stage the person was shown.
      await decide.getByRole('button',{name:'Move to next stage',exact:true}).click()
      await page.getByRole('dialog').getByRole('button',{name:'Move on',exact:true}).click()
      await expect.poll(()=>box.moved).toEqual([{confirmed:true,stage:'code'}])
      // The stage is run again on request, once.
      await decide.getByRole('button',{name:'Run this stage again in a new session',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Trying'})).toHaveText('Trying “Export API” again.')
      expect(box.retries).toEqual([{revision:7}])
      await expect(decide).toHaveCount(0)
      expect(w.errors).toEqual([])
      // A first read that fails is said to have failed; it is not shown as an empty factory.
      box.fail=true
      await page.goto(base+'/factory')
      await expect(page.getByRole('heading',{name:'The factory could not be read.',exact:true})).toBeVisible()
      await expect(page.getByText('No pipelines yet.')).toHaveCount(0)
      box.fail=false
      await page.getByRole('button',{name:'Try again now',exact:true}).click()
      await expect(page.getByRole('article',{name:/^Outcome: Ship CSV export/})).toBeVisible()
    } finally { await w.context.close() }
  }
  console.log('Factory and sessions: an ended session asks the person with choices that work, a removed session offers no dead link, a merge decision is not offered a setting that does nothing, a phone line names one stage, moving on names the stage seen, and a failed first read is not an empty factory, passed on desktop and phone.')
} finally { await browser.close() }
