FROM golang:1.27.3-bookworm

ENV GOTOOLCHAIN=auto

# nodejs + npm: TypeScript compiler for ts/ -> html/js
# ffmpeg, vlc-bin: external buffer options in xTeVe
# libxml2-utils (xmllint), python3-lxml, python3-pip: checking XMLTV files against the XMLTV DTD
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        jq \
        nodejs \
        npm \
        ffmpeg \
        vlc-bin \
        libxml2-utils \
        python3-lxml \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g typescript

RUN go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install golang.org/x/tools/cmd/goimports@latest \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest \
    && go install github.com/golangci/golines@latest

WORKDIR /workspace
