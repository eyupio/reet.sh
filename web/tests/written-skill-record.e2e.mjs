// A written skill appears once on the Skills page: drafts in the writing block,
// published skills as shelf cards with an edit link. The authoring page names
// the machines for every skill.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'

const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] })
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const now = Date.now()
const host = new URL(base).host
const id = slug => `hosted/${host}/sam/${slug}`
const asShelf = sk => ({ ...skill(sk.slug), id: id(sk.slug), source: `${host}/sam`, sourceType: 'hosted', slug: sk.slug, name: sk.slug, url: `${base}/sam/${sk.slug}`, description: sk.description, createdAt: now, updatedAt: now })
const asHosted = (slug, published) => ({ slug, description: `${slug} description`, content: `---\nname: ${slug}\ndescription: ${slug} description\n---\nbody`, files: {}, published, createdAt: now, updatedAt: now })
const scope = { id: 'dev-laptop-agents', host: 'dev-laptop', agent: 'agents', name: 'dev-laptop', skills: [], skillCount: 0, complete: true, scannedAt: now, lastSeen: now, os: 'linux', arch: 'amd64', version: 'test' }
const sync = {
  environments: [scope],
  machines: [{ host: 'dev-laptop', groups: [scope], skillCount: 0 }],
  agents: [{ id: 'agent-1', host: 'dev-laptop', version: 'test', lastSeen: now, paused: false, autoUpdate: false }],
  targets: [{ target: scope, sourceId: 'shelf', sourceName: 'Shelf', mode: 'additive', desired: [{ id: id('reet-published'), name: 'reet-published', slug: 'reet-published', source: `${host}/sam`, sourceType: 'hosted' }], missing: [], extra: [], revision: 'r1', updatedAt: now }],
  jobs: [], runs: [], updateJobs: [],
}

try {
  if (run('one card and Machines')) {
    let hosted = [asHosted('reet-published', true), asHosted('reet-draft', false)]
    let shelf = [asShelf(hosted[0])]
    const saveHosted = request => {
      const body = request.postDataJSON()
      hosted = hosted.map(item => item.slug === body.slug ? body : item)
      shelf = shelf.filter(item => item.id !== id(body.slug))
      if (body.published) shelf = [...shelf, asShelf(body)]
      return body
    }
    const { context, page, errors, state } = await openWorkspace(browser, {
      width: 1280, sync, hosting: { username: 'sam', prefix: 'reet-', skills: hosted },
      routes: {
        '/api/skills': () => ({ skills: shelf, counts: { all: shelf.length, used: 0, liked: 0, bookmarked: 0 } }),
        '/api/environments': { environments: [], machines: [] },
        '/api/ignored': { ignored: [] },
        '/api/v2/hosting': () => ({ username: 'sam', prefix: 'reet-', skills: hosted }),
        '/api/v2/hosting/skills/reet-draft': saveHosted,
        '/api/v2/hosting/skills/reet-published': saveHosted,
      },
    })

    await page.goto(`${base}/skills`)
    await expect(page.getByRole('heading', { name: /Your drafts/ })).toBeVisible()
    await expect(page.locator('.hosted-list')).toContainText('reet-draft')
    await expect(page.locator('.hosted-list')).not.toContainText('reet-published')
    await expect(page.locator(`li.skill[data-skill="${id('reet-published')}"]`)).toHaveCount(1)

    const card = page.locator(`li.skill[data-skill="${id('reet-published')}"]`)
    await card.getByRole('button', { name: 'More actions for reet-published' }).click()
    await expect(page.getByRole('menuitem', { name: 'Edit written skill' })).toHaveAttribute('href', '/hosting?edit=reet-published')
    await page.keyboard.press('Escape')

    await page.getByRole('button', { name: 'Publish & share' }).click()
    await expect(page.locator('.hosted-list')).not.toContainText('reet-draft')
    await expect(page.locator(`li.skill[data-skill="${id('reet-draft')}"]`)).toHaveCount(1)

    await page.goto(`${base}/hosting`)
    await expect(page.getByText('Machines: Set to sync to dev-laptop', { exact: true })).toBeVisible()
    await expect(page.getByText('Machines: Not on a machine yet', { exact: true })).toBeVisible()

    await page.locator('.skill-choice').filter({ hasText: 'reet-published' }).click()
    await page.locator('.publish input[type=checkbox]').uncheck()
    await page.locator('.save-dock').getByRole('button', { name: 'Save draft', exact: true }).click()
    await page.goto(`${base}/skills`)
    await expect(page.locator('.hosted-list')).toContainText('reet-published')
    await expect(page.locator(`li.skill[data-skill="${id('reet-published')}"]`)).toHaveCount(0)

    expect(errors).toEqual([])
    expect(state.unexpected).toEqual([])
    await context.close()
  }
} finally {
  await browser.close()
}
console.log('Written skill record: drafts once, published on the shelf with an edit link, and Machines on the authoring page.')
