//go:build !windows

package fileio

import (
	"errors"
	"os"
	"syscall"
)

// OpenFile opens a file, as os.OpenFile.
func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

// Rename renames a file, replacing what's at newpath, as os.Rename.
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func crossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func inUse(error) bool { return false }

func ownerOnly(string) error { return nil }

func isOwnerOnly(string) (bool, error) { return true, nil }
