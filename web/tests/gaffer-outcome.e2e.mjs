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
  {agentId:'a1',host:'workshop',ready:true,capabilities:{launch:true,acp:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available'},{tool:'codex',adapter:'codex-acp',state:'missing'}]}},
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
const PNG=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z0GQAAAAASUVORK5CYII=','base64')

function fixture({planFails=false}={}){
  const box={writes:[],plans:[],creates:[],setups:[],goalReads:{},sessions:[],goals:[],conversations:new Map(),planFails,uploads:0,uploadFails:false}
  const general={conversation:{id:generalId,sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/folder-projects':{projects,savedWaiting:0},
    '/api/v2/machines/a1/uploads':request=>{expect(request.method()).toBe('POST');box.uploads++;if(box.uploadFails)return{status:502,json:{error:'The image could not be uploaded.'}};return{status:200,headers:{'X-Reet-Upload-Path':'/home/dev/launch-uploads/outcome.png','X-Reet-Upload-Reference':'/home/dev/launch-uploads/outcome.png'},json:{uploaded:'true'}}},
    '/api/v2/gaffer/goals':()=>({goals:box.goals,readAt:now}),
    [`/api/v2/gaffer/providers/${pid}/plan`]:request=>{
      expect(request.method()).toBe('POST');box.plans.push(request.postDataJSON())
      return box.planFails?{status:502,json:{error:'This model did not return a plan. Choose a model that supports tools, or write the plan yourself.'}}:proposed
    },
    '/api/v2/terminal/sessions':request=>{
      if(request.method()!=='POST')return {sessions:box.sessions}
      const input=request.postDataJSON();box.creates.push(input)
      // The second approved session is refused by its machine; the others start.
      if(input.title==='Export button, renamed')return {status:409,json:{error:'That machine is busy starting another agent.'}}
      const n=box.sessions.length+1
      const session={id:`outcome-session-${n}`,agentId:input.agentId,title:input.title,state:'running',structured:true,tool:input.tool,dir:input.dir,projectId:input.projectId,createdAt:now,updatedAt:now,exitCode:null}
      box.sessions.push(session);return session
    },
    '/api/v2/gaffer/conversations':request=>{
      const query=new URL(request.url()).searchParams
      if(request.method()==='POST'){
        const input=request.postDataJSON(),n=Number(input.sessionId.slice(-1))
        const conversation={id:cid(n),sessionId:input.sessionId,providerId:input.providerId,model:input.model,revision:1,createdAt:now,updatedAt:now}
        box.conversations.set(input.sessionId,conversation);return {conversation,turns:[]}
      }
      if(query.get('all')==='true')return {conversations:[general.conversation,...box.conversations.values()]}
      const sid=query.get('sessionId')||''
      if(!sid)return general
      const found=box.conversations.get(sid)
      return found?{conversation:found,turns:[]}:{status:404,json:{error:'No conversation yet.'}}
    },
  }
  for(const n of [1,2]){
    const path=`/api/v2/gaffer/conversations/${cid(n)}/goal`
    // A chat that has only just started is not ready for follow-up on the first read.
    routes[path]=()=>{box.goalReads[n]=(box.goalReads[n]||0)+1;return {goal:null,pins,allowed:false,setupAllowed:box.goalReads[n]>1,providerName:'Workshop Gaffer',requiresProviderAccess:true,conversationRevision:1}}
    routes[path+'/setup']=request=>{
      const input=request.postDataJSON(),sid=`outcome-session-${n}`;box.setups.push({sid,...input})
      if(n===2)return {status:409,json:{error:'The worker’s message channel is unavailable.'}}
      const goal={...pins,id:cid(n),conversationId:cid(n),sessionId:sid,revision:1,text:input.text,state:'active',note:'Waiting for new worker progress.',maxReviews:input.maxReviews,usedReviews:0,expiresAt:now+input.hours*3600000,createdAt:now,updatedAt:now,nextAt:now,lastSeq:0,summary:'',usageReported:false,inputTokens:0,outputTokens:0}
      box.goals.push({goal,session:{id:sid,agentId:'a1',title:box.sessions[n-1].title,state:'running'},available:true,attention:false})
      return {goal,pins,allowed:true,setupAllowed:true}
    }
  }
  return {box,routes}
}
async function openOutcome(w){
  await w.page.goto(base+'/');await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
  await w.page.getByRole('dialog',{name:'Gaffer',exact:true}).getByRole('button',{name:'Open the factory floor',exact:true}).click()
  const floor=w.page.getByRole('dialog',{name:'Factory floor',exact:true})
  await floor.getByRole('button',{name:'Give Gaffer an outcome',exact:true}).click()
  return {floor,outcome:w.page.getByRole('dialog',{name:'Give Gaffer an outcome',exact:true})}
}
const watch=(w,box)=>w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')box.writes.push(`${r.method()} ${new URL(r.url()).pathname}`)})

try {
  for(const [width,theme] of [[1440,'light'],[390,'dark']]){
    const f=fixture(),w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      const {floor,outcome}=await openOutcome(w)
      const ask=outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}),own=outcome.getByRole('button',{name:'Write the plan myself',exact:true})
      // Nothing can be asked until there is an outcome, a connected project and an agent that can chat.
      await expect(ask).toBeDisabled();await expect(own).toBeDisabled()
      await expect(outcome.getByText('Write the outcome you want.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await expect(outcome.getByText('Choose the project the sessions work in.',{exact:true})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-attic')
      await expect(outcome.getByText('That project’s machine is not connected.',{exact:true})).toBeVisible();await expect(ask).toBeDisabled()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-notes')
      await expect(outcome.getByText('not a git repository',{exact:false})).toBeVisible()
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      // Only an agent whose chat adapter is installed on that machine is offered.
      await expect(outcome.getByRole('combobox',{name:'Agent',exact:true}).getByRole('option')).toHaveText(['Claude Code'])
      await expect(outcome.getByRole('combobox',{name:'Sessions',exact:true})).toHaveValue('3')
      await expect(outcome.getByText('sent only what you type here and the project’s name',{exact:false})).toBeVisible()
      expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await outcome.screenshot({path:`${shots}/gaffer-outcome-objective-${width}-${theme}.png`})
      expect(f.box.writes).toEqual([])
      await ask.click()
      // The plan is the typed objective and the project's name, nothing else.
      const plan=outcome.getByRole('list',{name:'Planned sessions',exact:true})
      await expect(plan.getByRole('listitem')).toHaveCount(2)
      expect(f.box.plans).toEqual([{revision:4,objective,project:'billing',sessions:3}])
      await expect(outcome.getByText('Two independent halves: the endpoint, then the button.',{exact:true})).toBeVisible()
      await expect(outcome.getByRole('textbox',{name:'Session 1 title',exact:true})).toHaveValue('Export API')
      await expect(outcome.getByRole('textbox',{name:'Session 2 prompt',exact:true})).toHaveValue(/^Add the export button/)
      await expect(outcome.getByText('with ordinary approvals (no YOLO)',{exact:false})).toBeVisible()
      // Every part of the plan is editable, and a session the person adds is as good as one Gaffer proposed.
      await outcome.getByRole('textbox',{name:'Session 2 title',exact:true}).fill('Export button, renamed')
      await outcome.getByRole('button',{name:'Add a session',exact:true}).click()
      const approve=outcome.getByRole('button',{name:/^Approve and start/})
      await expect(approve).toBeDisabled();await expect(outcome.getByText('Give session 3 a title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Session 3 title',exact:true}).fill('Export API')
      await outcome.getByRole('textbox',{name:'Session 3 prompt',exact:true}).fill('Document the export in the README. Raise a pull request. Do not merge.')
      await expect(outcome.getByText('Two sessions are both called “Export API”. Give each its own title.',{exact:true})).toBeVisible()
      await outcome.getByRole('textbox',{name:'Session 3 title',exact:true}).fill('Export docs')
      // The person may add up to the five an outcome can start, whatever size they asked Gaffer to plan.
      await expect(outcome.getByRole('button',{name:'Add a session',exact:true})).toBeVisible()
      await outcome.getByText('Change limits',{exact:true}).click()
      await outcome.getByRole('spinbutton',{name:'Review budget',exact:true}).fill('8')
      expect(await outcome.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await outcome.screenshot({path:`${shots}/gaffer-outcome-plan-${width}-${theme}.png`})
      // Until Approve, the only request made is the plan itself: no session, conversation, grant or goal.
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`])
      await expect(approve).toHaveText('Approve and start 3 sessions');await approve.click()
      await expect(outcome.getByRole('status')).toHaveText('1 started and followed, 1 started but not followed yet, 1 not started.',{timeout:20000})
      const results=outcome.getByRole('list',{name:'Outcome sessions',exact:true}).getByRole('listitem')
      await expect(results).toHaveCount(3)
      await expect(results.nth(0)).toContainText('Following · Started and followed.')
      await expect(results.nth(0).getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=outcome-session-1&gaffer=1')
      await expect(results.nth(1)).toContainText('Not started · Not started: That machine is busy starting another agent.')
      await expect(results.nth(2)).toContainText('Started, not followed · Started, but not followed: The worker’s message channel is unavailable.')
      // Each session is the ordinary reviewed launch of a chat in the project: the person's prompt, no YOLO, no model's choice of folder.
      expect(f.box.creates.map(c=>c.title)).toEqual(['Export API','Export button, renamed','Export docs'])
      for(const sent of f.box.creates){
        expect(sent).toMatchObject({agentId:'a1',tool:'claude',dir:'/work/billing',projectId:'p-billing',structured:true,cols:100,rows:30})
        expect(sent.yolo).toBeUndefined();expect(sent.requestId).toMatch(/^[A-Za-z0-9_-]{16,128}$/);expect(sent.prompt).toMatch(/Do not merge\.$/)
      }
      expect(new Set(f.box.creates.map(c=>c.requestId)).size).toBe(3)
      // A fresh chat was not ready on the first read, so setup waited for it; each goal names the outcome and the session's part.
      expect(f.box.goalReads[1]).toBe(2)
      expect(f.box.setups.map(s=>s.sid)).toEqual(['outcome-session-1','outcome-session-2'])
      expect(f.box.setups[0]).toMatchObject({...pins,revision:0,conversationRevision:1,confirmed:true,action:'start',maxReviews:8,hours:24})
      expect(f.box.setups[0].text).toMatch(/^Outcome: Ship CSV export for billing\.\nThis session’s part: Export API\n[\s\S]*Do not merge\.$/)
      await outcome.screenshot({path:`${shots}/gaffer-outcome-results-${width}-${theme}.png`})
      await outcome.getByRole('button',{name:'Back to the floor',exact:true}).click();await expect(outcome).toHaveCount(0)
      // The floor groups the followed session under its outcome; the started-but-unfollowed one waits under Not followed.
      const outcomes=floor.getByRole('region',{name:'Outcomes',exact:true})
      const group=outcomes.getByRole('button',{name:/^Ship CSV export for billing\./})
      await expect(group).toContainText('1 session · 1 following')
      await expect(floor.getByRole('heading',{name:'Export API',exact:true})).toBeVisible()
      const picks=floor.getByRole('group',{name:'Running chats not followed',exact:true})
      await expect(picks.getByRole('checkbox')).toHaveCount(1);await expect(picks).toContainText('Export docs')
      await group.click();await expect(group).toHaveAttribute('aria-pressed','true')
      await expect(picks).toHaveCount(0);await expect(floor.getByRole('list',{name:'Followed session goals',exact:true}).getByRole('listitem')).toHaveCount(1)
      await floor.getByRole('button',{name:'Show everything',exact:true}).click();await expect(picks.getByRole('checkbox')).toHaveCount(1)
      expect(await floor.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await floor.screenshot({path:`${shots}/gaffer-outcome-floor-${width}-${theme}.png`})
      expect(w.errors).toEqual([])
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
      await expect(w.page.getByRole('dialog',{name:'Factory floor',exact:true}).getByRole('button',{name:'Give Gaffer an outcome',exact:true})).toBeFocused()
      expect(f.box.writes).toEqual([`POST /api/v2/gaffer/providers/${pid}/plan`]);expect(f.box.creates).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // The optional image on an outcome: validation, removal, the plan staying text-only, one upload at approval,
  // a failed upload blocking approval until the image is removed, and a clean reopen.
  {
    const f=fixture(),w=await openWorkspace(browser,{width:1280,height:1000,terminalMachines:machines,routes:f.routes})
    watch(w,f.box)
    try {
      let opened=await openOutcome(w)
      let {outcome}=opened
      await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill(objective)
      await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p-billing')
      const input=outcome.locator('input[type=file]')
      await input.setInputFiles({name:'shot.gif',mimeType:'image/gif',buffer:PNG})
      await expect(outcome.getByText('Attach a PNG, JPEG or WebP image.',{exact:true})).toBeVisible()
      await input.setInputFiles({name:'huge.png',mimeType:'image/png',buffer:Buffer.alloc((5<<20)+1)})
      await expect(outcome.getByText('That image is larger than 5 MiB.',{exact:true})).toBeVisible()
      await input.setInputFiles({name:'outcome.png',mimeType:'image/png',buffer:PNG})
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      await outcome.getByRole('button',{name:/^Remove outcome\.png$/}).click()
      await expect(outcome.getByText('outcome.png',{exact:true})).toHaveCount(0)
      await input.setInputFiles({name:'outcome.png',mimeType:'image/png',buffer:PNG})
      expect(f.box.writes).toEqual([])
      await outcome.getByRole('button',{name:'Ask Gaffer for a plan',exact:true}).click()
      await expect(outcome.getByText('Two independent halves: the endpoint, then the button.',{exact:true})).toBeVisible()
      await expect(outcome.getByText('outcome.png',{exact:true})).toBeVisible()
      expect(f.box.uploads).toBe(0);expect(f.box.plans).toHaveLength(1)
      f.box.uploadFails=true
      await outcome.getByRole('button',{name:/^Approve and start/}).click()
      await expect(outcome.getByRole('alert')).toHaveText('The image could not be uploaded.')
      expect(f.box.creates).toEqual([])
      f.box.uploadFails=false
      await outcome.getByRole('button',{name:/^Approve and start/}).click()
      await expect(outcome.getByRole('status')).toHaveText('1 started and followed, 1 started but not followed yet.',{timeout:20000})
      expect(f.box.uploads).toBe(2)
      for(const sent of f.box.creates) expect(sent.prompt).toMatch(/\n\nImage: \/home\/dev\/launch-uploads\/outcome\.png$/)
      await outcome.getByRole('button',{name:'Back to the floor',exact:true}).click();await expect(outcome).toHaveCount(0)
      opened=await openOutcome(w);outcome=opened.outcome
      await expect(outcome.getByText('outcome.png',{exact:true})).toHaveCount(0)
      await expect(outcome.locator('.chosen')).toHaveCount(0)
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Gaffer outcome: objective gates, plan from the typed objective only, editable plan, nothing started before approval, ordinary launches without YOLO, waiting for a fresh chat, goals naming the outcome, partial results, outcome grouping on the floor, a model that cannot plan and a hand-written plan passed on desktop and phone.')
} finally { await browser.close() }
