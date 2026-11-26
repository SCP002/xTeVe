<div align="center" style="background-color: #111; padding: 100;">
    <a href="https://github.com/SCP002/xTeVe"><img width="880" height="200" src="html/img/logo_b_880x200.jpg" alt="xTeVe" /></a>
</div>
<br>

# xTeVe

## M3U Proxy and EPG aggregator for Plex DVR and Emby Live TV

### This is a fork of <https://github.com/xteve-project/xTeVe>, all credit goes to the original author

Documentation for setup and configuration is [here](https://github.com/xteve-project/xTeVe-Documentation/blob/master/en/configuration.md).

#### Donation

* **Bitcoin:** 1c1iCe4CJPfNUXtqxKBbW2Qd2EtqRPWme  
![Bitcoin](html/img/BC-QR.jpg "Bitcoin - xTeVe")

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

### xTeVe Beta branch

New features and bug fixes are only available in beta branch. Only after successful testing are they are merged into the master branch.

**It is not recommended to use the beta version in a production system.**  

With the command line argument `branch` the Git Branch can be changed. xTeVe must be started via the terminal.  

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

When the branch is changed, an update is only performed if there is a new version and the update function is activated in the settings.  

---

## Build from source code [Go / Golang]

### Requirements

* [Go](https://golang.org) (go1.27.3 or newer)

### Dependencies

* [avfs](https://github.com/avfs/avfs)
* [go-ssdp](https://github.com/koron/go-ssdp)
* [lo](https://github.com/samber/lo)
* [testify](https://github.com/stretchr/testify)
* [websocket](https://github.com/gorilla/websocket)

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

Then, to embed updated JavaScript files into the source code (src/webUI.go), run it in development mode at least once:

```sh
go build .
./xteve -dev
```

:exclamation: To not to get CreateFile error, do not forget to switch your binary to "regular" mode after runnning with `-dev` flag:

`xteve -branch master` or `xteve -branch beta`

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

When creating a fork, the xTeVe GitHub account must be changed from the source code or the update function disabled.

xteve.go - Line: 31

```go
var GitHub = GitHubStruct{Branch: "master", User: "SCP002", Repo: "xTeVe", Update: true}

// Branch: GitHub Branch
// User:   GitHub Username
// Repo:   GitHub Repository
// Update: Automatic updates from the GitHub repository [true|false]
```
