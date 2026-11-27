<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import { Switch } from '$lib/components/ui/switch'
  import type { UserData, UserForm } from '$lib/api/types'
  import { app } from '$lib/state.svelte'
  import { permissions, type Permission } from '$lib/users'

  let {
    open = $bindable(false),
    user,
    save,
  }: {
    open?: boolean
    /** The user to edit, undefined to add one. */
    user?: UserData
    save: (form: UserForm) => Promise<boolean>
  } = $props()

  let username = $state('')
  let password = $state('')
  let confirm = $state('')
  // Every permission starts with a value: a bound switch can not take undefined
  const none = () => Object.fromEntries(permissions.map((p) => [p.key, false])) as Record<Permission, boolean>
  let granted = $state(none())

  $effect(() => {
    if (!open) return
    username = user?.username ?? ''
    password = ''
    confirm = ''
    granted = Object.fromEntries(
      permissions.map((p) => [p.key, user ? user[p.key] : p.key !== 'authentication.api']),
    ) as Record<Permission, boolean>
  })

  const mismatch = $derived(confirm !== '' && confirm !== password)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (mismatch || password !== confirm) return
    const form: UserForm = { username, ...granted }
    if (password) Object.assign(form, { password, confirm })
    if (await save(form)) open = false
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>{user ? `Edit ${user.username}` : 'Add user'}</Dialog.Title>
      <Dialog.Description>
        {user ? 'Leave the password empty to keep it.' : 'The user logs in with this name and password.'}
      </Dialog.Description>
    </Dialog.Header>
    <form id="user" class="grid gap-4" onsubmit={submit}>
      <div class="grid gap-2">
        <Label for="user-name">Username</Label>
        <Input id="user-name" autocomplete="off" required bind:value={username} />
      </div>
      <div class="grid gap-2">
        <Label for="user-password">{user ? 'New password' : 'Password'}</Label>
        <Input id="user-password" type="password" autocomplete="new-password" required={!user} bind:value={password} />
      </div>
      <div class="grid gap-2">
        <Label for="user-confirm">Confirm password</Label>
        <Input
          id="user-confirm"
          type="password"
          autocomplete="new-password"
          required={!user || password !== ''}
          aria-invalid={mismatch}
          bind:value={confirm}
        />
        {#if mismatch}<p class="text-sm text-destructive">The passwords do not match.</p>{/if}
      </div>
      <fieldset class="grid gap-3">
        <legend class="mb-2 text-sm font-medium">May use</legend>
        {#each permissions as p (p.key)}
          {@const locked = p.key === 'authentication.web' && user?.defaultUser}
          <label class="flex items-center justify-between gap-4 text-sm">
            <span>
              {p.label}
              <span class="block text-muted-foreground">
                {locked ? 'The first user always keeps the web UI.' : p.text}
              </span>
            </span>
            <Switch bind:checked={granted[p.key]} disabled={locked} />
          </label>
        {/each}
      </fieldset>
    </form>
    <Dialog.Footer>
      <Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
      <Button type="submit" form="user" disabled={app.busy || mismatch}>{app.busy ? 'Please wait…' : 'Save'}</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
