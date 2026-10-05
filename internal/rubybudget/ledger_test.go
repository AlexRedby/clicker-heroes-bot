package rubybudget

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testConfig(t *testing.T, allowance, protected, cap uint64) Config {
	t.Helper()
	if allowance > 0 && !durabilityAvailable() {
		t.Skip("positive budgets intentionally unsupported on this platform")
	}
	return Config{Path: filepath.Join(t.TempDir(), "ruby.json"), ProfileID: "profile-1", InitialAllowance: allowance, ProtectedBalance: protected, PerAscensionCap: cap}
}

func TestZeroAndCapsAndProtectedBalance(t *testing.T) {
	c := testConfig(t, 0, 2, 3)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err = l.Reserve(1, "a", 10); !errors.Is(err, ErrNoFunds) {
		t.Fatalf("zero allowance: %v", err)
	}

	c = testConfig(t, 10, 2, 3)
	l, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(2, "a", 3); !errors.Is(err, ErrNoFunds) {
		t.Fatalf("protected balance: %v", err)
	}
	r, err := l.Reserve(3, "a", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.ConfirmSpent(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(1, "a", 10); !errors.Is(err, ErrCap) {
		t.Fatalf("cap: %v", err)
	}
	l.Close()
}

func TestRestartPendingBlocksUntilExplicitOutcome(t *testing.T) {
	c := testConfig(t, 10, 0, 10)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Reserve(4, "asc", 10)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	l, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(1, "other", 10); !errors.Is(err, ErrPending) {
		t.Fatalf("pending did not survive restart: %v", err)
	}
	if err = l.ProveCanceled(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(1, "other", 10); err != nil {
		t.Fatal(err)
	}
	l.Close()
}

func TestObservedBalanceDoesNotReplenishAllowance(t *testing.T) {
	c := testConfig(t, 5, 0, 5)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Reserve(5, "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.ConfirmSpent(r.ID); err != nil {
		t.Fatal(err)
	}
	if err = l.Observe(100); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(1, "b", 100); !errors.Is(err, ErrNoFunds) {
		t.Fatalf("earned rubies replenished allowance: %v", err)
	}
	l.Close()
}

func TestCorruptionIdentityAndLock(t *testing.T) {
	c := testConfig(t, 4, 0, 4)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open(c); !errors.Is(err, ErrLocked) {
		t.Fatalf("lock: %v", err)
	}
	l.Close()
	if err = os.WriteFile(c.Path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(c); err == nil {
		t.Fatal("corruption accepted")
	}
	if err = os.WriteFile(c.Path, []byte(`{"version":1,"profile_id":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(c); err == nil {
		t.Fatal("identity mismatch accepted")
	}
}

func TestPersistenceFailurePoisonsLedgerAfterRename(t *testing.T) {
	c := testConfig(t, 5, 0, 5)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	oldSync := syncFile
	defer func() { syncFile = oldSync }()
	calls := 0
	syncFile = func(f *os.File) error {
		calls++
		if calls == 2 {
			return errors.New("injected directory fsync failure")
		}
		return f.Sync()
	}
	if err = l.Observe(5); err == nil {
		t.Fatal("fsync failure was ignored")
	}
	if !errors.Is(l.Observe(5), ErrPoisoned) {
		t.Fatal("ledger did not remain poisoned")
	}
	if !errors.Is(l.ConfirmSpent("anything"), ErrPoisoned) {
		t.Fatal("confirm was allowed after persistence failure")
	}
	l.Close()
}

func TestReserveAndConfirmPersistenceFailuresFailClosed(t *testing.T) {
	c := testConfig(t, 5, 0, 5)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	oldSync, oldRename := syncFile, renameFile
	defer func() { syncFile, renameFile = oldSync, oldRename }()
	syncFile = func(*os.File) error { return errors.New("injected file fsync failure") }
	if _, err = l.Reserve(1, "a", 5); err == nil {
		t.Fatal("Reserve ignored file fsync failure")
	}
	if !errors.Is(l.Observe(5), ErrPoisoned) {
		t.Fatal("Reserve fsync failure did not poison")
	}
	l.Close()

	l, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	syncFile = oldSync
	r, err := l.Reserve(1, "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	renameFile = func(string, string) error { return errors.New("injected rename failure") }
	if err = l.ConfirmSpent(r.ID); err == nil {
		t.Fatal("ConfirmSpent ignored rename failure")
	}
	if _, err = l.Reserve(1, "b", 5); !errors.Is(err, ErrPoisoned) {
		t.Fatal("ConfirmSpent failure allowed later Reserve")
	}
	l.Close()
}

func TestFailureAfterRenameLeavesCommittedStateOnReopen(t *testing.T) {
	c := testConfig(t, 5, 0, 5)
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.Reserve(2, "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	oldSync := syncFile
	defer func() { syncFile = oldSync }()
	calls := 0
	syncFile = func(f *os.File) error {
		calls++
		if calls == 2 {
			return errors.New("directory sync failure after rename")
		}
		return f.Sync()
	}
	if err = l.ConfirmSpent(r.ID); err == nil {
		t.Fatal("post-rename sync failure was ignored")
	}
	l.Close()
	l, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s, err := l.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Spent != 2 || s.Pending != nil {
		t.Fatalf("reopen lost committed state: %+v", s)
	}
}

func TestUnsupportedPlatformFailsClosedAndZeroPreviewReopens(t *testing.T) {
	if durabilityAvailable() {
		t.Skip("positive budgets supported on this platform")
	}
	c := Config{Path: filepath.Join(t.TempDir(), "ruby.json"), ProfileID: "p", InitialAllowance: 1, PerAscensionCap: 1}
	if _, err := Open(c); !errors.Is(err, ErrUnsupportedDurability) {
		t.Fatalf("positive budget: %v", err)
	}
	c.InitialAllowance = 0
	l, err := Open(c)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	l, err = Open(c)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.Reserve(1, "asc", 100); !errors.Is(err, ErrNoFunds) {
		t.Fatalf("zero budget: %v", err)
	}
}
