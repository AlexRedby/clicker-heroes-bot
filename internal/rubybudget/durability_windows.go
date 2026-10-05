//go:build windows

package rubybudget

import "os"

// Windows chmod does not provide a 0600 ACL and directory handles cannot be
// synchronised portably through the standard library. Positive budgets fail
// closed in Open; zero-budget preview state remains inspectable.
func durabilityAvailable() bool                 { return false }
func checkStateFileMode(info os.FileInfo) error { return nil }
func syncDirectory(path string) error           { return nil }
