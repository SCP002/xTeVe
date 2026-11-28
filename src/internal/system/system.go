// Package system holds information about the running xTeVe: names, versions, paths, flags and addresses.
//
// The exported fields are set once at start (main and src.Init) and only read afterwards. What changes while
// xTeVe runs is kept behind methods and atomic values, so it is safe for use by multiple goroutines.
package system

import (
	"net"
	"sync/atomic"
)

// Info is the information about the running xTeVe.
//
// Info must not be copied after first use.
type Info struct {
	APIVersion       string
	AppName          string
	ARCH             string
	Branch           string
	Compatibility    string
	DBVersion        string
	Dev              bool
	Hostname         string
	Name             string
	OS               string
	PlexChannelLimit int
	URLBase          string
	UDPxy            string
	Version          string

	// The automatic update time chosen at start
	TimeForAutoUpdate string

	FFmpeg struct {
		DefaultOptions string
		Path           string
	}

	VLC struct {
		DefaultOptions string
		Path           string
	}

	File struct {
		Authentication    string
		M3U               string
		PMS               string
		ServerCert        string
		ServerCertPrivKey string
		Settings          string
		URLS              string
		XEPG              string
		XML               string
	}

	Compressed struct {
		GZxml string
	}

	Flag struct {
		Branch  string
		Debug   int
		Info    bool
		Port    string
		Restore string
		SSDP    bool
	}

	// Folders created at start. The temporary folder can change with the settings, see TempFolder.
	Folder struct {
		Backup       string
		Cache        string
		Certificates string
		Config       string
		Data         string
		ImagesCache  string
		ImagesUpload string
	}

	IPAddressesList   []string // Every IP address available (IPv4 + IPv6)
	IPAddressesV4     []string // Every IPv4 address available in string format
	IPAddressesV4Host []string // Every IPv4 address available except loopback and link-local
	IPAddressesV4Raw  []net.IP // Every IPv4 address available in net.IP format
	IPAddressesV6     []string // Every IPv6 address available

	GitHub struct {
		Branch string
		Repo   string
		Update bool
		User   string
	}

	// ConfigurationWizard reports that the first setup in the web UI is not finished yet.
	ConfigurationWizard atomic.Bool
	// ImageCachingInProgress reports that channel and program images are being downloaded.
	ImageCachingInProgress atomic.Bool

	scans      atomic.Int32
	domain     atomic.Pointer[string]
	tls        atomic.Bool
	tempFolder atomic.Pointer[string]
}

// BeginScan records that a database (streams or XEPG) is being rebuilt, until the returned function is called.
func (i *Info) BeginScan() (end func()) {
	i.scans.Add(1)
	var once atomic.Bool
	return func() {
		if once.CompareAndSwap(false, true) {
			i.scans.Add(-1)
		}
	}
}

// Scanning reports whether a database is being rebuilt.
func (i *Info) Scanning() bool { return i.scans.Load() > 0 }

// Domain returns the host and port that clients use to reach xTeVe, as seen in the latest request.
func (i *Info) Domain() string {
	if d := i.domain.Load(); d != nil {
		return *d
	}
	return ""
}

// SetDomain sets the host and port that clients use to reach xTeVe.
func (i *Info) SetDomain(domain string) { i.domain.Store(&domain) }

// Protocol returns "https" while the web server serves TLS, else "http".
func (i *Info) Protocol() string {
	if i.tls.Load() {
		return "https"
	}
	return "http"
}

// SetTLS records whether the web server serves TLS.
func (i *Info) SetTLS(tls bool) { i.tls.Store(tls) }

// TempFolder returns the folder for temporary files: buffers, backups to download and restores.
func (i *Info) TempFolder() string {
	if f := i.tempFolder.Load(); f != nil {
		return *f
	}
	return ""
}

// SetTempFolder sets the folder for temporary files.
func (i *Info) SetTempFolder(folder string) { i.tempFolder.Store(&folder) }
