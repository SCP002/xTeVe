<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import { app, start } from '$lib/state.svelte'
</script>

<main class="flex min-h-svh items-center justify-center bg-muted/40 p-4">
  <Card.Root class="w-full max-w-sm">
    <Card.Header>
      <Card.Title class="text-xl">xTeVe can not be reached</Card.Title>
      <Card.Description>{app.offline}</Card.Description>
    </Card.Header>
    <Card.Footer>
      <Button class="w-full" onclick={start}>Try again</Button>
    </Card.Footer>
  </Card.Root>
</main>
