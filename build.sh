#!/bin/bash

project_name="xteve"
build_path="build"
version="v2.5.0"
cgo_enabled=0

command -v go > /dev/null || { echo "go not found in PATH" >&2; exit 1; }
command -v npm > /dev/null || { echo "npm not found in PATH, it builds the web UI" >&2; exit 1; }

# The web UI is embedded into the binaries
echo "Building the web UI"
(cd web && npm ci && npm run build) || exit 1

mkdir -p "$build_path"

for target in $(go tool dist list); do
    os=${target%/*}
    arch=${target#*/}
    ext=""
    [[ $os == "windows" ]] && ext=".exe"
    [[ $arch == "wasm" ]] && ext=".wasm"

    echo "Building for $os / $arch"
    CGO_ENABLED=$cgo_enabled GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X ${project_name}/version.Version=${version}" -o "${build_path}/${project_name}-${os}-${arch}${ext}" .
done

# Checksums for the built-in updater, which only uses binaries listed here
(cd "$build_path" && sha256sum -- * > SHA256SUMS)
