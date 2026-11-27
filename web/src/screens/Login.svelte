<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import { createFirstUser, login } from '$lib/api/server'
  import { start } from '$lib/state.svelte'

  /** firstUser: no user exists yet, the form creates the first one. */
  let { firstUser = false }: { firstUser?: boolean } = $props()

  let username = $state('')
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let sending = $state(false)

  const mismatch = $derived(firstUser && confirm !== '' && confirm !== password)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (mismatch) return
    error = ''
    sending = true
    try {
      if (firstUser) await createFirstUser(username, password, confirm)
      else await login(username, password)
      await start()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      sending = false
    }
  }
</script>

<main class="flex min-h-svh items-center justify-center bg-muted/40 p-4">
  <Card.Root class="w-full max-w-sm">
    <Card.Header>
      <Card.Title class="text-xl">{firstUser ? 'Create an account' : 'Log in to xTeVe'}</Card.Title>
      <Card.Description>
        {firstUser
          ? 'The web UI needs a login. This first user may use all parts of xTeVe.'
          : 'Enter the name and password of your xTeVe user.'}
      </Card.Description>
    </Card.Header>
    <Card.Content>
      <form id="login" class="grid gap-4" onsubmit={submit}>
        <div class="grid gap-2">
          <Label for="username">Username</Label>
          <Input id="username" autocomplete="username" required bind:value={username} />
        </div>
        <div class="grid gap-2">
          <Label for="password">Password</Label>
          <Input
            id="password"
            type="password"
            autocomplete={firstUser ? 'new-password' : 'current-password'}
            required
            bind:value={password}
          />
        </div>
        {#if firstUser}
          <div class="grid gap-2">
            <Label for="confirm">Confirm password</Label>
            <Input
              id="confirm"
              type="password"
              autocomplete="new-password"
              required
              aria-invalid={mismatch}
              bind:value={confirm}
            />
            {#if mismatch}
              <p class="text-sm text-destructive">The passwords do not match.</p>
            {/if}
          </div>
        {/if}
        {#if error}
          <p class="text-sm text-destructive" role="alert">{error}</p>
        {/if}
      </form>
    </Card.Content>
    <Card.Footer>
      <Button type="submit" form="login" class="w-full" disabled={sending || mismatch}>
        {firstUser ? 'Create account' : 'Log in'}
      </Button>
    </Card.Footer>
  </Card.Root>
</main>
