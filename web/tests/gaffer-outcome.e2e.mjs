import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

// Gaffer owns an outcome: it proposes the sessions, the person edits and approves the plan, and only then are the
// sessions started through the ordinary launch and put in Gaffer's care. Nothing starts on the model's say-so.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-outcome'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),generalId='f'.repeat(32)
const pins={providerId:pid,providerRevision:4,model:'gaffer-model',contextRevision:0,workerRevision:0}
const provider={id:pid,revision:4,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
const machines=[
  {agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,yolo:true,acp:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'},{tool:'codex',adapter:'codex-acp',state:'missing'},{tool:'opencode',adapter:'opencode',state:'available'}]}},
  {agentId:'a2',host:'attic',ready:false,capabilities:{launch:true,acp:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'}]}},
]
const folder=(id,name,host,machine,isRepo=true)=>({id,host,machine,dir:`/work/${name}`,name,isRepo,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now})
const projects=[folder('p-billing','billing','a1','workshop'),folder('p-attic','attic-notes','a2','attic'),folder('p-notes','notes','a1','workshop',false)]
const objective='Ship CSV export for billing.'
const proposed={summary:'Two independent halves: the endpoint, then the button.',model:'gaffer-model',usageReported:true,inputTokens:40,outputTokens:60,sessions:[
  {title:'Export API',prompt:'Add the CSV export endpoint with tests. Raise a pull request. Do not merge.'},
  {title:'Export button',prompt:'Add the export button to the billing page. Raise a pull request. Do not merge.'},
]}
const cid=n=>String(n).repeat(32)

const oid='e'.repeat(32)
// `settles` is how the controller's record moves on each read; false keeps it starting until it is stopped.
function fixture({planFails=false,settles=true}={}){
  const box={writes:[],plans:[],approved:[],stops:[],reads:0,goals:[],record:null,planFails}
  const general={conversation:{id:generalId,sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
  const item=(n,title,prompt,stages=[])=>({key:'abcde'[n],title,prompt,requestId:String(n).repeat(32),state:'waiting',note:'Waiting its turn.',updatedAt:now,stage:0,stages:stages.map(stage=>({...stage,state:'pending',requestId:'r'}))})
  // What the controller does between reads: start one session, follow it, refuse the second, start the third unfollowed.
  const advance=()=>{
    const r=box.record
    if(!r||r.state!=='starting'||!settles)return
    box.reads++;r.revision++
    const [a,b,c]=r.sessions
    if(box.reads===1)Object.assign(a,{state:'starting',note:'Starting the session…'})
    if(box.reads===2){
      Object.assign(a,{state:'following',note:'Started and followed. Gaffer waits for its first progress.',sessionId:'outcome-session-1',conversationId:cid(1)})
      if(a.stages[0])a.stages[0].state='running'
      const goal={...pins,id:cid(1),conversationId:cid(1),sessionId:'outcome-session-1',revision:1,text:`Outcome: ${r.objective}\nThis session’s part: ${a.title}\nKeep this session moving. Do not merge.`,state:'active',note:'Waiting for new worker progress.',maxReviews:r.maxReviews,usedReviews:0,expiresAt:now+r.hours*3600000,createdAt:now,updatedAt:now,nextAt:now,lastSeq:0,summary:'',usageReported:false,inputTokens:0,outputTokens:0}
      box.goals.push({goal,session:{id:'outcome-session-1',agentId:'a1',title:a.title,state:'running'},available:true,attention:false})
      if(b)Object.assign(b,{state:'failed',note:'Not started: That agent isn’t installed on that machine.'})
    }
    if(box.reads>=3){
      if(c)Object.assign(c,{state:'unfollowed',note:'Started, but not followed: Its chat was not ready for follow-up in time. Look after it from the floor.',sessionId:'outcome-session-3'})
      Object.assign(r,{state:'settled',note:'Every session has its answer.'})
    }
  }
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/folder-projects':{projects,savedWaiting:0},
    '/api/v2/settings/yolo':{allowed:true,changedBy:'owner@example.test'},
    '/api/v2/profile/model-favourites':{version:1,favourites:[{tool:'claude',model:'starred-model',agentDefault:false}]},
    '/api/v2/machines/a1/models':request=>{const tool=new URL(request.url()).searchParams.get('tool');return {tool,state:'listed',models:[{id:`${tool}/on-the-machine`,provider:'workshop'}],more:false,source:'cli',version:'1',fetchedAt:now,checkedAt:now,reason:'',sentence:''}},
    '/api/v2/gaffer/goals':()=>({goals:box.goals,readAt:now}),
    '/api/v2/gaffer/conversations':request=>new URL(request.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
    '/api/v2/terminal/sessions':request=>{expect(request.method()).toBe('GET');return {sessions:(box.record?.sessions??[]).filter(s=>s.sessionId).map(s=>({id:s.sessionId,agentId:'a1',title:s.title,state:'running',structured:true,tool:'claude',dir:'/work/billing',createdAt:now,updatedAt:now,exitCode:null}))}},
    [`/api/v2/gaffer/providers/${pid}/plan`]:request=>{
      expect(request.method()).toBe('POST');box.plans.push(request.postDataJSON())
      return box.planFails?{status:502,json:{error:'This model did not return a plan. Choose a model that supports tools, or write the plan yourself.'}}:proposed
    },
    '/api/v2/gaffer/outcomes':request=>{
      if(request.method()!=='POST')return {outcomes:box.record?[box.record]:[]}
      const input=request.postDataJSON();box.approved.push(input)
      box.record={id:oid,revision:1,objective:input.objective,state:'starting',note:'',agentId:'a1',projectId:input.projectId,projectName:'billing',dir:'/work/billing',tool:input.tool,model:input.model,trust:input.trust,providerId:pid,maxReviews:input.maxReviews,hours:input.hours,autoMerge:input.autoMerge,sessions:input.sessions.map((s,n)=>({...item(n,s.title,s.prompt,s.stages),autoMerge:input.autoMerge})),createdAt:now,updatedAt:now}
      return {status:201,json:box.record}
    },
    [`/api/v2/gaffer/outcomes/${oid}`]:()=>{advance();return box.record},
    [`/api/v2/gaffer/outcomes/${oid}/stop`]:request=>{
      box.stops.push(request.postDataJSON());const r=box.record
      Object.assign(r,{state:'stopped',revision:r.revision+1,note:'You stopped this outcome. Sessions already started keep running.'})
      for(const s of r.sessions)if(s.state==='waiting'||s.state==='starting')Object.assign(s,{state:'cancelled',note:'Not started: this outcome was stopped first.'})
      return r
    },
  }
  return {box,routes}
}
// `direct` arrives the way the chat and Home send people: on the Factory page with the outcome dialog already up.
async function openOutcome(w,direct=false){
  if(direct){
    await w.page.goto(base+'/');await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
    const give=w.page.getByRole('dialog',{name:'Gaffer',exact:true}).getByRole('link',{name:'Give Gaffer an outcome',exact:true})
    await expect(give).toHaveAttribute('href','/factory?outcome=1');await give.click()
  } else {
    await w.page.goto(base+'/factory')
    await w.page.getByRole('button',{name:'Give Gaffer an outcome',exact:true}).click()
  }
  await expect(w.page.getByRole('heading',{name:'Factory',exact:true,level:1})).toBeVisible()
  return {floor:w.page.locator('[data-floor]'),outcome:w.page.getByRole('dialog',{name:'Give Gaffer an outcome',exact:true})}
}
const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})

try {
  for(const [width,theme] of [[1440,'light'],[390,'dark']]){
    const f=fixture(),w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {floor,outcome}=await openOutcome(w,width===1440)
      const ask=outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}),own=outcome.getByRole('button',{name:'Write the plan myself',exact:true})
      // Nothing can be asked until there is an outcome, a connected project and an agent that can chat.
      await expect(ask).toBeDisabled();await expect(own).toBeDisabled()
      // What is still needed reads as guidance, not as an error, until something has actually gone wrong.
      const need=outcome.getByText('Write the outcome you want.',{exact:true})
      await expect(need).toBeVisible();await expect(need).toHaveClass(/hint/)
      const steps=outcome.getByRole('list',{name:'Steps',exact:true})
      await expect(steps.locator('[aria-current="step"]')).toHaveText('1Outcome')
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await expect(outcome.getByText('Choose the project the sessions work in.',{exact:true})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-attic')
      await expect(outcome.getByText('That project’s machine is not connected.',{exact:true})).toBeVisible();await expect(ask).toBeDisabled()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-notes')
      await expect(outcome.getByText('not a git repository',{exact:false})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      // Only an agent whose chat adapter is installed on that machine is offered.
      await expect(outcome.getByRole('combobox',{name:'Agent',exact:true}).getByRole('option')).toHaveText(['Claude Code','OpenCode'])
      await expect(outcome.getByRole('combobox',{name:'Sessions',exact:true})).toHaveValue('3')
      // Right tool for the job: the thinking stages get a stronger model than the rest.
      await outcome.getByText('Choose who does what',{exact:true}).click()
      await expect(outcome.getByRole('combobox',{name:'Building agent',exact:true})).toHaveValue('claude')
      // Models are chosen from what the machine really offers, the person's favourites and reet's suggestions.
      const thinking=outcome.getByRole('combobox',{name:'Thinking model',exact:true})
      await expect(thinking.getByRole('option')).toHaveText(['Agent’s default model','starred-model (not in workshop’s list)','claude/on-the-machine','sonnet (not in workshop’s list)','opus (not in workshop’s list)','haiku (not in workshop’s list)','Other…'])
      await expect(outcome.getByRole('combobox',{name:'Model',exact:true}).getByRole('option',{name:'claude/on-the-machine',exact:true})).toBeAttached()
      // A name none of them has can still be typed.
      await thinking.selectOption({label:'Other…'});await outcome.getByRole('textbox',{name:'Thinking model name',exact:true}).fill('private-model')
      await thinking.selectOption('opus');await expect(outcome.getByRole('textbox',{name:'Thinking model name',exact:true})).toHaveCount(0)
      await expect(outcome.getByText('sent only what you type here and the project’s name',{exact:false})).toBeVisible()
      expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await outcome.screenshot({path:`${shots}/gaffer-outcome-objective-${width}-${theme}.png`})
      expect(f.box.writes).toEqual([])
      await ask.click()
      // The plan is the typed objective and the project's name, nothing else.
      const plan=outcome.getByRole('list',{name:'Planned sessions',exact:true})
      await expect(plan.getByRole('listitem')).toHaveCount(2)
      await expect(steps.locator('[aria-current="step"]')).toHaveText('2Plan')
      expect(f.box.plans).toEqual([{revision:4,objective,project:'billing',sessions:3}])
      await expect(outcome.getByText('Two independent halves: the endpoint, then the button.',{exact:true})).toBeVisible()
      await expect(outcome.getByRole('textbox',{name:'Session 1 title',exact:true})).toHaveValue('Export API')
      await expect(outcome.getByRole('textbox',{name:'Session 2 prompt',exact:true})).toHaveValue(/^Add the export button/)
      await expect(outcome.getByText('with ordinary approvals (no YOLO)',{exact:false})).toBeVisible()
      // Every part of the plan is editable, and a session the person adds is as good as one Gaffer proposed.
      await outcome.getByRole('textbox',{name:'Session 2 title',exact:true}).fill('Export button, renamed')
      // Each pipeline's stages are the person's to choose: this one gets a plan first and never merges.
      const picks2=outcome.getByRole('group',{name:'Session 2 stages',exact:true})
      await expect(picks2.getByRole('button',{pressed:true})).toHaveText(['Code','Review','Test','PR','Babysit','Merge'])
      await picks2.getByRole('button',{name:'Plan',exact:true}).click();await picks2.getByRole('button',{name:'Merge',exact:true}).click()
      await expect(picks2.getByRole('button',{pressed:true})).toHaveText(['Plan','Code','Review','Test','PR','Babysit'])
      // Thinking uses a different model from the rest, so the plan and the review each need a session of their own.
      await expect(outcome.getByText('4 agent sessions for 6 stages.',{exact:true})).toBeVisible()
      await outcome.getByRole('button',{name:'Add a session',exact:true}).click()
      const approve=outcome.getByRole('button',{name:/^Approve and start/})
      await expect(approve).toBeDisabled();await expect(outcome.getByText('Give session 3 a title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Session 3 title',exact:true}).fill('Export API')
      await outcome.getByRole('textbox',{name:'Session 3 prompt',exact:true}).fill('Document the export in the README. Raise a pull request. Do not merge.')
      await expect(outcome.getByText('Two sessions are both called “Export API”. Give each its own title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Session 3 title',exact:true}).fill('Export docs')
      // The person may add up to the five an outcome can start, whatever size they asked Gaffer to plan.
      await expect(outcome.getByRole('button',{name:'Add a session',exact:true})).toBeVisible()
      // A prompt the launch would refuse, and a limit that is not a whole number, are caught before anything starts.
      await outcome.getByRole('textbox',{name:'Session 3 prompt',exact:true}).fill('- Document the export')
      await expect(outcome.getByText('Start the prompt for “Export docs” with a word, not a dash.',{exact:true})).toBeVisible();await expect(approve).toBeDisabled()
      await outcome.getByRole('textbox',{name:'Session 3 prompt',exact:true}).fill('Document the export in the README.\tRaise a pull request. Do not merge.')
      await outcome.getByText('Change limits',{exact:true}).click()
      await outcome.getByRole('spinbutton',{name:'Review budget',exact:true}).fill('1.5')
      await expect(outcome.getByText('Choose a whole number of reviews from 1 to 48 and hours from 1 to 168.',{exact:true})).toBeVisible();await expect(approve).toBeDisabled()
      await outcome.getByRole('spinbutton',{name:'Review budget',exact:true}).fill('8')
      // Claude Code asks for no plugin trust, so none is offered.
      await expect(outcome.getByRole('checkbox',{name:/^I trust this folder/})).toHaveCount(0)
      await expect(outcome.getByRole('checkbox',{name:/^Merge by itself once every check passes/})).not.toBeChecked()
      await expect(outcome.getByText('whether or not this page stays open',{exact:false})).toBeVisible()
      expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await outcome.screenshot({path:`${shots}/gaffer-outcome-plan-${width}-${theme}.png`})
      // Until Approve, the only request made is the plan itself: no outcome, session, grant or goal.
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`])
      await expect(approve).toHaveText('Approve and start 3 sessions');await approve.click()
      // The browser hands the approved plan to the controller once, and from then on only reads its progress.
      await expect(outcome.getByRole('status')).toHaveText(/^Starting session \d of 3\. The controller carries on if you close this\.$/)
      await expect(outcome.getByRole('button',{name:'Close and let it run',exact:true})).toBeVisible()
      await expect(outcome.getByRole('status')).toHaveText('1 started and followed, 1 started but not followed yet, 1 not started.',{timeout:20000})
      const results=outcome.getByRole('list',{name:'Outcome sessions',exact:true}).getByRole('listitem')
      await expect(results).toHaveCount(3)
      await expect(results.nth(0)).toHaveText(/^Export API\s*Started and followed\. Gaffer waits for its first progress\.\s*Open session$/)
      await expect(results.nth(0).getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=outcome-session-1&gaffer=1')
      await expect(results.nth(1)).toContainText('Not started: That agent isn’t installed on that machine.')
      await expect(results.nth(1).getByRole('link')).toHaveCount(0)
      await expect(results.nth(2)).toContainText('Started, but not followed: Its chat was not ready')
      const cheap=kind=>({kind,tool:'claude',model:''}),strong=kind=>({kind,tool:'claude',model:'opus'})
      const usual=[cheap('code'),strong('review'),cheap('test'),cheap('pr'),cheap('babysit'),cheap('merge')]
      expect(f.box.approved).toEqual([{objective,projectId:'p-billing',tool:'claude',model:'',trust:false,yolo:false,autoMerge:false,maxReviews:8,hours:24,confirmed:true,sessions:[
        {title:'Export API',prompt:proposed.sessions[0].prompt,stages:usual},
        {title:'Export button, renamed',prompt:proposed.sessions[1].prompt,stages:[strong('plan'),...usual.filter(stage=>stage.kind!=='merge')]},
        {title:'Export docs',prompt:'Document the export in the README.    Raise a pull request. Do not merge.',stages:usual},
      ]}])
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`,'POST /api/v2/gaffer/outcomes'])
      await outcome.screenshot({path:`${shots}/gaffer-outcome-results-${width}-${theme}.png`})
      await outcome.getByRole('button',{name:'Back to the Factory',exact:true}).click();await expect(outcome).toHaveCount(0)
      // The page draws the controller's record as production lines: each pipeline's stages, who does the one in hand, and what was not started.
      const article=w.page.getByRole('article',{name:'Outcome: Ship CSV export for billing.',exact:true})
      const pipes=article.getByRole('list',{name:'Pipelines of this outcome',exact:true}).locator(':scope > li')
      await expect(pipes).toHaveCount(3)
      await expect(pipes.nth(0)).toContainText('Running')
      const stages=pipes.nth(0).getByRole('list',{name:'Stages of Export API',exact:true}).getByRole('listitem')
      await expect(stages).toHaveText([/Code/,/Review/,/Test/,/PR/,/Babysit/,/Merge/])
      await expect(stages.first()).toHaveAttribute('aria-current','step')
      await expect(pipes.nth(0)).toContainText('Waits for you')
      await expect(pipes.nth(0).getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=outcome-session-1&gaffer=1')
      await expect(pipes.nth(1)).toContainText('Not started');await expect(pipes.nth(1).getByRole('link')).toHaveCount(0)
      await expect(pipes.nth(1).getByRole('list',{name:'Stages of Export button, renamed',exact:true}).getByRole('listitem')).toHaveCount(6)
      await expect(pipes.nth(2)).toContainText('Stopped')
      await expect(w.page.getByRole('list',{name:'Factory at a glance',exact:true})).toContainText('1Running')
      // A session that was started but is not followed is still offered below, to be looked after.
      const picks=floor.getByRole('group',{name:'Running chats not followed',exact:true})
      await expect(picks.getByRole('checkbox')).toHaveCount(1);await expect(picks).toContainText('Export docs')
      // The followed pipeline's goal is drawn in its pipeline, not listed again among the other sessions.
      await expect(floor.getByRole('list',{name:'Sessions Gaffer looks after',exact:true})).toHaveCount(0)
      expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      await w.page.screenshot({path:`${shots}/gaffer-outcome-floor-${width}-${theme}.png`,fullPage:true})
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // Closing the dialog leaves the controller starting; the floor shows it and can stop what has not started.
  {
    const f=fixture({settles:false}),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      // OpenCode runs a folder's plugins, so the person's own trust is asked for and sent; it is off until ticked.
      await outcome.getByRole('combobox',{name:'Agent',exact:true}).selectOption('opencode')
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      const trust=outcome.getByRole('checkbox',{name:/^I trust this folder: OpenCode may run the plugins/})
      await expect(trust).not.toBeChecked();await trust.check()
      // Merging by itself is this outcome's own choice, off until ticked.
      const auto=outcome.getByRole('checkbox',{name:/^Merge by itself once every check passes/})
      await expect(auto).not.toBeChecked();await auto.check()
      // YOLO is off until confirmed, and is this outcome's choice only.
      const approvals=outcome.getByRole('radiogroup',{name:'Approvals',exact:true})
      await expect(approvals.getByRole('radio',{name:'Ask me',exact:true})).toBeChecked()
      await approvals.getByRole('radio',{name:'YOLO',exact:true}).click()
      const confirm=w.page.getByRole('alertdialog',{name:'Turn on YOLO for this launch?',exact:true})
      await expect(confirm).toContainText('Your account allows YOLO');await expect(confirm).toContainText('workshop accepts YOLO sessions');await expect(confirm).toContainText('Every agent in the crew has a YOLO option')
      await confirm.getByRole('button',{name:'Turn on YOLO',exact:true}).click()
      await expect(approvals.getByRole('radio',{name:'YOLO',exact:true})).toBeChecked()
      await expect(outcome.getByText('On YOLO the agents do not wait for you',{exact:false})).toBeVisible()
      await outcome.screenshot({path:`${shots}/gaffer-outcome-yolo-1280-light.png`})
      await outcome.getByRole('button',{name:'Approve and start 2 sessions',exact:true}).click()
      expect(f.box.approved[0]).toMatchObject({tool:'opencode',trust:true,yolo:true,model:'',autoMerge:true})
      expect(f.box.approved[0].sessions[0].stages.every(stage=>stage.tool==='opencode')).toBe(true)
      await expect(outcome.getByRole('status')).toHaveText('Starting session 1 of 2. The controller carries on if you close this.')
      await outcome.getByRole('button',{name:'Close and let it run',exact:true}).click();await expect(outcome).toHaveCount(0)
      const article=w.page.getByRole('article',{name:'Outcome: Ship CSV export for billing.',exact:true})
      await expect(article).toContainText('the controller is starting sessions')
      await article.getByRole('button',{name:'Stop starting sessions',exact:true}).click()
      await expect(w.page.getByText('Stopped. Sessions not yet started stay unstarted; the ones already started keep running.',{exact:true})).toBeVisible()
      await expect(article.getByRole('list',{name:'Pipelines of this outcome',exact:true}).locator(':scope > li')).toContainText(['Not started','Not started'])
      await expect(article.getByRole('button',{name:'Stop starting sessions',exact:true})).toHaveCount(0)
      expect(f.box.stops).toEqual([{revision:1}])
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`,'POST /api/v2/gaffer/outcomes',`POST /api/v2/gaffer/outcomes/${oid}/stop`]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A model that cannot plan says so, and the person can write the plan; the rest of the flow is the same.
  {
    const f=fixture({planFails:true}),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      await expect(outcome.getByRole('alert')).toHaveText('This model did not return a plan. Choose a model that supports tools, or write the plan yourself.')
      await outcome.getByRole('button',{name:'Write the plan myself',exact:true}).click()
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true}).getByRole('listitem')).toHaveCount(1)
      await expect(outcome.getByRole('button',{name:/^Approve and start/})).toBeDisabled()
      await outcome.getByRole('textbox',{name:'Session 1 title',exact:true}).fill('Whole export')
      await outcome.getByRole('textbox',{name:'Session 1 prompt',exact:true}).fill('Build the CSV export end to end. Raise a pull request. Do not merge.')
      await expect(outcome.getByRole('button',{name:'Approve and start 1 session',exact:true})).toBeEnabled()
      // Back keeps the objective; closing starts nothing.
      await outcome.getByRole('button',{name:'Back',exact:true}).click()
      await expect(outcome.getByRole('textbox',{name:'Outcome',exact:true})).toHaveValue(objective)
      await w.page.keyboard.press('Escape');await expect(outcome).toHaveCount(0)
      await expect(w.page.getByRole('button',{name:'Give Gaffer an outcome',exact:true})).toBeFocused()
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`]);expect(f.box.approved).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Gaffer outcome on the Factory page: objective gates, the crew and stages per pipeline, auto-merge as the outcome’s own choice, plan from the typed objective only, editable plan, refused prompts and limits caught first, nothing saved before approval, one approved plan handed to the controller, progress read from its record, partial results, the record on the floor, closing while it starts, stopping unstarted sessions, a model that cannot plan and a hand-written plan passed on desktop and phone.')
} finally { await browser.close() }
