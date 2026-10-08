//go:build !windows

package transcension

import (
	"errors"
	"os"
	"syscall"
)

func lockJournal(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrJournalLocked
	}
	return err
}

func unlockJournal(file *os.File) error      { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
func protectJournalFile(file *os.File) error { return file.Chmod(0600) }
func checkJournalMode(info os.FileInfo) error {
	if info.Mode().Perm() != 0600 {
		return os.ErrPermission
	}
	return nil
}
func replaceJournalFile(source, destination string) error { return os.Rename(source, destination) }
func syncJournalDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	err = journalSyncFile(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
