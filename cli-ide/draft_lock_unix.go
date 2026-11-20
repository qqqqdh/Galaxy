//go:build unix

package main

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

// Closing the file releases ownership even when Android kills the process.
func lockDraft(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
