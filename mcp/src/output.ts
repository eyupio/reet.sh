import { z } from 'zod';

// Preserve evolving API fields while declaring the stable fields agents use.
const jsonObject = <S extends z.ZodRawShape>(shape: S) => z.object(shape).catchall(z.json());
const named = { id: z.string(), name: z.string() };
const skill = jsonObject({
  ...named, notes: z.string(), liked: z.boolean(), bookmarked: z.boolean(), used: z.boolean(),
  installs: z.number(),
});
const inventorySkill = jsonObject(named);
const list = jsonObject({ id: z.number().int(), name: z.string(), skillCount: z.number().int() });
const page = (item: z.ZodType) => jsonObject({
  items: z.array(item), count: z.number().int().nonnegative(), total_count: z.number().int().nonnegative(),
  offset: z.number().int().nonnegative(), has_more: z.boolean(), next_offset: z.number().int().nonnegative().nullable(),
  truncated: z.boolean().optional(),
});

export const outputs: Record<string, z.ZodType> = {
  list_skills: page(skill),
  get_skill: skill,
  add_skill: skill.extend({ created: z.boolean() }),
  update_skill: skill,
  remove_skill: z.object({ ok: z.boolean() }),
  list_tags: page(jsonObject({ id: z.number().int(), name: z.string() })),
  tag_skill: skill,
  list_environments: page(jsonObject({ ...named, host: z.string(), agent: z.string(), complete: z.boolean(), scannedAt: z.number(), skillCount: z.number().int() })),
  get_environment: page(inventorySkill).extend({ environment: jsonObject({ ...named, complete: z.boolean(), scannedAt: z.number() }) }),
  forget_machine: jsonObject({ dry_run: z.boolean(), host: z.string(), environments: z.array(jsonObject(named)), skillCount: z.number().int(), applied: z.boolean() }),
  merge_machines: z.object({ dry_run: z.boolean(), from: z.string(), to: z.string(), applied: z.boolean() }),
  list_sync_targets: page(jsonObject({ target: jsonObject(named), sourceId: z.string(), desired: z.array(inventorySkill), missing: z.array(inventorySkill), extra: z.array(inventorySkill) })),
  list_lists: page(list),
  get_list: page(skill).extend({ list }),
  create_list: list.extend({ visibility: z.string(), skills: z.array(skill).nullable() }),
  add_list_skill: list,
  list_events: z.object({ events: z.array(jsonObject({ id: z.number().int(), at: z.number(), source: z.string(), action: z.string() })), limit: z.number().int() }),
  list_ignored: page(inventorySkill),
  unignore_skill: z.object({ ok: z.boolean() }),
};
