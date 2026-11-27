# Roadmap

Planned work on restoring xTeVe, in the intended order.

## 1. Verify with real data

The stream buffer received the most changes during the restoration and has not been run against a live stream yet.

- Stream through every buffer type: xTeVe (built-in), FFmpeg, VLC, and no buffer (redirect).
- Cover HLS (M3U8) and MPEG-TS sources, redirects, and a source that drops the connection.
- Connect Plex and Emby as DVR / tuner: discovery over SSDP, lineup, guide (XEPG and PMS modes).
- Check the tuner limit, the "stream limit" placeholder video and client disconnects.
- Check backup and restore through the web UI, and image caching.

## 2. Tests

Tests cover the M3U parser, `parseM3U8`, channel filters, the XEPG database and mapping, and a few helpers. Most useful
next:

- Backup round trip (`zipFiles` and `extractZIP` with the real file set).

## 3. Global state and data races

`Settings`, `Data` and `System` are global and are changed from many goroutines (web UI, maintenance, buffers) mostly without locks.

- Run the tests and the server built with `-race` and collect the reports.
- Decide on protection: a mutex per structure, or copies handed to readers.

## 4. Web UI (TypeScript)

The `ts/` sources and their build are as old as the Go part was.

- Update TypeScript and `tsconfig.json`, fix compile errors.
- Rebuild `html/js` and the embedded `src/webUI.go`.
- Consider a reproducible build step instead of running the binary with `-dev` to embed the files.

## 5. Built-in updater

Updates are disabled for this fork (`GitHub.Update` in `xteve.go`), but the updater code (`src/update.go`, `src/internal/up2date`) is still built and partly reachable through a custom update URL.

- Decide: remove it, or point it to GitHub releases of this fork (the release workflow already publishes binaries).

## 6. Web UI formatting in the devcontainer

The devcontainer already installs the TypeScript compiler (`npm install -g typescript`), but nothing formats `ts/`.

- Install Prettier in the devcontainer, preferably pinned together with TypeScript in a `package.json` instead of
  global installs.
- Add a VS Code task like in [eggplant](https://github.com/SCP002/eggplant/blob/main/.vscode/tasks.json#L68-L84):
  `npx prettier --write "**/*.{js,jsx,ts,tsx,json,css,md}" --ignore-path .gitignore`. Exclude the generated
  `html/js` and `src/webUI.go`.

## 7. EPG loading and memory use

Large EPG sources take several gigabytes of RAM, because every step holds the whole data in memory:

- `downloadFileFromServer` reads the whole response into a byte slice, `extractGZIP` unpacks it into another one,
  and `checkXMLCompatibility` parses all of it only to check it.
- `getLocalXMLTV` parses each XMLTV file into structs completely and keeps every file in `Data.Cache.XMLTV`
  for the whole run, including programs of channels that are not mapped.
- `createXMLTVFile` builds the whole `xteve.xml` in memory, marshals it into a byte slice and compresses that copy.

Direction:

- Stream downloads to disk: HTTP body → gzip reader → file, without intermediate byte slices.
- Parse XMLTV with `xml.Decoder` token by token and keep only programs of mapped channels (or only an index of
  channels for the mapping menu).
- Write `xteve.xml` and `xteve.xml.gz` with `xml.Encoder` directly to the files.
- Drop or limit `Data.Cache.XMLTV`; measure memory before and after with a large real EPG file.
