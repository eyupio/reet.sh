<script lang="ts">
  import { onMount } from 'svelte'
  import { listSkills, type Skill } from '../api'
  import { installPrompt } from '../install'
  import SlotPicker, { type SlotOption } from '../launch/SlotPicker.svelte'

  // Search the person's shelf and hand back one skill to put into the prompt. The prompt itself is inserted as ordinary
  // editable text by the composer; this dialog knows nothing about where the caret is.
  let { opener = null, sheet = false, oninsert, onclose }: {
    opener: HTMLElement | null
    sheet: boolean
    oninsert: (skill: Skill) => void
    onclose: () => void
  } = $props()

  const live = { current: true }
  let skills = $state<Skill[]>([])
  let loading = $state(true)
  let error = $state('')
  let query = $state('')
  let sequence = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  const rank = (group: string): number => ['In use', 'Saved', 'On your shelf'].indexOf(group)
  const options = $derived(skills.map(skill => {
    const prompt = installPrompt(skill)
    return {
      value: skill.id,
      label: skill.name || skill.slug || skill.id,
      description: skill.description || '',
      detail: skill.source || skill.url || '',
      group: skill.used ? 'In use' : skill.bookmarked ? 'Saved' : 'On your shelf',
      disabled: !prompt,
      reason: prompt ? undefined : 'No source to run this skill from. Link one on the Skills page.',
    } satisfies SlotOption
  }).sort((a, b) => rank(a.group ?? '') - rank(b.group ?? '') || a.label.localeCompare(b.label)))

  const status = $derived.by(() => {
    if (loading) return query ? `Searching for “${query}”…` : 'Reading your skills…'
    if (error) return error
    if (!skills.length) return query ? `No skills match “${query}”.` : 'Your shelf is empty. Add skills on the Skills page.'
    const count = `${skills.length} ${skills.length === 1 ? 'skill' : 'skills'}`
    return query ? `${count} matching “${query}”.` : `${count} ready to use.`
  })

  async function load(text: string) {
    query = text
    const mine = ++sequence
    loading = true
    error = ''
    clearTimeout(timer)
    try {
      const answer = await Promise.race([
        listSkills({ tab: 'all', q: text, sort: 'recent' }),
        new Promise<never>((_, no) => { timer = setTimeout(() => no(new Error('timeout')), 15_000) }),
      ])
      if (!live.current || mine !== sequence) return
      skills = answer.skills ?? []
    } catch {
      if (!live.current || mine !== sequence) return
      skills = []
      error = 'Couldn’t read your skills just now. Try again in a moment.'
    } finally {
      clearTimeout(timer)
      if (live.current && mine === sequence) loading = false
    }
  }

  function pick(value: string) {
    const skill = skills.find(item => item.id === value)
    if (skill) oninsert(skill)
  }

  onMount(() => {
    void load('')
    return () => { live.current = false; sequence++; clearTimeout(timer) }
  })
</script>

<SlotPicker label="Skill" options={options} value="" sheet={sheet} opener={opener}
  filter="Search your skills" onfilter={(text) => { void load(text) }} status={status}
  onpick={pick} {onclose} />
