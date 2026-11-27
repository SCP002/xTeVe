import { command } from '../state.svelte'
import type { ProviderKind, ProviderUpdate } from './types'

// The commands of the three kinds of providers differ only in their names and keys

function save(kind: ProviderKind, id: string, update: ProviderUpdate) {
  switch (kind) {
    case 'm3u':
      return command('saveFilesM3U', { files: { m3u: { [id]: update } } })
    case 'hdhr':
      return command('saveFilesHDHR', { files: { hdhr: { [id]: update } } })
    case 'xmltv':
      return command('saveFilesXMLTV', { files: { xmltv: { [id]: update } } })
  }
}

/** Downloads a provider again. */
export function refresh(kind: ProviderKind, id: string) {
  switch (kind) {
    case 'm3u':
      return command('updateFileM3U', { files: { m3u: { [id]: {} } } })
    case 'hdhr':
      return command('updateFileHDHR', { files: { hdhr: { [id]: {} } } })
    case 'xmltv':
      return command('updateFileXMLTV', { files: { xmltv: { [id]: {} } } })
  }
}

/** Adds a provider; xTeVe downloads it. */
export const add = (kind: ProviderKind, update: ProviderUpdate) => save(kind, '-', update)

/** Changes a provider; a changed source is downloaded again. */
export async function edit(kind: ProviderKind, id: string, update: ProviderUpdate, sourceChanged: boolean) {
  return (await save(kind, id, update)) && (!sourceChanged || (await refresh(kind, id)))
}

export const remove = (kind: ProviderKind, id: string) => save(kind, id, { delete: true })
