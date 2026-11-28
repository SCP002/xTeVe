package src

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// /xmltv/ serves only the XMLTV files of xTeVe, not the other files of the data folder like the downloaded
// playlists, whose stream URLs may contain credentials.
func TestServeXMLTVOnly(t *testing.T) {
	setSettings(t, func(s *SettingsStruct) { s.AuthenticationXML = false })
	var dir = t.TempDir() + string(os.PathSeparator)
	var old = System.Folder.Data
	System.Folder.Data = dir
	t.Cleanup(func() { System.Folder.Data = old })

	for name, content := range map[string]string{
		"xteve.xml":                "<tv></tv>",
		"xteve.xml.gz":             "gz",
		"MBONMGMS5T61YF7RYBLW.m3u": "#EXTM3U\nhttp://user:secret@provider/1.ts",
		"XAJ5GDOG5NH03BJSIHH9.xml": "<tv></tv>",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	for path, want := range map[string]int{
		"/xmltv/xteve.xml":                200,
		"/xmltv/xteve.xml.gz":             200,
		"/xmltv/MBONMGMS5T61YF7RYBLW.m3u": 404,
		"/xmltv/XAJ5GDOG5NH03BJSIHH9.xml": 404,
		"/xmltv/../settings.json":         404,
	} {
		var w = httptest.NewRecorder()
		xTeVe(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
}
