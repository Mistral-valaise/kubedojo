package store

import (
	"errors"
	"sync"
	"time"
)

// The conversations with the assistant: what the learner typed and what the
// model answered, per session and question. They are rows of their own and
// not part of the session's JSON, which is rewritten on every save.
const assistantSchema = `
CREATE TABLE IF NOT EXISTS assistant_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, question_id TEXT NOT NULL,
  role TEXT NOT NULL, content TEXT NOT NULL, partial INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS assistant_messages_q ON assistant_messages(session_id, question_id);
`

// MaxAssistantMessages is how many messages are kept per question. The
// oldest are deleted beyond it.
const MaxAssistantMessages = 200

// ErrNoMessage is returned for a message that does not exist.
var ErrNoMessage = errors.New("no such message")

// AssistantMessage is one message of a conversation with the assistant.
type AssistantMessage struct {
	ID         int64
	SessionID  string
	QuestionID string
	Role       string // user | assistant
	Content    string
	// Partial marks an answer that was cut short by an error or because the
	// learner left.
	Partial bool
	At      time.Time
}

// assistantReady remembers per database that the table exists. The table is
// created on first use, from this file, so that a database of an older
// release gets it without a migration step.
var assistantReady sync.Map // *sql.DB -> *tableOnce

type tableOnce struct {
	once sync.Once
	err  error
}

func (s *Store) assistant() error {
	v, _ := assistantReady.LoadOrStore(s.db, &tableOnce{})
	t := v.(*tableOnce)
	t.once.Do(func() {
		if _, t.err = s.db.Exec(assistantSchema); t.err != nil {
			// The next call tries again.
			assistantReady.CompareAndDelete(s.db, v)
		}
	})
	return t.err
}

// AddAssistantMessage stores a message and returns its id.
func (s *Store) AddAssistantMessage(m AssistantMessage) (int64, error) {
	if err := s.assistant(); err != nil {
		return 0, err
	}
	if m.At.IsZero() {
		m.At = time.Now()
	}
	res, err := s.db.Exec(`INSERT INTO assistant_messages(session_id,question_id,role,content,partial,created_at) VALUES(?,?,?,?,?,?)`,
		m.SessionID, m.QuestionID, m.Role, m.Content, m.Partial, m.At.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`DELETE FROM assistant_messages WHERE session_id=? AND question_id=? AND id NOT IN (
		SELECT id FROM assistant_messages WHERE session_id=? AND question_id=? ORDER BY id DESC LIMIT ?)`,
		m.SessionID, m.QuestionID, m.SessionID, m.QuestionID, MaxAssistantMessages)
	return id, err
}

// AssistantMessages returns the conversation of a question, oldest first.
func (s *Store) AssistantMessages(sessionID, questionID string) ([]AssistantMessage, error) {
	if err := s.assistant(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,role,content,partial,created_at FROM assistant_messages WHERE session_id=? AND question_id=? ORDER BY id`, sessionID, questionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssistantMessage
	for rows.Next() {
		m := AssistantMessage{SessionID: sessionID, QuestionID: questionID}
		var at string
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Partial, &at); err != nil {
			return nil, err
		}
		m.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteAssistantMessage removes one message of a question's conversation.
func (s *Store) DeleteAssistantMessage(sessionID, questionID string, id int64) error {
	if err := s.assistant(); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM assistant_messages WHERE session_id=? AND question_id=? AND id=?`, sessionID, questionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoMessage
	}
	return nil
}

// ClearAssistantMessages removes the conversation of a question.
func (s *Store) ClearAssistantMessages(sessionID, questionID string) error {
	if err := s.assistant(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM assistant_messages WHERE session_id=? AND question_id=?`, sessionID, questionID)
	return err
}

// PurgeAssistantMessages removes the conversations of sessions that are
// destroyed or no longer stored, and reports how many messages went.
func (s *Store) PurgeAssistantMessages() (int64, error) {
	if err := s.assistant(); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM assistant_messages WHERE session_id NOT IN (SELECT id FROM sessions WHERE status != 'destroyed')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
