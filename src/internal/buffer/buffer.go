// Package buffer streams channels to clients through a buffer: the built-in xTeVe buffer, FFmpeg or VLC.
//
// A Manager keeps the streams of each playlist. The first client of a channel starts a downloader, which writes the
// stream into numbered segment files ("1.ts", "2.ts", ...) in a folder of its own; every client of the channel reads
// these files. When the last client leaves, the downloader stops and removes its folder. A playlist streams at most
// as many channels at a time as it has tuners, other clients get a short "stream limit" video.
//
// The state shared between clients and downloaders is owned by the Manager and guarded by its mutex; what only a
// downloader uses stays in the downloader.
package buffer

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/logger"

	"github.com/avfs/avfs"
)

// Config is what the buffer needs from the rest of xTeVe.
type Config struct {
	// VFS holds the segment files, in memory or on disk, see also Manager.SetVFS.
	VFS avfs.VFS
	// TempFolder returns the folder for the segment files.
	TempFolder func() string
	// Playlist returns the name and the number of tuners of a playlist.
	Playlist func(playlistID string) (name string, tuners int)
	// StreamLimitVideo returns the video sent to clients when all tuners of a playlist are in use.
	StreamLimitVideo func() []byte
	// Error logs an error with an xTeVe error code.
	Error func(err error, code int)
}

// Manager streams channels to clients.
type Manager struct {
	cfg Config

	// runs numbers the downloads, so that every download has a folder of its own
	runs atomic.Int64

	mu        sync.Mutex
	vfs       avfs.VFS
	playlists map[string]*playlist
}

type playlist struct {
	id     string
	name   string
	folder string
	tuners int

	// streams by stream ID, guarded by Manager.mu
	streams map[int]*stream
}

type stream struct {
	id          int
	playlist    *playlist
	url         string
	channelName string
	folder      string
	// vfs holds the segment files of the stream
	vfs avfs.VFS

	// clients is guarded by Manager.mu
	clients int

	// stop is closed when the last client left, the downloader then ends
	stop chan struct{}
	// done is closed when the downloader has ended and removed its folder
	done chan struct{}
	// ready reports that the first segment is complete
	ready atomic.Bool

	failOnce sync.Once
	failed   chan struct{}
	err      error // set before failed is closed
}

// fail records the first error of the stream, its clients then leave.
func (s *stream) fail(err error) {
	s.failOnce.Do(func() {
		s.err = err
		close(s.failed)
	})
}

// stopped reports whether the last client left.
func (s *stream) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

// sleep waits for d and reports false if the stream was stopped meanwhile.
func (s *stream) sleep(d time.Duration) bool {
	var t = time.NewTimer(d)
	defer t.Stop()
	select {
	case <-s.stop:
		return false
	case <-t.C:
		return true
	}
}

// New returns a manager.
func New(cfg Config) *Manager {
	return &Manager{cfg: cfg, vfs: cfg.VFS, playlists: make(map[string]*playlist)}
}

// SetVFS sets where new streams keep their segment files. Running streams keep using theirs.
func (m *Manager) SetVFS(vfs avfs.VFS) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vfs = vfs
}

