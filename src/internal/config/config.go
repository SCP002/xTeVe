// Package config holds the settings of xTeVe (settings.json).
//
// The settings are published as immutable snapshots: Get returns the current one without locking, and Update changes
// a deep copy, saves it and publishes it as the new snapshot. A snapshot returned by Get must never be changed, its
// maps are shared with other readers.
package config

import (
	"errors"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// Settings is the content of settings.json.
type Settings struct {
	API                   bool     `json:"api"`
	AuthenticationAPI     bool     `json:"authentication.api"`
	AuthenticationM3U     bool     `json:"authentication.m3u"`
	AuthenticationPMS     bool     `json:"authentication.pms"`
	AuthenticationWEB     bool     `json:"authentication.web"`
	AuthenticationXML     bool     `json:"authentication.xml"`
	BackupKeep            int      `json:"backup.keep"`
	BackupPath            string   `json:"backup.path"`
	Branch                string   `json:"git.branch,omitempty"`
	Buffer                string   `json:"buffer"`
	BufferSize            int      `json:"buffer.size.kb"`
	BufferTimeout         float64  `json:"buffer.timeout"`
	CacheImages           bool     `json:"cache.images"`
	ClearXMLTVCache       bool     `json:"clearXMLTVCache"`
	DefaultMissingEPG     string   `json:"defaultMissingEPG"`
	DisallowURLDuplicates bool     `json:"disallowURLDuplicates"`
	EnableMappedChannels  bool     `json:"enableMappedChannels"`
	EpgSource             string   `json:"epgSource"`
	FFmpegOptions         string   `json:"ffmpeg.options"`
	FFmpegPath            string   `json:"ffmpeg.path"`
	VLCOptions            string   `json:"vlc.options"`
	VLCPath               string   `json:"vlc.path"`
	FileM3U               []string `json:"file,omitempty"` // In the Wizard, the M3U is saved in a Slice
	// Old Storage System of the provider XML File Slice (Required for the conversion to the new one)
	FileXMLTV []string `json:"xmltv,omitempty"`

	Files struct {
		HDHR  map[string]any `json:"hdhr"`
		M3U   map[string]any `json:"m3u"`
		XMLTV map[string]any `json:"xmltv"`
	} `json:"files"`

	FilesUpdate bool          `json:"files.update"`
	Filter      map[int64]any `json:"filter"`
	// IP chosen in web client. Used to form m3u and xml files.
	HostIP                    string   `json:"hostIP"`
	Key                       string   `json:"key,omitempty"`
	Language                  string   `json:"language"`
	LogEntriesRAM             int      `json:"log.entries.ram"`
	M3U8AdaptiveBandwidthMBPS int      `json:"m3u8.adaptive.bandwidth.mbps"`
	MappingFirstChannel       float64  `json:"mapping.first.channel"`
	Port                      string   `json:"port"`
	SSDP                      bool     `json:"ssdp"`
	StoreBufferInRAM          bool     `json:"storeBufferInRAM"`
	TempPath                  string   `json:"temp.path"`
	TLSMode                   bool     `json:"tlsMode"`
	Tuner                     int      `json:"tuner"`
	Update                    []string `json:"update"`
	UserAgent                 string   `json:"user.agent"`
	UUID                      string   `json:"uuid"`
	UDPxy                     string   `json:"udpxy"`
	Version                   string   `json:"version"`
	XepgReplaceMissingImages  bool     `json:"xepg.replace.missing.images"`
	XteveAutoUpdate           bool     `json:"xteveAutoUpdate"`
}

// Clone returns a deep copy of s: changing the copy, including its maps and slices, does not change s.
func (s *Settings) Clone() *Settings {
	var c = *s
	c.FileM3U = slices.Clone(s.FileM3U)
	c.FileXMLTV = slices.Clone(s.FileXMLTV)
	c.Update = slices.Clone(s.Update)
	c.Files.HDHR = cloneMap(s.Files.HDHR)
	c.Files.M3U = cloneMap(s.Files.M3U)
	c.Files.XMLTV = cloneMap(s.Files.XMLTV)
	c.Filter = cloneMap(s.Filter)
	return &c
}

// cloneMap copies a map with values as decoded from JSON or built in code: nested maps and slices are copied too.
func cloneMap[K comparable](m map[K]any) map[K]any {
	if m == nil {
		return nil
	}
	var c = make(map[K]any, len(m))
	for k, v := range m {
		c[k] = cloneValue(v)
	}
	return c
}

func cloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return cloneMap(v)
	case map[int64]any:
		return cloneMap(v)
	case []any:
		var c = make([]any, len(v))
		for i, e := range v {
			c[i] = cloneValue(e)
		}
		return c
	case []string:
		return slices.Clone(v)
	case map[string]int:
		return maps.Clone(v)
	case map[string]string:
		return maps.Clone(v)
	default:
		// Scalars, and values that are never changed in place
		return v
	}
}

// ErrNoSaver is returned by Update and Set before SetSaver was called.
var ErrNoSaver = errors.New("config: settings cannot be saved yet")

// Store holds the current settings.
type Store struct {
	current atomic.Pointer[Settings]

	mu   sync.Mutex
	save func(*Settings) error
}

// NewStore returns a store with empty settings.
func NewStore() *Store {
	var st = &Store{}
	st.current.Store(&Settings{})
	return st
}

// Get returns the current settings. They must not be changed.
func (st *Store) Get() *Settings { return st.current.Load() }

// SetSaver sets the function that completes and saves new settings before Update and Set publish them. It may
// change the settings it gets, e.g. to fill in defaults.
func (st *Store) SetSaver(save func(*Settings) error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.save = save
}

// Update changes a copy of the current settings with change, saves the copy and publishes it. If change or saving
// fails, the current settings stay as they are. Updates run one at a time, change must not call Update or Set.
func (st *Store) Update(change func(*Settings) error) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.save == nil {
		return ErrNoSaver
	}

	var next = st.current.Load().Clone()
	if err := change(next); err != nil {
		return err
	}
	if err := st.save(next); err != nil {
		return err
	}
	st.current.Store(next)
	return nil
}

// Set saves a copy of s and publishes it, replacing the current settings.
func (st *Store) Set(s *Settings) error {
	return st.Update(func(next *Settings) error {
		*next = *s.Clone()
		return nil
	})
}

var std = NewStore()

// Default returns the store used by the package level functions.
func Default() *Store { return std }

// Get returns the current settings of the default store, see Store.Get.
func Get() *Settings { return std.Get() }

// Update changes the settings of the default store, see Store.Update.
func Update(change func(*Settings) error) error { return std.Update(change) }
