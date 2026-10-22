import { McpServer } from '@modelcontextprotocol/server';
import { z } from 'zod';
import { collection, type JsonObject, ReetClient } from './client.js';
import { formatResult, page } from './response.js';
import { outputs } from './output.js';

const id = z.string().trim().min(1).max(500).describe('Exact ID returned by a reet read tool; e.g. anthropics/skills/mcp-builder.');
const environmentId = id.describe('Exact environment ID returned by reet_list_environments.');
const listId = z.number().int().positive().describe('Numeric list ID returned by reet_list_lists.');
const tagId = z.number().int().positive().describe('Numeric tag ID returned by reet_list_tags.');
const confirm = z.boolean().default(false).describe('Required to apply a destructive change; ignored for a dry run.');
const dryRun = z.boolean().default(true).describe('Preview only. Set false together with confirm=true to apply.');
const pagination = {
  limit: z.number().int().min(1).max(100).default(20).describe('Maximum items per page (1–100).'),
  offset: z.number().int().min(0).default(0).describe('Page offset; follow next_offset until has_more is false.'),
};
const patch = {
  liked: z.boolean().optional().describe('Like or unlike this skill.'),
  bookmarked: z.boolean().optional().describe('Save for testing. Stage is try when bookmarked and not used.'),
  used: z.boolean().optional().describe('Mark in use; takes precedence over bookmarked when deriving stage.'),
  notes: z.string().max(20_000).optional().describe('Personal notes. Replaces existing notes; empty string clears them.'),
};
const compactFields = ['id', 'name', 'source', 'sourceType', 'stage', 'liked', 'bookmarked', 'tags', 'environment_count'] as const;

export interface MCPForwarder {
  allowedTools: Set<string>;
  call(name: string, args: Record<string, unknown>): Promise<JsonObject>;
}

