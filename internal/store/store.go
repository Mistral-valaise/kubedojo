// Package store persists sessions and the learner's history in SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY, mode TEXT NOT NULL, track TEXT NOT NULL, status TEXT NOT NULL,
  created_at TEXT NOT NULL, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, mode TEXT NOT NULL,
  track TEXT NOT NULL, domain TEXT NOT NULL, question_id TEXT NOT NULL,
  score INTEGER NOT NULL, max INTEGER NOT NULL, at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS attempts_q ON attempts(track, question_id);
CREATE TABLE IF NOT EXISTS exams (
  session_id TEXT PRIMARY KEY, track TEXT NOT NULL, score INTEGER NOT NULL, max INTEGER NOT NULL,
  percent REAL NOT NULL, passed INTEGER NOT NULL, finished_at TEXT NOT NULL, domains TEXT NOT NULL);
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
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SaveSession(id, mode, track, status string, created time.Time, v any) error {
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
}

func (s *Store) AddAttempt(a Attempt) error {
	_, err := s.db.Exec(`INSERT INTO attempts(session_id,mode,track,domain,question_id,score,max,at) VALUES(?,?,?,?,?,?,?,?)`,
		a.SessionID, a.Mode, a.Track, a.Domain, a.QuestionID, a.Score, a.Max, a.At.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) Attempts(track string) ([]Attempt, error) {
	rows, err := s.db.Query(`SELECT session_id,mode,track,domain,question_id,score,max,at FROM attempts WHERE track=? ORDER BY id`, track)
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
}

func (s *Store) AddExam(e Exam) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO exams(session_id,track,score,max,percent,passed,finished_at,domains) VALUES(?,?,?,?,?,?,?,?)`,
		e.SessionID, e.Track, e.Score, e.Max, e.Percent, e.Passed, e.FinishedAt.UTC().Format(time.RFC3339), string(e.Domains))
	return err
}

func (s *Store) Exams(track string) ([]Exam, error) {
	rows, err := s.db.Query(`SELECT session_id,track,score,max,percent,passed,finished_at,domains FROM exams WHERE track=? ORDER BY finished_at`, track)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exam
	for rows.Next() {
		var e Exam
		var at, d string
		if err := rows.Scan(&e.SessionID, &e.Track, &e.Score, &e.Max, &e.Percent, &e.Passed, &at, &d); err != nil {
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
	_, err := s.db.Exec(`UPDATE sessions SET status='destroyed', data=json_set(data,'$.status','destroyed')`)
	return err
}
