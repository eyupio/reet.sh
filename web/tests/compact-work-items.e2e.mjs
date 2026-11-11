import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-compact-work'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const projects=Array.from({length:60},(_,n)=>({id:`p${n}`,name:`Project ${n}`,dir:`/work/project-${n}`,host:'a1',machine:'Workshop',isRepo:true,position:n,version:1,createdAt:now,updatedAt:now}))
const sessions=Array.from({length:140},(_,n)=>({id:`s${n}`,agentId:'a1',projectId:`p${n%60}`,title:n===79?'Locate the hidden release regression':`Repair session navigation ${n}`,state:n<80?'running':'exited',exitCode:n<80?null:0,createdAt:now-n*1000,updatedAt:now-n*1000,tool:'opencode',model:'fixture-model',dir:`/work/project-${n%60}`,worktreeBranch:`reet/fix-${n}`,promptExcerpt:`Fix the session navigation regression ${n}. Keep the running worker.`,attention:n===0?'waiting':n<80?'working':'finished'}))
const drafts=Array.from({length:60},(_,n)=>({id:`d${n}`,title:`Improve backlog planning ${n}`,prompt:`Make the planning tools clearer ${n}`,agentId:'a1',projectId:`p${n}`,dir:`/work/project-${n}`,tool:'opencode',model:'fixture-model',effort:'',imageCount:0,starting:n===1,version:1,createdAt:now-n*1000,updatedAt:now-n*1000}))
const machine={agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,terminalControl:true,terminalEnabled:true,readiness:'ready',os:'linux',launch:true,tools:['opencode'],agentsV2:true,acp:true}}
try {
 for(const theme of ['light','dark'])for(const width of [1440,390,320]){
  const w=await openWorkspace(browser,{width,height:950,theme,touch:width<1000,terminalStatus:{configured:true,available:true},terminalMachines:[machine],routes:{
   '/api/v2/terminal/sessions':{sessions},'/api/v2/session-backlog':{drafts},'/api/v2/folder-projects':{projects,savedWaiting:0},
   '/api/v2/machines/health':{serverNow:now,machines:[{...machine,state:'connected'}]},
  }})
  const writes=[];w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')writes.push(r.url())})
  await w.page.addInitScript(()=>{if(localStorage.getItem('reet.rail.collapsed')===null)localStorage.setItem('reet.rail.collapsed','0')})
  try {
   const page=w.page;await page.goto(base+'/')
   const board=page.getByRole('region',{name:'Your work, in good order.',exact:true})
   await expect(board.locator('.session-card')).toHaveCount(15)
   await expect(board.getByRole('button',{name:'Compact cards',exact:true})).toHaveAttribute('aria-pressed','true')
   for(const stage of ['backlog','active','finished'])await expect(board.locator(`.stage.${stage} .session-card`)).toHaveCount(5)
   const active=board.locator('.stage.active'),first=active.locator('.session-card').first()
   expect(await first.evaluate(el=>el.getBoundingClientRect().height)).toBeLessThan(110)
   await expect(first.locator('.agent-meta')).toBeHidden()
   const expand=first.getByRole('button',{name:'Show details for Repair session navigation 0',exact:true})
   expect(await expand.evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
   // Touching disclosure must never hit the adjacent Start action or another control.
   for (const stage of ['backlog','active','finished']) {
    const disclosure=board.locator(`.stage.${stage} .session-card`).first().getByRole('button',{name:/^Show details for /})
    await disclosure.evaluate(el=>el.scrollIntoView({block:'center'}))
    expect(await disclosure.evaluate(button=>{const r=button.getBoundingClientRect();return [0.2,0.5,0.8].map(f=>{const hit=document.elementFromPoint(r.left+r.width/2,r.top+r.height*f);return button.contains(hit)?null:{f,hit:hit?.outerHTML.slice(0,240),top:r.top,bottom:r.bottom}}).filter(Boolean)}), `${width} ${stage} disclosure hit targets`).toEqual([])
   }
   await expect(board.locator('.stage.backlog').getByRole('button',{name:'Retry start',exact:true})).toBeVisible()
   const backlog=board.locator('.stage.backlog .session-card').first()
   await expect(backlog.getByRole('button',{name:'Start',exact:true})).toBeHidden()
   await backlog.getByRole('button',{name:/^Show details for /}).click()
   await expect(backlog.getByRole('button',{name:'Start',exact:true})).toBeVisible()
   await expect(backlog.getByRole('button',{name:'Edit Improve backlog planning 0',exact:true})).toBeVisible()
   await backlog.getByRole('button',{name:/^Hide details for /}).click()
   await expand.click();await expect(first.locator('.agent-meta')).toBeVisible();await expect(first.getByText('reet/fix-0',{exact:false})).toBeVisible()
   await first.getByRole('button',{name:'Hide details for Repair session navigation 0',exact:true}).click()
   await expect(first.locator('.agent-meta')).toBeHidden()
   await active.getByRole('button',{name:'Show 5 more',exact:true}).click();await expect(active.locator('.session-card')).toHaveCount(10)
   // Search covers records beyond the visible slice, and resets the window.
   await board.getByRole('searchbox',{name:'Search sessions',exact:true}).fill('hidden release')
   await expect(active.locator('.session-card')).toHaveCount(1);await expect(active.getByRole('link',{name:'Locate the hidden release regression',exact:true})).toBeVisible()
   await board.getByRole('button',{name:'Clear search',exact:true}).click();await expect(active.locator('.session-card')).toHaveCount(5)
   await active.getByRole('button',{name:'View all 80',exact:true}).click();await expect(active.locator('.session-card')).toHaveCount(80);await expect(active.getByRole('button',{name:'Show fewer',exact:true})).toBeFocused()
   await active.getByRole('button',{name:'Show fewer',exact:true}).click();await expect(active.locator('.session-card')).toHaveCount(5);await expect(active.getByRole('button',{name:'Show 5 more',exact:true})).toBeFocused()
   if(width===1440){
    expect(await page.evaluate(()=>document.documentElement.scrollHeight<innerHeight*1.5)).toBe(true)
    // The project list is embedded in the workspace panel and scrolls inside it.
    const rail=page.locator('[data-workspace-rail]')
    await rail.locator('.card').last().evaluate(el=>el.scrollIntoView({block:'center'}))
    expect(await rail.evaluate(el=>el.scrollTop)).toBeGreaterThan(0)
    await expect(rail.locator('.card').last()).toBeInViewport()
    await rail.evaluate(el=>el.scrollTop=0)
   }
   if(width!==320)await page.screenshot({path:`${shots}/compact-board-${width}-${theme}.png`,fullPage:true})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   // Continue is bounded globally, even across sixty different project groups.
   await page.getByRole('button',{name:'Continue working',exact:true}).click()
   const flight=page.getByRole('region',{name:'Continue working',exact:true})
   await expect(flight.locator('.summary')).toHaveCount(5)
   await flight.getByRole('button',{name:/^Show details for /}).first().click();await expect(flight.locator('.item-details:visible')).toHaveCount(1)
   await flight.getByRole('button',{name:'Show 5 more',exact:true}).click();await expect(flight.locator('.summary')).toHaveCount(10)
   await flight.getByRole('radio',{name:'Machine',exact:true}).check();await expect(flight.locator('.summary')).toHaveCount(5)
   await flight.getByRole('button',{name:'View all 140',exact:true}).click();await expect(flight.locator('.summary')).toHaveCount(140)
   await flight.getByRole('button',{name:'Show fewer',exact:true}).click();await expect(flight.locator('.summary')).toHaveCount(5)
   if(width!==320)await page.screenshot({path:`${shots}/compact-continue-${width}-${theme}.png`,fullPage:true})
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   await page.getByRole('button',{name:'Planning board',exact:true}).click()
   await board.getByRole('button',{name:'Compact cards',exact:true}).click();await expect(board.getByRole('button',{name:'Compact cards',exact:true})).toHaveAttribute('aria-pressed','false')
   await page.reload();await expect(page.getByRole('button',{name:'Compact cards',exact:true})).toHaveAttribute('aria-pressed','false')
   expect(writes).toEqual([]);expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  } finally {await w.context.close()}
 }
 const waiting=sessions.slice(0,12).map(s=>({...s,attention:'waiting',changedSinceSeen:true,pendingPermission:{permissionId:`approval-${s.id}`,title:'Read project files',command:'ls',queued:0}}))
 const w=await openWorkspace(browser,{width:390,terminalStatus:{configured:true,available:true},terminalMachines:[machine],routes:{'/api/v2/terminal/sessions':{sessions:waiting},'/api/v2/session-backlog':{drafts:[]}}})
 try {
  await w.page.goto(base+'/')
  const attention=w.page.locator('.attention-detail')
  await expect(w.page.getByRole('button',{name:'Needs you: 12',exact:true})).toBeVisible()
  await expect(attention.locator('#needs-you > header .count')).toHaveText('12')
  await expect(attention.getByRole('button',{name:'Allow once',exact:true})).toHaveCount(5)
  await attention.getByRole('button',{name:'View all 12',exact:true}).click()
  await expect(attention.getByRole('button',{name:'Allow once',exact:true})).toHaveCount(12)
  await attention.getByRole('button',{name:'Show fewer',exact:true}).click()
  await expect(attention.getByRole('button',{name:'Allow once',exact:true})).toHaveCount(5)
  expect(w.errors).toEqual([])
 } finally {await w.context.close()}
 console.log('Compact work: 200 records, six theme/width views, bounded columns and sixty-group Continue, disclosure hit targets, complete search, more/all/fewer, focus, preference persistence, full attention counts and approval access, zero implicit writes passed.')
} finally {await browser.close()}
