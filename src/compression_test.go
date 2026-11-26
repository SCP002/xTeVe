package src

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeTestZIP(t *testing.T, files map[string]string) string {
	t.Helper()

	var archive = filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	return archive
}

func TestExtractZIP(t *testing.T) {
	var archive = writeTestZIP(t, map[string]string{"settings.json": "{}", "data/a.m3u": "#EXTM3U"})
	var target = t.TempDir()

	if err := os.MkdirAll(filepath.Join(target, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := extractZIP(archive, target); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(target, "data", "a.m3u"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "#EXTM3U" {
		t.Errorf("extracted content = %q", content)
	}
}

func TestExtractZIPRejectsPathTraversal(t *testing.T) {
	var archive = writeTestZIP(t, map[string]string{"../evil.txt": "x"})
	var parent = t.TempDir()
	var target = filepath.Join(parent, "restore")

	if err := extractZIP(archive, target); err == nil {
		t.Fatal("expected an error for a path outside of the target folder")
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("file was written outside of the target folder (stat error: %v)", err)
	}
}
