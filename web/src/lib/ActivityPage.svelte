<script lang="ts">
  import PageHeader from './PageHeader.svelte'
  import ActivitySection from './ActivitySection.svelte'

  // /activity for an instance without accounts: there is no account page to hold the list, so it keeps its own heading.
  // With accounts, the same section lives inside Account at /account/activity.
  let { accounts = true, writer = false, onsignedout }: { accounts?: boolean; writer?: boolean; onsignedout?: (e: unknown) => void } = $props()
</script>

<PageHeader eyebrow={accounts ? 'Your account' : 'This workspace'} title="Activity" lede="What you changed lately, newest first." />
<ActivitySection {writer} {onsignedout} flat />
