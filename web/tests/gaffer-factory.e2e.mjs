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
const machines=[{agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'},{tool:'codex',adapter:'codex-acp',state:'available',options:[{id:'effort',category:'thought_level',values:['default','low','high']}]}]}}]
// What the working agents reported: the code stage's own count, and the review stage in hand with its context window.
Object.assign(outcomes[0].sessions[0].stages[2],{agentInputTokens:60000,agentOutputTokens:24600,agentCachedTokens:410000,agentUsageReported:true})
Object.assign(outcomes[0].sessions[0].stages[3],{agentInputTokens:9000,agentOutputTokens:3400,agentUsageReported:true,contextUsed:176000,contextSize:200000})
// The first pipeline's plan was moved on by the person, and its review stage in hand reports what its agent is doing.
Object.assign(outcomes[0].sessions[0].stages[0],{closedBy:'person'});Object.assign(outcomes[0].sessions[0].stages[1],{closedBy:'gaffer'})
Object.assign(outcomes[0].sessions[0].stages[3],{startedAt:now-42*60000,agentState:'working',agentActiveAt:now-20*60000})
const story=[{id:9,at:now-60000,source:'gaffer',action:'gaffer-stage-started',detail:JSON.stringify({outcome:'e'.repeat(32),pipeline:'a',stage:'review',reason:'The review stage is under way.'})},{id:8,at:now-120000,source:'gaffer',action:'gaffer-stage-handed-over',detail:JSON.stringify({outcome:'e'.repeat(32),pipeline:'a',stage:'code',by:'gaffer',reason:'x'})}]
const projects=[{id:'p1',host:'a1',machine:'workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now}]
const routesFor=box=>({'/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},'/api/v2/terminal/sessions':{sessions},'/api/v2/gaffer/goals':{goals,readAt:now},'/api/v2/gaffer/outcomes':{outcomes},'/api/v2/folder-projects':{projects,savedWaiting:0},'/api/v2/gaffer/conversations':r=>new URL(r.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/history`]:{events:story},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/c/merge`]:request=>{box.merges.push(request.postDataJSON());const o=outcomes[0],piece=o.sessions[2];Object.assign(piece,{state:'following',mergeApproved:true,note:'The merge stage is under way.'});piece.stages[7].state='running';o.revision++;return o},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/b/advance`]:request=>{box.moved.push(request.postDataJSON());const o=outcomes[0],piece=o.sessions[1];piece.stages[piece.stage].state='done';piece.stage++;piece.stages[piece.stage].state='running';piece.note='The review stage is under way.';o.revision++;return o},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/a/tell`]:request=>{box.told.push(request.postDataJSON());return {message:{id:'m1',state:'queued'}}},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/a/back`]:request=>{const input=request.postDataJSON();box.back.push(input);const o=outcomes[0],piece=o.sessions[0];piece.note=`Sent back to the ${input.stage} stage. Starting it again.`;o.revision++;return o},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions/a/automerge`]:request=>{const input=request.postDataJSON();box.modes.push(input);const o=outcomes[0];o.sessions[0].autoMerge=input.on;o.revision++;return o},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/review`]:request=>{box.reviews.push(request.postDataJSON());const o=outcomes[0];o.revision++;o.report={text:'The endpoint is in review and the button is waiting on your answer. Nothing covers the changelog.',at:now,model:'gaffer-model',inputTokens:300,outputTokens:80,usageReported:true,proposals:[{title:'Changelog entry',prompt:'Add a changelog entry for CSV export. Do not merge.',why:'No pipeline covers the changelog.',stages:['code','pr']},{title:'Export metrics',prompt:'Add a counter for exports. Do not merge.',why:'Nice to have.'}]};return o},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/sessions`]:request=>{const input=request.postDataJSON();box.added.push(input);const o=outcomes[0];o.revision++;o.report.proposals=[];for(const item of input.sessions)o.sessions.push({key:'z',title:item.title,prompt:item.prompt,state:'waiting',note:'Waiting its turn.',updatedAt:now,stage:0,stages:item.stages.map(x=>st(x.kind,'claude','','pending'))});o.state='starting';return o},
 [`/api/v2/gaffer/outcomes/${'d'.repeat(32)}/sessions/b/retry`]:request=>{box.retries.push(request.postDataJSON());const piece=outcomes[1].sessions[1];Object.assign(piece,{state:'waiting',note:'Waiting its turn.'});outcomes[1].revision++;return outcomes[1]}})

const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})
try {
  for(const [width,theme] of [[1440,'light'],[390,'light'],[1440,'dark']]){
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[]}
    // Each run starts from the same record: the third pipeline is waiting for the merge decision.
    Object.assign(outcomes[0],{revision:5,state:'settled'});delete outcomes[0].report;outcomes[0].sessions.length=3;outcomes[1].revision=2;outcomes[1].sessions[1]={key:'b',title:'Importer docs',prompt:'p',state:'failed',note:'Not started: That agent isn’t installed on that machine.',updatedAt:now,stage:0,stages:[st('code','claude','','pending'),st('pr','claude','','pending')]};Object.assign(outcomes[0].sessions[2],{state:'ready_to_merge',mergeApproved:false,note:'Ready to merge. Nothing merges until you approve it.'});outcomes[0].sessions[2].stages[7].state='pending'
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
      await expect(api).toContainText('Claude Code · opus');await expect(api).toContainText('Reviews left9 of 12');await expect(api).toContainText(/Time left\d+ h \d+ min/);await expect(api).toContainText('Gaffer tokens2,252');await expect(api).toContainText('Waits for you')
      // What the agents themselves reported: tokens for the pipeline, and how much context the stage in hand has left.
      await expect(api).toContainText('Agent tokens97k · 410k cached');await expect(api).toContainText('Context left12% · 24k of 200k')
      await expect(pipes.nth(1)).toContainText('Agent tokensNot reported');await expect(pipes.nth(1)).toContainText('Context leftNot reported')
      await expect(api).toContainText('The review found one missing error case')
      // A pipeline waiting on the person says so and why.
      await expect(pipes.nth(1)).toContainText('Needs you');await expect(pipes.nth(1)).toContainText('Gaffer brought a question back to you.')
      // A finished pipeline that was allowed to merge says so.
      const done=page.getByRole('article',{name:/^Outcome: Fix the flaky importer test/})
      await expect(done).toContainText('Done');await expect(done).toContainText('By itself when green')
      // A pipeline's own goal is not listed again among the other sessions; a session started by hand is.
      const floor=page.locator('[data-floor]')
      await expect(floor.getByRole('list',{name:'Sessions Gaffer looks after',exact:true}).getByRole('listitem')).toHaveCount(1)
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
      // The outcome's own figures: what Gaffer's model has used so far, and the budget each stage gets. No prices.
      const totals=first.getByLabel('This outcome so far',{exact:true})
      await expect(totals).toContainText('Pipelines3');await expect(totals).toContainText('Gaffer reviews6');await expect(totals).toContainText('Gaffer tokens4,504');await expect(totals).toContainText('Budget per stage12 reviews · 24 h');await expect(totals).toContainText('ApprovalsAsk me');await expect(totals).toContainText('Agent tokens97k · 410k cached (Codex 85k, Claude Code 12k)')
      // A pipeline that never started says why and can be tried again, once, on request.
      const stopped=done.getByRole('list',{name:'Pipelines of this outcome',exact:true}).locator(':scope > li').nth(1)
      await expect(stopped).toContainText('Not started');await expect(stopped).toContainText('That agent isn’t installed on that machine.')
      await stopped.getByRole('button',{name:'Try again',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Trying'})).toHaveText('Trying “Importer docs” again.')
      expect(box.retries).toEqual([{revision:2}]);await expect(stopped.getByRole('button',{name:'Try again',exact:true})).toHaveCount(0)
      // Gaffer reports across the whole outcome when asked, and may propose pipelines; nothing is added until approved.
      await first.getByRole('button',{name:'Ask Gaffer for a report',exact:true}).click()
      const report=first.getByRole('region',{name:'Gaffer’s report on this outcome',exact:true})
      await expect(report).toContainText('Nothing covers the changelog.')
      const proposals=report.getByRole('list',{name:'Proposed pipelines',exact:true})
      await expect(proposals.getByRole('listitem')).toHaveCount(2)
      await expect(proposals.getByRole('listitem').first()).toContainText('Code → PR')
      expect(box.added).toEqual([])
      await proposals.getByRole('checkbox',{name:/^Export metrics/}).uncheck()
      await report.getByRole('button',{name:'Add 1 pipeline',exact:true}).click()
      const add=page.getByRole('dialog',{name:'Add “Changelog entry”?',exact:true})
      await expect(add).toContainText('never in YOLO');await expect(add).toContainText('stops at Ready to merge')
      // It starts on the outcome's own agent, and can be given another agent, model and effort for the job.
      const agent=add.getByRole('combobox',{name:'Agent for Changelog entry',exact:true})
      await expect(agent).toHaveValue('claude');await expect(agent.getByRole('option')).toHaveText(['Claude Code','Codex'])
      await agent.selectOption('codex')
      await add.getByRole('combobox',{name:'Model for Changelog entry',exact:true}).selectOption('gpt-5-codex')
      await add.getByRole('combobox',{name:'Effort for Changelog entry',exact:true}).selectOption('low')
      // A crew can do its stages: the shipping stage goes back to the outcome's own agent, which reports no effort here.
      await add.getByText('Choose who does what',{exact:true}).click()
      await expect(add.getByRole('combobox',{name:'Building agent for Changelog entry',exact:true})).toHaveValue('codex')
      await add.getByRole('combobox',{name:'Shipping agent for Changelog entry',exact:true}).selectOption('claude')
      await expect(add.getByRole('combobox',{name:'Shipping effort for Changelog entry',exact:true})).toBeDisabled()
      await expect(add.getByText('2 agent sessions for 2 stages.',{exact:false})).toBeVisible()
      expect(await add.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await add.screenshot({path:`${shots}/gaffer-factory-add-${width}-${theme}.png`})
      await add.getByRole('button',{name:'Add pipeline',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Added'})).toHaveText('Added “Changelog entry”. The controller is starting it.')
      expect(box.added).toEqual([{revision:7,confirmed:true,sessions:[{title:'Changelog entry',prompt:'Add a changelog entry for CSV export. Do not merge.',stages:[{kind:'code',tool:'codex',model:'gpt-5-codex',effort:'low'},{kind:'pr',tool:'claude',model:''}]}]}])
      await expect(pipes).toHaveCount(4);await expect(report.getByRole('list',{name:'Proposed pipelines',exact:true})).toHaveCount(0)
      await expect(totals).toContainText('Gaffer tokens4,884')
      expect(box.reviews).toEqual([{revision:6}])
      // The person runs the line: they can move a pipeline on from the stage in hand without waiting for Gaffer…
      const button=pipes.nth(1)
      await button.getByRole('button',{name:'Move to next stage',exact:true}).click()
      const move=page.getByRole('dialog',{name:/^Move “Export button” on to (review|test|pr)\?$/})
      await expect(move).toContainText('This is your call');await expect(move).toContainText('Nothing is merged by this.')
      expect(box.moved).toEqual([])
      await move.getByRole('button',{name:'Move on',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Moved'})).toHaveText('Moved “Export button” on. The controller is starting its next stage.')
      expect(box.moved).toEqual([{confirmed:true}])
      // …and decide, per pipeline and at any time, whether its merge waits for them.
      await pipes.nth(0).getByRole('button',{name:'Let Export API merge by itself',exact:true}).click()
      await expect(pipes.nth(0)).toContainText('By itself when green');expect(box.modes).toEqual([{on:true}])
      await pipes.nth(0).getByRole('button',{name:'Make Export API wait for you before merging',exact:true}).click()
      await expect(pipes.nth(0)).toContainText('Waits for you');expect(box.modes).toEqual([{on:true},{on:false}])
      // The card says who closed each stage, how long the stage in hand has run and what its agent last reported…
      await expect(pipes.nth(0).getByRole('listitem').filter({hasText:'Plan'})).toContainText('moved on by you, not handed over by Gaffer')
      await expect(pipes.nth(0).getByRole('listitem').filter({hasText:'Design'})).not.toContainText('moved on by you')
      await expect(pipes.nth(0)).toContainText('In this stage');await expect(pipes.nth(0)).toContainText('42 min')
      await expect(pipes.nth(0)).toContainText('Working · last active 20 min ago')
      // …and the outcome's history is read from the activity log only when asked for.
      await first.getByRole('button',{name:'History',exact:true}).click()
      const told=first.getByRole('region',{name:'History of this outcome',exact:true})
      await expect(told).toContainText("A pipeline's review stage started");await expect(told).toContainText("Gaffer handed over a pipeline's code stage");await expect(told).toContainText('Export API')
      await first.getByRole('button',{name:'Hide history',exact:true}).click();await expect(told).toHaveCount(0)
      // They can tell a pipeline's agent what to do next, in their own words…
      await pipes.nth(0).getByRole('button',{name:'Tell the agent…',exact:true}).click()
      const tell=page.getByRole('dialog',{name:'Tell the agent of “Export API”',exact:true})
      await expect(tell.getByRole('button',{name:'Send to the agent',exact:true})).toBeDisabled()
      await tell.getByRole('textbox',{name:'Instruction',exact:true}).fill('  Add the missing error case test.  ')
      await tell.getByRole('button',{name:'Send to the agent',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'instruction'})).toHaveText('Your instruction is with the agent of “Export API” for its next turn.')
      expect(box.told).toHaveLength(1);expect(box.told[0].text).toBe('Add the missing error case test.');expect(box.told[0].requestId).toMatch(/^[a-f0-9]{32}$/)
      // …and send it back to the stage in hand or an earlier one, with the reason.
      await pipes.nth(0).getByRole('button',{name:'Send back…',exact:true}).click()
      const back=page.getByRole('dialog',{name:'Send “Export API” back',exact:true})
      await expect(back).toContainText('Nothing already committed is undone.')
      const again=back.getByRole('combobox',{name:'Stage to do again',exact:true})
      await expect(again.getByRole('option')).toHaveText(['Plan','Design','Code','Review']);await expect(again).toHaveValue('review')
      await again.selectOption('code');await back.getByRole('textbox',{name:'Why it is going back',exact:true}).fill('The export leaks other tenants’ rows.')
      expect(await back.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await back.getByRole('button',{name:'Send back',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Sent'})).toHaveText('Sent “Export API” back to code. The controller is starting that stage again.')
      expect(box.back).toEqual([{stage:'code',reason:'The export leaks other tenants’ rows.',confirmed:true}])
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // A new installation is told what Gaffer still needs, each with the place to do it, and has no pipelines yet.
  {
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[]}
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
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[]}
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
  console.log('Factory page: a navigation tab, headline numbers, pipelines with every stage and who does it, a question waiting on the person, a finished pipeline, other sessions without the pipelines’ own goals, the merge decision asked for and sent once, usage and remaining budgets without prices, a stopped pipeline tried again, Gaffer’s report across an outcome with a proposal approved, setup guidance for a new installation and the account’s two Gaffer defaults passed on desktop, phone and dark.')
} finally { await browser.close() }
