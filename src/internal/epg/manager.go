// Package epg builds the XEPG database (xepg.json) from the active streams and the XMLTV files, maps the channels to
// XMLTV channels, and writes the XMLTV file of xTeVe (xteve.xml).
//
// A Manager runs one rebuild at a time and publishes its result as an immutable snapshot, so readers (web UI,
// lineup, M3U) never see a database that is half built. A published snapshot must not be changed.
package epg

import (
	"fmt"
	"maps"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/errcode"
	"xteve/src/internal/imgcache"
	"xteve/src/internal/logger"
	"xteve/src/internal/playlist"
	"xteve/src/internal/system"
)

// Config is what the EPG needs from the rest of xTeVe.
type Config struct {
	System *system.Info
	// Streams returns the active streams.
	Streams func() []playlist.Stream
	// XMLTVFiles returns the local copies of the XMLTV files.
	XMLTVFiles func() []string
	// XMLTVName returns the name of an XMLTV provider.
	XMLTVName func(id string) string
	// WriteM3U writes the M3U file of xTeVe, after the XMLTV file.
	WriteM3U func()
}

// Snapshot is the XEPG database with what was built along with it.
type Snapshot struct {
	// Channels by XEPG ID ("x-ID.12")
	Channels Channels
	// Count of active channels
	Count int64
	// XMLTV files used for the mapping
	Files []string
	// Mapping menu of the web UI
	Mapping Mapping
	// Images caches the logos and posters of the channels
	Images *imgcache.Cache
}

// Manager builds and holds the XEPG database.
type Manager struct {
	cfg Config

	// mu runs one rebuild at a time
	mu      sync.Mutex
	current atomic.Pointer[Snapshot]
	// pending is the newest mapping saved in the web UI while a rebuild was running
	pending atomic.Pointer[Channels]

	// writeMu writes xteve.xml and the M3U file one at a time
	writeMu sync.Mutex
	// imagesMu caches the images of one build at a time
	imagesMu sync.Mutex

	// cacheMu guards the channels of the XMLTV files, read once per file version
	cacheMu      sync.Mutex
	channelCache map[string]cachedChannels
}

type cachedChannels struct {
	size     int64
	modified time.Time
	channels map[string]MappingChannel
}

// build is the state of one rebuild. It is published as a snapshot at the end and not changed afterwards.
type build struct {
	m *Manager

	channels Channels
	count    int64
	files    []string
	mapping  Mapping
	images   *imgcache.Cache
	// cacheImages is the setting the image cache of the build was created with
	cacheImages bool

	streams       []playlist.Stream
	activeStreams []string
}

// NewManager returns a manager with an empty database.
func NewManager(cfg Config) *Manager {
	var m = &Manager{cfg: cfg, channelCache: make(map[string]cachedChannels)}
	m.current.Store(&Snapshot{Channels: Channels{}, Mapping: Mapping{}})
	return m
}

// Get returns the current database. It must not be changed.
func (m *Manager) Get() *Snapshot { return m.current.Load() }

func (m *Manager) newBuild() *build {
	var cacheImages = config.Get().CacheImages
	var images, err = imgcache.New(
		m.cfg.System.Folder.ImagesCache,
		fmt.Sprintf("%s://%s/images/", m.cfg.System.Protocol(), m.cfg.System.Domain()),
		cacheImages,
	)
	if err != nil {
		errcode.Report(err, 0)
	}

	var streams = m.cfg.Streams()
	var active = make([]string, 0, len(streams))
	for _, s := range streams {
		active = append(active, s["name"]+s["_file.m3u.id"])
	}

	return &build{m: m, images: images, cacheImages: cacheImages, streams: streams, activeStreams: active}
}

func (m *Manager) publish(b *build) {
	m.current.Store(&Snapshot{
		Channels: b.channels,
		Count:    b.count,
		Files:    b.files,
		Mapping:  b.mapping,
		Images:   b.images,
	})
}

// Rebuild builds the database from the active streams and the XMLTV files and publishes it. The XMLTV and M3U
// files are written afterwards in the background, then the images are cached if that is enabled.
func (m *Manager) Rebuild() {
	var endScan = m.cfg.System.BeginScan()

	m.mu.Lock()
	var b = m.newBuild()
	b.createXEPGMapping()
	if err := b.createXEPGDatabase(); err != nil {
		errcode.Report(err, 0)
	}
	if err := b.mapChannels(); err != nil {
		errcode.Report(err, 0)
	}
	b.cleanupXEPG()
	m.publish(b)
	m.mu.Unlock()

	go func() {
		defer endScan()
		m.writeFiles(b)
		logger.Info("XEPG:" + "Ready to use")
		m.cacheImages(b)

		if config.Get().ClearXMLTVCache {
			m.ClearXMLTVCache()
		}
	}()
}

