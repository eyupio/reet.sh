import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Factory pathways: a person's own lines of stages. This suite grows with the feature: the list (its rows, the on and off
// switch, Duplicate and Delete, the empty, loading and failed states, and the way in from the Factory), then the editor.
// The API is answered with fixtures that follow internal/api/gaffer_pathways.go.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-pathways'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const LIST='/api/v2/gaffer/pathways'
const st=(kind,extra={})=>({kind,...(kind==='custom'?{}:{tier:'standard'}),...extra})
const pathway=(id,name,stages,extra={})=>({id,name,description:'',enabled:true,stages,createdAt:now-86400000,updatedAt:now-3600000-id.length,...extra})
const standard=()=>pathway('p1','Standard',['code','review','test','pr','babysit','merge'].map(k=>st(k)),{description:'The usual line, from code to merge.'})
const careful=()=>pathway('p2','Careful release',[st('code',{skills:['lint-rules']}),st('custom',{name:'Security pass',prompt:'Look for secrets.'}),st('review'),st('pr'),st('merge')],
 {description:'For anything that touches billing.',problems:['The skill “lint-rules” on Code is no longer on your shelf. Work cannot start on this pathway until it is replaced.']})
const docs=()=>pathway('p3','Docs only',[st('custom',{name:'Write docs',prompt:'Write the docs.'}),st('pr')],{enabled:false,description:'Words, not code.'})
const three=()=>[standard(),careful(),docs()]

// What the editor reads beside the pathway: the skills a stage can load, the prompt library, model favourites and the crew.
const shelfSkill=(id,description,bytes)=>({id,name:id,description,bytes})
const SHELF=[shelfSkill('threat-model','Find the ways in before someone else does.',700),shelfSkill('deps-check','Check new dependencies for known problems.',500),shelfSkill('test-first','Write the failing test before the fix.',600)]
const PROMPTS=[{id:'pr1',title:'Audit endpoints',description:'',body:'Check every new endpoint for auth.\nList what you find.',category:'',tool:'',model:'',effort:'',revision:1,createdAt:now,updatedAt:now}]
const CREW={crew:{heavy:{tool:'claude',model:'claude-opus-5-5'},standard:{tool:'codex',model:'gpt-5'},light:{tool:'claude',model:'claude-haiku-4-5'}},saved:true,tiers:['light','standard','heavy'],defaults:{}}

// The pathways API over an in-memory list. `box` records what the page sent; `failSave`, `failList`, `failCreate` and `failDelete` make the next such call (or, for `failList`, that many) fail.
function fixture({list=three(),failList=0,shelf=SHELF}={}){
 const box={list,saves:[],deletes:[],creates:[],failSave:'',failList,failCreate:'',failDelete:false,created:0,hold:null}
 const err=(status,code,error,field)=>({status,json:{error,code,...(field?{field}:{})}})
 const routes={
  [`${LIST}/skills`]:{skills:shelf},
  '/api/v2/terminal/prompts':{prompts:PROMPTS},
  '/api/v2/profile/model-favourites':{version:1,favourites:[{tool:'claude',model:'claude-opus-5-5',agentDefault:false}]},
  '/api/v2/gaffer/crew':r=>r.method()==='GET'?CREW:{status:405,json:{error:'no'}},
  [LIST]:r=>{
   if(r.method()==='POST'){
    const b=r.postDataJSON();box.creates.push(b)
    if(box.failCreate==='limit')return err(409,'pathway_limit','This account already has 50 pathways. Delete one before saving another.')
    if(box.failCreate==='name'){box.failCreate='';return err(409,'pathway_name','A pathway with that name already exists.')}
    if(box.failCreate==='invalid'){box.failCreate='';return err(400,'invalid','The description cannot hold control characters.','description')}
    const made={...b,id:`new${++box.created}`,createdAt:Date.now(),updatedAt:Date.now()};box.list=[...box.list,made];return{status:201,json:made}
   }
   if(box.failList>0){box.failList--;return{status:502,json:{error:'502 Bad Gateway'}}}
   return{pathways:box.list}
  },
 }
 // Each pathway's own path, for every id a test can reach (the three, and copies made by Duplicate).
 for(const id of ['p1','p2','p3','p4','p5','p6','new1','new2','new3'])routes[`${LIST}/${id}`]=async r=>{
  const m=r.method()
  // `hold`, when set, is a promise a PUT's answer waits for, so a test can act while a save is out.
  if(m==='PUT'&&box.hold)await box.hold
  const at=box.list.findIndex(p=>p.id===id)
  if(m==='PUT'){
   const b=r.postDataJSON();box.saves.push(b)
   if(box.failSave==='changed'){box.failSave='';return err(409,'pathway_changed','That pathway changed. Reload it before trying again.')}
   if(at>=0&&b.updatedAt!==box.list[at].updatedAt)return err(409,'pathway_changed','That pathway changed. Reload it before trying again.')
   if(box.failSave){box.failSave='';return{status:502,json:{error:'502 Bad Gateway'}}}
   // As the server answers: the pathway saved, with a new updatedAt and what would stop a start on it now.
   const saved={...b,updatedAt:Date.now()+box.saves.length,problems:box.list[at]?.problems??[]};box.list=box.list.map(p=>p.id===id?saved:p);return saved
  }
  if(m==='DELETE'){box.deletes.push(id);if(box.failDelete){box.failDelete=false;return{status:502,json:{error:'502 Bad Gateway'}}}box.list=box.list.filter(p=>p.id!==id);return{status:204,json:null}}
  return at<0?err(404,'','That pathway is not available.'):box.list[at]
 }
 return {box,routes}
}
const open=async(width,theme,over={},extra={})=>{
 // REET_UI_SCALE=2 takes the screenshots at twice the pixels, for the docs.
 const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:900,theme,touch:width<1000,routes:f.routes,scale:Number(process.env.REET_UI_SCALE)||undefined,...extra})
 return {f,w,page:w.page}
}
const noOverflow=async page=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true)
// Every visible control inside `scope` is at least 44 px tall (phones only).
async function targets(page,scope='main'){
 const small=await page.evaluate(sel=>[...document.querySelectorAll(`${sel} :is(button,a[href],[role=menuitem])`)].filter(el=>{const r=el.getBoundingClientRect();return r.width>0&&r.height>0}).map(el=>({name:(el.getAttribute('aria-label')||el.textContent||'').trim().slice(0,40),h:Math.round(el.getBoundingClientRect().height)})).filter(t=>t.h<44),scope)
 expect(small).toEqual([])
}
// Taken once the boot shell has faded and any dialog has finished opening.
const shot=async(page,name,width,theme)=>{await page.mouse.move(0,0);await page.waitForFunction(()=>{const b=document.getElementById('boot');return !b||getComputedStyle(b).opacity==='0'});await page.waitForTimeout(300);await page.screenshot({path:`${shots}/${name}-${width}-${theme}.png`})}
// Taken mid-drag, with the pointer where it is.
const held=async(page,name,width,theme)=>{await page.waitForTimeout(300);await page.screenshot({path:`${shots}/${name}-${width}-${theme}.png`})}
const row=(page,name)=>page.getByRole('listitem').filter({has:page.getByRole('heading',{name,exact:true})})
const actions=(page,name)=>page.getByRole('button',{name:`Actions for ${name}`,exact:true})
const goList=async page=>{await page.goto(base+'/factory/pathways');await page.getByRole('heading',{name:'Pathways',level:1,exact:true}).waitFor()}

