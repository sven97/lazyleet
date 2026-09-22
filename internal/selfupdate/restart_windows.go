//go:build windows

package selfupdate

import (
	"errors"
	"os"
	"os/exec"
)

// Restart runs path with the same arguments and stdio, then exits with its
// status: Windows has no exec(2), so the new version runs as a child.
func Restart(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
