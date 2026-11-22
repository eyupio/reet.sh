import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Factory delivery review: the criteria agreed when a plan is approved, the card that shows each criterion with its
// evidence beside the two decisions, and Home's pointer to it. The API is answered with fixtures that follow
// docs/handoffs/factory-delivery-review-design.md. Accepting the delivery is never the merge approval.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-delivery'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),oid='e'.repeat(32)
const HEAD0='0a0a0a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f',HEAD1='9f2c1ab0d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8',HEAD2='1d4e8c20aa11bb22cc33dd44ee55ff6677889900'
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
const st=(kind,state,extra={})=>({kind,tool:'claude',model:'',state,requestId:'r',...extra})
const project=(id,name,dir,position)=>({id,host:'a1',machine:'workshop',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position,version:1,createdAt:now,updatedAt:now})
const machines=[{agentId:'a1',host:'workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',launch:true,acp:true,acpLiveControls:true,acpStartOptions:true,inspect:true,acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'available',options:[]}]}}]
const TEXTS={c1:'The export downloads as CSV.',c2:'Totals match the ledger.',c3:'Other tenants’ rows are never exported.'}

// One outcome with two pipelines: Export API waits for its delivery review (three criteria), and Importer retry is an
// outcome saved before criteria existed. `box.*` holds what the page sent and what the "machine" currently reports.
function fixture(over={}){
  const box={head:HEAD1,rev:3,state:'delivery_review',reads:0,gets:[],decisions:[],evidence:[],puts:[],merges:[],recorded:[],conflictNext:'',fail502:0,delay:0,readFails:0,pullAtHead:true,commitState:'verified',noNeedsWork:false,criteriaRevision:1,approved:[],...over}
  box.texts={...TEXTS}
  box.ids=['c1','c2','c3']
  const evidence=[
    {id:'e1',criterionId:'c1',kind:'check',name:'test',state:'SUCCESS',verdict:'passed',commit:HEAD1,criteriaRevision:1,url:'https://github.com/acme/billing/actions/runs/1',byName:'Paul',at:now-30*60000},
    ...(over.noNeedsWork?[]:[{id:'e2',criterionId:'c2',kind:'note',verdict:'needs_work',text:'Rounds the pence down.',commit:HEAD1,criteriaRevision:1,byName:'Paul',at:now-20*60000}]),
    {id:'e3',criterionId:'c3',kind:'note',verdict:'passed',text:'Checked by hand on the old commit.',commit:HEAD0,criteriaRevision:1,byName:'Paul',at:now-3*3600000},
    {id:'e4',criterionId:'c3',kind:'check',name:'security',state:'EXPECTED',verdict:'',commit:HEAD1,criteriaRevision:1,unverified:true,byName:'Paul',at:now-10*60000},
  ]
  box.evidence=evidence
  const piece={key:'a',title:'Export API',prompt:'p',state:'delivery_review',note:'Waiting for your delivery review.',sessionId:'s-a',conversationId:'1'.repeat(32),sessionState:'running',updatedAt:now-5*60000,stage:3,
    stages:[st('code','done'),st('review','done'),st('test','done'),st('merge','pending')],criteria:Object.keys(TEXTS).map(id=>({id,text:over.long&&id==='c1'?'x'.repeat(240):TEXTS[id],revision:1})),criteriaRevision:1,reviewRevision:3,deliveries:[]}
  if(over.note)piece.note=over.note
  if(over.long)box.recorded=[{requestId:'a'.repeat(32),decision:'send_back',commit:HEAD0,criteriaRevision:1,byName:'Paul'.repeat(20),at:now-7200000,notes:'n'.repeat(300),stage:'code',statuses:{c1:'needs_work',c2:'passed',c3:'unverified'},failed:['c1']}]
  const other={key:'b',title:'Importer retry',prompt:'p',state:'following',note:'The code stage is under way.',sessionId:'s-b',conversationId:'2'.repeat(32),sessionState:'running',updatedAt:now,stage:0,stages:[st('code','running',{sessionId:'s-b',conversationId:'2'.repeat(32)}),st('merge','pending')]}
  const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'claude',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[piece,other]}
  const statusOf=c=>{
    const live=box.evidence.filter(e=>e.criterionId===c.id&&e.commit===box.head&&!e.unverified)
    return live.some(e=>e.verdict==='needs_work')?'needs_work':live.some(e=>e.verdict==='passed')?'passed':'unverified'
  }
  const review=()=>{
    const criteria=piece.criteria.map(c=>({id:c.id,text:c.text,status:statusOf(c),evidence:box.evidence.filter(e=>e.criterionId===c.id).map(e=>({...e,historical:e.commit!==box.head,unverified:!!e.unverified}))}))
    const accepted=box.recorded.length>0&&box.recorded.at(-1).decision==='accept'
    return {outcomeId:oid,key:'a',title:'Export API',state:piece.state,open:piece.state==='delivery_review',criteriaRevision:box.criteriaRevision,reviewRevision:box.rev,commit:box.commitState==='verified'?box.head:'',commitState:box.commitState,commitNote:box.commitState==='unverified'?'The machine is too old to report its folder’s commit.':'',branch:box.long?'reet/'+'long-branch-name-'.repeat(8):'reet/export-api',remote:box.noRemote?'':'acme/billing',
      pull:{number:12,url:'https://github.com/acme/billing/pull/12',state:'OPEN',headCommit:box.pullAtHead?box.head:HEAD0,atHead:box.pullAtHead},checks:box.pullAtHead&&box.commitState==='verified'?[{name:'test',state:'SUCCESS',tone:'good',url:'https://github.com/acme/billing/actions/runs/1'},...(box.dupChecks?[{name:'test',state:'SUCCESS',tone:'good',url:'https://github.com/acme/billing/actions/runs/3'}]:[]),{name:'lint',state:'FAILURE',tone:'bad',url:'https://github.com/acme/billing/actions/runs/2'}]:[],
      criteria,decisions:box.recorded,accepted,acceptedCommit:accepted?box.recorded.at(-1).commit:'',acceptedCurrent:accepted&&box.recorded.at(-1).commit===box.head,stages:box.stagesEmpty?[]:['code','review','test'],defaultStage:box.stagesEmpty?'':'code',next:'ready_to_merge'}
  }
  const bump=()=>{box.rev++;piece.reviewRevision=box.rev;outcome.revision++;piece.criteriaRevision=box.criteriaRevision}
  const path=`/api/v2/gaffer/outcomes/${oid}/sessions/a/delivery`
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/gaffer/conversations':{conversation:{id:'f'.repeat(32),sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]},
    '/api/v2/gaffer/goals':()=>({goals:[],readAt:Date.now()}),
    '/api/v2/gaffer/outcomes':request=>{
      if(request.method()==='POST'){const input=request.postDataJSON();box.approved.push(input);return{status:201,json:{id:'d'.repeat(32),revision:1,objective:input.objective,state:'settled',note:'',agentId:'a1',projectId:input.projectId,projectName:'billing',dir:'/work/billing',tool:input.tool,providerId:pid,maxReviews:input.maxReviews,hours:input.hours,createdAt:now,updatedAt:now,sessions:input.sessions.map((s,n)=>({key:'abc'[n],title:s.title,prompt:s.prompt,state:'following',note:'Started and followed.',updatedAt:now,stage:0,stages:s.stages.map(x=>st(x.kind,'pending'))}))}}}
      if(box.promoting){if(box.promoting>1){Object.assign(piece,{state:'ready_to_merge',note:'Ready to merge. Nothing merges until you approve it.'});outcome.revision++;box.promoting=0}else box.promoting++}
      return {outcomes:[outcome]}
    },
    [`/api/v2/gaffer/outcomes/${'d'.repeat(32)}`]:()=>({id:'d'.repeat(32),revision:1,state:'settled',sessions:[]}),
    '/api/v2/terminal/status':{configured:true,available:true,reason:''},
    '/api/v2/terminal/sessions':{sessions:[]},
    '/api/v2/terminal/machines':{machines},
    '/api/v2/projects':{projects:[]},
    '/api/v2/folder-projects':{projects:[project('p1','billing','/work/billing',0)],savedWaiting:0},
    '/api/v2/settings/yolo':{allowed:false},
    [`/api/v2/gaffer/outcomes/${oid}/sessions/a/merge`]:request=>{box.merges.push(request.postDataJSON());Object.assign(piece,{state:'following',mergeApproved:true});outcome.revision++;return outcome},
    [path]:async request=>{
      box.reads++;box.gets.push(request.method())
      if(box.readFails>0){box.readFails--;return{status:503,json:{error:'The machine did not answer.'}}}
      return review()
    },
    [`${path}/evidence`]:request=>{
      const body=request.postDataJSON();box.evidenceBodies=[...(box.evidenceBodies??[]),body]
      if(body.reviewRevision!==box.rev)return{status:409,json:{error:'The review changed.',code:'delivery_changed'}}
      box.evidence.push({id:`n${box.evidence.length}`,criterionId:body.criterionId,kind:body.kind,name:body.name,state:body.kind==='check'?(body.name==='test'?'SUCCESS':'FAILURE'):undefined,verdict:body.kind==='check'?(body.name==='test'?'passed':'needs_work'):body.verdict,text:body.text,url:body.url,commit:box.head,criteriaRevision:box.criteriaRevision,byName:'Sam',at:Date.now()})
      bump();return review()
    },
    [`${path}/criteria`]:request=>{
      const body=request.postDataJSON();box.puts.push(body)
      if(box.criteriaConflict){box.criteriaConflict=false;box.criteriaRevision++;bump();return{status:409,json:{error:'The criteria changed.',code:'delivery_criteria_changed'}}}
      box.criteriaRevision++
      piece.criteria=body.criteria.map((c,n)=>({id:c.id||`new${n}`,text:c.text,revision:box.criteriaRevision}))
      bump();return review()
    },
    [`${path}/decision`]:async request=>{
      const body=request.postDataJSON();box.decisions.push(body)
      if(box.delay)await new Promise(r=>setTimeout(r,box.delay))
      const again=box.recorded.find(d=>d.requestId===body.requestId)
      if(again)return{outcome,review:review(),decision:again}
      if(box.fail502>0){box.fail502--;return{status:502,json:{error:'502 Bad Gateway'}}}
      if(box.conflictNext){
        const code=box.conflictNext;box.conflictNext=''
        if(code==='delivery_head_changed'){box.head=HEAD2;bump()}
        if(code==='delivery_closed'){piece.state='ready_to_merge';bump()}
        if(code==='delivery_criteria_changed'){box.criteriaRevision++;bump()}
        return{status:409,json:{error:'The review moved on.',code}}
      }
      if(piece.state!=='delivery_review')return{status:409,json:{error:'closed',code:'delivery_closed'}}
      if(body.criteriaRevision!==box.criteriaRevision)return{status:409,json:{error:'changed',code:'delivery_criteria_changed'}}
      if(body.commit!==(box.commitState==='verified'?box.head:''))return{status:409,json:{error:'changed',code:'delivery_head_changed'}}
      if(body.reviewRevision!==box.rev)return{status:409,json:{error:'changed',code:'delivery_changed'}}
      const decision={requestId:body.requestId,decision:body.decision,commit:body.commit,criteriaRevision:body.criteriaRevision,byName:'Sam',at:Date.now(),notes:body.notes??'',stage:body.stage??'',statuses:body.statuses,failed:Object.entries(body.statuses).filter(([,s])=>s==='needs_work').map(([id])=>id)}
      box.recorded.push(decision);piece.deliveries=[...piece.deliveries,{decision:body.decision}]
      if(body.decision==='accept'){piece.state='advancing';piece.note='You accepted the delivery. Moving on.';piece.deliveryAccepted=true;box.promoting=1}
      else{piece.state='advancing';piece.note=`Sent back to the ${body.stage} stage.`;piece.stage=['code','review','test'].indexOf(body.stage);piece.stages=piece.stages.map((s,n)=>n>=piece.stage?{...s,state:n===piece.stage?'running':'pending'}:s);piece.sessionId='s-a2'}
      bump();return{outcome,review:review(),decision}
    },
  }
  return {box,routes,outcome,piece,review}
}
const watch=(w,box)=>w.page.on('request',r=>{const p=new URL(r.url()).pathname;if(p.startsWith('/api/')&&r.method()!=='GET'&&p!=='/api/v2/skill-buddy/recommendations')box.writes=[...(box.writes??[]),`${r.method()} ${p}`]})
const open=async(width,theme,over={},extra={})=>{
  const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:1100,theme,touch:width<1000,terminalMachines:machines,routes:f.routes,...extra})
  watch(w,f.box);return {f,w,page:w.page}
}
const REVIEW=`${base}/factory?pipeline=${oid}.a&review=1`
const cardOf=page=>page.locator(`#delivery-${oid}-a`)
const noOverflow=async(page,card)=>{expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true);expect(await card.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)}
const FIRST=HEAD1.slice(0,7),SECOND=HEAD2.slice(0,7)
// What the keyboard is on, as the page names it.
const focused=page=>page.evaluate(()=>{const el=document.activeElement;return el?(el.getAttribute('aria-label')||el.textContent||el.tagName).trim().replace(/\s+/g,' '):''})
async function tabTo(page,name,max=60){
  const seen=[]
  for(let i=0;i<max;i++){await page.keyboard.press('Tab');const now=await focused(page);seen.push(now);if(now===name)return seen}
  throw new Error(`Tab never reached “${name}”: ${seen.join(' | ')}`)
}

