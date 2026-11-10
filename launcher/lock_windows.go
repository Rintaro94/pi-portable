//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"syscall"
)

const errSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION

func lockPath(root string) string { return filepath.Join(root, "tmp", ".pi-portable.lock") }

func openLock(root string, access, share uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(lockPath(root))
	if err != nil {
		return 0, err
	}
	return syscall.CreateFile(p, access, share, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
}

// holdRunLock keeps the lock file open with shared access while the
// launcher waits for pi. The handle closes when the launcher exits.
func holdRunLock(root string) error {
	_, err := openLock(root, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE)
	if errors.Is(err, errSharingViolation) {
		return errUpdating
	}
	return err
}

// lockExclusive opens the lock file with no sharing: it fails while a
// launcher (and so pi) is running, and blocks new starts until released.
func lockExclusive(root string) (func(), error) {
	h, err := openLock(root, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0)
	if err != nil {
		if errors.Is(err, errSharingViolation) {
			return nil, errPiRunning
		}
		return nil, err
	}
	return func() { syscall.CloseHandle(h) }, nil
}

func isInUse(err error) bool {
	return errors.Is(err, errSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
