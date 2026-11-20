package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
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
