package hdhr

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestPMSNumbers(t *testing.T) {
	var file = filepath.Join(t.TempDir(), "pms.json")
	if err := saveJSON(file, map[string]string{"Kids": "id-0"}); err != nil {
		t.Fatal(err)
	}

	var names = []string{"News", "Kids", "News", "News (2)"}
	var first, err = NewPMSNumbers(file).Numbers(names)
	if err != nil {
		t.Fatal(err)
	}
	if first[1] != "id-0" || len(slices.Compact(slices.Sorted(slices.Values(first)))) != 4 {
		t.Errorf("numbers = %v, want 4 different ones and the stored id-0 for Kids", first)
	}

	// Stored for the next start
	again, err := NewPMSNumbers(file).Numbers(names)
	if err != nil || !slices.Equal(again, first) {
		t.Errorf("after a restart: numbers = %v (err %v), want %v", again, err, first)
	}
}

// Run with -race.
func TestPMSNumbersConcurrent(t *testing.T) {
	var p = NewPMSNumbers(filepath.Join(t.TempDir(), "pms.json"))
	var results = make([][]string, 8)

	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			var names []string
			for n := range 50 {
				names = append(names, fmt.Sprintf("Channel %d", n))
			}
			var numbers, err = p.Numbers(names)
			if err != nil {
				t.Error(err)
			}
			results[i] = numbers
		})
	}
	wg.Wait()

	for i := range results {
		if !slices.Equal(results[i], results[0]) {
			t.Errorf("lineup %d got other numbers: %v", i, results[i][:3])
		}
	}
}

func TestURLs(t *testing.T) {
	var file = filepath.Join(t.TempDir(), "urls.json")
	var u = NewURLs(file)

	id, err := u.Add("M1", "1000", "BBC", "http://example.com/bbc\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := u.Add("M1", "1001", "BBC One", "http://example.com/bbc\r\n"); again != id {
		t.Errorf("ID changed for the same stream: %s, %s", id, again)
	}
	if other, _ := u.Add("M2", "1000", "BBC", "http://example.com/bbc\r\n"); other == id {
		t.Error("the same ID for another playlist")
	}

	if err := u.Save(); err != nil {
		t.Fatal(err)
	}

	// Read again after a restart
	s, err := NewURLs(file).Get(id)
	if err != nil || s.URL != "http://example.com/bbc" || s.Name != "BBC" || s.PlaylistID != "M1" {
		t.Errorf("stream = %+v, err %v", s, err)
	}
	if _, err := u.Get("unknown"); err == nil {
		t.Error("no error for an unknown ID")
	}
}

func TestLineupJSON(t *testing.T) {
	var empty, err = LineupJSON(nil)
	if err != nil || strings.TrimSpace(string(empty)) != "[]" {
		t.Errorf("empty lineup = %q, want []", empty)
	}

	content, err := LineupJSON([]LineupStream{{GuideNumber: "10"}, {GuideNumber: "abc"}, {GuideNumber: "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(content), `"2"`) > strings.Index(string(content), `"10"`) ||
		strings.Index(string(content), `"10"`) > strings.Index(string(content), `"abc"`) {
		t.Errorf("lineup is not sorted by guide number:\n%s", content)
	}
}

func TestDevice(t *testing.T) {
	var d = Device{BaseURL: "http://1.2.3.4:34400", DeviceID: "id:2", AppName: "xteve", Name: "xTeVe", Version: "1",
		TunerCount: 2}

	discover, err := DiscoverJSON(d)
	if err != nil || !strings.Contains(string(discover), `"LineupURL": "http://1.2.3.4:34400/lineup.json"`) ||
		!strings.Contains(string(discover), `"TunerCount": 2`) {
		t.Errorf("discover.json = %s, err %v", discover, err)
	}

	device, err := DeviceXML(d)
	if err != nil || !strings.Contains(string(device), "<UDN>uuid:id:2</UDN>") ||
		!strings.HasPrefix(string(device), "<?xml") {
		t.Errorf("device.xml = %s, err %v", device, err)
	}

	status, err := LineupStatusJSON(true)
	if err != nil || !strings.Contains(string(status), `"ScanInProgress": 1`) {
		t.Errorf("lineup_status.json = %s, err %v", status, err)
	}
}
