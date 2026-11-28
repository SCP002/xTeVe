// The messages of the WebSocket /data/ of xTeVe, as the Go structs in src/struct-webserver.go and src/internal
// encode them. Field names are the JSON names of Go.

/** An M3U playlist, HDHomeRun tuner or XMLTV file (config.Provider). */
export interface Provider {
  'id.provider': string
  name: string
  description: string
  type: string
  /** The local copy in the data folder. */
  'file.xteve': string
  'file.source': string
  /** Streams at a time, not used for XMLTV files. */
  tuner?: number
  'last.update': string
  compatibility: Record<string, number> | null
  'counter.error': number
  'counter.download': number
  'provider.availability': number
  /** Added and not downloaded yet. */
  new?: boolean
}

export type Providers = Record<string, Provider>

export type ProviderKind = 'm3u' | 'hdhr' | 'xmltv'

/** A change of a provider: only the fields that are set change (config.ProviderUpdate). */
export interface ProviderUpdate {
  name?: string
  description?: string
  'file.source'?: string
  tuner?: number
  delete?: boolean
}

/** A filter as saved in the settings (config.FilterSettings). */
export interface Filter {
  caseSensitive: boolean
  preserveMapping: boolean
  description: string
  exclude: string
  filter: string
  include: string
  name: string
  rule?: string
  /** "group-title" or "custom-filter" */
  type: string
  startingChannel: string
}

/** A change of a filter (config.FilterUpdate). */
export type FilterUpdate = Partial<Omit<Filter, 'rule'>> & { delete?: boolean }

/** settings.json (config.Settings). */
export interface Settings {
  api: boolean
  'authentication.api': boolean
  'authentication.m3u': boolean
  'authentication.pms': boolean
  'authentication.web': boolean
  'authentication.xml': boolean
  'backup.keep': number
  'backup.path': string
  'git.branch'?: string
  /** "-" (none), "xteve", "ffmpeg" or "vlc" */
  buffer: string
  'buffer.size.kb': number
  'buffer.timeout': number
  'cache.images': boolean
  clearXMLTVCache: boolean
  /** "-" or a dummy duration like "60_Minutes" */
  defaultMissingEPG: string
  disallowURLDuplicates: boolean
  enableMappedChannels: boolean
  /** "PMS" or "XEPG" */
  epgSource: string
  'ffmpeg.options': string
  'ffmpeg.path': string
  'vlc.options': string
  'vlc.path': string
  files: { hdhr: Providers | null; m3u: Providers | null; xmltv: Providers | null }
  'files.update': boolean
  filter: Record<string, Filter> | null
  hostIP: string
  language: string
  'log.entries.ram': number
  'm3u8.adaptive.bandwidth.mbps': number
  'mapping.first.channel': number
  port: string
  ssdp: boolean
  storeBufferInRAM: boolean
  'temp.path': string
  tlsMode: boolean
  tuner: number
  /** Hours of the day ("0000" to "2300") at which the files are updated. */
  update: string[] | null
  'user.agent': string
  uuid: string
  udpxy: string
  version: string
  'xepg.replace.missing.images': boolean
  xteveAutoUpdate: boolean
}

/** The settings the web UI can change with saveSettings (RequestStruct.Settings). */
export type SettingsUpdate = Partial<
  Pick<
    Settings,
    | 'api'
    | 'authentication.api'
    | 'authentication.m3u'
    | 'authentication.pms'
    | 'authentication.web'
    | 'authentication.xml'
    | 'backup.keep'
    | 'backup.path'
    | 'buffer'
    | 'buffer.size.kb'
    | 'buffer.timeout'
    | 'cache.images'
    | 'clearXMLTVCache'
    | 'defaultMissingEPG'
    | 'disallowURLDuplicates'
    | 'enableMappedChannels'
    | 'epgSource'
    | 'ffmpeg.options'
    | 'ffmpeg.path'
    | 'vlc.options'
    | 'vlc.path'
    | 'files.update'
    | 'hostIP'
    | 'temp.path'
    | 'tlsMode'
    | 'tuner'
    | 'udpxy'
    | 'update'
    | 'user.agent'
    | 'xepg.replace.missing.images'
    | 'xteveAutoUpdate'
    | 'storeBufferInRAM'
  >
> & { 'scheme.m3u'?: string; 'scheme.xml'?: string }

