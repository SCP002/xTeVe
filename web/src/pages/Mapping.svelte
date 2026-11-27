<script lang="ts">
  import ChevronLeft from '@lucide/svelte/icons/chevron-left'
  import ChevronRight from '@lucide/svelte/icons/chevron-right'
  import Pencil from '@lucide/svelte/icons/pencil'
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import { Checkbox } from '$lib/components/ui/checkbox'
  import { Input } from '$lib/components/ui/input'
  import { Switch } from '$lib/components/ui/switch'
  import * as Table from '$lib/components/ui/table'
  import * as Tabs from '$lib/components/ui/tabs'
  import type { Channels, XEPGChannel } from '$lib/api/types'
  import { byNumber, freeChannelNumber, mappable, xmltvChannelName, xmltvFiles } from '$lib/mapping'
  import { app, command, server } from '$lib/state.svelte'
  import ChannelDialog from '../components/ChannelDialog.svelte'

  const xepg = $derived(server().xepg ?? { epgMapping: {}, xmltvMap: {} })
  const providers = $derived(server().settings.files.xmltv)
  const files = $derived(xmltvFiles(xepg.xmltvMap, providers))

  // Changes are made to a copy and saved together; without changes the copy follows xTeVe
  let draft = $state<Channels>({})
  let dirty = $state(false)
  $effect(() => {
    const current = xepg.epgMapping
    if (!dirty) draft = structuredClone($state.snapshot(current))
  })

  let search = $state('')
  let status = $state<'all' | 'active' | 'inactive'>('all')
  let page = $state(0)
  const pageSize = 50

  const visible = $derived.by(() => {
    const words = search.toLowerCase().split(/\s+/).filter(Boolean)
    return Object.keys(draft)
      .filter((id) => {
        const c = draft[id]
        if (status === 'active' && !c['x-active']) return false
        if (status === 'inactive' && c['x-active']) return false
        const text = [c['x-channelID'], c['x-name'], c.name, c['x-group-title'], c['x-mapping'], c['_file.m3u.name']]
          .join(' ')
          .toLowerCase()
        return words.every((w) => text.includes(w))
      })
      .sort(byNumber(draft))
  })
  const pages = $derived(Math.max(1, Math.ceil(visible.length / pageSize)))
  const shown = $derived(visible.slice(page * pageSize, (page + 1) * pageSize))
  $effect(() => {
    // A new search starts at the first page
    void search
    void status
    page = 0
  })

  const counts = $derived.by(() => {
    const all = Object.values(draft)
    const active = all.filter((c) => c['x-active']).length
    return { all: all.length, active, inactive: all.length - active }
  })

  let selected = $state(new Set<string>())
  const allShownSelected = $derived(shown.length > 0 && shown.every((id) => selected.has(id)))

  function toggleSelected(id: string, on: boolean) {
    const next = new Set(selected)
    if (on) next.add(id)
    else next.delete(id)
    selected = next
  }

  function toggleShown(on: boolean) {
    const next = new Set(selected)
    for (const id of shown) {
      if (on) next.add(id)
      else next.delete(id)
    }
    selected = next
  }

  function change(id: string, changes: Partial<XEPGChannel>) {
    Object.assign(draft[id], changes)
    dirty = true
  }

  function setActive(id: string, active: boolean) {
    if (active && !mappable(draft[id])) {
      toast.error(`${draft[id]['x-name']}: choose an XMLTV file and channel first`)
      return
    }
    change(id, { 'x-active': active })
  }

  function setNumber(id: string, input: HTMLInputElement) {
    const number = parseFloat(input.value.replace(',', '.'))
    if (Number.isNaN(number)) {
      toast.error('Invalid channel number')
      input.value = draft[id]['x-channelID']
      return
    }
    const free = freeChannelNumber(draft, id, number)
    if (free !== number) toast.info(`${number} is taken, the channel gets ${free}`)
    input.value = String(free)
    change(id, { 'x-channelID': String(free) })
  }

  let dialogOpen = $state(false)
  let editing = $state<string[]>([])

  function edit(id: string) {
    // With a selection, the channel is edited together with the selected ones
    editing = selected.size > 0 && selected.has(id) ? [...selected] : [id]
    dialogOpen = true
  }

  function apply(ids: string[], changes: Partial<XEPGChannel>) {
    for (const id of ids) {
      const next = { ...changes }
      // A channel without mapping can not be active, also in a bulk edit
      if (next['x-active'] && !mappable({ ...draft[id], ...next })) next['x-active'] = false
      change(id, next)
    }
  }

  async function save() {
    if (await command('saveEpgMapping', { epgMapping: $state.snapshot(draft) })) {
      dirty = false
      selected = new Set()
      toast.success('Mapping saved')
    }
  }

  function discard() {
    dirty = false
    draft = structuredClone($state.snapshot(xepg.epgMapping))
  }

  function fileName(file: string) {
    return file === '-' ? '' : (files.find((f) => f.value === file)?.label ?? file)
  }

  // Leaving with unsaved changes asks first
  $effect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    addEventListener('beforeunload', warn)
    return () => removeEventListener('beforeunload', warn)
  })
</script>

