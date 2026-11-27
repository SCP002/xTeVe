package src

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// Uploaded logos stay in their folder and are images.
func TestUploadLogo(t *testing.T) {
	var dir = t.TempDir()
	var upload = filepath.Join(dir, "upload") + string(os.PathSeparator)
	if err := os.Mkdir(upload, 0755); err != nil {
		t.Fatal(err)
	}
	var old = System.Folder.ImagesUpload
	System.Folder.ImagesUpload = upload
	t.Cleanup(func() { System.Folder.ImagesUpload = old })

	var data = "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	for _, c := range []struct {
		name, stored string
		ok           bool
	}{
		{"logo.png", "logo.png", true},
		{"LOGO.JPG", "LOGO.JPG", true},
		{"../settings.json", "", false},
		{"../../escape.png", "escape.png", true},
		{`..\..\escape2.png`, "escape2.png", true},
		{"script.svg", "", false},
		{"", "", false},
		{"..", "", false},
	} {
		var url, err = uploadLogo(data, c.name)
		if (err == nil) != c.ok {
			t.Errorf("%q: err %v, want ok = %v", c.name, err, c.ok)
			continue
		}
		if c.ok {
			if _, err := os.Stat(filepath.Join(upload, c.stored)); err != nil || filepath.Base(url) != c.stored {
				t.Errorf("%q: stored as %s (%v), want %s in the upload folder", c.name, url, err, c.stored)
			}
		}
	}

	// Nothing was written outside the folder
	var outside, _ = filepath.Glob(filepath.Join(dir, "*"))
	if len(outside) != 1 {
		t.Errorf("files next to the upload folder: %v", outside)
	}
}
