<script lang="ts">
  // The plans of every agent across the account's machines, for a page that shows the whole fleet (Home, Sessions).
  // Only machines that read sign-ins are asked, and the controller answers from each machine's last read, asking the
  // machine again once that is over five minutes old.
  import { planEntries, type MachineAccounts } from './agentAccounts'
  import { machinePlans, PLAN_REFRESH } from './planLimits'
  import PlanMeters from './PlanMeters.svelte'

  let { machines, max = 3, caption = '' }: { machines: { agentId: string; host: string; capabilities?: { agentAccounts?: boolean } }[] | undefined; max?: number; caption?: string } = $props()
  const asked = $derived((machines ?? []).filter(machine => machine.capabilities?.agentAccounts === true))
  const askedKey = $derived(asked.map(machine => machine.agentId).sort().join(','))
  let answers = $state<Record<string, MachineAccounts>>({}), now = $state(Date.now())
  $effect(() => {
    const ids = askedKey ? askedKey.split(',') : []
    if (!ids.length) return
    let live = true
    const read = () => { for (const id of ids) void machinePlans(id).then(answer => { if (live && answer) { answers = { ...answers, [id]: answer }; now = Date.now() } }) }
    read()
    const timer = setInterval(read, PLAN_REFRESH)
    return () => { live = false; clearInterval(timer) }
  })
  const entries = $derived(planEntries(asked.map(machine => ({ host: machine.host, answer: answers[machine.agentId] })), now))
</script>

<PlanMeters {entries} {now} {max} {caption} />
