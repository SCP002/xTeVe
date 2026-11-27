<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil'
  import Plus from '@lucide/svelte/icons/plus'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import { Badge } from '$lib/components/ui/badge'
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu'
  import * as Table from '$lib/components/ui/table'
  import type { Filter, FilterUpdate } from '$lib/api/types'
  import { app, command, server } from '$lib/state.svelte'
  import FilterDialog, { type FilterType } from '../components/FilterDialog.svelte'

  const filters = $derived(
    Object.entries(server().settings.filter ?? {}).sort(([, a], [, b]) => a.name.localeCompare(b.name)),
  )
  const preview = $derived(server().data.StreamPreviewUI)

  let dialogOpen = $state(false)
  let dialogType = $state<FilterType>('group-title')
  let editing = $state<{ id: string; filter: Filter } | undefined>()
  let deleting = $state<{ id: string; filter: Filter } | undefined>()

  function openAdd(type: FilterType) {
    editing = undefined
    dialogType = type
    dialogOpen = true
  }

  function openEdit(id: string, filter: Filter) {
    editing = { id, filter }
    dialogType = filter.type as FilterType
    dialogOpen = true
  }

  // A new filter has the ID -1
  const save = (update: FilterUpdate) => command('saveFilter', { filter: { [editing?.id ?? '-1']: update } })

  async function confirmDelete() {
    if (deleting && (await command('saveFilter', { filter: { [deleting.id]: { delete: true } } }))) {
      deleting = undefined
    }
  }

  function rule(f: Filter) {
    if (f.type !== 'group-title') return f.filter
    return [f.filter, f.include && `{${f.include}}`, f.exclude && `!{${f.exclude}}`].filter(Boolean).join(' ')
  }
</script>

<div class="grid gap-4">
  <Card.Root>
    <Card.Header>
      <Card.Title>Filters</Card.Title>
      <Card.Description>
        With filters, only the streams they take are active. Without filters, all streams are active.
      </Card.Description>
      <Card.Action>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger>
            {#snippet child({ props })}
              <Button {...props} size="sm" disabled={app.busy}><Plus /> Add</Button>
            {/snippet}
          </DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            <DropdownMenu.Item onclick={() => openAdd('group-title')}>Group filter</DropdownMenu.Item>
            <DropdownMenu.Item onclick={() => openAdd('custom-filter')}>Custom filter</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      </Card.Action>
    </Card.Header>
    <Card.Content>
      {#if filters.length === 0}
        <p class="text-sm text-muted-foreground">None yet.</p>
      {:else}
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Name</Table.Head>
              <Table.Head>Type</Table.Head>
              <Table.Head>Rule</Table.Head>
              <Table.Head class="w-0"><span class="sr-only">Actions</span></Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each filters as [id, f] (id)}
              <Table.Row>
                <Table.Cell>
                  <div class="font-medium">{f.name}</div>
                  {#if f.description}<div class="text-xs text-muted-foreground">{f.description}</div>{/if}
                </Table.Cell>
                <Table.Cell>
                  <Badge variant="outline">{f.type === 'group-title' ? 'Group' : 'Custom'}</Badge>
                  {#if f.caseSensitive}<Badge variant="outline">Aa</Badge>{/if}
                </Table.Cell>
                <Table.Cell class="font-mono text-xs">{rule(f)}</Table.Cell>
                <Table.Cell>
                  <div class="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Edit {f.name}"
                      disabled={app.busy}
                      onclick={() => openEdit(id, f)}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Delete {f.name}"
                      disabled={app.busy}
                      onclick={() => (deleting = { id, filter: f })}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>

  <div class="grid gap-4 lg:grid-cols-2">
    {#each [{ title: 'Active streams', list: preview.activeStreams ?? [] }, { title: 'Inactive streams', list: preview.inactiveStreams ?? [] }] as part (part.title)}
      <Card.Root>
        <Card.Header>
          <Card.Title>{part.title}</Card.Title>
          <Card.Description>{part.list.length} streams</Card.Description>
        </Card.Header>
        <Card.Content>
          <ul class="max-h-80 overflow-y-auto text-sm">
            {#each part.list as stream, i (i)}
              <li class="truncate border-b py-1 last:border-0">{stream}</li>
            {:else}
              <li class="text-muted-foreground">None.</li>
            {/each}
          </ul>
        </Card.Content>
      </Card.Root>
    {/each}
  </div>
</div>

<FilterDialog bind:open={dialogOpen} type={dialogType} filter={editing?.filter} {save} />

<AlertDialog.Root open={deleting !== undefined} onOpenChange={(open) => !open && (deleting = undefined)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Delete the filter {deleting?.filter.name}?</AlertDialog.Title>
      <AlertDialog.Description>Its streams become inactive unless another filter takes them.</AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={confirmDelete} disabled={app.busy}>Delete</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
