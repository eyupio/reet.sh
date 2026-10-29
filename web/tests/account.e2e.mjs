// Settings > Agents contract: the account's new-session defaults (agent, model, thinking, base branch, worktrees),
// the favourite-models manager (star/unstar, add with Other, move, one default per agent, stale order), a viewer's
// read-only view, and the phone fit. Runs against the built app with every API answer mocked.
import { chromium, expect } from '@playwright/test'
import { installApi } from './helpers/longtext.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY ? new RegExp(process.env.REET_E2E_ONLY, 'i') : null
const run = name => !only || only.test(name)

const now = Date.now()
const owner = {id:1, name:'Sam Example', email:'sam@example.test', role:'owner'}
const viewer = {id:3, name:'Viewer', email:'v@example.test', role:'viewer'}
let currentUser = owner
let profile, favourites, patches, addCalls, deleteCalls, orderCalls, defaultCalls, staleOrder, defaultGate

function reset(user = owner) {
  currentUser = user
  profile = {theme:'system', timezone:'Europe/London', dateFormat:'', shelfView:'list', cardsExpanded:false, hostedPrefix:'reet-', updatedAt:now,
    defaultBaseBranch:'', worktreesByDefault:false, defaultTool:'', defaultModel:'', defaultEffort:''}
  favourites = {version:1, favourites:[{tool:'claude', model:'sonnet', agentDefault:false}, {tool:'opencode', model:'anthropic/claude-sonnet-4-5', agentDefault:false}]}
  patches = []; addCalls = []; deleteCalls = []; orderCalls = []; defaultCalls = []; staleOrder = false; defaultGate = null
}

const extra = {
  '/api/v2/profile': req => {
    if (req.method() === 'PATCH') {
      const body = req.postDataJSON()
      patches.push(body)
      profile = {...profile, ...body, updatedAt:Date.now()}
    }
    return {user:currentUser, profile, sessionCount:1, accounts:true}
  },
  '/api/v2/profile/model-favourites': req => {
    const url = new URL(req.url())
    if (req.method() === 'POST') {
      const body = req.postDataJSON()
      addCalls.push(body)
      if (!favourites.favourites.some(f => f.tool === body.tool && f.model === body.model)) {
        favourites = {version:favourites.version + 1, favourites:[...favourites.favourites, {tool:body.tool, model:body.model, agentDefault:false}]}
      }
      return favourites
    }
    if (req.method() === 'DELETE') {
      const tool = url.searchParams.get('tool'), model = url.searchParams.get('model')
      deleteCalls.push({tool, model})
      favourites = {version:favourites.version + 1, favourites:favourites.favourites.filter(f => f.tool !== tool || f.model !== model)}
      return {}
    }
    return favourites
  },
  '/api/v2/profile/model-favourites/order': req => {
    const body = req.postDataJSON()
    orderCalls.push(body)
    const byKey = new Map(favourites.favourites.map(f => [`${f.tool}\u0000${f.model}`, f]))
    favourites = {version:favourites.version + 1, favourites:body.items.map(item => ({...byKey.get(`${item.tool}\u0000${item.model}`), tool:item.tool, model:item.model}))}
    return favourites
  },
  '/api/v2/profile/model-favourites/default': req => {
    const body = req.postDataJSON()
    defaultCalls.push(body)
    favourites = {version:favourites.version + 1, favourites:favourites.favourites.map(f => f.tool === body.tool ? {...f, agentDefault:!!body.model && f.model === body.model} : f)}
    return favourites
  },
  '/api/v2/models': req => {
    const tool = new URL(req.url()).searchParams.get('tool')
    return {machines:[{agentId:'studio', host:'studio', state:'listed', tool,
      models:tool === 'opencode' ? [{id:'openai/gpt-5', provider:'openai'}, {id:'anthropic/claude-sonnet-4-5', provider:'anthropic'}] : [],
      more:false, source:'opencode models', version:'', fetchedAt:now, checkedAt:now, reason:'', sentence:''}]}
  },
}

async function open({viewport = {width:1280, height:900}, touch = false, user = owner} = {}) {
  reset(user)
  const status = {mode:'accounts', setupRequired:false, bootstrapRequired:false, signupEnabled:false, user}
  const context = await browser.newContext({viewport, hasTouch:touch, isMobile:touch})
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', e => errors.push(e.message))
  const unmocked = await installApi(page, {status, extra})
  // The stale-order case needs a 409 status, which the shared fixture's own answers cannot send.
  await page.route('**/api/v2/profile/model-favourites/order', async route => {
    if (staleOrder) { staleOrder = false; return route.fulfill({status:409, json:{error:'Your favourites changed. Reload them, then try again.'}}) }
    return route.fallback()
  })
  // Hold one default answer so the local one-default-per-agent rule is seen before the server's list arrives.
  await page.route('**/api/v2/profile/model-favourites/default', async route => {
    if (defaultGate) await defaultGate
    return route.fallback()
  })
  await page.goto(`${base}/account/profile#agents`)
  await expect(page.getByRole('heading', {name:'Agents', level:2})).toBeVisible()
  await expect(page).toHaveURL(`${base}/account/agents`)
  await expect(page.locator('nav[aria-label="Account"] a[href="/account/agents"]').first()).toHaveAttribute('aria-current', 'page')
  return {context, page, errors, unmocked}
}

