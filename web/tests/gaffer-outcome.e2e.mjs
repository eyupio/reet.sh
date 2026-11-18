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
  {agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,yolo:true,acp:true,acpStartOptions:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:[{id:'effort',category:'thought_level',values:['default','low','medium','high','max']}]},{tool:'codex',adapter:'codex-acp',state:'missing'},{tool:'opencode',adapter:'opencode',state:'available'}]}},
  {agentId:'a2',host:'attic',ready:false,capabilities:{launch:true,acp:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'}]}},
]
const folder=(id,name,host,machine,isRepo=true)=>({id,host,machine,dir:`/work/${name}`,name,isRepo,baseBranch:'',worktreeDefault:'',colour:'',createdAt:now,updatedAt:now})
const projects=[folder('p-billing','billing','a1','workshop'),folder('p-attic','attic-notes','a2','attic'),folder('p-notes','notes','a1','workshop',false)]
const objective='Ship CSV export for billing.'
const proposed={summary:'Two independent halves: the endpoint, then the button.',model:'gaffer-model',usageReported:true,inputTokens:40,outputTokens:60,sessions:[
  // Gaffer's assessment: this code stage is harder than code usually is, and it says why. The rest take their defaults.
  {title:'Export API',prompt:'Add the CSV export endpoint with tests. Raise a pull request. Do not merge.',stages:[{kind:'code',tier:'heavy',tierWhy:'The export reads the billing ledger while it is being written.'},{kind:'review'},{kind:'test'},{kind:'pr'},{kind:'babysit'},{kind:'merge'}]},
  // Gaffer split this pipeline's code stage into two parts of different work, the first of them light.
  {title:'Export button',prompt:'Add the export button to the billing page. Raise a pull request. Do not merge.',stages:[{kind:'code',part:'Button',brief:'Add the button and wire it to the endpoint.',tier:'light',tierWhy:'Wiring only.'},{kind:'code',part:'Empty state',brief:'Draw the empty state.'},{kind:'review'},{kind:'test'},{kind:'pr'},{kind:'babysit'},{kind:'merge'}]},
]}
const revised={summary:'One session, end to end: the endpoint, its tests and the button together.',model:'gaffer-model',usageReported:true,inputTokens:25,outputTokens:35,sessions:[
  {title:'Export, end to end',prompt:'Build the CSV export API and its button with tests. Raise a pull request. Do not merge.',stages:['plan','code','review','test','pr','babysit'].map(kind=>({kind}))},
]}
const cid=n=>String(n).repeat(32)
const PNG=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z0GQAAAAASUVORK5CYII=','base64')

const oid='e'.repeat(32)
// `settles` is how the controller's record moves on each read; false keeps it starting until it is stopped.
function fixture({planFails=false,settles=true,crew=null,crewFails=false,crewEmpty=false}={}){
  const box={writes:[],plans:[],replies:[],approved:[],stops:[],reads:0,goals:[],record:null,planFails,uploads:0,uploadFails:false,crew,crewFails,crewEmpty,crewSaves:[]}
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
    // The account's crew: read when the dialog opens, saved only when the person asks.
    '/api/v2/gaffer/crew':request=>{if(box.crewFails&&request.method()==='GET')return{status:500,json:{error:'The crew could not be read.'}};if(request.method()==='PUT'){box.crew=request.postDataJSON();box.crewSaves.push(box.crew)}return {crew:box.crew??{light:{},standard:{},heavy:{}},saved:!!box.crew||box.crewEmpty,tiers:['light','standard','heavy'],defaults:{}}},
    '/api/v2/folder-projects':{projects,savedWaiting:0},
    '/api/v2/settings/yolo':{allowed:true,changedBy:'owner@example.test'},
    '/api/v2/machines/a1/uploads':request=>{expect(request.method()).toBe('POST');box.uploads++;if(box.uploadFails)return{status:502,json:{error:'The image could not be uploaded.'}};return{status:200,headers:{'X-Reet-Upload-Path':'/home/dev/launch-uploads/outcome.png','X-Reet-Upload-Reference':'/home/dev/launch-uploads/outcome.png'},json:{uploaded:'true'}}},
    '/api/v2/profile/model-favourites':{version:1,favourites:[{tool:'claude',model:'starred-model',agentDefault:false}]},
    '/api/v2/machines/a1/models':request=>{const tool=new URL(request.url()).searchParams.get('tool');return {tool,state:'listed',models:[{id:`${tool}/on-the-machine`,provider:'workshop'}],more:false,source:'cli',version:'1',fetchedAt:now,checkedAt:now,reason:'',sentence:''}},
    '/api/v2/gaffer/goals':()=>({goals:box.goals,readAt:now}),
    '/api/v2/gaffer/conversations':request=>new URL(request.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
    '/api/v2/terminal/sessions':request=>{expect(request.method()).toBe('GET');return {sessions:(box.record?.sessions??[]).filter(s=>s.sessionId).map(s=>({id:s.sessionId,agentId:'a1',title:s.title,state:'running',structured:true,tool:'claude',dir:'/work/billing',createdAt:now,updatedAt:now,exitCode:null}))}},
    [`/api/v2/gaffer/providers/${pid}/plan`]:request=>{
      expect(request.method()).toBe('POST');const input=request.postDataJSON();box.plans.push(input)
      if(input.reply)box.replies.push(input)
      return box.planFails?{status:502,json:{error:'This model did not return a plan. Choose a model that supports tools, or write the plan yourself.'}}:input.reply?revised:proposed
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
const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET'&&new URL(r.url()).pathname!=='/api/v2/gaffer/crew')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})

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
      // Gaffer settings has no crew yet: the dialog says so, unobtrusively, instead of showing blank agent and model fields.
      const savedSettings=outcome.getByRole('region',{name:'Your Gaffer settings',exact:true})
      await expect(savedSettings).toContainText('No crew is saved in Gaffer settings yet.')
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await expect(outcome.getByText('Choose the project the sessions work in.',{exact:true})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-attic')
      await expect(outcome.getByText('That project’s machine is not connected.',{exact:true})).toBeVisible();await expect(ask).toBeDisabled()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-notes')
      await expect(outcome.getByText('not a git repository',{exact:false})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      // A level this machine reports can be chosen; another project's machine that reports none drops it back to the
      // agent's default, so what is shown is what the outcome would start with.
      await outcome.getByRole('combobox',{name:'Effort',exact:true}).selectOption('max')
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-attic')
      await expect(outcome.getByRole('combobox',{name:'Effort',exact:true})).toHaveValue('')
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      await expect(outcome.getByRole('combobox',{name:'Effort',exact:true})).toHaveValue('')
      // Only an agent whose chat adapter is installed on that machine is offered.
      await expect(outcome.getByRole('combobox',{name:'Agent',exact:true}).getByRole('option')).toHaveText(['Claude Code','OpenCode'])
      await expect(outcome.getByRole('combobox',{name:'Sessions',exact:true})).toHaveValue('3')
      // Right tool for the job: the heavy tier gets a stronger model than the rest.
      await outcome.getByText('Choose who does what',{exact:true}).click()
      await expect(outcome.getByRole('combobox',{name:'Standard agent',exact:true})).toHaveValue('claude')
      // Models are chosen from what the machine really offers, the person's favourites and reet's suggestions.
      const thinking=outcome.getByRole('combobox',{name:'Heavy model',exact:true})
      await expect(thinking.getByRole('option')).toHaveText(['Agent’s default model','starred-model (not in workshop’s list)','claude/on-the-machine','sonnet (not in workshop’s list)','opus (not in workshop’s list)','haiku (not in workshop’s list)','Other…'])
      await expect(outcome.getByRole('combobox',{name:'Model',exact:true}).getByRole('option',{name:'claude/on-the-machine',exact:true})).toBeAttached()
      // A name none of them has can still be typed.
      await thinking.selectOption({label:'Other…'});await outcome.getByRole('textbox',{name:'Heavy model name',exact:true}).fill('private-model')
      await thinking.selectOption('opus');await expect(outcome.getByRole('textbox',{name:'Heavy model name',exact:true})).toHaveCount(0)
      // Effort is per tier too: heavy stages can think harder than light ones.
      await expect(outcome.getByRole('combobox',{name:'Heavy effort',exact:true}).getByRole('option')).toHaveText(['Default effort','Low effort','Medium effort','High effort','Max effort'])
      await outcome.getByRole('combobox',{name:'Heavy effort',exact:true}).selectOption('high')
      await outcome.getByRole('combobox',{name:'Light effort',exact:true}).selectOption('low')
      // The crew can be kept for the account: every later outcome starts from it. Nothing else is saved by it.
      await outcome.getByRole('button',{name:'Save as my crew',exact:true}).click()
      await expect(outcome.getByText('Saved. New outcomes start from this crew.',{exact:true})).toBeVisible()
      expect(f.box.crewSaves).toEqual([{light:{tool:'claude',model:'',effort:'low'},standard:{tool:'claude',model:'',effort:''},heavy:{tool:'claude',model:'opus',effort:'high'},balanceBelow:20,gafferDecides:false}])
      // A tier can have a backup: who does its stages when the first choice's plan is running low. None is set until chosen.
      const spare=outcome.getByRole('group',{name:'Heavy backup',exact:true})
      await expect(spare.getByRole('combobox',{name:'Heavy backup agent',exact:true})).toHaveValue('')
      await expect(spare.getByRole('combobox',{name:'Heavy backup model',exact:true})).toHaveCount(0)
      await spare.getByRole('combobox',{name:'Heavy backup agent',exact:true}).selectOption('claude')
      await spare.getByRole('combobox',{name:'Heavy backup model',exact:true}).selectOption('sonnet')
      await expect(outcome).toContainText('under 20% left in its tightest window and the backup’s has more')
      // When a backup takes over is the person's to set, within bounds: a figure out of range is brought back in.
      const below=outcome.getByRole('group',{name:'When a backup takes over',exact:true}).getByRole('spinbutton')
      await expect(below).toHaveValue('20')
      await below.fill('400');await below.blur();await expect(below).toHaveValue('90')
      await below.fill('35');await below.blur();await expect(outcome).toContainText('under 35% left in its tightest window')
      expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await outcome.locator('details.crew').screenshot({path:`${shots}/gaffer-outcome-backup-${width}-${theme}.png`})
      // A tier can have further backups, tried in order: up to three in all. One is added and then removed again.
      await spare.getByRole('button',{name:'Add another backup for Heavy',exact:true}).click()
      // A further backup starts on another agent than the one before it.
      await expect(spare.getByRole('combobox',{name:'Heavy backup 2 agent',exact:true})).not.toHaveValue('')
      await spare.getByRole('button',{name:'Add another backup for Heavy',exact:true}).click()
      await expect(spare.getByRole('button',{name:'Add another backup for Heavy',exact:true})).toHaveCount(0)
      await spare.getByRole('combobox',{name:'Heavy backup 3 agent',exact:true}).selectOption('')
      await spare.getByRole('combobox',{name:'Heavy backup 2 agent',exact:true}).selectOption('')
      await expect(spare.getByRole('combobox',{name:/^Heavy backup \d agent$/})).toHaveCount(0)
      // Saved with the crew, the backup comes back for the next outcome.
      await outcome.getByRole('button',{name:'Save as my crew',exact:true}).click()
      await expect.poll(()=>f.box.crewSaves.length).toBe(2)
      expect(f.box.crewSaves[1].heavy).toEqual({tool:'claude',model:'opus',effort:'high',backup:{tool:'claude',model:'sonnet',effort:''}});expect(f.box.crewSaves[1].balanceBelow).toBe(35)
      await expect(outcome.getByRole('combobox',{name:'Effort',exact:true})).toHaveValue('')
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
      await expect(outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('Export API')
      await expect(outcome.getByRole('textbox',{name:'Pipeline 2 prompt',exact:true})).toHaveValue(/^Add the export button/)
      await expect(outcome.getByText('with ordinary approvals (no YOLO)',{exact:false})).toBeVisible()
      // Gaffer advises how demanding each stage is. Where its assessment differs from the default for that kind of
      // stage the plan arrives with it applied and marked, with the reason, and one click puts it back.
      const tiers1=outcome.getByRole('group',{name:'Pipeline 1 stage tiers',exact:true}),code1=tiers1.locator('[data-stage=code]')
      await expect(tiers1.getByRole('combobox')).toHaveCount(6)
      await expect(code1.getByRole('combobox',{name:'Code tier in pipeline 1',exact:true})).toHaveValue('heavy')
      await expect(code1).toHaveClass(/changed/)
      await expect(code1).toContainText('Gaffer’s assessment. The export reads the billing ledger while it is being written.')
      await expect(code1).toContainText('Claude Code · opus · high effort')
      await expect(code1.getByRole('button',{name:'Put Code in pipeline 1 back on Standard',exact:true})).toBeVisible()
      await tiers1.scrollIntoViewIfNeeded();await tiers1.screenshot({path:`${shots}/gaffer-outcome-tiers-${width}-${theme}.png`})
      // A stage Gaffer did not rate is on its default and says nothing more.
      await expect(tiers1.locator('[data-stage=review]')).not.toHaveClass(/changed/)
      await expect(tiers1.getByRole('combobox',{name:'Review tier in pipeline 1',exact:true})).toHaveValue('heavy')
      await expect(tiers1.getByRole('combobox',{name:'Merge tier in pipeline 1',exact:true})).toHaveValue('light')
      // The person's own choice is marked as theirs, and replaces Gaffer's if it was Gaffer's.
      const tiers2=outcome.getByRole('group',{name:'Pipeline 2 stage tiers',exact:true}),test2=tiers2.locator('[data-stage=test]')
      await tiers2.getByRole('combobox',{name:'Test tier in pipeline 2',exact:true}).selectOption('light')
      await expect(test2).toHaveClass(/changed/);await expect(test2).toContainText('Your choice. The default for test is Standard.')
      // A stage Gaffer split is shown as its parts, in the order they run: each a name, a brief and a tier, with the
      // agent that tier means, Gaffer's reason, and a way to do the stage as one after all. Each is the person's to edit.
      const split=tiers2.getByRole('group',{name:'Code parts in pipeline 2',exact:true})
      await expect(split).toContainText('Split into 2 parts, done in this order, each by its own agent.')
      const part1=split.getByRole('group',{name:'Part 1 of Code in pipeline 2',exact:true}),part2=split.getByRole('group',{name:'Part 2 of Code in pipeline 2',exact:true})
      await expect(part1.getByRole('textbox',{name:'Part 1 name',exact:true})).toHaveValue('Button');await expect(part1.getByRole('combobox',{name:'Part 1 tier',exact:true})).toHaveValue('light')
      await expect(part1).toContainText('Claude Code · low effort');await expect(part1).toContainText('Gaffer’s assessment. Wiring only.')
      await expect(part2.getByRole('combobox',{name:'Part 2 tier',exact:true})).toHaveValue('standard')
      await part2.getByRole('textbox',{name:'Part 2 brief',exact:true}).fill('Draw the empty state, with the illustration.')
      await expect(split.getByRole('button',{name:'Do Code in pipeline 2 as one stage',exact:true})).toBeVisible()
      // A stage can also be split by hand: it starts as two parts on the stage's own tier, and a part can be added.
      // Parts need a name and a brief before the plan can be approved; doing the stage as one again clears that.
      await tiers2.getByRole('button',{name:'Split Review in pipeline 2 into parts',exact:true}).click()
      const mine=tiers2.getByRole('group',{name:'Review parts in pipeline 2',exact:true})
      await expect(mine.getByRole('group',{name:/^Part \d of Review in pipeline 2$/})).toHaveCount(2)
      await expect(mine.getByRole('combobox',{name:'Part 1 tier',exact:true})).toHaveValue('heavy')
      await mine.getByRole('button',{name:'Add a part to Review in pipeline 2',exact:true}).click()
      await expect(mine.getByRole('group',{name:/^Part \d of Review in pipeline 2$/})).toHaveCount(3)
      await expect(outcome.getByRole('status').filter({hasText:'Give every part'})).toHaveText('Give every part of the review stage of “Export button” a name and a brief saying what it does.')
      await expect(outcome.getByRole('button',{name:/^Approve and start/})).toBeDisabled()
      await mine.getByRole('button',{name:'Do Review in pipeline 2 as one stage',exact:true}).click()
      await expect(mine).toHaveCount(0);await expect(outcome.getByRole('status').filter({hasText:'Give every part'})).toHaveCount(0)
      // The pull request, babysitting and merge are never split.
      await expect(tiers2.getByRole('button',{name:/^Split (PR|Babysit|Merge) in pipeline 2/})).toHaveCount(0)
      await expect(outcome.getByText('6 stages, 1 split into parts: 7 agent sessions in all, one after another.',{exact:true})).toBeVisible()
      await split.scrollIntoViewIfNeeded();expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await split.screenshot({path:`${shots}/gaffer-outcome-parts-${width}-${theme}.png`})
      // Every part of the plan is editable, and a session the person adds is as good as one Gaffer proposed.
      await outcome.getByRole('textbox',{name:'Pipeline 2 title',exact:true}).fill('Export button, renamed')
      // Each pipeline's stages are the person's to choose: this one gets a plan first and never merges.
      const picks2=outcome.getByRole('group',{name:'Pipeline 2 stages',exact:true})
      await expect(picks2.getByRole('button',{pressed:true})).toHaveText(['Code','Review','Test','PR','Babysit','Merge'])
      await picks2.getByRole('button',{name:'Plan',exact:true}).click();await picks2.getByRole('button',{name:'Merge',exact:true}).click()
      await expect(picks2.getByRole('button',{pressed:true})).toHaveText(['Plan','Code','Review','Test','PR','Babysit'])
      await expect(outcome.getByText('6 stages, each in an agent session of its own.',{exact:true}).first()).toBeVisible()
      await outcome.getByRole('button',{name:'Add a pipeline',exact:true}).click()
      const approve=outcome.getByRole('button',{name:/^Approve and start/})
      await expect(approve).toBeDisabled();await expect(outcome.getByText('Give session 3 a title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Pipeline 3 title',exact:true}).fill('Export API')
      await outcome.getByRole('textbox',{name:'Pipeline 3 prompt',exact:true}).fill('Document the export in the README. Raise a pull request. Do not merge.')
      await expect(outcome.getByText('Two sessions are both called “Export API”. Give each its own title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Pipeline 3 title',exact:true}).fill('Export docs')
      // The person may add up to the five an outcome can start, whatever size they asked Gaffer to plan.
      await expect(outcome.getByRole('button',{name:'Add a pipeline',exact:true})).toBeVisible()
      // A prompt the launch would refuse, and a limit that is not a whole number, are caught before anything starts.
      await outcome.getByRole('textbox',{name:'Pipeline 3 prompt',exact:true}).fill('- Document the export')
      await expect(outcome.getByText('Start the prompt for “Export docs” with a word, not a dash.',{exact:true})).toBeVisible();await expect(approve).toBeDisabled()
      await outcome.getByRole('textbox',{name:'Pipeline 3 prompt',exact:true}).fill('Document the export in the README.\tRaise a pull request. Do not merge.')
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
      await expect(approve).toHaveText('Approve and start 3 pipelines');await approve.click()
      // The browser hands the approved plan to the controller once, and from then on only reads its progress.
      await expect(outcome.getByRole('status')).toHaveText(/^Starting session \d of 3\. The controller carries on if you close this\.$/)
      await expect(outcome.getByRole('button',{name:'Close and let it run',exact:true})).toBeVisible()
      await expect(outcome.getByRole('status')).toHaveText('1 started and followed, 1 started but not followed yet, 1 not started.',{timeout:20000})
      const results=outcome.getByRole('list',{name:'Outcome sessions',exact:true}).getByRole('listitem')
      await expect(results).toHaveCount(3)
      await expect(results.nth(0)).toHaveText(/^Export API\s*Started and followed\. Gaffer waits for its first progress\.\s*Open session$/)
      await expect(results.nth(0).getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=outcome-session-1')
      await expect(results.nth(1)).toContainText('Not started: That agent isn’t installed on that machine.')
      await expect(results.nth(1).getByRole('link')).toHaveCount(0)
      await expect(results.nth(2)).toContainText('Started, but not followed: Its chat was not ready')
      // Each stage is sent with its tier and the agent, model and effort that tier means; Gaffer's reason goes with its own.
      const cheap=kind=>({kind,tool:'claude',model:'',tier:'standard'}),strong=kind=>({kind,tool:'claude',model:'opus',effort:'high',tier:'heavy',backup:{tool:'claude',model:'sonnet'}}),ship=kind=>({kind,tool:'claude',model:'',effort:'low',tier:'light'})
      const usual=[cheap('code'),strong('review'),cheap('test'),ship('pr'),ship('babysit'),ship('merge')]
      expect(f.box.approved).toEqual([{objective,projectId:'p-billing',tool:'claude',model:'',provider:'',trust:false,yolo:false,autoReport:false,automation:false,balanceBelow:35,autoMerge:false,maxReviews:8,hours:24,confirmed:true,sessions:[
        {title:'Export API',prompt:proposed.sessions[0].prompt,stages:[(({backup,...rest})=>({...rest,tierWhy:'The export reads the billing ledger while it is being written.',backup}))(strong('code')),...usual.slice(1)]},
        {title:'Export button, renamed',prompt:proposed.sessions[1].prompt,stages:[strong('plan'),{...ship('code'),tierWhy:'Wiring only.',part:'Button',brief:'Add the button and wire it to the endpoint.'},{...cheap('code'),part:'Empty state',brief:'Draw the empty state, with the illustration.'},...usual.slice(1).filter(stage=>stage.kind!=='merge').map(stage=>stage.kind==='test'?ship('test'):stage)]},
        {title:'Export docs',prompt:'Document the export in the README.    Raise a pull request. Do not merge.',stages:usual},
      ]}])
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`,'POST /api/v2/gaffer/outcomes'])
      await outcome.screenshot({path:`${shots}/gaffer-outcome-results-${width}-${theme}.png`})
      await outcome.getByRole('button',{name:'Back to the Factory',exact:true}).click();await expect(outcome).toHaveCount(0)
      // The page lists the controller's record by what each pipeline needs: the one not started waits for the person, the
      // followed one is working, and the one started but not followed has stopped.
      const page=w.page,wide=width>=1100
      await expect(page.getByText('1 waiting for you · 1 working · 1 finished',{exact:true})).toBeVisible()
      const failed=page.getByRole('region',{name:'Waiting for you',exact:true}).locator('[id^="pipeline-"]')
      await expect(failed).toHaveCount(1);await expect(failed).toContainText('Failed');await expect(failed).toContainText('Export button, renamed')
      await expect(failed.getByRole('list',{name:'Stages of Export button, renamed',exact:true}).getByRole('listitem')).toHaveCount(7)
      // A running pipeline shows its stages in its item; its detail adds who does the one in hand and the rest.
      await page.getByRole('button',{name:/^Working/}).click()
      const running=page.getByRole('region',{name:'Working',exact:true}).locator('[id^="pipeline-"]')
      await expect(running).toContainText('Running')
      await expect(running.getByRole('list',{name:'Stages of Export API',exact:true}).getByRole('listitem')).toHaveCount(6)
      await page.getByRole('button',{name:/^Finished or stopped/}).click()
      await expect(page.getByRole('region',{name:'Finished or stopped',exact:true}).locator('[id^="pipeline-"]')).toContainText('Stopped')
      await page.getByRole('region',{name:'Working',exact:true}).getByRole('button',wide?{name:'Running: Export API',exact:true}:{name:'Details and other actions for Export API',exact:true}).click()
      const detail=page.getByRole('region',{name:'Export API',exact:true})
      const stages=detail.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem')
      await expect(stages).toHaveText([/Code/,/Review/,/Test/,/PR/,/Babysit/,/Merge/])
      await expect(stages.first()).toHaveAttribute('aria-current','step')
      await expect(detail.locator('details.row summary').filter({hasText:'Settings for this outcome'})).toContainText('merge waits for you')
      await expect(detail.getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=outcome-session-1')
      // The one not started has no session to open.
      if(!wide)await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
      await page.getByRole('region',{name:'Waiting for you',exact:true}).getByRole('button',wide?{name:'Failed: Export button, renamed',exact:true}:{name:'Details and other actions for Export button, renamed',exact:true}).click()
      const notStarted=page.getByRole('region',{name:'Export button, renamed',exact:true})
      await expect(notStarted.getByRole('region',{name:'What is waiting',exact:true})).toContainText('Not started: That agent isn’t installed on that machine.')
      await expect(notStarted.getByRole('link')).toHaveCount(0)
      if(!wide)await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
      await page.getByRole('button',{name:/^Other sessions/}).click()
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
      // OpenCode's descriptor names no thought levels, so no level is guessed: a stage the agent cannot confirm
      // would fail the whole launch. The selector says so and stays at the agent's default.
      const effort=outcome.getByRole('combobox',{name:'Effort',exact:true})
      await expect(effort).toBeDisabled()
      await expect(effort).toHaveValue('')
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      const trust=outcome.getByRole('checkbox',{name:/^I trust this folder: OpenCode may run the plugins/})
      await expect(trust).not.toBeChecked();await trust.check()
      // Merging by itself is this outcome's own choice, off until ticked.
      const auto=outcome.getByRole('checkbox',{name:/^Merge by itself once every check passes/})
      await expect(auto).not.toBeChecked();await auto.check()
      // Reporting by itself spends Gaffer tokens unasked, so it is off until ticked.
      const reports=outcome.getByRole('checkbox',{name:/^Have Gaffer report across this outcome by itself/})
      await expect(reports).not.toBeChecked();await reports.check()
      // YOLO is off until confirmed, and is this outcome's choice only.
      const approvals=outcome.getByRole('radiogroup',{name:'Approvals',exact:true})
      // Who decides when a stage stops is the person's choice too, and theirs unless they say otherwise.
      const decisions=outcome.getByRole('radiogroup',{name:'Decisions',exact:true})
      await expect(decisions.getByRole('radio',{name:'Ask me',exact:true})).toBeChecked()
      await decisions.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
      await expect(outcome).toContainText('at most three times for a stage');await expect(outcome).toContainText('It also answers an agent’s multiple-choice question.');await expect(outcome).toContainText('Merges, approvals and questions that need your own words still come to you.')
      await decisions.getByRole('radio',{name:'Ask me',exact:true}).check()
      await expect(outcome).toContainText('shows you the choices with a recommendation first, and waits for you.')
      await expect(approvals.getByRole('radio',{name:'Ask me',exact:true})).toBeChecked()
      await approvals.getByRole('radio',{name:'YOLO',exact:true}).click()
      const confirm=w.page.getByRole('alertdialog',{name:'Turn on YOLO for this launch?',exact:true})
      await expect(confirm).toContainText('Your account allows YOLO');await expect(confirm).toContainText('workshop accepts YOLO sessions');await expect(confirm).toContainText('Every agent in the crew has a YOLO option')
      await confirm.getByRole('button',{name:'Turn on YOLO',exact:true}).click()
      await expect(approvals.getByRole('radio',{name:'YOLO',exact:true})).toBeChecked()
      await expect(outcome.getByText('On YOLO the agents do not wait for you',{exact:false})).toBeVisible()
      await outcome.screenshot({path:`${shots}/gaffer-outcome-yolo-1280-light.png`})
      await outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true}).click()
      expect(f.box.approved[0]).toMatchObject({tool:'opencode',trust:true,yolo:true,autoReport:true,model:'',autoMerge:true})
      expect(f.box.approved[0].sessions[0].stages.every(stage=>stage.tool==='opencode'&&!stage.effort)).toBe(true)
      await expect(outcome.getByRole('status')).toHaveText('Starting session 1 of 2. The controller carries on if you close this.')
      await outcome.getByRole('button',{name:'Close and let it run',exact:true}).click();await expect(outcome).toHaveCount(0)
      // Nothing waits yet, so the first pipeline working is open beside the list; its outcome's menu stops the starting.
      const page=w.page,detail=page.locator('.pane')
      await expect(detail).toContainText('the controller is starting sessions')
      await detail.getByRole('button',{name:'More for this outcome',exact:true}).click()
      await detail.getByRole('button',{name:'Stop starting sessions',exact:true}).click()
      await expect(page.getByText('Stopped. Sessions not yet started stay unstarted; the ones already started keep running.',{exact:true})).toBeVisible()
      // A stopped outcome has nothing moving: its pipelines that never started sit in Finished or stopped.
      await expect(page.getByText('0 waiting for you · 0 working · 2 finished',{exact:true})).toBeVisible()
      await page.getByRole('button',{name:/^Finished or stopped/}).click()
      const halted=page.getByRole('region',{name:'Finished or stopped',exact:true}).locator('[id^="pipeline-"]')
      await expect(halted).toHaveCount(2);await expect(halted).toContainText(['Stopped','Stopped'])
      // Each says why it never started, in its detail, though it waits for nobody.
      await expect(detail).toContainText('Not started: this outcome was stopped first.')
      await halted.nth(1).click()
      await expect(detail.getByRole('heading',{level:2})).toHaveText(await halted.nth(1).locator('.title').innerText())
      await expect(detail).toContainText('Not started: this outcome was stopped first.')
      await detail.getByRole('button',{name:'More for this outcome',exact:true}).click()
      await expect(detail.getByRole('button',{name:'Stop starting sessions',exact:true})).toHaveCount(0)
      expect(f.box.stops).toEqual([{revision:1}])
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`,'POST /api/v2/gaffer/outcomes',`POST /api/v2/gaffer/outcomes/${oid}/stop`]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A model that cannot plan says so, and the person can write the plan; the rest of the flow is the same.
  {
    // This account saved a crew earlier: the dialog starts from it, for the tiers this machine can run.
    const f=fixture({planFails:true,crew:{light:{tool:'codex',model:'gone'},standard:{},heavy:{tool:'claude',model:'opus',effort:'high'}}}),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      // The saved crew is read when the dialog opens and shown tier by tier before anything is submitted: the agent and
      // model already chosen in Gaffer settings, including a tier this machine cannot run.
      const fromSettings=outcome.getByRole('region',{name:'Your Gaffer settings',exact:true})
      await expect(fromSettings).toContainText('Heavy')
      await expect(fromSettings).toContainText('Agent Claude Code')
      await expect(fromSettings).toContainText('Model opus')
      await expect(fromSettings).toContainText('Light')
      await expect(fromSettings).toContainText('Agent Codex')
      await expect(fromSettings).toContainText('Model gone')
      // The unavailability fallback is stated where the saved crew is read, so a listed agent this machine cannot run
      // is not mistaken for one that will be used as it is.
      await expect(fromSettings).toContainText('a saved agent this machine cannot run falls back to the outcome’s own agent')
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      await outcome.getByText('Choose who does what',{exact:true}).click()
      await expect(outcome.getByText('Started from your saved crew.',{exact:true})).toBeVisible()
      await expect(outcome.getByRole('combobox',{name:'Heavy model',exact:true})).toHaveValue('opus')
      await expect(outcome.getByRole('combobox',{name:'Heavy effort',exact:true})).toHaveValue('high')
      // A saved tier whose agent this machine cannot run is left on the outcome's own agent.
      await expect(outcome.getByRole('combobox',{name:'Light agent',exact:true})).toHaveValue('claude')
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      await expect(outcome.getByRole('alert')).toHaveText('This model did not return a plan. Choose a model that supports tools, or write the plan yourself.')
      await outcome.getByRole('button',{name:'Write the plan myself',exact:true}).click()
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true}).getByRole('listitem')).toHaveCount(1)
      await expect(outcome.getByRole('button',{name:/^Approve and start/})).toBeDisabled()
      await outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true}).fill('Whole export')
      await outcome.getByRole('textbox',{name:'Pipeline 1 prompt',exact:true}).fill('Build the CSV export end to end. Raise a pull request. Do not merge.')
      await expect(outcome.getByRole('button',{name:'Approve and start 1 pipeline',exact:true})).toBeEnabled()
      // Back keeps the objective; closing starts nothing.
      await outcome.getByRole('button',{name:'Back',exact:true}).click()
      await expect(outcome.getByRole('textbox',{name:'Outcome',exact:true})).toHaveValue(objective)
      await w.page.keyboard.press('Escape');await expect(outcome).toHaveCount(0)
      await expect(w.page.getByRole('button',{name:'Give Gaffer an outcome',exact:true})).toBeFocused()
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`]);expect(f.box.approved).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // Gaffer settings reports a saved crew but hands back nothing, and cannot be read at all: either way the dialog says
  // so and leaves the crew to the person, rather than showing blank agent and model fields as if none were saved.
  {
    const f=fixture({crewEmpty:true}),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      const fromSettings=outcome.getByRole('region',{name:'Your Gaffer settings',exact:true})
      await expect(fromSettings).toContainText('No crew is saved in Gaffer settings yet.')
      await expect(fromSettings.locator('dl')).toHaveCount(0)
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // Gaffer settings cannot be read: the dialog says so and leaves the crew to the person, rather than failing or showing
  // blank settings as if none were saved.
  {
    const f=fixture({crewFails:true}),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      const fromSettings=outcome.getByRole('region',{name:'Your Gaffer settings',exact:true})
      await expect(fromSettings).toContainText('Your Gaffer settings could not be read.')
      await expect(fromSettings).toContainText('Each stage will use the agent and model you choose below.')
      await expect(fromSettings).not.toContainText('No crew is saved')
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // The optional image on an outcome: validation, removal, planning stays text-only, one upload at approval,
  // a failed upload blocking approval until it is retried, retention across a project change, and a clean reopen.
  {
    const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      const input=outcome.locator('input[type=file]')
      await input.setInputFiles({name:'shot.gif',mimeType:'image/gif',buffer:PNG})
      await expect(outcome.getByText('Attach a PNG, JPEG or WebP image.',{exact:true})).toBeVisible()
      await input.setInputFiles({name:'huge.png',mimeType:'image/png',buffer:Buffer.alloc((5<<20)+1)})
      await expect(outcome.getByText('That image is larger than 5 MiB.',{exact:true})).toBeVisible()
      await input.setInputFiles({name:'broken.png',mimeType:'image/png',buffer:Buffer.from('not an image')})
      await expect(outcome.getByText('That image could not be read.',{exact:true})).toBeVisible();await expect(outcome.locator('.chosen')).toHaveCount(0)
      await input.setInputFiles({name:'outcome.png',mimeType:'image/png',buffer:PNG})
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      await outcome.getByRole('button',{name:/^Remove outcome\.png$/}).click()
      await expect(outcome.getByText('outcome.png',{exact:true})).toHaveCount(0)
      await input.setInputFiles({name:'outcome.png',mimeType:'image/png',buffer:PNG})
      // The upload targets the project chosen at approval, so changing the project keeps the picture.
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-notes')
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      expect(f.box.uploads).toBe(0)
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true}).getByRole('listitem')).toHaveCount(2)
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      expect(f.box.uploads).toBe(0)
      f.box.uploadFails=true
      await outcome.getByRole('button',{name:/^Approve and start/}).click()
      await expect(outcome.getByRole('alert')).toHaveText('The image could not be uploaded.')
      expect(f.box.approved).toEqual([])
      f.box.uploadFails=false
      await outcome.getByRole('button',{name:/^Approve and start/}).click()
      await expect(outcome.getByRole('button',{name:'Close and let it run',exact:true})).toBeVisible({timeout:20000})
      expect(f.box.uploads).toBe(2)
      for(const sent of f.box.approved[0].sessions) expect(sent.prompt).toMatch(/\n\nImage: \/home\/dev\/launch-uploads\/outcome\.png$/)
      await outcome.getByRole('button',{name:'Close and let it run',exact:true}).click();await expect(outcome).toHaveCount(0)
      const again=await openOutcome(w)
      await expect(again.outcome.getByText('outcome.png',{exact:true})).toHaveCount(0)
      await expect(again.outcome.locator('.chosen')).toHaveCount(0)
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A proposal can be answered back, and turned down on purpose, without starting anything.
  {
    const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true}).getByRole('listitem')).toHaveCount(2)
      const replyText='Not that: keep the export in one session and put its tests with it.'
      const replyBox=outcome.getByRole('textbox',{name:'Reply to Gaffer',exact:true})
      await expect(replyBox).toBeVisible()
      await expect(outcome.getByRole('button',{name:'Send reply',exact:true})).toBeDisabled()
      await replyBox.fill(replyText)
      await outcome.getByRole('button',{name:'Send reply',exact:true}).click()
      // The reply is in the conversation, and Gaffer's revised proposal replaces the rows.
      const talk=outcome.getByRole('region',{name:'Conversation with Gaffer',exact:true})
      await expect(talk).toContainText(replyText)
      await expect(talk).toContainText('One session, end to end: the endpoint, its tests and the button together.')
      await expect(outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true})).toHaveValue('Export, end to end')
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true}).getByRole('listitem')).toHaveCount(1)
      const stages=['code','review','test','pr','babysit','merge']
      expect(f.box.replies).toEqual([{revision:4,objective,project:'billing',sessions:3,reply:replyText,previous:{summary:proposed.summary,sessions:[{title:'Export API',stages},{title:'Export button',stages}]}}])
      // Declining is explicit, and nothing is started or sent.
      await outcome.getByRole('button',{name:'Decline proposal',exact:true}).click()
      const confirm=w.page.getByRole('dialog',{name:'Decline this proposal?',exact:true})
      await expect(confirm).toContainText('Nothing has started.')
      await confirm.getByRole('button',{name:'Decline proposal',exact:true}).click()
      await expect(outcome.getByText('Proposal declined. Nothing was started.',{exact:false})).toBeVisible()
      await expect(outcome.getByRole('list',{name:'Planned sessions',exact:true})).toHaveCount(0)
      await expect(outcome.getByRole('button',{name:/^Approve and start/})).toHaveCount(0)
      await expect(outcome.getByRole('textbox',{name:'Reply to Gaffer',exact:true})).toBeVisible()
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`,`POST /api/v2/gaffer/providers/${pid}/plan`])
      expect(f.box.approved).toEqual([])
      await outcome.getByRole('button',{name:'Back to the outcome',exact:true}).click()
      await expect(outcome.getByRole('textbox',{name:'Outcome',exact:true})).toHaveValue(objective)
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A repository From GitHub in place of a project: the person picks one from the machine's list, and Ask Gaffer for a
  // plan first has the machine clone it and saves it as a project, then plans in it. Nobody adds a project by hand.
  for(const [width,theme] of [[1440,'light'],[390,'dark']]){
    const f=fixture()
    const cloner={...machines[0],capabilities:{...machines[0].capabilities,repoClone:true,repoList:true,worktree:true}}
    const cloned={...folder('p-site','site','a1','workshop'),dir:'/home/dev/reet/repos/acme/site',baseBranch:'main',origin:'acme/site',cloned:true}
    const list=[...projects],starts=[],reads=[]
    f.routes['/api/v2/folder-projects']=()=>({projects:list,savedWaiting:0})
    f.routes['/api/v2/folder-projects/github-repos']=()=>({repos:[{name:'acme/site',private:true,pushedAt:Math.floor(now/1000)-3600},{name:'acme/docs',private:false,pushedAt:0}],more:false})
    f.routes['/api/v2/folder-projects/from-github']=request=>{starts.push(request.postDataJSON());return {status:202,json:{cloneId:'c1'.repeat(16),dir:'~/reet/repos/acme/site'}}}
    f.routes[`/api/v2/folder-projects/clones/${'c1'.repeat(16)}`]=()=>{reads.push(Date.now());if(reads.length<2)return {state:'cloning'};if(!list.some(p=>p.id==='p-site'))list.push(cloned);return {state:'done',project:cloned}}
    const w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,terminalMachines:[cloner,machines[1]],routes:f.routes})
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption({label:'A repository from GitHub…'})
      // Only a connected machine that can clone is offered, and with one it is chosen already.
      await expect(outcome.getByRole('combobox',{name:'Machine',exact:true}).getByRole('option')).toHaveText(['workshop'])
      await expect(outcome.getByText('Choose a repository, or type owner/name.',{exact:true})).toBeVisible()
      const field=outcome.getByRole('combobox',{name:'Repository',exact:true})
      await field.click()
      await field.pressSequentially('site')
      await outcome.getByRole('listbox',{name:"workshop's repositories"}).getByRole('option',{name:/acme\/site/}).click()
      await expect(field).toHaveValue('acme/site')
      await expect(outcome.getByText('reet clones it to ~/reet/repos/acme/site on workshop before Gaffer plans. Once the outcome is done reet removes the clone, unless that would lose work.',{exact:true})).toBeVisible()
      // The agents are the chosen machine's.
      await expect(outcome.getByRole('combobox',{name:'Agent',exact:true}).getByRole('option')).toHaveText(['Claude Code','OpenCode'])
      const ask=outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true})
      await expect(ask).toBeEnabled()
      await ask.click()
      await expect(outcome.getByText('Cloning acme/site on workshop… This can take a few minutes; Gaffer plans when it\'s done.',{exact:true})).toBeVisible()
      await expect(outcome.getByRole('button',{name:'Cloning…',exact:true})).toBeDisabled()
      // Once cloned it is a project like any other: Gaffer plans in it, and the plan names it.
      await expect.poll(()=>f.box.plans.length,{timeout:15000}).toBe(1)
      expect(starts).toEqual([{host:'a1',repo:'acme/site',name:'',temporary:true}])
      expect(f.box.plans[0].project).toBe('site')
      await expect(outcome.getByText('site. Each row below is a pipeline',{exact:false})).toBeVisible()
      await expect(outcome.getByText('in its own worktree',{exact:false})).toBeVisible()
    } finally { await w.context.close() }
  }
  // A repository already cloned there is used as it is: nothing is cloned again.
  {
    const f=fixture()
    const cloner={...machines[0],capabilities:{...machines[0].capabilities,repoClone:true,worktree:true}}
    const cloned={...folder('p-site','site','a1','workshop'),origin:'acme/site',cloned:true}
    const starts=[]
    f.routes['/api/v2/folder-projects']={projects:[...projects,cloned],savedWaiting:0}
    f.routes['/api/v2/folder-projects/from-github']=request=>{starts.push(request.postDataJSON());return {status:500,json:{error:'not expected'}}}
    const w=await openWorkspace(browser,{width:1440,height:1000,terminalMachines:[cloner,machines[1]],routes:f.routes})
    try {
      const {outcome}=await openOutcome(w)
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption({label:'A repository from GitHub…'})
      // No list on a machine that cannot list: the plain field takes what is typed.
      await outcome.getByRole('textbox',{name:'Repository',exact:true}).fill('https://github.com/Acme/Site')
      await expect(outcome.getByText('Works in your project site, /work/site.',{exact:true})).toBeVisible()
      await outcome.getByRole('button',{name:'Write the plan myself',exact:true}).click()
      await expect(outcome.getByText('site. Each row below is a pipeline',{exact:false})).toBeVisible()
      expect(starts).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Gaffer outcome on the Factory page: objective gates, the crew and stages per pipeline, auto-merge as the outcome’s own choice, plan from the typed objective only, editable plan, refused prompts and limits caught first, nothing saved before approval, one approved plan handed to the controller, progress read from its record, partial results, the record on the floor, closing while it starts, stopping unstarted sessions, a model that cannot plan and a hand-written plan, the optional image attach/remove/upload, and replying to or declining a proposal, and a repository from GitHub cloned (or reused) as the project, passed on desktop and phone.')
} finally { await browser.close() }
