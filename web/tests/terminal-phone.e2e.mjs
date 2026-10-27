// Phone terminal layout: compact chrome, the keyboard focus layout, the font default,
// and the size sent to the host (floor, debounce, repaint on return). Desktop and
// tablet keep their layout. The on-screen keyboard is simulated: the visual viewport
// is replaced so its height can shrink while the layout viewport stays (Android's
// default), or the layout viewport itself shrinks (interactive-widget=resizes-content).
import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),agentA='connector-laptop000000001',sessionA='session-build00000000001'
const sessions=[{id:sessionA,agentId:agentA,title:'Agent session',state:'running',createdAt:now,updatedAt:now,exitCode:null}]
const machines=[{agentId:agentA,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,readiness:'ready'}}]
// A full-screen app that repaints every row for the size it was given, with its input on the last row.
const app=(cols,rows)=>{let out='\x1b[?1049h\x1b[2J'
 for(let row=1;row<=rows;row++)out+=`\x1b[${row};1H`+(row===1?`agent ${cols}x${rows}`:row===rows?'> ask anything':`row ${row}`).slice(0,cols)
 return Buffer.from(out)}

async function open({width,height,touch,query='',layout=null,motion='no-preference',screen=null}){
 const context=await browser.newContext({viewport:{width,height},hasTouch:touch,isMobile:touch,reducedMotion:motion,...(screen?{screen}:{})})
 const page=await context.newPage(),errors=[],tickets=[],frames=[]
 // A saved sessions-panel layout, as a wider screen would have left it.
 if(layout)await page.addInitScript(value=>{if(!sessionStorage.getItem('seeded')){sessionStorage.setItem('seeded','1');localStorage.setItem('reet-terminal-layout:1',value)}},JSON.stringify(layout))
 let sockets=0
 page.on('pageerror',error=>errors.push(error.message))
 await page.addInitScript(()=>{
  // A visual viewport whose height a test can shrink, as an on-screen keyboard does.
  const events=new EventTarget(),real=window.visualViewport
  let height=null,hidden=false
  const visual={get width(){return innerWidth},get height(){return height??innerHeight},offsetTop:0,offsetLeft:0,pageTop:0,pageLeft:0,scale:1,
   addEventListener:(...args)=>{events.addEventListener(...args);real?.addEventListener(...args)},removeEventListener:(...args)=>{events.removeEventListener(...args);real?.removeEventListener(...args)}}
  Object.defineProperty(window,'visualViewport',{configurable:true,get:()=>visual})
  window.__keyboard=value=>{height=value;events.dispatchEvent(new Event('resize'))}
  Object.defineProperty(document,'visibilityState',{configurable:true,get:()=>hidden?'hidden':'visible'})
  Object.defineProperty(document,'hidden',{configurable:true,get:()=>hidden})
  window.__visibility=value=>{hidden=value;document.dispatchEvent(new Event('visibilitychange'))}
  // Lets a test pretend time passed, for the repaint-on-return gate and cooldown.
  const realNow=performance.now.bind(performance);let skew=0
  performance.now=()=>realNow()+skew;window.__advance=ms=>{skew+=ms}
 })
 await page.route('**/api/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  let data={}
  if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Phone tester',email:'phone@example.test',role:'owner'}}
  else if(path==='/api/v2/profile')data={profile:{theme:'light',shelfView:'list',cardsExpanded:false}}
  else if(path==='/api/v2/notifications/count')data={unread:0}
  else if(path==='/api/version')data={version:'fixture',commit:'fixture'}
  else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[],history:[]}
  else if(path==='/api/v2/terminal/machines')data={machines}
  else if(path==='/api/v2/terminal/status')data={available:true,configured:true,reason:''}
  else if(path==='/api/v2/terminal/sessions'){if(request.method()!=='GET')return route.fulfill({status:400,json:{error:'No implicit creation permitted'}});data={sessions}}
  else if(path.endsWith('/attachments')){tickets.push(request.postDataJSON());data={ticket:'fixture-ticket00000000000'}}
  else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}
  else if(path==='/api/environments')data={environments:[],machines:[]}
  else if(path==='/api/tags')data={tags:[]}
  else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
  else if(path==='/api/v2/sync')data={environments:[],machines:[],targets:[],agents:[],jobs:[]}
  else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
  await route.fulfill({json:data})
 })
 await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{
  sockets++
  socket.onMessage(message=>{
   if(typeof message!=='string')return
   const data=JSON.parse(message)
   if(data.ticket){const ticket=tickets.at(-1);socket.send('{"type":"ready"}');socket.send(app(ticket.cols,ticket.rows))}
   else if(data.type==='resize'){frames.push({cols:data.cols,rows:data.rows});socket.send(app(data.cols,data.rows))}
  })
 })
 await page.goto(`${base}/terminals?agent=${agentA}&session=${sessionA}${query}`)
 const pane=page.locator('.pane')
 await expect(pane).toHaveAttribute('data-connection-state','live')
 return {context,page,errors,tickets,frames,pane,sockets:()=>sockets}
}
const box=async(page,selector)=>page.evaluate(selector=>{const element=document.querySelector(selector);if(!element||!element.getClientRects().length)return null;const r=element.getBoundingClientRect();return {top:r.top,bottom:r.bottom,height:r.height}},selector)
// Rows the host was last told, once the debounce has settled.
async function settledSize(page,frames,tickets){await page.waitForTimeout(450);return frames.at(-1)||tickets.at(-1)}
const settle=page=>page.waitForTimeout(450)
// Whole rows the screen shows: its height over the rendered row height.
const visibleRows=page=>page.evaluate(()=>{const screen=document.querySelector('.screen'),rows=document.querySelectorAll('.xterm-rows>div').length,grid=document.querySelector('.xterm-screen').getBoundingClientRect().height;return Math.floor(screen.clientHeight/(grid/rows)+0.01)})
const viewportMeta=page=>page.locator('meta[name="viewport"]').getAttribute('content')

