<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import * as Select from '$lib/components/ui/select'
  import { Switch } from '$lib/components/ui/switch'
  import { Textarea } from '$lib/components/ui/textarea'
  import type { Filter, FilterUpdate } from '$lib/api/types'
  import { app, server } from '$lib/state.svelte'

  export type FilterType = 'group-title' | 'custom-filter'

  let {
    open = $bindable(false),
    type,
    filter,
    save,
  }: {
    open?: boolean
    type: FilterType
    /** The filter to edit, undefined to add one. */
    filter?: Filter
    save: (update: FilterUpdate) => Promise<boolean>
  } = $props()

  let name = $state('')
  let description = $state('')
  let rule = $state('')
  let include = $state('')
  let exclude = $state('')
  let caseSensitive = $state(false)
  let preserveMapping = $state(true)
  let startingChannel = $state('')

  $effect(() => {
    if (!open) return
    name = filter?.name ?? ''
    description = filter?.description ?? ''
    rule = filter?.filter ?? ''
    include = filter?.include ?? ''
    exclude = filter?.exclude ?? ''
    caseSensitive = filter?.caseSensitive ?? false
    preserveMapping = filter?.preserveMapping ?? true
    startingChannel = filter?.startingChannel ?? String(server().settings['mapping.first.channel'])
  })

  // The groups of the playlists, with the number of their streams
  const groups = $derived.by(() => {
    const { text, value } = server().data.playlist.m3u.groups
    return (value ?? []).map((v, i) => ({ value: v, label: text?.[i] ?? v }))
  })
  const groupLabel = $derived(groups.find((g) => g.value === rule)?.label ?? (rule || 'Select a group'))

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    const update: FilterUpdate = { name, description, type, filter: rule, caseSensitive }
    if (type === 'group-title') Object.assign(update, { include, exclude, preserveMapping, startingChannel })
    if (await save(update)) open = false
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-lg">
    <Dialog.Header>
      <Dialog.Title>
        {filter ? 'Edit filter' : type === 'group-title' ? 'Add group filter' : 'Add custom filter'}
      </Dialog.Title>
      <Dialog.Description>
        {type === 'group-title'
          ? 'Takes the streams of an M3U group.'
          : 'Takes the streams whose M3U entry contains the rule.'}
      </Dialog.Description>
    </Dialog.Header>
    <form id="filter" class="grid gap-4" onsubmit={submit}>
      <div class="grid gap-2">
        <Label for="filter-name">Name</Label>
        <Input id="filter-name" required bind:value={name} />
      </div>
      <div class="grid gap-2">
        <Label for="filter-description">Description</Label>
        <Textarea id="filter-description" rows={2} bind:value={description} />
      </div>

      {#if type === 'group-title'}
        <div class="grid gap-2">
          <Label for="filter-group">Group</Label>
          <Select.Root type="single" bind:value={rule}>
            <Select.Trigger id="filter-group" class="w-full">{groupLabel}</Select.Trigger>
            <Select.Content>
              {#each groups as group (group.value)}
                <Select.Item value={group.value} label={group.label} />
              {:else}
                <p class="p-2 text-sm text-muted-foreground">No groups: add a playlist first.</p>
              {/each}
            </Select.Content>
          </Select.Root>
          <p class="text-sm text-muted-foreground">The filter stops working if the playlist renames the group.</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="grid gap-2">
            <Label for="filter-include">Name includes</Label>
            <Input id="filter-include" placeholder="HD,FHD" bind:value={include} />
          </div>
          <div class="grid gap-2">
            <Label for="filter-exclude">Name excludes</Label>
            <Input id="filter-exclude" placeholder="ES,IT" bind:value={exclude} />
          </div>
        </div>
        <p class="-mt-2 text-sm text-muted-foreground">Comma separated, a comma means "or".</p>
        <div class="grid gap-2">
          <Label for="filter-start">First channel number</Label>
          <Input id="filter-start" inputmode="decimal" bind:value={startingChannel} />
        </div>
        <label class="flex items-center justify-between gap-4 text-sm">
          Keep the channel numbers of the playlist
          <Switch bind:checked={preserveMapping} />
        </label>
      {:else}
        <div class="grid gap-2">
          <Label for="filter-rule">Rule</Label>
          <Input id="filter-rule" required placeholder={'Sport {HD} !{ES,IT}'} bind:value={rule} />
          <p class="text-sm text-muted-foreground">
            Text the M3U entry must contain; <code>{'{HD}'}</code> requires one of the words,
            <code>{'!{ES,IT}'}</code> excludes them.
          </p>
        </div>
      {/if}

      <label class="flex items-center justify-between gap-4 text-sm">
        Case sensitive
        <Switch bind:checked={caseSensitive} />
      </label>
    </form>
    <Dialog.Footer>
      <Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
      <Button type="submit" form="filter" disabled={app.busy || !rule}>{app.busy ? 'Please wait…' : 'Save'}</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