// The rows: each pathway's name (a link into the editor), its stage line, its description and any problem.
async function listRows(width,theme){
 const {w,page}=await open(width,theme)
 try{
  await goList(page)
  await expect(page.getByText('Your own lines of stages, each with its model, skills and prompt.')).toBeVisible()
  await expect(page.getByRole('list',{name:'Pathways'}).getByRole('listitem').filter({has:page.getByRole('heading')})).toHaveCount(3)
  await expect(row(page,'Standard').getByRole('list',{name:'Stages of Standard'}).locator('.chip')).toHaveText(['Code','Review','Test','PR','Babysit','Merge'])
  await expect(row(page,'Careful release').getByRole('list',{name:'Stages of Careful release'}).locator('.chip')).toHaveText(['Code','Security pass','Review','PR','Merge'])
  await expect(row(page,'Careful release').locator('.chip.own')).toHaveText(['Security pass'])
  await expect(row(page,'Careful release')).toContainText('! The skill “lint-rules” on Code is no longer on your shelf.')
  await expect(row(page,'Docs only')).toContainText('Off: not offered to you or Gaffer.')
  await expect(page.getByRole('switch',{name:'Docs only is off, switch on',exact:true})).toHaveAttribute('aria-checked','false')
  await expect(page.getByRole('link',{name:'Standard',exact:true})).toHaveAttribute('href','/factory/pathways/p1')
  await noOverflow(page)
  if(width<1000)await targets(page)
  await shot(page,'list',width,theme)
  // The menu: Edit, Duplicate and Delete, inside the window.
  await actions(page,'Docs only').click()
  const menu=page.getByRole('menu',{name:'Actions for Docs only'})
  await expect(menu.getByRole('menuitem')).toHaveText(['Edit','Duplicate','Delete'])
  await expect(menu.getByRole('menuitem',{name:'Edit'})).toBeFocused()
  expect(await menu.evaluate(el=>{const r=el.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight})).toBe(true)
  // It never covers the button that opened it, and Delete is in the danger colour.
  const [m,b]=[await menu.boundingBox(),await actions(page,'Docs only').boundingBox()]
  expect(m.y>=b.y+b.height||m.y+m.height<=b.y).toBe(true)
  const colour=sel=>page.evaluate(s=>{const p=document.createElement('p');p.style.color=s;document.body.append(p);const c=getComputedStyle(p).color;p.remove();return c},sel)
  expect(await menu.getByRole('menuitem',{name:'Delete'}).evaluate(el=>getComputedStyle(el).color)).toBe(await colour('var(--bad)'))
  if(width<1000)await targets(page,'[role=menu]')
  await shot(page,'list-menu',width,theme)
  await page.keyboard.press('Escape')
  await expect(menu).toHaveCount(0)
  await expect(actions(page,'Docs only')).toBeFocused()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// The switch saves at once with the pathway's updatedAt, and a refused save puts it back with a notice.
async function listSwitch(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goList(page)
  const before=f.box.list[0].updatedAt
  await page.getByRole('switch',{name:'Standard is on, switch off',exact:true}).click()
  await expect(page.getByRole('switch',{name:'Standard is off, switch on',exact:true})).toHaveAttribute('aria-checked','false')
  expect(f.box.saves.length).toBe(1)
  expect(f.box.saves[0]).toMatchObject({id:'p1',name:'Standard',enabled:false,updatedAt:before})
  expect(f.box.saves[0].problems).toBeUndefined()
  await expect(row(page,'Standard')).toContainText('Off: not offered to you or Gaffer.')
  // A failed save rolls back.
  f.box.failSave='down'
  await page.getByRole('switch',{name:'Docs only is off, switch on',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('“Docs only” could not be switched on just now. Try again.')
  await expect(page.getByRole('switch',{name:'Docs only is off, switch on',exact:true})).toHaveAttribute('aria-checked','false')
  // Someone else's save first: the row is read again and the switch is as the server has it.
  f.box.failSave='changed'
  await page.getByRole('switch',{name:'Careful release is on, switch off',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('“Careful release” was changed somewhere else, so the list now shows it as it is saved.')
  await expect(page.getByRole('switch',{name:'Careful release is on, switch off',exact:true})).toHaveAttribute('aria-checked','true')
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// A change made here stays on screen from the server's answer, whatever a later read of the list does: the switch keeps its
// new value and its next save sends the new updatedAt, a deleted row goes, and a delete that fails keeps the row and says so.
async function listKeepsSaved(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goList(page)
  f.box.failList=5
  await page.getByRole('switch',{name:'Standard is on, switch off',exact:true}).click()
  await expect(page.getByRole('switch',{name:'Standard is off, switch on',exact:true})).toHaveAttribute('aria-checked','false')
  const first=f.box.list.find(p=>p.id==='p1').updatedAt
  expect(first).not.toBe(f.box.saves[0].updatedAt)
  await page.getByRole('switch',{name:'Standard is off, switch on',exact:true}).click()
  await expect(page.getByRole('switch',{name:'Standard is on, switch off',exact:true})).toHaveAttribute('aria-checked','true')
  expect(f.box.saves.map(s=>[s.enabled,s.updatedAt])).toEqual([[false,f.box.saves[0].updatedAt],[true,first]])
  await expect(page.getByRole('alert')).toHaveCount(0)
  // Delete, with the list still unreadable: the row goes.
  await actions(page,'Docs only').click()
  await page.getByRole('menuitem',{name:'Delete'}).click()
  await page.getByRole('dialog',{name:'Delete “Docs only”?'}).getByRole('button',{name:'Delete',exact:true}).click()
  await expect(row(page,'Docs only')).toHaveCount(0)
  await expect(page.getByRole('status').filter({hasText:'Sorted. “Docs only” is deleted.'})).toBeVisible()
  // A delete that fails keeps the row, says so, and gives focus back to the menu button.
  f.box.failDelete=true
  await actions(page,'Careful release').click()
  await page.getByRole('menuitem',{name:'Delete'}).click()
  await page.getByRole('dialog',{name:'Delete “Careful release”?'}).getByRole('button',{name:'Delete',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('“Careful release” could not be deleted just now. Try again.')
  await expect(row(page,'Careful release')).toBeVisible()
  await expect(actions(page,'Careful release')).toBeFocused()
  // A read of the list that fails once it is shown says so above it, with Retry, and keeps the rows.
  // A failed Duplicate gives focus back to the menu button too.
  f.box.failCreate='limit'
  await actions(page,'Careful release').click()
  await page.getByRole('menuitem',{name:'Duplicate'}).click()
  await expect(page.getByRole('alert')).toContainText('This account already has 50 pathways.')
  await expect(actions(page,'Careful release')).toBeFocused()
  f.box.failCreate=''
  await page.getByRole('link',{name:'Standard',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/p1')
  await page.goBack()
  await expect(page.getByRole('alert')).toContainText('Your pathways could not be read. The list below is from the last time they were.')
  await expect(row(page,'Careful release')).toBeVisible()
  f.box.failList=0
  await page.getByRole('button',{name:'Retry',exact:true}).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Delete asks first, says running work is untouched, then deletes and the row goes. Cancel keeps it.
async function listDelete(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goList(page)
  await actions(page,'Docs only').click()
  await page.getByRole('menuitem',{name:'Delete'}).click()
  const dialog=page.getByRole('dialog',{name:'Delete “Docs only”?'})
  await expect(dialog).toContainText('Work already running on it is not affected.')
  await shot(page,'delete',width,theme)
  await dialog.getByRole('button',{name:'Cancel'}).click()
  await expect(dialog).toHaveCount(0)
  expect(f.box.deletes).toEqual([])
  await actions(page,'Docs only').click()
  await page.getByRole('menuitem',{name:'Delete'}).click()
  await page.getByRole('dialog',{name:'Delete “Docs only”?'}).getByRole('button',{name:'Delete',exact:true}).click()
  await expect(row(page,'Docs only')).toHaveCount(0)
  expect(f.box.deletes).toEqual(['p3'])
  await expect(page.getByRole('status').filter({hasText:'Sorted. “Docs only” is deleted.'})).toBeVisible()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Duplicate makes "<name> copy" (then "copy 2"), with the same stages and on or off, and opens it; back returns to the list.
async function listDuplicate(width,theme){
 const {f,w,page}=await open(width,theme,{list:[...three(),pathway('p4','Docs only copy',[st('pr')])]})
 try{
  await goList(page)
  await actions(page,'Docs only').click()
  await page.getByRole('menuitem',{name:'Duplicate'}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new1')
  await expect(page.getByRole('heading',{name:'Pathway editor'})).toBeVisible()
  expect(f.box.creates).toEqual([{name:'Docs only copy 2',description:'Words, not code.',enabled:false,stages:docs().stages}])
  await page.goBack()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(row(page,'Docs only copy 2')).toBeVisible()
  // A name at the limit is cut so the copy still fits 60 characters.
  const long='L'.repeat(60)
  f.box.list=[...f.box.list,pathway('new2','x',[st('pr')],{name:long})];f.box.created=2
  await page.reload();await goList(page)
  await actions(page,long).click()
  await page.getByRole('menuitem',{name:'Duplicate'}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new3')
  expect(f.box.creates[1].name).toBe('L'.repeat(55)+' copy')
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// The name opens the editor in place; browser back comes back to the list.
async function listRoutes(width,theme){
 const {w,page}=await open(width,theme)
 try{
  await goList(page)
  await page.getByRole('link',{name:'Careful release',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/p2')
  await expect(page.getByRole('heading',{name:'Pathway editor'})).toBeVisible()
  await page.goBack()
  await expect(page.getByRole('heading',{name:'Pathways',level:1})).toBeVisible()
  await page.goForward()
  await expect(page.getByRole('heading',{name:'Pathway editor'})).toBeVisible()
  await page.getByRole('link',{name:'Pathways',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(row(page,'Standard')).toBeVisible()
  // New pathway goes to the new address.
  await page.getByRole('button',{name:'New pathway',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new')
  await expect(page.getByRole('heading',{name:'Pathway editor'})).toBeVisible()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// The Factory page's Pathways link lands on the list; back returns to the Factory.
async function fromFactory(width,theme){
 const {w,page}=await open(width,theme)
 try{
  await page.goto(base+'/factory')
  await page.getByRole('heading',{name:'Factory',level:1}).waitFor()
  await page.getByRole('link',{name:'Pathways',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(page.getByRole('heading',{name:'Pathways',level:1})).toBeVisible()
  await expect(page).toHaveTitle(/^Pathways · reet\.sh$/)
  // The page header leads back to the Factory, and the Factory tab is current.
  await expect(page.getByRole('link',{name:'Factory',exact:true}).and(page.locator('[aria-current=page]'))).toHaveCount(1)
  await page.goBack()
  await expect(page).toHaveURL(base+'/factory')
  await expect(page.getByRole('heading',{name:'Factory',level:1})).toBeVisible()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Nothing saved yet: a sentence, New pathway and Start from the standard stages.
async function listEmpty(width,theme){
 const {w,page}=await open(width,theme,{list:[]})
 try{
  await goList(page)
  await expect(page.getByText('A pathway is a line of stages you choose once and reuse')).toBeVisible()
  await expect(page.getByRole('button',{name:'New pathway',exact:true})).toHaveCount(1)
  await noOverflow(page)
  if(width<1000)await targets(page)
  await shot(page,'empty',width,theme)
  await page.getByRole('button',{name:'Start from the standard stages',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new')
  await expect(page.getByRole('heading',{name:'Pathway editor'})).toBeVisible()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Loading keeps the page's shape; a failed read says so with Retry; at 50 New pathway is off and says why.
async function listStates(width,theme){
 let release
 const gate=new Promise(r=>{release=r})
 const {w,page}=await open(width,theme,{failList:1},{gates:{[LIST]:gate}})
 try{
  await page.goto(base+'/factory/pathways')
  await expect(page.getByRole('status',{name:'Reading your pathways…'})).toBeVisible()
  await shot(page,'loading',width,theme)
  release()
  await expect(page.getByRole('alert')).toContainText('Your pathways could not be read.')
  await shot(page,'failed',width,theme)
  await page.getByRole('button',{name:'Retry',exact:true}).click()
  await expect(row(page,'Standard')).toBeVisible()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
 const full=Array.from({length:50},(_,n)=>pathway(`q${n}`,`Line ${n+1}`,[st('code')]))
 const {w:w2,page:page2}=await open(width,theme,{list:full})
 try{
  await goList(page2)
  const add=page2.getByRole('button',{name:'New pathway',exact:true})
  await expect(add).toBeDisabled()
  await expect(add).toHaveAccessibleDescription('You have 50 pathways, the most you can keep. Delete one to make another.')
  await expect(page2.getByText('You have 50 pathways, the most you can keep.')).toBeVisible()
 }finally{await w2.context.close()}
}

// The editor. Cards are found by their title; the palette and the inspector by their regions.
const audit=()=>pathway('p4','Ship with audit',[st('code'),st('review',{tier:'heavy'}),st('test'),
 st('custom',{name:'Security audit',prompt:'Check every new endpoint for auth. List what you find, worst first.',agent:{tool:'claude',model:'claude-opus-5-5',effort:'high'},skills:['threat-model','deps-check']}),
 st('pr',{tier:'light'}),st('merge',{tier:'light'})],{description:'For changes that touch sign-in or billing.'})
const everything=()=>pathway('p5','Everything, carefully',[...['plan','design','code','review','test'].map(k=>st(k)),
 ...['Threat model','Security audit','Load test','Docs pass'].map(name=>st('custom',{name,prompt:`Do the ${name.toLowerCase()}.`})),...['pr','babysit','merge'].map(k=>st(k))])
const cards=page=>page.getByRole('list',{name:'Stages'}).locator('[data-stage-index]')
const card=(page,title)=>cards(page).filter({has:page.locator('.title').getByText(title,{exact:true})})
const titles=page=>cards(page).locator('.title').allTextContents()
const palette=page=>page.getByRole('region',{name:'Palette'})
const inspector=page=>page.getByRole('region',{name:'Selected stage'})
const live=page=>page.locator('[data-pathway-live]')
const tab=(page,name)=>palette(page).getByRole('tab',{name,exact:true})
const item=(page,name)=>palette(page).locator(`[data-palette-item="${name}"]`)
// On a phone the palette is a sheet that Add a stage opens; on a desktop it is always there.
const showPalette=async(page,width)=>{if(width<700&&!await page.getByRole('dialog',{name:'Add a stage'}).count())await page.getByRole('button',{name:'Add a stage'}).click()}
const closeSheet=async(page,width)=>{if(width<700)await page.getByRole('dialog').getByRole('button',{name:'Close'}).click()}
const closeAny=async page=>{const d=page.locator('dialog[open]');if(await d.count())await d.getByRole('button',{name:'Close',exact:true}).click()}
// A stage's controls: the inspector on a desktop; on a phone the stage's sheet, opened from its card on `part`.
const stageControls=async(page,width,title,part)=>{if(width<700){await closeAny(page);await card(page,title).click();if(part)await inspector(page).getByRole('tab',{name:part,exact:true}).click()}return inspector(page)}
// A palette tab: always there on a desktop; on a phone in the sheet Add a stage opens.
const paletteTab=async(page,width,name)=>{if(width<700&&!await page.getByRole('dialog',{name:'Add a stage'}).count()){await closeAny(page);await page.getByRole('button',{name:'Add a stage'}).click()}await tab(page,name).click()}
const goEditor=async(page,id)=>{await page.goto(`${base}/factory/pathways/${id}`);await page.getByRole('heading',{name:'Pathway editor'}).waitFor();await cards(page).first().waitFor()}

// Built by keyboard and buttons from the standard stages, then saved: the body is what the server is sent. On a phone the
// palette and the stage's controls are sheets, and the line runs top to bottom.
async function editorBuild(width,theme){
 const {f,w,page}=await open(width,theme,{list:[]})
 const earlier=width<700?'Alt+ArrowUp':'Alt+ArrowLeft'
 try{
  await goList(page)
  await page.getByRole('button',{name:'Start from the standard stages',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new')
  await expect.poll(()=>titles(page)).toEqual(['Code','Review','Test','PR','Babysit','Merge'])
  await page.getByRole('textbox',{name:'Pathway name'}).fill('Ship with audit')
  // Your own stage goes in before Merge, is selected and named "Stage 1".
  await paletteTab(page,width,'Stages')
  await item(page,'stage:custom').click()
  await expect.poll(()=>titles(page)).toEqual(['Code','Review','Test','PR','Babysit','Stage 1','Merge'])
  await expect(card(page,'Stage 1')).toHaveAttribute('aria-pressed','true')
  let controls=await stageControls(page,width,'Stage 1','Prompt')
  await expect(controls.getByText('Stage 6 of 7')).toBeVisible()
  await controls.getByRole('textbox',{name:'Name',exact:true}).fill('Security audit')
  await controls.getByRole('textbox',{name:'Prompt'}).fill('Check every new endpoint.')
  await expect(controls).toContainText('25 of 4,000 bytes')
  // Two places earlier by keyboard, each heard.
  await closeAny(page)
  await card(page,'Security audit').focus()
  await page.keyboard.press(earlier)
  await expect(live(page)).toHaveText('Security audit moved to place 5 of 7')
  await page.keyboard.press(earlier)
  await expect(live(page)).toHaveText('Security audit moved to place 4 of 7')
  await expect.poll(()=>titles(page)).toEqual(['Code','Review','Test','Security audit','PR','Babysit','Merge'])
  await expect(card(page,'Security audit')).toBeFocused()
  // A move the rules refuse is said, and nothing moves: Merge cannot go earlier. Said again, it is heard again.
  await card(page,'Merge').focus()
  await page.keyboard.press(earlier)
  await expect(live(page)).toHaveText('Merge is always the last stage.')
  await page.keyboard.press(earlier)
  await expect(live(page)).toHaveText('Merge is always the last stage.')
  await card(page,'Security audit').click()
  // A named model, from the favourites, then an effort.
  controls=await stageControls(page,width,'Security audit','Model')
  if(width<700)await controls.getByRole('button',{name:'A named model'}).click()
  else await controls.getByRole('radio',{name:'A named model'}).check()
  await paletteTab(page,width,'Models')
  await item(page,'model:claude/claude-opus-5-5').click()
  controls=await stageControls(page,width,'Security audit','Model')
  await controls.getByRole('combobox',{name:'Effort'}).selectOption('high')
  await expect(card(page,'Security audit')).toContainText('claude-opus-5-5 · high')
  await expect(controls.getByText('A named model has no stand-in when its plan runs low. A crew tier does.')).toBeVisible()
  // Two skills, and the third is refused with its reason.
  await paletteTab(page,width,'Skills')
  await item(page,'skill:threat-model').click()
  await item(page,'skill:deps-check').click()
  await expect(item(page,'skill:test-first')).toBeDisabled()
  await expect(item(page,'skill:test-first')).toContainText('A stage can have up to 2 skills.')
  controls=await stageControls(page,width,'Security audit','Skills')
  await expect(controls.getByText(width<700?'2 of 2 skills.':'Skills (2 of 2)')).toBeVisible()
  await expect(card(page,'Security audit')).toContainText('2 skills · prompt ✓')
  // A library prompt is added after what was typed.
  await paletteTab(page,width,'Prompts')
  await item(page,'prompt:pr1').click()
  controls=await stageControls(page,width,'Security audit','Prompt')
  await expect(controls.getByRole('textbox',{name:'Prompt'})).toHaveValue('Check every new endpoint.\n\nCheck every new endpoint for auth.\nList what you find.')
  // A second Code is not offered.
  await paletteTab(page,width,'Stages')
  await expect(item(page,'stage:code')).toBeDisabled()
  await expect(item(page,'stage:code')).toContainText('in use')
  await closeAny(page)
  await expect(page.getByText('Not saved yet')).toBeVisible()
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways/new1')
  expect(f.box.creates).toEqual([{name:'Ship with audit',description:'',enabled:true,stages:[
   {kind:'code',tier:'standard'},{kind:'review',tier:'heavy'},{kind:'test',tier:'standard'},
   {kind:'custom',name:'Security audit',prompt:'Check every new endpoint.\n\nCheck every new endpoint for auth.\nList what you find.',agent:{tool:'claude',model:'claude-opus-5-5',effort:'high'},skills:['threat-model','deps-check']},
   {kind:'pr',tier:'light'},{kind:'babysit',tier:'light'},{kind:'merge',tier:'light'}]}])
  await expect(page.getByText('Unsaved changes')).toHaveCount(0)
  await expect(page).toHaveTitle('Ship with audit · Pathways · reet.sh')
  // An edit, then the back link: asked first; Keep editing stays, Discard changes leaves without saving.
  await page.getByRole('textbox',{name:'Pathway name'}).fill('Ship with audit 2')
  await expect(page.getByText('Unsaved changes')).toBeVisible()
  await page.getByRole('link',{name:'Pathways',exact:true}).click()
  const ask=page.getByRole('dialog',{name:'Leave without saving?'})
  await ask.getByRole('button',{name:'Keep editing'}).click()
  await expect(ask).toHaveCount(0)
  await expect(page.getByRole('textbox',{name:'Pathway name'})).toHaveValue('Ship with audit 2')
  await page.getByRole('link',{name:'Pathways',exact:true}).click()
  await ask.getByRole('button',{name:'Discard changes'}).click()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(row(page,'Ship with audit')).toBeVisible()
  expect(f.box.saves).toEqual([])
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Typing while a save is out: the typed text stays, is unsaved, and the next Save goes on the answer's updatedAt.
async function editorSaveInFlight(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goEditor(page,'p1')
  const name=page.getByRole('textbox',{name:'Pathway name'})
  await name.fill('Standard A')
  let release
  f.box.hold=new Promise(r=>{release=r})
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByText('Saving…').first()).toBeVisible()
  await name.fill('Standard B')
  f.box.hold=null
  release()
  await expect.poll(()=>f.box.saves.length).toBe(1)
  await expect(page.getByText('Unsaved changes')).toBeVisible()
  await expect(name).toHaveValue('Standard B')
  const answered=f.box.list.find(p=>p.id==='p1').updatedAt
  expect(answered).not.toBe(f.box.saves[0].updatedAt)
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByText('✓ Saved')).toBeVisible()
  expect(f.box.saves[1]).toMatchObject({name:'Standard B',updatedAt:answered})
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Browser back and closing the tab ask first; Discard puts the saved pathway back.
async function editorLeave(width,theme){
 const {w,page}=await open(width,theme)
 try{
  await goList(page)
  await page.getByRole('link',{name:'Standard',exact:true}).click()
  await cards(page).first().waitFor()
  const description=page.getByRole('textbox',{name:'Description'})
  await description.fill('Changed.')
  await page.goBack()
  const ask=page.getByRole('dialog',{name:'Leave without saving?'})
  await expect(ask).toBeVisible()
  await expect(page).toHaveURL(base+'/factory/pathways/p1')
  await shot(page,'editor-leave',width,theme)
  await ask.getByRole('button',{name:'Keep editing'}).click()
  await expect(description).toHaveValue('Changed.')
  await page.getByRole('button',{name:'Discard',exact:true}).click()
  await expect(description).toHaveValue('The usual line, from code to merge.')
  await expect(page.getByText('Unsaved changes')).toHaveCount(0)
  await description.fill('Changed again.')
  await page.goBack()
  await ask.getByRole('button',{name:'Discard changes'}).click()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(row(page,'Standard')).toContainText('The usual line, from code to merge.')
  // Reached by browser back, then forward with an edit: Discard changes throws the edits away and goes on, and the guard still asks next time.
  await page.getByRole('link',{name:'Standard',exact:true}).click()
  await cards(page).first().waitFor()
  await page.getByRole('link',{name:'Pathways',exact:true}).click()
  await expect(page).toHaveURL(base+'/factory/pathways')
  await page.goBack()
  await expect(page).toHaveURL(base+'/factory/pathways/p1')
  await description.fill('Changed, then forward.')
  await page.goForward()
  await expect(page).toHaveURL(base+'/factory/pathways/p1')
  await ask.getByRole('button',{name:'Discard changes'}).click()
  await expect(ask).toHaveCount(0)
  await expect(page).toHaveURL(base+'/factory/pathways')
  await expect(row(page,'Standard')).toContainText('The usual line, from code to merge.')
  await page.goForward()
  await expect(page).toHaveURL(base+'/factory/pathways/p1')
  await expect(description).toHaveValue('The usual line, from code to merge.')
  await expect(page.getByText('Unsaved changes')).toHaveCount(0)
  await description.fill('Changed after discarding.')
  await page.goBack()
  await expect(ask).toBeVisible()
  await ask.getByRole('button',{name:'Keep editing'}).click()
  await expect(description).toHaveValue('Changed after discarding.')
  // Closing the tab with an edit asks the browser's own question.
  await description.fill('Changed once more.')
  const asked=page.waitForEvent('dialog')
  await page.close({runBeforeUnload:true})
  const dialog=await asked
  expect(dialog.type()).toBe('beforeunload')
  await dialog.dismiss()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Someone else's save landed first: the editor says so, keeps the edits, and offers both ways on.
async function editorChanged(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goEditor(page,'p1')
  const name=page.getByRole('textbox',{name:'Pathway name'})
  await name.fill('Standard line')
  const elsewhere=now+5000
  f.box.list=f.box.list.map(p=>p.id==='p1'?{...p,name:'Standard, as saved elsewhere',updatedAt:elsewhere}:p)
  await page.getByRole('button',{name:'Save',exact:true}).click()
  const alert=page.getByRole('alert').filter({hasText:'was changed somewhere else'})
  await expect(alert).toBeVisible()
  await expect(alert.getByRole('button',{name:'Load the saved one'})).toBeVisible()
  await expect(name).toHaveValue('Standard line')
  await shot(page,'editor-changed',width,theme)
  // Keep editing: the next Save is the person's, over the newer one.
  await alert.getByRole('button',{name:'Keep editing'}).click()
  await expect(alert).toHaveCount(0)
  await expect(name).toHaveValue('Standard line')
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByText('✓ Saved')).toBeVisible()
  expect(f.box.saves.at(-1)).toMatchObject({name:'Standard line',updatedAt:elsewhere})
  // Load the saved one: the newer one replaces the edits.
  await name.fill('Standard mine')
  f.box.list=f.box.list.map(p=>p.id==='p1'?{...p,name:'Standard theirs',updatedAt:elsewhere+1}:p)
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await page.getByRole('button',{name:'Load the saved one'}).click()
  await expect(name).toHaveValue('Standard theirs')
  await expect(page.getByText('Unsaved changes')).toHaveCount(0)
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// A skill gone from the shelf: ! on the card and in the inspector, Save says why, and removing it lets Save go.
async function editorMissingSkill(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goEditor(page,'p2')
  await expect(card(page,'Code')).toContainText('!')
  const save=page.getByRole('button',{name:'Save',exact:true})
  await expect(save).toBeDisabled()
  await expect(page.getByRole('list',{name:'Why Save is off'})).toContainText('The skill lint-rules is no longer available. Remove it.')
  await shot(page,'editor-problems',width,theme)
  await card(page,'Code').click()
  await expect(inspector(page)).toContainText('The skill lint-rules is no longer available.')
  await inspector(page).getByRole('button',{name:'Remove skill lint-rules'}).click()
  await closeSheet(page,width)
  await expect(save).toBeEnabled()
  await save.click()
  await expect(page.getByText('✓ Saved')).toBeVisible()
  expect(f.box.saves[0].stages[0]).toEqual({kind:'code',tier:'standard'})
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// A new, empty pathway; an order rule refused by button with a sentence; the server's refusals under their fields.
async function editorRefusals(width,theme){
 const {f,w,page}=await open(width,theme)
 try{
  await goList(page)
  await page.getByRole('button',{name:'New pathway',exact:true}).click()
  await expect(page.getByText('No stages yet.')).toBeVisible()
  await expect(page.getByRole('button',{name:'Save',exact:true})).toBeDisabled()
  await expect(page.getByRole('list',{name:'Why Save is off'})).toContainText('Add at least one stage.')
  if(width>=700)await expect(page.getByText('Select a stage to set its model, skills and prompt.')).toBeVisible()
  await shot(page,'editor-empty',width,theme)
  await showPalette(page,width)
  await item(page,'stage:merge').click()
  await expect(palette(page).getByRole('status')).toHaveText('Merge needs a PR stage before it.')
  await expect(cards(page)).toHaveCount(0)
  await item(page,'stage:code').click()
  await expect.poll(()=>titles(page)).toEqual(['Code'])
  await page.getByRole('textbox',{name:'Pathway name'}).fill('Docs only')
  f.box.failCreate='name'
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('textbox',{name:'Pathway name'})).toHaveAccessibleDescription('You already have a pathway with that name.')
  await page.getByRole('textbox',{name:'Pathway name'}).fill('Docs first')
  f.box.failCreate='invalid'
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('textbox',{name:'Description'})).toHaveAccessibleDescription('The description cannot hold control characters.')
  f.box.failCreate='limit'
  await page.getByRole('button',{name:'Save',exact:true}).click()
  await expect(page.getByRole('alert')).toContainText('This account already has 50 pathways.')
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Twelve stages: every card reached by keyboard and scrolled into view when selected; nothing more can be added.
async function editorLong(width,theme){
 const {w,page}=await open(width,theme,{list:[...three(),everything()]})
 try{
  await goEditor(page,'p5')
  await expect(cards(page)).toHaveCount(12)
  await showPalette(page,width)
  await expect(item(page,'stage:custom')).toBeDisabled()
  await expect(item(page,'stage:custom')).toContainText('A pathway can have up to 12 stages.')
  await closeSheet(page,width)
  await cards(page).first().focus()
  for(let n=1;n<12;n++)await page.keyboard.press('Tab')
  await expect(cards(page).last()).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(cards(page).last()).toHaveAttribute('aria-pressed','true')
  await expect(inspector(page).getByText('Stage 12 of 12')).toBeVisible()
  await expect(cards(page).last()).toBeInViewport()
  const scroller=await page.locator('[data-pathway-line]').boundingBox(),last=await cards(page).last().boundingBox()
  expect(last.x>=scroller.x-1&&last.x+last.width<=scroller.x+scroller.width+1).toBe(true)
  await noOverflow(page)
  await shot(page,'editor-long',width,theme)
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// The look at desktop: a custom stage on a named model selected, then the Skills tab.
async function editorLook(width,theme){
 const {w,page}=await open(width,theme,{list:[...three(),audit()]})
 try{
  await goEditor(page,'p4')
  await card(page,'Security audit').click()
  await expect(inspector(page).getByRole('radio',{name:'A named model'})).toBeChecked()
  await expect(card(page,'Merge')).toContainText('Built in · waits for you')
  await expect(card(page,'Merge')).toContainText('Always last')
  await expect(card(page,'Security audit')).toContainText('Your own')
  await noOverflow(page)
  await shot(page,'editor',width,theme)
  await tab(page,'Skills').click()
  await expect(item(page,'skill:threat-model')).toContainText('Find the ways in before someone else does.')
  await shot(page,'editor-skills',width,theme)
  // Remove in the inspector: focus goes to the card now in that place, then to + once the line is empty.
  await inspector(page).getByRole('button',{name:'Remove',exact:true}).click()
  await expect(cards(page).nth(3)).toBeFocused()
  await expect(cards(page).nth(3).locator('.title')).toHaveText('PR')
  for(let n=5;n>0;n--){await cards(page).first().click();await inspector(page).getByRole('button',{name:'Remove',exact:true}).click()}
  await expect(page.getByRole('button',{name:'Add a stage'})).toBeFocused()
  // Earlier stays focusable at the first place, so focus is not lost there.
  await tab(page,'Stages').click()
  await item(page,'stage:plan').click()
  await item(page,'stage:design').click()
  await inspector(page).getByRole('button',{name:'Earlier',exact:true}).click()
  await expect(inspector(page).getByRole('button',{name:'Earlier',exact:true})).toHaveAttribute('aria-disabled','true')
  await expect(inspector(page).getByRole('button',{name:'Earlier',exact:true})).toBeFocused()
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// A phone: the line top to bottom, the stage's controls in a bottom sheet with three tabs, and adding through the palette sheet.
async function editorPhone(width,theme){
 const {w,page}=await open(width,theme,{list:[...three(),audit()]})
 try{
  await goEditor(page,'p4')
  await noOverflow(page)
  await targets(page)
  const [a,b]=[await cards(page).nth(0).boundingBox(),await cards(page).nth(1).boundingBox()]
  expect(b.y>a.y+a.height).toBe(true)
  await card(page,'Security audit').click()
  const sheet=page.getByRole('dialog',{name:'Security audit'})
  await expect(sheet).toBeVisible()
  for(const name of ['Model','Skills','Prompt']){
   await sheet.getByRole('tab',{name,exact:true}).click()
   await expect(sheet.getByRole('tab',{name,exact:true})).toHaveAttribute('aria-selected','true')
   await targets(page,'dialog[open]')
   await shot(page,`editor-sheet-${name.toLowerCase()}`,width,theme)
  }
  await sheet.getByRole('tab',{name:'Skills',exact:true}).click()
  await expect(sheet.getByRole('button',{name:/^threat-model/})).toContainText('✓')
  await sheet.getByRole('tab',{name:'Model',exact:true}).click()
  await sheet.getByRole('button',{name:/^Heavy/}).click()
  await sheet.getByRole('button',{name:'Move earlier'}).click()
  await expect(live(page)).toHaveText('Security audit moved to place 3 of 6')
  await sheet.getByRole('button',{name:'Close'}).click()
  await expect(sheet).toHaveCount(0)
  await expect(card(page,'Security audit')).toContainText('heavy · crew')
  await page.getByRole('button',{name:'Add a stage'}).click()
  const add=page.getByRole('dialog',{name:'Add a stage'})
  await expect(add).toBeVisible()
  await shot(page,'editor-sheet-add',width,theme)
  await add.locator('[data-palette-item="stage:plan"]').click()
  await expect(add).toHaveCount(0)
  await expect.poll(()=>titles(page)).toEqual(['Code','Review','Security audit','Test','PR','Plan','Merge'])
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Dragging, with real pointer input. `drag` presses on `from`, moves in steps to the point `to` and lets go, or with `hold` keeps
// the button down for a look mid-drag. `between` is the point in the gap between two cards (null for either end of the line).
// Both measure once the cards have finished sliding.
const centre=box=>({x:box.x+box.width/2,y:box.y+box.height/2})
const settled=page=>page.waitForFunction(()=>!document.querySelector('[data-pathway-line]').getAnimations({subtree:true}).length)
const over=async(page,title)=>{await settled(page);return centre(await card(page,title).boundingBox())}
async function between(page,a,b,vertical=false){
 await settled(page)
 const [l,r]=[a?await card(page,a).boundingBox():null,b?await card(page,b).boundingBox():null]
 const one=l??r
 if(vertical)return {x:one.x+one.width/2,y:((l?l.y+l.height:r.y-14)+(r?r.y:l.y+l.height+14))/2}
 return {x:((l?l.x+l.width:r.x-40)+(r?r.x:l.x+l.width+40))/2,y:one.y+one.height/2}
}
async function drag(page,from,to,{hold=false}={}){
 const a=centre(await from.boundingBox())
 await page.mouse.move(a.x,a.y);await page.mouse.down()
 await page.mouse.move(a.x+10,a.y+4,{steps:2})
 await page.mouse.move(to.x,to.y,{steps:14})
 if(!hold)await page.mouse.up()
}
const bar=page=>page.locator('[data-drag-bar]')
const hint=page=>page.locator('[data-drag-hint]')
const planFirst=()=>pathway('p6','Plan first',[st('plan'),st('code'),st('custom',{name:'Security audit',prompt:'Check every new endpoint for auth.',skills:['threat-model','deps-check']}),st('pr')])

// A stage block on to the line, a card along it, and a skill, a model and a prompt on to a card; Escape puts everything back.
async function editorDrag(width,theme){
 const {w,page}=await open(width,theme,{list:[...three(),planFirst()]})
 try{
  await goEditor(page,'p6')
  await drag(page,item(page,'stage:design'),await between(page,'Plan','Code'),{hold:true})
  await expect(bar(page)).toBeVisible()
  await expect(hint(page)).toHaveText('Drop to add Design at place 2.')
  await held(page,'drag-stage',width,theme)
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Plan','Design','Code','Security audit','PR'])
  await expect(card(page,'Design')).toHaveAttribute('aria-pressed','true')
  await expect(bar(page)).toHaveCount(0)
  await expect(hint(page)).toHaveCount(0)
  // A card two places later.
  await drag(page,card(page,'Plan'),await between(page,'Code','Security audit'),{hold:true})
  await expect(bar(page)).toBeVisible()
  await expect(hint(page)).toHaveText('Drop to move Plan to place 3.')
  await held(page,'drag-card',width,theme)
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Design','Code','Plan','Security audit','PR'])
  await expect(live(page)).toHaveText('Plan moved to place 3 of 5')
  // A skill over a card that can take it: a ring and a preview; the full card is dimmed and says why.
  await tab(page,'Skills').click()
  await drag(page,item(page,'skill:test-first'),await over(page,'Code'),{hold:true})
  await expect(card(page,'Code')).toHaveAttribute('data-drop','take')
  await expect(card(page,'Code')).toContainText('+ test-first')
  await expect(card(page,'Security audit')).toHaveAttribute('data-drop','refuse')
  await expect(card(page,'Security audit')).toContainText('2 skills · full')
  await expect(hint(page)).toHaveText('Drop to give Code the skill test-first. Security audit already has 2.')
  await held(page,'drag-skill',width,theme)
  await page.mouse.up()
  await expect(card(page,'Code')).toContainText('1 skill')
  await expect(card(page,'Code')).toHaveAttribute('aria-pressed','true')
  await expect(card(page,'Security audit')).not.toHaveAttribute('data-drop')
  // Over the full card: the reason, and the drop changes nothing.
  await drag(page,item(page,'skill:test-first'),await over(page,'Security audit'),{hold:true})
  await expect(hint(page)).toHaveText('Security audit cannot take test-first. A stage can have up to 2 skills.')
  await page.mouse.up()
  await expect(card(page,'Security audit')).toContainText('2 skills · prompt ✓')
  await expect(card(page,'Code')).toHaveAttribute('aria-pressed','true')
  // A model; then a prompt, added after what the stage already has, as pressing it does.
  await tab(page,'Models').click()
  await drag(page,item(page,'tier:heavy'),await over(page,'Code'))
  await expect(card(page,'Code')).toContainText('heavy · crew')
  await tab(page,'Prompts').click()
  await drag(page,item(page,'prompt:pr1'),await over(page,'Security audit'),{hold:true})
  await expect(hint(page)).toHaveText('Drop to add this prompt to Security audit.')
  await expect(card(page,'Security audit')).toContainText('+ prompt')
  await page.mouse.up()
  await expect(inspector(page).getByRole('textbox',{name:'Prompt'})).toHaveValue('Check every new endpoint for auth.\n\nCheck every new endpoint for auth.\nList what you find.')
  // Escape mid-drag: nothing moves, the press does not select, and the page can be selected again.
  await drag(page,card(page,'Design'),await between(page,'Plan','Security audit'),{hold:true})
  await expect(bar(page)).toBeVisible()
  expect(await page.evaluate(()=>document.documentElement.style.userSelect)).toBe('none')
  await page.keyboard.press('Escape')
  await expect(bar(page)).toHaveCount(0)
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Design','Code','Plan','Security audit','PR'])
  await expect(card(page,'Security audit')).toHaveAttribute('aria-pressed','true')
  expect(await page.evaluate(()=>document.documentElement.style.userSelect)).toBe('')
  // The window losing focus mid-drag cancels it: the ghost goes, the page is itself again, and nothing moves.
  await drag(page,card(page,'Design'),await between(page,'Plan','Security audit'),{hold:true})
  await expect(page.locator('.ghost')).toHaveCount(1)
  await page.evaluate(()=>window.dispatchEvent(new Event('blur')))
  await expect(page.locator('.ghost')).toHaveCount(0)
  await expect(bar(page)).toHaveCount(0)
  expect(await page.evaluate(()=>document.documentElement.hasAttribute('data-pathway-drag'))).toBe(false)
  expect(await page.evaluate(()=>document.documentElement.style.userSelect)).toBe('')
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Design','Code','Plan','Security audit','PR'])
  // A press that does not move is still a press.
  await card(page,'Code').click()
  await expect(card(page,'Code')).toHaveAttribute('aria-pressed','true')
  await tab(page,'Stages').click()
  await item(page,'stage:test').click()
  await expect.poll(()=>titles(page)).toEqual(['Design','Code','Plan','Security audit','PR','Test'])
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// An order rule is never offered: Merge is offered only after PR, the Merge card cannot be dragged earlier, no card can go
// past Merge, and PR cannot go after Babysit.
async function editorDragRules(width,theme){
 const {w,page}=await open(width,theme)
 try{
  await goEditor(page,'p3')
  await drag(page,item(page,'stage:merge'),await between(page,null,'Write docs'),{hold:true})
  await expect(hint(page)).toHaveText('Merge is always the last stage.')
  await expect(bar(page)).toHaveCount(0)
  let to=await between(page,'Write docs','PR')
  await page.mouse.move(to.x,to.y,{steps:6})
  await expect(hint(page)).toHaveText('Merge is always the last stage.')
  await expect(bar(page)).toHaveCount(0)
  to=await between(page,'PR',null)
  await page.mouse.move(to.x,to.y,{steps:6})
  await expect(bar(page)).toBeVisible()
  await expect(hint(page)).toHaveText('Drop to add Merge at place 3.')
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Write docs','PR','Merge'])
  await drag(page,card(page,'Merge'),await between(page,null,'Write docs'),{hold:true})
  await expect(hint(page)).toHaveText('Merge is always the last stage.')
  await expect(bar(page)).toHaveCount(0)
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Write docs','PR','Merge'])
  await item(page,'stage:babysit').click()
  await expect.poll(()=>titles(page)).toEqual(['Write docs','PR','Babysit','Merge'])
  await drag(page,card(page,'Write docs'),await between(page,'Merge',null),{hold:true})
  await expect(hint(page)).toHaveText('Merge is always the last stage.')
  await expect(bar(page)).toHaveCount(0)
  await page.mouse.up()
  await drag(page,card(page,'PR'),await between(page,'Babysit','Merge'),{hold:true})
  await expect(hint(page)).toHaveText('Babysit needs a PR stage before it.')
  await expect(bar(page)).toHaveCount(0)
  await page.mouse.up()
  await expect.poll(()=>titles(page)).toEqual(['Write docs','PR','Babysit','Merge'])
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

// Touch on a phone: hold a card and drag it down one place; a quick swipe scrolls and moves nothing.
async function editorTouch(width,theme){
 const {w,page}=await open(width,theme,{list:[...three(),everything()]})
 try{
  await goEditor(page,'p5')
  await shot(page,'editor-phone',width,theme)
  const cdp=await page.context().newCDPSession(page)
  const touch=(type,p)=>cdp.send('Input.dispatchTouchEvent',{type,touchPoints:p?[{x:Math.round(p.x),y:Math.round(p.y)}]:[]})
  const scrolled=()=>page.evaluate(()=>{let e=document.querySelector('[data-pathway-line]');while(e&&!(e.scrollHeight>e.clientHeight&&/auto|scroll/.test(getComputedStyle(e).overflowY)))e=e.parentElement;return (e??document.scrollingElement).scrollTop})
  const from=await over(page,'Design'),to=await between(page,'Code','Review',true)
  await touch('touchStart',from)
  await page.waitForTimeout(400)
  for(let n=1;n<=10;n++)await touch('touchMove',{x:from.x,y:from.y+(to.y-from.y)*n/10})
  await expect(bar(page)).toBeVisible()
  await expect(hint(page)).toHaveText('Drop to move Design to place 3.')
  await held(page,'drag-phone',width,theme)
  await touch('touchEnd')
  await expect.poll(()=>titles(page)).toEqual(['Plan','Code','Design','Review','Test','Threat model','Security audit','Load test','Docs pass','PR','Babysit','Merge'])
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(await page.evaluate(()=>document.documentElement.style.userSelect)).toBe('')
  const top=await scrolled(),start=await over(page,'Load test')
  await touch('touchStart',start)
  for(let n=1;n<=8;n++)await touch('touchMove',{x:start.x,y:start.y-n*30})
  await touch('touchEnd')
  await expect.poll(scrolled).toBeGreaterThan(top)
  await expect.poll(()=>titles(page)).toEqual(['Plan','Code','Design','Review','Test','Threat model','Security audit','Load test','Docs pass','PR','Babysit','Merge'])
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(bar(page)).toHaveCount(0)
  expect(w.errors).toEqual([])
 }finally{await w.context.close()}
}

try{
 for(const [width,theme] of [[1280,'light'],[1280,'dark'],[390,'light'],[390,'dark']]){
  await listRows(width,theme)
  await listSwitch(width,theme)
  await listKeepsSaved(width,theme)
  await listEmpty(width,theme)
 }
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  await listDelete(width,theme)
  await listDuplicate(width,theme)
  await listRoutes(width,theme)
  await fromFactory(width,theme)
  await listStates(width,theme)
 }
 for(const [width,theme] of [[1280,'light'],[1280,'dark']]){
  await editorLook(width,theme)
  await editorBuild(width,theme)
 }
 await editorBuild(390,'light')
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  await editorLeave(width,theme)
  await editorChanged(width,theme)
  await editorSaveInFlight(width,theme)
  await editorMissingSkill(width,theme)
  await editorRefusals(width,theme)
  await editorLong(width,theme)
 }
 await editorLong(390,'light')
 for(const theme of ['light','dark'])await editorPhone(390,theme)
 for(const theme of ['light','dark']){await editorDrag(1280,theme);await editorTouch(390,theme)}
 await editorDragRules(1280,'light')
 console.log('Pathways: the list, its switch, Duplicate and Delete, the empty, loading and failed states, the limit, and the way in from the Factory, and the editor (building by keyboard and buttons, saving, leaving, someone else\'s save, a missing skill, refusals, twelve stages, the phone sheets, and dragging by mouse and touch) pass at 1280 and 390, light and dark.')
}finally{await browser.close()}
