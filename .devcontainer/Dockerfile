# Node.js LTS for the web UI (Debian has only Node 18, Vite needs 20.19 or newer)
FROM node:26-bookworm-slim AS node

FROM golang:1.27.3-bookworm

ENV GOTOOLCHAIN=auto

# ffmpeg, vlc-bin: external buffer options in xTeVe
# libxml2-utils (xmllint), python3-lxml, python3-pip: checking XMLTV files against the XMLTV DTD
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        jq \
        ffmpeg \
        vlc-bin \
        libxml2-utils \
        python3-lxml \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# TypeScript compiler for the old web UI (ts/ -> html/js); the new one in web/ pins its tools in package.json
RUN npm install -g typescript

# System libraries of Chromium, to look at the web UI with Playwright (the browser itself comes with the version
# of Playwright in web/package.json)
RUN npx -y playwright install-deps chromium \
    && rm -rf /var/lib/apt/lists/*

RUN go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install golang.org/x/tools/cmd/goimports@latest \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest \
    && go install github.com/golangci/golines@latest

WORKDIR /workspace
