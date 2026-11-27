<script lang="ts">
  import { ModeWatcher } from 'mode-watcher'
  import { onMount } from 'svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { Toaster } from '$lib/components/ui/sonner'
  import { app, start } from '$lib/state.svelte'
  import Login from './screens/Login.svelte'
  import Offline from './screens/Offline.svelte'
  import Shell from './screens/Shell.svelte'
  import Wizard from './screens/Wizard.svelte'

  onMount(start)
</script>

<ModeWatcher />
<Toaster richColors />

{#if app.screen === 'loading'}
  <main class="flex min-h-svh items-center justify-center">
    <LoaderCircle class="size-6 animate-spin text-muted-foreground" aria-label="Loading" />
  </main>
{:else if app.screen === 'login'}
  <Login />
{:else if app.screen === 'first-user'}
  <Login firstUser />
{:else if app.screen === 'wizard'}
  <Wizard />
{:else if app.screen === 'offline'}
  <Offline />
{:else if app.server}
  <Shell />
{/if}
