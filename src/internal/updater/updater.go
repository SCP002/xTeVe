// Package updater updates the xTeVe binary from the GitHub releases of the repository.
//
// A release has a binary per platform ("xteve-linux-amd64", "xteve-windows-amd64.exe", ...) and a SHA256SUMS file
// with their checksums. The binary of the running platform is downloaded, checked against its checksum and replaces
// the running binary; the old one is restored if that fails.
package updater

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// ChecksumFile is the name of the checksum file of a release.
const ChecksumFile = "SHA256SUMS"

// ErrNoRelease is returned by Latest when no release has a binary for this platform and checksums.
var ErrNoRelease = errors.New("no release with a binary for this platform and " + ChecksumFile)

// Source is where the releases are.
type Source struct {
	// APIURL is the GitHub API, "https://api.github.com" if empty.
	APIURL string
	Owner  string
	Repo   string
	// Prerelease also uses pre-releases (the "beta" branch).
	Prerelease bool
	// Client is used for all requests, http.DefaultClient if nil.
	Client *http.Client
	// TargetPath is the binary to replace, the running one if empty.
	TargetPath string
}

// Release is a release with the download URL of the binary for this platform and of the checksum file.
type Release struct {
	Version     string // "v2.6.0"
	BinaryURL   string
	ChecksumURL string
	BinaryName  string
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// BinaryName returns the name of the release binary for a platform, as build.sh names it.
func BinaryName(name, goos, goarch string) string {
	var ext string
	switch {
	case goos == "windows":
		ext = ".exe"
	case goarch == "wasm":
		ext = ".wasm"
	}
	return fmt.Sprintf("%s-%s-%s%s", strings.ToLower(name), goos, goarch, ext)
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

// Latest returns the newest release with a binary for this platform.
func (s Source) Latest(ctx context.Context, name string) (Release, error) {
	var api = s.APIURL
	if api == "" {
		api = "https://api.github.com"
	}

	// The latest release is the newest one that is neither a draft nor a pre-release
	var url = fmt.Sprintf("%s/repos/%s/%s/releases/latest", api, s.Owner, s.Repo)
	if s.Prerelease {
		url = fmt.Sprintf("%s/repos/%s/%s/releases?per_page=20", api, s.Owner, s.Repo)
	}

	var body, err = s.get(ctx, url)
	if err != nil {
		return Release{}, err
	}

	var releases []githubRelease
	if s.Prerelease {
		err = json.Unmarshal(body, &releases)
	} else {
		var r githubRelease
		err = json.Unmarshal(body, &r)
		releases = []githubRelease{r}
	}
	if err != nil {
		return Release{}, fmt.Errorf("invalid release information: %w", err)
	}

	var binaryName = BinaryName(name, runtime.GOOS, runtime.GOARCH)
	var newest Release
	for _, r := range releases {
		if r.Draft || !semver.IsValid(r.TagName) {
			continue
		}
		if newest.Version != "" && semver.Compare(r.TagName, newest.Version) <= 0 {
			continue
		}

		var release = Release{Version: r.TagName, BinaryName: binaryName}
		for _, a := range r.Assets {
			switch a.Name {
			case binaryName:
				release.BinaryURL = a.URL
			case ChecksumFile:
				release.ChecksumURL = a.URL
			}
		}
		if release.BinaryURL != "" && release.ChecksumURL != "" {
			newest = release
		}
	}

	if newest.Version == "" {
		return Release{}, fmt.Errorf("%w (%s)", ErrNoRelease, binaryName)
	}
	return newest, nil
}

// Updatable reports whether a version can be updated: a local build ("dev") is not a semantic version and is never
// updated.
func Updatable(version string) bool { return semver.IsValid(withV(version)) }

func withV(version string) string {
	if !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

// Newer reports whether release is newer than the version ("2.5.0" or "v2.5.0"). A version that is not a semantic
// version (e.g. "dev" of a local build) is never updated.
func Newer(release Release, version string) bool {
	version = withV(version)
	return semver.IsValid(version) && semver.Compare(release.Version, version) > 0
}

// CanApply reports an error if the binary cannot be replaced, e.g. in a container where it belongs to root.
func (s Source) CanApply() error { return canReplace(s.TargetPath) }

// Apply downloads the binary of the release, checks its checksum and replaces the running binary with it.
func (s Source) Apply(ctx context.Context, release Release) error {
	var sums, err = s.get(ctx, release.ChecksumURL)
	if err != nil {
		return err
	}
	checksum, err := findChecksum(sums, release.BinaryName)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.BinaryURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", release.BinaryURL, resp.Status)
	}

	return replaceBinary(resp.Body, checksum, s.TargetPath)
}

// findChecksum returns the checksum of a file in a SHA256SUMS file ("<hex>  <name>" per line).
func findChecksum(sums []byte, name string) ([]byte, error) {
	var scanner = bufio.NewScanner(strings.NewReader(string(sums)))
	for scanner.Scan() {
		var fields = strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			var sum, err = hex.DecodeString(fields[0])
			if err != nil || len(sum) != 32 {
				return nil, fmt.Errorf("invalid checksum of %s", name)
			}
			return sum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("no checksum for " + name)
}

func (s Source) get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
