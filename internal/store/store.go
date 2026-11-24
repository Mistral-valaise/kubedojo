// Package store persists sessions and the learner's history in SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db       *sql.DB
	observer atomic.Pointer[func(operation string, d time.Duration)]
}

// Operations are the names Observe reports: one per method that reads or
// writes the database. The set is fixed, so a name can be a metric label.
var Operations = []string{"ping", "save_session", "load_sessions", "add_attempt", "attempts",
	"add_exam", "exams", "mark_all_destroyed", "add_rating", "ratings", "backup"}

// Observe makes the store report every operation to fn: its name, one of
// Operations, and how long it took. fn runs on the goroutine of the caller
// and must be quick. A nil fn stops the reports.
func (s *Store) Observe(fn func(operation string, d time.Duration)) {
	if fn == nil {
		s.observer.Store(nil)
		return
	}
	s.observer.Store(&fn)
}

// timed starts the clock of an operation. The returned function reports it:
//
//	defer s.timed("save_session")()
func (s *Store) timed(operation string) func() {
	fn := s.observer.Load()
	if fn == nil {
		return func() {}
	}
	start := time.Now()
	return func() { (*fn)(operation, time.Since(start)) }
}

// ErrLocked is returned by Lock when a server uses the data directory.
var ErrLocked = errors.New("a kubedojo server is using this data directory")

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY, mode TEXT NOT NULL, track TEXT NOT NULL, status TEXT NOT NULL,
  created_at TEXT NOT NULL, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, mode TEXT NOT NULL,
  track TEXT NOT NULL, domain TEXT NOT NULL, question_id TEXT NOT NULL,
  score INTEGER NOT NULL, max INTEGER NOT NULL, at TEXT NOT NULL, owner TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS attempts_q ON attempts(track, question_id);
