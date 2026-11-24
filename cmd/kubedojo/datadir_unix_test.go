//go:build !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/store"
)

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// The data directory of an older release: everything readable for everyone.
func TestSecureDataDir(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)

	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(dir, "observability", "dashboards"), 0o755); err != nil {
		t.Fatal(err)
	}
	private := []string{"kubedojo.db", "kubedojo.db-wal", "kubedojo.db-shm", "settings.json", "kubedojo.lock",
		"kubedojo-20260101-120000.db", "kubedojo.db.before-restore-20260101-120000"}
	untouched := []string{"notes.txt", "observability/datasources.yaml", "observability/dashboards/kubedojo.db"}
	// A link that matches a pattern must not lead the chmod out of the directory.
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Symlink(outside, filepath.Join(dir, "kubedojo-link.db")); err != nil {
		t.Fatal(err)
	}
	for _, f := range append(append([]string{outside}, private...), untouched...) {
		if !filepath.IsAbs(f) {
			f = filepath.Join(dir, f)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for range 2 { // the second time on what is already right
		if err := secureDataDir(dir); err != nil {
			t.Fatal(err)
		}
	}
	if got := mode(t, dir); got != 0o700 {
		t.Errorf("the data directory has mode %o, want 700", got)
	}
	for _, f := range private {
		if got := mode(t, filepath.Join(dir, f)); got != 0o600 {
			t.Errorf("%s has mode %o, want 600", f, got)
		}
	}
	// Grafana reads these from inside its container, as another user.
	for _, f := range untouched {
		if got := mode(t, filepath.Join(dir, f)); got != 0o644 {
			t.Errorf("%s has mode %o, want it left at 644", f, got)
		}
	}
	for _, d := range []string{"observability", "observability/dashboards"} {
		if got := mode(t, filepath.Join(dir, d)); got != 0o755 {
			t.Errorf("%s has mode %o, want it left at 755", d, got)
		}
	}
	if got := mode(t, outside); got != 0o644 {
		t.Errorf("a file outside the data directory was changed through a link: mode %o", got)
	}
}

// A new data directory and what the commands write: nothing is readable for
// others, whatever the umask of the shell is.
func TestNewDataIsPrivate(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	ctx := context.Background()

	dir := filepath.Join(t.TempDir(), "new", "data")
	c := common{dataDir: dir}
	st, err := c.openStore()
	if err != nil {
		t.Fatal(err)
	}
	// A write, so that SQLite creates its -wal and -shm files.
	if err := st.AddAttempt(store.Attempt{SessionID: "s", Mode: "practice", Track: "cka", Domain: "d", QuestionID: "q", Score: 1, Max: 1, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if got := mode(t, dir); got != 0o700 {
		t.Errorf("the data directory has mode %o, want 700", got)
	}
	for _, f := range []string{"kubedojo.db", "kubedojo.db-wal", "kubedojo.db-shm"} {
		if got := mode(t, filepath.Join(dir, f)); got != 0o600 {
			t.Errorf("%s has mode %o, want 600", f, got)
		}
	}

	// Backups: in the data directory by default, or where --out says.
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.db")
	if err := cmdBackup(ctx, []string{"--data", dir}); err != nil {
		t.Fatal(err)
	}
	if err := cmdBackup(ctx, []string{"--data", dir, "--out", elsewhere}); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "kubedojo-*.db"))
	if len(backups) != 1 {
		t.Fatalf("backups in the data directory: %v", backups)
	}
	for _, f := range []string{backups[0], elsewhere} {
		if got := mode(t, f); got != 0o600 {
			t.Errorf("the backup %s has mode %o, want 600", f, got)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// A restore into the directory of an older release: the database it
	// replaces is kept, and that copy is private as well.
	for path, m := range map[string]os.FileMode{dir: 0o755, filepath.Join(dir, "kubedojo.db"): 0o644} {
		if err := os.Chmod(path, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmdRestore(ctx, []string{"--data", dir, "--from", elsewhere}); err != nil {
		t.Fatal(err)
	}
	kept, _ := filepath.Glob(filepath.Join(dir, "kubedojo.db.before-restore-*"))
	if len(kept) != 1 {
		t.Fatalf("kept databases: %v", kept)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, kept[0]: 0o600, filepath.Join(dir, "kubedojo.db"): 0o600} {
		if got := mode(t, path); got != want {
			t.Errorf("after the restore %s has mode %o, want %o", path, got, want)
		}
	}
}

// A server starts in a data directory that does not exist yet: the lock is
// the first thing it writes there.
func TestLockCreatesTheDataDirectory(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	dir := filepath.Join(t.TempDir(), "new", "data")
	unlock, held, err := store.Lock(dir)
	if err != nil || !held {
		t.Fatalf("Lock = %v, %v", held, err)
	}
	defer unlock()
	if got := mode(t, dir); got != 0o700 {
		t.Errorf("the data directory has mode %o, want 700", got)
	}
	if got := mode(t, filepath.Join(dir, "kubedojo.lock")); got != 0o600 {
		t.Errorf("the lock has mode %o, want 600", got)
	}
}
