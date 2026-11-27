package playlist

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeSource(t *testing.T, name, content string) string {
	t.Helper()
	var path = filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

const testM3U = `#EXTM3U
#EXTINF:-1 tvg-id="bbc" group-title="News",BBC
http://example.com/bbc
#EXTINF:-1 group-title="News",CNN
http://example.com/cnn
#EXTINF:-1 group-title="Sport",ESPN
http://example.com/espn
#EXTINF:-1 group-title="Sport",ESPN copy
http://example.com/espn
`

const testHDHR = `[
	{"GuideName": "Local 1", "GuideNumber": "1.1", "URL": "http://tuner/auto/v1.1"},
	{"GuideNumber": "2.1", "URL": "http://tuner/auto/v2.1"}
]`

func testSources(t *testing.T) []Source {
	return []Source{
		{ID: "M1", Type: "m3u", Name: "Playlist", Path: writeSource(t, "M1.m3u", testM3U)},
		{ID: "H1", Type: "hdhr", Name: "Tuner", Path: writeSource(t, "H1.json", testHDHR)},
		{ID: "M2", Type: "m3u", Name: "Broken", Path: writeSource(t, "M2.m3u", "not a playlist")},
	}
}

func names(streams []Stream) []string {
	var n []string
	for _, s := range streams {
		n = append(n, s["name"])
	}
	slices.Sort(n)
	return n
}

func TestBuildWithoutFilters(t *testing.T) {
	var snapshot, compatibilities, err = Build(testSources(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	// The broken playlist is skipped, the channel without a name of the tuner too
	var want = []string{"BBC", "CNN", "ESPN", "ESPN copy", "Local 1"}
	if got := names(snapshot.All); !slices.Equal(got, want) {
		t.Errorf("All = %v, want %v", got, want)
	}
	if got := names(snapshot.Active); !slices.Equal(got, want) || len(snapshot.Inactive) != 0 {
		t.Errorf("Active = %v, Inactive = %v; without filters all streams are active", got, names(snapshot.Inactive))
	}
	if snapshot.PreviewInactive == nil || len(snapshot.PreviewActive) != 5 ||
		snapshot.PreviewActive[0] != "BBC [News]" {
		t.Errorf("previews = %q, %q", snapshot.PreviewActive, snapshot.PreviewInactive)
	}

	var bbc = snapshot.All[slices.IndexFunc(snapshot.All, func(s Stream) bool { return s["name"] == "BBC" })]
	if bbc["_file.m3u.id"] != "M1" || bbc["_file.m3u.name"] != "Playlist" || bbc["_file.m3u.path"] == "" {
		t.Errorf("BBC = %v, want the source keys", bbc)
	}
	var local = snapshot.All[slices.IndexFunc(snapshot.All, func(s Stream) bool { return s["name"] == "Local 1" })]
	if local["group-title"] != "Tuner" || local["ID-H1"] != "1.1" || local["_uuid.key"] != "ID-H1" {
		t.Errorf("tuner channel = %v", local)
	}

	// Groups of all playlists, text and value in the same order
	var wantGroups = []Group{{"News (2)", "News"}, {"Sport (2)", "Sport"}, {"Tuner (1)", "Tuner"}}
	if !slices.Equal(snapshot.Groups, wantGroups) {
		t.Errorf("Groups = %v, want %v", snapshot.Groups, wantGroups)
	}

	if c := compatibilities["M1"]; c["streams"] != 4 || c["tvg.id"] != 25 || c["group.title"] != 100 {
		t.Errorf("compatibility of M1 = %v", c)
	}
	if _, ok := compatibilities["M2"]; ok {
		t.Error("a broken playlist got a compatibility")
	}
}

func TestBuildWithFilters(t *testing.T) {
	var snapshot, _, err = Build(testSources(t), Options{
		Filters:               map[int64]any{0: groupFilter("News", "", "CNN")},
		DisallowURLDuplicates: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := names(snapshot.All); slices.Contains(got, "ESPN copy") {
		t.Errorf("All = %v, want the duplicated URL removed", got)
	}
	if got := names(snapshot.Active); !slices.Equal(got, []string{"BBC"}) {
		t.Errorf("Active = %v, want [BBC]", got)
	}
	if got := names(snapshot.Inactive); !slices.Equal(got, []string{"CNN", "ESPN", "Local 1"}) {
		t.Errorf("Inactive = %v", got)
	}
	if len(snapshot.Filters) != 1 || snapshot.Filters[0].Rule != "News !{CNN}" {
		t.Errorf("Filters = %+v", snapshot.Filters)
	}
	if snapshot.Active[0]["_preserve-mapping"] != "true" || snapshot.Active[0]["_starting-channel"] != "1000" {
		t.Errorf("active stream = %v, want the marks of the filter", snapshot.Active[0])
	}
}

func TestStore(t *testing.T) {
	var st Store
	if s := st.Get(); s == nil || len(s.All) != 0 || s.PreviewActive == nil {
		t.Errorf("empty store = %+v", s)
	}

	var snapshot = &Snapshot{All: []Stream{{"name": "A"}}}
	st.Set(snapshot)
	if st.Get() != snapshot {
		t.Error("Get does not return the published snapshot")
	}
}
