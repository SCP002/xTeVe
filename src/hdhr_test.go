package src

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"testing"

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
	var oldSettings, oldFiles, oldStreams = Settings, System.File, Data.Streams.Active
	var oldURLs = Data.Cache.StreamingURLS
	t.Cleanup(func() {
		Settings, System.File, Data.Streams.Active = oldSettings, oldFiles, oldStreams
		Data.Cache.StreamingURLS = oldURLs
	})

	var dir = t.TempDir()
	System.File.PMS = filepath.Join(dir, "pms.json")
	System.File.URLS = filepath.Join(dir, "urls.json")
	// "Kids" already has an ID from an earlier run
	if err := saveMapToJSONFile(System.File.PMS, map[string]string{"Kids": "id-0"}); err != nil {
		t.Fatal(err)
	}
	Settings.EpgSource = "PMS"
	Data.Cache.StreamingURLS = make(map[string]StreamInfo)
	Data.Streams.Active = []any{
		m3uStream("News", "A"),
		m3uStream("Kids", "A"),
		m3uStream("News", "B"),
		m3uStream("News (2)", "C"), // a real channel with the name of a generated key
	}

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
