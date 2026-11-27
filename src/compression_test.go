package src

import (
	"archive/zip"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
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

// backupTest creates a config folder with the files xTeVe backs up, sets the
// System folders the way config.go does and restores them after the test.
// It returns the files by their path relative to the config folder.
func backupTest(t *testing.T) map[string]string {
	t.Helper()

	var oldFolder, oldTLS = System.Folder, Settings.TLSMode
	t.Cleanup(func() { System.Folder, Settings.TLSMode = oldFolder, oldTLS })

	var sep = string(os.PathSeparator)
	System.Folder.Config = t.TempDir() + sep
	System.Folder.Data = System.Folder.Config + "data" + sep
	System.Folder.Certificates = System.Folder.Config + "certificates" + sep
	System.Folder.ImagesUpload = System.Folder.Data + "images" + sep
	System.Folder.Backup = System.Folder.Config + "backup" + sep
	System.Folder.Temp = t.TempDir() + sep
	Settings.TLSMode = true

	var files = map[string]string{
		"data/M1.m3u":               "#EXTM3U",
		"data/X1.xml":               "<tv></tv>",
		"data/images/logo.png":      "png",
		"data/images/sub/inner.png": "inner",
		"certificates/xteve.crt":    "crt",
		"certificates/xteve.key":    "key",
	}
	for _, f := range SystemFiles {
		files[f] = "{\"file\": \"" + f + "\"}"
	}
	for name, content := range files {
		var path = filepath.Join(System.Folder.Config, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Not part of any backup
	if err := os.MkdirAll(System.Folder.Backup, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(System.Folder.Config, "other.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	return files
}

// checkArchive checks the entry names of a ZIP archive, extracts it and
// compares the result with want.
func checkArchive(t *testing.T, archive string, want map[string]string) {
	t.Helper()

	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range reader.File {
		// ZIP entries must use "/" and be relative, or other tools and
		// platforms cannot extract them
		if strings.Contains(f.Name, `\`) || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "//") {
			t.Errorf("invalid entry name %q", f.Name)
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	var target = t.TempDir()
	if err := extractZIP(archive, target); err != nil {
		t.Fatal(err)
	}

	var got = make(map[string]string)
	err = filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(target, path)
		got[filepath.ToSlash(rel)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Errorf("extracted files = %v\nwant %v", got, want)
	}
}

func TestBackupRoundTrip(t *testing.T) {
	var files = backupTest(t)

	archive, err := xteveBackup()
	if err != nil {
		t.Fatal(err)
	}
	checkArchive(t, filepath.Join(System.Folder.Temp, archive), files)
}

func TestAutoBackupRoundTrip(t *testing.T) {
	var files = backupTest(t)
	var oldPath, oldKeep = Settings.BackupPath, Settings.BackupKeep
	t.Cleanup(func() { Settings.BackupPath, Settings.BackupKeep = oldPath, oldKeep })
	Settings.BackupPath, Settings.BackupKeep = "", 2

	// The auto backup contains only the uploaded images of the data folder
	for name := range files {
		if strings.HasPrefix(name, "data/") && !strings.HasPrefix(name, "data/images/") {
			delete(files, name)
		}
	}

	// Older backups beyond BackupKeep are removed
	for _, old := range []string{"xteve_auto_backup_20200101_0000.zip", "xteve_auto_backup_20200102_0000.zip"} {
		if err := os.WriteFile(filepath.Join(System.Folder.Backup, old), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := xTeVeAutoBackup(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(System.Folder.Backup)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || names[0] != "xteve_auto_backup_20200102_0000.zip" {
		t.Fatalf("backups = %v, want the newer old one and a new one", names)
	}
	checkArchive(t, filepath.Join(System.Folder.Backup, names[1]), files)
}

func TestBackupWithoutTLS(t *testing.T) {
	var files = backupTest(t)
	Settings.TLSMode = false
	for name := range files {
		if strings.HasPrefix(name, "certificates/") {
			delete(files, name)
		}
	}

	archive, err := xteveBackup()
	if err != nil {
		t.Fatal(err)
	}
	checkArchive(t, filepath.Join(System.Folder.Temp, archive), files)
}

func TestExtractZIPWindowsNames(t *testing.T) {
	var archive = writeTestZIP(t, map[string]string{
		`settings.json`:        "{}",
		`data\`:                "",
		`data\images\logo.png`: "png",
		`..\evil.txt`:          "x",
	})
	var parent = t.TempDir()
	var target = filepath.Join(parent, "restore")

	if err := extractZIP(archive, target); err == nil {
		t.Error(`expected an error for "..\evil.txt"`)
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("file was written outside of the target folder (stat error: %v)", err)
	}

	// Without the invalid entry the archive is extracted into folders
	archive = writeTestZIP(t, map[string]string{`data\`: "", `data\images\logo.png`: "png"})
	if err := extractZIP(archive, target); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(target, "data", "images", "logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "png" {
		t.Errorf("content = %q", content)
	}
}

func TestRemoveBackupContentKeepsBackups(t *testing.T) {
	var files = backupTest(t)
	var autoBackup = filepath.Join(System.Folder.Backup, "xteve_auto_backup_20200101_0000.zip")
	if err := os.WriteFile(autoBackup, []byte("zip"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := removeBackupContent(); err != nil {
		t.Fatal(err)
	}

	for name := range files {
		if _, err := os.Stat(filepath.Join(System.Folder.Config, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("%s was not removed (stat error: %v)", name, err)
		}
	}
	for _, kept := range []string{autoBackup, filepath.Join(System.Folder.Config, "other.txt")} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was removed: %v", kept, err)
		}
	}
}