/** A channel of the XEPG database (epg.XEPGChannel). */
export interface XEPGChannel {
  '_file.m3u.id': string
  '_file.m3u.name': string
  '_file.m3u.path': string
  'group-title': string
  name: string
  'tvg-id': string
  'tvg-logo': string
  'tvg-name': string
  'tvg-shift': string
  'update-channel-name-regex': string
  'update-channel-name-by-group-regex': string
  url: string
  '_uuid.key': string
  '_uuid.value'?: string
  _values: string
  'x-active': boolean
  'x-category': string
  'x-channelID': string
  'x-epg': string
  'x-group-title': string
  /** The XMLTV channel ID, or the dummy duration for the "xTeVe Dummy" file. */
  'x-mapping': string
  /** The XMLTV file ("X…xml", "xTeVe Dummy" or "-"). */
  'x-xmltv-file': string
  'x-name': string
  'x-update-channel-icon': boolean
  'x-update-channel-name': boolean
  'x-update-channel-group': boolean
  'x-description': string
  'x-timeshift': string
}

/** XEPG ID ("x-ID.12") -> channel. */
export type Channels = Record<string, XEPGChannel>

export interface DisplayName {
  Lang: string
  Value: string
}

/** An XMLTV channel of the mapping menu (epg.MappingChannel). */
export interface MappingChannel {
  id: string
  'display-names': DisplayName[] | null
  icon: string
}

/** XMLTV file -> XMLTV channel ID -> channel. */
export type Mapping = Record<string, Record<string, MappingChannel>>

export interface UserData {
  username?: string
  defaultUser?: boolean
  'authentication.web': boolean
  'authentication.pms': boolean
  'authentication.m3u': boolean
  'authentication.xml': boolean
  'authentication.api': boolean
}

/** A user as the web UI sees it, without hashes (authentication.PublicUser). */
export interface PublicUser {
  data: UserData
}

/** A user as saveUserData and saveNewUser take it (UserForm). */
export interface UserForm extends UserData {
  password?: string
  confirm?: string
  delete?: boolean
}

export interface Notification {
  headline: string
  message: string
  new: boolean
  time: string
  /** "info", "warning" or "error" */
  type: string
}

export interface ClientInfo {
  arch: string
  branch?: string
  DVR: string
  epgSource: string
  errors: number
  'm3u-url': string
  os: string
  /** "active / all" */
  streams: string
  uuid: string
  version: string
  warnings: number
  xepg: number
  'xepg-url': string
}

/** The answer to every command (ResponseStruct). */
export interface ServerResponse {
  clientInfo: ClientInfo
  data: {
    playlist: { m3u: { groups: { text: string[] | null; value: string[] | null } } }
    StreamPreviewUI: { activeStreams: string[] | null; inactiveStreams: string[] | null }
  }
  alert?: string
  configurationWizard: boolean
  err?: string
  ipAddressesV4Host: string[] | null
  log: { errors: number; log: string[] | null; warnings: number }
  logoURL?: string
  openLink?: string
  openMenu?: string
  reload?: boolean
  settings: Settings
  status: boolean
  token?: string
  users?: Record<string, PublicUser>
  /** The next step of the configuration wizard. */
  wizard: number
  xepg: { epgMapping: Channels; xmltvMap: Mapping } | null
  notification?: Record<string, Notification>
}

/** The commands and what they take besides "cmd" (RequestStruct). */
export interface Commands {
  getServerConfig: Record<string, never>
  updateLog: Record<string, never>
  loadFiles: Record<string, never>
  saveSettings: { settings: SettingsUpdate }
  saveFilesM3U: { files: { m3u: Record<string, ProviderUpdate> } }
  updateFileM3U: { files: { m3u: Record<string, ProviderUpdate> } }
  saveFilesHDHR: { files: { hdhr: Record<string, ProviderUpdate> } }
  updateFileHDHR: { files: { hdhr: Record<string, ProviderUpdate> } }
  saveFilesXMLTV: { files: { xmltv: Record<string, ProviderUpdate> } }
  updateFileXMLTV: { files: { xmltv: Record<string, ProviderUpdate> } }
  saveFilter: { filter: Record<string, FilterUpdate> }
  saveEpgMapping: { epgMapping: Channels }
  saveUserData: { userData: Record<string, UserForm>; deleteUser?: boolean }
  saveNewUser: { userData: UserForm }
  resetLogs: Record<string, never>
  xteveBackup: Record<string, never>
  xteveRestore: { base64: string }
  uploadLogo: { base64: string; filename: string }
  saveWizard: { wizard: { epgSource?: string; m3u?: string; tuner?: number; xmltv?: string } }
}

export type Command = keyof Commands

/** /web/api/session */
export interface Session {
  authentication: boolean
  firstUser: boolean
  loggedIn: boolean
  wizard: boolean
}
