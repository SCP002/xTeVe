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

Tests exist only for the M3U parser and a few helpers. Most useful next:

- `parseM3U8`: segments, durations, bandwidth switching, relative and absolute URLs.
- Channel filters (`filterThisStream`, `createFilterRules`): group and custom filters with include / exclude rules.
- XEPG mapping (`createXEPGDatabase`, `mapping`): channel numbers, missing EPG data, regex renaming.
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
