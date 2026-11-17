import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace, owner } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const shots = process.env.REET_UI_SCREENSHOTS || '/tmp/reet-home-followups'
await mkdir(shots, {recursive:true})
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const now = Date.now(), id = '1'.repeat(32)
function fixture() {
  const pins = {providerId:'a'.repeat(32),providerRevision:1,model:'fixture-model',contextRevision:1,workerRevision:1}
  const row = (n, state, attention) => ({goal:{...pins,id:String(n).repeat(32),conversationId:String(n).repeat(32),sessionId:`worker-${n}`,revision:1,text:'Review fresh worker progress. Ask before changing scope.',state,maxReviews:12,usedReviews:2,expiresAt:now+3600000,createdAt:now,updatedAt:now},session:{id:`worker-${n}`,agentId:'a1',title:n===1?'Release review':'Build follow-up',state:'running'},available:true,attention})
  const box = {rows:[row(1,'needs_user',true),row(2,'active',false)],reads:0,fail:false,controls:[]}
  const routes = {
    '/api/v2/gaffer/goals':request => { expect(request.method()).toBe('GET'); box.reads++; return box.fail?{status:503,json:{error:'Follow-ups temporarily unavailable'}}:{goals:box.rows,readAt:now} },
    [`/api/v2/gaffer/conversations/${id}/goal`]:request => { expect(request.method()).toBe('GET');return {goal:box.rows[0].goal,pins,allowed:true} },
    [`/api/v2/gaffer/conversations/${id}/goal/control`]:request => {
      const body=request.postDataJSON();expect(body.action).toBe('stop');expect(body.revision).toBe(1)
      box.controls.push(body);box.rows[0].goal={...box.rows[0].goal,state:'stopped',revision:2};box.rows[0].attention=false
      return {goal:box.rows[0].goal,pins,allowed:true}
    },
  }
  return {box,routes}
}
// Home reads Gaffer's goals and nothing else of Gaffer's, and makes no write by reading them. A goal that needs review on a
// session with no card and no pipeline is an entry of Waiting for you (chip Gaffer, Open to the session); the Factory page
// is where goals are reviewed and stopped.
try {
  for (const theme of ['light','dark']) for (const width of [1440,390,320]) {
    const f=fixture(), w=await openWorkspace(browser,{width,height:900,theme,touch:width<1000,routes:f.routes})
    const requests=[]
    w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/'))requests.push({path:new URL(r.url()).pathname,method:r.method()})})
    try {
      await w.page.goto(base+'/')
      // The head says what waits in one line; there is no Gaffer pill and no Factory or Projects button in it. The way to
      // the Factory is the tab bar's.
      const heading=w.page.locator('.workspace-heading'),waiting=w.page.getByRole('region',{name:'Waiting for you',exact:true})
      // The goal that needs review is waiting for the person: one entry, counted in the head, the bell and the title.
      await expect(heading.locator('.counts')).toHaveText('1 waiting for you')
      const goal=waiting.locator('.waiting-list > li')
      await expect(goal).toHaveCount(1)
      await expect(goal.locator('.chip')).toHaveText('Gaffer')
      await expect(goal).toContainText('Release review');await expect(goal).toContainText('Session')
      await expect(goal).toContainText('Gaffer’s goal needs you.')
      await expect(goal.getByRole('link',{name:'Open Release review',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=worker-1')
      expect(await goal.getByRole('link',{name:'Open Release review',exact:true}).evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
      await expect(w.page.getByRole('button',{name:'Needs you: 1',exact:true})).toBeVisible()
      await expect(w.page).toHaveTitle(/^\(1\) /)
      await expect(w.page.getByRole('link',{name:/^Gaffer/})).toHaveCount(0)
      await expect(heading.getByRole('link',{name:'Factory',exact:true})).toHaveCount(0)
      await expect(heading.getByRole('button',{name:'Projects',exact:true})).toHaveCount(0)
      if(width>=1100)await expect(w.page.getByRole('navigation').getByRole('link',{name:'Factory',exact:true}).first()).toHaveAttribute('href','/factory')
      await expect.poll(()=>f.box.reads).toBeGreaterThan(0)
      expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      if(width===1440||width===390)await w.page.screenshot({path:`${shots}/home-followups-${width}-${theme}.png`,fullPage:true})
      // A failed refresh of the goals keeps the last read: the goal is still waiting.
      f.box.fail=true
      const before=f.box.reads
      await w.page.evaluate(()=>window.dispatchEvent(new Event('reet:gaffer-goals-changed')))
      await expect.poll(()=>f.box.reads).toBeGreaterThan(before)
      await expect(heading.locator('.counts')).toHaveText('1 waiting for you')
      await expect(goal).toHaveCount(1)
      // It says the read failed, rather than nothing.
      await expect(w.page.locator('.problem')).toHaveText('The Factory could not be read; showing what was last read.')
      f.box.fail=false
      // Once nothing is followed, nothing waits.
      f.box.rows=[]
      await w.page.evaluate(()=>window.dispatchEvent(new Event('reet:gaffer-goals-changed')))
      await expect(heading.locator('.counts')).toHaveText('0 waiting for you')
      await expect(waiting).toContainText('Nothing is waiting for you.')
      await expect(w.page.locator('.problem')).toHaveCount(0)
      // Home read the goals and made no write, and never touched Gaffer's providers or conversations.
      expect(requests.filter(r=>r.method!=='GET')).toEqual([])
      expect(requests.some(r=>r.path==='/api/v2/gaffer/providers'||r.path==='/api/v2/gaffer/conversations')).toBe(false)
      expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  for (const status of [{user:{...owner,role:'viewer'}},{mode:'none',user:null}]) {
    const f=fixture(),w=await openWorkspace(browser,{width:390,status,routes:f.routes})
    try {await w.page.goto(base+'/');await expect(w.page.getByRole('heading',{name:'Home',exact:true})).toBeVisible();await expect(w.page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0);expect(f.box.reads).toBe(0)}
    finally {await w.context.close()}
  }
  // Goals that cannot be read leave Home whole, and Home says so: it never claims nothing waits, and nothing throws.
  const failed=fixture();failed.box.fail=true
  const failedWorkspace=await openWorkspace(browser,{width:390,routes:failed.routes})
  try {
    await failedWorkspace.page.goto(base+'/')
    await expect(failedWorkspace.page.locator('.problem')).toHaveText('The Factory could not be read. Home tries again every 15 seconds.')
    await expect(failedWorkspace.page.getByRole('region',{name:'Waiting for you',exact:true})).toContainText('Nothing that could be read is waiting for you.')
    await expect(failedWorkspace.page.getByRole('region',{name:'Waiting for you',exact:true})).not.toContainText('Nothing is waiting for you.')
    expect(failedWorkspace.errors).toEqual([])
  } finally {await failedWorkspace.context.close()}
  // An account without Gaffer (its reads refused or missing) is not told anything failed.
  for (const status of [403,404]) {
    const quiet=fixture()
    quiet.routes['/api/v2/gaffer/goals']=()=>({status,json:{error:'no'}});quiet.routes['/api/v2/gaffer/outcomes']=()=>({status,json:{error:'no'}})
    const q=await openWorkspace(browser,{width:390,routes:quiet.routes})
    try {
      await q.page.goto(base+'/')
      await expect(q.page.getByRole('region',{name:'Waiting for you',exact:true})).toContainText('Nothing is waiting for you.')
      await expect(q.page.locator('.problem')).toHaveCount(0)
      expect(q.errors).toEqual([])
    } finally {await q.context.close()}
  }
  // The Factory page, reached from the tab bar, is where a goal is reviewed and stopped explicitly.
  for (const width of [1440,390]) {
    const f=fixture(),w=await openWorkspace(browser,{width,height:900,touch:width<1000,routes:f.routes})
    const writes=[]
    w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')writes.push(new URL(r.url()).pathname)})
    try {
      const page=w.page
      await page.goto(base+'/factory')
      await expect(page.getByRole('heading',{name:'Factory',exact:true,level:1})).toBeVisible()
      // Gaffer's other sessions are the page's Other sessions section. A goal in it needs the person, so it is open on
      // arrival, and its summary says so first.
      const others=page.getByRole('button',{name:/^Other sessions/})
      await expect(others).toHaveAttribute('aria-expanded','true');await expect(others).toContainText('1 needs you · 1 followed')
      const floor=page.locator('[data-floor]')
      await expect(floor.getByRole('list',{name:'Sessions Gaffer looks after',exact:true}).getByRole('listitem')).toHaveCount(2)
      await expect(floor.getByRole('link',{name:'Open session',exact:true}).first()).toHaveAttribute('href','/sessions?agent=a1&session=worker-1')
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      expect(writes).toEqual([])
      await floor.getByRole('button',{name:'Review goal',exact:true}).first().click()
      const goal=page.getByRole('dialog',{name:'Look after this session',exact:true})
      await goal.getByRole('button',{name:'Stop goal',exact:true}).click()
      await page.getByRole('dialog',{name:'Stop this goal?',exact:true}).getByRole('button',{name:'Stop goal',exact:true}).click()
      await expect(goal.getByRole('heading',{name:'Stopped',exact:true})).toBeVisible()
      // Nothing in Other sessions needs the person now. The page reads that on its next refresh, and the section it
      // opened by itself stays open for the visit: it does not close under the dialog the goal was stopped in.
      await expect(others).not.toContainText('needs you',{timeout:20000})
      await expect(others).toHaveAttribute('aria-expanded','true')
      await expect(goal.getByRole('heading',{name:'Stopped',exact:true})).toBeVisible()
      expect(f.box.controls).toHaveLength(1);expect(writes).toEqual([`/api/v2/gaffer/conversations/${id}/goal/control`])
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // The session holding the approval is one stage of a Factory pipeline that is working by itself (its goal does not say it
  // waits on the agent). The approval is only on the session's card, so the card stays in Waiting for you, with Allow and
  // Deny and the followed goal as its second reason, and the bell lists it; the pipeline is in Factory, working without you.
  const folded=fixture()
  folded.routes['/api/v2/terminal/sessions']={sessions:[{id:'worker-1',agentId:'a1',title:'Release review',state:'running',createdAt:now-7200000,updatedAt:now-720000,exitCode:null,structured:true,attention:'waiting',attentionSource:'chat',bellAt:now-60000,changedSinceSeen:true,pendingPermission:{permissionId:'perm-aaaa00000001',title:'go test',command:'go test ./...',queued:0}}]}
  // The session is one stage of a Factory pipeline, and Home says so wherever it lists the session.
  folded.routes['/api/v2/gaffer/outcomes']={outcomes:[{id:'o'.repeat(32),revision:1,objective:'Ship the release.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'reet',dir:'/work/reet',tool:'claude',providerId:'p'.repeat(32),maxReviews:12,hours:24,createdAt:now,updatedAt:now,sessions:[{key:'a',title:'Release review',prompt:'p',state:'following',note:'The plan stage is under way.',sessionId:'worker-1',conversationId:'c'.repeat(32),updatedAt:now,stage:0,stages:[{kind:'plan',tool:'claude',state:'running',sessionId:'worker-1'},{kind:'code',tool:'claude',state:'pending'}]}]}]}
  const foldedWorkspace=await openWorkspace(browser,{width:390,terminalStatus:{configured:true,available:true,reason:''},routes:folded.routes})
  try {
    const page=foldedWorkspace.page
    await page.goto(base+'/')
    const waiting=page.getByRole('region',{name:'Waiting for you',exact:true}),item=waiting.locator('.waiting-list > li')
    await expect(item).toHaveCount(1)
    await expect(item).toContainText('Release review');await expect(item).toContainText('Session')
    await expect(item.locator('.chip')).toHaveText('Approval')
    await expect(item.getByRole('button',{name:'Allow once'})).toBeVisible();await expect(item.getByRole('button',{name:'Deny'})).toBeVisible()
    await expect(item.getByText('Gaffer’s goal also needs you',{exact:true})).toHaveCount(1)
    await expect(page.getByText('Factory · Plan 1 of 2',{exact:true}).first()).toBeVisible()
    await expect(waiting.getByRole('button',{name:/^Waiting for you /})).toContainText('· 1')
    await expect(page.getByRole('button',{name:/^Factory, working without you /})).toContainText('Release review')
    await expect(page.getByRole('button',{name:'Needs you: 1',exact:true})).toBeVisible()
    await expect(page).toHaveTitle(/^\(1\) /)
    await page.getByRole('button',{name:'Needs you: 1',exact:true}).click()
    const pop=page.getByRole('banner').getByRole('region',{name:'Needs you'})
    await expect(pop.locator('.item')).toHaveCount(1)
    await expect(pop.getByRole('button',{name:'Allow once'})).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('link',{name:/^Gaffer/})).toHaveCount(0)
    expect(foldedWorkspace.errors).toEqual([]);expect(foldedWorkspace.state.unexpected).toEqual([])
  } finally {await foldedWorkspace.context.close()}
  // The only goal that needs review is on a session with an approval card and no pipeline: it is folded into that card,
  // which says so once, and is counted once.
  const only=fixture();only.box.rows=only.box.rows.slice(0,1)
  only.routes['/api/v2/terminal/sessions']=folded.routes['/api/v2/terminal/sessions']
  const onlyWorkspace=await openWorkspace(browser,{width:390,terminalStatus:{configured:true,available:true,reason:''},routes:only.routes})
  try {
    const page=onlyWorkspace.page
    await page.goto(base+'/')
    const waiting=page.getByRole('region',{name:'Waiting for you',exact:true}),item=waiting.locator('.waiting-list > li')
    await expect(item).toHaveCount(1)
    await expect(item.filter({hasText:'Release review'}).getByText('Gaffer’s goal also needs you',{exact:true})).toHaveCount(1)
    await expect(waiting.getByRole('button',{name:/^Waiting for you /})).toContainText('· 1')
    await expect(page.getByRole('button',{name:'Needs you: 1',exact:true})).toBeVisible()
    await expect(page).toHaveTitle(/^\(1\) /)
    await expect(page.getByRole('link',{name:/^Gaffer/})).toHaveCount(0)
    expect(onlyWorkspace.errors).toEqual([]);expect(onlyWorkspace.state.unexpected).toEqual([])
  } finally {await onlyWorkspace.context.close()}
  console.log('Home follow-ups: six theme/width views with the counts line and no Gaffer pill, a failed goals read leaving Home whole, a Gaffer goal waiting as its own entry (kept through a failed read), an approval on a working pipeline’s session kept in the list and the bell, a goal folded into its session card, no implicit writes or provider reads on Home, viewer/open-mode boundaries, and explicit review and stop on the Factory page passed.')
} finally {await browser.close()}
