<script lang="ts">
  import Upload from '@lucide/svelte/icons/upload'
  import { toast } from 'svelte-sonner'
  import { Button } from '$lib/components/ui/button'
  import * as Dialog from '$lib/components/ui/dialog'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import * as Select from '$lib/components/ui/select'
  import { Switch } from '$lib/components/ui/switch'
  import { send } from '$lib/api/server'
  import type { Channels, Mapping, Providers, XEPGChannel } from '$lib/api/types'
  import { categories, dummyFile, xmltvFiles } from '$lib/mapping'
  import XmltvChannelPicker from './XmltvChannelPicker.svelte'

  type Field = keyof XEPGChannel
  type Changes = Partial<XEPGChannel>

  let {
    open = $bindable(false),
    ids,
    channels,
    mapping,
    providers,
    apply,
  }: {
    open?: boolean
    /** The channels to edit: one, or several selected ones. */
    ids: string[]
    channels: Channels
    mapping: Mapping
    providers: Providers | null
    /** Applies the changed fields to the channels. */
    apply: (ids: string[], changes: Changes) => void
  } = $props()

  const bulk = $derived(ids.length > 1)
  const first = $derived(channels[ids[0]])

  // The form starts with the values of the first channel; only fields that were changed are applied
  let form = $state({} as XEPGChannel)
  let changed = $state(new Set<Field>())
  let uploading = $state(false)

  $effect(() => {
    if (!open || !first) return
    form = { ...first }
    changed = new Set()
  })

  function set<F extends Field>(field: F, value: XEPGChannel[F]) {
    form[field] = value
    changed = new Set(changed).add(field)
  }

  const files = $derived(xmltvFiles(mapping, providers))
  const fileLabel = $derived(files.find((f) => f.value === form['x-xmltv-file'])?.label ?? 'None')
  const categoryLabel = $derived(categories.find((c) => c.value === form['x-category'])?.label ?? 'None')

  // The icon of the chosen XMLTV channel, used when the logo follows the XMLTV file
  const xmltvIcon = $derived(mapping[form['x-xmltv-file']]?.[form['x-mapping']]?.icon ?? '')

  function chooseFile(file: string) {
    set('x-xmltv-file', file)
    // The channel with the tvg-id of the stream, if the file has it
    const tvgID = first?.['tvg-id'] ?? ''
    chooseChannel(file !== dummyFile && tvgID && mapping[file]?.[tvgID] ? tvgID : '-')
  }

  function chooseChannel(id: string) {
    set('x-mapping', id)
    // A mapped channel is active, an unmapped one can not be
    set('x-active', id !== '-' && form['x-xmltv-file'] !== '-')
    if (form['x-xmltv-file'] === dummyFile) set('x-update-channel-icon', false)
    else if (form['x-update-channel-icon'] && xmltvIcon) set('tvg-logo', xmltvIcon)
  }

  function toggleGroupUpdate(on: boolean) {
    set('x-update-channel-group', on)
    if (on) set('x-group-title', first['group-title'])
  }

  function toggleIconUpdate(on: boolean) {
    set('x-update-channel-icon', on)
    if (on && xmltvIcon) set('tvg-logo', xmltvIcon)
  }

  async function upload(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    uploading = true
    try {
      const base64 = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve(reader.result as string)
        reader.onerror = () => reject(reader.error)
        reader.readAsDataURL(file)
      })
      // The answer of uploadLogo carries only the URL, not the state of xTeVe
      const response = await send('uploadLogo', { base64, filename: file.name })
      if (!response.status || !response.logoURL) throw new Error(response.err ?? 'The logo was not saved')
      set('tvg-logo', response.logoURL)
      set('x-update-channel-icon', false)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
    } finally {
      uploading = false
      input.value = ''
    }
  }

  const activeError = $derived(form['x-active'] && (form['x-xmltv-file'] === '-' || form['x-mapping'] === '-'))

  function submit(event: SubmitEvent) {
    event.preventDefault()
    if (activeError) return
    const changes: Changes = {}
    for (const field of changed) Object.assign(changes, { [field]: form[field] })
    apply(ids, changes)
    open = false
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
    <Dialog.Header>
      <Dialog.Title>{bulk ? `Edit ${ids.length} channels` : `Edit ${first?.['x-name']}`}</Dialog.Title>
      <Dialog.Description>
        {bulk
          ? 'Only the fields you change are applied to all selected channels.'
          : `Stream "${first?.name}" of ${first?.['_file.m3u.name']}`}
      </Dialog.Description>
    </Dialog.Header>

    {#if first}
      <form id="channel" class="grid gap-4 sm:grid-cols-2" onsubmit={submit}>
        <div class="grid gap-2">
          <Label for="channel-file">XMLTV file</Label>
          <Select.Root type="single" value={form['x-xmltv-file']} onValueChange={chooseFile}>
            <Select.Trigger id="channel-file" class="w-full">{fileLabel}</Select.Trigger>
            <Select.Content>
              <Select.Item value="-" label="None" />
              {#each files as f (f.value)}<Select.Item value={f.value} label={f.label} />{/each}
            </Select.Content>
          </Select.Root>
        </div>
        <div class="grid gap-2">
          <Label for="channel-xmltv">XMLTV channel</Label>
          {#if form['x-xmltv-file'] === '-'}
            <Input id="channel-xmltv" disabled value="Choose a file first" />
          {:else}
            <XmltvChannelPicker
              id="channel-xmltv"
              file={form['x-xmltv-file']}
              {mapping}
              bind:value={() => form['x-mapping'], chooseChannel}
            />
          {/if}
        </div>

        <label class="flex items-center justify-between gap-4 text-sm sm:col-span-2">
          <span>
            Active
            {#if activeError}<span class="block text-destructive">Needs an XMLTV file and channel.</span>{/if}
          </span>
          <Switch checked={form['x-active']} onCheckedChange={(v) => set('x-active', v)} />
        </label>

        {#if !bulk}
          <div class="grid gap-2 sm:col-span-2">
            <Label for="channel-name">Channel name</Label>
            <Input id="channel-name" value={form['x-name']} oninput={(e) => set('x-name', e.currentTarget.value)} />
          </div>
        {/if}
        <div class="grid gap-2 sm:col-span-2">
          <Label for="channel-description">Description</Label>
          <Input
            id="channel-description"
            value={form['x-description']}
            oninput={(e) => set('x-description', e.currentTarget.value)}
          />
        </div>

        <div class="grid gap-2">
          <Label for="channel-group">Group (in xteve.m3u)</Label>
          <Input
            id="channel-group"
            value={form['x-group-title']}
            disabled={form['x-update-channel-group']}
            oninput={(e) => set('x-group-title', e.currentTarget.value)}
          />
        </div>
        <div class="grid gap-2">
          <Label for="channel-category">EPG category</Label>
          <Select.Root type="single" value={form['x-category']} onValueChange={(v) => set('x-category', v)}>
            <Select.Trigger id="channel-category" class="w-full">{categoryLabel}</Select.Trigger>
            <Select.Content>
              {#each categories as c (c.value)}<Select.Item value={c.value} label={c.label} />{/each}
            </Select.Content>
          </Select.Root>
        </div>
        <label class="flex items-center justify-between gap-4 text-sm sm:col-span-2">
          <span>
            Take the group of the playlist
            {#if !bulk}<span class="block text-muted-foreground">{first['group-title'] || 'No group'}</span>{/if}
          </span>
          <Switch checked={form['x-update-channel-group']} onCheckedChange={toggleGroupUpdate} />
        </label>

        <div class="grid gap-2 sm:col-span-2">
          <Label for="channel-logo">Logo URL</Label>
          <div class="flex gap-2">
            {#if form['tvg-logo']}
              <img src={form['tvg-logo']} alt="" class="size-9 shrink-0 rounded border object-contain" />
            {/if}
            <Input id="channel-logo" value={form['tvg-logo']} oninput={(e) => set('tvg-logo', e.currentTarget.value)} />
            <Button
              variant="outline"
              disabled={uploading}
              onclick={() => document.getElementById('logo-file')?.click()}
            >
              <Upload /> Upload
            </Button>
            <input id="logo-file" type="file" accept=".png,.jpg,.jpeg,.gif,.webp" hidden onchange={upload} />
          </div>
        </div>
        <label class="flex items-center justify-between gap-4 text-sm sm:col-span-2">
          Take the logo of the XMLTV channel
          <Switch
            checked={form['x-update-channel-icon']}
            disabled={form['x-xmltv-file'] === dummyFile}
            onCheckedChange={toggleIconUpdate}
          />
        </label>

        <div class="grid gap-2">
          <Label for="channel-timeshift">Timeshift (hours)</Label>
          <Input
            id="channel-timeshift"
            placeholder="0"
            value={form['x-timeshift']}
            oninput={(e) => set('x-timeshift', e.currentTarget.value)}
          />
        </div>

        {#if first['_uuid.key']}
          <label class="flex items-center justify-between gap-4 text-sm sm:col-span-2">
            Take the name of the playlist when it changes
            <Switch checked={form['x-update-channel-name']} onCheckedChange={(v) => set('x-update-channel-name', v)} />
          </label>
        {/if}
        <div class="grid gap-2">
          <Label for="channel-name-regex">Rename by regex</Label>
          <Input
            id="channel-name-regex"
            value={form['update-channel-name-regex']}
            oninput={(e) => set('update-channel-name-regex', e.currentTarget.value)}
          />
        </div>
        <div class="grid gap-2">
          <Label for="channel-group-regex">Only in groups matching</Label>
          <Input
            id="channel-group-regex"
            value={form['update-channel-name-by-group-regex']}
            oninput={(e) => set('update-channel-name-by-group-regex', e.currentTarget.value)}
          />
        </div>
        <p class="-mt-2 text-sm text-muted-foreground sm:col-span-2">
          On an update of the playlist, the channel takes the first new stream name matching the regex, if its group
          matches the second one.
        </p>
      </form>
    {/if}

    <Dialog.Footer>
      <Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
      <Button type="submit" form="channel" disabled={activeError || changed.size === 0}>Apply</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
