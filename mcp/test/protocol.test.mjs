import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { connect, fixture, call } from './helpers.mjs';
import { Client, StreamableHTTPClientTransport } from '@modelcontextprotocol/client';

let api, client;
before(async () => { api = await fixture(); client = await connect(api.url); });
after(async () => { await client?.close(); await api?.close(); });

test('stdio discovery declares schemas, strict inputs and correct mutation hints', async () => {
  const { tools } = await client.listTools();
  assert.equal(tools.length, 52);
  for (const tool of tools) {
    assert.ok(tool.title && tool.description && tool.outputSchema);
    assert.equal(tool.inputSchema.additionalProperties, false);
    assert.equal(typeof tool.annotations.readOnlyHint, 'boolean');
  }
  assert.equal(tools.find(t => t.name === 'reet_remove_skill').annotations.destructiveHint, true);
  assert.equal(tools.find(t => t.name === 'reet_list_skills').annotations.readOnlyHint, true);
  // Adding a skill or changing its stage changes what is on every machine, and the tools say so.
  for (const name of ['reet_add_skill', 'reet_update_skill', 'reet_bulk_update_skills']) {
    assert.match(tools.find(t => t.name === name).description, /every machine that follows the library/, name);
  }
  assert.doesNotMatch(tools.find(t => t.name === 'reet_add_skill').description, /run installs/);
});

test('skill tools speak Active, Archived and Delete, and say what happens on machines', async () => {
  const { tools } = await client.listTools();
  const byName = Object.fromEntries(tools.map(t => [t.name, t]));
  // The ignore list and saved sync plans are gone; Archived skills replace them.
  for (const gone of ['reet_list_ignored', 'reet_unignore_skill', 'reet_list_sync_targets']) assert.equal(byName[gone], undefined, gone);
  assert.equal(byName.reet_list_archived.annotations.readOnlyHint, true);
  assert.equal(byName.reet_restore_skill.annotations.readOnlyHint, false);
  const props = name => byName[name].inputSchema.properties;
  assert.deepEqual(props('reet_list_skills').status.enum, ['active', 'archived']);
  assert.equal(props('reet_list_skills').stage, undefined);
  assert.deepEqual(props('reet_update_skill').status.enum, ['active', 'archived']);
  for (const old of ['used', 'bookmarked']) assert.equal(props('reet_update_skill')[old], undefined, old);
  assert.deepEqual(props('reet_bulk_update_skills').status.enum, ['active', 'archived']);
  assert.equal(props('reet_bulk_update_skills').stage, undefined);
  assert.equal(props('reet_remove_skill').ignore, undefined);
  for (const flag of ['noCopyConfirmed', 'writtenConfirmed']) assert.equal(props('reet_remove_skill')[flag].type, 'boolean', flag);
  // Each says in plain words what happens on the machines, and that they keep a backup.
  for (const name of ['reet_update_skill', 'reet_bulk_update_skills']) {
    assert.match(byName[name].description, /archived.*taken off every machine/s, name);
  }
  assert.match(byName.reet_remove_skill.description, /removed from every machine/);
  for (const name of ['reet_update_skill', 'reet_bulk_update_skills', 'reet_remove_skill', 'reet_list_archived', 'reet_restore_skill']) {
    assert.match(byName[name].description, /backup for 30 days/, name);
  }
  assert.match(byName.reet_restore_skill.description, /every machine/);
  // list_archived says which name it shows and how its rows join reet_list_skills.
  assert.match(byName.reet_list_archived.description, /name is the skill’s folder name/);
  assert.match(byName.reet_list_archived.description, /shelfId is its ID in reet_list_skills/);
  assert.doesNotMatch(byName.reet_merge_machines.description, /sync plans/);
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
  // Copy Desk is Archived and every other skill is Active, so each status
  // keeps a different set: the filter really tells them apart.
  const active = await call(client, 'list_skills', { status: 'active', sort: 'name' });
  assert.deepEqual(active.items.map(s => s.name), ['Audit Trail', 'Deploy Guide', 'Layout Lab', 'Metric Watch', 'Test Pilot']);
  assert.ok(active.items.every(s => s.status === 'active'));
  const archived = await call(client, 'list_skills', { status: 'archived' });
  assert.deepEqual(archived.items.map(s => [s.name, s.status]), [['Copy Desk', 'archived']]);
  const filtered = await call(client, 'list_skills', { tags: [backend.id], status: 'active' });
  assert.deepEqual(filtered.items.map(s => s.name).sort(), ['Audit Trail', 'Deploy Guide', 'Metric Watch', 'Test Pilot']);
  const archivedBackend = await call(client, 'list_skills', { tags: [backend.id], status: 'archived' });
  assert.equal(archivedBackend.total_count, 0);
  assert.deepEqual(archivedBackend.items, []);
  const listed = await call(client, 'list_archived');
  // The library names a skill by its folder name.
  assert.deepEqual(listed.items.map(s => [s.name, s.shelfId, s.status]), [['copy', 'demo/skills/copy', 'archived']]);
  assert.match(listed.items[0].id, /^[0-9a-f]{32}$/);
  assert.deepEqual(listed.counts, { active: 5, archived: 1 });
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
});

