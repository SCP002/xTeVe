<script lang="ts">
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import { Textarea } from '$lib/components/ui/textarea'
  import type { Provider, ProviderKind, ProviderUpdate } from '$lib/api/types'
  import { app } from '$lib/state.svelte'

  let {
    open = $bindable(false),
    kind,
    provider,
    save,
  }: {
    open?: boolean
    kind: ProviderKind
    /** The provider to edit, undefined to add one. */
    provider?: Provider
    save: (update: ProviderUpdate) => Promise<boolean>
  } = $props()

  const texts = {
    m3u: { what: 'playlist', source: 'File path or URL', placeholder: 'http://provider/playlist.m3u' },
    hdhr: { what: 'HDHomeRun tuner', source: 'IP address and port', placeholder: '192.168.1.10:5004' },
    xmltv: { what: 'XMLTV file', source: 'File path or URL', placeholder: 'http://provider/epg.xml.gz' },
  }
  const text = $derived(texts[kind])

  let name = $state('')
  let description = $state('')
  let source = $state('')
  let tuner = $state(1)

  // The form starts with the provider each time it opens
  $effect(() => {
    if (!open) return
    name = provider?.name ?? ''
    description = provider?.description ?? ''
    source = provider?.['file.source'] ?? ''
    tuner = provider?.tuner || 1
  })

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    const update: ProviderUpdate = { name, description, 'file.source': source }
    if (kind !== 'xmltv') update.tuner = tuner
    if (await save(update)) open = false
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-lg">
    <Dialog.Header>
      <Dialog.Title>{provider ? `Edit ${text.what}` : `Add ${text.what}`}</Dialog.Title>
      <Dialog.Description>
        {provider ? 'A changed source is downloaded again.' : 'It is downloaded when you add it.'}
      </Dialog.Description>
    </Dialog.Header>
    <form id="provider" class="grid gap-4" onsubmit={submit}>
      <div class="grid gap-2">
        <Label for="provider-name">Name</Label>
        <Input id="provider-name" required bind:value={name} />
      </div>
      <div class="grid gap-2">
        <Label for="provider-description">Description</Label>
        <Textarea id="provider-description" rows={2} bind:value={description} />
      </div>
      <div class="grid gap-2">
        <Label for="provider-source">{text.source}</Label>
        <Input id="provider-source" required placeholder={text.placeholder} bind:value={source} />
      </div>
      {#if kind !== 'xmltv'}
        <div class="grid gap-2">
          <Label for="provider-tuner">Tuners</Label>
          <Input id="provider-tuner" type="number" min="1" max="100" required bind:value={tuner} />
          <p class="text-sm text-muted-foreground">
            Streams at the same time. Only used with a buffer; applies after all streams of the playlist ended.
          </p>
        </div>
      {/if}
    </form>
    <Dialog.Footer>
      <Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
      <Button type="submit" form="provider" disabled={app.busy}>{app.busy ? 'Please wait…' : 'Save'}</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
