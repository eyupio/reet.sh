<script lang="ts">
  import PageHeader from './PageHeader.svelte'
  import { t } from './copy'
</script>

<PageHeader group={t('notFound.eyebrow')} title={t('notFound.title')} lede={t('notFound.lede')}>
  {#snippet actions()}<a class="btn primary" href="/">{t('notFound.back')}</a>{/snippet}
</PageHeader>
