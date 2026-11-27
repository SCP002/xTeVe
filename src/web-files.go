package src

import (
	"fmt"
	"io/fs"
)

// webFiles holds the html folder of the web UI: embedded into the binary, or the local folder in developer mode.
var webFiles fs.FS

// SetWebFiles sets where the files of the web UI come from. Paths in it start with "html/".
func SetWebFiles(files fs.FS) { webFiles = files }

// readWebFile returns a file of the web UI, e.g. "html/index.html".
func readWebFile(name string) ([]byte, error) {
	if webFiles == nil {
		return nil, fmt.Errorf("%s: the files of the web UI are not set", name)
	}
	return fs.ReadFile(webFiles, name)
}