try {
  // A writer saves the new-session agent, model, thinking and the existing base-branch/worktree defaults in one PATCH.
  if (run('defaults save')) {
    const {context, page, errors, unmocked} = await open()
    await page.getByLabel('Default agent', {exact:true}).selectOption('cline')
    await expect(page.getByLabel('Thinking')).toBeVisible()
    await page.getByLabel('Thinking').selectOption('high')
    await page.getByRole('button', {name:/^Model:/}).click()
    const list = page.getByRole('listbox', {name:'Model'})
    await list.getByRole('option', {name:'Other…'}).click()
    await page.getByLabel('Model name').fill('cline/model-x')
    await page.getByRole('button', {name:'Use', exact:true}).click()
    await expect(page.getByRole('button', {name:'Model: cline/model-x'})).toBeVisible()
    await page.getByLabel('Base branch for fresh worktrees').fill('main')
    await page.getByLabel('Worktrees by default').selectOption('true')
    await page.getByRole('button', {name:'Save defaults'}).click()
    await expect(page.getByText(/defaults saved/)).toBeVisible()
    expect(patches).toHaveLength(1)
    expect(patches[0]).toMatchObject({defaultTool:'cline', defaultModel:'cline/model-x', defaultEffort:'high', defaultBaseBranch:'main', worktreesByDefault:true})
    expect(errors).toEqual([])
    expect(unmocked).toEqual([])
    await context.close()
  }

  // A model needs its agent: the page refuses before the server, and nothing is PATCHed.
  if (run('model needs agent')) {
    const {context, page, errors} = await open()
    await page.getByLabel('Default agent', {exact:true}).selectOption('claude')
    await page.getByRole('button', {name:/^Model:/}).click()
    await page.getByRole('listbox', {name:'Model'}).getByRole('option', {name:'opus', exact:true}).click()
    await page.getByLabel('Default agent', {exact:true}).selectOption('')
    await page.getByRole('button', {name:'Save defaults'}).click()
    await expect(page.getByRole('alert')).toHaveText('Choose an agent before setting a model.')
    expect(patches).toEqual([])
    expect(errors).toEqual([])
    await context.close()
  }

  // Favourites: star/unstar from a picker, add through Other, move, one default per agent, clear, and a stale order.
  if (run('favourites')) {
    const {context, page, errors, unmocked} = await open()
    await page.getByLabel('Default agent', {exact:true}).selectOption('claude')
    await page.getByRole('button', {name:/^Model:/}).click()
    await page.getByRole('listbox', {name:'Model'}).getByRole('button', {name:'Star haiku for Claude Code'}).click()
    await expect(page.getByRole('listbox', {name:'Model'}).getByRole('button', {name:'Unstar haiku for Claude Code'})).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByText('haiku', {exact:true})).toBeVisible()
    expect(addCalls).toEqual([{tool:'claude', model:'haiku'}])

    await page.getByRole('button', {name:/^Model:/}).click()
    await page.getByRole('listbox', {name:'Model'}).getByRole('button', {name:'Unstar haiku for Claude Code'}).click()
    await page.keyboard.press('Escape')
    await expect(page.getByText('haiku', {exact:true})).toHaveCount(0)
    expect(deleteCalls).toEqual([{tool:'claude', model:'haiku'}])

    await page.getByLabel('Agent for the new favourite').selectOption('opencode')
    await page.getByRole('button', {name:'Add a model for OpenCode'}).click()
    await page.getByRole('listbox', {name:'Add a favourite model'}).getByRole('option', {name:'Other…'}).click()
    await page.getByLabel('Model name').fill('vendor/new-model')
    await page.getByRole('button', {name:'Use', exact:true}).click()
    await expect(page.getByText('vendor/new-model', {exact:true})).toBeVisible()
    expect(addCalls.at(-1)).toEqual({tool:'opencode', model:'vendor/new-model'})

    await page.getByRole('button', {name:'Move vendor/new-model up for OpenCode'}).click()
    await expect.poll(() => orderCalls.length).toBe(1)
    expect(orderCalls[0].items.filter(item => item.tool === 'opencode').map(item => item.model)).toEqual(['vendor/new-model', 'anthropic/claude-sonnet-4-5'])

    const newDefault = page.getByRole('checkbox', {name:'Use vendor/new-model as the default when I pick OpenCode'})
    await newDefault.check()
    await expect(newDefault).toBeChecked()
    expect(defaultCalls[0]).toEqual({tool:'opencode', model:'vendor/new-model'})

    const otherDefault = page.getByRole('checkbox', {name:'Use anthropic/claude-sonnet-4-5 as the default when I pick OpenCode'})
    let releaseDefault
    defaultGate = new Promise(resolve => { releaseDefault = resolve })
    const checking = otherDefault.check()
    // Before the server answers, the local rule must have cleared the other default for the same agent.
    await expect(newDefault).not.toBeChecked()
    releaseDefault()
    defaultGate = null
    await checking
    await expect(otherDefault).toBeChecked()
    expect(defaultCalls[1]).toEqual({tool:'opencode', model:'anthropic/claude-sonnet-4-5'})

    await otherDefault.uncheck()
    await expect(otherDefault).not.toBeChecked()
    expect(defaultCalls[2]).toEqual({tool:'opencode', model:''})

    staleOrder = true
    await page.getByRole('button', {name:'Move vendor/new-model down for OpenCode'}).click()
    await expect(page.getByRole('alert')).toHaveText('Your favourites changed. Reload them, then try again.')
    await expect(page.getByRole('heading', {name:'Agents', level:2})).toBeVisible()
    await expect(page).toHaveURL(`${base}/account/agents`)
    await expect(page.getByText('vendor/new-model', {exact:true})).toBeVisible()
    await page.getByRole('button', {name:'Move vendor/new-model down for OpenCode'}).click()
    await expect.poll(() => orderCalls.length).toBe(2)
    expect(orderCalls[1].items.filter(item => item.tool === 'opencode').map(item => item.model)).toEqual(['anthropic/claude-sonnet-4-5', 'vendor/new-model'])
    expect(errors).toEqual([])
    expect(unmocked).toEqual([])
    await context.close()
  }

  // A viewer reads the defaults and favourites, but every mutation is disabled.
  if (run('viewer read only')) {
    const {context, page, errors} = await open({user:viewer})
    await expect(page.getByText('Viewer accounts can read these defaults but can\'t change them.')).toBeVisible()
    await expect(page.getByRole('button', {name:'Save defaults'})).toBeDisabled()
    await expect(page.getByLabel('Default agent', {exact:true})).toBeDisabled()
    await expect(page.getByRole('button', {name:'Add a model for Claude Code'})).toBeDisabled()
    await expect(page.getByRole('checkbox', {name:'Use sonnet as the default when I pick Claude Code'})).toBeDisabled()
    await expect(page.getByRole('button', {name:'Remove sonnet for Claude Code'})).toBeDisabled()
    await expect(page.getByRole('button', {name:'Move sonnet down for Claude Code'})).toBeDisabled()
    expect(patches).toEqual([])
    expect(addCalls).toEqual([])
    expect(deleteCalls).toEqual([])
    expect(errors).toEqual([])
    await context.close()
  }

  // On a phone the section keeps the page width, its fields are 16 px and its targets 44 px; the picker is the sheet.
  if (run('phone fit')) {
    const {context, page, errors, unmocked} = await open({viewport:{width:390, height:844}, touch:true})
    const fit = await page.evaluate(() => {
      const fields = [...document.querySelectorAll('main input:not([type=checkbox]), main select, main textarea')]
        .filter(el => { const r = el.getBoundingClientRect(); return r.width > 2 && r.height > 2 })
        .map(el => ({label:el.getAttribute('aria-label') || el.id, size:parseFloat(getComputedStyle(el).fontSize)}))
      const targets = [...document.querySelectorAll('main select, main input:not([type=checkbox]), main button')]
        .filter(el => { const r = el.getBoundingClientRect(); return r.width > 2 && r.height > 2 })
        .map(el => ({label:(el.getAttribute('aria-label') || el.textContent || el.id).trim().slice(0, 40), height:el.getBoundingClientRect().height}))
      return {scrollWidth:document.documentElement.scrollWidth, innerWidth, scale:visualViewport?.scale ?? 1, fields, targets}
    })
    expect(fit.scrollWidth).toBeLessThanOrEqual(fit.innerWidth)
    expect(fit.scale).toBe(1)
    for (const field of fit.fields) expect(field.size, `${field.label} field size`).toBeGreaterThanOrEqual(16)
    for (const target of fit.targets) expect(target.height, `${target.label} target height`).toBeGreaterThanOrEqual(44)
    await page.getByLabel('Default agent', {exact:true}).selectOption('claude')
    await page.getByRole('button', {name:/^Model:/}).click()
    const sheet = page.getByRole('dialog', {name:'Model'})
    await expect(sheet).toBeVisible()
    await expect(sheet.getByRole('searchbox', {name:'Filter models'})).toBeVisible()
    expect(await sheet.getByRole('searchbox', {name:'Filter models'}).evaluate(el => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(16)
    await sheet.getByRole('button', {name:'Close'}).click()
    expect(errors).toEqual([])
    expect(unmocked).toEqual([])
    await context.close()
  }

  console.log('Account Agents: defaults, local model rule, favourites, stale order, viewer and phone fit pass.')
} finally {
  await browser.close()
}