export function createServer(client: ReetClient, forwarder?: MCPForwarder): McpServer {
  const server = new McpServer({ name: 'reet-mcp-server', version: '0.1.0' });

  function tool<S extends z.ZodRawShape>(
    name: string, title: string, description: string, shape: S,
    run: (args: z.infer<z.ZodObject<S>>) => Promise<JsonObject>,
    options: { write?: boolean; destructive?: boolean; idempotent?: boolean } = {},
  ): void {
    if (forwarder && !forwarder.allowedTools.has(`reet_${name}`)) return;
    server.registerTool(`reet_${name}`, {
      title, description: `${description} Returns {data: object} as structured content and text.${'offset' in shape ? ' Pages contain items, count, total_count, has_more and next_offset.' : ''}`,
      inputSchema: z.object({ ...shape, response_format: z.enum(['json', 'markdown']).default('json').describe('JSON or a Markdown JSON code block.') }).strict(),
      outputSchema: z.object({ data: outputs[name] ?? z.looseObject({}) }),
      annotations: {
        readOnlyHint: !options.write, destructiveHint: options.destructive ?? false,
        idempotentHint: options.idempotent ?? !options.write, openWorldHint: true,
      },
    }, async (args) => {
      try {
        const { response_format, ...input } = args as unknown as z.infer<z.ZodObject<S>> & { response_format: 'json' | 'markdown' };
        const data = forwarder ? await forwarder.call(`reet_${name}`, { ...input, response_format }) : await run(input as z.infer<z.ZodObject<S>>);
        try {
          return formatResult(data, response_format);
        } catch (error) {
          if (options.write) throw new Error('The change succeeded, but its response is too large. Read it with a filtered or paginated tool before making another change.');
          throw error;
        }
      } catch (error) {
        return { isError: true, content: [{ type: 'text' as const, text: error instanceof Error ? error.message : 'reet.sh request failed.' }] };
      }
    });
  }

  tool('list_skills', 'Search the shelf', 'Search personal shelf names, IDs, descriptions and notes. Filters combine with AND; #tag syntax is supported by q. compact=true returns only id, name, source, sourceType, stage, liked, bookmarked, tags and environment_count; use it to page a large shelf. Does not change anything. Example: {stage:"try",compact:true,limit:100}.', {
    ...pagination,
    q: z.string().max(500).optional().describe('Search text, e.g. testing #development.'),
    tab: z.enum(['all', 'liked', 'bookmarked', 'used']).optional().describe('Optional flag filter.'),
    stage: z.enum(['shelf', 'try', 'use']).optional().describe('On shelf, to test, or in use.'),
    sort: z.enum(['recent', 'name', 'installs']).default('recent').describe('Order by last update, name, or install popularity.'),
    tags: z.array(z.number().int().positive()).max(50).optional().describe('Require every tag ID; discover IDs using reet_list_tags.'),
    env: environmentId.optional(),
    machine: z.string().max(255).optional().describe('Exact host name, e.g. sam-mbp.'),
    only: z.literal('one-machine').optional().describe('Only skills installed on a single machine.'),
    compact: z.boolean().default(false).describe('Return the compact skill shape; recommended for large shelves.'),
    fields: z.array(z.enum(compactFields)).max(9).optional().describe('Compact fields to include; implies compact=true.'),
  }, async ({ limit, offset, tags, compact, fields, ...filters }) => {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) if (value !== undefined) query.set(key, String(value));
    for (const tag of tags ?? []) query.append('tag', String(tag));
    if (compact) query.set('compact', '1');
    if (fields?.length) query.set('fields', fields.join(','));
    const data = await client.request('/api/skills', 'GET', undefined, query);
    return page(collection(data, 'skills'), { limit, offset }, { counts: data.counts });
  });

  tool('get_skill', 'Get a shelf skill', 'Get one personal shelf skill by exact ID, including notes, flags, tags and environments. Use reet_list_skills to discover IDs. Does not change anything.', { id }, async ({ id }) => {
    const data = await client.request('/api/skills', 'GET', undefined, new URLSearchParams({ q: id }));
    const skill = collection(data, 'skills').find(skill => skill.id === id);
    if (!skill) throw new Error('Skill not found on this account’s shelf. Use reet_list_skills to find its exact ID.');
    return skill;
  });

  tool('add_skill', 'Add a shelf skill', 'Add a skills.sh URL or owner/repo/skill reference. Existing skills keep unspecified flags and notes; ignored entries are unignored. Metadata fetch is queued by reet.sh. Does not add local files or run installs. Returns the skill and created flag.', {
    input: z.string().trim().min(1).max(1000).describe('e.g. anthropics/skills/mcp-builder or https://skills.sh/anthropics/skills/mcp-builder'), ...patch,
  }, async args => client.request('/api/skills', 'POST', args), { write: true, destructive: true });

  tool('update_skill', 'Update shelf flags or notes', 'Change only supplied fields. For stage shelf set used=false, bookmarked=false; try: used=false, bookmarked=true; use: used=true. Does not edit tags or lists. Returns the updated skill.', { id, ...patch }, async args => {
    if (Object.keys(args).length === 1) throw new Error('Supply at least one of liked, bookmarked, used or notes.');
    return client.request('/api/skill', 'PATCH', args);
  }, { write: true, destructive: true, idempotent: true });

  tool('remove_skill', 'Remove a shelf skill', 'Remove this account’s shelf entry, including personal notes and flags. ignore=true also prevents reimport by future scans. Does not delete files from any machine. Returns {ok:true}.', {
    id, ignore: z.boolean().default(false).describe('Also add to this account’s ignored list.'),
  }, async ({ id, ignore }) => client.request('/api/skill', 'DELETE', undefined, new URLSearchParams({ id, ignore: ignore ? '1' : '0' })), { write: true, destructive: true, idempotent: true });

  tool('list_tags', 'List shelf tags', 'List tag IDs, names and shelf counts. Use IDs with reet_list_skills, reet_tag_skill or reet_bulk_update_skills. Does not change anything.', pagination, async args => {
    return page(collection(await client.request('/api/tags'), 'tags'), args);
  });

  tool('tag_skill', 'Edit skill tags', 'Add tag names and/or remove tag IDs from a shelf skill. Removed topic tags stay hidden until explicitly added again. Does not rename or delete the tag itself. Returns the updated skill.', {
    id,
    add: z.array(z.string().trim().min(1).max(100)).max(50).default([]).describe('Tag names to add.'),
    remove: z.array(z.number().int().positive()).max(50).default([]).describe('Tag IDs to remove; discover using reet_list_tags.'),
  }, async args => {
    if (!args.add.length && !args.remove.length) throw new Error('Supply tag names to add or tag IDs to remove.');
    return client.request('/api/skill/tags', 'POST', args);
  }, { write: true, destructive: true, idempotent: true });

  tool('create_tag', 'Create a tag', 'Create an empty tag by name, without attaching it to a skill. Existing names return the existing tag. Does not change any skill. Returns the tag.', {
    name: z.string().trim().min(1).max(100).describe('Tag name.'),
  }, async args => client.request('/api/v2/tags', 'POST', args), { write: true, idempotent: true });

  tool('rename_tag', 'Rename a tag', 'Rename one tag. Reports affected skills. Admins rename it everywhere; members rename it for their own account. Does not merge tags.', {
    tag_id: tagId, name: z.string().trim().min(1).max(100).describe('New tag name.'),
  }, async ({ tag_id, name }) => client.request(`/api/v2/tags/${tag_id}`, 'PATCH', { name }), { write: true, destructive: true, idempotent: true });

  tool('delete_tag', 'Delete a tag', 'Remove a tag from this account’s skills and report how many were affected. Dry run by default; set dry_run=false and confirm=true to apply. Does not delete skills or other accounts’ tags.', {
    tag_id: tagId, dry_run: dryRun, confirm,
  }, async ({ tag_id, ...body }) => client.request(`/api/v2/tags/${tag_id}/delete`, 'POST', body), { write: true, destructive: true });

  tool('list_environments', 'List scanned environments', 'List machine/agent environments with scan completeness, timestamps and skill counts. empty_only and agent narrow the result. Inventories may be stale; complete=false is not proof a missing skill is uninstalled. Does not scan or install anything.', {
    ...pagination,
    host: z.string().max(255).optional().describe('Exact host name filter.'),
    empty_only: z.boolean().default(false).describe('Only environments with zero linked skills.'),
    agent: z.string().max(100).optional().describe('Exact agent filter, e.g. claude or reet-server.'),
  }, async ({ host, empty_only, agent, ...args }) => {
    const query = new URLSearchParams();
    if (empty_only) query.set('empty_only', '1');
    if (agent) query.set('agent', agent);
    const data = await client.request('/api/environments', 'GET', undefined, query);
    const items = collection(data, 'environments').filter(env => !host || env.host === host).map(({ skills, ...env }) => ({ ...env, skillCount: Array.isArray(skills) ? skills.length : 0 }));
    return page(items, args);
  });

  tool('get_environment', 'Read an environment inventory', 'Get scan metadata and a page of installed skills for an exact environment ID. No local filesystem is scanned and nothing is installed.', { id: environmentId, ...pagination }, async ({ id, ...args }) => {
    const data = await client.request('/api/environments');
    const env = collection(data, 'environments').find(item => item.id === id);
    if (!env) throw new Error('Environment not found. Use reet_list_environments to find its ID.');
    const { skills: _skills, ...metadata } = env;
    return page(collection(env, 'skills'), args, { environment: metadata });
  });

  tool('delete_environment', 'Delete environment reports', 'Delete one or more environment reports. Dry run by default; set dry_run=false and confirm=true to apply. Refuses an environment that still has skills unless force=true. Does not remove shelf skills or installed files.', {
    id: environmentId.optional(), ids: z.array(environmentId).max(500).optional().describe('Delete several reports in one call.'),
    force: z.boolean().default(false).describe('Also delete reports that still have linked skills.'),
    dry_run: dryRun, confirm,
  }, async body => {
    if (!body.id && !body.ids?.length) throw new Error('Supply id or ids.');
    return client.request('/api/v2/environments/delete', 'POST', body);
  }, { write: true, destructive: true });

  tool('merge_environments', 'Merge environment reports', 'Move one environment report’s skills into another and delete the source report. Dry run by default; set dry_run=false and confirm=true to apply. Does not merge two enrolled machine identities.', {
    from_id: environmentId, to_id: environmentId, dry_run: dryRun, confirm,
  }, async body => client.request('/api/v2/environments/merge', 'POST', body), { write: true, destructive: true });


  tool('forget_machine', 'Forget a machine', 'Delete a machine’s scanned environments by exact host name (see reet_list_environments). Dry run by default: returns what would be removed and changes nothing. Set confirm=true to apply. Refuses machines that still list skills unless force=true. Never touches shelf skills, lists or tags; the host name stays aliased so scans cannot resurrect duplicates.', {
    host: z.string().trim().min(1).max(255).describe('Exact host name from reet_list_environments.'),
    confirm: z.boolean().default(false).describe('Apply the change. false returns a preview only.'),
    force: z.boolean().default(false).describe('Allow forgetting a machine whose environments still list skills.'),
  }, async ({ host, confirm, force }) => {
    const environments = collection(await client.request('/api/environments'), 'environments').filter(env => env.host === host);
    if (!environments.length) throw new Error('No environments for that host. Use reet_list_environments to find its exact host name.');
    const summary = environments.map(({ skills, ...env }) => ({ id: env.id, name: env.name, agent: env.agent, skillCount: Array.isArray(skills) ? skills.length : 0 }));
    const skillCount = summary.reduce((total, env) => total + env.skillCount, 0);
    if (!confirm) return { dry_run: true, host, environments: summary, skillCount, applied: false };
    if (skillCount > 0 && !force) throw new Error(`That machine still lists ${skillCount} skills. Pass force=true to forget it anyway.`);
    await client.request('/api/v2/machines', 'DELETE', undefined, new URLSearchParams({ host }));
    return { dry_run: false, host, environments: summary, skillCount, applied: true };
  }, { write: true, destructive: true, idempotent: true });

  tool('merge_machines', 'Merge two machines', 'Merge one machine name into another so a renamed host is not counted twice. Dry run by default; set confirm=true to apply. Moves environments, sync plans and aliases from the source host to the target host; shelf skills, lists and tags are unchanged.', {
    from: z.string().trim().min(1).max(255).describe('Host name to merge away.'),
    to: z.string().trim().min(1).max(255).describe('Host name to keep.'),
    confirm: z.boolean().default(false).describe('Apply the change. false returns a preview only.'),
  }, async ({ from, to, confirm }) => {
    const hosts = new Set(collection(await client.request('/api/environments'), 'environments').map(env => env.host));
    if (!hosts.has(from) || !hosts.has(to)) throw new Error('Both hosts must exist. Use reet_list_environments to find exact host names.');
    if (from === to) throw new Error('from and to must be different hosts.');
    if (!confirm) return { dry_run: true, from, to, applied: false };
    await client.request('/api/v2/machines/merge', 'POST', { from, to });
    return { dry_run: false, from, to, applied: true };
  }, { write: true, destructive: true });

  tool('list_sync_targets', 'Inspect saved sync plans', 'Read saved sync targets and desired/missing/extra inventories. Does not create a plan or apply installs/removals; use reet sync locally to preview and confirm changes. Returns a page of sync target objects.', pagination, async args => {
    return page(collection(await client.request('/api/v2/sync'), 'targets'), args);
  });

  tool('list_lists', 'Find accessible shared lists', 'List personal and shared lists this account can access, with role and skill counts. Search matches name and description. Does not change list membership or sharing.', {
    ...pagination, q: z.string().max(500).optional().describe('Case-insensitive name or description search.'),
  }, async ({ q, ...args }) => {
    const items = collection(await client.request('/api/v2/lists'), 'lists');
    return page(items.filter(item => !q || `${item.name} ${item.description}`.toLowerCase().includes(q.toLowerCase())), args);
  });

  tool('get_list', 'Read list skills and permissions', 'Read list metadata, list notes, permissions and a page of skills. Personal shelf notes are separate from list notes. Members and audience rules are managed in the web UI.', { list_id: listId, ...pagination }, async ({ list_id, ...args }) => {
    const data = await client.request(`/api/v2/lists/${list_id}`);
    const { skills: _skills, members: _members, audience: _audience, ...metadata } = data;
    return page(collection(data, 'skills'), args, { list: metadata });
  });

  tool('create_list', 'Create a personal list', 'Create a personal list (no shared audience or publication). Returns list metadata and skills. Manage sharing in the web UI.', {
    name: z.string().trim().min(1).max(200).describe('List name, e.g. Frontend starter kit.'),
    description: z.string().max(5000).default('').describe('Purpose of the list.'),
  }, async args => client.request('/api/v2/lists', 'POST', { ...args, visibility: 'personal' }), { write: true });

  tool('update_list', 'Rename or describe a list', 'Change a list’s name and/or description. Does not change visibility, members or skills.', {
    list_id: listId,
    name: z.string().trim().min(1).max(200).optional().describe('New list name.'),
    description: z.string().max(5000).optional().describe('New description; empty string clears it.'),
  }, async ({ list_id, ...body }) => {
    if (body.name === undefined && body.description === undefined) throw new Error('Supply name or description.');
    const data = await client.request(`/api/v2/lists/${list_id}`, 'PATCH', body);
    return { id: data.id, name: data.name, description: data.description, skillCount: data.skillCount };
  }, { write: true, destructive: true, idempotent: true });

  tool('delete_list', 'Delete a list', 'Delete a list and its list notes. Dry run by default; set dry_run=false and confirm=true to apply. Does not delete shelf skills or personal notes.', {
    list_id: listId, dry_run: dryRun, confirm,
  }, async ({ list_id, ...body }) => client.request(`/api/v2/lists/${list_id}/delete`, 'POST', body), { write: true, destructive: true });

  tool('add_list_skill', 'Add a skill to a list', 'Add one shelf skill ID (including local/...) or reference to a list this account can edit, with a shared list note. Local skills are allowed only on personal lists. Does not add it to a personal shelf. Returns the updated list’s skill count and ID.', {
    list_id: listId,
    id: id.optional().describe('Exact shelf skill ID, including local/... . Use this instead of input.'),
    input: z.string().trim().min(1).max(1000).optional().describe('owner/repo/skill or skills.sh URL.'),
    note: z.string().max(20_000).default('').describe('Shared list note; separate from personal notes.'),
  }, async ({ list_id, ...body }) => {
    if ((!body.id) === (!body.input)) throw new Error('Supply exactly one of id (a shelf skill ID) or input (an owner/repo/skill reference).');
    const data = await client.request(`/api/v2/lists/${list_id}/skills`, 'POST', body);
    return { id: data.id, name: data.name, skillCount: data.skillCount };
  }, { write: true, destructive: true });

  tool('add_list_skills', 'Add many skills to a list', 'Add up to 100 shelf IDs or references to one list. Returns per-item success or a specific error. Local skills are allowed only on personal lists. Does not add them to a personal shelf.', {
    list_id: listId,
    inputs: z.array(z.object({
      id: id.optional().describe('Exact shelf skill ID, including local/... .'),
      input: z.string().trim().min(1).max(1000).optional().describe('owner/repo/skill or skills.sh URL.'),
      note: z.string().max(20_000).default('').describe('Shared list note for this skill.'),
    }).strict()).min(1).max(100).describe('Skills to add.'),
  }, async ({ list_id, inputs }) => client.request(`/api/v2/lists/${list_id}/skills/bulk`, 'POST', { inputs }), { write: true, destructive: true });

  tool('remove_list_skill', 'Remove a skill from a list', 'Remove one exact skill ID from a list this account can edit. Does not remove it from the shelf. Returns the updated list’s skill count and ID.', {
    list_id: listId, id,
  }, async ({ list_id, id }) => {
    const data = await client.request(`/api/v2/lists/${list_id}/skills`, 'DELETE', undefined, new URLSearchParams({ id }));
    return { id: data.id, name: data.name, skillCount: data.skillCount };
  }, { write: true, destructive: true, idempotent: true });

  tool('bulk_update_skills', 'Update many skills at once', 'Apply flags, a stage, tag additions/removals and one list membership to up to 500 skills, selected by ids or by a filter. dry_run=true previews without changing anything. Returns per-item success or failure. Does not edit notes or delete skills.', {
    ids: z.array(id).max(500).optional().describe('Exact shelf skill IDs; omit when using filter.'),
    filter: z.object({
      q: z.string().max(500).optional().describe('Search text.'),
      source_type: z.string().max(50).optional().describe('e.g. local, github, hosted or well-known.'),
      env: environmentId.optional(),
      stage: z.enum(['shelf', 'try', 'use']).optional(),
      sort: z.enum(['recent', 'name', 'installs']).optional(),
      tags: z.array(z.number().int().positive()).max(50).optional().describe('Require every tag ID.'),
      untagged: z.boolean().optional().describe('Only skills with no visible tags.'),
    }).strict().optional().describe('Select skills instead of listing ids.'),
    add_tags: z.array(z.string().trim().min(1).max(100)).max(50).default([]).describe('Tag names to add.'),
    remove_tag_ids: z.array(z.number().int().positive()).max(50).default([]).describe('Tag IDs to remove.'),
    stage: z.enum(['shelf', 'try', 'use']).optional().describe('Set the same stage for every skill.'),
    liked: z.boolean().optional(),
    bookmarked: z.boolean().optional(),
    add_to_list_id: z.number().int().positive().optional().describe('Also add every skill to this list.'),
    dry_run: z.boolean().default(false).describe('Preview the per-item result without changing anything.'),
  }, async body => {
    if (!body.ids?.length && !body.filter) throw new Error('Supply ids or a filter.');
    return client.request('/api/v2/skills/bulk', 'POST', body);
  }, { write: true, destructive: true });

  tool('find_duplicates', 'Find likely duplicate skills', 'Group shelf skills by normalised name and upstream source, flag local versus upstream pairs, and suggest a canonical ID. Read-only; it does not merge or remove anything.', {}, async () => client.request('/api/v2/skills/duplicates'));

  tool('list_events', 'Read shelf activity', 'Read audit events with optional skill, action and source filters. Use before to page older and after to page newer; follow has_more. source=mcp shows changes made by MCP clients, with the client name in detail. Does not change anything.', {
    limit: pagination.limit,
    skill: id.optional(),
    action: z.string().max(100).optional().describe('Exact action, e.g. bulk-update or list-delete.'),
    source: z.enum(['ui', 'mcp', 'scan', 'import']).optional().describe('Who made the change.'),
    before: z.string().regex(/^\d+$/).optional().describe('Cursor: return events older than this event ID.'),
    after: z.string().regex(/^\d+$/).optional().describe('Cursor: return events newer than this event ID.'),
  }, async ({ limit, skill, action, source, before, after }) => {
    const query = new URLSearchParams({ limit: String(limit) });
    for (const [key, value] of Object.entries({ skill, action, source, before, after })) if (value) query.set(key, String(value));
    return client.request('/api/events', 'GET', undefined, query);
  });

  tool('list_ignored', 'List ignored skills', 'List shelf entries excluded from scanner import, with reason and timestamp. Does not change anything.', pagination, async args => {
    return page(collection(await client.request('/api/ignored'), 'ignored'), args);
  });

  tool('unignore_skill', 'Allow future skill imports', 'Remove a skill from the ignored list. Future scans can import it again; this does not add it to the shelf now. Returns {ok:true}.', { id }, async ({ id }) => {
    return client.request('/api/skill/ignore', 'DELETE', undefined, new URLSearchParams({ id }));
  }, { write: true, idempotent: true });

  return server;
}
