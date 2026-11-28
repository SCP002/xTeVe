<div align="center">
  <a href="https://github.com/SCP002/xTeVe"><img width="880" height="200" src=".github/logo.jpg" alt="xTeVe" /></a>
</div>

# xTeVe

M3U proxy and EPG aggregator for Plex DVR, Emby Live TV and IPTV players.

xTeVe merges IPTV playlists and XMLTV program guides, lets you choose and map the channels, and passes them on as an
HDHomeRun tuner (Plex, Emby) or as one M3U playlist with one XMLTV guide (any IPTV player).

This is a maintained fork of [xteve-project/xTeVe](https://github.com/xteve-project/xTeVe).

## Features

- **Playlists:** M3U files and HDHomeRun tuners, local or remote, merged and updated on a schedule.
- **Program guides:** XMLTV files (also gzipped), merged into one guide that follows the XMLTV DTD. Guides that do
  not follow it are read anyway.
- **Channels:** filters by group or text, channel numbers, names, groups, logos (also uploaded), EPG categories,
  timeshift, dummy programs for channels without guide.
- **Streaming:** optional buffer (xTeVe, FFmpeg or VLC) with HLS support, re-streaming to several clients, tuner limit
  per playlist.
- **Images:** logos and posters can be cached by xTeVe.
- **Web UI** with login, users and permissions for each part (web UI, Plex/Emby, M3U, XMLTV, API).
- Backups before every scheduled update, automatic updates of xTeVe itself.

## Install

### Binary

Download the binary for your system from the [releases](https://github.com/SCP002/xTeVe/releases) and start it:

```sh
./xteve-linux-amd64
```

The settings and data are kept in `~/.xteve/` unless `-config` says otherwise.

### Docker

```sh
git clone https://github.com/SCP002/xTeVe.git
cd xTeVe
docker compose up -d
```

[compose.yaml](compose.yaml) builds the image from the repository and starts xTeVe at port 34400. Change it to your
needs before the first start:

- `TZ`: your time zone, used for the guide and the update times.
- `XTEVE_PORT`: another port than the one of the settings; publish the same port in `ports`.
- `network_mode: host`: lets Plex and Emby find xTeVe by themselves (see below).

The volume `xteve_conf` keeps the settings, playlists and guides when the container is recreated. xTeVe runs as the
user `xteve`, FFmpeg and VLC are included for the buffer.

```sh
docker compose logs -f                     # console of xTeVe
git pull && docker compose up -d --build   # update to the latest version
docker compose down                        # stop and remove the container, the volume stays
```

The image takes its version from `build.sh`; `docker compose build --build-arg VERSION=vX.Y.Z` sets another one.

## First start

Open `http://<host>:34400/web/`. A wizard asks for the number of tuners, the EPG source, the playlist and, with XEPG,
the XMLTV file.

The EPG source decides how xTeVe works:

- **XEPG** (recommended): xTeVe builds the guide from your XMLTV files. Channels are managed on the *Mapping* page,
  and xTeVe also offers an M3U playlist and an XMLTV guide for IPTV players.
- **PMS**: Plex or Emby use their own guide; xTeVe only passes on the streams.

Then choose the channels: *Filters* decide which streams of the playlists are used (without filters all are), and
*Mapping* assigns each channel its XMLTV channel and number. Only active channels are passed on.

### Plex and Emby

Add a DVR / live TV tuner of the type HDHomeRun with the address `<host>:34400`. Plex and Emby can also find xTeVe by
themselves (SSDP), but only in the same network: in Docker that needs host networking (`--network host`) or a macvlan
network, as Docker does not pass the multicast requests into a bridge network. Entering the address works in any case.

With XEPG, give Plex or Emby the XMLTV guide of xTeVe as the program guide.

### IPTV players

The *Overview* page shows the addresses:

- M3U: `http://<host>:34400/m3u/xteve.m3u`, only some groups with `?group-title=News,Sport`
- XMLTV: `http://<host>:34400/xmltv/xteve.xml`

All addresses of xTeVe and the API are described in [docs/api.md](docs/api.md).

### Login and users

*Settings → Login* turns on a login for each part of xTeVe. The first time the web UI needs one, it asks to create the
first user, who may use everything. Further users and what they may use are set on the *Users* page. With a login:

- Plex / Emby: `username:password@<host>:34400` as the tuner address
- M3U and XMLTV: add `?username=...&password=...` to the address
- API: see [docs/api.md](docs/api.md)

Passwords are stored as bcrypt hashes.

### HTTPS

*Settings → General → HTTPS* serves xTeVe over HTTPS. Without a certificate, xTeVe creates a self-signed one, which
lets xTeVe start but is not trusted by browsers, Plex or Emby. For working HTTPS put your own certificate and key into
the config folder:

```text
~/.xteve/certificates/xteve.crt
~/.xteve/certificates/xteve.key
```

`xteve.crt` holds the server certificate followed by its intermediates and the CA certificate. A certificate of your
own CA must also be trusted by the clients.

### Backups

xTeVe makes a backup before every scheduled update (*Settings → Backup*). A backup can also be downloaded and
restored there, or from the command line with `xteve -restore <file>`.

## Updates

With *Settings → General → Update xTeVe automatically*, xTeVe looks for a new release at the start and once a day,
checks the download against the `SHA256SUMS` of the release, replaces itself and restarts. In Docker, rebuild the
image instead (`git pull && docker compose up -d --build`).

The branch `master` takes stable releases, `beta` also pre-releases, which are not meant for production:

```sh
xteve -branch beta
```

## Command line

| Option                | Meaning                                                         |
|-----------------------|-----------------------------------------------------------------|
| `-config <folder>`    | Folder of the settings and data (default `~/.xteve/`)           |
| `-port <port>`        | Port of the web server instead of the one in the settings (34400) |
| `-restore <file>`     | Restore a backup and exit                                       |
| `-branch master\|beta` | Release branch for updates                                      |
| `-debug 0-3`          | Debug output                                                    |
| `-info`               | Show system information and exit                                |
| `-dev`                | Developer mode, see below                                       |
| `-h`                  | Help                                                            |

## Development

Requirements: [Go](https://go.dev) 1.27.3 or newer and [Node.js](https://nodejs.org) 20.19 or newer. The
[devcontainer](.devcontainer/) has both, the linters, Prettier, and Playwright for browser checks.

### Build

```sh
(cd web && npm ci && npm run build)   # the web UI, embedded by go build
go build .
```

Without the web UI build, `go build` still works and `/web/` explains how to build it. A binary built this way reports
its version as `dev`. `./build.sh` builds the web UI and the release binaries of every OS and architecture Go supports
into `build/`, with the version set in `build.sh`.

### Web UI

The web UI in [web/](web/) is Svelte 5 with Vite, TypeScript, Tailwind and shadcn-svelte. It talks to xTeVe over the
WebSocket `/data/` (one command per connection, typed in [web/src/lib/api/types.ts](web/src/lib/api/types.ts)) and
`/web/api/` for the login.

During development run xTeVe and the Vite dev server, which reloads on every change and forwards to xTeVe:

```sh
go run . -port 34400
cd web && npm run dev                          # http://localhost:5173/web/
XTEVE=http://127.0.0.1:34500 npm run dev       # xTeVe at another address
npm run check                                  # svelte-check and tsc
```

In VS Code the tasks *xTeVe: run* and *Web UI: dev server* do the same; the devcontainer publishes both ports.
`xteve -dev` serves the web UI from `web/dist/` on disk instead of the embedded one.

### Code style and tests

```sh
./format.sh          # goimports, golines (120 columns), golangci-lint --fix, Prettier
go test -race ./...
```

Planned work is in [ROADMAP.md](ROADMAP.md).

### Release

1. Set the version in `build.sh`, e.g. `version="v2.6.0"`.
2. Push a tag of the same name: `git tag v2.6.0 && git push origin v2.6.0`.

The [release workflow](.github/workflows/release.yml) checks the version, the web UI and the tests, builds the
binaries and publishes them with their `SHA256SUMS` as a GitHub release.

## Forks

A fork must change the GitHub account in `xteve.go` or turn the updates off, otherwise it updates itself to the
releases of this repository:

```go
var GitHub = GitHubStruct{Branch: "master", User: "SCP002", Repo: "xTeVe", Update: true}
```

## License

[MIT](LICENSE)
