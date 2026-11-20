package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opening an existing database runs the migrations again. They must not fail
// on what is already there.
func TestOpenTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for i := range 2 {
		st, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		if err := st.Ping(context.Background()); err != nil {
			t.Fatalf("ping %d: %v", i+1, err)
		}
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// A migration that fails for another reason than "already done" used to be
// ignored, and the server started on a database it could not write to.
func TestMigrationErrorsAreReported(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// No tables at all: adding a column cannot work.
	err = migrate(db)
	if err == nil || !strings.Contains(err.Error(), "attempts") {
		t.Fatalf("migrate on an empty database = %v, want an error naming the table", err)
	}
}

func TestPingAfterClose(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err := st.Ping(context.Background()); err == nil {
		t.Fatal("Ping on a closed database reported no error")
	}
}

// A backup taken while the server writes holds a consistent state: every
// attempt that was acknowledged before the backup started, and no half of one.
func TestBackupWhileWriting(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "kubedojo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	add := func(i int) {
		if err := st.AddAttempt(Attempt{SessionID: "s", Mode: "practice", Track: "cka", Domain: "d", QuestionID: "q",
			Score: i, Max: 100, At: time.Now()}); err != nil {
			t.Error(err)
		}
	}
	for i := range 50 {
		add(i)
	}
	// Writes go on while the backup is taken. They are in the WAL, which a
	// copy of the database file alone would miss.
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for i := 50; ; i++ {
			select {
			case <-stop:
				return
			default:
				add(i)
			}
		}
	}()
	backup := filepath.Join(dir, "backup.db")
	if err := st.Backup(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	close(stop)
	<-done

	c, err := Inspect(context.Background(), backup)
	if err != nil {
		t.Fatal(err)
	}
	if c.Attempts < 50 {
		t.Fatalf("the backup has %d attempts; 50 were written before it started", c.Attempts)
	}
	// It opens as a database of its own and needs no WAL next to it.
	if _, err := os.Stat(backup + "-wal"); err == nil {
		t.Error("the backup depends on a WAL file")
	}
	again, err := Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got, err := again.Attempts("cka", "")
	if err != nil || len(got) != c.Attempts {
		t.Fatalf("the backup opens with %d attempts, Inspect counted %d: %v", len(got), c.Attempts, err)
	}
	// An existing file is never overwritten.
	if err := st.Backup(context.Background(), backup); err == nil {
		t.Fatal("Backup overwrote an existing file")
	}
}

func TestInspectRejectsWhatIsNotADatabase(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.db")
	if err := os.WriteFile(junk, []byte("this is not a database, it only has the name of one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), junk); err == nil {
		t.Error("a text file was accepted as a backup")
	}
	if _, err := Inspect(context.Background(), filepath.Join(dir, "missing.db")); err == nil {
		t.Error("a missing file was accepted as a backup")
	}
	// A SQLite database that is not kubedojo's.
	other := filepath.Join(dir, "other.db")
	db, err := sql.Open("sqlite", other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE notes (text TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Inspect(context.Background(), other); err == nil || !strings.Contains(err.Error(), "not a kubedojo database") {
		t.Errorf("Inspect of a foreign database = %v", err)
	}
}
