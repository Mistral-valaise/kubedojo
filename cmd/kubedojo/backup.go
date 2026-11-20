package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/store"
)

func describe(c store.Contents) string {
	return fmt.Sprintf("%d sessions, %d graded questions, %d exams, %d ratings", c.Sessions, c.Attempts, c.Exams, c.Ratings)
}

// cmdBackup writes a consistent copy of the database. The server may run.
func cmdBackup(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("backup", flag.ContinueOnError)
	data := fl.String("data", env("KUBEDOJO_DATA", defaultDataDir()), "data directory")
	out := fl.String("out", "", "file to write (default: kubedojo-<date>.db in the data directory)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		*out = filepath.Join(*data, "kubedojo-"+time.Now().UTC().Format("20060102-150405")+".db")
	}
	db := filepath.Join(*data, "kubedojo.db")
	if _, err := os.Stat(db); err != nil {
		return fmt.Errorf("no database to back up: %w", err)
	}
	st, err := store.Open(db)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Backup(ctx, *out); err != nil {
		return err
	}
	c, err := store.Inspect(ctx, *out)
	if err != nil {
		return fmt.Errorf("the backup was written but does not verify: %w", err)
	}
	fmt.Printf("%s: %s\n", *out, describe(c))
	return nil
}

// cmdRestore replaces the database by a backup. The server must not run.
func cmdRestore(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("restore", flag.ContinueOnError)
	data := fl.String("data", env("KUBEDOJO_DATA", defaultDataDir()), "data directory")
	from := fl.String("from", "", "backup file to restore")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return fmt.Errorf("--from names the backup to restore")
	}
	c, err := store.Inspect(ctx, *from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*data, 0o755); err != nil {
		return err
	}
	db := filepath.Join(*data, "kubedojo.db")
	// The lock a server holds for as long as it runs.
	unlock, _, err := store.Lock(*data)
	if errors.Is(err, store.ErrLocked) {
		return fmt.Errorf("%w (%s). Stop it first: a restore under a running server would replace the database below it", err, *data)
	}
	if err != nil {
		return err
	}
	defer unlock()
	// What is there now is kept, not deleted.
	kept := ""
	if _, err := os.Stat(db); err == nil {
		kept = db + ".before-restore-" + time.Now().UTC().Format("20060102-150405")
		if err := os.Rename(db, kept); err != nil {
			return err
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(db + suffix)
	}
	if err := copyFile(*from, db); err != nil {
		if kept != "" {
			_ = os.Rename(kept, db)
		}
		return err
	}
	fmt.Printf("restored %s: %s\n", *from, describe(c))
	if kept != "" {
		fmt.Printf("the previous database is kept as %s\n", kept)
	}
	return nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(to)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
