import type { UserData } from './api/types'

export type Permission =
  'authentication.web' | 'authentication.pms' | 'authentication.m3u' | 'authentication.xml' | 'authentication.api'

/** What a user may use, as the settings of the same names turn the login on for it. */
export const permissions: { key: Permission; label: string; text: string }[] = [
  { key: 'authentication.web', label: 'Web UI', text: 'This web UI' },
  { key: 'authentication.pms', label: 'Plex / Emby', text: 'The HDHomeRun tuner for Plex and Emby' },
  { key: 'authentication.m3u', label: 'M3U', text: 'The M3U playlist of xTeVe' },
  { key: 'authentication.xml', label: 'XMLTV', text: 'The XMLTV guide of xTeVe' },
  { key: 'authentication.api', label: 'API', text: 'The API at /api/' },
]

export const allowed = (data: UserData) => permissions.filter((p) => data[p.key])