test('shelf mutation, tags, event history, archive, restore and delete round trip', async () => {
  const added = await call(client, 'add_skill', { input: 'demo/skills/new-skill', notes: 'keep this', liked: true });
  assert.equal(added.created, true);
  assert.equal(added.status, 'active');
  const archived = await call(client, 'update_skill', { id: added.id, status: 'archived' });
  assert.equal(archived.notes, 'keep this');
  assert.equal(archived.liked, true);
  assert.equal(archived.status, 'archived');
  const listed = (await call(client, 'list_archived', { limit: 100 })).items.find(s => s.shelfId === added.id);
  assert.equal(listed.name, 'new-skill');
  // Restoring takes the shelf ID or the library ID reet_list_archived gives.
  const restored = await call(client, 'restore_skill', { id: listed.id });
  assert.deepEqual([restored.id, restored.shelfId, restored.name, restored.status], [listed.id, added.id, 'new-skill', 'active']);
  assert.equal((await call(client, 'get_skill', { id: added.id })).status, 'active');
  assert.ok(!(await call(client, 'list_archived', { limit: 100 })).items.some(s => s.shelfId === added.id));
  const noted = await call(client, 'update_skill', { id: added.id, notes: 'still here', status: 'archived' });
  assert.deepEqual([noted.notes, noted.status], ['still here', 'archived']);
  await call(client, 'restore_skill', { id: added.id });
  const tagged = await call(client, 'tag_skill', { id: added.id, add: ['MCP test'] });
  assert.ok(tagged.tags.some(t => t.name === 'MCP test'));
  const events = await call(client, 'list_events', { skill: added.id, limit: 2 });
  assert.equal(events.events.length, 2);
  assert.ok(events.events.every(event => event.skillId === added.id));
  // reet has no copy of it, so deleting removes it from every machine: it needs confirming.
  await assert.rejects(call(client, 'remove_skill', { id: added.id }), { message: 'reet.sh returned HTTP 409 [no-copy]: reet has no copy of new-skill. Deleting removes it from every machine, which keeps a backup for 30 days. Delete it again with noCopyConfirmed to delete it from every machine.' });
  assert.equal((await call(client, 'get_skill', { id: added.id })).status, 'active');
  assert.deepEqual(await call(client, 'remove_skill', { id: added.id, noCopyConfirmed: true }), { deleted: 1 });
  assert.ok(!(await call(client, 'list_skills', { status: 'active', limit: 100 })).items.some(s => s.id === added.id));
  assert.ok(!(await call(client, 'list_archived', { limit: 100 })).items.some(s => s.shelfId === added.id));
  await assert.rejects(call(client, 'restore_skill', { id: 'demo/skills/never-added' }), /not found/);
  await assert.rejects(call(client, 'get_skill', { id: 'demo/skills/never-added' }), { message: 'Skill not found in this account’s library. Use reet_list_skills or reet_list_archived to find its ID.' });
});

