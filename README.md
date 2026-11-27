<div align="center" style="background-color: #111; padding: 100;">
    <a href="https://github.com/SCP002/xTeVe"><img width="880" height="200" src="html/img/logo_b_880x200.jpg" alt="xTeVe" /></a>
</div>
<br>

# xTeVe

## M3U Proxy and EPG aggregator for Plex DVR and Emby Live TV

---

## Features

### Files

* Merge external M3U files
* Merge external XMLTV files (EPG aggregation)
* Automatic M3U and XMLTV update
* M3U and XMLTV export

#### Channel management

* Filtering streams
* Teleguide timeshift
* Channel mapping
* Channel order
* Channel logos
* Channel categories

#### Streaming

* Buffer with HLS / M3U8 support
* Re-streaming
* Number of tuners adjustable
* Compatible with Plex / Emby EPG

---

## Downloads

* See [releases page](https://github.com/SCP002/xTeVe/releases)

---

## TLS mode

This mode can be enabled by ticking the checkbox in `Settings -> General`.

Unless the server's certificate and it's private key already exists in xTeVe config directory, xTeVe will generate a self-signed automatically.

Self-signed certificate will only allow TLS mode to start up but not to actually establish a secure connections.
For truly working HTTPS, you should [generate](https://gist.github.com/fntlnz/cf14feb5a46b2eda428e000157447309) a certificate by yourself and **also** add the CA certificate to the client-side certificate storage (where the web browser, Plex etc. is).

Certificate and it's private key should be placed in xTeVe config directory like so:

```text
/home/username/.xteve/certificates/xteve.crt
/home/username/.xteve/certificates/xteve.key
```

If the certificate is signed by a certificate authority (CA), it should be the concatenation of the server's certificate, any intermediates, and the CA's certificate.

---

## Docker

### Get an image

Pull from dockerhub:

```sh
docker pull scp002/xteve:latest
```

**OR** build your own image from the source code in this repository:

```sh
git clone https://github.com/SCP002/xTeVe.git
cd xTeVe
docker build --tag scp002/xteve .
```

The image version is taken from `build.sh`. To set another one, add `--build-arg VERSION=vX.Y.Z`.

### Create a container

```sh
docker create \
    --tty \
    --publish 34400:34400 \
    --volume xteve_conf:/home/xteve/conf \
    --name xteve \
    scp002/xteve
```

The volume keeps settings, playlists and the EPG data when the container is recreated.

With the specific timezone, ip and port:

```sh
docker create \
    --tty \
    --env TZ=Europe/Amsterdam \
    --env XTEVE_PORT=12345 \
    --publish 192.168.88.218:12345:12345 \
    --volume xteve_conf:/home/xteve/conf \
    --name xteve \
    scp002/xteve
```

`XTEVE_PORT` overrides the port from the settings. Without it, the port set in the web UI is used (34400 by default).

Other command line arguments of xTeVe can be appended after the image name, for example `scp002/xteve -debug=1`.

### Start a container

```sh
docker start xteve
```

#### Attach to a started container

```sh
docker attach xteve
```

To detach from a container, press `Ctrl + P`, `Ctrl + Q`. Pressing `Ctrl + C` stops xTeVe.

#### Access web UI

Open `http(s)://<ip>:<port>/web/` in browser, for example:
`http://192.168.88.218:34400/web/`

#### Stop a running container

```sh
docker stop xteve
```

---

### Updates and the beta branch

xTeVe updates itself from the [GitHub releases](https://github.com/SCP002/xTeVe/releases) at start and once a day,
if the automatic update is enabled in the settings. The binary of the platform is checked against the `SHA256SUMS`
of the release before it replaces the running one, then xTeVe restarts. In Docker update the image instead.

The master branch uses stable releases only, the beta branch also pre-releases.

**It is not recommended to use the beta version in a production system.**  

With the command line argument `branch` the branch can be changed. xTeVe must be started via the terminal.  

#### Switch from master to beta branch

```text
xteve -branch beta

...
[xTeVe] GitHub:                https://github.com/SCP002
[xTeVe] Git Branch:            beta [SCP002]
...
```

#### Switch from beta to master branch

```text
xteve -branch master

...
[xTeVe] GitHub:                https://github.com/SCP002
[xTeVe] Git Branch:            master [SCP002]
...
```

When the branch is changed, an update is only performed if there is a newer release and the update function is activated in the settings.  

---

## Build from source code [Go / Golang]

### Requirements

* [Go](https://golang.org) (go1.27.3 or newer)

### Build

```sh
git clone https://github.com/SCP002/xTeVe.git
cd xTeVe
go build .
```

A binary built this way reports its version as `dev`.

To build release binaries for every supported OS / architecture pair into the `build` folder, with the version from `build.sh`:

```sh
./build.sh
```

### Update web files (optional)

If TypeScript files were changed, run:

```sh
tsc -p ./ts/tsconfig.json
```

The `html` folder is embedded into the binary at build time (`go:embed`), so `go build .` is enough. To see changes
without a new build, run xTeVe in developer mode in the folder of the source code: it then reads the web UI from the
local `html` folder.

```sh
./xteve -dev
```

### Development

The repository contains a [devcontainer](.devcontainer/) with Go, the linters and the tools for the web UI.

Format the code, sort imports and run the linter (also the default build task in VS Code):

```sh
./format.sh
```

Planned work is listed in [ROADMAP.md](ROADMAP.md).

### Release

1. Set the new version in `build.sh`, for example `version="v2.6.0"`.
2. Commit and push a tag with the same name: `git tag v2.6.0 && git push origin v2.6.0`.

The [release workflow](.github/workflows/release.yml) checks that the tag matches `build.sh`, runs the tests, builds the binaries and publishes them as a GitHub release.

---

## Forks

When creating a fork, the GitHub account must be changed in the source code or the update function disabled, otherwise
the fork updates itself to the releases of this repository.

xteve.go:

```go
var GitHub = GitHubStruct{Branch: "master", User: "SCP002", Repo: "xTeVe", Update: true}

// Branch: GitHub Branch
// User:   GitHub Username
// Repo:   GitHub Repository
// Update: Updates from the GitHub releases of the repository [true|false]
```
