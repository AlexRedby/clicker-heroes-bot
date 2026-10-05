//go:build windows

package rubybudget

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockFile(f *os.File) error {
	o := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, o)
	if err == windows.ERROR_LOCK_VIOLATION {
		return ErrLocked
	}
	return err
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
