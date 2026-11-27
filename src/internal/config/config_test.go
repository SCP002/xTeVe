package config

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func testSettings() *Settings {
	var s = &Settings{Tuner: 2, Update: []string{"0000"}}
	s.Files.M3U = map[string]any{
		"M1": map[string]any{
			"name":          "Playlist",
			"tuner":         1.0,
			"compatibility": map[string]int{"streams": 10},
			"list":          []any{"a", map[string]any{"b": 1}},
		},
	}
	s.Filter = map[int64]any{0: map[string]any{"filter": "News"}}
	return s
}

func TestClone(t *testing.T) {
	var s = testSettings()
	var c = s.Clone()

	var entry = c.Files.M3U["M1"].(map[string]any)
	entry["name"] = "Changed"
	entry["compatibility"].(map[string]int)["streams"] = 99
	entry["list"].([]any)[1].(map[string]any)["b"] = 2
	c.Filter[0].(map[string]any)["filter"] = "Sport"
	c.Files.M3U["M2"] = map[string]any{}
	c.Update[0] = "1200"

	var original = s.Files.M3U["M1"].(map[string]any)
	if original["name"] != "Playlist" ||
		original["compatibility"].(map[string]int)["streams"] != 10 ||
		original["list"].([]any)[1].(map[string]any)["b"] != 1 ||
		s.Filter[0].(map[string]any)["filter"] != "News" ||
		len(s.Files.M3U) != 1 || s.Update[0] != "0000" {
		t.Errorf("changing the clone changed the original: %+v", s)
	}
}

func newTestStore(t *testing.T) (*Store, *[]*Settings) {
	t.Helper()
	var saved []*Settings
	var st = NewStore()
	st.SetSaver(func(s *Settings) error {
		if s.Tuner == 0 {
			s.Tuner = 1 // the saver completes defaults
		}
		saved = append(saved, s)
		return nil
	})
	return st, &saved
}

func TestUpdate(t *testing.T) {
	var st, saved = newTestStore(t)
	var before = st.Get()

	if err := st.Update(func(s *Settings) error { s.EpgSource = "XEPG"; return nil }); err != nil {
		t.Fatal(err)
	}

	var after = st.Get()
	if after.EpgSource != "XEPG" || after.Tuner != 1 {
		t.Errorf("settings = %+v, want the change and the default of the saver", after)
	}
	if before.EpgSource != "" {
		t.Errorf("the earlier snapshot was changed: %+v", before)
	}
	if len(*saved) != 1 || (*saved)[0] != after {
		t.Errorf("saved %d times, want once with the published settings", len(*saved))
	}
}

func TestUpdateFails(t *testing.T) {
	var st, saved = newTestStore(t)
	if err := st.Set(testSettings()); err != nil {
		t.Fatal(err)
	}
	var current = st.Get()

	var errChange = errors.New("invalid")
	var err = st.Update(func(s *Settings) error {
		s.Files.M3U["M1"].(map[string]any)["name"] = "Half done"
		return errChange
	})
	if !errors.Is(err, errChange) {
		t.Errorf("Update error = %v, want %v", err, errChange)
	}

	st.SetSaver(func(*Settings) error { return errors.New("disk full") })
	if err := st.Update(func(s *Settings) error { s.Tuner = 5; return nil }); err == nil {
		t.Error("Update with a failing saver succeeded")
	}

	if st.Get() != current || current.Files.M3U["M1"].(map[string]any)["name"] != "Playlist" || current.Tuner != 2 {
		t.Errorf("failed updates changed the settings: %+v", st.Get())
	}
	if len(*saved) != 1 {
		t.Errorf("saved %d times, want once (by Set)", len(*saved))
	}
}

func TestNoSaver(t *testing.T) {
	if err := NewStore().Update(func(*Settings) error { return nil }); !errors.Is(err, ErrNoSaver) {
		t.Errorf("Update without saver = %v, want ErrNoSaver", err)
	}
}

// Readers walk the maps of their snapshot while updates change the settings. Run with -race.
func TestConcurrentUse(t *testing.T) {
	var st, _ = newTestStore(t)
	if err := st.Set(testSettings()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 200 {
				var err = st.Update(func(s *Settings) error {
					s.Files.M3U[fmt.Sprintf("M%d-%d", g, i%5)] = map[string]any{"name": i}
					s.Files.M3U["M1"].(map[string]any)["counter"] = i
					return nil
				})
				if err != nil {
					t.Error(err)
					return
				}
			}
		})
		wg.Go(func() {
			for range 200 {
				for _, entry := range st.Get().Files.M3U {
					for range entry.(map[string]any) {
					}
				}
			}
		})
	}
	wg.Wait()

	if n := len(st.Get().Files.M3U); n != 1+4*5 {
		t.Errorf("playlists = %d, want 21", n)
	}
}
