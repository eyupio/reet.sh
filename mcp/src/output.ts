import { z } from 'zod';

// Preserve evolving API fields while declaring the stable fields agents use.
const jsonObject = <S extends z.ZodRawShape>(shape: S) => z.object(shape).catchall(z.json());
const named = { id: z.string(), name: z.string() };
const skill = jsonObject({
  ...named, notes: z.string(), liked: z.boolean(), bookmarked: z.boolean(), used: z.boolean(),
  installs: z.number(),
});
const skillSummary = jsonObject(named);
const inventorySkill = jsonObject(named);
const list = jsonObject({ id: z.number().int(), name: z.string(), skillCount: z.number().int() });
const tag = jsonObject({ id: z.number().int(), name: z.string() });
const page = (item: z.ZodType) => jsonObject({
  items: z.array(item), count: z.number().int().nonnegative(), total_count: z.number().int().nonnegative(),
  offset: z.number().int().nonnegative(), has_more: z.boolean(), next_offset: z.number().int().nonnegative().nullable(),
  truncated: z.boolean().optional(), truncated_reason: z.string().optional(),
});
const change = jsonObject({ dry_run: z.boolean() });

export const outputs: Record<string, z.ZodType> = {
  list_skills: page(skillSummary),
  get_skill: skill,
  add_skill: skill.extend({ created: z.boolean() }),
  update_skill: skill,
  remove_skill: z.object({ ok: z.boolean() }),
  list_tags: page(tag),
  tag_skill: skill,
  create_tag: tag,
  rename_tag: jsonObject({ id: z.number().int(), name: z.string(), affected: z.number().int().nonnegative() }),
  delete_tag: jsonObject({ id: z.number().int(), affected: z.number().int().nonnegative(), dry_run: z.boolean().optional(), deleted: z.boolean().optional() }),
  list_environments: page(jsonObject({ ...named, host: z.string(), agent: z.string(), complete: z.boolean(), scannedAt: z.number(), skillCount: z.number().int() })),
  get_environment: page(inventorySkill).extend({ environment: jsonObject({ ...named, complete: z.boolean(), scannedAt: z.number() }) }),
  delete_environment: jsonObject({ dry_run: z.boolean(), deleted: z.number().int().optional(), would_delete: z.array(jsonObject(named)).optional(), environments: z.array(jsonObject(named)).optional(), blocked: z.array(jsonObject({ id: z.string() })).optional() }),
  merge_environments: jsonObject({ dry_run: z.boolean(), from: z.any(), to: z.any(), skills_moved: z.number().int() }),
  forget_machine: jsonObject({ dry_run: z.boolean(), host: z.string(), environments: z.array(jsonObject(named)), skillCount: z.number().int(), applied: z.boolean() }),
  merge_machines: z.object({ dry_run: z.boolean(), from: z.string(), to: z.string(), applied: z.boolean() }),
  list_sync_targets: page(jsonObject({ target: jsonObject(named), sourceId: z.string(), desired: z.array(inventorySkill), missing: z.array(inventorySkill), extra: z.array(inventorySkill) })),
  list_lists: page(list),
  get_list: page(skill).extend({ list }),
  create_list: list.extend({ visibility: z.string(), skills: z.array(skill).nullable() }),
  update_list: list,
  delete_list: jsonObject({ deleted: z.boolean().optional(), dry_run: z.boolean().optional(), list }),
  add_list_skill: list,
  add_list_skills: jsonObject({ added: z.number().int(), failed: z.number().int(), results: z.array(jsonObject({ id: z.string().optional(), ok: z.boolean() })), list }),
  remove_list_skill: list,
  bulk_update_skills: jsonObject({ dry_run: z.boolean(), updated: z.number().int(), failed: z.number().int(), results: z.array(jsonObject({ id: z.string(), ok: z.boolean() })) }),
  find_duplicates: jsonObject({ groups: z.array(jsonObject({ kind: z.string(), key: z.string(), suggestedCanonicalId: z.string(), skills: z.array(jsonObject(named)) })), total_groups: z.number().int(), total_skills: z.number().int() }),
  list_events: jsonObject({ events: z.array(jsonObject({ id: z.number().int(), at: z.number(), source: z.string(), action: z.string() })), limit: z.number().int().optional(), has_more: z.boolean().optional(), next_before: z.number().int().nullable().optional(), next_after: z.number().int().nullable().optional() }),
  list_ignored: page(inventorySkill),
  unignore_skill: z.object({ ok: z.boolean() }),
};
