//go:build unix

package updater

import (
	"os"
	"syscall"
)

// Restart replaces the running process with the updated binary. The process keeps its ID, which matters for
// service managers and for xTeVe as the main process of a container.
func Restart() error {
	var executable, err = os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executable, os.Args, os.Environ())
}
