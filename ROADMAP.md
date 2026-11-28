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

## 3. Web UI

The web UI was rewritten in `web/` (Svelte 5, Vite, TypeScript, Tailwind, shadcn-svelte) and replaced the old one at
`/web/`; the WebSocket protocol (`/data/`) stayed. Every page was checked against a running xTeVe, the mapping also
with 3000 channels.

## 4. Plex, Jellyfin, Emby and Kodi

Support the four clients and cover what each of them reads from xTeVe with tests. Plex and Emby are documented
already; Jellyfin and Kodi are not mentioned anywhere yet.

- Plex: HDHomeRun tuner (`discover.json`, `lineup_status.json`, `lineup.json`, `device.xml`), SSDP discovery, the
  XMLTV guide in XEPG mode, the channel limit of 480 (`PlexChannelLimit`).
- Jellyfin: either the HDHomeRun tuner or an M3U tuner with the XMLTV guide of xTeVe. Jellyfin also calls
  `/lineup.post?scan=start`, which currently falls through to `device.xml`. Its HDHomeRun discovery uses the UDP
  protocol of libhdhomerun (port 65001), not SSDP, so check whether adding by address is enough.
- Emby: the same two ways as Jellyfin.
- Kodi: IPTV Simple Client with the M3U and XMLTV of xTeVe (`tvg-id`, `tvg-name`, `tvg-logo`, `group-title`,
  `tvg-chno`). The HDHomeRun add-on of Kodi also discovers by UDP and is not supported.
- Document Jellyfin and Kodi in the README and in `docs/api.md`.

Tests:

- HTTP tests of `Index` for every HDHomeRun path: status, `Content-Type`, the fields that the clients read
  (`BaseURL`, `LineupURL`, `DeviceID`, `TunerCount`, `GuideNumber`, `GuideName`, `URL`), basic authentication of
  `lineup.json`, and `lineup.post`.
- `lineup.json` in PMS and XEPG mode: the guide numbers match the channels of the M3U and of xteve.xml, the channel
  limit of Plex.
- The M3U for Kodi, Jellyfin and Emby: the attributes above, `url-tvg`, the stream URLs with and without a buffer.
- xteve.xml: the channel IDs match `tvg-id` of the M3U.
- An SSDP test: an M-SEARCH for `upnp:rootdevice` gets an answer with the `LOCATION` of `device.xml`.
- A manual check with each client in Docker (host networking for discovery), recorded in section 1.
