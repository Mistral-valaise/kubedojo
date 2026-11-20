package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/store"
)

func attempts(t *testing.T, dir string) int {
	t.Helper()
	c, err := store.Inspect(context.Background(), filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	return c.Attempts
}

// Back up, lose progress, restore: the progress is back, and what was there
// before the restore is kept next to it.
func TestBackupAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	add := func() {
		if err := st.AddAttempt(store.Attempt{SessionID: "s", Mode: "practice", Track: "cka", Domain: "d", QuestionID: "q", Score: 1, Max: 2, At: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	add()
	add()

	// The server is running: it holds the lock of its data directory. A
	// backup works, a restore is refused.
	unlock, held, err := store.Lock(dir)
	if err != nil || !held {
		t.Fatalf("Lock = %v, %v", held, err)
	}
	if _, _, err := store.Lock(dir); !errors.Is(err, store.ErrLocked) {
		t.Fatalf("a second server on the same data directory = %v, want ErrLocked", err)
	}
	backup := filepath.Join(dir, "saved.db")
	if err := cmdBackup(ctx, []string{"--data", dir, "--out", backup}); err != nil {
		t.Fatalf("backup under a running server: %v", err)
	}
	add() // progress after the backup
	err = cmdRestore(ctx, []string{"--data", dir, "--from", backup})
	if err == nil || !strings.Contains(err.Error(), "Stop it first") {
		t.Fatalf("restore under a running server = %v, want a refusal", err)
	}
	st.Close()
	unlock()
	if got := attempts(t, dir); got != 3 {
		t.Fatalf("%d attempts before the restore, want 3", got)
	}

	// The server is stopped.
	if err := cmdRestore(ctx, []string{"--data", dir, "--from", backup}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := attempts(t, dir); got != 2 {
		t.Fatalf("%d attempts after the restore, want the 2 of the backup", got)
	}
	kept, _ := filepath.Glob(filepath.Join(dir, "kubedojo.db.before-restore-*"))
	if len(kept) != 1 {
		t.Fatalf("the previous database was not kept: %v", kept)
	}
	if c, err := store.Inspect(ctx, kept[0]); err != nil || c.Attempts != 3 {
		t.Fatalf("the kept database: %+v, %v; want the 3 attempts from before", c, err)
	}
	// The restored database works.
	st, err = store.Open(filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	// Restore into an empty data directory: a new host.
	fresh := t.TempDir()
	if err := cmdRestore(ctx, []string{"--data", fresh, "--from", backup}); err != nil {
		t.Fatalf("restore into an empty directory: %v", err)
	}
	if got := attempts(t, fresh); got != 2 {
		t.Fatalf("%d attempts in the new directory, want 2", got)
	}

	// What is not a backup is refused before anything is touched.
	junk := filepath.Join(dir, "junk.db")
	_ = os.WriteFile(junk, []byte("not a database"), 0o600)
	if err := cmdRestore(ctx, []string{"--data", fresh, "--from", junk}); err == nil {
		t.Fatal("a file that is not a database was restored")
	}
	if got := attempts(t, fresh); got != 2 {
		t.Fatalf("a refused restore changed the database: %d attempts", got)
	}
	if err := cmdRestore(ctx, []string{"--data", fresh}); err == nil {
		t.Fatal("restore without --from did something")
	}
}