<div class="grid gap-4">
  <div class="flex flex-wrap items-center gap-2">
    <Input class="w-full sm:w-72" type="search" placeholder="Search channels…" bind:value={search} />
    <Tabs.Root bind:value={status}>
      <Tabs.List>
        <Tabs.Trigger value="all">All {counts.all}</Tabs.Trigger>
        <Tabs.Trigger value="active">Active {counts.active}</Tabs.Trigger>
        <Tabs.Trigger value="inactive">Inactive {counts.inactive}</Tabs.Trigger>
      </Tabs.List>
    </Tabs.Root>
    {#if selected.size > 0}
      <Button variant="outline" size="sm" onclick={() => edit([...selected][0])}>Edit {selected.size} selected</Button>
      <Button variant="ghost" size="sm" onclick={() => (selected = new Set())}>Clear selection</Button>
    {/if}
    <div class="ml-auto flex items-center gap-2">
      {#if dirty}<span class="text-sm text-muted-foreground">Unsaved changes</span>{/if}
      <Button variant="outline" size="sm" disabled={!dirty || app.busy} onclick={discard}>Discard</Button>
      <Button size="sm" disabled={!dirty || app.busy} onclick={save}>Save</Button>
    </div>
  </div>

  {#if counts.all === 0}
    <p class="text-sm text-muted-foreground">No channels: add a playlist, and filters if needed.</p>
  {:else}
    <div class="rounded-md border">
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head class="w-8">
              <Checkbox
                checked={allShownSelected}
                onCheckedChange={toggleShown}
                aria-label="Select the shown channels"
              />
            </Table.Head>
            <Table.Head class="w-24">Number</Table.Head>
            <Table.Head>Channel</Table.Head>
            <Table.Head>Group</Table.Head>
            <Table.Head>XMLTV</Table.Head>
            <Table.Head class="w-16">Active</Table.Head>
            <Table.Head class="w-0"><span class="sr-only">Edit</span></Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each shown as id (id)}
            {@const c = draft[id]}
            <Table.Row data-state={selected.has(id) ? 'selected' : undefined}>
              <Table.Cell>
                <Checkbox
                  checked={selected.has(id)}
                  onCheckedChange={(on) => toggleSelected(id, on)}
                  aria-label="Select {c['x-name']}"
                />
              </Table.Cell>
              <Table.Cell>
                <Input
                  class="h-8 w-20 tabular-nums"
                  inputmode="decimal"
                  value={c['x-channelID']}
                  aria-label="Number of {c['x-name']}"
                  onchange={(e) => setNumber(id, e.currentTarget)}
                />
              </Table.Cell>
              <Table.Cell class="max-w-72">
                <div class="flex items-center gap-2">
                  {#if c['tvg-logo']}
                    <img src={c['tvg-logo']} alt="" loading="lazy" class="size-8 shrink-0 rounded object-contain" />
                  {:else}
                    <div class="size-8 shrink-0 rounded bg-muted"></div>
                  {/if}
                  <div class="min-w-0">
                    <div class={['truncate font-medium', !c['x-active'] && 'text-muted-foreground']}>{c['x-name']}</div>
                    {#if c['x-name'] !== c.name}<div class="truncate text-xs text-muted-foreground">{c.name}</div>{/if}
                  </div>
                </div>
              </Table.Cell>
              <Table.Cell class="max-w-40 truncate text-muted-foreground">{c['x-group-title']}</Table.Cell>
              <Table.Cell class="max-w-64">
                {#if mappable(c)}
                  <div class="truncate">{xmltvChannelName(xepg.xmltvMap, c['x-xmltv-file'], c['x-mapping'])}</div>
                  <div class="truncate text-xs text-muted-foreground">{fileName(c['x-xmltv-file'])}</div>
                {:else}
                  <span class="text-xs text-muted-foreground">Not mapped</span>
                {/if}
              </Table.Cell>
              <Table.Cell>
                <Switch
                  checked={c['x-active']}
                  onCheckedChange={(on) => setActive(id, on)}
                  aria-label="Active {c['x-name']}"
                />
              </Table.Cell>
              <Table.Cell>
                <Button variant="ghost" size="icon" aria-label="Edit {c['x-name']}" onclick={() => edit(id)}>
                  <Pencil />
                </Button>
              </Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row>
              <Table.Cell colspan={7} class="text-muted-foreground">No channel matches.</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    </div>

    {#if pages > 1}
      <div class="flex items-center justify-end gap-2 text-sm">
        <span class="text-muted-foreground">
          {page * pageSize + 1}–{Math.min((page + 1) * pageSize, visible.length)} of {visible.length}
        </span>
        <Button variant="outline" size="icon" aria-label="Previous page" disabled={page === 0} onclick={() => page--}>
          <ChevronLeft />
        </Button>
        <Button
          variant="outline"
          size="icon"
          aria-label="Next page"
          disabled={page >= pages - 1}
          onclick={() => page++}
        >
          <ChevronRight />
        </Button>
      </div>
    {/if}
  {/if}
</div>

<ChannelDialog bind:open={dialogOpen} ids={editing} channels={draft} mapping={xepg.xmltvMap} {providers} {apply} />
