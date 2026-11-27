<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil'
  import Plus from '@lucide/svelte/icons/plus'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import * as Alert from '$lib/components/ui/alert'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import { Badge } from '$lib/components/ui/badge'
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import * as Table from '$lib/components/ui/table'
  import type { UserData, UserForm } from '$lib/api/types'
  import { href } from '$lib/router.svelte'
  import { app, command, server } from '$lib/state.svelte'
  import { allowed } from '$lib/users'
  import UserDialog from '../components/UserDialog.svelte'

  const users = $derived(
    Object.entries(server().users ?? {})
      .map(([id, u]) => ({ id, data: u.data }))
      .sort(
        (a, b) =>
          Number(b.data.defaultUser) - Number(a.data.defaultUser) ||
          (a.data.username ?? '').localeCompare(b.data.username ?? ''),
      ),
  )
  // Which parts of xTeVe need a login at all
  const settings = $derived(server().settings)
  const loginFor = $derived(
    (
      [
        ['authentication.web', 'web UI'],
        ['authentication.pms', 'Plex / Emby'],
        ['authentication.m3u', 'M3U'],
        ['authentication.xml', 'XMLTV'],
        ['authentication.api', 'API'],
      ] as const
    )
      .filter(([key]) => settings[key])
      .map(([, label]) => label),
  )

  let dialogOpen = $state(false)
  let editing = $state<{ id: string; data: UserData } | undefined>()
  let deleting = $state<{ id: string; data: UserData } | undefined>()

  function openDialog(user?: { id: string; data: UserData }) {
    editing = user
    dialogOpen = true
  }

  const save = (form: UserForm) =>
    editing ? command('saveUserData', { userData: { [editing.id]: form } }) : command('saveNewUser', { userData: form })

  async function confirmDelete() {
    if (
      deleting &&
      (await command('saveUserData', { userData: { [deleting.id]: { ...deleting.data, delete: true } } }))
    ) {
      deleting = undefined
    }
  }
</script>

<div class="grid gap-4">
  {#if loginFor.length === 0}
    <Alert.Root>
      <Alert.Title>No login is required</Alert.Title>
      <Alert.Description>
        <p>
          Users only matter once a login is turned on in the <a class="underline" href={href('settings')}>settings</a>.
        </p>
      </Alert.Description>
    </Alert.Root>
  {/if}

  <Card.Root>
    <Card.Header>
      <Card.Title>Users</Card.Title>
      <Card.Description>
        {loginFor.length ? `A login is required for: ${loginFor.join(', ')}.` : 'Nothing requires a login now.'}
      </Card.Description>
      <Card.Action>
        <Button size="sm" onclick={() => openDialog()} disabled={app.busy}><Plus /> Add</Button>
      </Card.Action>
    </Card.Header>
    <Card.Content>
      {#if users.length === 0}
        <p class="text-sm text-muted-foreground">No users yet. The first one is created at the first login.</p>
      {:else}
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Name</Table.Head>
              <Table.Head>May use</Table.Head>
              <Table.Head class="w-0"><span class="sr-only">Actions</span></Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each users as user (user.id)}
              <Table.Row>
                <Table.Cell class="font-medium">
                  {user.data.username}
                  {#if user.data.defaultUser}<Badge variant="secondary" class="ml-2">First user</Badge>{/if}
                </Table.Cell>
                <Table.Cell>
                  <div class="flex flex-wrap gap-1">
                    {#each allowed(user.data) as p (p.key)}<Badge variant="outline">{p.label}</Badge>{:else}
                      <span class="text-sm text-muted-foreground">Nothing</span>
                    {/each}
                  </div>
                </Table.Cell>
                <Table.Cell>
                  <div class="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Edit {user.data.username}"
                      disabled={app.busy}
                      onclick={() => openDialog(user)}
                    >
                      <Pencil />
                    </Button>
                    {#if !user.data.defaultUser}
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label="Delete {user.data.username}"
                        disabled={app.busy}
                        onclick={() => (deleting = user)}
                      >
                        <Trash2 />
                      </Button>
                    {/if}
                  </div>
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>
</div>

<UserDialog bind:open={dialogOpen} user={editing?.data} {save} />

<AlertDialog.Root open={deleting !== undefined} onOpenChange={(open) => !open && (deleting = undefined)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Delete the user {deleting?.data.username}?</AlertDialog.Title>
      <AlertDialog.Description>Plex, players or scripts using this login stop working.</AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={confirmDelete} disabled={app.busy}>Delete</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
