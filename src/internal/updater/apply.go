//go:build !js && !wasip1 && !aix

package updater

import (
	"fmt"
	"io"

	"github.com/fynelabs/selfupdate"
)

// canReplace reports an error if the binary cannot be replaced, e.g. in a container where it belongs to root.
func canReplace(target string) error {
	var opts = selfupdate.Options{TargetPath: target}
	return opts.CheckPermissions()
}

// replaceBinary replaces the running binary with the new one if its SHA-256 checksum matches.
func replaceBinary(binary io.Reader, checksum []byte, target string) error {
	if err := selfupdate.Apply(binary, selfupdate.Options{Checksum: checksum, TargetPath: target}); err != nil {
		if rollbackErr := selfupdate.RollbackError(err); rollbackErr != nil {
			return fmt.Errorf("update failed and the old binary could not be restored: %w", rollbackErr)
		}
		return err
	}
	return nil
}
