package epg

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xteve/src/internal/config"
	"xteve/src/internal/playlist"
	"xteve/src/internal/system"
)

const testXMLTV = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="bbc.uk"><display-name>BBC One</display-name></channel>
  <channel id="cnn"><display-name>CNN</display-name></channel>
  <programme channel="bbc.uk" start="20260101120000 +0000" stop="20260101130000 +0000">
    <title lang="en">News at Noon</title>
  </programme>
</tv>`

// managerTest returns a manager with an XMLTV file "X1.xml" and the streams, all files in a temporary folder.
func managerTest(t *testing.T, streams ...playlist.Stream) (*Manager, *system.Info) {
	t.Helper()

	var old = config.Get()
	config.Default().SetSaver(func(*config.Settings) error { return nil })
	t.Cleanup(func() {
		if err := config.Default().Set(old); err != nil {
			t.Error(err)
		}
	})
	setSettings(t, func(s *config.Settings) {
		s.EpgSource = "XEPG"
		s.MappingFirstChannel = 1000
		s.DefaultMissingEPG = "-"
		s.EnableMappedChannels = true
		s.CacheImages = false
		s.Files.M3U = config.Providers{"M1": {Name: "Playlist"}}
	})

	var dir = t.TempDir()
	var info = &system.Info{Name: "xTeVe", Version: "test"}
	info.Folder.Data = dir + string(os.PathSeparator)
	info.Folder.ImagesCache = filepath.Join(dir, "cache") + string(os.PathSeparator)
	info.File.XEPG = filepath.Join(dir, "xepg.json")
	info.File.XML = filepath.Join(dir, "xteve.xml")
	info.Compressed.GZxml = filepath.Join(dir, "xteve.xml.gz")

	if err := os.WriteFile(filepath.Join(dir, "X1.xml"), []byte(testXMLTV), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveMapToJSONFile(info.File.XEPG, Channels{}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var current = streams
	var m = NewManager(Config{
		System: info,
		Streams: func() []playlist.Stream {
			mu.Lock()
			defer mu.Unlock()
			return current
		},
		XMLTVFiles: func() []string { return []string{filepath.Join(dir, "X1.xml")} },
		XMLTVName:  func(string) string { return "Guide" },
	})
	return m, info
}

// waitScan waits until the rebuild including the files has finished.
func waitScan(t *testing.T, info *system.Info) {
	t.Helper()
	var end = time.Now().Add(10 * time.Second)
	for info.Scanning() {
		if time.Now().After(end) {
			t.Fatal("the rebuild did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRebuild(t *testing.T) {
	var bbc = m3uStream("BBC One", "News", "tvg-id", "bbc.uk")
	var other = m3uStream("Unknown", "News")
	var m, info = managerTest(t, bbc, other)

	m.Rebuild()
	waitScan(t, info)

	var snapshot = m.Get()
	if len(snapshot.Channels) != 2 || snapshot.Count != 1 {
		t.Errorf("channels = %d, active = %d; want 2 and 1", len(snapshot.Channels), snapshot.Count)
	}
	if _, ok := snapshot.Mapping["X1.xml"]["bbc.uk"]; !ok {
		t.Errorf("mapping menu = %v", snapshot.Mapping)
	}

	var xml, err = os.ReadFile(info.File.XML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xml), "News at Noon") || !strings.Contains(string(xml), "<display-name>BBC One") {
		t.Errorf("xteve.xml =\n%s", xml)
	}

	gz, err := os.Open(info.Compressed.GZxml)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gz.Close() }()
	r, err := gzip.NewReader(gz)
	if err != nil {
		t.Fatal(err)
	}
	unzipped, err := io.ReadAll(r)
	if err != nil || string(unzipped) != string(xml) {
		t.Errorf("xteve.xml.gz differs from xteve.xml (err %v)", err)
	}
}

func TestSaveMapping(t *testing.T) {
	var m, info = managerTest(t, m3uStream("BBC One", "News", "tvg-id", "bbc.uk"))
	m.Rebuild()
	waitScan(t, info)

	var edited = func(name string) Channels {
		var channels = make(Channels)
		for id, channel := range m.Get().Channels {
			channel.XName = name
			channels[id] = channel
		}
		return channels
	}

	// Saved while a rebuild runs: the newest mapping wins
	m.mu.Lock()
	m.SaveMapping(edited("First"))
	m.SaveMapping(edited("Second"))
	m.mu.Unlock()

	var end = time.Now().Add(10 * time.Second)
	for {
		var names []string
		for _, channel := range m.Get().Channels {
			names = append(names, channel.XName)
		}
		if len(names) == 1 && names[0] == "Second" {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("channel names = %v, want [Second]", names)
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitScan(t, info)
}

// Readers use snapshots while rebuilds and saved mappings replace them. Run with -race.
func TestConcurrentRebuilds(t *testing.T) {
	var m, info = managerTest(t, m3uStream("BBC One", "News", "tvg-id", "bbc.uk"), m3uStream("CNN", "News"))

	var stop = make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				var s = m.Get()
				for _, c := range s.Channels {
					_ = mapToJSON(c)
				}
				for range s.Mapping {
				}
			}
		})
	}

	var writers sync.WaitGroup
	for i := range 3 {
		writers.Go(func() {
			m.Rebuild()
			if i == 1 {
				m.SaveMapping(m.Get().Channels)
			}
			m.WriteFiles()
		})
	}
	writers.Wait()
	waitScan(t, info)
	close(stop)
	readers.Wait()

	if len(m.Get().Channels) != 2 {
		t.Errorf("channels = %d, want 2", len(m.Get().Channels))
	}

	// The files written at the same time are complete, no temporary file is left
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if _, _, err := Count(info.File.XML); err != nil {
		t.Errorf("xteve.xml: %v", err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(filepath.Dir(info.File.XML), "*.tmp")); len(tmp) != 0 {
		t.Errorf("temporary files left: %v", tmp)
	}
}