// SaveMapping replaces the channels with the ones edited in the web UI, removes channels without stream and
// rebuilds the database in the background. While a rebuild runs, the newest saved mapping is applied after it.
func (m *Manager) SaveMapping(channels Channels) {
	if m.mu.TryLock() {
		m.applyMapping(channels)
		m.mu.Unlock()
		go m.Rebuild()
		return
	}

	m.pending.Store(&channels)
	go func() {
		m.mu.Lock()
		var latest = m.pending.Swap(nil)
		if latest != nil {
			m.applyMapping(*latest)
		}
		m.mu.Unlock()

		if latest != nil {
			m.Rebuild()
		}
	}()
}

// applyMapping publishes the edited channels. m.mu must be held.
func (m *Manager) applyMapping(channels Channels) {
	var current = m.Get()
	var b = m.newBuild()
	b.channels = maps.Clone(channels)
	b.files, b.mapping, b.images = current.Files, current.Mapping, current.Images
	b.cleanupXEPG()
	m.publish(b)
}

// WriteFiles writes the XMLTV and M3U files of the current database.
func (m *Manager) WriteFiles() {
	var current = m.Get()
	m.writeFiles(&build{
		m:        m,
		channels: current.Channels,
		count:    current.Count,
		files:    current.Files,
		mapping:  current.Mapping,
		images:   current.Images,
		streams:  m.cfg.Streams(),
	})
}

func (m *Manager) writeFiles(b *build) {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	if err := b.createXMLTVFile(); err != nil {
		errcode.Report(err, 0)
	}
	if m.cfg.WriteM3U != nil {
		m.cfg.WriteM3U()
	}
}

// cacheImages downloads the images of the build in the background and writes the files again with the cached
// images, unless that is disabled or already running.
func (m *Manager) cacheImages(b *build) {
	// Only a cache created with caching enabled has a queue of images
	if b.images == nil || !b.cacheImages {
		return
	}

	go func() {
		// The images of a newer build are cached after the running caching
		m.imagesMu.Lock()
		defer m.imagesMu.Unlock()
		m.cfg.System.ImageCachingInProgress.Store(true)
		defer m.cfg.System.ImageCachingInProgress.Store(false)

		b.images.RLock()
		var queued = len(b.images.Queue)
		b.images.RUnlock()
		logger.Info(fmt.Sprintf("Image Caching:Images are cached (%d)", queued))

		if err := b.images.Image.Caching(); err != nil {
			errcode.Report(err, 0)
		}
		if err := b.images.Image.Remove(); err != nil {
			errcode.Report(err, 0)
		}
		logger.Info("Image Caching:Done")

		m.WriteFiles()
	}()
}

// ClearXMLTVCache forgets the channels read from the XMLTV files, they are read again for the next rebuild.
func (m *Manager) ClearXMLTVCache() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.channelCache = make(map[string]cachedChannels)
}

// ForgetXMLTV forgets the channels of one XMLTV file, e.g. after it was downloaded again.
func (m *Manager) ForgetXMLTV(file string) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	delete(m.channelCache, file)
}

// xmltvChannels returns the channels of an XMLTV file for the mapping menu. They are read again when the file
// changed (size or time).
func (m *Manager) xmltvChannels(file string) (map[string]MappingChannel, error) {
	var info, err = os.Stat(file)
	if err != nil {
		return nil, errcode.With(fmt.Errorf("local copy of the file can not be read: %w", err), 1004)
	}

	m.cacheMu.Lock()
	var cached, ok = m.channelCache[file]
	m.cacheMu.Unlock()
	if ok && cached.size == info.Size() && cached.modified.Equal(info.ModTime()) {
		return cached.channels, nil
	}

	var channels = make(map[string]MappingChannel)
	err = readXMLTV(file, func(c *Channel) error {
		channels[c.ID] = MappingChannel{ID: c.ID, DisplayNames: c.DisplayNames, Icon: c.IconSrc()}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}

	m.cacheMu.Lock()
	m.channelCache[file] = cachedChannels{size: info.Size(), modified: info.ModTime(), channels: channels}
	m.cacheMu.Unlock()
	return channels, nil
}

// Reset empties the database and xepg.json.
func (m *Manager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b = &build{m: m, channels: Channels{}, mapping: Mapping{}}
	m.publish(b)
	return saveMapToJSONFile(m.cfg.System.File.XEPG, b.channels)
}

// ImageURL returns the URL of an image: the cached one, or src if images are not cached (or there is no cache yet).
func ImageURL(images *imgcache.Cache, src string) string {
	if images == nil {
		return src
	}
	return images.Image.GetURL(src)
}

func (m *Manager) xmltvName(id string) string {
	if m.cfg.XMLTVName == nil {
		return id
	}
	return m.cfg.XMLTVName(id)
}
