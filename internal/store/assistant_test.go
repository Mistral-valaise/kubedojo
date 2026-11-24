package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAssistantMessages(t *testing.T) {
	st := openTest(t)
	at := time.Date(2026, 11, 24, 10, 0, 0, 123456789, time.UTC)
	uid, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s1", QuestionID: "q1", Role: "user", Content: "why?", At: at})
	if err != nil {
		t.Fatal(err)
	}
	aid, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s1", QuestionID: "q1", Role: "assistant", Content: "because", Partial: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s1", QuestionID: "q2", Role: "user", Content: "other question"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s2", QuestionID: "q1", Role: "user", Content: "other session"}); err != nil {
		t.Fatal(err)
	}

	got, err := st.AssistantMessages("s1", "q1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != uid || got[1].ID != aid || uid >= aid {
		t.Fatalf("messages = %+v", got)
	}
	if got[0].Role != "user" || got[0].Content != "why?" || got[0].Partial || !got[0].At.Equal(at) {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Role != "assistant" || !got[1].Partial || got[1].At.IsZero() {
		t.Errorf("second = %+v", got[1])
	}

	// Deleting is scoped to the conversation the message belongs to.
	if err := st.DeleteAssistantMessage("s2", "q1", aid); !errors.Is(err, ErrNoMessage) {
		t.Errorf("deleting through another session: %v", err)
	}
	if err := st.DeleteAssistantMessage("s1", "q1", aid); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAssistantMessage("s1", "q1", aid); !errors.Is(err, ErrNoMessage) {
		t.Errorf("deleting twice: %v", err)
	}
	if err := st.ClearAssistantMessages("s1", "q1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.AssistantMessages("s1", "q1"); len(got) != 0 {
		t.Errorf("after clearing: %+v", got)
	}
	for _, other := range [][2]string{{"s1", "q2"}, {"s2", "q1"}} {
		if got, _ := st.AssistantMessages(other[0], other[1]); len(got) != 1 {
			t.Errorf("clearing s1/q1 touched %v: %+v", other, got)
		}
	}
}

func TestAssistantMessagesAreCapped(t *testing.T) {
	st := openTest(t)
	for i := range MaxAssistantMessages + 7 {
		if _, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s", QuestionID: "q", Role: "user", Content: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = st.AddAssistantMessage(AssistantMessage{SessionID: "s", QuestionID: "other", Role: "user", Content: "kept"})
	got, err := st.AssistantMessages("s", "q")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxAssistantMessages || got[0].Content != "7" || got[len(got)-1].Content != fmt.Sprint(MaxAssistantMessages+6) {
		t.Errorf("%d messages, first %q, last %q", len(got), got[0].Content, got[len(got)-1].Content)
	}
	if other, _ := st.AssistantMessages("s", "other"); len(other) != 1 {
		t.Error("the cap of one question removed messages of another")
	}
}

func TestPurgeAssistantMessages(t *testing.T) {
	st := openTest(t)
	now := time.Now()
	for id, status := range map[string]string{"live": "ready", "gone": "destroyed"} {
		if err := st.SaveSession(id, "practice", "t", status, now, map[string]string{"id": id, "status": status}); err != nil {
			t.Fatal(err)
		}
	}
	for _, sid := range []string{"live", "gone", "never-stored"} {
		for range 2 {
			if _, err := st.AddAssistantMessage(AssistantMessage{SessionID: sid, QuestionID: "q", Role: "user", Content: "x"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	n, err := st.PurgeAssistantMessages()
	if err != nil || n != 4 {
		t.Fatalf("purged %d (%v), want 4", n, err)
	}
	if got, _ := st.AssistantMessages("live", "q"); len(got) != 2 {
		t.Errorf("the conversation of a live session was purged: %d left", len(got))
	}
	// kubedojo down marks every session destroyed.
	if err := st.MarkAllDestroyed(); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.PurgeAssistantMessages(); n != 2 {
		t.Errorf("after MarkAllDestroyed: purged %d, want 2", n)
	}
}

// The conversations are in the database file, so a backup has them.
func TestBackupIncludesAssistantMessages(t *testing.T) {
	st := openTest(t)
	if _, err := st.AddAssistantMessage(AssistantMessage{SessionID: "s", QuestionID: "q", Role: "user", Content: "kept in the backup"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := st.Backup(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(context.Background(), path); err != nil {
		t.Fatalf("the backup does not verify: %v", err)
	}
	restored, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.AssistantMessages("s", "q")
	if err != nil || len(got) != 1 || got[0].Content != "kept in the backup" {
		t.Errorf("in the backup: %+v (%v)", got, err)
	}
}

// A database of a release without the assistant gets the table on first use.
func TestAssistantTableIsCreatedLazily(t *testing.T) {
	st := openTest(t)
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='assistant_messages'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the table exists before first use: %d (%v)", n, err)
	}
	if got, err := st.AssistantMessages("s", "q"); err != nil || len(got) != 0 {
		t.Fatalf("first read: %v", err)
	}
	if err := st.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='assistant_messages'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("the table was not created: %d (%v)", n, err)
	}
}
