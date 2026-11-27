# Roadmap

Planned work on restoring xTeVe, in the intended order.

## 1. Verify with real data

Checked with a real MPEG-TS playlist (432 channels, PMS mode) and local test sources:

- Buffers: no buffer (redirect), xTeVe, FFmpeg. Sources: MPEG-TS, HLS media and master playlists, a source that
  redirects, a source that drops the connection (reconnects), a dead source (404).
- Tuner limit with the "stream limit" video, two clients sharing one channel, client disconnects (the upstream
  connection and FFmpeg are stopped).
- HDHR emulation: `discover.json`, `lineup.json`, `lineup_status.json`, `device.xml`, an SSDP M-SEARCH answer.
- Backup and restore through the web UI.

Still open:

- VLC: refuses to run as root, and as `nobody` exits silently in the devcontainer. The Docker image runs xTeVe as
  the `xteve` user, check it there.
- XEPG mode and the guide: needs an XMLTV source.
- Image caching: needs a playlist or XMLTV with logos.
- Plex and Emby themselves. In Docker, SSDP discovery needs host networking.
- A dead source answers the client with an empty `200`, a `502` or `404` would let Plex fail fast.
- HLS bandwidth: the measured network bandwidth is bytes/s * 1000 instead of bits/s and is averaged over the whole
  stream including waits between segments, so after the first segment the highest variant is always chosen. Fixing
  only the unit would move streams down to the lowest variant: measure the download time of each segment instead.

## 2. Global state and data races

`Settings`, `Data` and `System` are global and are changed from many goroutines (web UI, maintenance, buffers) mostly
without locks.

The server built with `-race` reported 85 races during the checks of point 1:

- Most are in the web log: `showInfo`, `showDebug`, `ShowError` and `logCleanUp` change `WebScreenLog` from every
  goroutine. The mutex in `showDebug` is a local variable and protects nothing.
- `bufferingStream` against `connectToStreamingServer` and `thirdPartyBuffer` (shared playlist / stream state).
- `maintenance` against `saveSettings`, `updateServerSettings`, `buildDatabaseDVR` and `buildXEPG` (`Settings`,
  `System.ScanInProgress`).

Next:

- Decide on protection: a mutex per structure, or copies handed to readers. Start with the web log.

## 3. Web UI security

- The WebSocket upgrader accepts any `Origin` (`CheckOrigin` returns true). Without authentication any web page the
  user opens can connect to `/data/` and change settings. Check the origin against the host.

## 4. Web UI (TypeScript)

The `ts/` sources and their build are as old as the Go part was.

- Update TypeScript and `tsconfig.json`, fix compile errors.
- Rebuild `html/js` and the embedded `src/webUI.go`.
- Consider a reproducible build step instead of running the binary with `-dev` to embed the files.

## 5. Built-in updater

Updates are disabled for this fork (`GitHub.Update` in `xteve.go`), but the updater code (`src/update.go`, `src/internal/up2date`) is still built and partly reachable through a custom update URL.

- Decide: remove it, or point it to GitHub releases of this fork (the release workflow already publishes binaries).

## 6. Web UI formatting in the devcontainer

The devcontainer already installs the TypeScript compiler (`npm install -g typescript`), but nothing formats `ts/`.

- Install Prettier in the devcontainer, preferably pinned together with TypeScript in a `package.json` instead of
  global installs.
- Add a VS Code task like in [eggplant](https://github.com/SCP002/eggplant/blob/main/.vscode/tasks.json#L68-L84):
  `npx prettier --write "**/*.{js,jsx,ts,tsx,json,css,md}" --ignore-path .gitignore`. Exclude the generated
  `html/js` and `src/webUI.go`.

## 7. EPG loading and memory use

Large EPG sources take several gigabytes of RAM, because every step holds the whole data in memory:

- `downloadFileFromServer` reads the whole response into a byte slice, `extractGZIP` unpacks it into another one,
  and `checkXMLCompatibility` parses all of it only to check it.
- `getLocalXMLTV` parses each XMLTV file into structs completely and keeps every file in `Data.Cache.XMLTV`
  for the whole run, including programs of channels that are not mapped.
- `createXMLTVFile` builds the whole `xteve.xml` in memory, marshals it into a byte slice and compresses that copy.

Direction:

- Stream downloads to disk: HTTP body → gzip reader → file, without intermediate byte slices.
- Parse XMLTV with `xml.Decoder` token by token and keep only programs of mapped channels (or only an index of
  channels for the mapping menu).
- Write `xteve.xml` and `xteve.xml.gz` with `xml.Encoder` directly to the files.
- Drop or limit `Data.Cache.XMLTV`; measure memory before and after with a large real EPG file.
