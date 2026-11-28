# HTTP addresses and API

xTeVe listens on one port (34400 by default) for the web UI, Plex and Emby, IPTV players and the API. `<host>` below
is the address of xTeVe, e.g. `192.168.1.10:34400`.

## Addresses

| Address                                 | For                                                                   |
|-----------------------------------------|-----------------------------------------------------------------------|
| `/web/`                                 | The web UI                                                            |
| `/discover.json`, `/lineup.json`, `/lineup_status.json`, `/device.xml` | Plex and Emby (HDHomeRun emulation)   |
| `/m3u/xteve.m3u`                        | The M3U playlist of the active channels (XEPG source only)            |
| `/xmltv/xteve.xml`, `/xmltv/xteve.xml.gz` | The XMLTV guide of the active channels (XEPG source only)           |
| `/stream/…`                             | The streams of the channels, as listed in the lineup and the playlist |
| `/images/…`                             | Cached logos and posters (with image caching)                         |
| `/data_images/…`                        | Logos uploaded in the web UI                                          |
| `/api/`                                 | The API, see below                                                    |

### Playlist of some groups

`/m3u/xteve.m3u?group-title=News,Sport` lists only the channels of these groups.

### With a login

When *Settings → Login* requires a login, the user needs the matching permission on the *Users* page:

| Part        | How to log in                                                         |
|-------------|-----------------------------------------------------------------------|
| Plex / Emby | Tuner address `username:password@<host>` (HTTP basic authentication)  |
| M3U         | `/m3u/xteve.m3u?username=…&password=…` (also with `&group-title=…`)   |
| XMLTV       | `/xmltv/xteve.xml?username=…&password=…`                              |
| API         | `login` command, see below                                            |

A wrong login is answered with `403`.

## API

The API is off by default: turn it on in *Settings → General → API*. Every request is a `POST` to `/api/` with a JSON
object; its `cmd` is the command.

```sh
curl -X POST -d '{"cmd":"status"}' http://<host>/api/
```

### Commands

| Command        | Does                                                                                      |
|----------------|-------------------------------------------------------------------------------------------|
| `status`       | Returns the state of xTeVe (below)                                                        |
| `update.m3u`   | Downloads all M3U playlists again and rebuilds the list of streams                       |
| `update.hdhr`  | Reads all HDHomeRun tuners again and rebuilds the list of streams                        |
| `update.xmltv` | Downloads all XMLTV files again                                                           |
| `update.xepg`  | Rebuilds the XEPG database, xteve.xml and xteve.m3u from the streams and XMLTV files      |
| `login`        | Logs in, see below                                                                        |

The update commands answer when they are done. `update.m3u` and `update.xmltv` do not rebuild the channels and the
guide: send `update.xepg` after them to use the new files.

### Answers

Every answer has `status`. Successful `status` answer:

```json
{
  "status": true,
  "epg.source": "XEPG",
  "streams.active": 7,
  "streams.all": 63,
  "streams.xepg": 2,
  "url.dvr": "192.168.1.10:34400",
  "url.m3u": "http://192.168.1.10:34400/m3u/xteve.m3u",
  "url.xepg": "http://192.168.1.10:34400/xmltv/xteve.xml",
  "version.api": "1.1.0",
  "version.xteve": "2.5.0"
}
```

| Field            | Meaning                                       |
|------------------|-----------------------------------------------|
| `epg.source`     | `XEPG` or `PMS`                               |
| `streams.active` | Streams the filters take                      |
| `streams.all`    | All streams of the playlists                  |
| `streams.xepg`   | Active channels of the XEPG database          |
| `url.dvr`        | Tuner address for Plex and Emby               |
| `url.m3u`        | Playlist address                              |
| `url.xepg`       | Guide address                                 |

Fields that are 0 are left out.

A command that fails is answered with HTTP `200`, `"status": false` and the reason in `err`:

```json
{ "status": false, "err": "Invalid API command" }
```

HTTP errors: `423` when the API is turned off, `404` for other methods than `POST`, `400` when the body is not JSON.

### With a login

With *Settings → Login → API*, the API needs a user with the permission *API*. Log in first:

```sh
curl -X POST -d '{"cmd":"login","username":"plex","password":"secret"}' http://<host>/api/
```

```json
{ "status": true, "token": "U0T-NTSaigh-RlbkqERsHvUpgvaaY2dyRGuwIIvv" }
```

Then send the token with every command. **Every answer carries a new token and the one sent is no longer valid**:
send each command with the token of the previous answer.

```sh
curl -X POST -d '{"cmd":"status","token":"U0T-NTSaigh-RlbkqERsHvUpgvaaY2dyRGuwIIvv"}' http://<host>/api/
```

A token is valid for 60 minutes. A used or expired token is answered with `"err": "Session has expired"`, a command
without token (other than `login`) with `"err": "Login incorrect"`.

## Web UI

The web UI uses its own interface, which is not meant for scripts and may change with the web UI: the WebSocket
`/data/` and `/web/api/` (login and first user). Its messages are described in
[web/src/lib/api/types.ts](../web/src/lib/api/types.ts).
