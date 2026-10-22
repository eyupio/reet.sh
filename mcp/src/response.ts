import type { JsonObject } from './client.js';

export const CHARACTER_LIMIT = 25_000;
const PAGE_BUDGET = CHARACTER_LIMIT - 128;

export interface PageArgs { limit: number; offset: number }

export function page(items: JsonObject[], args: PageArgs, metadata: JsonObject = {}): JsonObject {
  const selected = items.slice(args.offset, args.offset + args.limit);
  const result: JsonObject = { ...metadata, items: selected, total_count: items.length, offset: args.offset };
  const update = (): void => {
    result.count = selected.length;
    result.has_more = args.offset + selected.length < items.length;
    result.next_offset = result.has_more ? args.offset + selected.length : null;
  };
  update();
  while (JSON.stringify({ data: result }).length > PAGE_BUDGET && selected.length > 1) {
    selected.pop();
    update();
    result.truncated = true;
    result.truncated_reason = 'response_character_limit';
  }
  if (JSON.stringify({ data: result }).length > PAGE_BUDGET) {
    throw new Error('One record exceeds the 25,000 character response limit. Narrow the query or shorten its notes in the web UI.');
  }
  return result;
}

export function formatResult(data: JsonObject, format: 'json' | 'markdown') {
  const structuredContent = { data };
  const json = JSON.stringify(structuredContent);
  if (json.length > CHARACTER_LIMIT) {
    throw new Error('Response exceeds 25,000 characters. Use a paginated list tool or narrow the query.');
  }
  const pretty = `reet.sh result\n\n\`\`\`json\n${JSON.stringify(data, null, 2)}\n\`\`\``;
  const markdown = pretty.length <= CHARACTER_LIMIT ? pretty : `reet.sh result\n\n\`\`\`json\n${JSON.stringify(data)}\n\`\`\``;
  if (format === 'markdown' && markdown.length > CHARACTER_LIMIT) {
    throw new Error('Response exceeds 25,000 characters in Markdown. Request response_format="json" or a smaller page.');
  }
  return {
    content: [{ type: 'text' as const, text: format === 'json' ? json : markdown }],
    structuredContent,
  };
}
