import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1440,height:1000},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage(),errors=[],inputs=[],tickets=[],mutations=[],commands=[]
let historyReads=0
const savedHistory={entries:[{command:'git status',source:'bash',timestamp:Date.now()},{command:"printf 'first'\nprintf 'second'",source:'zsh'}],sources:[{source:'bash',available:true},{source:'zsh',available:true}],truncated:false,skipped:0}
let profile={theme:'light',shelfView:'list',cardsExpanded:false,terminalRemoveOnClose:false},conflict=false
const now=Date.now(),a='organisation-machine00001',b='organisation-machine00002',c='organisation-machine00003'
const creations=new Map();let failCreateOnce=true
const machines=[{agentId:a,host:'Laptop',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,commandHistory:true,readiness:'ready'}},{agentId:b,host:'Builder',ready:false,serverEnabled:true,capabilities:{osUser:'builder',terminalEnabled:true,terminalControl:true,commandHistory:true,readiness:'ready'}}]
machines.push({agentId:c,host:'New machine',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,commandHistory:true,readiness:'ready'}})
const sessions=[{id:'organisation-session0001',agentId:a,title:'Alpha shell',state:'running',createdAt:now-4,updatedAt:now,exitCode:null},{id:'organisation-session0002',agentId:b,title:'Build shell',state:'running',createdAt:now-3,updatedAt:now,exitCode:null},{id:'organisation-session0003',agentId:a,title:'Closed shell',state:'ended',createdAt:now-2,updatedAt:now,exitCode:null},{id:'organisation-session0004',agentId:a,title:'Zulu shell',state:'running',createdAt:now-1,updatedAt:now,exitCode:null}]
page.on('pageerror',e=>errors.push(e.message))
await context.route('**/api/**',async route=>{
 const r=route.request(),path=new URL(r.url()).pathname,method=r.method();let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:7,name:'Engineer',email:'engineer@example.test',role:'owner'}}
 else if(path==='/api/v2/profile'){if(method==='PATCH'){profile={...profile,...r.postDataJSON()};mutations.push({kind:'profile',...r.postDataJSON()})}data={profile}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}
 else if(path==='/api/environments')data={environments:[],machines:[]}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'fixture',commit:'fixture'}
 else if(path==='/api/v2/terminal/machines')data={machines}
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[],history:[]}
 else if(path==='/api/v2/terminal/sessions'){
  if(method==='GET')data={sessions};else{const body=r.postDataJSON();mutations.push({kind:'create',...body});let item=creations.get(body.requestId);if(!item){item={id:`organisation-quick-session${creations.size}`,agentId:body.agentId,title:body.title,state:'running',createdAt:Date.now(),updatedAt:Date.now(),exitCode:null};creations.set(body.requestId,item);sessions.push(item)}if(failCreateOnce){failCreateOnce=false;return route.fulfill({status:503,json:{error:'Creation unconfirmed'}})}data=item}
 }
 else if(path.endsWith('/command-history')){historyReads++;data=savedHistory}
 else if(path.endsWith('/history'))return route.fulfill({body:'Read-only output snapshot'})
 else if(path.endsWith('/attachments')){tickets.push(path.split('/').at(-2));data={ticket:'organisation-ticket00001'}}
 else if(path==='/api/v2/terminal/commands'&&method==='GET')data={commands}
 else if(path.startsWith('/api/v2/terminal/commands')){
  if(conflict)return route.fulfill({status:409,json:{error:'This command changed in another browser.'}})
  const body=r.postDataJSON(),id=path.split('/').at(-1),old=commands.find(c=>c.id===id)
  if(method==='DELETE'){commands.splice(commands.indexOf(old),1);data={ok:true}}
  else {data={...body,id:old?.id||`command-${commands.length}`,revision:(old?.revision||0)+1};if(old)commands.splice(commands.indexOf(old),1);commands.push(data)}
 }else if(path.endsWith('/archive')){const id=path.split('/').at(-2);sessions.splice(sessions.findIndex(s=>s.id===id),1);mutations.push({kind:'archive',id});data={ok:true}}
 else if(path.endsWith('/end')||method==='PATCH'){
  const id=path.split('/').at(path.endsWith('/end')?-2:-1),body=r.postDataJSON(),item=sessions.find(s=>s.id===id)
  if(path.endsWith('/end')){item.state='end_requested';item.removeHistory=body.removeHistory;mutations.push({kind:'end',id,...body})}else{item.title=body.title;mutations.push({kind:'rename',id,...body})}data=item
 }else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
