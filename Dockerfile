# syntax=docker/dockerfile:1

FROM golang:1.27.3-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Version is taken from build.sh unless set explicitly.
ARG VERSION
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    version="${VERSION:-$(sed -n 's/^version="\(.*\)"$/\1/p' build.sh)}" \
    && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X xteve/version.Version=${version}" \
        -o /xteve .

FROM debian:bookworm-slim

# FFmpeg and VLC are optional external stream buffers, selectable in the web UI.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
        ffmpeg \
        vlc-bin \
        vlc-plugin-base \
    && rm -rf /var/lib/apt/lists/*

ENV XTEVE_HOME=/home/xteve
ENV XTEVE_CONF=/home/xteve/conf
ENV XTEVE_TEMP=/tmp/xteve

# VLC refuses to run as root
RUN useradd --uid 31337 --create-home --home-dir "${XTEVE_HOME}" xteve \
    && mkdir -p "${XTEVE_HOME}/cache" "${XTEVE_CONF}" "${XTEVE_TEMP}" \
    && chown -R xteve:xteve "${XTEVE_HOME}" "${XTEVE_TEMP}"

COPY --from=builder /xteve /usr/local/bin/xteve
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

USER xteve
WORKDIR ${XTEVE_HOME}

VOLUME ["/home/xteve/conf", "/tmp/xteve"]

EXPOSE 34400

ENTRYPOINT ["docker-entrypoint.sh"]
