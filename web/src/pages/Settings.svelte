<script lang="ts">
  import Download from '@lucide/svelte/icons/download'
  import Upload from '@lucide/svelte/icons/upload'
  import { toast } from 'svelte-sonner'
  import * as AlertDialog from '$lib/components/ui/alert-dialog'
  import { Button } from '$lib/components/ui/button'
  import * as Card from '$lib/components/ui/card'
  import { Input } from '$lib/components/ui/input'
  import { Label } from '$lib/components/ui/label'
  import * as Select from '$lib/components/ui/select'
  import { Switch } from '$lib/components/ui/switch'
  import * as Tabs from '$lib/components/ui/tabs'
  import type { SettingsUpdate } from '$lib/api/types'
  import { categories, formValue, settingValue, validHours, type Field } from '$lib/settings'
  import { app, command, server } from '$lib/state.svelte'

  const fields = categories.flatMap((c) => c.fields)

  // The form starts with the settings of xTeVe and follows them while nothing is changed
  let form = $state<Record<string, unknown>>({})
  let dirty = $state(false)
  $effect(() => {
    const values = Object.fromEntries(
      fields.map((f) => [f.key, formValue(f, (server().settings as unknown as Record<string, unknown>)[f.key])]),
    )
    if (!dirty) form = values
  })

  const changes = $derived.by(() => {
    const settings = server().settings as unknown as Record<string, unknown>
    const result: Record<string, unknown> = {}
    for (const f of fields) {
      if (form[f.key] !== formValue(f, settings[f.key])) result[f.key] = settingValue(f, form[f.key])
    }
    return result as SettingsUpdate
  })
  const changed = $derived(Object.keys(changes).length)
  const hoursError = $derived(!validHours(String(form.update ?? '')))

  function set(key: string, value: unknown) {
    form[key] = value
    dirty = true
  }

  async function save() {
    if (hoursError) return
    if (await command('saveSettings', { settings: changes })) {
      dirty = false
      toast.success('Settings saved')
    }
  }

  function discard() {
    dirty = false
  }

  let tab = $state('general')

  function optionLabel(field: Field, value: unknown) {
    if (field.kind !== 'select') return ''
    return field.options(server()).find((o) => o.value === value)?.label ?? String(value)
  }

  // Backup and restore
  let restoreFile = $state<File | undefined>()

  async function restore() {
    const file = restoreFile
    restoreFile = undefined
    if (!file) return
    const base64 = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as string)
      reader.onerror = () => reject(reader.error)
      reader.readAsDataURL(file)
    })
    await command('xteveRestore', { base64 })
  }
</script>

<div class="grid gap-4">
  <div class="flex flex-wrap items-center gap-2">
    <Tabs.Root bind:value={tab}>
      <Tabs.List class="flex-wrap">
        {#each categories as c (c.id)}<Tabs.Trigger value={c.id}>{c.title}</Tabs.Trigger>{/each}
      </Tabs.List>
    </Tabs.Root>
    <div class="ml-auto flex items-center gap-2">
      {#if changed}<span class="text-sm text-muted-foreground">{changed} unsaved</span>{/if}
      <Button variant="outline" size="sm" disabled={!changed || app.busy} onclick={discard}>Discard</Button>
      <Button size="sm" disabled={!changed || hoursError || app.busy} onclick={save}>Save</Button>
    </div>
  </div>

  {#each categories.filter((c) => c.id === tab) as category (category.id)}
    <Card.Root>
      <Card.Content class="grid gap-6">
        {#each category.fields.filter((f) => !f.when || f.when(form)) as field (field.key)}
          <div class="grid gap-2 sm:grid-cols-[16rem_1fr] sm:gap-6">
            <div>
              <Label for="setting-{field.key}">{field.label}</Label>
              {#if field.description}<p class="mt-1 text-sm text-muted-foreground">{field.description}</p>{/if}
            </div>
            <div class="sm:pt-0.5">
              {#if field.kind === 'switch'}
                <Switch
                  id="setting-{field.key}"
                  checked={form[field.key] as boolean}
                  onCheckedChange={(v) => set(field.key, v)}
                />
              {:else if field.kind === 'select'}
                <Select.Root type="single" value={form[field.key] as string} onValueChange={(v) => set(field.key, v)}>
                  <Select.Trigger id="setting-{field.key}" class="w-full max-w-md">
                    {optionLabel(field, form[field.key])}
                  </Select.Trigger>
                  <Select.Content>
                    {#each field.options(server()) as o (o.value)}<Select.Item value={o.value} label={o.label} />{/each}
                  </Select.Content>
                </Select.Root>
              {:else if field.kind === 'number'}
                <Input
                  id="setting-{field.key}"
                  class="max-w-32"
                  type="number"
                  min={field.min}
                  max={field.max}
                  value={form[field.key] as number}
                  oninput={(e) => set(field.key, e.currentTarget.valueAsNumber)}
                />
              {:else}
                <Input
                  id="setting-{field.key}"
                  class="max-w-md"
                  inputmode={field.kind === 'decimal' ? 'decimal' : undefined}
                  placeholder={field.kind === 'hours' ? '0000, 1200' : undefined}
                  aria-invalid={field.kind === 'hours' && hoursError}
                  value={form[field.key] as string}
                  oninput={(e) => set(field.key, e.currentTarget.value)}
                />
                {#if field.kind === 'hours' && hoursError}
                  <p class="mt-1 text-sm text-destructive">Times are four digits like 0800 or 2330.</p>
                {/if}
              {/if}
            </div>
          </div>
        {/each}
      </Card.Content>
    </Card.Root>

    {#if category.id === 'backup'}
      <Card.Root>
        <Card.Header>
          <Card.Title>Backup and restore</Card.Title>
          <Card.Description>
            A backup has the settings, users, mapping and the local copies of the files.
          </Card.Description>
        </Card.Header>
        <Card.Content class="flex flex-wrap gap-2">
          <Button variant="outline" disabled={app.busy} onclick={() => command('xteveBackup', {})}>
            <Download /> Download backup
          </Button>
          <Button
            variant="outline"
            disabled={app.busy}
            onclick={() => document.getElementById('restore-file')?.click()}
          >
            <Upload /> Restore backup…
          </Button>
          <input
            id="restore-file"
            type="file"
            accept=".zip"
            hidden
            onchange={(e) => {
              restoreFile = e.currentTarget.files?.[0]
              e.currentTarget.value = ''
            }}
          />
        </Card.Content>
      </Card.Root>
    {/if}
  {/each}
</div>

<AlertDialog.Root open={restoreFile !== undefined} onOpenChange={(open) => !open && (restoreFile = undefined)}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>Restore {restoreFile?.name}?</AlertDialog.Title>
      <AlertDialog.Description>All data of xTeVe is replaced with the backup.</AlertDialog.Description>
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action onclick={restore}>Restore</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
