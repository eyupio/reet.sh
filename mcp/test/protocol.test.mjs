import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { connect, fixture, call } from './helpers.mjs';
import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client';

let api, client;
before(async () => { api = await fixture(); client = await connect(api.url); });
after(async () => { await client?.close(); await api?.close(); });

test('stdio discovery declares schemas, strict inputs and correct mutation hints', async () => {
  const { tools } = await client.listTools();
  assert.equal(tools.length, 28);
  for (const tool of tools) {
    assert.ok(tool.title && tool.description && tool.outputSchema);
    assert.equal(tool.inputSchema.additionalProperties, false);
    assert.equal(typeof tool.annotations.readOnlyHint, 'boolean');
  }
  assert.equal(tools.find(t => t.name === 'reet_remove_skill').annotations.destructiveHint, true);
  assert.equal(tools.find(t => t.name === 'reet_list_skills').annotations.readOnlyHint, true);
});

test('real API filters, tags, IDs and pagination preserve all matching results', async () => {
  const first = await call(client, 'list_skills', { limit: 2, sort: 'name' });
  const second = await call(client, 'list_skills', { limit: 2, offset: first.next_offset, sort: 'name' });
  const third = await call(client, 'list_skills', { limit: 2, offset: second.next_offset, sort: 'name' });
  assert.equal(first.total_count, 6);
  assert.equal(new Set([...first.items, ...second.items, ...third.items].map(s => s.id)).size, 6);
  assert.equal(third.has_more, false);
  const tags = await call(client, 'list_tags');
  const backend = tags.items.find(t => t.name === 'Backend');
  const filtered = await call(client, 'list_skills', { tags: [backend.id], stage: 'try' });
  assert.deepEqual(filtered.items.map(s => s.name), ['Deploy Guide']);
  assert.equal((await call(client, 'get_skill', { id: 'demo/skills/tests' })).name, 'Test Pilot');
  assert.equal((await call(client, 'list_skills', { q: 'does-not-exist' })).total_count, 0);
  assert.equal((await call(client, 'list_skills', { offset: 100 })).next_offset, null);
});

test('environment summaries retain scan quality and inventory pages', async () => {
  const envs = await call(client, 'list_environments', { host: 'laptop' });
  assert.equal(envs.items.length, 1);
  assert.equal(envs.items[0].complete, true);
  assert.equal(envs.items[0].skillCount, 3);
  assert.equal(envs.items[0].skills, undefined);
  const detail = await call(client, 'get_environment', { id: envs.items[0].id, limit: 1 });
  assert.equal(detail.items.length, 1);
  assert.equal(detail.total_count, 3);
  const plans = await call(client, 'list_sync_targets');
  assert.equal(plans.items.length, 1);
  assert.deepEqual(plans.items[0].missing.map(s => s.name).sort(), ['Audit Trail', 'Metric Watch']);
});

test('shelf mutation, tags, event history, ignore and unignore round trip', async () => {
  const added = await call(client, 'add_skill', { input: 'demo/skills/new-skill', notes: 'keep this', liked: true });
  assert.equal(added.created, true);
  const updated = await call(client, 'update_skill', { id: added.id, bookmarked: true });
  assert.equal(updated.notes, 'keep this');
  assert.equal(updated.liked, true);
  assert.equal(updated.bookmarked, true);
  const tagged = await call(client, 'tag_skill', { id: added.id, add: ['MCP test'] });
  assert.ok(tagged.tags.some(t => t.name === 'MCP test'));
  const events = await call(client, 'list_events', { skill: added.id, limit: 2 });
  assert.equal(events.events.length, 2);
  assert.ok(events.events.every(event => event.skillId === added.id));
  await call(client, 'remove_skill', { id: added.id, ignore: true });
  assert.ok((await call(client, 'list_ignored')).items.some(s => s.id === added.id));
  await call(client, 'unignore_skill', { id: added.id });
  assert.ok(!(await call(client, 'list_ignored')).items.some(s => s.id === added.id));
});

test('personal list creation and list notes use the real API contracts', async () => {
  const created = await call(client, 'create_list', { name: 'MCP round trip' });
  assert.equal(created.visibility, 'personal');
  await call(client, 'add_list_skill', { list_id: created.id, input: 'demo/skills/tests', note: 'Shared note' });
  const detail = await call(client, 'get_list', { list_id: created.id });
  assert.equal(detail.list.canEdit, true);
  assert.equal(detail.items[0].note, 'Shared note');
  assert.notEqual((await call(client, 'get_skill', { id: 'demo/skills/tests' })).notes, 'Shared note');
});

test('invalid arguments are rejected before HTTP and errors do not leak credentials', async () => {
  await assert.rejects(call(client, 'list_skills', { limit: 0 }));
  await assert.rejects(call(client, 'list_skills', { surprise: true }));
  await assert.rejects(call(client, 'update_skill', { id: 'demo/skills/tests' }), /at least one/);
  await assert.rejects(call(client, 'get_skill', { id: 'missing' }), /not found/);
  await assert.rejects(call(client, 'get_list', { list_id: 99999 }), /404/);
  const unauthorized = await connect(api.url, 'do-not-expose-this-token');
  try {
    const result = await unauthorized.callTool({ name: 'reet_list_skills', arguments: {} });
    assert.equal(result.isError, true);
    assert.match(result.content[0].text, /401/);
    assert.ok(!JSON.stringify(result).includes('do-not-expose-this-token'));
  } finally { await unauthorized.close(); }
});