test('a skill written in reet and never published is listed by its library id, and archived and deleted by it', async () => {
  const put = await fetch(`${api.url}/api/v2/hosting/skills/reet-field-notes`, {
    method: 'PUT', headers: { Authorization: 'Bearer fixture-token', 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: '---\nname: reet-field-notes\ndescription: Written in reet\n---\nBody.\n', files: {} }),
  });
  assert.equal(put.status, 200, await put.text());
  const listed = (await call(client, 'list_skills', { status: 'active', limit: 100 })).items.find(s => s.name === 'reet-field-notes');
  assert.match(listed.id, /^[0-9a-f]{32}$/);
  assert.equal(listed.status, 'active');
  assert.equal((await call(client, 'list_skills', { q: 'field-notes', compact: true })).items[0].id, listed.id);
  assert.equal((await call(client, 'get_skill', { id: listed.id })).name, 'reet-field-notes');
  assert.equal((await call(client, 'update_skill', { id: listed.id, status: 'archived' })).status, 'archived');
  assert.ok((await call(client, 'list_skills', { status: 'archived' })).items.some(s => s.id === listed.id));
  await assert.rejects(call(client, 'remove_skill', { id: listed.id }), /HTTP 409 \[written\]: You wrote reet-field-notes\. Deleting it deletes your writing too\. Delete it again with writtenConfirmed/);
  assert.deepEqual(await call(client, 'remove_skill', { id: listed.id, writtenConfirmed: true }), { deleted: 1 });
  assert.ok(!(await call(client, 'list_skills', { limit: 100 })).items.some(s => s.id === listed.id));
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

test('lists accept shelf IDs and machine cleanup is a dry run until confirmed', async () => {
  const created = await call(client, 'create_list', { name: 'By shelf ID' });
  const added = await call(client, 'add_list_skill', { list_id: created.id, id: 'demo/skills/tests' });
  assert.equal(added.skillCount, 1);
  await assert.rejects(call(client, 'add_list_skill', { list_id: created.id }), /exactly one/);
  await assert.rejects(call(client, 'add_list_skill', { list_id: created.id, id: 'demo/skills/tests', input: 'demo/skills/tests' }), /exactly one/);
  const preview = await call(client, 'forget_machine', { host: 'laptop' });
  assert.equal(preview.dry_run, true);
  assert.equal(preview.applied, false);
  assert.equal(preview.skillCount, 3);
  await assert.rejects(call(client, 'forget_machine', { host: 'laptop', confirm: true }), /still lists 3 skills/);
  await assert.rejects(call(client, 'forget_machine', { host: 'no-such-host' }), /No environments/);
  await assert.rejects(call(client, 'merge_machines', { from: 'laptop', to: 'laptop' }), /different/);
  assert.equal((await call(client, 'list_environments', { host: 'laptop' })).items.length, 1);
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
    assert.equal((await remote.listTools()).tools.length, 52);
    assert.equal((await bridge.listTools()).tools.length, 20);
    assert.deepEqual(await call(remote, 'list_context_access', {}), { scopes: [] });
    assert.deepEqual(await call(bridge, 'list_context_access', {}), { scopes: [] });
    await assert.rejects(call(remote, 'get_context', {}), /not allowed|forbidden|access/i);
    await assert.rejects(call(bridge, 'get_context', {}), /not allowed|forbidden|access/i);
    const hosted = await call(remote, 'list_skills', { limit: 2, sort: 'name' });
    const forwarded = await call(bridge, 'list_skills', { limit: 2, sort: 'name' });
    assert.deepEqual(forwarded, hosted);
    assert.equal(hosted.total_count, 6);
    const added = await call(remote, 'add_skill', { input: 'demo/skills/http-test', notes: 'HTTP round trip' });
    assert.equal(added.created, true);
    await assert.rejects(call(remote, 'remove_skill', { id: added.id }), { message: 'reet.sh returned HTTP 409 [no-copy]: reet has no copy of http-test. Deleting removes it from every machine, which keeps a backup for 30 days. Delete it again with noCopyConfirmed to delete it from every machine.' });
    // The hosted server and the stdio bridge archive, list and restore alike.
    assert.equal((await call(remote, 'update_skill', { id: added.id, status: 'archived' })).status, 'archived');
    const viaBridge = await call(bridge, 'list_archived');
    assert.deepEqual(viaBridge, await call(remote, 'list_archived'));
    assert.ok(viaBridge.items.some(s => s.shelfId === added.id));
    assert.deepEqual(await call(remote, 'list_skills', { status: 'archived' }), await call(bridge, 'list_skills', { status: 'archived' }));
    assert.equal((await call(remote, 'restore_skill', { id: added.id })).status, 'active');
    assert.deepEqual(await call(remote, 'remove_skill', { id: added.id, noCopyConfirmed: true }), { deleted: 1 });
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
    'environment_count', 'id', 'liked', 'name', 'source', 'sourceType', 'status', 'tags',
  ]);
  const picked = await call(client, 'list_skills', { fields: ['id', 'name', 'status'], status: 'archived' });
  assert.deepEqual(picked.items, [{ id: 'demo/skills/copy', name: 'Copy Desk', status: 'archived' }]);

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
    ids: ['demo/skills/tests', 'demo/skills/deploy'], add_tags: ['MCP bulk'], status: 'archived',
  });
  assert.equal(bulk.updated, 2);
  assert.equal(bulk.failed, 0);
  assert.deepEqual((await call(client, 'list_skills', { status: 'archived', sort: 'name' })).items.map(s => s.name), ['Copy Desk', 'Deploy Guide', 'Test Pilot']);
  const bulkTag = (await call(client, 'list_tags', { limit: 100 })).items.find(t => t.name === 'MCP bulk');
  const back = await call(client, 'bulk_update_skills', { filter: { status: 'archived', tags: [bulkTag.id] }, status: 'active' });
  assert.deepEqual([back.updated, back.failed], [2, 0]);
  assert.deepEqual((await call(client, 'list_skills', { status: 'archived' })).items.map(s => s.name), ['Copy Desk']);
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
