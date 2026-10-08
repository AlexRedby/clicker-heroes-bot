//go:build windows

package transcension

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockJournal(file *os.File) error {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
	if err == windows.ERROR_LOCK_VIOLATION {
		return ErrJournalLocked
	}
	return err
}

func unlockJournal(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}

func protectJournalFile(file *os.File) error {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	protected := os.NewFile(uintptr(handle), file.Name())
	defer protected.Close()
	original, originalErr := file.Stat()
	opened, openedErr := protected.Stat()
	if originalErr != nil || openedErr != nil || !os.SameFile(original, opened) {
		return errors.New("journal changed while opening its security handle")
	}
	previous, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := previous.Owner()
	if err != nil || owner == nil || owner.String() != user.User.Sid.String() {
		return errors.New("journal belongs to another Windows user")
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

func checkJournalMode(info os.FileInfo) error { return nil }
func replaceJournalFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	// The temporary file is synced before the native write-through replacement.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func syncJournalDirectory(path string) error { return nil }
