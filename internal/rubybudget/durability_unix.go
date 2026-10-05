//go:build !windows

package rubybudget

import "os"

func durabilityAvailable() bool { return true }
func checkStateFileMode(info os.FileInfo) error {
	if info == nil || info.Mode().Perm() != 0600 {
		return os.ErrPermission
	}
	return nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = syncFile(f)
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	return err
}
