import type { Channels, Mapping, Providers, XEPGChannel } from './api/types'

/** The "XMLTV file" of the dummy programs. */
export const dummyFile = 'xTeVe Dummy'

/** The durations of dummy programs, as values of x-mapping for the dummy file. */
export const dummyDurations = ['30', '60', '90', '120', '180', '240', '360'].map((m) => ({
  value: `${m}_Minutes`,
  label: `${m} minutes`,
}))

/** EPG categories xTeVe adds to the programs of a channel. */
export const categories = [
  { value: '', label: 'None' },
  { value: 'Kids', label: 'Kids (Emby only)' },
  { value: 'News', label: 'News' },
  { value: 'Movie', label: 'Movie' },
  { value: 'Series', label: 'Series' },
  { value: 'Sports', label: 'Sports' },
]

/** The XMLTV files a channel can be mapped to: x-xmltv-file values with their names. */
export function xmltvFiles(mapping: Mapping, providers: Providers | null) {
  return Object.keys(mapping)
    .map((file) => ({
      value: file,
      label: file === dummyFile ? 'Dummy programs' : (providers?.[file.replace(/\.xml$/, '')]?.name ?? file),
    }))
    .sort((a, b) => Number(a.value === dummyFile) - Number(b.value === dummyFile) || a.label.localeCompare(b.label))
}

/** The name of an XMLTV channel for display: its first display name. */
export function xmltvChannelName(mapping: Mapping, file: string, id: string): string {
  if (file === dummyFile) return dummyDurations.find((d) => d.value === id)?.label ?? id
  return mapping[file]?.[id]?.['display-names']?.[0]?.Value ?? id
}

/** A channel can only be active with an XMLTV file and channel. */
export const mappable = (c: XEPGChannel) => c['x-xmltv-file'] !== '-' && c['x-mapping'] !== '-'

/**
 * Returns number, or the next free one if another channel has it: whole numbers count up by 1, numbers with a
 * fraction by 0.1, as the old web UI did.
 */
export function freeChannelNumber(channels: Channels, id: string, number: number): number {
  const taken = new Set(
    Object.entries(channels)
      .filter(([other]) => other !== id)
      .map(([, c]) => parseFloat(c['x-channelID'])),
  )
  while (taken.has(number)) {
    number = Number.isInteger(number) ? number + 1 : Math.round((number + 0.1) * 10) / 10
  }
  return number
}

/** Sorts channel IDs by channel number. */
export function byNumber(channels: Channels) {
  return (a: string, b: string) => parseFloat(channels[a]['x-channelID']) - parseFloat(channels[b]['x-channelID'])
}
