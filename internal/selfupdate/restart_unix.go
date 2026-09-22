//go:build !windows

package selfupdate

import (
	"os"
	"syscall"
)

// Restart replaces the current process with a fresh run of path, keeping the
// same arguments and environment. It only returns on failure.
func Restart(path string) error {
	args := append([]string{path}, os.Args[1:]...)
	return syscall.Exec(path, args, os.Environ())
}
