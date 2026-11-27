<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil'
  import Plus from '@lucide/svelte/icons/plus'
  import RefreshCw from '@lucide/svelte/icons/refresh-cw'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import { Badge } from '$lib/components/ui/badge'
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import * as Table from '$lib/components/ui/table'
  import * as Tooltip from '$lib/components/ui/tooltip'
  import { add, edit, refresh, remove } from '$lib/api/providers'
  import type { Provider, ProviderKind } from '$lib/api/types'
  import { app, server } from '$lib/state.svelte'
  import ProviderDialog from './ProviderDialog.svelte'

  let { kind, title, description }: { kind: ProviderKind; title: string; description: string } = $props()

  const providers = $derived(
    Object.entries(server().settings.files[kind] ?? {}).sort(([, a], [, b]) => a.name.localeCompare(b.name)),
  )

  // Columns of the statistics xTeVe keeps for the kind
  const stats: { key: string; label: string; percent?: boolean; hint?: string }[] = $derived(
    kind === 'xmltv'
      ? [
          { key: 'xmltv.channels', label: 'Channels' },
          { key: 'xmltv.programs', label: 'Programs' },
        ]
      : [
          { key: 'streams', label: 'Streams' },
          { key: 'group.title', label: 'group-title', percent: true, hint: 'Streams with a group' },
          { key: 'tvg.id', label: 'tvg-id', percent: true, hint: 'Streams with an EPG ID' },
          { key: 'stream.id', label: 'Unique ID', percent: true, hint: 'Streams with a unique ID of the provider' },
        ],
  )

  let dialogOpen = $state(false)
  let editing = $state<{ id: string; provider: Provider } | undefined>()
  let deleting = $state<{ id: string; provider: Provider } | undefined>()

  function openAdd() {
    editing = undefined
    dialogOpen = true
  }

  function openEdit(id: string, provider: Provider) {
    editing = { id, provider }
    dialogOpen = true
  }

  function save(update: Parameters<typeof add>[1]) {
    if (!editing) return add(kind, update)
    const changed = update['file.source'] !== editing.provider['file.source']
    return edit(kind, editing.id, update, changed)
  }

  async function confirmDelete() {
    if (deleting && (await remove(kind, deleting.id))) deleting = undefined
  }

  function availability(p: Provider) {
    const value = p['provider.availability']
    return { value, variant: value >= 90 ? 'secondary' : value >= 50 ? 'outline' : 'destructive' } as const
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>{title}</Card.Title>
    <Card.Description>{description}</Card.Description>
    <Card.Action>
      <Button size="sm" onclick={openAdd} disabled={app.busy}><Plus /> Add</Button>
    </Card.Action>
  </Card.Header>
  <Card.Content>
    {#if providers.length === 0}
      <p class="text-sm text-muted-foreground">None yet.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Name</Table.Head>
            {#if kind !== 'xmltv'}<Table.Head class="text-right">Tuners</Table.Head>{/if}
            {#each stats as stat (stat.key)}
              <Table.Head class="text-right" title={stat.hint}>{stat.label}</Table.Head>
            {/each}
            <Table.Head>Last update</Table.Head>
            <Table.Head>Availability</Table.Head>
            <Table.Head class="w-0"><span class="sr-only">Actions</span></Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each providers as [id, p] (id)}
            <Table.Row>
              <Table.Cell class="max-w-80">
                <div class="font-medium">{p.name}</div>
                <div class="truncate text-xs text-muted-foreground" title={p['file.source']}>{p['file.source']}</div>
                {#if p.description}<div class="text-xs text-muted-foreground">{p.description}</div>{/if}
              </Table.Cell>
              {#if kind !== 'xmltv'}<Table.Cell class="text-right tabular-nums">{p.tuner}</Table.Cell>{/if}
              {#each stats as stat (stat.key)}
                <Table.Cell class="text-right tabular-nums">
                  {p.compatibility?.[stat.key] ?? '–'}{stat.percent && p.compatibility?.[stat.key] !== undefined
                    ? ' %'
                    : ''}
                </Table.Cell>
              {/each}
              <Table.Cell class="text-sm whitespace-nowrap">{p['last.update']}</Table.Cell>
              <Table.Cell>
                <Badge variant={availability(p).variant}>{availability(p).value} %</Badge>
              </Table.Cell>
              <Table.Cell>
                <div class="flex justify-end gap-1">
                  {#each [{ label: 'Download again', icon: RefreshCw, run: () => refresh(kind, id) }, { label: 'Edit', icon: Pencil, run: () => openEdit(id, p) }, { label: 'Delete', icon: Trash2, run: () => (deleting = { id, provider: p }) }] as action (action.label)}
                    <Tooltip.Root>
                      <Tooltip.Trigger>
                        {#snippet child({ props })}
                          <Button
                            {...props}
                            variant="ghost"
                            size="icon"
                            aria-label="{action.label} {p.name}"
                            disabled={app.busy}
                            onclick={action.run}
                          >
                            <action.icon />
                          </Button>
                        {/snippet}
                      </Tooltip.Trigger>
                      <Tooltip.Content>{action.label}</Tooltip.Content>
                    </Tooltip.Root>
                  {/each}
                </div>
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>

<ProviderDialog bind:open={dialogOpen} {kind} provider={editing?.provider} {save} />

<AlertDialog.Root open={deleting !== undefined} onOpenChange={(open) => !open && (deleting = undefined)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Delete {deleting?.provider.name}?</AlertDialog.Title>
      <AlertDialog.Description>
        The local copy is deleted, and its channels leave the database at the next update.
      </AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={confirmDelete} disabled={app.busy}>Delete</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
