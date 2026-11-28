package src

import _ "embed"

// streamLimit is the video clients get when all tuners of a playlist are in use.
//
//go:embed assets/stream-limit.ts
var streamLimit []byte
