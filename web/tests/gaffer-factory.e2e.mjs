import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

// The Factory page: Gaffer's outcomes drawn as production lines, with the numbers that say how each is going, the merge
// decision that is the person's, and the setup a new installation still needs. Reading it starts nothing.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-factory'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32)
const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1}
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const general={conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
const goal=(cid,sid,title,state,attention,summary)=>({goal:{...pins,id:cid,conversationId:cid,sessionId:sid,revision:1,text:'Outcome: x',state,note:attention?'The worker needs your decision before continuing.':'Waiting for new worker progress.',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,lastObservedAt:now-120000,lastReviewedAt:now-90000,summary,usageReported:true,inputTokens:1840,outputTokens:412},session:{id:sid,agentId:'a1',title,state:'running'},available:true,attention,...(attention?{reason:'Gaffer brought a question back to you.'}:{})})
const st=(kind,tool,model,state,extra={})=>({kind,tool,model,state,requestId:'r',...extra})
const kinds=['plan','design','code','review','test','pr','babysit','merge']
const line=(doneUpTo,cur)=>kinds.map((k,i)=>st(k,['plan','design','review'].includes(k)?'claude':['code','test'].includes(k)?'codex':'claude',['plan','design','review'].includes(k)?'opus':['code','test'].includes(k)?'':'haiku',i<doneUpTo?'done':i===doneUpTo&&cur?'running':'pending',i<doneUpTo?{summary:'Handed over.'}:{}))
const outcomes=[{id:'e'.repeat(32),revision:5,objective:'Ship CSV export for billing: an endpoint, a button and tests.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
 {key:'a',title:'Export API',prompt:'p',state:'following',note:'The review stage is under way.',sessionId:'s1',conversationId:'1'.repeat(32),updatedAt:now,stage:3,stages:line(3,true)},
 {key:'b',title:'Export button',prompt:'p',state:'following',note:'The code stage is under way.',sessionId:'s2',conversationId:'2'.repeat(32),updatedAt:now,stage:2,stages:line(2,true)},
 {key:'c',title:'Export docs',prompt:'p',state:'ready_to_merge',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s3',conversationId:'3'.repeat(32),updatedAt:now,stage:7,stages:line(7,false)},
]},{id:'d'.repeat(32),revision:2,objective:'Fix the flaky importer test.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'importer',dir:'/work/importer',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-86400000,updatedAt:now,sessions:[
 {key:'a',title:'Importer retry',prompt:'p',state:'done',note:'Every stage is done. Check the result; only you mark the work complete.',sessionId:'s4',conversationId:'4'.repeat(32),updatedAt:now,stage:3,autoMerge:true,stages:['code','test','pr','merge'].map(k=>st(k,'claude','','done',{summary:'ok'}))},
]}]
const goals=[goal('1'.repeat(32),'s1','Export API','active',false,'The endpoint and its tests are committed. The review found one missing error case and is fixing it.'),goal('2'.repeat(32),'s2','Export button','needs_user',true,'The button is in, but the design leaves the empty state open. Which of the two do you want?'),goal('5'.repeat(32),'s5','Login tokens','active',false,'Checks are green.')]
const sessions=['s1','s2','s3','s4','s5','s6'].map((id,n)=>({id,agentId:'a1',title:['Export API','Export button','Export docs','Importer retry','Login tokens','Docs refresh'][n],state:'running',structured:true,tool:'claude',dir:'/work/x',createdAt:now,updatedAt:now,exitCode:null}))
const machines=[{agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,acp:true,acpLiveControls:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'},{tool:'codex',adapter:'codex-acp',state:'available'}]}}]
const projects=[{id:'p1',host:'a1',machine:'workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now}]
const routesFor=box=>({'/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},'/api/v2/terminal/sessions':{sessions},'/api/v2/gaffer/goals':{goals,readAt:now},'/api/v2/gaffer/outcomes':{outcomes},'/api/v2/folder-projects':{projects,savedWaiting:0},'/api/v2/gaffer/conversations':r=>new URL(r.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/c/merge`]:request=>{box.merges.push(request.postDataJSON());const o=outcomes[0],piece=o.sessions[2];Object.assign(piece,{state:'following',mergeApproved:true,note:'The merge stage is under way.'});piece.stages[7].state='running';o.revision++;return o}})

const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})
try {
  for(const [width,theme] of [[1440,'light'],[390,'light'],[1440,'dark']]){
    const box={writes:[],merges:[]}
    // Each run starts from the same record: the third pipeline is waiting for the merge decision.
    Object.assign(outcomes[0],{revision:5});Object.assign(outcomes[0].sessions[2],{state:'ready_to_merge',mergeApproved:false,note:'Ready to merge. Nothing merges until you approve it.'});outcomes[0].sessions[2].stages[7].state='pending'
    const w=await openWorkspace(browser,{width,height:width<500?844:1100,theme,touch:width<1000,terminalMachines:machines,routes:routesFor(box)})
    watch(w,box)
    try {
      const page=w.page
      // The Factory is a place of its own in the navigation.
      await page.goto(base+'/')
      const tab=page.getByRole('navigation',{name:'Main navigation',exact:true}).getByRole('link',{name:'Factory',exact:true})
      await expect(tab.first()).toHaveAttribute('href','/factory');await tab.first().click()
      await expect(page).toHaveURL(/\/factory$/);await expect(page).toHaveTitle(/Factory/)
      await expect(page.getByRole('heading',{name:'Factory',exact:true,level:1})).toBeVisible()
      await expect(page.getByRole('navigation',{name:'Main navigation',exact:true}).getByRole('link',{name:'Factory',exact:true}).first()).toHaveAttribute('aria-current','page')
      // The headline numbers say who has to act.
      const tiles=page.getByRole('list',{name:'Factory at a glance',exact:true}).getByRole('listitem')
      await expect(tiles).toHaveText(['1Need you','1Ready to merge','1Running','1Done'])
      // With a machine, a project and a provider in place there is nothing left to set up.
      await expect(page.getByRole('region',{name:'Set up your factory',exact:true})).toHaveCount(0)
      const first=page.getByRole('article',{name:/^Outcome: Ship CSV export for billing/})
      const pipes=first.getByRole('list',{name:'Pipelines of this outcome',exact:true}).locator(':scope > li')
      await expect(pipes).toHaveCount(3)
      // A pipeline shows every stage: done, in hand and to come, and who is doing the one in hand.
      const api=pipes.nth(0),stages=api.getByRole('list',{name:'Stages of Export API',exact:true}).getByRole('listitem')
      await expect(stages).toHaveCount(8)
      await expect(stages.nth(2)).toContainText('done');await expect(stages.nth(3)).toHaveAttribute('aria-current','step');await expect(stages.nth(3)).toContainText('in hand');await expect(stages.nth(4)).toContainText('to come')
      await expect(api).toContainText('Claude Code · opus');await expect(api).toContainText('3 of 12');await expect(api).toContainText('2,252');await expect(api).toContainText('Waits for you')
      await expect(api).toContainText('The review found one missing error case')
      // A pipeline waiting on the person says so and why.
      await expect(pipes.nth(1)).toContainText('Needs you');await expect(pipes.nth(1)).toContainText('Gaffer brought a question back to you.')
      // A finished pipeline that was allowed to merge says so.
      const done=page.getByRole('article',{name:/^Outcome: Fix the flaky importer test/})
      await expect(done).toContainText('Done');await expect(done).toContainText('By itself when green')
      // A pipeline's own goal is not listed again among the other sessions; a session started by hand is.
      const floor=page.locator('[data-floor]')
      await expect(floor.getByRole('list',{name:'Followed session goals',exact:true}).getByRole('listitem')).toHaveCount(1)
      await expect(floor.getByRole('heading',{name:'Login tokens',exact:true})).toBeVisible()
      await expect(floor.getByRole('group',{name:'Running chats not followed',exact:true}).getByRole('checkbox')).toHaveCount(1)
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      await page.screenshot({path:`${shots}/gaffer-factory-${width}-${theme}.png`,fullPage:width<500})
      expect(box.writes).toEqual([])
      // Merging is the person's decision: it is asked for plainly, can be backed out of, and is sent once.
      const docs=pipes.nth(2)
      await expect(docs).toContainText('Ready to merge')
      await expect(docs.getByRole('list',{name:'Stages of Export docs',exact:true}).getByRole('listitem').nth(7)).toContainText('waiting for you')
      await docs.getByRole('button',{name:'Approve merge',exact:true}).click()
      const confirm=page.getByRole('dialog',{name:'Merge “Export docs”?',exact:true})
      await expect(confirm).toContainText('once every check passes, no review thread is open and there are no conflicts')
      await confirm.getByRole('button',{name:'Cancel',exact:true}).click();await expect(confirm).toHaveCount(0);expect(box.writes).toEqual([])
      await docs.getByRole('button',{name:'Approve merge',exact:true}).click()
      await confirm.getByRole('button',{name:'Approve merge',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Merge approved'})).toContainText('Merge approved for “Export docs”.')
      expect(box.merges).toEqual([{revision:5,confirmed:true}])
      await expect(docs.getByRole('button',{name:'Approve merge',exact:true})).toHaveCount(0)
      await expect(docs).toContainText('You approved it')
      await expect(tiles).toHaveText(['1Need you','0Ready to merge','2Running','1Done'])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // A new installation is told what Gaffer still needs, each with the place to do it, and has no pipelines yet.
  {
    const box={writes:[],merges:[]}
    const w=await openWorkspace(browser,{width:1280,height:1000,routes:{'/api/v2/gaffer/providers':{providers:[],presets:[],available:true,canUseLocal:true,maxProviders:12}}})
    watch(w,box)
    try {
      const page=w.page
      await page.goto(base+'/factory')
      const setup=page.getByRole('region',{name:'Set up your factory',exact:true})
      await expect(setup.getByRole('listitem')).toHaveCount(3)
      await expect(setup.getByRole('link',{name:'Add a machine',exact:true})).toHaveAttribute('href','/machines')
      await expect(setup.getByRole('link',{name:'Add a project on Home',exact:true})).toHaveAttribute('href','/')
      await expect(setup.getByRole('link',{name:'Open Gaffer settings',exact:true})).toHaveAttribute('href','/account/gaffer')
      await expect(setup).toContainText('switched on by default')
      await expect(page.getByRole('heading',{name:'No pipelines yet.',exact:true})).toBeVisible()
      await expect(page.getByRole('list',{name:'Factory at a glance',exact:true}).getByRole('listitem')).toHaveText(['0Need you','0Ready to merge','0Running','0Done'])
      expect(box.writes).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // How Gaffer works is set once for the account: looking after sessions is pre-approved, and pipelines do not merge by themselves, until changed.
  {
    const box={writes:[],merges:[]}
    const w=await openWorkspace(browser,{width:1280,height:1000,routes:routesFor(box)})
    try {
      const page=w.page
      await page.goto(base+'/account/gaffer')
      const ways=page.getByRole('region',{name:'How Gaffer works',exact:true})
      const standing=ways.getByRole('checkbox',{name:/^Look after sessions without asking each time/}),auto=ways.getByRole('checkbox',{name:/^Let new pipelines merge by themselves/})
      await expect(standing).toBeChecked();await expect(auto).not.toBeChecked()
      await expect(ways).toContainText('Nothing is read or sent until you ask')
      await auto.check();await expect.poll(()=>w.state.patches).toEqual([{gafferAutoMerge:true}])
      await standing.uncheck();await expect.poll(()=>w.state.patches).toEqual([{gafferAutoMerge:true},{gafferStanding:false}])
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Factory page: a navigation tab, headline numbers, pipelines with every stage and who does it, a question waiting on the person, a finished pipeline, other sessions without the pipelines’ own goals, the merge decision asked for and sent once, setup guidance for a new installation and the account’s two Gaffer defaults passed on desktop, phone and dark.')
} finally { await browser.close() }
