import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { ReetClient, collection } from '../dist/client.js';
import { page, formatResult, CHARACTER_LIMIT } from '../dist/response.js';

async function serve(handler, run) {
  const server = createServer(handler);
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  try { await run(`http://127.0.0.1:${server.address().port}`); }
  finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }
}

test('URL configuration rejects credential/query injection and non-HTTP schemes', () => {
  for (const url of ['bad-url', 'file:///etc/passwd', 'https://user:secret@example.com', 'https://example.com?token=secret', 'https://example.com/#fragment']) {
    assert.throws(() => new ReetClient(url), /REET_URL/);
  }
});

test('HTTP client preserves base paths, encodes query and authenticates with a header', async () => {
  await serve((req, res) => {
    assert.equal(req.url, '/reet/api/skill?id=a%2Fb%26c');
    assert.equal(req.headers.authorization, 'Bearer test-only');
    assert.equal(req.headers['x-reet-source'], 'mcp');
    res.writeHead(204).end();
  }, async url => {
    assert.deepEqual(await new ReetClient(`${url}/reet`, 'test-only').request('/api/skill', 'DELETE', undefined, new URLSearchParams({ id: 'a/b&c' })), { ok: true });
  });
});

test('redirects never forward authentication', async () => {
  let destinationCalls = 0;
  await serve((_req, res) => { destinationCalls++; res.end('{}'); }, async destination => {
    await serve((_req, res) => res.writeHead(302, { Location: destination }).end(), async url => {
      await assert.rejects(new ReetClient(url, 'private-token').request('/api/skills'), /Redirects are rejected/);
    });
  });
  assert.equal(destinationCalls, 0);
});

test('timeouts, malformed responses and oversized upstream data fail clearly', async () => {
  await serve((_req, _res) => {}, async url => {
    await assert.rejects(new ReetClient(url, undefined, 20).request('/api/skills'), /time out/);
  });
  await serve((_req, res) => res.end('<html>sign in</html>'), async url => {
    await assert.rejects(new ReetClient(url).request('/api/skills'), /invalid JSON/);
  });
  await serve((_req, res) => res.end('x'.repeat(8 * 1024 * 1024 + 1)), async url => {
    await assert.rejects(new ReetClient(url).request('/api/skills'), /8 MiB/);
  });
  assert.deepEqual(collection({ items: null }, 'items'), []);
  assert.throws(() => collection({}, 'items'), /missing items/);
});

test('bounded pages advance without losing records and both formats retain structured content', () => {
  const items = Array.from({ length: 10 }, (_, id) => ({ id, notes: 'x'.repeat(5000) }));
  const seen = [];
  let offset = 0;
  do {
    const data = page(items, { limit: 10, offset });
    assert.ok(data.count > 0);
    assert.equal(data.total_count, 10);
    for (const format of ['json', 'markdown']) {
      const result = formatResult(data, format);
      assert.ok(result.content[0].text.length <= CHARACTER_LIMIT);
      assert.deepEqual(result.structuredContent.data, data);
    }
    seen.push(...data.items.map(item => item.id));
    offset = data.next_offset;
  } while (offset !== null);
  assert.deepEqual(seen, items.map(item => item.id));
  assert.throws(() => page([{ notes: 'x'.repeat(30_000) }], { limit: 1, offset: 0 }), /One record/);
});
