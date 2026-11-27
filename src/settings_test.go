package src

import (
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
