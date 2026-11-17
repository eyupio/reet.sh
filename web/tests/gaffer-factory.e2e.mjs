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
 {key:'a',title:'Export API',prompt:'p',state:'following',note:'The review stage is under way.',sessionId:'s1',conversationId:'1'.repeat(32),updatedAt:now,stage:3,stages:line(3,true).map(s=>s.kind==='code'?{...s,tier:'heavy',tierWhy:'The export reads the billing ledger while it is being written.'}:s.kind==='merge'?{...s,tier:'light'}:s)},
 {key:'b',title:'Export button',prompt:'p',state:'following',note:'The code stage is under way.',sessionId:'s2',conversationId:'2'.repeat(32),updatedAt:now,stage:2,stages:line(2,true)},
 {key:'c',title:'Export docs',prompt:'p',state:'ready_to_merge',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s3',conversationId:'3'.repeat(32),updatedAt:now,stage:7,stages:line(7,false)},
]},{id:'d'.repeat(32),revision:2,objective:'Fix the flaky importer test.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'importer',dir:'/work/importer',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-86400000,updatedAt:now,sessions:[
 {key:'a',title:'Importer retry',prompt:'p',state:'done',note:'Every stage is done. Check the result; only you mark the work complete.',sessionId:'s4',conversationId:'4'.repeat(32),updatedAt:now,stage:3,autoMerge:true,stages:['code','test','pr','merge'].map(k=>st(k,'claude','','done',{summary:'ok'}))},
]}]
const goals=[goal('1'.repeat(32),'s1','Export API','active',false,'The endpoint and its tests are committed. The review found one missing error case and is fixing it.'),goal('2'.repeat(32),'s2','Export button','needs_user',true,'The button is in, but the design leaves the empty state open. Which of the two do you want?'),goal('5'.repeat(32),'s5','Login tokens','active',false,'Checks are green.')]
// Gaffer's review of the button stage said what it would do about its own question, and why.
goals[1].goal.advice={choice:'tell',why:'The design already names the empty state; the agent only needs telling which.',text:'Use the illustrated empty state from the design.'}
const sessions=['s1','s2','s3','s4','s5','s6'].map((id,n)=>({id,agentId:'a1',title:['Export API','Export button','Export docs','Importer retry','Login tokens','Docs refresh'][n],state:'running',structured:true,tool:'claude',dir:'/work/x',createdAt:now,updatedAt:now,exitCode:null,...(id==='s5'?{projectId:'p1'}:{})}))
const machines=[{agentId:'a1',host:'workshop',ready:true,capabilities:{agentAccounts:true,launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:[{id:'effort',category:'thought_level',values:['default','low','high']}]},{tool:'codex',adapter:'codex-acp',state:'available',options:[{id:'effort',category:'thought_level',values:['default','low','high']}]}]}}]
// What the working agents reported: the code stage's own count, and the review stage in hand with its context window.
Object.assign(outcomes[0].sessions[0].stages[2],{agentInputTokens:60000,agentOutputTokens:24600,agentCachedTokens:410000,agentUsageReported:true})
Object.assign(outcomes[0].sessions[0].stages[3],{agentInputTokens:9000,agentOutputTokens:3400,agentUsageReported:true,contextUsed:176000,contextSize:200000})
// The first pipeline's plan was moved on by the person, and its review stage in hand reports what its agent is doing.
Object.assign(outcomes[0].sessions[0].stages[0],{closedBy:'person'});Object.assign(outcomes[0].sessions[0].stages[1],{closedBy:'gaffer'})
Object.assign(outcomes[0].sessions[0].stages[3],{startedAt:now-42*60000,agentState:'working',agentActiveAt:now-20*60000})
// What each stage's session was read back to be on: the code stage ran as chosen (its model left to the agent), and the
// review stage in hand is on a lower effort than the one chosen for it.
Object.assign(outcomes[0].sessions[0].stages[2],{choiceCheck:'applied',appliedTool:'codex',appliedModel:'gpt-coder',appliedEffort:'medium',appliedAt:now-90*60000})
Object.assign(outcomes[0].sessions[0].stages[3],{effort:'high',choiceCheck:'mismatch',appliedTool:'claude',appliedModel:'opus',appliedEffort:'low',appliedAt:now-42*60000,choiceNote:'You chose the effort high; the session is on low.'})
// A decision Gaffer took earlier, while it had this outcome's decisions.
outcomes[0].decisions=[{at:now-50*60000,key:'a',title:'Export API',stage:'code',about:'question',choice:'move',why:'The endpoint and its tests are in the handover; nothing more belongs in this stage.',model:'gaffer-model'},
 // …and an agent's multiple-choice question it answered.
 {at:now-30*60000,key:'a',title:'Export API',stage:'review',about:'agent_question',choice:'answer',why:'The brief says the export must not load the primary.',text:'Asked: Which database should the export read from? Answered: The read replica',model:'gaffer-model'}]