try{
  for(const [width,theme] of [[1440,'light'],[390,'dark']]){
    const wide=width>=1100
    // 1. Criteria are agreed where the plan is approved, and reach the create body: empty rows are dropped, a pipeline with none sends none.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(base+'/factory')
        await page.getByRole('button',{name:'New Factory outcome',exact:true}).first().click()
        const outcome=page.getByRole('dialog',{name:'New Factory outcome',exact:true})
        await outcome.getByRole('textbox',{name:'Outcome',exact:true}).fill('Ship CSV export for billing.')
        await outcome.getByRole('combobox',{name:'Project',exact:true}).selectOption('p1')
        await outcome.getByRole('button',{name:'Write the plan myself',exact:true}).click();
        await outcome.getByRole('textbox',{name:'Pipeline 1 title',exact:true}).fill('Export API')
        await outcome.getByRole('textbox',{name:'Pipeline 1 prompt',exact:true}).fill('Build the CSV export. Raise a pull request. Do not merge.')
        const criteria=outcome.getByRole('group',{name:'Acceptance criteria',exact:true})
        await expect(criteria).toContainText('The pipeline stops for a delivery review against these before it merges or finishes.')
        await expect(outcome.getByRole('listitem').filter({has:page.getByRole('textbox',{name:/^Criterion/})})).toHaveCount(0)
        await outcome.getByRole('button',{name:'Add a criterion',exact:true}).click()
        // The new row takes the focus, so the keyboard can type at once.
        await expect(outcome.getByRole('textbox',{name:'Criterion 1 for pipeline 1',exact:true})).toBeFocused()
        await page.keyboard.type('The export downloads as CSV.')
        await page.keyboard.press('Enter')
        await expect(outcome.getByRole('textbox',{name:'Criterion 2 for pipeline 1',exact:true})).toBeFocused()
        await page.keyboard.type('  Totals match   the ledger. ')
        await outcome.getByRole('button',{name:'Add another criterion',exact:true}).click()
        await expect(outcome.getByRole('textbox',{name:'Criterion 3 for pipeline 1',exact:true})).toBeFocused()
        // A row left empty is dropped when sent.
        await outcome.getByRole('button',{name:'Add another criterion',exact:true}).click()
        await expect(outcome.getByRole('textbox',{name:/^Criterion \d for pipeline 1$/})).toHaveCount(4)
        await outcome.getByRole('button',{name:'Remove criterion 3 for pipeline 1',exact:true}).click()
        await expect(outcome.getByRole('textbox',{name:/^Criterion \d for pipeline 1$/})).toHaveCount(3)
        // A second pipeline with no criteria sends none and is approved as before.
        await outcome.getByRole('button',{name:'Add a pipeline',exact:true}).click()
        await outcome.getByRole('textbox',{name:'Pipeline 2 title',exact:true}).fill('Export docs')
        await outcome.getByRole('textbox',{name:'Pipeline 2 prompt',exact:true}).fill('Document the export. Raise a pull request. Do not merge.')
        // Eleven is over the limit and says so; ten is fine.
        for(let n=0;n<7;n++)await outcome.getByRole('button',{name:/^Add (a|another) criterion$/}).first().click()
        await expect(outcome.getByRole('button',{name:'Add another criterion',exact:true}).first()).toBeDisabled()
        await expect(outcome.getByText('That is the most a pipeline takes (10).',{exact:true})).toBeVisible()
        for(let n=10;n>=4;n--)await outcome.getByRole('button',{name:`Remove criterion ${n} for pipeline 1`,exact:true}).click()
        await expect(outcome.getByRole('textbox',{name:/^Criterion \d for pipeline 1$/})).toHaveCount(3)
        await noOverflow(page,outcome)
        await outcome.screenshot({path:`${shots}/plan-criteria-${width}-${theme}.png`})
        await outcome.getByRole('button',{name:'Approve and start 2 pipelines',exact:true}).click()
        await expect.poll(()=>f.box.approved.length).toBe(1)
        const sent=f.box.approved[0].sessions
        expect(sent[0].criteria).toEqual(['The export downloads as CSV.','Totals match the ledger.'])
        expect('criteria' in sent[1]).toBe(false)
        expect(w.errors).toEqual([])
      }finally{await w.context.close()}
    }

    // 2. The card: a calm loading state with what is being read, then every criterion with its status and evidence, the
    //    reviewed commit and the pull request. Opening it from the address focuses it and reads it at once.
    {
      let release;const gate=new Promise(r=>{release=r})
      const {f,w,page}=await open(width,theme,{dupChecks:true},{gates:{[`/api/v2/gaffer/outcomes/${oid}/sessions/a/delivery`]:gate}})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        const heading=card.getByRole('heading',{name:'Delivery review',level:3})
        await expect(heading).toBeFocused()
        await expect(card.getByRole('status').filter({hasText:'Reading the pipeline’s folder on its machine. This can take up to 15 seconds.'})).toBeVisible()
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toHaveCount(0)
        release()
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        // The reviewed commit: short in the page, the whole of it in the title and on the clipboard button's name.
        await expect(card.locator('code[title="'+HEAD1+'"]').first()).toHaveText(FIRST)
        await expect(card.getByRole('button',{name:`Copy the full commit ID ${HEAD1}`,exact:true})).toBeVisible()
        await expect(card).toContainText('Verified on the machine')
        await expect(card.getByRole('definition').filter({hasText:'reet/export-api'})).toBeVisible()
        await expect(card.getByRole('link',{name:'#12',exact:true})).toHaveAttribute('href','https://github.com/acme/billing/pull/12')
        await expect(card).not.toContainText('is not the reviewed commit')
        // Every criterion has its status in words and a shape, never colour alone, and its evidence beneath.
        const items=card.getByRole('list',{name:'Acceptance criteria',exact:true}).locator(':scope > li')
        await expect(items).toHaveCount(3)
        await expect(items.nth(0)).toContainText('Passed');await expect(items.nth(0)).toContainText(TEXTS.c1)
        await expect(items.nth(1)).toContainText('Needs work');await expect(items.nth(2)).toContainText('Unverified')
        await expect(card).toContainText('1 of 3 criteria passed · 1 needs work · 1 unverified')
        expect(await items.nth(0).locator('.glyph').getAttribute('aria-hidden')).toBe('true')
        const e1=items.nth(0).getByRole('list',{name:'Evidence for criterion 1',exact:true})
        await expect(e1).toContainText('GitHub check');await expect(e1).toContainText('test');await expect(e1).toContainText('Passed');await expect(e1).toContainText('added by Paul')
        await expect(e1.getByRole('link',{name:'Open the check',exact:true})).toHaveAttribute('href','https://github.com/acme/billing/actions/runs/1')
        const e2=items.nth(1).getByRole('list',{name:'Evidence for criterion 2',exact:true})
        await expect(e2).toContainText('Reviewer note');await expect(e2).toContainText('Needs work');await expect(e2).toContainText('Rounds the pence down.');await expect(e2).toContainText('Paul')
        // Earlier evidence is labelled and never counts; evidence that could not be verified says so.
        const e3=items.nth(2).getByRole('list',{name:'Evidence for criterion 3',exact:true})
        await expect(e3.locator('li.historical')).toContainText('Earlier commit');await expect(e3.locator('li.historical')).toContainText('Checked by hand on the old commit.')
        await expect(e3.locator('li.unverified')).toContainText('Could not be verified')
        // The decisions sit with the findings, and the copy says accepting is not the merge approval.
        await expect(card.getByRole('group',{name:'Your decision on this delivery',exact:true}).getByRole('button')).toHaveText(['Accept delivery','Send back'])
        await expect(card).toContainText('It does not approve the GitHub merge: Approve the merge stays its own action.')
        await expect(card).toContainText('Accepting is not approving the merge: Approve the merge stays your separate decision.')
        await noOverflow(page,card)
        await card.scrollIntoViewIfNeeded();await card.screenshot({path:`${shots}/card-${width}-${theme}.png`})
        // The pipeline's own page says it waits, with the one filled button that brings the person here.
        await expect(page.getByRole('region',{name:'What is waiting',exact:true})).toContainText('The delivery review is below. Accepting the delivery is not approving the merge.')
        expect(f.box.reads).toBe(1);expect(f.box.writes??[]).toEqual([])
        // Evidence is added against a criterion: a reviewer note, a check, an artefact link.
        await card.getByRole('button',{name:'Add evidence for criterion 3',exact:true}).click()
        const form=card.getByRole('form',{name:'Add evidence for criterion 3',exact:true})
        await expect(form.getByRole('button',{name:'Add evidence',exact:true})).toBeDisabled()
        await form.getByRole('textbox',{name:'Note',exact:true}).fill('Read the export query: it filters by tenant.')
        await form.getByRole('button',{name:'Add evidence',exact:true}).click()
        await expect(card.getByRole('status').filter({hasText:'Evidence added.'})).toBeVisible()
        expect(f.box.evidenceBodies).toEqual([{requestId:expect.stringMatching(/^[0-9a-f]{32}$/),reviewRevision:3,criterionId:'c3',kind:'note',verdict:'passed',text:'Read the export query: it filters by tenant.'}])
        await expect(items.nth(2)).toContainText('Passed');await expect(items.nth(2)).toContainText('Read the export query: it filters by tenant.')
        await expect(card).toContainText('2 of 3 criteria passed · 1 needs work')
        await card.getByRole('button',{name:'Add evidence for criterion 1',exact:true}).click()
        const f1=card.getByRole('form',{name:'Add evidence for criterion 1',exact:true})
        await f1.getByRole('combobox',{name:'What kind',exact:true}).selectOption('check')
        await expect(f1.getByRole('combobox',{name:'Check',exact:true}).getByRole('option')).toHaveText(['Choose a check','test · Passed','test · Passed','lint · Failed'])
        await f1.getByRole('combobox',{name:'Check',exact:true}).selectOption({label:'lint · Failed'})
        await f1.getByRole('button',{name:'Add evidence',exact:true}).click()
        await expect(items.nth(0)).toContainText('Needs work')
        expect(f.box.evidenceBodies[1]).toMatchObject({reviewRevision:4,criterionId:'c1',kind:'check',name:'lint'})
        await card.getByRole('button',{name:'Add evidence for criterion 2',exact:true}).click()
        const f2=card.getByRole('form',{name:'Add evidence for criterion 2',exact:true})
        await f2.getByRole('combobox',{name:'What kind',exact:true}).selectOption('artefact')
        await f2.getByRole('textbox',{name:'Link inside the repository',exact:true}).fill('https://github.com/acme/billing/blob/main/docs/export.md')
        await f2.getByRole('combobox',{name:'Verdict',exact:true}).selectOption('needs_work')
        await f2.getByRole('button',{name:'Cancel',exact:true}).click()
        await expect(f2).toHaveCount(0);expect(f.box.evidenceBodies.length).toBe(2)
        expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
      }finally{await w.context.close()}
    }

    // 2b. The read can fail, and says so with Retry; a pull request on another commit and an unverifiable commit are said plainly.
    {
      const {f,w,page}=await open(width,theme,{readFails:1,pullAtHead:false,commitState:'unverified'})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByRole('alert')).toHaveText('The delivery review could not be read: The machine did not answer.')
        await card.getByRole('button',{name:'Retry',exact:true}).click()
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        await expect(card).toContainText('Could not be verified: The machine is too old to report its folder’s commit.')
        await expect(card.getByRole('note').filter({hasText:'is not the reviewed commit'})).toContainText(`Its head (${HEAD0.slice(0,7)}) is not the reviewed commit, so its checks do not count until they do.`)
        await expect(card).toContainText('No commit could be read')
        await expect(card.getByRole('button',{name:/^Copy the full commit ID/})).toHaveCount(0)
        await card.getByRole('button',{name:'Add evidence for criterion 1',exact:true}).click()
        await card.getByRole('form',{name:'Add evidence for criterion 1',exact:true}).getByRole('combobox',{name:'What kind',exact:true}).selectOption('check')
        await expect(card).toContainText('The pull request is not at the reviewed commit, so its checks are not offered.')
        // A decision is not offered while the commit cannot be verified.
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toBeDisabled();await expect(card.getByRole('button',{name:'Send back',exact:true})).toBeDisabled()
        await expect(card.getByRole('note').filter({hasText:'A decision is not recorded until it can.'})).toBeVisible()
        await noOverflow(page,card);await card.screenshot({path:`${shots}/card-unverified-${width}-${theme}.png`})
        expect(f.box.reads).toBe(2);expect(f.box.decisions).toEqual([])
      }finally{await w.context.close()}
    }

    // 3. Accept, by keyboard alone from the focused card: the confirmation names what is not Passed, the decision carries
    //    the revisions, the commit, the statuses and a request ID, and the merge approval stays a separate action.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        // Tab walks the card in reading order: the commit, the pull request, each criterion's evidence and its own Add evidence, then the decisions.
        const seen=await tabTo(page,'Accept delivery')
        expect(seen[0]).toMatch(/^Copy the full commit ID/)
        const adds=seen.filter(name=>name.startsWith('Add evidence for criterion'))
        expect(adds).toEqual(['Add evidence for criterion 1','Add evidence for criterion 2','Add evidence for criterion 3'])
        expect(seen.indexOf('#12')).toBeGreaterThan(0)
        await page.keyboard.press('Enter')
        const ask=card.getByRole('group',{name:'Accept with 2 not Passed?',exact:true})
        await expect(ask).toContainText(`You are accepting commit ${FIRST} of “Export API”. 2 of 3 criteria are not Passed (1 needs work, 1 unverified).`)
        await expect(ask).toContainText('Accepting is not approving the merge: Approve the merge stays your separate decision.')
        await expect(ask.getByRole('button',{name:'Cancel',exact:true})).toBeFocused()
        expect(f.box.decisions).toEqual([])
        await page.keyboard.press('Tab');await expect(ask.getByRole('button',{name:'Accept anyway',exact:true})).toBeFocused()
        await page.keyboard.press('Enter')
        await expect(card.getByRole('status').filter({hasText:'Delivery accepted'})).toContainText(`Delivery accepted on ${FIRST}.`)
        expect(f.box.decisions).toEqual([{requestId:expect.stringMatching(/^[0-9a-f]{32}$/),decision:'accept',criteriaRevision:1,reviewRevision:3,commit:HEAD1,statuses:{c1:'passed',c2:'needs_work',c3:'unverified'},confirmed:true}])
        // The keyboard stays in the card, on its heading, now that the buttons are gone.
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toHaveCount(0)
        await expect(card).toContainText(`The delivery was accepted on the current commit (${FIRST}).`)
        // The pipeline is advancing for a moment; the polled list then brings it to Ready to merge, with no new read of the machine.
        await expect(card).toContainText('Accepting the delivery did not approve the merge. This pipeline waits at Ready to merge: Approve the merge is its own action on this page.',{timeout:15000})
        expect(f.box.reads).toBe(1)
        const recorded=card.getByRole('list',{name:'Recorded decisions, newest first',exact:true})
        await expect(recorded).toContainText(`Accepted on ${FIRST} by Sam`)
        // Nothing merged: the existing merge approval is its own action, still there to be pressed.
        expect(f.box.merges).toEqual([])
        await expect(page.getByRole('region',{name:'Export API',exact:true}).getByRole('button',{name:/^Approve the merge/})).toBeVisible({timeout:15000})
        await expect(page.getByRole('region',{name:'Export API',exact:true}).locator('.chip').first()).toHaveText('Ready to merge',{timeout:15000})
        expect(f.box.writes).toEqual([`POST /api/v2/gaffer/outcomes/${oid}/sessions/a/delivery/decision`])
        await noOverflow(page,card);await card.screenshot({path:`${shots}/card-accepted-${width}-${theme}.png`})
        expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
      }finally{await w.context.close()}
    }

    // 3b. An acceptance of a commit that is no longer the current one says so.
    {
      const {f,w,page}=await open(width,theme,{state:'ready_to_merge',recorded:[{requestId:'f'.repeat(32),decision:'accept',commit:HEAD0,criteriaRevision:1,byName:'Paul',at:now-3600000,notes:'',stage:'',statuses:{},failed:[]}]})
      try{
        f.piece.state='ready_to_merge'
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card).toContainText(`The delivery was accepted on ${HEAD0.slice(0,7)}, which is not the current commit (${FIRST}).`)
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toHaveCount(0)
      }finally{await w.context.close()}
    }

    // 4. Send back carries notes and the stage; notes are needed only when no criterion needs work.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await expect(form.getByRole('combobox',{name:'Stage to do again',exact:true}).getByRole('option')).toHaveText(['Code','Review','Test'])
        await expect(form.getByRole('combobox',{name:'Stage to do again',exact:true})).toHaveValue('code')
        // A criterion needs work, so the agent already has something to go on and notes are optional.
        await expect(form.getByRole('button',{name:'Send back',exact:true})).toBeEnabled()
        await form.getByRole('combobox',{name:'Stage to do again',exact:true}).selectOption('review')
        await form.getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('Totals round down; use banker’s rounding.')
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect(card.getByRole('status').filter({hasText:'Sent back to review.'})).toBeVisible()
        expect(f.box.decisions).toEqual([{requestId:expect.stringMatching(/^[0-9a-f]{32}$/),decision:'send_back',criteriaRevision:1,reviewRevision:3,commit:HEAD1,statuses:{c1:'passed',c2:'needs_work',c3:'unverified'},notes:'Totals round down; use banker’s rounding.',stage:'review',confirmed:true}])
        await expect(card.getByRole('list',{name:'Recorded decisions, newest first',exact:true})).toContainText(`Sent back to Review on ${FIRST} by Sam`)
        await expect(card.getByRole('list',{name:'Recorded decisions, newest first',exact:true})).toContainText('Totals round down; use banker’s rounding.')
      }finally{await w.context.close()}
    }
    {
      // With no criterion marked Needs work, notes are required.
      const {f,w,page}=await open(width,theme,{noNeedsWork:true})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await expect(form.getByRole('button',{name:'Send back',exact:true})).toBeDisabled()
        await expect(form.getByRole('status')).toContainText('Say what is wrong. No criterion is marked Needs work')
        await form.getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('Nothing proves the tenant filter.')
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect.poll(()=>f.box.decisions.length).toBe(1)
        expect(f.box.decisions[0]).toMatchObject({decision:'send_back',stage:'code',notes:'Nothing proves the tenant filter.',statuses:{c1:'passed',c2:'unverified',c3:'unverified'}})
      }finally{await w.context.close()}
    }

    // 5. A new commit since the person looked: nothing is lost, the page says what changed, reloads, and the person decides again.
    {
      const {f,w,page}=await open(width,theme,{conflictNext:'delivery_head_changed'})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await form.getByRole('combobox',{name:'Stage to do again',exact:true}).selectOption('test')
        await form.getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('Totals round down.')
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect(card.getByRole('alert')).toContainText('A new commit arrived since you looked. The review below is as it is now. What you typed is kept.')
        // Reloaded: the new commit, and the evidence for the old one no longer counts.
        await expect(card.locator(`code[title="${HEAD2}"]`).first()).toHaveText(SECOND)
        await expect(card).toContainText('0 of 3 criteria passed · 3 unverified')
        await expect(form.getByRole('textbox',{name:'Notes for the agent',exact:true})).toHaveValue('Totals round down.')
        await expect(form.getByRole('combobox',{name:'Stage to do again',exact:true})).toHaveValue('test')
        await expect(card.getByRole('list',{name:'Evidence for criterion 2',exact:true}).locator('li.historical')).toContainText('Earlier commit')
        expect(f.box.reads).toBe(2)
        // Deciding again is a new request about what is now shown.
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect.poll(()=>f.box.decisions.length).toBe(2)
        expect(f.box.decisions[1]).toMatchObject({decision:'send_back',commit:HEAD2,reviewRevision:4,notes:'Totals round down.',stage:'test',statuses:{c1:'unverified',c2:'unverified',c3:'unverified'}})
        expect(f.box.decisions[1].requestId).not.toBe(f.box.decisions[0].requestId)
        await expect(card.getByRole('status').filter({hasText:'Sent back to test.'})).toBeVisible()
        await noOverflow(page,card)
      }finally{await w.context.close()}
    }
    {
      // Someone else decided first.
      const {f,w,page}=await open(width,theme,{conflictNext:'delivery_closed'})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Accept delivery',exact:true}).click()
        await card.getByRole('button',{name:'Accept anyway',exact:true}).click()
        await expect(card.getByRole('alert')).toContainText('This delivery was already decided, or is no longer waiting for a decision.')
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toHaveCount(0)
        expect(f.box.decisions.length).toBe(1)
      }finally{await w.context.close()}
    }

    // 6. A lost answer: the retry carries the same request ID, so the server records it once.
    {
      const {f,w,page}=await open(width,theme,{fail502:1})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Accept delivery',exact:true}).click()
        await card.getByRole('button',{name:'Accept anyway',exact:true}).click()
        await expect(card.getByRole('alert')).toContainText('502 Bad Gateway')
        await expect(card.getByRole('alert')).toContainText('Trying again is safe')
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toBeFocused()
        await card.getByRole('button',{name:'Accept delivery',exact:true}).click()
        await card.getByRole('button',{name:'Accept anyway',exact:true}).click()
        await expect(card.getByRole('status').filter({hasText:'Delivery accepted'})).toBeVisible()
        expect(f.box.decisions.length).toBe(2)
        expect(f.box.decisions[1].requestId).toBe(f.box.decisions[0].requestId)
        expect(f.box.recorded.length).toBe(1)
      }finally{await w.context.close()}
    }

    // 7. A double press sends one decision, and the buttons wait while it is in flight.
    {
      const {f,w,page}=await open(width,theme,{delay:700})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await form.getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('Again, please.')
        await form.getByRole('button',{name:'Send back',exact:true}).dblclick()
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toBeDisabled()
        await expect(card.getByRole('status').filter({hasText:'Sent back to code.'})).toBeVisible()
        expect(f.box.decisions.length).toBe(1);expect(f.box.recorded.length).toBe(1)
      }finally{await w.context.close()}
    }
    {
      const {f,w,page}=await open(width,theme,{delay:700})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Accept delivery',exact:true}).click()
        await card.getByRole('button',{name:'Accept anyway',exact:true}).dblclick()
        await expect(card.getByRole('status').filter({hasText:'Delivery accepted'})).toBeVisible()
        expect(f.box.decisions.length).toBe(1)
      }finally{await w.context.close()}
    }

    // 8. The criteria are edited in place with the revision that was shown; a conflict keeps what was typed.
    {
      const {f,w,page}=await open(width,theme,{criteriaConflict:true})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Edit criteria',exact:true}).click()
        const edit=card.getByRole('form',{name:'Edit the acceptance criteria',exact:true})
        await expect(edit.getByRole('textbox',{name:'Criterion 2 for Export API',exact:true})).toHaveValue(TEXTS.c2)
        await edit.getByRole('textbox',{name:'Criterion 2 for Export API',exact:true}).fill('Totals match the ledger to the penny.')
        await edit.getByRole('button',{name:'Remove criterion 3 for Export API',exact:true}).click()
        await edit.getByRole('button',{name:'Add another criterion',exact:true}).click()
        await page.keyboard.type('The file opens in a spreadsheet.')
        await edit.getByRole('button',{name:'Add another criterion',exact:true}).click()
        await edit.getByRole('button',{name:'Save criteria',exact:true}).click()
        await expect(card.getByRole('alert')).toContainText('The criteria were changed while you were looking at them. The review below is as it is now. What you typed is kept.')
        await expect(edit.getByRole('textbox',{name:'Criterion 2 for Export API',exact:true})).toHaveValue(TEXTS.c2)
        await expect(card.getByRole('note').filter({hasText:'What you typed, to copy from:'})).toContainText('Totals match the ledger to the penny.')
        await expect(card.getByRole('note').filter({hasText:'What you typed, to copy from:'})).toContainText('The file opens in a spreadsheet.')
        expect(f.box.puts[0]).toEqual({criteriaRevision:1,criteria:[{id:'c1',text:TEXTS.c1},{id:'c2',text:'Totals match the ledger to the penny.'},{text:'The file opens in a spreadsheet.'}]})
        await edit.getByRole('textbox',{name:'Criterion 2 for Export API',exact:true}).fill('Totals match the ledger to the penny.')
        await edit.getByRole('button',{name:'Save criteria',exact:true}).click()
        await expect.poll(()=>f.box.puts.length).toBe(2)
        expect(f.box.puts[1].criteriaRevision).toBe(2)
        await expect(card.getByRole('button',{name:'Edit criteria',exact:true})).toBeFocused()
        await expect(card.getByRole('list',{name:'Acceptance criteria',exact:true}).locator(':scope > li')).toHaveCount(3)
      }finally{await w.context.close()}
    }

    // 8b. The review goes back to the person after an acceptance when the work changed under it: the pipeline's own note
    //     says why and that the approval was withdrawn, on the card, on the pipeline's page and on Home.
    {
      const note='Delivery review. The work changed after you accepted it: a new commit arrived, the criteria were reworded, or the commit could not be read. Nothing merges until you accept what is there now.'
      const {f,w,page}=await open(width,theme,{note,recorded:[{requestId:'f'.repeat(32),decision:'accept',commit:HEAD0,criteriaRevision:1,byName:'Paul',at:now-3600000,notes:'',stage:'',statuses:{c1:'passed',c2:'passed',c3:'passed'},failed:[]}]})
      try{
        f.piece.deliveries=[{decision:'accept'}]
        await page.goto(base+'/')
        const item=page.locator('article').filter({hasText:'Export API'}).first()
        await expect(item).toContainText('Delivery review · changed after you accepted it · 3 criteria')
        expect(f.box.reads).toBe(0)
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByRole('note').filter({hasText:'The work changed after you accepted it'})).toContainText('Your acceptance and any merge approval you gave were withdrawn.')
        await expect(page.getByRole('region',{name:'What is waiting',exact:true})).toContainText('The work changed after you accepted it, so your acceptance and any merge approval were withdrawn.')
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toBeEnabled()
        await noOverflow(page,card)
      }finally{await w.context.close()}
    }

    // 8c. The criteria change under a decision: it is refused with that reason, the typed notes stay, the review is read
    //     again, and the keyboard is back on the button that was pressed.
    {
      const {f,w,page}=await open(width,theme,{conflictNext:'delivery_criteria_changed'})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await form.getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('Keep this text.')
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect(card.getByRole('alert')).toContainText('The criteria were changed while you were looking at them. The review below is as it is now. What you typed is kept.')
        await expect(form.getByRole('textbox',{name:'Notes for the agent',exact:true})).toHaveValue('Keep this text.')
        await expect(card.getByRole('button',{name:'Send back',exact:true}).first()).toBeFocused()
        expect(f.box.reads).toBe(2)
        await form.getByRole('button',{name:'Send back',exact:true}).click()
        await expect.poll(()=>f.box.decisions.length).toBe(2)
        expect(f.box.decisions[1].criteriaRevision).toBe(2)
        // Cancelling the form puts the keyboard back on its toggle.
      }finally{await w.context.close()}
    }

    // 8d. Send back by keyboard alone, and Cancel returns to the toggle.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        await tabTo(page,'Send back')
        await page.keyboard.press('Enter')
        const form=card.getByRole('form',{name:'Send this delivery back',exact:true})
        await expect(form).toBeVisible()
        await page.keyboard.press('Tab');await expect(form.getByRole('combobox',{name:'Stage to do again',exact:true})).toBeFocused()
        await expect(form.getByRole('combobox',{name:'Stage to do again',exact:true})).toHaveValue('code')
        await page.keyboard.press('Tab');await expect(form.getByRole('textbox',{name:'Notes for the agent',exact:true})).toBeFocused()
        await page.keyboard.type('Use banker’s rounding.')
        await page.keyboard.press('Tab');await expect(form.getByRole('button',{name:'Send back',exact:true})).toBeFocused()
        await page.keyboard.press('Tab');await expect(form.getByRole('button',{name:'Cancel',exact:true})).toBeFocused()
        await page.keyboard.press('Enter')
        await expect(form).toHaveCount(0)
        await expect(card.getByRole('button',{name:'Send back',exact:true})).toBeFocused()
        expect(f.box.decisions).toEqual([])
        await page.keyboard.press('Enter')
        // The notes typed before Cancel are still there: Tab past the stage and them to the submit button.
        await page.keyboard.press('Tab');await page.keyboard.press('Tab');await page.keyboard.press('Tab');await page.keyboard.press('Enter')
        await expect.poll(()=>f.box.decisions.length).toBe(1)
        expect(f.box.decisions[0]).toMatchObject({decision:'send_back',stage:'code',notes:'Use banker’s rounding.',confirmed:true})
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
      }finally{await w.context.close()}
    }

    // 8e. Long words in a criterion, the branch, a person's name and a note never push the page sideways.
    {
      const {f,w,page}=await open(width,theme,{long:true})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        await expect(card).toContainText('x'.repeat(240))
        await expect(card).toContainText('n'.repeat(300))
        await card.getByRole('button',{name:'Send back',exact:true}).click()
        await card.getByRole('form',{name:'Send this delivery back',exact:true}).getByRole('textbox',{name:'Notes for the agent',exact:true}).fill('w'.repeat(400))
        await noOverflow(page,card);await card.screenshot({path:`${shots}/card-long-${width}-${theme}.png`})
        // The history counts what was not Passed from the statuses it was shown, not from the failures alone.
        await expect(card.getByRole('list',{name:'Recorded decisions, newest first',exact:true})).toContainText('Not passed then: 2 criteria.')
      }finally{await w.context.close()}
    }

    // 8f. Not a Git repository, and a pipeline with no stage before its merge: no commit to copy, notes only for evidence,
    //     and Send back says why it is not offered. Accepting records no commit.
    {
      const {f,w,page}=await open(width,theme,{commitState:'none',noRemote:true,stagesEmpty:true})
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card).toContainText('No commit: not a Git repository')
        await expect(card.getByRole('button',{name:'Send back',exact:true})).toBeDisabled()
        await expect(card).toContainText('Send back is not offered: this pipeline has no stage before its merge to do again.')
        await card.getByRole('button',{name:'Add evidence for criterion 1',exact:true}).click()
        const kind=card.getByRole('form',{name:'Add evidence for criterion 1',exact:true}).getByRole('combobox',{name:'What kind',exact:true})
        await expect(kind).toBeDisabled();await expect(kind.getByRole('option')).toHaveText(['Reviewer note'])
        await card.getByRole('button',{name:'Accept delivery',exact:true}).click()
        await card.getByRole('button',{name:'Accept anyway',exact:true}).click()
        await expect(card.getByRole('status').filter({hasText:'Delivery accepted.'})).toBeVisible()
        expect(f.box.decisions[0].commit).toBe('')
      }finally{await w.context.close()}
    }

    // 8g. Another reviewer decides while this card is open: the polled list leaves delivery review and the card reads again
    //     (once), showing no decision to make. The merge setting changing is worked out on the page, with no read.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(REVIEW)
        const card=cardOf(page)
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        expect(f.box.reads).toBe(1)
        // Letting the pipeline merge by itself changes what accepting leads to, and asks the machine nothing.
        f.piece.autoMerge=true
        await expect(card.getByRole('group',{name:'Your decision on this delivery',exact:true})).toContainText('goes on to its merge stage and merges by itself once its checks pass',{timeout:15000})
        expect(f.box.reads).toBe(1)
        f.box.recorded.push({requestId:'c'.repeat(32),decision:'accept',commit:HEAD1,criteriaRevision:1,byName:'Paul',at:Date.now(),notes:'',stage:'',statuses:{c1:'passed',c2:'passed',c3:'passed'},failed:[]})
        Object.assign(f.piece,{state:'ready_to_merge',note:'Ready to merge.'});f.box.rev++;f.piece.reviewRevision=f.box.rev;f.outcome.revision++
        await expect(card.getByRole('button',{name:'Accept delivery',exact:true})).toHaveCount(0,{timeout:20000})
        await expect(card).toContainText('The delivery was accepted on the current commit')
        expect(f.box.reads).toBe(2)
      }finally{await w.context.close()}
    }

    // 9. An outcome saved before criteria existed renders exactly as it did: no card, and no read of the machine.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(`${base}/factory?pipeline=${oid}.b`)
        const detail=page.getByRole('region',{name:'Importer retry',exact:true})
        await expect(detail).toBeVisible()
        await expect(page.getByRole('heading',{name:'Delivery review',level:3})).toHaveCount(0)
        await expect(page.locator('[id^="delivery-"]')).toHaveCount(0)
        await expect(detail).not.toContainText('criteria')
        // Even the pipeline that has criteria reads nothing from its machine until the review is shown.
        if(!wide)await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
        expect(f.box.reads).toBe(0)
        expect(w.errors).toEqual([])
      }finally{await w.context.close()}
    }
    {
      // A pipeline with criteria that is not waiting for its review keeps the review closed until it is asked for.
      const {f,w,page}=await open(width,theme,{state:'following'})
      try{
        f.piece.state='following';f.piece.note='The test stage is under way.'
        await page.goto(`${base}/factory?pipeline=${oid}.a`)
        const card=cardOf(page)
        await expect(card).toContainText('3 criteria agreed')
        expect(f.box.reads).toBe(0)
        await card.getByRole('button',{name:'Show the delivery review',exact:true}).click()
        await expect(card.getByText('No decision waiting',{exact:true})).toBeVisible()
        expect(f.box.reads).toBe(1)
      }finally{await w.context.close()}
    }

    // 10. Home says what waits from the outcome list alone, and its button opens the exact review.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(base+'/')
        const item=page.locator('article').filter({hasText:'Export API'}).first()
        await expect(item).toContainText('Delivery review')
        await expect(item).toContainText('Delivery review · 3 criteria')
        await expect(item.getByRole('list',{name:'Stages of Export API',exact:true})).toBeVisible()
        // Nothing was read from the machine to say so.
        expect(f.box.reads).toBe(0)
        await expect(item.getByRole('button',{name:'Review delivery',exact:true})).toBeVisible()
        await item.screenshot({path:`${shots}/home-${width}-${theme}.png`})
        await item.getByRole('button',{name:'Review delivery',exact:true}).click()
        await expect(page).toHaveURL(new RegExp(`/factory\\?pipeline=${oid}\\.a&review=1$`))
        const card=cardOf(page)
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        expect(await card.evaluate(el=>{const r=el.getBoundingClientRect();return r.top>=-1&&r.top<innerHeight})).toBe(true)
        expect(f.box.reads).toBe(1)
        // A reload keeps the same pipeline chosen and its review in view.
        await page.reload()
        await expect(cardOf(page).getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(cardOf(page).getByText('Waiting for your decision',{exact:true})).toBeVisible()
        // Back returns to Home.
        await page.goBack()
        await expect(page).toHaveURL(base+'/')
        expect(w.errors).toEqual([])
      }finally{await w.context.close()}
    }

    // 11. On the Factory page the waiting item itself offers Review delivery and opens the review.
    {
      const {f,w,page}=await open(width,theme)
      try{
        await page.goto(base+'/factory')
        const item=page.locator(`#pipeline-${oid}-a`)
        await expect(item).toContainText('Delivery review')
        // A wide screen has the item as one button beside the open detail, whose own bar has the filled Review delivery;
        // a phone's item is a card with it.
        if(wide){await expect(item).toHaveAttribute('aria-label','Delivery review: Export API');await page.getByRole('button',{name:/^Review delivery/}).click()}
        else await item.getByRole('button',{name:'Review delivery',exact:true}).click()
        const card=cardOf(page)
        await expect(card.getByRole('heading',{name:'Delivery review',level:3})).toBeFocused()
        await expect(card.getByText('Waiting for your decision',{exact:true})).toBeVisible()
        await expect(page).toHaveURL(/review=1/)
      }finally{await w.context.close()}
    }
  }
  console.log('Factory delivery review: criteria agreed with the plan, a card that shows each criterion with its evidence, Accept and Send back tied to the commit and revisions, conflicts that keep what was typed, one decision per request, old outcomes unchanged, and Home’s pointer to the exact review.')
}finally{await browser.close()}
