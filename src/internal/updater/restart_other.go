//go:build !unix

package updater

import "os"

// Restart starts the updated binary and ends the running process.
func Restart() error {
	var executable, err = os.Executable()
	if err != nil {
		return err
	}
	var wd, _ = os.Getwd()
	if _, err := os.StartProcess(executable, os.Args, &os.ProcAttr{
		Dir:   wd,
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
