<script lang="ts">
  import Check from '@lucide/svelte/icons/check'
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down'
  import { Button } from '$lib/components/ui/button'
  import * as Command from '$lib/components/ui/command'
  import * as Popover from '$lib/components/ui/popover'
  import type { Mapping } from '$lib/api/types'
  import { dummyDurations, dummyFile, xmltvChannelName } from '$lib/mapping'

  let {
    value = $bindable(),
    id,
    file,
    mapping,
  }: { value: string; id?: string; file: string; mapping: Mapping } = $props()

  let open = $state(false)
  let search = $state('')

  // All channels of the file with their names; files have thousands of channels, so only the first matches show
  const options = $derived.by(() => {
    if (file === dummyFile) return dummyDurations.map((d) => ({ value: d.value, label: d.label, names: d.label }))
    return Object.entries(mapping[file] ?? {}).map(([channelID, c]) => {
      const names = (c['display-names'] ?? []).map((n) => n.Value)
      return { value: channelID, label: names[0] ?? channelID, names: `${channelID} ${names.join(' ')}`.toLowerCase() }
    })
  })
  const limit = 100
  const matches = $derived.by(() => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean)
    const found = options.filter((o) => words.every((w) => o.names.includes(w)))
    return { shown: found.slice(0, limit), more: Math.max(found.length - limit, 0) }
  })

  const label = $derived(value && value !== '-' ? xmltvChannelName(mapping, file, value) : 'None')

  function choose(v: string) {
    value = v
    open = false
  }
</script>

<Popover.Root bind:open>
  <Popover.Trigger>
    {#snippet child({ props })}
      <Button {...props} {id} variant="outline" role="combobox" class="w-full justify-between font-normal">
        <span class="truncate">{label}</span>
        {#if value && value !== '-' && file !== dummyFile}
          <span class="ml-auto truncate pl-2 text-xs text-muted-foreground">{value}</span>
        {/if}
        <ChevronsUpDown class="opacity-50" />
      </Button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content class="w-(--bits-popover-anchor-width) min-w-72 p-0" align="start">
    <Command.Root shouldFilter={false}>
      <Command.Input placeholder="Search by name or ID…" bind:value={search} />
      <Command.List>
        <Command.Empty>No channel found.</Command.Empty>
        <Command.Group>
          <Command.Item value="-" onSelect={() => choose('-')}>
            <Check class={value === '-' ? '' : 'invisible'} />
            None
          </Command.Item>
          {#each matches.shown as option (option.value)}
            <Command.Item value={option.value} onSelect={() => choose(option.value)}>
              <Check class={value === option.value ? '' : 'invisible'} />
              <span class="truncate">{option.label}</span>
              {#if file !== dummyFile}
                <span class="ml-auto truncate text-xs text-muted-foreground">{option.value}</span>
              {/if}
            </Command.Item>
          {/each}
        </Command.Group>
        {#if matches.more > 0}
          <p class="p-2 text-center text-xs text-muted-foreground">{matches.more} more, refine the search</p>
        {/if}
      </Command.List>
    </Command.Root>
  </Popover.Content>
</Popover.Root>