try{
 // ── Phone: 390×844, touch ────────────────────────────────────────────────
 const phone=await open({width:390,height:844,touch:true})
 const {page,frames,tickets,pane}=phone
 // Font default: 11 px with nothing stored, so the agent gets about 54 columns, not 41.
 expect(await page.evaluate(()=>localStorage.getItem('reet-terminal-font'))).toBeNull()
 await expect(page.locator('.xterm-rows')).toHaveCSS('font-size','11px')
 expect(tickets[0].cols).toBeGreaterThanOrEqual(50)
 // The first attach is not nudged: tmux repaints its grid on attach.
 await settle(page);expect(frames.filter(frame=>frame.rows<tickets[0].rows)).toEqual([])
 // Compact one-line chrome; the toolbar and extra keys are 40 × 40 px or larger, the header controls 40 px tall.
 for(const selector of ['.pane>header','.terminal-toolbar','.mobile-keys'])expect((await box(page,selector)).height).toBeLessThanOrEqual(42)
 for(const control of await page.locator('.terminal-toolbar button,.mobile-keys button').all()){const size=await control.boundingBox();expect(size.height).toBeGreaterThanOrEqual(40);expect(size.width).toBeGreaterThanOrEqual(40)}
 for(const control of await page.locator('.pane>header .controls>button,.session-menu>summary').all())expect((await control.boundingBox()).height).toBeGreaterThanOrEqual(40)
 // Only the terminal workspace asks the browser to resize the page for the keyboard.
 expect(await viewportMeta(page)).toBe('width=device-width, initial-scale=1.0, interactive-widget=resizes-content')
 await expect(page.locator('.pane>header')).toContainText('developer@dev-laptop')
 // Clipped rows say so with a fade, and scroll by touch or keyboard.
 const toolbar=page.locator('.terminal-toolbar'),keys=page.locator('.mobile-keys')
 for(const row of [toolbar,keys]){
  expect(await row.evaluate(e=>e.scrollWidth>e.clientWidth)).toBe(true)
  await expect(row).toHaveClass(/more-end/);await expect(row).not.toHaveClass(/more-start/)
  await expect(row).toHaveCSS('overflow-x','auto')
  expect(await row.evaluate(e=>getComputedStyle(e).maskImage)).toContain('linear-gradient')
 }
 await keys.focus();await page.keyboard.press('ArrowRight')
 await expect.poll(()=>keys.evaluate(e=>e.scrollLeft)).toBeGreaterThan(0)
 await expect(keys).toHaveClass(/more-start/)
 await keys.evaluate(e=>e.scrollTo({left:e.scrollWidth}))
 await expect(keys).not.toHaveClass(/more-end/)
 await toolbar.evaluate(e=>e.scrollTo({left:e.scrollWidth}))
 await expect(toolbar).toHaveClass(/more-start/);await expect(toolbar).not.toHaveClass(/more-end/)
 await toolbar.evaluate(e=>e.scrollTo({left:0}))
 // The Alt+Shift+M advice is for hardware keyboards; it is not shown on touch screens.
 await page.getByRole('button',{name:'Maximise',exact:true}).click()
 await expect(pane).toHaveClass(/maximised/)
 await expect(page.locator('.maximise-hint')).toHaveCount(1)
 await expect(page.locator('.maximise-hint')).toBeHidden()
 await expect(page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Restore size',exact:true}).click()
 await settle(page)

 // A shrinking viewport without focus in the terminal (another field, split screen) keeps the layout.
 const keyboardTop=Math.round(844*.55)
 await page.evaluate(()=>document.activeElement instanceof HTMLElement&&document.activeElement.blur())
 await page.evaluate(height=>window.__keyboard(height),keyboardTop)
 await settle(page)
 await expect(page.locator('.site-header')).toBeVisible()
 await expect(page.locator('.workspace-toolbar')).toBeVisible()
 await page.evaluate(()=>window.__keyboard(null))
 await settle(page)
 // Keyboard open while typing in the terminal: header and workspace toolbar step aside.
 await page.getByRole('button',{name:'Focus terminal',exact:true}).click()
 await expect(page.locator('.xterm-helper-textarea')).toBeFocused()
 const beforeKeyboard=frames.length
 // A keyboard animates in: many viewport heights in quick succession, then one size is sent.
 for(let step=1;step<=10;step++){await page.evaluate(height=>window.__keyboard(height),Math.round(844-(844-keyboardTop)*step/10));await page.waitForTimeout(16)}
 await expect(page.locator('.site-header')).toBeHidden()
 await expect(page.locator('.workspace-toolbar')).toBeHidden()
 await expect(page.locator('.pane>footer')).toBeHidden()
 await expect(page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true})).toBeVisible()
 const typing=await settledSize(page,frames,tickets)
 console.log(`Keyboard open: ${frames.length-beforeKeyboard} resize frame(s), ${typing.cols}x${typing.rows}`)
 expect(frames.length-beforeKeyboard).toBe(1)
 expect(typing.rows).toBeGreaterThanOrEqual(15)
 let paneBox=await box(page,'.pane')
 expect(paneBox.bottom).toBeLessThanOrEqual(keyboardTop+1)
 expect(paneBox.bottom).toBeGreaterThanOrEqual(keyboardTop-10)
 // The same viewport again sends nothing.
 for(let i=0;i<3;i++)await page.evaluate(height=>window.__keyboard(height),keyboardTop)
 await settle(page);expect(frames.length-beforeKeyboard).toBe(1)
 // Keyboard closed: the layout comes back.
 await page.evaluate(()=>window.__keyboard(null))
 await expect(page.locator('.site-header')).toBeVisible()
 await expect(page.locator('.workspace-toolbar')).toBeVisible()
 await expect(page.locator('.pane>footer')).toBeVisible()

 // Maximised with the keyboard open: at least 15 rows and no band above the keyboard.
 await page.getByRole('button',{name:'Maximise',exact:true}).click()
 await page.getByRole('button',{name:'Focus terminal',exact:true}).click()
 await page.evaluate(height=>window.__keyboard(height),keyboardTop)
 const maximised=await settledSize(page,frames,tickets)
 console.log(`Maximised, keyboard open: ${maximised.cols}x${maximised.rows}`)
 expect(maximised.rows).toBeGreaterThanOrEqual(15)
 paneBox=await box(page,'.pane')
 expect(Math.abs(paneBox.bottom-keyboardTop)).toBeLessThanOrEqual(2)
 const screenBox=await box(page,'.screen'),rendered=await box(page,'.xterm-screen')
 expect(rendered.bottom).toBeLessThanOrEqual(screenBox.bottom+1)
 await expect(page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true})).toBeVisible()
 // The page resized for the keyboard (resizes-content) but the visual viewport under-reports
 // by a hidden bottom toolbar's height: the page height wins, so no blank band.
 await page.evaluate(()=>window.__keyboard(null))
 await page.setViewportSize({width:390,height:keyboardTop})
 await page.evaluate(height=>window.__keyboard(height),keyboardTop-91)
 await settle(page)
 paneBox=await box(page,'.pane')
 console.log(`Under-reported visual viewport: pane bottom ${Math.round(paneBox.bottom)} of ${keyboardTop}`)
 expect(Math.abs(paneBox.bottom-keyboardTop)).toBeLessThanOrEqual(2)
 await page.evaluate(()=>window.__keyboard(null))
 await page.setViewportSize({width:390,height:844})
 await page.getByRole('button',{name:'Restore size',exact:true}).click()
 await expect(page.locator('.site-header')).toBeVisible()

 // Floor: a very short screen never tells the host fewer than 8 rows, and the last row stays visible.
 await page.evaluate(()=>document.activeElement instanceof HTMLElement&&document.activeElement.blur())
 const beforeFloor=frames.length
 await page.setViewportSize({width:390,height:330})
 const floored=await settledSize(page,frames,tickets)
 console.log(`Short screen: ${floored.cols}x${floored.rows}`)
 expect(frames.length).toBeGreaterThan(beforeFloor)
 expect(floored.rows).toBe(8)
 expect(frames.every(frame=>frame.rows>=8&&frame.cols>=20)).toBe(true)
 await expect(page.locator('.xterm-accessibility-tree [role=listitem]').last()).toContainText('> ask anything')
 await expect.poll(async()=>{const screen=await box(page,'.screen'),grid=await box(page,'.xterm-screen');return Math.round(grid.bottom-screen.bottom)}).toBeLessThanOrEqual(1)
 // A repaint nudge at the floor goes one row taller, never below 8.
 const beforeFloorReturn=frames.length
 await page.evaluate(()=>window.__visibility(true));await page.evaluate(()=>window.__visibility(false))
 await settle(page)
 expect(frames.slice(beforeFloorReturn)).toEqual([{cols:floored.cols,rows:9},{cols:floored.cols,rows:8}])
 await page.setViewportSize({width:390,height:844})
 const settled=await settledSize(page,frames,tickets)
 await page.evaluate(()=>window.__advance(6000))

 // Returning to the page repaints once: one row shorter, then the real size, on the same socket.
 const beforeReturn=frames.length,socketsBefore=phone.sockets()
 await page.evaluate(()=>window.__visibility(true))
 await page.evaluate(()=>window.__visibility(false))
 await settle(page)
 expect(frames.slice(beforeReturn)).toEqual([{cols:settled.cols,rows:settled.rows-1},{cols:settled.cols,rows:settled.rows}])
 expect(phone.sockets()).toBe(socketsBefore)
 // Not again within 5 s.
 const beforeCooldown=frames.length
 await page.evaluate(()=>window.__visibility(true));await page.evaluate(()=>window.__visibility(false))
 await settle(page);expect(frames.length).toBe(beforeCooldown)
 // A reattach repaints once too.
 await page.getByRole('button',{name:'Disconnect',exact:true}).click()
 await page.getByRole('button',{name:'Reconnect',exact:true}).click()
 await expect(pane).toHaveAttribute('data-connection-state','live')
 const beforeReattach=tickets.length
 await settle(page)
 expect(tickets.length).toBe(beforeReattach)
 const reattached=tickets.at(-1)
 // The ticket carries the size at Reconnect (the disconnected notice took a few rows); the nudge ends on the fitted size.
 expect(frames.slice(-2)).toEqual([{cols:reattached.cols,rows:reattached.rows-1},{cols:settled.cols,rows:settled.rows}])
 // A stored font size still wins on a phone.
 await page.evaluate(()=>localStorage.setItem('reet-terminal-font','16'))
 await page.reload();await expect(pane).toHaveAttribute('data-connection-state','live')
 await expect(page.locator('.xterm-rows')).toHaveCSS('font-size','16px')
 expect(phone.errors).toEqual([])
 // Other pages keep the default viewport meta, so their fixed bars and dialogs are unaffected.
 for(const path of ['/','/hosting','/machines']){await page.goto(`${base}${path}`);expect(await viewportMeta(page)).toBe('width=device-width, initial-scale=1.0')}
 await phone.context.close()

 // ── The opt-in viewport readout for checking a real phone ───────────────
 const debug=await open({width:390,height:844,touch:true,query:'&viewport-debug=1'})
 const readout=debug.page.locator('.viewport-debug')
 await expect(readout).toContainText('innerHeight 844')
 await expect(readout).toContainText('keyboard closed')
 await expect(readout).toHaveCSS('pointer-events','none')
 await debug.page.getByRole('button',{name:'Focus terminal',exact:true}).click()
 await debug.page.evaluate(height=>window.__keyboard(height),keyboardTop)
 await expect(readout).toContainText(`visualViewport.height ${keyboardTop}`)
 await expect(readout).toContainText(`used height ${keyboardTop}`)
 await expect(readout).toContainText('keyboard open')
 await expect(debug.page.locator('.xterm-helper-textarea')).toBeFocused()
 expect(await debug.page.evaluate(()=>JSON.stringify({...localStorage}))).not.toContain('viewport')
 expect(debug.errors).toEqual([])
 await debug.context.close()
 const plain=await open({width:390,height:844,touch:true})
 await expect(plain.page.locator('.viewport-debug')).toHaveCount(0)
 await plain.context.close()

 // ── Phones held sideways: tools and keys share a row, the screen keeps the rows ──
 for(const [width,height,motion] of [[844,390,'reduce'],[915,412,'no-preference']]){
  const side=await open({width,height,touch:true,motion,layout:{open:true,mode:'mini',width:320,pinned:true}})
  const p=side.page
  // A phone held sideways keeps the drawer, even with a pinned mini layout saved: no rail, no separator, closed on arrival.
  const drawer=p.locator('aside#terminal-sessions.session-sidebar')
  await expect(p.getByRole('complementary',{name:'Session rail'})).toHaveCount(0);await expect(drawer).toHaveCount(0)
  await p.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true}).click()
  await expect(drawer).toBeVisible();expect(await drawer.evaluate(e=>getComputedStyle(e).position)).toBe('absolute')
  await expect(p.getByRole('separator')).toHaveCount(0);await expect(drawer.getByRole('button',{name:'Keep open'})).toHaveCount(0)
  // The drawer slides in, unless the person asked for reduced motion.
  const animation=await drawer.evaluate(e=>getComputedStyle(e).animationName)
  if(motion==='reduce')expect(animation).toBe('none');else expect(animation).toContain('drawer-in')
  await drawer.getByRole('button',{name:'Close session navigation',exact:true}).click();await expect(drawer).toHaveCount(0)
  await expect(p.locator('.xterm-rows')).toHaveCSS('font-size','11px')
  await expect(p.locator('.tool-rows .terminal-toolbar')).toBeVisible()
  await expect(p.locator('.tool-rows .mobile-keys')).toBeVisible()
  await expect(p.locator('.pane>footer')).toBeHidden()
  await settle(p)
  const closedRows=await visibleRows(p)
  // Every tool stays reachable: the toolbar fades at its hidden end and scrolls to the last control.
  const tools=p.locator('.tool-rows .terminal-toolbar')
  await expect(tools).toHaveClass(/more-end/)
  await tools.evaluate(e=>e.scrollTo({left:e.scrollWidth}))
  const last=await p.getByRole('button',{name:'Maximise',exact:true}).boundingBox(),row=await tools.boundingBox()
  expect(last.x+last.width).toBeLessThanOrEqual(row.x+row.width+1)
  await tools.evaluate(e=>e.scrollTo({left:0}))
  // Keyboard open while typing: the site header, workspace toolbar and pane header step aside.
  const top=Math.round(height*.44)
  await p.locator('.xterm-helper-textarea').focus()
  await p.evaluate(value=>window.__keyboard(value),top)
  await expect(p.locator('.site-header')).toBeHidden()
  await expect(p.locator('.pane>header')).toBeHidden()
  await settle(p)
  const openRows=await visibleRows(p),paneBottom=(await box(p,'.pane')).bottom
  await expect(p.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true})).toBeAttached()
  await p.evaluate(()=>window.__keyboard(null))
  await expect(p.locator('.pane>header')).toBeVisible()
  await p.getByRole('button',{name:'Maximise',exact:true}).click()
  await p.locator('.xterm-helper-textarea').focus()
  await p.evaluate(value=>window.__keyboard(value),top)
  await settle(p)
  const maximisedRows=await visibleRows(p),maximisedBottom=(await box(p,'.pane')).bottom
  console.log(`Sideways ${width}x${height}: ${closedRows} rows, ${openRows} with the keyboard open, ${maximisedRows} maximised with the keyboard open`)
  expect(closedRows).toBeGreaterThanOrEqual(12)
  expect(openRows).toBeGreaterThanOrEqual(6)
  expect(maximisedRows).toBeGreaterThanOrEqual(6)
  expect(paneBottom).toBeGreaterThanOrEqual(top-10);expect(paneBottom).toBeLessThanOrEqual(top+1)
  expect(Math.abs(maximisedBottom-top)).toBeLessThanOrEqual(2)
  expect(side.frames.every(frame=>frame.rows>=8&&frame.cols>=20)).toBe(true)
  expect(side.errors).toEqual([])
  await side.context.close()
 }

 // ── A small tablet held sideways with tall browser chrome (834 × 480 page, an 834 px short screen side) is a tablet:
 // the sessions panel docks beside the terminal by default, with Keep open and the separator ──
 {
  const tab=await open({width:834,height:480,touch:true,screen:{width:1194,height:834}})
  const p=tab.page,panel=p.locator('aside#terminal-sessions.session-sidebar')
  await expect(panel).toBeVisible()
  await expect(panel.getByRole('button',{name:'Keep open',exact:true})).toHaveAttribute('aria-pressed','true')
  await expect(p.getByRole('separator',{name:'Resize session navigation'})).toBeVisible()
  const a=await panel.boundingBox(),t=await tab.pane.boundingBox()
  expect(a.x+a.width).toBeLessThanOrEqual(t.x+.5);expect(t.x+t.width).toBeLessThanOrEqual(834.5)
  expect(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(tab.errors).toEqual([])
  await tab.context.close()
 }

 // ── Desktop and tablet keep their layout; a narrow window without touch keeps its header ──
 for(const [width,height] of [[1360,900],[768,900],[390,844]]){
  const desk=await open({width,height,touch:false})
  const p=desk.page
  await expect(p.locator('.xterm-rows')).toHaveCSS('font-size','14px')
  if(width>650){
  await expect(p.locator('.pane>header')).toHaveCSS('padding','7px 12px')
  await expect(p.locator('.terminal-toolbar')).toHaveCSS('padding','5px 8px')
  await expect(p.locator('.screen-frame')).toHaveCSS('padding','12px 10px 8px 14px')
  await expect(p.locator('.terminal-toolbar')).toHaveCSS('mask-image','none')
  await expect(p.locator('.mobile-keys')).toBeHidden()
  await expect(p.locator('.pane>footer')).toBeVisible()
  await expect(p.locator('.tool-rows')).toHaveCount(0)
  }
  if(width===1360){
   // With a mouse, a quick return to the page sends nothing; a return after 30 s or more repaints once.
   await settle(p);const before=desk.frames.length
   await p.evaluate(()=>window.__visibility(true));await p.evaluate(()=>window.__visibility(false))
   await settle(p);expect(desk.frames.length).toBe(before)
   await p.evaluate(()=>{window.__visibility(true);window.__advance(31000);window.__visibility(false)})
   await settle(p);expect(desk.frames.length).toBe(before+2)
  }
  // No keyboard focus layout without a touch screen, even when the visual viewport shrinks.
  await p.getByRole('button',{name:'Focus terminal',exact:true}).click()
  await p.evaluate(height=>window.__keyboard(height),Math.round(height*.55))
  await settle(p)
  await expect(p.locator('.site-header')).toBeVisible()
  await expect(p.locator('.workspace-toolbar')).toBeVisible()
  await p.evaluate(()=>window.__keyboard(null))
  await p.getByRole('button',{name:'Maximise',exact:true}).click()
  await expect(p.locator('.maximise-hint')).toBeVisible()
  const maxBox=await box(p,'.pane')
  expect(Math.round(maxBox.top)).toBe(width>650?12:0)
  expect(Math.round(maxBox.height)).toBe(width>650?height-24:height)
  expect(desk.errors).toEqual([])
  await desk.context.close()
 }
 console.log('Terminal phone layout passed: 11 px default, compact chrome with 40 × 40 px tools and overflow fades, sideways phones, keyboard focus layout, no blank band, 20×8 floor, debounced and deduplicated resizes, repaint on return (gated, with a cooldown) and reattach, the viewport readout, the scoped viewport meta, desktop and tablet unchanged.')
}finally{await browser.close()}
