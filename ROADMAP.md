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
- HLS bandwidth: the measured network bandwidth is bytes/s * 1000 instead of bits/s and is averaged over the whole
  stream including waits between segments, so after the first segment the highest variant is always chosen. Fixing
  only the unit would move streams down to the lowest variant: measure the download time of each segment instead.

## 2. Follow-ups of the package split

The state of `src` is split into `internal/logger`, `system`, `config`, `buffer`, `playlist`, `epg` and `hdhr`; the
race scenarios with a real playlist (PMS and XEPG) find no race anymore. Left over:

- XEPG channels are `map[string]any` and go through JSON in every step; use `epg.XEPGChannel` throughout.
- `imgcache`: `Caching` holds the cache lock while it downloads all images, so building the M3U and XMLTV files
  waits for it.
- The buffer answers with `200` only together with the first data: a slow source (AceStream needs more than 10 s
  for some channels) leaves the client without headers. Send the headers at once, or answer `502` when the source
  fails before any data.

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