await context.routeWebSocket('**/api/v2/terminal/socket',socket=>socket.onMessage(message=>{if(typeof message==='string'){if(JSON.parse(message).ticket){socket.send('{"type":"ready"}');socket.send(Buffer.from('\x1b[2J\x1b[HWorkspace shell\r\nready> '))}}else{inputs.push(message.toString());socket.send(message)}}))
const sidebar=page.getByRole('complementary',{name:'Session navigation'}),menu=page.getByRole('menu'),pane=page.locator('.pane')
const row=name=>sidebar.locator('.session-row').filter({has:page.getByText(name,{exact:true})})
async function showSidebar(){if(await sidebar.count()===0)await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).click()}
async function live(){await expect(pane).toHaveAttribute('data-connection-state','live')}
try{
 await page.goto(`${base}/terminals?agent=${a}&session=${sessions[0].id}`);await live()
 await expect(sidebar.getByLabel('Machine',{exact:true})).toHaveValue('*')
 await expect(sidebar.locator('.session-row')).toHaveCount(4)
 const originalTicket=tickets.at(-1)
 await sidebar.getByText('Choose multiple machines',{exact:true}).click()
 await sidebar.getByRole('checkbox',{name:'Builder',exact:true}).uncheck()
 await expect(sidebar.locator('.session-row')).toHaveCount(3)
 await sidebar.getByRole('checkbox',{name:'Laptop',exact:true}).uncheck()
 await expect(sidebar.locator('.session-row')).toHaveCount(0)
 await sidebar.getByRole('checkbox',{name:'Builder',exact:true}).check()
 await expect(sidebar.locator('.session-row')).toHaveCount(1)
 await sidebar.getByRole('button',{name:'Show all machines'}).click()
 await sidebar.getByText('Choose multiple machines',{exact:true}).click()
 await expect(sidebar.locator('.session-row')).toHaveCount(4)
 expect(tickets.at(-1)).toBe(originalTicket)
 await row('Build shell').click({button:'right'})
 await expect(menu.getByRole('menuitem',{name:'Disconnect this browser'})).toBeDisabled()
 await menu.getByRole('menuitem',{name:'Rename session'}).click()
 await page.getByLabel('Session name',{exact:true}).fill('Builder renamed')
 await page.getByRole('button',{name:'Save name',exact:true}).click()
 await expect(row('Builder renamed')).toBeVisible()
 expect(tickets.at(-1)).toBe(originalTicket)
 await row('Builder renamed').press('Shift+F10')
 await menu.getByRole('menuitem',{name:'Stop session…'}).click()
 await page.getByLabel('Remove from history when this session ends').check()
 await page.getByLabel('Use this as my account default').check()
 await page.getByRole('dialog').getByRole('button',{name:'End session',exact:true}).click()
 await expect(row('Builder renamed')).toContainText('end requested')
 expect(mutations.find(m=>m.kind==='end')).toMatchObject({id:sessions[1].id,removeHistory:true})
 await expect.poll(()=>profile.terminalRemoveOnClose).toBe(true)
 expect(tickets.at(-1)).toBe(originalTicket)
 await sidebar.getByLabel('Group sessions').selectOption('none')
 await sidebar.getByLabel('Sort sessions').selectOption('name')
 expect(await sidebar.locator('.session-row strong').allTextContents()).toEqual(['Alpha shell','Builder renamed','Closed shell','Zulu shell'])
 await row('Zulu shell').click({button:'right'})
 await menu.getByRole('menuitemradio',{name:'Blue',exact:true}).click()
 await menu.press('Escape')
 await sidebar.getByLabel('Group sessions').selectOption('colour')
 await expect(sidebar.getByRole('heading',{name:'Blue 1'})).toBeVisible()
 await sidebar.getByLabel('Group sessions').selectOption('none')
 await sidebar.getByLabel('Sort sessions').selectOption('custom')
 await sidebar.getByRole('button',{name:'Actions for Zulu shell'}).dragTo(row('Alpha shell'))
 await expect(sidebar.locator('.session-row strong').first()).toHaveText('Zulu shell')
 const separator=sidebar.getByRole('separator'),box=await separator.boundingBox()
 await page.mouse.move(box.x+box.width/2,box.y+70);await page.mouse.down();await page.mouse.move(box.x+110,box.y+70,{steps:10});await page.mouse.up()
 await expect.poll(async()=>Math.round((await sidebar.boundingBox()).width)).toBeGreaterThan(360)
 await separator.focus();await separator.press('End');await expect(separator).toHaveAttribute('aria-valuenow','480')
 await page.reload();await live()
 await expect(sidebar.getByLabel('Sort sessions')).toHaveValue('custom')
 await expect(sidebar.locator('.session-row strong').first()).toHaveText('Zulu shell')
 await expect(sidebar.getByRole('separator')).toHaveAttribute('aria-valuenow','480')
 await row('Alpha shell').click({button:'right'});await menu.getByRole('menuitem',{name:'Stop session…'}).click()
 await expect(page.getByLabel('Remove from history when this session ends')).toBeChecked()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await row('Closed shell').click({button:'right'});await menu.getByRole('menuitem',{name:'Remove from history…'}).click()
 await page.getByRole('dialog').getByRole('button',{name:'Remove from history',exact:true}).click()
 await expect(row('Closed shell')).toHaveCount(0)
 await sidebar.getByLabel('Machine',{exact:true}).selectOption(b)
 await expect(sidebar.locator('.session-row')).toHaveCount(1)
 await sidebar.getByLabel('Machine',{exact:true}).selectOption('*')
 await expect(sidebar.locator('.session-row')).toHaveCount(3)
 await row('Alpha shell').click();await live()
 await page.getByRole('button',{name:'Recent history',exact:true}).click()
 await page.getByLabel('Search command history').fill('git status')
 await expect(page.getByRole('list',{name:'Saved shell commands'}).locator('li')).toHaveCount(1)
 const beforeRecall=inputs.length
 await page.getByRole('button',{name:'Paste history command 1'}).click()
 await expect.poll(()=>inputs.slice(beforeRecall).join('')).toBe('git status')
 await page.getByRole('button',{name:'Interrupt shell',exact:true}).click()
 await page.getByRole('button',{name:'Recent history',exact:true}).click()
 await page.getByLabel('Search command history').fill('printf')
 const beforeRun=inputs.length
 await page.getByRole('button',{name:'Run history command 1'}).click()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(page.getByLabel('Search command history')).toHaveValue('printf')
 expect(inputs.length).toBe(beforeRun)
 await page.getByRole('button',{name:'Run history command 1'}).click()
 await page.getByRole('button',{name:'Run command',exact:true}).click()
 await expect.poll(()=>inputs.slice(beforeRun).join('')).toBe("printf 'first'\rprintf 'second'\r")
 machines[0].capabilities.commandHistory=false
 await sidebar.getByRole('button',{name:'Refresh status',exact:true}).click()
 await page.getByRole('button',{name:'Recent history',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Update this machine’s reet agent')
 const legacyReads=historyReads
 await page.getByRole('button',{name:'Terminal output history →',exact:true}).click()
 await expect(page.getByRole('region',{name:'Earlier terminal output'})).toContainText('Read-only output snapshot')
 await page.getByRole('button',{name:'Back to live ↓',exact:true}).click()
 expect(historyReads).toBe(legacyReads)
 await page.getByRole('button',{name:'Appearance',exact:true}).click()
 await page.getByRole('button',{name:'Use Graphite theme'}).click()
 await expect(page.getByRole('region',{name:'Remote terminal screen',exact:true})).toHaveCSS('background-color','rgb(28, 30, 34)')
 await page.getByRole('button',{name:'Close Terminal appearance'}).click()
 if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/organisation-desktop.png`})}
 await page.getByRole('button',{name:'Saved commands',exact:true}).click()
 await page.getByRole('button',{name:'Save your first command'}).click()
 await page.getByLabel('Command name').fill('Disk usage');await page.getByLabel('Saved command text').fill('df -h')
 await page.getByRole('button',{name:'Save command',exact:true}).click()
 await expect(page.getByRole('dialog').getByRole('status')).toContainText('Saved to your account.')
 const before=inputs.length
 await page.getByRole('button',{name:'Insert Disk usage'}).click()
 await expect.poll(()=>inputs.slice(before).join('')).toBe('df -h')
 await page.getByRole('button',{name:'Saved commands',exact:true}).click()
 await page.getByText('Disk usage',{exact:true}).click({button:'right'})
 await page.getByRole('menuitem',{name:'Edit command'}).click()
 await page.getByLabel('Saved command text').fill('df -h\necho reviewed')
 conflict=true;await page.getByRole('button',{name:'Save command',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('changed in another browser')
 await expect(page.getByLabel('Saved command text')).toHaveValue('df -h\necho reviewed')
 conflict=false;await page.getByRole('button',{name:'Save command',exact:true}).click()
 const beforeReview=inputs.length
 await page.getByRole('button',{name:'Insert Disk usage'}).click()
 await expect(page.getByRole('dialog')).toContainText('Paste')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 expect(inputs.length).toBe(beforeReview)
 await page.getByRole('button',{name:'Close session navigation'}).click()
 await page.reload();await live();await expect(sidebar).toHaveCount(0)
 await showSidebar();await expect(sidebar.locator('.session-row')).toHaveCount(3)
 // Machine quick actions include empty machines and preserve ambiguous-create IDs.
 await sidebar.getByLabel('Group sessions').selectOption('machine')
 const laptopGroup=sidebar.locator('.session-group').filter({has:page.getByRole('heading',{name:/^Laptop/})})
 const builderGroup=sidebar.locator('.session-group').filter({has:page.getByRole('heading',{name:/^Builder/})})
 const emptyGroup=sidebar.locator('.session-group').filter({has:page.getByRole('heading',{name:/^New machine/})})
 await expect(emptyGroup).toContainText('No sessions yet')
 await expect(builderGroup.getByRole('button',{name:'New terminal on Builder',exact:true})).toBeDisabled()
 const beforeQuickTickets=tickets.length
 await laptopGroup.getByRole('button',{name:'Laptop',exact:true}).click({button:'right'})
 await expect(menu).toHaveAccessibleName('Actions for machine Laptop')
 await menu.press('Escape');expect(tickets.length).toBe(beforeQuickTickets)
 await emptyGroup.getByRole('button',{name:'New terminal on New machine',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('Creation unconfirmed')
 expect(new URL(page.url()).searchParams.get('agent')).toBe(c)
 expect(new URL(page.url()).searchParams.get('session')).toBeNull()
 expect(tickets.length).toBe(beforeQuickTickets)
 await page.getByRole('button',{name:'Retry creation',exact:true}).click();await live()
 const creates=mutations.filter(item=>item.kind==='create')
 expect(creates).toHaveLength(2);expect(creates[0].agentId).toBe(c);expect(creates[0].title).toBe('Shell');expect(creates[1]).toEqual(creates[0]);expect(creations.size).toBe(1)
 expect(tickets.at(-1)).toBe([...creations.values()][0].id)
 // The collapsed sidebar can be reopened beside the terminal, including theatre view.
 await sidebar.getByRole('button',{name:'Close session navigation',exact:true}).click()
 await page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true}).click()
 await expect(sidebar).toBeVisible()
 await page.getByRole('button',{name:'Theatre view',exact:true}).click()
 await expect(sidebar).toBeHidden()
 await page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true}).click()
 await expect(sidebar).toBeVisible();await expect(page.locator('.workspace')).not.toHaveClass(/theatre/)
 await page.getByRole('button',{name:'Maximise',exact:true}).click()
 await page.locator('.terminal-toolbar').getByRole('button',{name:'Show sessions',exact:true}).click()
 await expect(sidebar).toBeVisible();await expect(pane).not.toHaveClass(/maximised/)
 const beforeMenuInput=inputs.length
 await page.getByRole('region',{name:'Remote terminal screen',exact:true}).click({button:'right'})
 await page.getByRole('menu',{name:'Terminal actions',exact:true}).getByRole('menuitem',{name:'Saved commands…',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Saved commands')
 await page.getByRole('button',{name:'Close Saved commands',exact:true}).click()
 expect(inputs.length).toBe(beforeMenuInput)
 // Active landing cards target the clicked session, with explicit stop/removal review.
 await page.goto(`${base}/control`)
 const band=page.locator('#active-sessions'),target=band.getByRole('link',{name:'Continue Zulu shell on Laptop',exact:true})
 const landingTickets=tickets.length
 expect(await target.evaluate(element=>element.dispatchEvent(new MouseEvent('contextmenu',{button:2,shiftKey:true,bubbles:true,cancelable:true})))).toBe(true)
 await expect(menu).toHaveCount(0)
 await target.click({button:'right'})
 expect(await band.getByRole('list',{name:'Active terminal sessions'}).evaluate(element=>[...element.children].every(child=>child.tagName==='LI'))).toBe(true)
 await page.getByRole('menuitem',{name:'Rename session…',exact:true}).click()
 await page.getByLabel('Session name',{exact:true}).fill('Landing rename')
 await page.getByRole('button',{name:'Save name',exact:true}).click()
 await expect(band.getByRole('link',{name:'Continue Landing rename on Laptop',exact:true})).toBeVisible()
 expect(mutations.filter(m=>m.kind==='rename').at(-1).id).toBe('organisation-session0004')
 await band.getByRole('link',{name:'Continue Landing rename on Laptop',exact:true}).click({button:'right'})
 await page.getByRole('menuitem',{name:'Stop session…',exact:true}).click()
 await expect(page.getByLabel('Remove from history when this session ends')).toBeChecked()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 const stops=mutations.filter(m=>m.kind==='end').length
 await band.getByRole('link',{name:'Continue Landing rename on Laptop',exact:true}).click({button:'right'})
 await page.getByRole('menuitem',{name:'Stop session…',exact:true}).click()
 await page.getByRole('button',{name:'End session',exact:true}).click()
 await expect(band.getByRole('link',{name:'Continue Landing rename on Laptop',exact:true})).toContainText('End requested')
 expect(mutations.filter(m=>m.kind==='end').length).toBe(stops+1)
 expect(mutations.filter(m=>m.kind==='end').at(-1)).toMatchObject({id:'organisation-session0004',removeHistory:true})
 expect(tickets.length).toBe(landingTickets)
 await band.getByRole('link',{name:'Continue Alpha shell on Laptop',exact:true}).click({button:'right'})
 await page.getByRole('menuitem',{name:'Saved commands…',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Saved commands')
 await expect(page.getByRole('button',{name:/^Insert /})).toHaveCount(0)
 await page.getByRole('button',{name:'Close Saved commands',exact:true}).click()
 const landingMachine=page.getByRole('article',{name:'Machine Builder',exact:true})
 await expect(landingMachine.getByRole('button',{name:'New terminal on Builder',exact:true})).toBeDisabled()
 await landingMachine.click({button:'right'})
 await expect(page.getByRole('menuitem',{name:'New terminal',exact:true})).toBeDisabled()
 await menu.press('Escape')
 const createsBeforeLanding=mutations.filter(m=>m.kind==='create').length
 await page.getByRole('article',{name:'Machine Laptop',exact:true}).getByRole('button',{name:'New terminal on Laptop',exact:true}).click()
 await live()
 await expect.poll(()=>new URL(page.url()).searchParams.get('agent')).toBe(a)
 expect(mutations.filter(m=>m.kind==='create').length).toBe(createsBeforeLanding+1)
 expect(mutations.filter(m=>m.kind==='create').at(-1).agentId).toBe(a)
 await page.goto(`${base}/terminals?agent=${a}&session=organisation-session0001`);await live();await showSidebar()
 await page.setViewportSize({width:390,height:844});await showSidebar()
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/organisation-phone.png`})}
 expect(errors).toEqual([])
 console.log('Session organisation passed: immutable cross-machine context actions, pending close/removal preference, sorting/grouping, colours, drag ordering, resize/collapse persistence, themes and guarded command insertion.')
}finally{await browser.close()}
