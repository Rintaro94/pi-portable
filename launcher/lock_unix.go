//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func lockPath(root string) string { return filepath.Join(root, "tmp", ".pi-portable.lock") }

// holdRunLock takes a shared lock that lives as long as pi runs. The fd is
// opened without O_CLOEXEC, so it survives the exec into pi and the lock is
// released when pi exits.
func holdRunLock(root string) error {
	fd, err := syscall.Open(lockPath(root), syscall.O_RDWR|syscall.O_CREAT, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(fd, syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		syscall.Close(fd)
		return errUpdating
	}
	return nil
}

// lockExclusive fails while any pi started from this folder is running, and
// blocks new starts until the returned function is called.
func lockExclusive(root string) (func(), error) {
	f, err := os.OpenFile(lockPath(root), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errPiRunning
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}

func isInUse(error) bool { return false }