// The review stage in hand was started on its backup because its first choice's plan was running low.
Object.assign(outcomes[0].sessions[0].stages[3],{balanced:'Claude Code (opus)’s plan had 12% left, so this stage runs on Claude Code (sonnet), which had 60% left.',first:{tool:'claude',model:'opus'},backup:{tool:'claude',model:'sonnet'}})
const story=[{id:9,at:now-60000,source:'gaffer',action:'gaffer-stage-started',detail:JSON.stringify({outcome:'e'.repeat(32),pipeline:'a',stage:'review',reason:'The review stage is under way.'})},{id:8,at:now-120000,source:'gaffer',action:'gaffer-stage-handed-over',detail:JSON.stringify({outcome:'e'.repeat(32),pipeline:'a',stage:'code',by:'gaffer',reason:'x'})}]
const projects=[{id:'p1',host:'a1',machine:'workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:0,version:1,createdAt:now,updatedAt:now},{id:'p2',host:'a1',machine:'workshop',dir:'/work/docs-site',name:'docs-site',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:1,version:1,createdAt:now,updatedAt:now}]
// What the machine last read of its agents' plans. Claude Code is on a plan with two windows; Codex is signed in with a key, which has none.
const plans={state:'read',sentence:'',planLimits:true,checkedAt:now,accounts:[{tool:'claude',state:'signed_in',method:'subscription',account:'sam@example.test',plan:'max',limitsState:'read',limitsAt:now,limits:[{kind:'session',usedPercent:19,resetsAt:now+130*60000},{kind:'week',usedPercent:88,resetsAt:now+3*86400000+3600000}]},{tool:'codex',state:'signed_in',method:'api_key',limitsState:'unsupported'}]}
// The account's saved crew gives the light tier a backup; an added pipeline takes it for its light stages.
const savedCrew={crew:{heavy:{},standard:{},light:{tool:'claude',backup:{tool:'codex',model:'gpt-small'}},updatedAt:now},saved:true,tiers:[],defaults:{}}
const routesFor=box=>({'/api/v2/gaffer/crew':savedCrew,'/api/v2/machines/a1/accounts':plans,'/api/v2/settings/yolo':{allowed:false},'/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},'/api/v2/terminal/sessions':{sessions},'/api/v2/gaffer/goals':{goals,readAt:now},'/api/v2/gaffer/outcomes':{outcomes},'/api/v2/folder-projects':{projects,savedWaiting:0},'/api/v2/gaffer/conversations':r=>new URL(r.url()).searchParams.get('all')==='true'?{conversations:[general.conversation]}:general,
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/history`]:{events:story},
 [`/api/v2/gaffer/outcomes/${'e'.repeat(32)}/automation`]:request=>{const input=request.postDataJSON();box.who.push(input.on);outcomes[0].automation=input.on;return outcomes[0]},
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
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[],who:[]}
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
      // The project list is the page's left column on a desktop, as on Home. Choosing a project narrows the page to its
      // outcomes, and the outcome dialog then opens on that project; the card's + does the same in one press.
      const projectRail=page.getByRole('complementary',{name:'Projects'})
      if(width>=1100){
        await expect(projectRail).toBeVisible()
        await projectRail.getByRole('button',{name:/^docs-site, /}).click()
        await expect(page.locator('.scope')).toContainText('Showing docs-site. A new outcome starts in this project.')
        await expect(page.getByRole('heading',{name:'Nothing in docs-site yet.',exact:true})).toBeVisible()
        await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0)
        // Other sessions follow the chosen project: one in another project, or in none, is left out.
        const others=page.getByRole('region',{name:'Other sessions',exact:true})
        await page.getByRole('button',{name:/^Other sessions/}).click()
        await expect(others).toContainText('Only those in docs-site are listed.')
        await expect(others.getByRole('heading',{name:'Login tokens',exact:true})).toHaveCount(0)
        await expect(others).not.toContainText('Docs refresh')
        await page.getByRole('button',{name:'Give Gaffer an outcome in docs-site',exact:true}).click()
        let asking=page.getByRole('dialog',{name:'Give Gaffer an outcome',exact:true})
        await expect(asking.getByLabel('Project',{exact:true})).toHaveValue('p2')
        await asking.getByRole('button',{name:'Close Give Gaffer an outcome',exact:true}).click()
        await projectRail.getByRole('button',{name:/^billing, /}).click()
        await expect(page.getByText('3 waiting for you · 1 working · 1 finished',{exact:true})).toBeVisible()
        await expect(others.getByRole('heading',{name:'Login tokens',exact:true})).toBeVisible()
        await expect(others).not.toContainText('Docs refresh')
        await page.getByRole('button',{name:'Show all projects',exact:true}).click()
        await expect(page.locator('.scope')).toHaveCount(0)
        await projectRail.getByRole('button',{name:'New outcome in billing',exact:true}).click()
        asking=page.getByRole('dialog',{name:'Give Gaffer an outcome',exact:true})
        await expect(asking.getByLabel('Project',{exact:true})).toHaveValue('p1')
        await asking.getByRole('button',{name:'Close Give Gaffer an outcome',exact:true}).click()
        await page.getByRole('button',{name:/^Other sessions/}).click()
      } else await expect(projectRail).toHaveCount(0)
      // What works by itself is a section of its own, closed at first, whose header still names what is in it. Opened,
      // each pipeline shows every stage and where it stands.
      const wide=width>=1100,oe='e'.repeat(32),od='d'.repeat(32)
      const workingHead=page.getByRole('button',{name:/^Working/})
      await expect(workingHead).toHaveAttribute('aria-expanded','false');await expect(workingHead).toContainText('Export API')
      await workingHead.click()
      const apiItem=page.getByRole('region',{name:'Working',exact:true}).locator(`#pipeline-${oe}-a`)
      const foldedStages=apiItem.getByRole('list',{name:'Stages of Export API',exact:true})
      await expect(foldedStages.getByRole('listitem')).toHaveCount(8)
      await expect(foldedStages.getByRole('listitem')).toContainText(['Plan','Design','Code','Review','Test','PR','Babysit','Merge'])
      await expect(apiItem).toContainText(wide?'Review · 4 of 8 · next Test':'Review in hand · Plan, Design, Code done · next Test')
      // The sections the person opened are still open after a reload.
      await page.reload()
      await expect(page.getByRole('button',{name:/^Working/})).toHaveAttribute('aria-expanded','true')
      // One line of counts says who has to act. A pipeline that failed to start waits for the person too.
      await expect(page.getByText('3 waiting for you · 1 working · 1 finished',{exact:true})).toBeVisible()
      await expect(page.getByRole('list',{name:'Factory at a glance',exact:true})).toHaveCount(0)
      // Every pipeline that waits for the person is an item of Waiting for you, across outcomes.
      const waiting=page.getByRole('region',{name:'Waiting for you',exact:true}),waits=waiting.locator('[id^="pipeline-"]')
      await expect(waits).toHaveCount(3)
      await expect(waits.nth(0)).toContainText('Decision');await expect(waits.nth(0)).toContainText('Export button')
      await expect(waits.nth(1)).toContainText('Ready to merge');await expect(waits.nth(1)).toContainText('Export docs')
      await expect(waits.nth(2)).toContainText('Failed');await expect(waits.nth(2)).toContainText('Importer docs')
      if(!wide){
        // A card's filled button is the choice marked first, Gaffer's advice is said in the detail's words and marks it,
        // and the merge decision says what it waits for.
        await expect(waits.nth(0).locator('.btn.primary')).toHaveText('Tell the agent Recommended')
        await expect(waits.nth(0)).toContainText('Gaffer recommends: Tell the agent. The design already names the empty state; the agent only needs telling which.')
        await expect(waits.nth(1)).toContainText('Nothing merges until you approve it.');await expect(waits.nth(1).locator('.btn.primary')).toHaveText('Approve the merge')
      }
      // Waits until either the list or a detail is on screen, then goes back to the list if a detail is.
      const toList=async()=>{
        if(wide)return
        const back=page.getByRole('button',{name:'‹ Factory',exact:true}),title=page.getByRole('heading',{name:'Factory',level:1,exact:true})
        await expect(back.or(title)).toBeVisible()
        if(await back.isVisible()){await back.click();await expect(title).toBeVisible()}
      }
      // Opens one pipeline: beside the list on a wide screen, as its own screen from Details on a narrow one.
      const choose=async(section,title,chip)=>{
        await toList()
        const head=page.getByRole('button',{name:new RegExp(`^${section}`)})
        if(await head.getAttribute('aria-expanded')!=='true')await head.click()
        const region=page.getByRole('region',{name:section,exact:true})
        await region.getByRole('button',wide?{name:`${chip}: ${title}`,exact:true}:{name:`Details and other actions for ${title}`,exact:true}).click()
        const detail=page.getByRole('region',{name:title,exact:true})
        await expect(detail.getByRole('heading',{name:title,level:2,exact:true})).toBeVisible()
        return detail
      }
      // Opens one of the detail's closed rows.
      const row=async(detail,name)=>{
        const it=detail.locator('details.row').filter({has:page.locator('summary .label').getByText(name,{exact:true})})
        if(await it.getAttribute('open')===null)await it.locator('summary').click()
        return it
      }
      const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
      // The first pipeline waiting for the person is open beside the list on arrival; on a phone, Details opens it.
      let button
      if(wide){button=page.getByRole('region',{name:'Export button',exact:true});await expect(waiting.getByRole('button',{name:'Decision: Export button',exact:true})).toHaveAttribute('aria-pressed','true')}
      else button=await choose('Waiting for you','Export button','Decision')
      await expect(button.getByRole('heading',{name:'Export button',level:2,exact:true})).toBeVisible()
      await expect(button).toContainText('Ship CSV export for billing: an endpoint, a button and tests · pipeline 2 of 3')
      // It says what it needs and offers what to do: the choice Gaffer's review recommended comes first, marked, with its reason.
      const needy=button.getByRole('region',{name:'What is waiting',exact:true})
      await expect(needy).toContainText('Gaffer brought a question back to you.')
      await expect(bar.getByRole('button')).toHaveText(['Tell the agent… Recommended','Have Gaffer look again','Move to next stage','Send back…'])
      await expect(needy).toContainText('Gaffer recommends: Tell the agent. The design already names the empty state; the agent only needs telling which.')
      await expect(needy).toContainText('Which of the two do you want?')
      await expect(needy).toContainText('It would say: “Use the illustrated empty state from the design.”')
      // The stage it waits in is the one whose facts are shown.
      await expect(button.getByRole('region',{name:/^(Code|Review|Test|PR) stage$/})).toBeVisible()
      // Taking Gaffer's recommendation starts from the words it would use; they are the person's to change, and nothing is sent yet.
      await bar.getByRole('button',{name:'Tell the agent… Recommended',exact:true}).click()
      const suggest=page.getByRole('dialog',{name:'Tell the agent of “Export button”',exact:true})
      await expect(suggest.getByRole('textbox',{name:'Instruction',exact:true})).toHaveValue('Use the illustrated empty state from the design.')
      await suggest.getByRole('button',{name:'Cancel',exact:true}).click();expect(box.told).toEqual([])
      // What Gaffer decided earlier is kept with what had stopped the stage, what it did and why.
      const decided=await row(button,'What Gaffer decided')
      await expect(decided.locator('summary')).toContainText('2 decisions')
      const took=decided.getByRole('list',{name:'What Gaffer decided, newest first',exact:true}).getByRole('listitem')
      await expect(took).toHaveCount(2);await expect(took.nth(1)).toContainText('Export API · Code')
      await expect(took.nth(1)).toContainText('Gaffer’s review brought the stage back. Moved to the next stage. The endpoint and its tests are in the handover; nothing more belongs in this stage.')
      // Newest first: the agent's question Gaffer answered, with what was asked and what it answered.
      await expect(took.first()).toContainText('The agent asked a multiple-choice question. Answered it. The brief says the export must not load the primary.')
      await expect(took.first()).toContainText('Asked: Which database should the export read from? Answered: The read replica')
      await decided.locator('summary').click()
      // The person can give Gaffer the decisions, and take them back, in Settings for this outcome. A merge stays theirs either way.
      const settings=await row(button,'Settings for this outcome')
      const decider=settings.getByRole('radiogroup',{name:'Who decides when a stage stops',exact:true})
      await expect(decider.getByRole('radio',{name:'You decide',exact:true})).toBeChecked()
      await decider.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
      await expect(page.getByRole('status').filter({hasText:'Gaffer takes'})).toContainText('Merges, approvals and questions that need your own words still come to you.')
      await expect(needy).toContainText('Gaffer is deciding this. You can still choose first.')
      await expect(settings).toContainText('at most 3 times for a stage')
      await expect(settings.locator('summary')).toContainText('Gaffer decides · Ask me · merge waits for you')
      // Nothing in the detail runs sideways: on a wide screen within its pane, on a narrow one within the window.
      expect(await (wide?page.locator('.pane').evaluate(el=>el.scrollWidth<=el.clientWidth):page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))).toBe(true)
      await page.screenshot({path:`${shots}/gaffer-factory-decisions-${width}-${theme}.png`})
      const docs=await choose('Waiting for you','Export docs','Ready to merge')
      await expect(docs.getByRole('region',{name:'What is waiting',exact:true})).toContainText('Nothing merges until you approve it.')
      await expect(docs.getByRole('region',{name:'What is waiting',exact:true})).not.toContainText('Gaffer is deciding')
      await (await row(docs,'Settings for this outcome')).getByRole('radio',{name:'You decide',exact:true}).check()
      await expect(page.getByRole('status').filter({hasText:'You take'})).toHaveText('You take this outcome’s decisions again.');expect(box.who).toEqual([true,false])
      // Those two switches are the only writes so far.
      expect(box.writes.splice(0)).toEqual(Array(2).fill(`POST /api/v2/gaffer/outcomes/${oe}/automation`))
      // With a machine, a project and a provider in place there is nothing left to set up.
      await expect(page.getByRole('region',{name:'Set up your factory',exact:true})).toHaveCount(0)
      // A pipeline shows every stage: done, in hand and to come, and who is doing the one in hand.
      const api=await choose('Working','Export API','Running')
      const stages=api.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem')
      await expect(stages).toHaveCount(8)
      await expect(stages.nth(2)).toContainText('done');await expect(stages.nth(3)).toHaveAttribute('aria-current','step');await expect(stages.nth(3)).toContainText('in hand');await expect(stages.nth(4)).toContainText('to come')
      const inHand=api.getByRole('region',{name:'Review stage',exact:true})
      await expect(inHand).toContainText('Claude Code · opus');await expect(inHand).not.toContainText('stage ·')
      // A stage that ran on its backup says so, and why.
      await expect(inHand).toContainText('BalancedClaude Code (opus)’s plan had 12% left, so this stage runs on Claude Code (sonnet), which had 60% left.')
      await expect(inHand).toContainText('The review found one missing error case')
      // Picking another stage shows who did it, how demanding it was judged to be, and Gaffer's reason where the judgment was its own.
      await stages.nth(2).getByRole('button').click()
      const coded=api.getByRole('region',{name:'Code stage',exact:true})
      await expect(coded).toContainText('Heavy stage');await expect(coded).toContainText('Gaffer’s assessment: The export reads the billing ledger while it is being written.')
      await stages.nth(7).getByRole('button').click()
      const later=api.getByRole('region',{name:'Merge stage',exact:true})
      await expect(later).toContainText('Light stage');await expect(later).toContainText('will do this stage');await expect(later).not.toContainText('Gaffer’s assessment')
      await stages.nth(3).getByRole('button').click();await expect(inHand).toBeVisible()
      // What the pipeline has used and has left, and what the agents themselves reported, are in Usage and budget.
      const usage=await row(api,'Usage and budget')
      await expect(usage).toContainText('Reviews left9 of 12');await expect(usage).toContainText(/Time left\d+ h \d+ min/);await expect(usage).toContainText('Gaffer tokens2,252')
      await expect(usage).toContainText('Agent tokens97k · 410k cached');await expect(usage).toContainText('Context left12% · 24k of 200k')
      await expect(await row(api,'Settings for this outcome')).toContainText('Waits for you')
      const buttonAgain=await choose('Waiting for you','Export button','Decision'),buttonUsage=await row(buttonAgain,'Usage and budget')
      await expect(buttonUsage).toContainText('Agent tokensNot reported');await expect(buttonUsage).toContainText('Context leftNot reported')
      // A finished pipeline that was allowed to merge says so.
      const finished=await choose('Finished or stopped','Importer retry','Finished')
      await expect(finished).toContainText('Finished');await expect(await row(finished,'Settings for this outcome')).toContainText('By itself when green')
      // A pipeline's own goal is not listed again among the other sessions; a session started by hand is.
      await toList()
      const othersHead=page.getByRole('button',{name:/^Other sessions/})
      await expect(othersHead).toContainText('1 followed · 1 not followed')
      await othersHead.click()
      const floor=page.locator('[data-floor]')
      await expect(floor.getByRole('list',{name:'Sessions Gaffer looks after',exact:true}).getByRole('listitem')).toHaveCount(1)
      await expect(floor.getByRole('heading',{name:'Login tokens',exact:true})).toBeVisible()
      await expect(floor.getByRole('group',{name:'Running chats not followed',exact:true}).getByRole('checkbox')).toHaveCount(1)
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      await floor.getByRole('heading',{name:'Login tokens',exact:true}).scrollIntoViewIfNeeded()
      await page.screenshot({path:`${shots}/gaffer-factory-${width}-${theme}.png`,fullPage:width<500})
      await othersHead.click()
      expect(box.writes).toEqual([])
      // Merging is the person's decision: it is asked for plainly, can be backed out of, and is sent once.
      const merge=await choose('Waiting for you','Export docs','Ready to merge')
      await expect(merge.getByRole('list',{name:'Choose a stage of Export docs',exact:true}).getByRole('listitem').nth(7)).toContainText('waiting for you')
      await bar.getByRole('button',{name:'Approve the merge… Recommended',exact:true}).click()
      const confirm=page.getByRole('dialog',{name:'Merge “Export docs”?',exact:true})
      await expect(confirm).toContainText('once every check passes, no review thread is open and there are no conflicts')
      await confirm.getByRole('button',{name:'Cancel',exact:true}).click();await expect(confirm).toHaveCount(0);expect(box.writes).toEqual([])
      await bar.getByRole('button',{name:'Approve the merge… Recommended',exact:true}).click()
      await confirm.getByRole('button',{name:'Approve merge',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Merge approved'})).toContainText('Merge approved for “Export docs”.')
      expect(box.merges).toEqual([{revision:5,confirmed:true}])
      // It no longer waits: the detail stays on it, and it has moved to Working.
      await expect(bar.getByRole('button',{name:/Approve/})).toHaveCount(0)
      await expect(merge.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
      await expect(await row(merge,'Settings for this outcome')).toContainText('You approved it')
      if(!wide)await toList()
      await expect(page.getByText('2 waiting for you · 2 working · 1 finished',{exact:true})).toBeVisible()
      // The outcome's own figures: what Gaffer's model has used so far, and the budget each stage gets. No prices.
      const outcome=await choose('Working','Export docs','Running')
      const totals=(await row(outcome,'Usage and budget')).getByLabel('This outcome so far',{exact:true})
      await expect(totals).toContainText('Pipelines3');await expect(totals).toContainText('Gaffer reviews6');await expect(totals).toContainText('Gaffer tokens4,504');await expect(totals).toContainText('Budget per stage12 reviews · 24 h');await expect(totals).toContainText('Agent tokens97k · 410k cached (Codex 85k, Claude Code 12k)')
      await expect(await row(outcome,'Settings for this outcome')).toContainText('ApprovalsAsk me')
      // A pipeline that never started waits for the person, says why, and can be tried again, once, on request: from its
      // item on a narrow screen, from its detail on a wide one.
      let stopped
      if(wide){stopped=await choose('Waiting for you','Importer docs','Failed');await expect(stopped.getByRole('region',{name:'What is waiting',exact:true})).toContainText('That agent isn’t installed on that machine.');await bar.getByRole('button',{name:'Try again',exact:true}).click()}
      else{await toList();stopped=waiting.locator(`#pipeline-${od}-b`);await expect(stopped).toContainText('Not started: That agent isn’t installed on that machine.');await stopped.getByRole('button',{name:'Try again',exact:true}).click()}
      await expect(page.getByRole('status').filter({hasText:'Trying'})).toHaveText('Trying “Importer docs” again.')
      expect(box.retries).toEqual([{revision:2}]);await expect(page.getByRole('button',{name:'Try again',exact:true})).toHaveCount(0)
      // Gaffer reports across the whole outcome when asked, and may propose pipelines; nothing is added until approved.
      const reported=await choose('Waiting for you','Export button','Decision'),report=await row(reported,'Gaffer’s report')
      await expect(report.locator('summary')).toContainText('Not asked for yet')
      await report.getByRole('button',{name:'Ask Gaffer for a report',exact:true}).click()
      await expect(report).toContainText('Nothing covers the changelog.');await expect(report.locator('summary')).toContainText('2 pipelines proposed')
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
      // A crew can do its stages: the shipping stage goes back to the outcome's own agent, and the level this
      // machine reports for it travels with that stage.
      await add.getByText('Choose who does what',{exact:true}).click()
      await expect(add.getByRole('combobox',{name:'Standard agent for Changelog entry',exact:true})).toHaveValue('codex')
      await add.getByRole('combobox',{name:'Light agent for Changelog entry',exact:true}).selectOption('claude')
      await add.getByRole('combobox',{name:'Light effort for Changelog entry',exact:true}).selectOption('low')
      await expect(add.getByText('2 stages, each in an agent session of its own.',{exact:false})).toBeVisible()
      // The light tier's backup comes from the saved crew and is said, not edited here.
      const lightSpare=add.getByRole('group',{name:'Light backup for Changelog entry',exact:true}),lightAgent=lightSpare.getByRole('combobox',{name:'Light backup agent for Changelog entry',exact:true})
      await expect(lightAgent).toHaveValue('crew');await expect(lightAgent.locator('option:checked')).toHaveText('From your crew: Codex · gpt-small')
      // It can be left off this pipeline, or replaced with a backup of this pipeline's own, and put back.
      await expect(lightSpare.getByRole('combobox')).toHaveCount(1)
      await lightAgent.selectOption('');await expect(lightSpare.getByRole('combobox')).toHaveCount(1)
      await lightAgent.selectOption('codex')
      await expect(lightSpare.getByRole('combobox',{name:'Light backup model for Changelog entry',exact:true})).toBeVisible()
      await expect(lightSpare.getByRole('combobox',{name:'Light backup effort for Changelog entry',exact:true})).toBeVisible()
      await lightAgent.selectOption('crew');await expect(lightSpare.getByRole('combobox')).toHaveCount(1)
      // A tier with nothing saved starts with no backup and can be given one here.
      await expect(add.getByRole('combobox',{name:'Standard backup agent for Changelog entry',exact:true})).toHaveValue('')
      // A stage of an added pipeline can be split into parts too; undone here, so the pipeline is added whole.
      await add.getByRole('button',{name:'Split Code of Changelog entry into parts',exact:true}).click()
      await expect(add.getByRole('group',{name:'Code parts in Changelog entry',exact:true}).getByRole('group',{name:/^Part \d of Code in Changelog entry$/})).toHaveCount(2)
      await expect(add.getByRole('button',{name:'Add pipeline',exact:true})).toBeDisabled()
      await add.getByRole('button',{name:'Do Code in Changelog entry as one stage',exact:true}).click()
      await expect(add.getByRole('button',{name:'Add pipeline',exact:true})).toBeEnabled()
      await expect(add.getByRole('group',{name:'Standard crew for Changelog entry',exact:true})).not.toContainText('Backup from your crew')
      expect(await add.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await add.screenshot({path:`${shots}/gaffer-factory-add-${width}-${theme}.png`})
      await add.getByRole('button',{name:'Add pipeline',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Added'})).toHaveText('Added “Changelog entry”. The controller is starting it.')
      expect(box.added).toEqual([{revision:7,confirmed:true,sessions:[{title:'Changelog entry',prompt:'Add a changelog entry for CSV export. Do not merge.',stages:[{kind:'code',tool:'codex',model:'gpt-5-codex',effort:'low',tier:'standard'},{kind:'pr',tool:'claude',model:'',effort:'low',tier:'light',backup:{tool:'codex',model:'gpt-small'}}]}]}])
      await expect(reported).toContainText('pipeline 2 of 4');await expect(report.getByRole('list',{name:'Proposed pipelines',exact:true})).toHaveCount(0)
      await expect((await row(reported,'Usage and budget')).getByLabel('This outcome so far',{exact:true})).toContainText('Gaffer tokens4,884')
      expect(box.reviews).toEqual([{revision:6}])
      // The person runs the line: they can move a pipeline on from the stage in hand without waiting for Gaffer…
      await bar.getByRole('button',{name:'Move to next stage',exact:true}).click()
      const move=page.getByRole('dialog',{name:/^Move “Export button” on to (review|test|pr)\?$/})
      await expect(move).toContainText('This is your call');await expect(move).toContainText('Nothing is merged by this.')
      // The move names the stage the person was shown, so a pipeline that left it meanwhile is not moved again.
      const seen=kinds[outcomes[0].sessions[1].stage]
      expect(box.moved).toEqual([])
      await move.getByRole('button',{name:'Move on',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Moved'})).toHaveText('Moved “Export button” on. The controller is starting its next stage.')
      expect(box.moved).toEqual([{confirmed:true,stage:seen}])
      // …and its stage in hand shows no applied choice where none was read back.
      await expect(reported.getByRole('region',{name:/ stage$/})).toBeVisible()
      await expect(reported.getByRole('region',{name:/ stage$/})).not.toContainText('Applied')
      // …and decide, per pipeline and at any time, whether its merge waits for them.
      const line=await choose('Working','Export API','Running'),own=await row(line,'Settings for this outcome')
      await own.getByRole('button',{name:'Let Export API merge by itself',exact:true}).click()
      await expect(own).toContainText('By itself when green');expect(box.modes).toEqual([{on:true}])
      await expect(own.locator('summary')).toContainText('merges by itself')
      await own.getByRole('button',{name:'Make Export API wait for you before merging',exact:true}).click()
      await expect(own).toContainText('Waits for you');expect(box.modes).toEqual([{on:true},{on:false}])
      // The detail says who closed each stage, how long the stage in hand has run and what its agent last reported…
      const lineStages=line.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem')
      await expect(lineStages.filter({hasText:'Plan'})).toContainText('moved on by you, not handed over by Gaffer')
      await expect(lineStages.filter({hasText:'Design'})).not.toContainText('moved on by you')
      const review=line.getByRole('region',{name:'Review stage',exact:true})
      await expect(review).toContainText('In this stage');await expect(review).toContainText('42 min')
      await expect(review).toContainText('Working · last active 20 min ago')
      // …and whether the agent, model and effort chosen for a stage were applied, as read back from the machine: a
      // stage on something else says what, and is never shown as confirmed.
      await expect(review).toContainText('Not as chosen. You chose the effort high; the session is on low.')
      await expect(review).toContainText('high effort')
      await lineStages.nth(2).getByRole('button').click()
      await expect(line.getByRole('region',{name:'Code stage',exact:true})).toContainText('Confirmed. The session is on gpt-coder · medium effort. The model and effort were left to the agent.')
      await lineStages.nth(3).getByRole('button').click()
      // What is left of the plan of the agent doing the stage in hand: every window, marked when one is nearly spent.
      // (An agent with no plan to read shows no line: agentAccounts.test.mjs.)
      const planLine=review.locator('dl.facts').first().locator('div').filter({hasText:'Plan left'}).locator('dd'),planChips=planLine.locator('button.strip')
      await expect(planChips.locator('.words')).toHaveText('Session 81% left · Week 12% left');await expect(planLine.locator('.chip')).toHaveClass(/low/)
      // A click on it opens every limit with its reset and the account, over the detail and inside the window.
      await planChips.click()
      const planPanel=page.getByRole('dialog',{name:'Plan limits',exact:true})
      await expect(planPanel).toContainText('Claude Code');await expect(planPanel).toContainText('Max');await expect(planPanel).toContainText('sam@example.test')
      await expect(planPanel.getByRole('meter',{name:'Week left',exact:true})).toHaveAttribute('aria-valuenow','12')
      await expect(planPanel).toContainText('Read on workshop')
      await page.keyboard.press('Escape');await expect(planPanel).toBeHidden()
      // …and the outcome's history is read from the activity log only when asked for, in its own row.
      const history=await row(line,'History')
      await expect(history).toContainText("A pipeline's review stage started");await expect(history).toContainText("Gaffer handed over a pipeline's code stage");await expect(history).toContainText('Export API')
      await expect(history.locator('summary')).toContainText('2 events · last')
      await history.locator('summary').click();await expect(history.getByText("A pipeline's review stage started")).toBeHidden()
      // They can tell a pipeline's agent what to do next, in their own words…
      await bar.getByRole('button',{name:'Tell the agent…',exact:true}).click()
      const tell=page.getByRole('dialog',{name:'Tell the agent of “Export API”',exact:true})
      await expect(tell.getByRole('textbox',{name:'Instruction',exact:true})).toHaveValue('')
      await expect(tell.getByRole('button',{name:'Send to the agent',exact:true})).toBeDisabled()
      await tell.getByRole('textbox',{name:'Instruction',exact:true}).fill('  Add the missing error case test.  ')
      await tell.getByRole('button',{name:'Send to the agent',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'instruction'})).toHaveText('Your instruction is with the agent of “Export API” for its next turn.')
      expect(box.told).toHaveLength(1);expect(box.told[0].text).toBe('Add the missing error case test.');expect(box.told[0].requestId).toMatch(/^[a-f0-9]{32}$/)
      // …and send it back to the stage in hand or an earlier one, with the reason. Its code stage was split into two
      // parts: a split stage is offered once, since sending back does the whole stage again from its first part.
      const piece=outcomes[0].sessions[0],at=piece.stages.findIndex(stage=>stage.kind==='code'),whole=piece.stages[at]
      piece.stages.splice(at,1,{...whole,part:'Ledger query',brief:'Write the query.'},{...whole,part:'Download button',brief:'Add the button.'});piece.stage++
      // The page reads the floor again every few seconds; the detail then shows both parts as steps of their own.
      await expect(line.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem')).toHaveCount(9,{timeout:20000})
      await bar.getByRole('button',{name:'Send back…',exact:true}).click()
      const back=page.getByRole('dialog',{name:'Send “Export API” back',exact:true})
      await expect(back).toContainText('Nothing already committed is undone.')
      const again=back.getByRole('combobox',{name:'Stage to do again',exact:true})
      await expect(again.getByRole('option')).toHaveText(['Plan','Design','Code','Review']);await expect(again).toHaveValue('review')
      await again.selectOption('code');await back.getByRole('textbox',{name:'Why it is going back',exact:true}).fill('The export leaks other tenants’ rows.')
      expect(await back.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await back.getByRole('button',{name:'Send back',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Sent'})).toHaveText('Sent “Export API” back to code. The controller is starting that stage again.')
      expect(box.back).toEqual([{stage:'code',reason:'The export leaks other tenants’ rows.',confirmed:true}])
      // The fixture is shared with the passes that follow: the stage is one again for them.
      piece.stages.splice(at,2,whole);piece.stage--
      expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
    } finally { await w.context.close() }
  }
  // A finished outcome's pipelines sit in Finished or stopped, and the outcome can be archived (kept, hidden, restorable) or deleted (after a confirmation); the account's default only picks which comes first.
  for(const [width,mode] of [[1280,'archive'],[390,'delete']]){
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[],who:[]}
    const finished={...structuredClone(outcomes[1]),id:'c'.repeat(32),revision:3,objective:'Tidy the importer logs.',report:{text:'Everything is merged. '.repeat(60),at:now,proposals:[],inputTokens:1,outputTokens:1,usageReported:true},sessions:[{key:'a',title:'Importer logs',prompt:'p',state:'done',note:'Every stage is done.',sessionId:'s9',conversationId:'9'.repeat(32),updatedAt:now,stage:7,stages:line(7,false)}]}
    const list=[finished]
    const routes={...routesFor(box),'/api/v2/gaffer/outcomes':()=>({outcomes:list}),
      '/api/v2/profile':request=>{if(request.method()==='PATCH')return {user:{},profile:{}};return {user:{id:1,name:'Owner',email:'o@example.test',role:'owner'},sessionCount:1,accounts:true,profile:{theme:'system',timezone:'Europe/London',dateFormat:'',shelfView:'list',cardsExpanded:false,factoryRemoveMode:mode,hostedPrefix:'reet-',updatedAt:now}}},
      [`/api/v2/gaffer/outcomes/${'c'.repeat(32)}/archive`]:request=>{box.writes.push('archive');box.archived=request.postDataJSON();finished.archivedAt=now;finished.revision++;return finished},
      [`/api/v2/gaffer/outcomes/${'c'.repeat(32)}/restore`]:request=>{box.writes.push('restore');delete finished.archivedAt;finished.revision++;return finished},
      [`/api/v2/gaffer/outcomes/${'c'.repeat(32)}`]:request=>{if(request.method()!=='DELETE')return finished;box.deleted=new URL(request.url()).search;list.length=0;return {}}}
    const w=await openWorkspace(browser,{width,height:width<500?844:1100,terminalMachines:machines,routes})
    try {
      const page=w.page
      await page.goto(base+'/factory')
      // Finished work needs nobody: it sits in Finished or stopped, closed at first, and its header names it. With nothing
      // waiting or working, the wide screen's detail waits for a pipeline to be chosen.
      const wide=width>=1100
      await expect(page.getByText('0 waiting for you · 0 working · 1 finished',{exact:true})).toBeVisible()
      const head=page.getByRole('button',{name:/^Finished or stopped/})
      await expect(head).toHaveAttribute('aria-expanded','false');await expect(head).toContainText('Importer logs')
      if(wide)await expect(page.getByText('Nothing is waiting for you or working. Choose a pipeline in the list to see it here.',{exact:true})).toBeVisible()
      const card=page.getByRole('region',{name:'Importer logs',exact:true})
      const open=async()=>{
        if(await head.getAttribute('aria-expanded')!=='true')await head.click()
        await page.getByRole('region',{name:'Finished or stopped',exact:true}).getByRole('button',wide?{name:'Finished: Importer logs',exact:true}:{name:'Details and other actions for Importer logs',exact:true}).click()
        await expect(card.getByRole('heading',{name:'Importer logs',level:2,exact:true})).toBeVisible()
      }
      await open()
      // Gaffer's report is one of the detail's closed rows; opened, the long report is clamped until it is read in full.
      const report=card.locator('details.row').filter({has:page.locator('summary .label').getByText('Gaffer’s report',{exact:true})})
      await expect(report.locator('summary')).toContainText('written ')
      await report.locator('summary').click()
      expect(await report.locator('.report-clamp').evaluate(el=>el.getBoundingClientRect().height)).toBeLessThan(180)
      await report.getByRole('button',{name:'Read all of it',exact:true}).click()
      await expect(report.getByRole('button',{name:'Show less',exact:true})).toBeVisible()
      // Archive and Delete are both in the outcome's menu; the account's default comes first, and Delete is marked.
      const menu=card.getByRole('button',{name:'More for this outcome',exact:true})
      await menu.click()
      await expect(card.locator('.menu ul button')).toHaveText(mode==='archive'?['Archive','Delete']:['Delete','Archive'])
      await expect(card.getByRole('button',{name:'Delete',exact:true})).toHaveClass(/danger/)
      // Archive keeps the record: it leaves the floor and the counts, comes back on request, and can be restored.
      await card.getByRole('button',{name:'Archive',exact:true}).click()
      expect(box.archived).toEqual({revision:3})
      await expect(page.getByRole('status').filter({hasText:'Archived'})).toContainText('Show archived outcomes to bring it back.')
      await expect(page.getByRole('heading',{name:'Nothing on the floor.',exact:true})).toBeVisible()
      await page.getByRole('button',{name:'Show archived (1)',exact:true}).click()
      // On a wide screen the pipeline that was open comes back with its outcome. On a narrow one archiving went back to
      // the list and cleared the address, so it is opened from the list.
      if(!wide){expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull();await open()}
      await expect(card.getByRole('heading',{name:'Importer logs',level:2,exact:true})).toBeVisible()
      await expect(card).toContainText('archived');if(wide)await expect(page.getByText('0 waiting for you · 0 working · 0 finished',{exact:true})).toBeVisible()
      // An archived outcome's pipeline offers no action: no bar in its detail, no request for a report, and in the list
      // only its Details.
      await expect(card.getByRole('group',{name:'What do you want to do?',exact:true})).toHaveCount(0)
      await expect(card.getByRole('button',{name:/^Ask Gaffer/})).toHaveCount(0)
      if(!wide){
        await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
        if(await head.getAttribute('aria-expanded')!=='true')await head.click()
        const item=page.locator(`#pipeline-${'c'.repeat(32)}-a`)
        await expect(item.getByRole('button',{name:'Details and other actions for Importer logs',exact:true})).toBeVisible();await expect(item.getByRole('button')).toHaveCount(1)
        await item.getByRole('button',{name:'Details and other actions for Importer logs',exact:true}).click()
        await expect(card.getByRole('heading',{name:'Importer logs',level:2,exact:true})).toBeVisible()
      }
      await menu.click();await expect(card.locator('.menu ul button')).toHaveText(['Restore'])
      await card.getByRole('button',{name:'Restore',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Restored'})).toBeVisible()
      await expect(page.getByRole('button',{name:/archived/})).toHaveCount(0)
      // Delete asks first, says what it does not touch, and names the revision it means.
      await menu.click();await card.getByRole('button',{name:'Delete',exact:true}).click()
      const ask=page.getByRole('dialog',{name:'Delete “Tidy the importer logs.”?',exact:true})
      await expect(ask).toContainText('not touched');await ask.getByRole('button',{name:'Cancel',exact:true}).click()
      expect(box.deleted).toBeUndefined()
      await menu.click();await card.getByRole('button',{name:'Delete',exact:true}).click()
      await page.getByRole('dialog',{name:'Delete “Tidy the importer logs.”?',exact:true}).getByRole('button',{name:'Delete outcome',exact:true}).click()
      await expect(page.getByRole('status').filter({hasText:'Deleted'})).toBeVisible()
      expect(box.deleted).toBe('?revision=5')
      await expect(page.getByRole('heading',{name:'Put Gaffer on the job.',exact:true})).toBeVisible()
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A new installation is told what Gaffer still needs, each with the place to do it, and has no pipelines yet.
  {
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[],who:[]}
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
      await expect(page.getByRole('heading',{name:'Put Gaffer on the job.',exact:true})).toBeVisible()
      await expect(page.locator('.counts')).toHaveCount(0)
      expect(box.writes).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // How Gaffer works is set once for the account: looking after sessions is pre-approved, and pipelines do not merge by themselves, until changed.
  {
    const box={writes:[],merges:[],reviews:[],added:[],retries:[],moved:[],modes:[],told:[],back:[],who:[],crews:[]}
    const w=await openWorkspace(browser,{width:1280,height:1000,routes:{...routesFor(box),'/api/v2/gaffer/crew':request=>{if(request.method()==='PUT'){box.crews.push(request.postDataJSON());return {crew:{...box.crews.at(-1),updatedAt:now},saved:true,tiers:[],defaults:{}}}return {crew:{light:{},standard:{},heavy:{}},saved:false,tiers:[],defaults:{}}}}})
    try {
      const page=w.page
      await page.goto(base+'/account/gaffer')
      const ways=page.getByRole('region',{name:'How Gaffer works',exact:true})
      const standing=ways.getByRole('checkbox',{name:/^Look after sessions without asking each time/}),auto=ways.getByRole('checkbox',{name:/^Let new pipelines merge by themselves/})
      await expect(standing).toBeChecked();await expect(auto).not.toBeChecked()
      await expect(ways).toContainText('Nothing is read or sent until you ask')
      await auto.check();await expect.poll(()=>w.state.patches).toEqual([{gafferAutoMerge:true}])
      await standing.uncheck();await expect.poll(()=>w.state.patches).toEqual([{gafferAutoMerge:true},{gafferStanding:false}])
      // The crew is the account's too: who does each tier, its backup, when a backup takes over, and where the outcome dialog starts for who decides.
      const crew=page.getByRole('region',{name:'Your Factory crew',exact:true}),heavy=crew.getByRole('group',{name:'Heavy tier',exact:true})
      await expect(crew).toContainText('Not saved yet: outcomes use their own agent for every stage.')
      await expect(heavy.getByRole('combobox',{name:'Heavy agent',exact:true})).toHaveValue('')
      await expect(heavy.getByRole('textbox')).toHaveCount(0);await expect(heavy).toContainText('Stages of this tier run on whichever agent the outcome is given.')
      await heavy.getByRole('combobox',{name:'Heavy agent',exact:true}).selectOption('claude')
      await heavy.getByRole('textbox',{name:'Heavy model',exact:true}).fill('opus');await heavy.getByRole('textbox',{name:'Heavy effort',exact:true}).fill('high')
      await expect(heavy).toContainText('Stages stay on the first choice.')
      await heavy.getByRole('combobox',{name:'Heavy backup agent',exact:true}).selectOption('codex')
      await heavy.getByRole('textbox',{name:'Heavy backup model',exact:true}).fill('gpt-strong')
      const below=crew.getByRole('group',{name:'When a backup takes over',exact:true}).getByRole('spinbutton')
      await expect(below).toHaveValue('20');await below.fill('3');await below.blur();await expect(below).toHaveValue('5');await below.fill('30')
      const who=crew.getByRole('radiogroup',{name:'Who decides by default',exact:true})
      await expect(who.getByRole('radio',{name:'Ask me',exact:true})).toBeChecked();await who.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
      // A Cline tier can be given the billing source it starts on; any other agent has no such field.
      const light=crew.getByRole('group',{name:'Light tier',exact:true})
      await light.getByRole('combobox',{name:'Light agent',exact:true}).selectOption('cline')
      await light.getByRole('textbox',{name:'Light Cline billing source',exact:true}).fill('openrouter')
      await expect(heavy.getByRole('textbox',{name:/Cline billing source/})).toHaveCount(0)
      // A tier can have a second backup, tried after the first.
      await heavy.getByRole('button',{name:'Add another backup for Heavy',exact:true}).click()
      await heavy.getByRole('combobox',{name:'Heavy backup 2 agent',exact:true}).selectOption('opencode')
      await expect(crew).toContainText('an outcome started any other way, such as over MCP, is never given to Gaffer by this.')
      expect(await crew.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      await crew.screenshot({path:`${shots}/gaffer-crew-settings-1280-light.png`})
      await crew.getByRole('button',{name:'Save crew',exact:true}).click()
      await expect(crew.getByRole('status')).toHaveText('Saved. New outcomes start from this crew.')
      expect(box.crews).toEqual([{heavy:{tool:'claude',model:'opus',effort:'high',backup:{tool:'codex',model:'gpt-strong',effort:''},moreBackups:[{tool:'opencode',model:'',effort:''}]},standard:{},light:{tool:'cline',model:'',effort:'',provider:'openrouter'},balanceBelow:30,gafferDecides:true}])
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Factory page: a navigation tab, one line of counts, sections that open, close and are remembered, pipelines with every stage and who does it, a question waiting on the person, a finished pipeline, other sessions without the pipelines’ own goals, the merge decision asked for and sent once, usage and remaining budgets without prices, a stopped pipeline tried again, Gaffer’s report across an outcome with a proposal approved, setup guidance for a new installation and the account’s two Gaffer defaults passed on desktop, phone and dark.')
} finally { await browser.close() }
