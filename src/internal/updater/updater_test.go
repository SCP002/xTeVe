package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testServer is a GitHub API with releases and their files.
func testServer(t *testing.T, binary []byte, releases ...string) *httptest.Server {
	t.Helper()

	var name = BinaryName("xteve", runtime.GOOS, runtime.GOARCH)
	var sum = sha256.Sum256(binary)

	var srv *httptest.Server
	var release = func(tag string, pre bool, files ...string) string {
		var assets []string
		for _, f := range files {
			assets = append(assets,
				fmt.Sprintf(`{"name": %q, "browser_download_url": "%s/download/%s/%s"}`, f, srv.URL, tag, f))
		}
		return fmt.Sprintf(`{"tag_name": %q, "prerelease": %t, "assets": [%s]}`, tag, pre, strings.Join(assets, ","))
	}

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var all = map[string]string{
			"v2.5.0":      release("v2.5.0", false, name, ChecksumFile),
			"v2.10.0":     release("v2.10.0", false, name, ChecksumFile),
			"v2.11.0-rc1": release("v2.11.0-rc1", true, name, ChecksumFile),
			"v2.12.0":     release("v2.12.0", false, name), // no checksums
		}
		switch {
		case r.URL.Path == "/repos/o/r/releases/latest":
			_, _ = w.Write([]byte(all[releases[0]]))
		case r.URL.Path == "/repos/o/r/releases":
			var list []string
			for _, tag := range releases {
				list = append(list, all[tag])
			}
			_, _ = w.Write([]byte("[" + strings.Join(list, ",") + "]"))
		case strings.HasSuffix(r.URL.Path, "/"+ChecksumFile):
			_, _ = fmt.Fprintf(w, "%s  other-file\n%s  %s\n", strings.Repeat("0", 64), hex.EncodeToString(sum[:]), name)
		case strings.HasSuffix(r.URL.Path, "/"+name):
			_, _ = w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatest(t *testing.T) {
	var srv = testServer(t, []byte("new"), "v2.10.0", "v2.11.0-rc1", "v2.5.0", "v2.12.0")

	var stable, err = Source{APIURL: srv.URL, Owner: "o", Repo: "r"}.Latest(t.Context(), "xTeVe")
	if err != nil || stable.Version != "v2.10.0" || !strings.HasSuffix(stable.ChecksumURL, ChecksumFile) {
		t.Errorf("latest = %+v, err %v", stable, err)
	}

	// With pre-releases the newest one with checksums, compared as versions and not as text
	beta, err := Source{APIURL: srv.URL, Owner: "o", Repo: "r", Prerelease: true}.Latest(t.Context(), "xTeVe")
	if err != nil || beta.Version != "v2.11.0-rc1" {
		t.Errorf("latest with pre-releases = %+v, err %v", beta, err)
	}

	// A release without checksums is not used
	var noSums = testServer(t, nil, "v2.12.0")
	if _, err := (Source{APIURL: noSums.URL, Owner: "o", Repo: "r"}).Latest(t.Context(), "xTeVe"); err == nil {
		t.Error("a release without checksums was accepted")
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		release, version string
		want             bool
	}{
		{"v2.10.0", "2.9.0", true},
		{"v2.9.0", "2.10.0", false},
		{"v2.5.0", "2.5.0", false},
		{"v2.5.0", "v2.4.1", true},
		{"v2.6.0", "dev", false},
		{"v2.6.0-rc1", "2.6.0", false},
	}
	for _, tt := range tests {
		if got := Newer(Release{Version: tt.release}, tt.version); got != tt.want {
			t.Errorf("Newer(%s, %s) = %v, want %v", tt.release, tt.version, got, tt.want)
		}
	}
}

func TestApply(t *testing.T) {
	var target = filepath.Join(t.TempDir(), "xteve")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	var srv = testServer(t, []byte("new binary"), "v2.10.0")
	var source = Source{APIURL: srv.URL, Owner: "o", Repo: "r", TargetPath: target}
	var release, err = source.Latest(t.Context(), "xteve")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.CanApply(); err != nil {
		t.Fatal(err)
	}
	if err := source.Apply(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(target); string(content) != "new binary" {
		t.Errorf("binary = %q, want the new one", content)
	}

	// A binary that does not match its checksum is not used
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	release.BinaryName = "other-file"
	if err := source.Apply(t.Context(), release); err == nil {
		t.Error("a binary with the wrong checksum was applied")
	}
	if content, _ := os.ReadFile(target); string(content) != "old" {
		t.Errorf("binary = %q after a failed update, want the old one", content)
	}
}

func TestBinaryName(t *testing.T) {
	tests := map[[2]string]string{
		{"linux", "amd64"}:   "xteve-linux-amd64",
		{"windows", "arm64"}: "xteve-windows-arm64.exe",
		{"wasip1", "wasm"}:   "xteve-wasip1-wasm.wasm",
	}
	for platform, want := range tests {
		if got := BinaryName("xTeVe", platform[0], platform[1]); got != want {
			t.Errorf("BinaryName(%v) = %q, want %q", platform, got, want)
		}
	}
}

func TestCanApplyReadOnly(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write everywhere")
	}
	var dir = t.TempDir()
	var target = filepath.Join(dir, "xteve")
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	if err := (Source{TargetPath: target}).CanApply(); err == nil {
		t.Error("a binary in a read only folder can be replaced")
	}
}
