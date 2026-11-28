<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { app, command, server } from '$lib/state.svelte'

  const entries = $derived(
    (server().log.log ?? []).map((text) => {
      // Info entries are aligned with non-breaking spaces (&nbsp;)
      text = text.replaceAll('&nbsp;', ' ')
      const kind = text.includes('[ERROR]') ? 'error' : text.includes('[WARNING]') ? 'warning' : 'info'
      return { text, kind }
    }),
  )

  // The newest entries are at the end, shown first
  const newest = $derived([...entries].reverse())

  // The log is refreshed while the page is open
  const timer = setInterval(() => {
    if (!app.busy) void command('updateLog', {})
  }, 5000)
  onDestroy(() => clearInterval(timer))
</script>

<div class="mb-4 flex items-center gap-4 text-sm text-muted-foreground">
  <span>{entries.length} entries</span>
  <span class="text-destructive">{server().log.errors} errors</span>
  <span class="text-amber-600 dark:text-amber-400">{server().log.warnings} warnings</span>
  <Button class="ml-auto" variant="outline" size="sm" onclick={() => command('resetLogs', {})}>Clear log</Button>
</div>

<div class="overflow-x-auto rounded-md border bg-muted/30 p-3 font-mono text-xs leading-relaxed">
  {#each newest as entry, i (i)}
    <div
      class={[
        'whitespace-pre',
        entry.kind === 'error' && 'text-destructive',
        entry.kind === 'warning' && 'text-amber-600 dark:text-amber-400',
      ]}
    >
      {entry.text}
    </div>
  {:else}
    <p class="text-muted-foreground">The log is empty.</p>
  {/each}
</div>
