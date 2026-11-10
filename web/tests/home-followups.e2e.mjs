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
// Home's Gaffer entry is the way onto the Factory page: a link that says what is waiting there. Home itself reads the
// goals and nothing else of Gaffer's; the page it leads to is where goals are reviewed and stopped.
try {
  for (const theme of ['light','dark']) for (const width of [1440,390,320]) {
    const f=fixture(), w=await openWorkspace(browser,{width,height:900,theme,touch:width<1000,routes:f.routes})
    const requests=[]
    w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/'))requests.push({path:new URL(r.url()).pathname,method:r.method()})})
    try {
      await w.page.goto(base+'/')
      const entry=w.page.getByRole('link',{name:'Gaffer: 1 follow-up needs review',exact:true})
      await expect(entry).toBeVisible();await expect(entry).toHaveAttribute('href','/factory')
      expect(await entry.evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
      expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      if(width===1440||width===390)await w.page.screenshot({path:`${shots}/home-followups-${width}-${theme}.png`,fullPage:true})
      // Refresh failures retain the last known count and explicitly label it as old.
      f.box.fail=true
      await w.page.evaluate(()=>window.dispatchEvent(new Event('reet:gaffer-goals-changed')))
      await expect(w.page.getByRole('link',{name:'Gaffer: 1 follow-up needs review · last read',exact:true})).toBeVisible()
      f.box.fail=false
      // With nothing followed the line goes; Home's own Factory button is always there.
      f.box.rows=[]
      await w.page.evaluate(()=>window.dispatchEvent(new Event('reet:gaffer-goals-changed')))
      await expect(w.page.locator('.workspace-summary .followups')).toHaveCount(0)
      await expect(w.page.locator('a.home-factory')).toHaveAttribute('href','/factory')
      if(width>=1100)await expect(w.page.locator('a.home-factory')).toBeVisible()
      // Home read the goals and made no write, and never touched Gaffer's providers or conversations.
      expect(requests.filter(r=>r.method!=='GET')).toEqual([])
      expect(requests.some(r=>r.path==='/api/v2/gaffer/providers'||r.path==='/api/v2/gaffer/conversations')).toBe(false)
      expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  for (const status of [{user:{...owner,role:'viewer'}},{mode:'none',user:null}]) {
    const f=fixture(),w=await openWorkspace(browser,{width:390,status,routes:f.routes})
    try {await w.page.goto(base+'/');await expect(w.page.getByRole('heading',{name:'Home',exact:true})).toBeVisible();await expect(w.page.locator('.workspace-summary .followups')).toHaveCount(0);expect(f.box.reads).toBe(0)}
    finally {await w.context.close()}
  }
  const failed=fixture();failed.box.fail=true
  const failedWorkspace=await openWorkspace(browser,{width:390,routes:failed.routes})
  try {
    await failedWorkspace.page.goto(base+'/')
    await expect(failedWorkspace.page.getByRole('link',{name:'Gaffer follow-ups unavailable',exact:true})).toHaveAttribute('href','/factory')
  } finally {await failedWorkspace.context.close()}
  // The entry leads to the Factory page, where a goal is reviewed and stopped explicitly.
  for (const width of [1440,390]) {
    const f=fixture(),w=await openWorkspace(browser,{width,height:900,touch:width<1000,routes:f.routes})
    const writes=[]
    w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET')writes.push(new URL(r.url()).pathname)})
    try {
      const page=w.page
      await page.goto(base+'/');await page.getByRole('link',{name:'Gaffer: 1 follow-up needs review',exact:true}).click()
      await expect(page).toHaveURL(/\/factory$/)
      await expect(page.getByRole('heading',{name:'Factory',exact:true,level:1})).toBeVisible()
      const floor=page.locator('[data-floor]')
      await expect(floor.getByRole('list',{name:'Followed session goals',exact:true}).getByRole('listitem')).toHaveCount(2)
      await expect(floor.getByRole('link',{name:'Open session',exact:true}).first()).toHaveAttribute('href','/sessions?agent=a1&session=worker-1&gaffer=1')
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
      expect(writes).toEqual([])
      await floor.getByRole('button',{name:'Review goal',exact:true}).first().click()
      const goal=page.getByRole('dialog',{name:'Look after this session',exact:true})
      await goal.getByRole('button',{name:'Stop goal',exact:true}).click()
      await page.getByRole('dialog',{name:'Stop this goal?',exact:true}).getByRole('button',{name:'Stop goal',exact:true}).click()
      await expect(goal.getByRole('heading',{name:'Stopped',exact:true})).toBeVisible()
      expect(f.box.controls).toHaveLength(1);expect(writes).toEqual([`/api/v2/gaffer/conversations/${id}/goal/control`])
      expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  // A goal that needs review on a session which already has an approval card is that card's second reason: the card says so
  // once, the Gaffer entry no longer counts it, and the card count (header, bell, title) stays the number of cards.
  const folded=fixture()
  folded.routes['/api/v2/terminal/sessions']={sessions:[{id:'worker-1',agentId:'a1',title:'Release review',state:'running',createdAt:now-7200000,updatedAt:now-720000,exitCode:null,structured:true,attention:'waiting',attentionSource:'chat',bellAt:now-60000,changedSinceSeen:true,pendingPermission:{permissionId:'perm-aaaa00000001',title:'go test',command:'go test ./...',queued:0}}]}
  const foldedWorkspace=await openWorkspace(browser,{width:390,terminalStatus:{configured:true,available:true,reason:''},routes:folded.routes})
  try {
    const page=foldedWorkspace.page
    await page.goto(base+'/')
    const needs=page.locator('#needs-you')
    await expect(needs.locator('.card')).toHaveCount(1)
    const card=needs.locator('.card',{hasText:'Release review'})
    await expect(card.getByText('Gaffer follow-up also needs review',{exact:true})).toHaveCount(1)
    await expect(needs.locator('.count')).toHaveText('1')
    await expect(page.getByRole('button',{name:'Needs you: 1',exact:true})).toBeVisible()
    await expect(page).toHaveTitle(/^\(1\) /)
    await expect(page.getByRole('link',{name:'Gaffer: following 1 session',exact:true})).toBeVisible()
    await expect(page.getByRole('link',{name:/follow-ups? needs? review/})).toHaveCount(0)
    expect(foldedWorkspace.errors).toEqual([]);expect(foldedWorkspace.state.unexpected).toEqual([])
  } finally {await foldedWorkspace.context.close()}
  // The only goal that needs review is on a session with an approval card, with nothing else followed: it is folded into
  // that card, and Home's entry says where the goal went.
  const only=fixture();only.box.rows=only.box.rows.slice(0,1)
  only.routes['/api/v2/terminal/sessions']=folded.routes['/api/v2/terminal/sessions']
  const onlyWorkspace=await openWorkspace(browser,{width:390,terminalStatus:{configured:true,available:true,reason:''},routes:only.routes})
  try {
    const page=onlyWorkspace.page
    await page.goto(base+'/')
    const needs=page.locator('#needs-you')
    await expect(needs.locator('.card')).toHaveCount(1)
    await expect(needs.locator('.card',{hasText:'Release review'}).getByText('Gaffer follow-up also needs review',{exact:true})).toHaveCount(1)
    await expect(needs.locator('.count')).toHaveText('1')
    await expect(page.getByRole('button',{name:'Needs you: 1',exact:true})).toBeVisible()
    await expect(page).toHaveTitle(/^\(1\) /)
    await expect(page.getByRole('link',{name:'Gaffer: 1 follow-up is in Needs you',exact:true})).toHaveAttribute('href','/factory')
    await expect(page.getByRole('link',{name:/follow-ups? needs? review/})).toHaveCount(0)
    expect(onlyWorkspace.errors).toEqual([]);expect(onlyWorkspace.state.unexpected).toEqual([])
  } finally {await onlyWorkspace.context.close()}
  console.log('Home follow-ups: six theme/width views, the entry as a link to the Factory with its wording, a goal folded into its session card (counted once, entry kept), no implicit writes or provider reads on Home, stale read, Home’s own Factory button, viewer/open-mode boundaries, and explicit review and stop on the Factory page passed.')
} finally {await browser.close()}
