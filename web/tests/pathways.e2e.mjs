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

// The pathways API over an in-memory list. `box` records what the page sent; `failSave`, `failList` and `failCreate` make the next such call fail.
function fixture({list=three(),failList=0}={}){
 const box={list,saves:[],deletes:[],creates:[],failSave:'',failList,failCreate:'',created:0}
 const err=(status,code,error)=>({status,json:{error,code}})
 const routes={
  [LIST]:r=>{
   if(r.method()==='POST'){
    const b=r.postDataJSON();box.creates.push(b)
    if(box.failCreate==='limit')return err(409,'pathway_limit','This account already has 50 pathways. Delete one before saving another.')
    const made={...b,id:`new${++box.created}`,createdAt:Date.now(),updatedAt:Date.now()};box.list=[...box.list,made];return{status:201,json:made}
   }
   if(box.failList>0){box.failList--;return{status:502,json:{error:'502 Bad Gateway'}}}
   return{pathways:box.list}
  },
 }
 // Each pathway's own path, for every id a test can reach (the three, and copies made by Duplicate).
 for(const id of ['p1','p2','p3','new1','new2','new3'])routes[`${LIST}/${id}`]=r=>{
  const m=r.method(),at=box.list.findIndex(p=>p.id===id)
  if(m==='PUT'){
   const b=r.postDataJSON();box.saves.push(b)
   if(box.failSave==='changed'){box.failSave='';return err(409,'pathway_changed','That pathway changed. Reload it before trying again.')}
   if(box.failSave){box.failSave='';return{status:502,json:{error:'502 Bad Gateway'}}}
   const saved={...b,updatedAt:Date.now()};box.list=box.list.map(p=>p.id===id?saved:p);return saved
  }
  if(m==='DELETE'){box.deletes.push(id);box.list=box.list.filter(p=>p.id!==id);return{status:204,json:null}}
  return at<0?err(404,'','That pathway is not available.'):box.list[at]
 }
 return {box,routes}
}
const open=async(width,theme,over={},extra={})=>{
 const f=fixture(over),w=await openWorkspace(browser,{width,height:width<500?844:900,theme,touch:width<1000,routes:f.routes,...extra})
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
  await expect(row(page,'Standard').getByRole('list',{name:'Stages of Standard'}).getByRole('listitem')).toHaveText(['Code','Review','Test','PR','Babysit','Merge'])
  await expect(row(page,'Careful release').getByRole('list',{name:'Stages of Careful release'}).getByRole('listitem')).toHaveText(['Code','Security pass','Review','PR','Merge'])
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

try{
 for(const [width,theme] of [[1280,'light'],[1280,'dark'],[390,'light'],[390,'dark']]){
  await listRows(width,theme)
  await listSwitch(width,theme)
  await listEmpty(width,theme)
 }
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  await listDelete(width,theme)
  await listDuplicate(width,theme)
  await listRoutes(width,theme)
  await fromFactory(width,theme)
  await listStates(width,theme)
 }
 console.log('Pathways: the list, its switch, Duplicate and Delete, the empty, loading and failed states, the limit, and the way in from the Factory pass at 1280 and 390, light and dark.')
}finally{await browser.close()}
