package store

import (
	"context"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"
)

// The observer sees every operation once, under a name from Operations: the
// names become a metric label, so there must be no others.
func TestObserveReportsEveryOperation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Without an observer nothing is reported and nothing breaks.
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	st.Observe(func(op string, d time.Duration) {
		if d < 0 {
			t.Errorf("%s took %s", op, d)
		}
		seen[op]++
	})
	now := time.Now()
	ctx := context.Background()
	steps := []func() error{
		func() error { return st.Ping(ctx) },
		func() error {
			return st.SaveSession("s", "practice", "cka", "ready", now, map[string]string{"id": "s"})
		},
		func() error { _, err := st.LoadSessions(); return err },
		func() error { return st.AddAttempt(Attempt{SessionID: "s", Track: "cka", At: now}) },
		func() error { _, err := st.Attempts("cka", ""); return err },
		func() error {
			return st.AddExam(Exam{SessionID: "s", Track: "cka", FinishedAt: now, Domains: []byte("[]")})
		},
		func() error { _, err := st.Exams("cka", ""); return err },
		func() error { return st.AddRating(Rating{Track: "cka", QuestionID: "q", Stars: 4, At: now}) },
		func() error { _, err := st.Ratings(""); return err },
		func() error { return st.Backup(ctx, filepath.Join(t.TempDir(), "backup.db")) },
		func() error { return st.MarkAllDestroyed() },
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	var got []string
	for op, n := range seen {
		if n != 1 {
			t.Errorf("%s was reported %d times, want once", op, n)
		}
		got = append(got, op)
	}
	want := slices.Clone(Operations)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("operations reported: %v\nOperations:          %v", got, want)
	}

	// A failed operation is reported as well: it took time too.
	if err := st.Backup(ctx, filepath.Join(t.TempDir(), "missing", "dir", "backup.db")); err == nil {
		t.Fatal("a backup into a directory that does not exist succeeded")
	}
	if seen["backup"] != 2 {
		t.Errorf("the failed backup was not reported: %v", seen)
	}

	st.Observe(nil)
	if err := st.Ping(ctx); err != nil || seen["ping"] != 1 {
		t.Errorf("after Observe(nil): err %v, ping reported %d times", err, seen["ping"])
	}
}