// Serve streams the channel with the URL of the playlist to the client.
func (m *Manager) Serve(w http.ResponseWriter, r *http.Request, playlistID, url, channelName string) {
	time.Sleep(time.Duration(config.Get().BufferTimeout) * time.Millisecond)

	w.Header().Set("Connection", "close")

	var s, ok = m.join(playlistID, url, channelName)
	if !ok {
		m.sendStreamLimit(w)
		return
	}
	defer m.leave(s)

	w.WriteHeader(http.StatusOK)

	// Wait until the first segment has been downloaded by the buffer
	var timeout = time.NewTimer(20 * time.Second)
	defer timeout.Stop()

	for !s.ready.Load() {
		select {
		case <-r.Context().Done():
			return
		case <-s.failed:
			return
		case <-s.done:
			return
		case <-timeout.C:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Send the segments to the client
	var sent []string

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.failed:
			m.cfg.Error(s.err, 0)
			return
		default:
		}

		var segments, ok = m.newSegments(s, sent)
		if !ok {
			// The downloader has ended and removed its folder
			return
		}

		for _, name := range segments {
			sent = append(sent, name)

			var data, err = readFile(s.vfs, s.folder+name)
			if err != nil {
				// Removed already by another client
				continue
			}

			logger.Debug(2, fmt.Sprintf("Buffer Status:Send to client (%s)", s.folder+name))

			if _, err := w.Write(data); err != nil {
				return
			}

			// Clients remove segments they are done with
			if len(sent) > 21 {
				if err := s.vfs.RemoveAll(s.folder + sent[0]); err != nil {
					m.cfg.Error(err, 4007)
				}
				sent = sent[1:]
			}
		}

		if len(segments) == 0 {
			select {
			case <-r.Context().Done():
				return
			case <-s.done:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

// join adds a client to the stream of the URL, or starts a new stream if a tuner of the playlist is free. It reports
// false if all tuners are in use.
func (m *Manager) join(playlistID, url, channelName string) (*stream, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var p, ok = m.playlists[playlistID]
	if !ok {
		var name, tuners = m.cfg.Playlist(playlistID)
		p = &playlist{
			id:      playlistID,
			name:    name,
			folder:  m.cfg.TempFolder() + playlistID + string(os.PathSeparator),
			tuners:  tuners,
			streams: make(map[int]*stream),
		}
		m.playlists[playlistID] = p
	}

	// The URL is already being streamed to another client
	for _, s := range p.streams {
		if s.url == url {
			s.clients++
			logger.Debug(1, fmt.Sprintf("Restream Status:Playlist: %s - Channel: %s - Connections: %d", p.name,
				s.channelName, s.clients))
			logger.Info(fmt.Sprintf("Streaming Status:Channel: %s (Clients: %d)", s.channelName, s.clients))
			return s, true
		}
	}

	if len(p.streams) >= p.tuners {
		logger.Info(fmt.Sprintf("Streaming Status:Playlist: %s - No new connections available. Tuner = %d", p.name,
			p.tuners))
		if len(p.streams) == 0 {
			delete(m.playlists, playlistID)
		}
		return nil, false
	}

	var s = &stream{
		id:          freeStreamID(p.streams),
		playlist:    p,
		url:         url,
		channelName: channelName,
		folder: p.folder + md5Hex(url) + "-" + strconv.FormatInt(m.runs.Add(1), 10) +
			string(os.PathSeparator),
		vfs:     m.vfs,
		clients: 1,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		failed:  make(chan struct{}),
	}
	p.streams[s.id] = s
	logger.Debug(1, fmt.Sprintf("Streaming Status:Stream ID = %d", s.id))

	switch config.Get().Buffer {
	case "xteve":
		go m.download(s)
	case "ffmpeg", "vlc":
		go m.thirdPartyBuffer(s)
	default:
		close(s.done)
	}

	logger.Info(fmt.Sprintf("Streaming Status:Playlist: %s - Tuner: %d / %d", p.name, len(p.streams), p.tuners))

	return s, true
}

// leave removes a client from its stream. When the last client left, the stream stops and frees its tuner.
func (m *Manager) leave(s *stream) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s.clients--
	logger.Info("Streaming Status:Client has terminated the connection")
	logger.Info(fmt.Sprintf("Streaming Status:Channel: %s (Clients: %d)", s.channelName, s.clients))

	if s.clients > 0 {
		return
	}

	var p = s.playlist
	delete(p.streams, s.id)
	close(s.stop)
	if len(p.streams) == 0 {
		delete(m.playlists, p.id)
	}

	logger.Info(fmt.Sprintf(
		"Streaming Status:Channel: %s - No client is using this channel anymore. Streaming Server connection has ended",
		s.channelName,
	))
	logger.Info(fmt.Sprintf("Streaming Status:Playlist: %s - Tuner: %d / %d", p.name, len(p.streams), p.tuners))
}

// Streams returns the number of streams in use per playlist.
func (m *Manager) Streams() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	var streams = make(map[string]int, len(m.playlists))
	for id, p := range m.playlists {
		streams[id] = len(p.streams)
	}
	return streams
}

// sendStreamLimit sends the "stream limit" video for 30 seconds.
func (m *Manager) sendStreamLimit(w http.ResponseWriter) {
	var video = m.cfg.StreamLimitVideo()
	if len(video) == 0 {
		return
	}

	w.Header().Set("Content-type", "video/mpeg")
	w.WriteHeader(http.StatusOK)

	var rc = http.NewResponseController(w)
	for range 59 {
		if _, err := w.Write(video); err != nil {
			logger.Debug(1, fmt.Sprintf("HTTP Response:%s", err))
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// newSegments returns the complete segments of the stream that are not in sent, in order. It reports false if the
// folder of the stream is gone.
func (m *Manager) newSegments(s *stream, sent []string) ([]string, bool) {
	var files, err = s.vfs.ReadDir(s.folder)
	if err != nil {
		if isNotExist(err) {
			return nil, false
		}
		m.cfg.Error(err, 0)
		return nil, true
	}

	var numbers []int
	for _, f := range files {
		if n, err := strconv.Atoi(strings.TrimSuffix(f.Name(), ".ts")); err == nil {
			numbers = append(numbers, n)
		}
	}
	slices.Sort(numbers)

	// The newest segment is still being written
	if len(numbers) < 2 {
		return nil, true
	}
	numbers = numbers[:len(numbers)-1]

	var segments []string
	for _, n := range numbers {
		var name = strconv.Itoa(n) + ".ts"
		if !slices.Contains(sent, name) && (len(sent) == 0 || n > segmentNumber(sent[len(sent)-1])) {
			segments = append(segments, name)
		}
	}
	return segments, true
}

func segmentNumber(name string) int {
	var n, _ = strconv.Atoi(strings.TrimSuffix(name, ".ts"))
	return n
}

func readFile(vfs avfs.VFS, name string) ([]byte, error) {
	var f, err = vfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer closeFile(f)
	return io.ReadAll(f)
}

// freeStreamID returns the lowest stream ID that is not in use.
func freeStreamID(streams map[int]*stream) int {
	for id := 0; ; id++ {
		if _, ok := streams[id]; !ok {
			return id
		}
	}
}

func md5Hex(s string) string {
	var sum = md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, avfs.ErrWinPathNotFound) ||
		errors.Is(err, avfs.ErrWinFileNotFound)
}
