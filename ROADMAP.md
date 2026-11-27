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

## 3. Web UI security

- The WebSocket upgrader accepts any `Origin` (`CheckOrigin` returns true). Without authentication any web page the
  user opens can connect to `/data/` and change settings. Check the origin against the host.
- Passwords are hashed with `SHA256(secret, salt)`, an HMAC with the password as key over a fixed text: the salt is
  not used and the hash is fast. Use bcrypt or argon2 and rehash a password at its next successful login (the old
  hash can only be checked, not converted).

## 4. Web UI (TypeScript)

The `ts/` sources and their build are as old as the Go part was.

- Update TypeScript and `tsconfig.json`, fix compile errors.
- Rebuild `html/js` and the embedded `src/webUI.go`.
- Consider a reproducible build step instead of running the binary with `-dev` to embed the files.

## 5. Web UI formatting in the devcontainer

The devcontainer already installs the TypeScript compiler (`npm install -g typescript`), but nothing formats `ts/`.

- Install Prettier in the devcontainer, preferably pinned together with TypeScript in a `package.json` instead of
  global installs.
- Add a VS Code task like in [eggplant](https://github.com/SCP002/eggplant/blob/main/.vscode/tasks.json#L68-L84):
  `npx prettier --write "**/*.{js,jsx,ts,tsx,json,css,md}" --ignore-path .gitignore`. Exclude the generated
  `html/js` and `src/webUI.go`.