CREATE TABLE IF NOT EXISTS ratings (
  id INTEGER PRIMARY KEY AUTOINCREMENT, track TEXT NOT NULL, question_id TEXT NOT NULL,
  owner TEXT NOT NULL DEFAULT '', stars INTEGER NOT NULL, comment TEXT NOT NULL DEFAULT '', at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS exams (
  session_id TEXT PRIMARY KEY, track TEXT NOT NULL, score INTEGER NOT NULL, max INTEGER NOT NULL,
  percent REAL NOT NULL, passed INTEGER NOT NULL, finished_at TEXT NOT NULL, domains TEXT NOT NULL, owner TEXT NOT NULL DEFAULT '');
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// migrate brings a database of an older release up to date. Every step can
// run again on a database that already has it.
func migrate(db *sql.DB) error {
	// Databases created before per-user history get the owner column added.
	for _, table := range []string{"attempts", "exams"} {
		if err := addColumn(db, table, "owner TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	// Exams recorded before quick rounds were told apart get their size from
	// the session they belong to, as far as that session is still stored.
	for _, col := range []string{"questions", "minutes"} {
		if err := addColumn(db, "exams", col+" INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	_, err := db.Exec(`UPDATE exams SET
		questions = COALESCE((SELECT json_array_length(data, '$.questions') FROM sessions WHERE id = exams.session_id), 0),
		minutes = COALESCE((SELECT json_extract(data, '$.durationMinutes') FROM sessions WHERE id = exams.session_id), 0)
		WHERE questions = 0`)
	return err
}

// addColumn adds a column unless the table already has it. SQLite has no
// ADD COLUMN IF NOT EXISTS, so the one expected error is told apart by its text.
func addColumn(db *sql.DB, table, def string) error {
	_, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + def)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("add column %s to %s: %w", def, table, err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ping reports whether the database answers.
func (s *Store) Ping(ctx context.Context) error {
	defer s.timed("ping")()
	var one int
	return s.db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

func (s *Store) SaveSession(id, mode, track, status string, created time.Time, v any) error {
	defer s.timed("save_session")()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO sessions(id,mode,track,status,created_at,data) VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET status=excluded.status, data=excluded.data`,
		id, mode, track, status, created.UTC().Format(time.RFC3339), string(b))
	return err
}

// LoadSessions returns the raw JSON of every session that is not destroyed.
func (s *Store) LoadSessions() ([][]byte, error) {
	defer s.timed("load_sessions")()
	rows, err := s.db.Query(`SELECT data FROM sessions WHERE status != 'destroyed' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, []byte(d))
	}
	return out, rows.Err()
}

type Attempt struct {
	SessionID  string    `json:"sessionId"`
	Mode       string    `json:"mode"`
	Track      string    `json:"track"`
	Domain     string    `json:"domain"`
	QuestionID string    `json:"questionId"`
	Score      int       `json:"score"`
	Max        int       `json:"max"`
	At         time.Time `json:"at"`
	Owner      string    `json:"-"`
}

func (s *Store) AddAttempt(a Attempt) error {
	defer s.timed("add_attempt")()
	_, err := s.db.Exec(`INSERT INTO attempts(session_id,mode,track,domain,question_id,score,max,at,owner) VALUES(?,?,?,?,?,?,?,?,?)`,
		a.SessionID, a.Mode, a.Track, a.Domain, a.QuestionID, a.Score, a.Max, a.At.UTC().Format(time.RFC3339), a.Owner)
	return err
}

func (s *Store) Attempts(track, owner string) ([]Attempt, error) {
	defer s.timed("attempts")()
	rows, err := s.db.Query(`SELECT session_id,mode,track,domain,question_id,score,max,at FROM attempts WHERE track=? AND owner=? ORDER BY id`, track, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var a Attempt
		var at string
		if err := rows.Scan(&a.SessionID, &a.Mode, &a.Track, &a.Domain, &a.QuestionID, &a.Score, &a.Max, &at); err != nil {
			return nil, err
		}
		a.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, a)
	}
	return out, rows.Err()
}

type Exam struct {
	SessionID  string          `json:"sessionId"`
	Track      string          `json:"track"`
	Score      int             `json:"score"`
	Max        int             `json:"max"`
	Percent    float64         `json:"percent"`
	Passed     bool            `json:"passed"`
	FinishedAt time.Time       `json:"finishedAt"`
	Domains    json.RawMessage `json:"domains"`
	Owner      string          `json:"-"`
	// Questions and Minutes are the size of the assessment, which tells a quick
	// round from a full one. Zero for an exam recorded before they were stored.
	Questions int `json:"questions"`
	Minutes   int `json:"minutes"`
}

func (s *Store) AddExam(e Exam) error {
	defer s.timed("add_exam")()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO exams(session_id,track,score,max,percent,passed,finished_at,domains,owner,questions,minutes) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		e.SessionID, e.Track, e.Score, e.Max, e.Percent, e.Passed, e.FinishedAt.UTC().Format(time.RFC3339), string(e.Domains), e.Owner, e.Questions, e.Minutes)
	return err
}

func (s *Store) Exams(track, owner string) ([]Exam, error) {
	defer s.timed("exams")()
	rows, err := s.db.Query(`SELECT session_id,track,score,max,percent,passed,finished_at,domains,questions,minutes FROM exams WHERE track=? AND owner=? ORDER BY finished_at`, track, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exam
	for rows.Next() {
		var e Exam
		var at, d string
		if err := rows.Scan(&e.SessionID, &e.Track, &e.Score, &e.Max, &e.Percent, &e.Passed, &at, &d, &e.Questions, &e.Minutes); err != nil {
			return nil, err
		}
		e.FinishedAt, _ = time.Parse(time.RFC3339, at)
		e.Domains = json.RawMessage(d)
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkAllDestroyed flags every session as destroyed (after `kubedojo down`).
func (s *Store) MarkAllDestroyed() error {
	defer s.timed("mark_all_destroyed")()
	_, err := s.db.Exec(`UPDATE sessions SET status='destroyed', data=json_set(data,'$.status','destroyed')`)
	return err
}

// Rating is a learner's feedback on one question.
type Rating struct {
	Track      string    `json:"track"`
	QuestionID string    `json:"questionId"`
	Owner      string    `json:"owner,omitempty"`
	Stars      int       `json:"stars"`
	Comment    string    `json:"comment"`
	At         time.Time `json:"at"`
}

func (s *Store) AddRating(r Rating) error {
	defer s.timed("add_rating")()
	_, err := s.db.Exec(`INSERT INTO ratings(track,question_id,owner,stars,comment,at) VALUES(?,?,?,?,?,?)`,
		r.Track, r.QuestionID, r.Owner, r.Stars, r.Comment, r.At.UTC().Format(time.RFC3339))
	return err
}

// Ratings returns all ratings, newest first; track may be empty for all tracks.
func (s *Store) Ratings(track string) ([]Rating, error) {
	defer s.timed("ratings")()
	rows, err := s.db.Query(`SELECT track,question_id,owner,stars,comment,at FROM ratings WHERE (?='' OR track=?) ORDER BY id DESC`, track, track)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rating
	for rows.Next() {
		var r Rating
		var at string
		if err := rows.Scan(&r.Track, &r.QuestionID, &r.Owner, &r.Stars, &r.Comment, &at); err != nil {
			return nil, err
		}
		r.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Backup writes a consistent copy of the database to path, which must not
// exist. It is safe while a server uses the database: SQLite takes the copy
// inside one read transaction, so it holds either all of a write or nothing
// of it. Copying the database file instead is not safe: in WAL mode the
// latest writes are in a second file.
func (s *Store) Backup(ctx context.Context, path string) error {
	defer s.timed("backup")()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists already", path)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("backup to %s: %w", path, err)
	}
	return nil
}

// Contents is what a database holds, for a person to recognise it.
type Contents struct {
	Sessions, Attempts, Exams, Ratings int
}

// Inspect opens a database file read-only, checks that it is an intact
// kubedojo database and counts what it holds.
func Inspect(ctx context.Context, path string) (Contents, error) {
	var c Contents
	if _, err := os.Stat(path); err != nil {
		return c, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return c, err
	}
	defer db.Close()
	var ok string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok); err != nil {
		return c, fmt.Errorf("%s is not a SQLite database: %w", path, err)
	}
	if ok != "ok" {
		return c, fmt.Errorf("%s is damaged: %s", path, ok)
	}
	for table, n := range map[string]*int{"sessions": &c.Sessions, "attempts": &c.Attempts, "exams": &c.Exams, "ratings": &c.Ratings} {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(n); err != nil {
			return c, fmt.Errorf("%s is not a kubedojo database: %w", path, err)
		}
	}
	return c, nil
}
