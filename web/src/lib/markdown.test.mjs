import { test } from 'node:test'
import assert from 'node:assert/strict'
import { markdownBlocks } from './markdown.ts'

const t = text => ({ kind: 'text', text })
const inline = text => { const [block] = markdownBlocks(text); return block.spans }

test('blocks: headings of every level, paragraphs, rules, quotes and fenced code', () => {
  assert.deepEqual(markdownBlocks('## Fix\n\nplain\nnext line\n\n---\n\n##### Small\n\n> note\n\n```go extra\nfmt.Println()\n```'), [
    { kind: 'h', level: 2, spans: [t('Fix')] },
    { kind: 'p', spans: [t('plain'), { kind: 'br' }, t('next line')] },
    { kind: 'rule' },
    { kind: 'h', level: 5, spans: [t('Small')] },
    { kind: 'quote', blocks: [{ kind: 'p', spans: [t('note')] }] },
    { kind: 'code', lang: 'go', text: 'fmt.Println()' },
  ])
  // A fence still open while the answer streams in is already code.
  assert.deepEqual(markdownBlocks('```sh\nnpm test'), [{ kind: 'code', lang: 'sh', text: 'npm test' }])
})

test('lists nest, number from where they start, and carry task boxes', () => {
  assert.deepEqual(markdownBlocks('3. first\n   - [x] done\n   - [ ] open\n4. second'), [
    { kind: 'list', ordered: true, start: 3, items: [
      { checked: null, blocks: [
        { kind: 'text', spans: [t('first')] },
        { kind: 'list', ordered: false, start: 1, items: [
          { checked: true, blocks: [{ kind: 'text', spans: [t('done')] }] },
          { checked: false, blocks: [{ kind: 'text', spans: [t('open')] }] },
        ] },
      ] },
      { checked: null, blocks: [{ kind: 'text', spans: [t('second')] }] },
    ] },
  ])
})

test('pipe tables become tables, with or without their header', () => {
  assert.deepEqual(markdownBlocks('Plan:\n\n| Change | Where |\n|:--|--:|\n| Reuse `ScrollRail` | **App** |\n\n### Leave alone'), [
    { kind: 'p', spans: [t('Plan:')] },
    { kind: 'table', head: [[t('Change')], [t('Where')]], align: ['left', 'right'], rows: [[[t('Reuse '), { kind: 'code', text: 'ScrollRail' }], [{ kind: 'strong', spans: [t('App')] }]]] },
    { kind: 'h', level: 3, spans: [t('Leave alone')] },
  ])
  // A long answer can lose its first lines to the history bound, so rows whose header is gone are still a table.
  assert.deepEqual(markdownBlocks('| a | `x | y` | c |\n| d | e |'), [
    { kind: 'table', head: null, align: [null, null, null], rows: [[[t('a')], [{ kind: 'code', text: 'x | y' }], [t('c')]], [[t('d')], [t('e')], []]] },
  ])
  // One line with pipes around it is not enough to call it a table.
  assert.deepEqual(markdownBlocks('| just a line |'), [{ kind: 'p', spans: [t('| just a line |')] }])
})

test('inline: code, bold, italic, strikethrough and entities', () => {
  assert.deepEqual(inline('run `go test` and **keep** *this* ~~not that~~ &amp; go'), [
    t('run '), { kind: 'code', text: 'go test' }, t(' and '), { kind: 'strong', spans: [t('keep')] }, t(' '),
    { kind: 'em', spans: [t('this')] }, t(' '), { kind: 'del', spans: [t('not that')] }, t(' & go'),
  ])
})

test('links: only http(s) targets qualify, and images are linked rather than loaded', () => {
  assert.deepEqual(inline('see https://example.com/a?b=1. Next'), [
    t('see '), { kind: 'link', href: 'https://example.com/a?b=1', spans: [t('https://example.com/a?b=1')] }, t('. Next'),
  ])
  assert.deepEqual(inline('[the PR](https://github.com/o/r/pull/1) is up'), [
    { kind: 'link', href: 'https://github.com/o/r/pull/1', spans: [t('the PR')] }, t(' is up'),
  ])
  assert.deepEqual(inline('[article](https://en.wikipedia.org/wiki/Function_(mathematics)) ends'), [
    { kind: 'link', href: 'https://en.wikipedia.org/wiki/Function_(mathematics)', spans: [t('article')] }, t(' ends'),
  ])
  assert.deepEqual(inline('`https://example.com` and javascript:alert(1)'), [{ kind: 'code', text: 'https://example.com' }, t(' and javascript:alert(1)')])
  assert.deepEqual(inline('[click](javascript:alert(1)) [mail](mailto:a@b.co) [data](data:text/html,x)'), [t('click mail data')])
  assert.deepEqual(inline('![shot](https://example.com/a.png) ![x](file:///etc/passwd)'), [
    { kind: 'link', href: 'https://example.com/a.png', spans: [t('shot')] }, t(' x'),
  ])
})

test('HTML in a message is shown as text, never as markup', () => {
  assert.deepEqual(inline('a <b onclick="x()">bold</b> tag'), [t('a <b onclick="x()">bold</b> tag')])
  assert.deepEqual(markdownBlocks('<script>alert(1)</script>'), [{ kind: 'p', spans: [t('<script>alert(1)</script>')] }])
})
