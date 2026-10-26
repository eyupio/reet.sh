import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:900},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage(),errors=[],inputs=[],tickets=[],ticketSizes=[],writes=[]
page.on('pageerror',error=>errors.push(error.message))
const now=Date.now(),agentA='connector-laptop000000001',agentB='connector-builder00000001',sessionA='session-build00000000001',sessionB='session-deploy0000000001'
let failHistory=false,historyReads=0,failSessions=false,denied=false,viewer=false,websockets=0
const sessions=[{id:sessionA,agentId:agentA,title:'Build workspace',state:'running',createdAt:now,updatedAt:now,exitCode:null},{id:sessionB,agentId:agentB,title:'Deployment checks',state:'running',createdAt:now,updatedAt:now,exitCode:null}]
const machines=[{agentId:agentA,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,readiness:'ready'}},{agentId:agentB,host:'build-server',ready:true,serverEnabled:true,capabilities:{osUser:'builder',terminalEnabled:true,terminalControl:true,readiness:'ready'}}]
await page.addInitScript(()=>{
 window.__nativeClipboard=navigator.clipboard
 let clipboard='from-clipboard'
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{readText:async()=>clipboard,writeText:async text=>{clipboard=text;window.__copied=text}}})
 window.__setClipboard=text=>clipboard=text
})
await page.route('**/api/**',async route=>{
 const request=route.request(),url=new URL(request.url()),path=url.pathname
 let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Workspace tester',email:'test@example.test',role:viewer?'viewer':'owner'}}
 else if(path==='/api/v2/profile')data={profile:{theme:'light',shelfView:'list',cardsExpanded:false}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}
 else if(path==='/api/environments')data={environments:[],machines:[]}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data={environments:[],machines:[],targets:[],agents:[],jobs:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'fixture',commit:'fixture'}
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[],history:[]}
 else if(path==='/api/v2/terminal/machines')data={machines}
 else if(path==='/api/v2/terminal/status')data={available:true,configured:true,reason:''}
 else if(path==='/api/v2/terminal/sessions'){
  if(request.method()!=='GET'){writes.push(request.postDataJSON());return route.fulfill({status:400,json:{error:'No implicit creation permitted'}})}
  if(failSessions||denied)return route.fulfill({status:denied?403:503,json:{error:'Unavailable'}})
  data={sessions}
 }
 else if(path.endsWith('/attachments')){tickets.push(path.split('/').at(-2));ticketSizes.push(request.postDataJSON());data={ticket:'fixture-ticket00000000000'}}
 else if(path.endsWith('/history')){historyReads++;return route.fulfill(failHistory?{status:503,json:{error:'History temporarily unavailable'}}:{body:Array.from({length:400},(_,i)=>`earlier output ${i}`).join('\n')})}
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
let releaseInitialRedraw,holdInitialRedraw=true
await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{
 websockets++
 socket.onMessage(message=>{
  if(typeof message==='string'){
   const data=JSON.parse(message)
   if(data.ticket){socket.send('{"type":"ready"}');const redraw=()=>socket.send(Buffer.from('\x1b[?2026h\x1b[2J\x1b[HExisting persistent shell\r\nready> \x1b[?2026l'));if(holdInitialRedraw){holdInitialRedraw=false;releaseInitialRedraw=redraw}else redraw()}
  }else{inputs.push(message.toString());socket.send(message)}
 })
})
const pane=page.locator('.pane'),screen=page.getByRole('region',{name:'Remote terminal screen',exact:true})
async function live(){await expect(pane).toHaveAttribute('data-connection-state','live')}
async function bounds(width,height){
 await page.setViewportSize({width,height})
 await expect.poll(async()=>screen.evaluate(element=>Math.round(element.getBoundingClientRect().height))).toBeGreaterThan(height*(height<600?.2:width<650?.5:.6))
 await expect.poll(async()=>screen.evaluate(element=>Math.round(element.getBoundingClientRect().bottom))).toBeLessThanOrEqual(height+1)
 const b=await screen.boundingBox()
 // The renderer itself must fit, including its final row; outer bounds alone miss clipped content.
 await expect.poll(async()=>{const host=await screen.boundingBox(),rendered=await page.locator('.xterm-screen').boundingBox(),padding=await screen.evaluate(e=>parseFloat(getComputedStyle(e).paddingBottom)||0);return Math.ceil(rendered.y+rendered.height-(host.y+host.height-padding))}).toBeLessThanOrEqual(0)
 console.log(`Screen ${width}x${height}: top=${Math.round(b.y)}, height=${Math.round(b.height)} (${Math.round(b.height/height*100)}%)`)
 expect((await pane.boundingBox()).y).toBeLessThan(160)
 expect(b.y).toBeLessThan(240)
 expect(b.y+b.height).toBeLessThanOrEqual(height+1)
 expect(await page.evaluate(()=>scrollY)).toBe(0)
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 expect(await page.evaluate(()=>document.documentElement.scrollHeight<=innerHeight+1)).toBe(true)
}
try{
 await page.goto(`${base}/control`)
 const hero=page.getByRole('region',{name:'2 active sessions'})
 await expect(hero).toBeVisible()
 expect((await hero.boundingBox()).y).toBeLessThan(400)
 const links=hero.getByRole('link',{name:/^Continue /})
 await expect(links).toHaveCount(2)
 failSessions=true
 await hero.getByRole('button',{name:'Refresh active sessions'}).click()
 await expect(hero).toContainText('Showing the last received session list')
 await expect(links).toHaveCount(2)
 failSessions=false
 await hero.getByRole('button',{name:'Refresh active sessions'}).click()
 await expect(hero.getByText('Last known',{exact:true})).toHaveCount(0)
 const started=Date.now()
 await hero.getByRole('link',{name:'Continue Build workspace on dev-laptop'}).click()
 await expect(pane).toHaveAttribute('data-connection-state','resynchronising')
 // Let the restoration status change the layout before the host redraw arrives.
 await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))
 expect(await page.locator('.xterm-accessibility-tree [role=listitem]').count()).toBe(ticketSizes[0].rows)
 releaseInitialRedraw()
 await live()
 await expect(page.locator('.xterm-accessibility-tree')).toContainText('Existing persistent shell')
 console.log(`Synthetic continuation: ${Date.now()-started}ms`)
 expect(Date.now()-started).toBeLessThan(2000)
 expect(tickets.at(-1)).toBe(sessionA)
 expect(writes).toHaveLength(0)
 await bounds(1360,900)
 const connections=websockets
 await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).click()
 await bounds(1360,900)
 expect(websockets).toBe(connections)
 await page.getByRole('button',{name:'Details & help',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Controller terminal access')
 await page.getByRole('button',{name:'Close Machine & terminal details'}).click()
 await live();expect(websockets).toBe(connections)
 // Shell control keys reach the wire; desktop clipboard shortcuts do not duplicate input.
 await page.getByRole('button',{name:'Focus terminal',exact:true}).click()
 await page.keyboard.press('Control+c')
 await expect.poll(()=>inputs.join('')).toContain('\x03')
 await page.keyboard.press('Control+x')
 await expect.poll(()=>inputs.join('')).toContain('\x18')
 // Copy selected output without interrupting, then use the browser's real native paste event.
 const grid=await page.locator('.xterm-screen').boundingBox()
 await page.mouse.move(grid.x+2,grid.y+8);await page.mouse.down();await page.mouse.move(grid.x+100,grid.y+8,{steps:10});await page.mouse.up()
 const beforeCopy=inputs.length
 await page.keyboard.press('Control+c')
 await expect.poll(()=>page.evaluate(()=>window.__copied||'')).not.toBe('')
 expect(inputs.length).toBe(beforeCopy)
 await page.getByRole('button',{name:'Interrupt shell',exact:true}).click()
 await expect.poll(()=>inputs.length).toBeGreaterThan(beforeCopy)
 expect(inputs.at(-1)).toBe('\x03')
 await page.evaluate(()=>window.__nativeClipboard.writeText('native-paste-once'))
 await page.getByRole('button',{name:'Focus terminal',exact:true}).click()
 await page.keyboard.press('Control+v')
 await expect.poll(()=>inputs.join('')).toContain('native-paste-once')
 expect(inputs.join('').split('native-paste-once').length-1).toBe(1)
 // Normal app fields retain native cut semantics and do not forward editor keys.
 await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).click()
 const filter=page.getByRole('searchbox',{name:'Find sessions'})
 await filter.fill('cut this filter');await filter.press('Control+a')
 const beforeCut=inputs.length
 await filter.press('Control+x');await expect(filter).toHaveValue('')
 expect(inputs.length).toBe(beforeCut)
 await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).click()
 await page.evaluate(()=>window.__setClipboard('from-clipboard'))
 await screen.dispatchEvent('contextmenu',{button:2,clientX:300,clientY:220})
 await expect(page.getByRole('menu',{name:'Terminal actions'})).toBeVisible()
 await page.getByRole('menuitem',{name:'Paste',exact:true}).click()
 await expect.poll(()=>inputs.join('')).toContain('from-clipboard')
 expect(inputs.join('').split('from-clipboard').length-1).toBe(1)
 const beforeDenied=inputs.length
 await page.evaluate(()=>{window.__normalRead=navigator.clipboard.readText;navigator.clipboard.readText=async()=>{throw new DOMException('denied','NotAllowedError')}})
 await page.getByRole('button',{name:'Paste',exact:true}).click()
 await expect(page.getByText('Clipboard access was denied. Paste with your keyboard inside the terminal.',{exact:true})).toBeVisible()
 expect(inputs.length).toBe(beforeDenied)
 await page.evaluate(()=>navigator.clipboard.readText=window.__normalRead)
 await page.evaluate(()=>window.__setClipboard('context-danger\n'))
 const beforePaste=inputs.length
 await screen.dispatchEvent('contextmenu',{button:2,clientX:300,clientY:220})
 await page.getByRole('menuitem',{name:'Paste',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Paste into the remote shell?')
 await page.getByRole('dialog').getByRole('button',{name:'Cancel',exact:true}).click()
 expect(inputs.length).toBe(beforePaste)
 await screen.evaluate(element=>element.dispatchEvent(new MouseEvent('contextmenu',{button:2,shiftKey:true,bubbles:true,cancelable:true})))
 await expect(page.getByRole('menu')).toHaveCount(0)
 // Earlier output has a native scrollbar; reading/searching never sends shell input.
 const beforeScroll=inputs.length,beforeViews=websockets,sidebarBefore=await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).getAttribute('aria-expanded')
 await page.getByRole('button',{name:'Earlier output',exact:true}).click()
 const output=page.getByRole('region',{name:'Scrollable terminal output',exact:true})
 await expect(output).toContainText('earlier output 0')
 expect(await output.evaluate(e=>getComputedStyle(e).overflowY)).toBe('auto')
 expect(await output.evaluate(e=>e.scrollHeight>e.clientHeight)).toBe(true)
 const outputBox=await output.boundingBox();await page.mouse.move(outputBox.x+80,outputBox.y+80);await page.mouse.wheel(0,-300)
 await expect.poll(()=>output.evaluate(e=>e.scrollTop)).toBeLessThan(await output.evaluate(e=>e.scrollHeight-e.clientHeight))
 await output.focus();await page.keyboard.press('Control+Home')
 await expect.poll(()=>output.evaluate(e=>e.scrollTop)).toBe(0)
 await page.getByLabel('Find in earlier output').fill('earlier output 8')
 await expect(output.locator('mark')).toHaveCount(11)
 await page.getByRole('button',{name:'Next output match',exact:true}).click()
 await expect(page.getByText('2 / 11',{exact:true})).toBeVisible()
 failHistory=true
 await page.getByRole('region',{name:'Earlier terminal output'}).getByRole('button',{name:'Refresh',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('Showing the previous snapshot')
 await expect(output).toContainText('earlier output 399')
 failHistory=false
 await page.getByRole('button',{name:'Back to live ↓',exact:true}).click()
 expect(inputs.length).toBe(beforeScroll);expect(historyReads).toBe(2)
 await page.getByRole('button',{name:'Focus terminal',exact:true}).click();await page.keyboard.press('Shift+PageUp')
 await expect(output).toBeVisible();await page.getByRole('button',{name:'Back to live ↓',exact:true}).click()
 expect(inputs.length).toBe(beforeScroll)
 await page.getByRole('button',{name:'Theatre view',exact:true}).click()
 await expect(page.locator('.workspace')).toHaveClass(/theatre/)
 await expect(page.getByRole('navigation',{name:'Terminal sessions'})).toBeHidden()
 await expect(page.locator('.site-header')).toBeVisible()
 await expect(page.locator('.workspace-toolbar')).toBeHidden()
 expect((await screen.boundingBox()).width).toBeGreaterThan(1250)
 await page.getByRole('button',{name:'Maximise',exact:true}).click()
 await expect(pane).toHaveClass(/maximised/)
 await page.getByRole('button',{name:'Restore size',exact:true}).click()
 await page.getByRole('button',{name:'Exit theatre view',exact:true}).click()
 await expect(page.locator('.workspace-toolbar')).toBeVisible()
 expect(await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).getAttribute('aria-expanded')).toBe(sidebarBefore)
 expect(websockets).toBe(beforeViews);expect(inputs.length).toBe(beforeScroll)
 // A deliberate browser disconnect survives page visibility/network events.
 await page.getByRole('button',{name:'Disconnect',exact:true}).click()
 await expect(pane).toHaveAttribute('data-connection-state','detached')
 const detached=websockets
 await page.evaluate(()=>{dispatchEvent(new Event('online'));document.dispatchEvent(new Event('visibilitychange'))})
 await expect(pane).toHaveAttribute('data-connection-state','detached')
 expect(websockets).toBe(detached)
 await page.getByRole('button',{name:'Reconnect',exact:true}).click()
 await live();expect(tickets.at(-1)).toBe(sessionA);expect(writes).toHaveLength(0)
 for(const [width,height] of [[390,844],[768,900],[1360,900],[390,420]]){
  await bounds(width,height)
  await page.evaluate(()=>document.documentElement.dataset.theme='dark')
  if(process.env.REET_UI_SCREENSHOTS&&height===844){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/workspace-mobile-dark.png`})}
 }
 await page.setViewportSize({width:1360,height:900})
 await page.evaluate(()=>document.documentElement.dataset.theme='light')
 if(process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/workspace-desktop.png`})
 await page.goto(`${base}/control`)
 await page.getByRole('link',{name:'Continue Deployment checks on build-server'}).click()
 await live();expect(tickets.at(-1)).toBe(sessionB);expect(writes).toHaveLength(0)
 await page.goto(`${base}/machines?view=terminal&agent=${agentA}&session=${sessionA}`)
 await live();await bounds(1360,900)
 // A new session always names its machine first; arriving without one attaches nothing.
 await page.goto(`${base}/terminals`)
 await expect(page.getByRole('heading',{name:'Terminals'})).toBeVisible()
 expect(await page.locator('.workspace-toolbar').innerText()).not.toContain('dev-laptop')
 await page.getByRole('button',{name:'New session',exact:true}).click()
 await expect(page.getByRole('dialog',{name:'Choose a machine'})).toBeVisible()
 await page.getByRole('button',{name:'Close Choose a machine'}).click()
 await expect(page.getByRole('button',{name:'New terminal',exact:true})).toBeEnabled()
 await page.getByRole('button',{name:'New terminal',exact:true}).click()
 const chooseMachine=page.getByRole('dialog',{name:'Choose a machine'})
 await expect(chooseMachine).toBeVisible()
 await chooseMachine.getByRole('button',{name:/dev-laptop/}).click()
 await expect(page.getByRole('dialog',{name:'New terminal session'})).toContainText('dev-laptop')
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 await expect(page.getByRole('heading',{name:'Your next shell starts here.'})).toBeVisible()
 expect(writes).toHaveLength(0)
 // Denied refresh clears the private hero inventory; viewers never fetch terminal APIs.
 await page.goto(`${base}/control`);denied=true
 await page.getByRole('button',{name:'Refresh active sessions'}).click()
 await expect(page.getByRole('link',{name:/^Continue /})).toHaveCount(0)
 denied=false;viewer=true
 const terminalRequests=[]
 page.on('request',request=>{if(new URL(request.url()).pathname.startsWith('/api/v2/terminal/'))terminalRequests.push(request.url())})
 await page.goto(`${base}/control`)
 await expect(page.getByRole('heading',{name:'Your fleet. Ready for work.'})).toBeVisible()
 await expect(page.locator('#active-sessions')).toHaveCount(0)
 expect(terminalRequests).toEqual([])
 expect(errors).toEqual([])
 console.log('Terminal workspace passed: viewport bounds, quick continuation, per-session targeting, no creation, drawers, native control keys, guarded context paste, intentional disconnect, responsive layouts and role isolation.')
}catch(error){if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/workspace-failure.png`,fullPage:true})}throw error}finally{await browser.close()}
