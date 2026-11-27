<script lang="ts">
  import Copy from '@lucide/svelte/icons/copy'
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import { server } from '$lib/state.svelte'

  const info = $derived(server().clientInfo)
  const notifications = $derived(
    Object.values(server().notification ?? {}).sort((a, b) => b.time.localeCompare(a.time)),
  )

  // What Plex, Emby or an IPTV player is given. xTeVe sends the address followed by a hint ("(Specific groups:
  // …)", after spaces or <br>), or in PMS mode only why there is none.
  function address(label: string, text: string) {
    const [value = '', ...rest] = text.replace(/<br>/g, ' ').trim().split(/\s+/)
    const usable = /^https?:\/\/|^[\w.@:-]+:\d+$/.test(value)
    return usable ? { label, value, hint: rest.join(' ') } : { label, value: '', hint: text }
  }

  const addresses = $derived([
    address('HDHomeRun tuner (Plex, Emby)', info.DVR),
    address('M3U playlist', info['m3u-url']),
    address('XMLTV guide', info['xepg-url']),
  ])

  const stats = $derived([
    { label: 'Streams (active / all)', value: info.streams },
    { label: 'EPG source', value: info.epgSource },
    ...(info.epgSource === 'XEPG' ? [{ label: 'XEPG channels', value: String(info.xepg) }] : []),
    { label: 'Errors / warnings', value: `${info.errors} / ${info.warnings}` },
  ])

  async function copy(value: string) {
    await navigator.clipboard.writeText(value)
    toast.success('Copied')
  }
</script>

<div class="grid gap-4">
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
    {#each stats as stat (stat.label)}
      <Card.Root>
        <Card.Header>
          <Card.Description>{stat.label}</Card.Description>
          <Card.Title class="text-2xl tabular-nums">{stat.value}</Card.Title>
        </Card.Header>
      </Card.Root>
    {/each}
  </div>

  <Card.Root>
    <Card.Header>
      <Card.Title>Addresses</Card.Title>
      <Card.Description>Give these to Plex, Emby or an IPTV player.</Card.Description>
    </Card.Header>
    <Card.Content class="grid gap-3">
      {#each addresses as address (address.label)}
        <div class="grid gap-1 sm:grid-cols-[14rem_1fr_auto] sm:items-center">
          <span class="text-sm text-muted-foreground">{address.label}</span>
          {#if address.value}
            <code class="truncate rounded bg-muted px-2 py-1 text-sm">{address.value}</code>
            <Button variant="ghost" size="icon" aria-label="Copy {address.label}" onclick={() => copy(address.value)}>
              <Copy />
            </Button>
          {/if}
          {#if address.hint}
            <span class={['text-sm text-muted-foreground sm:col-start-2', !address.value && 'sm:col-span-2']}>
              {address.hint}
            </span>
          {/if}
        </div>
      {/each}
    </Card.Content>
  </Card.Root>

  <div class="grid gap-4 lg:grid-cols-2">
    <Card.Root>
      <Card.Header><Card.Title>System</Card.Title></Card.Header>
      <Card.Content>
        <dl class="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
          <dt class="text-muted-foreground">Version</dt>
          <dd>{info.version}{info.branch ? ` (${info.branch})` : ''}</dd>
          <dt class="text-muted-foreground">System</dt>
          <dd>{info.os} / {info.arch}</dd>
          <dt class="text-muted-foreground">Device ID</dt>
          <dd class="font-mono text-xs leading-5">{info.uuid}</dd>
        </dl>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>Notifications</Card.Title></Card.Header>
      <Card.Content class="grid gap-3 text-sm">
        {#each notifications as n (n.time)}
          <div>
            <div class={['font-medium', n.type === 'error' && 'text-destructive']}>{n.headline}</div>
            <div class="text-muted-foreground">{n.message}</div>
          </div>
        {:else}
          <p class="text-muted-foreground">No notifications.</p>
        {/each}
      </Card.Content>
    </Card.Root>
  </div>
</div>
