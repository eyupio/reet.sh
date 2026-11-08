<script lang="ts">
  import { getContext, type Snippet } from 'svelte'
  import SectionNav, { type SectionItem } from './SectionNav.svelte'
  import { PAGE_GROUP, type PageGroupContext } from './shell/pageNav'

  // `eyebrow` is a temporary alias for `group` until every page has moved over.
  let { group, groupHref, title, lede = '', titleId, sections, sectionsLabel, meta, actions, eyebrow }: {
    group?: string
    groupHref?: string
    title: string
    lede?: string
    titleId?: string
    sections?: SectionItem[]
    sectionsLabel?: string
    meta?: Snippet
    actions?: Snippet
    eyebrow?: string
  } = $props()

  // The page's group from the More list, when App.svelte shares one (a signed-out page has none).
  const shared = getContext<PageGroupContext | undefined>(PAGE_GROUP)
  const context = $derived(shared?.group ?? null)
  const label = $derived(group ?? eyebrow ?? context?.group ?? '')
  const row = $derived(sections
    ? { label: sectionsLabel ?? label, items: sections }
    : context && context.items.length >= 2 ? { label: context.group, items: context.items } : null)
</script>

<div class="page-head">
  <header class="page-heading">
    <div class="copy">
      {#if label}<!-- `eyebrow` is kept as a hook: navigation.e2e.mjs finds a page's label by it. --><p class="group eyebrow">{#if groupHref}<a href={groupHref}>{label}</a>{:else}{label}{/if}</p>{/if}
      <h1 id={titleId}>{title}</h1>
      {#if lede}<p class="lede">{lede}</p>{/if}
      {#if meta}<div class="meta">{@render meta()}</div>{/if}
    </div>
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </header>
  {#if row}<div class="row"><SectionNav label={row.label} items={row.items} /></div>{/if}
</div>

<style>
  .page-head { margin-bottom: 24px; padding-bottom: 16px; border-bottom: 1px solid var(--line-soft); }
  .page-heading { display: flex; justify-content: space-between; align-items: flex-end; gap: 16px 24px; }
  .copy { min-width: 0; }
  .group { margin: 0; color: var(--muted); font: 400 0.8125rem/1.4 var(--mono); letter-spacing: 0.01em; overflow-wrap: anywhere; }
  .group a { color: inherit; text-decoration: none; }
  .group a:hover { color: var(--ink); text-decoration: underline; }
  h1 { margin: 4px 0 6px; font: 600 1.9rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .lede { margin: 0; color: var(--muted); max-width: 66ch; font-size: 0.875rem; line-height: 1.55; }
  .meta { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; color: var(--muted); font-size: 0.875rem; font-variant-numeric: tabular-nums; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; flex: none; }
  /* The row sits above the header's hairline, so it brings no rule or spacing of its own. */
  .row { margin-top: 12px; }
  .row :global(nav) { border-bottom: 0; padding-bottom: 0; margin: 0; }
  @media (max-width: 1000px) {
    .page-head { margin-bottom: 16px; padding-bottom: 12px; }
    .page-heading { align-items: flex-start; flex-direction: column; gap: 12px; }
    h1 { font-size: 1.5rem; }
  }
</style>
