package src

import (
	"encoding/json"
	"testing"

	"xteve/src/internal/config"
)

// setSettings changes the current settings for one test and restores them after it. The settings are not written
// to a file.
func setSettings(t *testing.T, change func(s *SettingsStruct)) {
	t.Helper()

	var old = config.Get()
	config.Default().SetSaver(func(*SettingsStruct) error { return nil })
	t.Cleanup(func() {
		if err := config.Default().Set(old); err != nil {
			t.Error(err)
		}
	})

	if err := config.Update(func(s *SettingsStruct) error { change(s); return nil }); err != nil {
		t.Fatal(err)
	}
}

// The keys of settings.json replace the defaults, missing keys keep them.
func TestDefaultSettings(t *testing.T) {
	var settings = defaultSettings()
	var file = `{"tuner": 3, "epgSource": "XEPG", "files": {"m3u": {"M1": {"name": "Playlist", "tuner": "2"}}}}`
	if err := json.Unmarshal([]byte(file), &settings); err != nil {
		t.Fatal(err)
	}

	if settings.Tuner != 3 || settings.EpgSource != "XEPG" {
		t.Errorf("tuner %d, EPG source %q: want the values of the file", settings.Tuner, settings.EpgSource)
	}
	if settings.Port != "34400" || settings.Buffer != "-" || !settings.XteveAutoUpdate || len(settings.Update) != 1 {
		t.Errorf("settings = %+v, want the defaults for missing keys", settings)
	}
	if settings.Files.M3U["M1"].Tuner != 2 || settings.Files.XMLTV == nil || settings.Filter == nil {
		t.Errorf("files = %+v, filter = %v", settings.Files, settings.Filter)
	}
}
