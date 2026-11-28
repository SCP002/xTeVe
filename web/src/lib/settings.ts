import type { ServerResponse, SettingsUpdate } from './api/types'
import { dummyDurations } from './mapping'

export type SettingKey = keyof SettingsUpdate

type Option = { value: string; label: string }

export type Field = {
  key: SettingKey
  label: string
  description?: string
  /** Only shown when this is true. */
  when?: (s: Record<string, unknown>) => boolean
} & (
  | { kind: 'switch' | 'text' | 'decimal' | 'hours' }
  | { kind: 'number'; min: number; max: number }
  | { kind: 'select'; options: (server: ServerResponse) => Option[]; number?: boolean }
)

const usesBuffer = (s: Record<string, unknown>) => s.buffer !== '-'

/** The settings of the web UI by category. */
export const categories: { id: string; title: string; fields: Field[] }[] = [
  {
    id: 'general',
    title: 'General',
    fields: [
      {
        key: 'epgSource',
        label: 'EPG source',
        kind: 'select',
        description:
          'XEPG: XMLTV files, channel management, and M3U and XMLTV for players. PMS: the guide of Plex or Emby.',
        options: () => [
          { value: 'XEPG', label: 'XEPG' },
          { value: 'PMS', label: 'PMS' },
        ],
      },
      {
        key: 'tuner',
        label: 'Tuners',
        description:
          'Streams at the same time for Plex and Emby, and for M3U with a buffer. After a change, remove xTeVe from the DVR settings of Plex or Emby and set it up again.',
        kind: 'number',
        min: 1,
        max: 100,
      },
      {
        key: 'hostIP',
        label: 'Host IP',
        description: 'The address xTeVe puts into the M3U and XMLTV files.',
        kind: 'select',
        options: (server) => (server.ipAddressesV4Host ?? []).map((ip) => ({ value: ip, label: ip })),
      },
      {
        key: 'tlsMode',
        label: 'HTTPS',
        description: 'Serves xTeVe over HTTPS with its own certificate. The web UI moves to the new address.',
        kind: 'switch',
      },
      {
        key: 'xteveAutoUpdate',
        label: 'Update xTeVe automatically',
        description: 'Installs new versions from GitHub when they are published.',
        kind: 'switch',
      },
      {
        key: 'disallowURLDuplicates',
        label: 'Skip duplicate URLs',
        description: 'A stream whose URL another stream has already is not added.',
        kind: 'switch',
      },
      {
        key: 'clearXMLTVCache',
        label: 'Do not keep XMLTV files in memory',
        description: 'Needs much less memory when idle, but every update of the XEPG database reads the files again.',
        kind: 'switch',
      },
      { key: 'api', label: 'API', description: 'Commands to xTeVe over HTTP at /api/.', kind: 'switch' },
    ],
  },
  {
    id: 'mapping',
    title: 'Mapping',
    fields: [
      {
        key: 'defaultMissingEPG',
        label: 'Channels without guide',
        description: 'Fills channels that have no XMLTV channel with dummy programs.',
        kind: 'select',
        options: () => [{ value: '-', label: 'No dummy programs' }, ...dummyDurations],
      },
      {
        key: 'enableMappedChannels',
        label: 'Activate mapped channels',
        description: 'Channels that get an XMLTV channel are activated automatically.',
        kind: 'switch',
      },
    ],
  },
  {
    id: 'files',
    title: 'Files',
    fields: [
      {
        key: 'update',
        label: 'Update at',
        description:
          'Times of the day (24 hours, e.g. 0800) at which playlists and XMLTV files are downloaded again and a backup is made, separated by commas. Empty for no updates.',
        kind: 'hours',
      },
      {
        key: 'files.update',
        label: 'Update at start',
        description: 'Downloads all playlists, tuners and XMLTV files when xTeVe starts.',
        kind: 'switch',
      },
      { key: 'temp.path', label: 'Temporary files', description: 'The folder of the buffer files.', kind: 'text' },
      {
        key: 'cache.images',
        label: 'Cache images',
        description: 'Keeps the images of the XMLTV files in xTeVe, downloaded in the background.',
        kind: 'switch',
      },
      {
        key: 'xepg.replace.missing.images',
        label: 'Channel logo for programs without image',
        kind: 'switch',
      },
    ],
  },
  {
    id: 'streaming',
    title: 'Streaming',
    fields: [
      {
        key: 'buffer',
        label: 'Buffer',
        description:
          'With a buffer, xTeVe, FFmpeg or VLC connects to the stream and passes it on: short dropouts are bridged, HLS and (with FFmpeg or VLC) RTP work, and each playlist gets its own tuner limit.',
        kind: 'select',
        options: () => [
          { value: '-', label: 'No buffer' },
          { value: 'xteve', label: 'xTeVe' },
          { value: 'ffmpeg', label: 'FFmpeg' },
          { value: 'vlc', label: 'VLC' },
        ],
      },
      {
        key: 'buffer.size.kb',
        label: 'Buffer size',
        description: 'HLS segments smaller than this are passed on whole.',
        kind: 'select',
        number: true,
        when: usesBuffer,
        options: () =>
          [512, 1024, 2048, 3072, 4096, 5120, 6144, 7168, 8192].map((kb) => ({
            value: String(kb),
            label: `${kb / 1024} MB`,
          })),
      },
      {
        key: 'storeBufferInRAM',
        label: 'Buffer in memory',
        description: 'Keeps the buffer in memory instead of the temporary folder.',
        kind: 'switch',
        when: usesBuffer,
      },
      {
        key: 'buffer.timeout',
        label: 'Wait for new clients (ms)',
        description: 'Helps fast channel switching.',
        kind: 'decimal',
        when: usesBuffer,
      },
      {
        key: 'udpxy',
        label: 'UDPxy address',
        description: 'Multicast streams are fetched through this UDPxy server, e.g. 192.168.1.1:4022.',
        kind: 'text',
      },
      {
        key: 'user.agent',
        label: 'User agent',
        description: 'Sent with every request. Only change it if a provider blocks xTeVe.',
        kind: 'text',
      },
      { key: 'ffmpeg.path', label: 'FFmpeg path', kind: 'text', when: (s) => s.buffer === 'ffmpeg' },
      {
        key: 'ffmpeg.options',
        label: 'FFmpeg options',
        description: 'Empty for the defaults.',
        kind: 'text',
        when: (s) => s.buffer === 'ffmpeg',
      },
      { key: 'vlc.path', label: 'VLC path', kind: 'text', when: (s) => s.buffer === 'vlc' },
      {
        key: 'vlc.options',
        label: 'VLC options',
        description: 'Empty for the defaults.',
        kind: 'text',
        when: (s) => s.buffer === 'vlc',
      },
    ],
  },
  {
    id: 'backup',
    title: 'Backup',
    fields: [
      {
        key: 'backup.path',
        label: 'Backup folder',
        description: 'xTeVe makes a backup before every scheduled update and needs write access here.',
        kind: 'text',
      },
      {
        key: 'backup.keep',
        label: 'Backups to keep',
        kind: 'select',
        number: true,
        options: () => [5, 10, 20, 30, 40, 50].map((n) => ({ value: String(n), label: String(n) })),
      },
    ],
  },
  {
    id: 'authentication',
    title: 'Login',
    fields: [
      { key: 'authentication.web', label: 'Web UI', description: 'The web UI needs a login.', kind: 'switch' },
      {
        key: 'authentication.pms',
        label: 'Plex / Emby',
        description:
          'Plex and Emby need a login. Afterwards, remove xTeVe from their DVR settings and set it up again.',
        kind: 'switch',
      },
      { key: 'authentication.m3u', label: 'M3U', description: 'xteve.m3u needs a login.', kind: 'switch' },
      { key: 'authentication.xml', label: 'XMLTV', description: 'xteve.xml needs a login.', kind: 'switch' },
      { key: 'authentication.api', label: 'API', description: 'The API needs a login.', kind: 'switch' },
    ],
  },
]

/** The value of a setting as the form shows it. */
export function formValue(field: Field, value: unknown): unknown {
  if (field.kind === 'hours') return ((value as string[] | null) ?? []).join(', ')
  if (field.kind === 'select' || field.kind === 'decimal') return String(value ?? '')
  return value
}

/** The value of a setting as xTeVe takes it. */
export function settingValue(field: Field, value: unknown): unknown {
  switch (field.kind) {
    case 'hours':
      return String(value)
        .split(/[\s,]+/)
        .filter(Boolean)
    case 'decimal':
      return parseFloat(String(value)) || 0
    case 'number':
      return Number(value)
    case 'select':
      return field.number ? Number(value) : value
    default:
      return value
  }
}

/** Checks the update times (HHMM). */
export const validHours = (text: string) =>
  text
    .split(/[\s,]+/)
    .filter(Boolean)
    .every((h) => /^([01]\d|2[0-3])[0-5]\d$/.test(h))
