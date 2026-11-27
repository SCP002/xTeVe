# Roadmap

Planned work on restoring xTeVe, in the intended order.

## 1. Verify with real data

Checked with a real MPEG-TS playlist (432 channels, PMS mode) and local test sources:

- Buffers: no buffer (redirect), xTeVe, FFmpeg. Sources: MPEG-TS, HLS media and master playlists, a source that
  redirects, a source that drops the connection (reconnects), a dead source (404).
- Tuner limit with the "stream limit" video, two clients sharing one channel, client disconnects (the upstream
  connection and FFmpeg are stopped).
- HDHR emulation: `discover.json`, `lineup.json`, `lineup_status.json`, `device.xml`, an SSDP M-SEARCH answer.
- Backup and restore through the web UI, web authentication.
- XEPG mode with the EPG of epg.one (3214 channels, 662000 programmes): mapping by name, xteve.xml, M3U, lineup,
  image caching of the channel logos.
- XMLTV sources iptvx.one, programtv.ru, epg.it999.ru (edem) and epg.one are read completely; non-standard parts
  (`<year>` of iptvx.one, `source-info-logo`) are accepted. A full XEPG rebuild with epg.one takes about 3 s with
  28 MB of memory, and xteve.xml is valid against the XMLTV DTD (`xmllint --dtdvalid`). The JTV format
  (teleguide.info) is not XMLTV and is not supported.

Still open:

- VLC: refuses to run as root, and as `nobody` exits silently in the devcontainer. The Docker image runs xTeVe as
  the `xteve` user, check it there.
- Plex and Emby themselves. Both can add the tuner by its address (`http://<host>:34400`), which needs only the TCP
  port. SSDP discovery does not work through a published UDP port: the M-SEARCH requests go to the multicast address
  239.255.255.250, which Docker does not forward into a bridge network. It needs host networking or a macvlan
  network.

## 2. Follow-ups of the package split

The state of `src` is split into `internal/logger`, `system`, `config`, `buffer`, `playlist`, `epg` and `hdhr`; the
race scenarios with a real playlist (PMS and XEPG) find no race anymore. The image cache downloads without holding
its lock, and the buffer sends its headers to slow sources after 3 s and answers `502` when a source fails before.

## 3. New web UI

The web UI in `ts/` builds the DOM by hand in old TypeScript (about 4200 lines, `menu_ts.js` alone 81 KB). It is
rewritten in `web/` (Svelte 5, Vite, TypeScript, Tailwind, shadcn-svelte) and served at `/ui/`; the Go side and the
WebSocket protocol (`/data/`) stay.

Done: the build (`build.sh`, Dockerfile, release workflow, a placeholder for `go build` without Node), Prettier in
`format.sh`, typed WebSocket commands sent one after the other (every answer replaces the login token), `/ui/api/`
for the login and the first user, the configuration wizard, the overview and the log.

Left, screen by screen, each checked against a running xTeVe:

- Playlists (M3U and HDHomeRun) and XMLTV files: list, add, edit, update, delete.
- Filters.
- Mapping (XEPG): the big table, bulk edit, XMLTV channel search, logo upload.
- Users.
- Settings, backup and restore.
- Then remove the old web UI (`ts/`, `html/`, `/web/`, the global `tsc` of the devcontainer) and serve the new one
  at `/web/`.
