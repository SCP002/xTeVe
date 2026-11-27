package src

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"xteve/src/internal/playlist"

	"github.com/samber/lo"
)

func TestGuideNumberLess(t *testing.T) {
	var numbers = []string{"10", "abc", "2", "1000.5", "", "1.5", "Zed"}
	sort.SliceStable(numbers, func(i, j int) bool { return guideNumberLess(numbers[i], numbers[j]) })

	var want = []string{"1.5", "2", "10", "1000.5", "", "Zed", "abc"}
	if !slices.Equal(numbers, want) {
		t.Errorf("sorted = %q, want %q", numbers, want)
	}
}

func TestGetLineupPMSDuplicateNames(t *testing.T) {
	var oldFiles, oldStreams = System.File, streamsDB.Get()
	var oldURLs = Data.Cache.StreamingURLS
	t.Cleanup(func() {
		System.File = oldFiles
		streamsDB.Set(oldStreams)
		Data.Cache.StreamingURLS = oldURLs
	})

	var dir = t.TempDir()
	System.File.PMS = filepath.Join(dir, "pms.json")
	System.File.URLS = filepath.Join(dir, "urls.json")
	// "Kids" already has an ID from an earlier run
	if err := saveMapToJSONFile(System.File.PMS, map[string]string{"Kids": "id-0"}); err != nil {
		t.Fatal(err)
	}
	setSettings(t, func(s *SettingsStruct) { s.EpgSource = "PMS" })
	Data.Cache.StreamingURLS = make(map[string]StreamInfo)
	streamsDB.Set(&playlist.Snapshot{Active: []playlist.Stream{
		m3uStream("News", "A"),
		m3uStream("Kids", "A"),
		m3uStream("News", "B"),
		m3uStream("News (2)", "C"), // a real channel with the name of a generated key
	}})

	var guideNumbers = func() []string {
		content, err := getLineup()
		if err != nil {
			t.Fatal(err)
		}
		var lineup []LineupStream
		if err := json.Unmarshal(content, &lineup); err != nil {
			t.Fatal(err)
		}
		var numbers []string
		for _, s := range lineup {
			numbers = append(numbers, s.GuideNumber)
		}
		return numbers
	}

	var first = guideNumbers()
	if len(first) != 4 || len(lo.Uniq(first)) != 4 {
		t.Fatalf("guide numbers = %v, want 4 different ones", first)
	}
	if !slices.Contains(first, "id-0") {
		t.Errorf("guide numbers = %v, want the stored id-0 of Kids", first)
	}

	var stored, err = loadJSONFileToMap(System.File.PMS)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 4 {
		t.Errorf("pms.json = %v, want 4 IDs", stored)
	}

	if again := guideNumbers(); !slices.Equal(again, first) {
		t.Errorf("second run: guide numbers = %v, want %v", again, first)
	}
}

// m3uStream returns a stream of the playlist "M1".
func m3uStream(name, group string, extra ...string) map[string]string {
	var s = map[string]string{
		"_file.m3u.id": "M1",
		"name":         name,
		"group-title":  group,
		"url":          "http://example.com/" + name + "/" + group,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		s[extra[i]] = extra[i+1]
	}
	return s
}
