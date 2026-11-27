package src

/*
  Render Tuner Stream-Limit image as Video [ffmpeg]
  -loop 1 -i stream-limit.jpg -c:v libx264 -t 1 -pix_fmt yuv420p -vf scale=1920:1080  stream-limit.ts
*/

import (
	"xteve/src/internal/buffer"
	"xteve/src/internal/config"

	"github.com/avfs/avfs/vfs/memfs"
	"github.com/avfs/avfs/vfs/osfs"
)

// buffers streams channels through the buffer, see bufferingStream.
var buffers = buffer.New(buffer.Config{
	VFS:              osfs.New(),
	TempFolder:       System.TempFolder,
	Playlist:         bufferPlaylist,
	StreamLimitVideo: streamLimitVideo,
	Error:            ShowError,
})

// initBufferVFS sets where new streams keep their segment files: in memory or on disk.
func initBufferVFS(virtual bool) {
	if virtual {
		buffers.SetVFS(memfs.New())
	} else {
		buffers.SetVFS(osfs.New())
	}
}

// bufferPlaylist returns the name and the number of tuners of a playlist for the buffer.
func bufferPlaylist(playlistID string) (name string, tuners int) {
	var playlistType string
	switch playlistID[0:1] {
	case "M":
		playlistType = "m3u"
	case "H":
		playlistType = "hdhr"
	}

	return providerName(playlistID, playlistType), getTuner(playlistID, playlistType)
}

// streamLimitVideo returns the video sent to clients when all tuners of a playlist are in use.
func streamLimitVideo() []byte {
	if value, ok := webUI["html/video/stream-limit.ts"]; ok {
		return []byte(GetHTMLString(value))
	}
	return nil
}

func getTuner(id, playlistType string) (tuner int) {
	switch config.Get().Buffer {
	case "-":
		tuner = config.Get().Tuner

	case "xteve", "ffmpeg", "vlc":
		// A playlist without a number of tuners has one
		var provider, _ = getProvider(id, playlistType)
		tuner = max(int(provider.Tuner), 1)
	}

	return
}
