<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import { app, command } from '$lib/state.svelte'

  // The steps of xTeVe's configuration wizard, in its order; xTeVe answers with the next one
  const steps = ['tuner', 'epgSource', 'm3u', 'xmltv'] as const

  const step = $derived(steps[app.server?.wizard ?? 0] ?? 'tuner')

  let tuner = $state(1)
  let epgSource = $state('XEPG')
  let m3u = $state('')
  let xmltv = $state('')

  const sources = [
    { value: 'XEPG', title: 'XEPG', text: 'One or more XMLTV files, channel management, M3U and XMLTV for IPTV apps' },
    { value: 'PMS', title: 'PMS', text: 'The guide of Plex or Emby' },
  ]

  async function next(event: SubmitEvent) {
    event.preventDefault()
    switch (step) {
      case 'tuner':
        await command('saveWizard', { wizard: { tuner } })
        break
      case 'epgSource':
        await command('saveWizard', { wizard: { epgSource } })
        break
      case 'm3u':
        await command('saveWizard', { wizard: { m3u } })
        break
      case 'xmltv':
        await command('saveWizard', { wizard: { xmltv } })
        break
    }
  }
</script>

<main class="flex min-h-svh items-center justify-center bg-muted/40 p-4">
  <Card.Root class="w-full max-w-lg">
    <Card.Header>
      <Card.Description>Set up xTeVe · step {steps.indexOf(step) + 1} of {steps.length}</Card.Description>
      <Card.Title class="text-xl">
        {{ tuner: 'Number of tuners', epgSource: 'EPG source', m3u: 'M3U playlist', xmltv: 'XMLTV file' }[step]}
      </Card.Title>
    </Card.Header>
    <Card.Content>
      <form id="wizard" class="grid gap-3" onsubmit={next}>
        {#if step === 'tuner'}
          <Label for="tuner">Streams at the same time</Label>
          <Input id="tuner" type="number" min="1" max="100" required bind:value={tuner} />
          <p class="text-sm text-muted-foreground">
            How many streams the provider allows at once. Used by Plex and Emby, and for M3U with a buffer. After a
            change, xTeVe has to be removed from the DVR settings of Plex or Emby and set up again.
          </p>
        {:else if step === 'epgSource'}
          <div class="grid gap-2" role="radiogroup" aria-label="EPG source">
            {#each sources as source (source.value)}
              <label
                class="flex cursor-pointer gap-3 rounded-md border p-3 has-checked:border-primary has-checked:bg-muted/50"
              >
                <input type="radio" name="epgSource" value={source.value} bind:group={epgSource} class="mt-1" />
                <span>
                  <span class="font-medium">{source.title}</span>
                  <span class="block text-sm text-muted-foreground">{source.text}</span>
                </span>
              </label>
            {/each}
          </div>
        {:else if step === 'm3u'}
          <Label for="m3u">File path or URL of the playlist</Label>
          <Input id="m3u" required placeholder="http://provider/playlist.m3u" bind:value={m3u} />
          <p class="text-sm text-muted-foreground">A local or remote M3U playlist. It is downloaded now.</p>
        {:else}
          <Label for="xmltv">File path or URL of the guide</Label>
          <Input id="xmltv" required placeholder="http://provider/epg.xml.gz" bind:value={xmltv} />
          <p class="text-sm text-muted-foreground">A local or remote XMLTV file, also gzipped. It is downloaded now.</p>
        {/if}
      </form>
    </Card.Content>
    <Card.Footer>
      <Button type="submit" form="wizard" class="ml-auto" disabled={app.busy}>
        {app.busy ? 'Please wait…' : 'Next'}
      </Button>
    </Card.Footer>
  </Card.Root>
</main>
