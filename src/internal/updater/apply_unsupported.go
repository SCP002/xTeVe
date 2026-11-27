//go:build js || wasip1 || aix

package updater

import (
	"errors"
	"io"
)

func canReplace(string) error { return errors.New("self update is not supported on this platform") }

// replaceBinary is not possible on platforms that selfupdate does not support.
func replaceBinary(io.Reader, []byte, string) error {
	return errors.New("self update is not supported on this platform")
}