test('personal API tokens retain account isolation and viewer write restrictions', async () => {
  const accounts = await fixture('accounts');
  const owner = await connect(accounts.url);
  const viewer = await connect(accounts.url, 'viewer-token');
  try {
    const ownerShelf = await call(owner, 'list_skills');
    const viewerShelf = await call(viewer, 'list_skills');
    assert.equal(ownerShelf.total_count, 6);
    assert.equal(viewerShelf.total_count, 1);
    assert.equal(viewerShelf.items[0].notes, 'Private viewer notes');
    assert.ok(!ownerShelf.items.some(s => s.id === viewerShelf.items[0].id));
    await assert.rejects(call(owner, 'get_skill', { id: viewerShelf.items[0].id }), /not found/);
    await assert.rejects(call(viewer, 'update_skill', { id: viewerShelf.items[0].id, liked: true }), /403/);
    assert.equal((await call(viewer, 'get_skill', { id: viewerShelf.items[0].id })).liked, false);
  } finally {
    await owner.close(); await viewer.close(); await accounts.close();
  }
});

test('hosted HTTP and stdio forwarding preserve user scope across SDK languages', async () => {
  const accounts = await fixture('accounts');
  const remote = new Client({ name: 'reet-http-test', version: '1.0.0' });
  const bridge = await connect(accounts.url, 'reet_mcp_fixture_read');
  try {
    await remote.connect(new StreamableHTTPClientTransport(new URL(`${accounts.url}/mcp`), { requestInit: { headers: { Authorization: 'Bearer reet_mcp_fixture_write' } } }));
    assert.equal((await remote.listTools()).tools.length, 28);
    assert.equal((await bridge.listTools()).tools.length, 11);
    const hosted = await call(remote, 'list_skills', { limit: 2, sort: 'name' });
    const forwarded = await call(bridge, 'list_skills', { limit: 2, sort: 'name' });
    assert.deepEqual(forwarded, hosted);
    assert.equal(hosted.total_count, 6);
    const added = await call(remote, 'add_skill', { input: 'demo/skills/http-test', notes: 'HTTP round trip' });
    assert.equal(added.created, true);
    await call(remote, 'remove_skill', { id: added.id });
    const response = await fetch(`${accounts.url}/api/skills`, { headers: { Authorization: 'Bearer reet_mcp_fixture_write' } });
    assert.equal(response.status, 403);
  } finally {
    await bridge.close(); await remote.close(); await accounts.close();
  }
});

test('tidy tools cover compact reads, batch edits, dry runs and MCP audit events', async () => {
  const compact = await call(client, 'list_skills', { compact: true, limit: 100 });
  assert.equal(compact.count, 6);
  assert.equal(compact.truncated, undefined);
  assert.deepEqual(Object.keys(compact.items[0]).sort(), [
    'bookmarked', 'environment_count', 'id', 'liked', 'name', 'source', 'sourceType', 'stage', 'tags',
  ]);

  const created = await call(client, 'create_list', { name: 'MCP tidy' });
  const batch = await call(client, 'add_list_skills', {
    list_id: created.id,
    inputs: [{ id: 'demo/skills/tests' }, { id: 'demo/skills/deploy', note: 'Tidy note' }],
  });
  assert.equal(batch.added, 2);
  assert.equal(batch.failed, 0);
  const removed = await call(client, 'remove_list_skill', { list_id: created.id, id: 'demo/skills/tests' });
  assert.equal(removed.skillCount, 1);
  const renamed = await call(client, 'update_list', { list_id: created.id, name: 'MCP tidy renamed' });
  assert.equal(renamed.name, 'MCP tidy renamed');
  const deletePreview = await call(client, 'delete_list', { list_id: created.id });
  assert.equal(deletePreview.dry_run, true);
  await call(client, 'delete_list', { list_id: created.id, dry_run: false, confirm: true });

  const tag = await call(client, 'create_tag', { name: 'MCP tidy tag' });
  const bulk = await call(client, 'bulk_update_skills', {
    ids: ['demo/skills/tests', 'demo/skills/deploy'], add_tags: ['MCP bulk'], stage: 'try',
  });
  assert.equal(bulk.updated, 2);
  assert.equal(bulk.failed, 0);
  const renameTag = await call(client, 'rename_tag', { tag_id: tag.id, name: 'MCP tidy renamed tag' });
  assert.equal(renameTag.name, 'MCP tidy renamed tag');
  const deleteTagPreview = await call(client, 'delete_tag', { tag_id: tag.id });
  assert.equal(deleteTagPreview.dry_run, true);
  await call(client, 'delete_tag', { tag_id: tag.id, dry_run: false, confirm: true });

  const duplicates = await call(client, 'find_duplicates');
  assert.ok(Array.isArray(duplicates.groups));
  const envs = await call(client, 'list_environments', { limit: 100 });
  const envPreview = await call(client, 'delete_environment', { id: envs.items[0].id, force: true });
  assert.equal(envPreview.dry_run, true);
  const mergePreview = await call(client, 'merge_environments', { from_id: envs.items[0].id, to_id: envs.items[1].id });
  assert.equal(mergePreview.dry_run, true);

  const events = await call(client, 'list_events', { source: 'mcp', action: 'bulk-update', limit: 5 });
  assert.ok(events.events.length >= 1);
  assert.ok(events.next_before > 0);
});
