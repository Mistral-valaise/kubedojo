package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/httpsec"
)

// bodyTimeout is how long a client may take to send a request body. The
// server has no read timeout of its own: grading holds a request for minutes.
// A variable for the tests.
var bodyTimeout = 15 * time.Second

// errMediaType is readJSON's error for a body that is not declared as JSON.
var errMediaType = errors.New("the request body must be sent with Content-Type: application/json")

// isJSON reports whether the request declares its body as JSON. Parameters
// (charset) are fine; text/plain is not: a form on any web page can post
// that to this server without the browser asking first, and its body may
// well be JSON.
func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// readJSON decodes the request body into v and reads at most limit bytes of
// it. It writes no response: a handler with its own error message uses it
// directly, after it has given rejected the error. An empty body is io.EOF.
//
// A request that has a body must declare it as JSON, or the error is
// errMediaType. The client has bodyTimeout to send the body.
func readJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return io.EOF
	}
	// The deadline is taken back once the body is read to its end, and only
	// then: the handler may run for minutes, but as long as a part of the
	// body is missing, nothing may wait for it without a limit. That includes
	// the server, which reads what a handler left over before it answers.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(bodyTimeout)) // not every ResponseWriter has one (tests)
	done := func() { _ = rc.SetReadDeadline(time.Time{}) }
	body := http.MaxBytesReader(w, r.Body, limit)
	if !isJSON(r) {
		// A body of unknown length (chunked) may still be empty.
		if r.ContentLength < 0 {
			if _, err := io.ReadFull(body, make([]byte, 1)); err == io.EOF {
				done()
				return io.EOF
			}
		}
		return errMediaType
	}
	err := json.NewDecoder(body).Decode(v)
	if err == nil {
		// What follows the value is not looked at, but it is read.
		_, err = io.Copy(io.Discard, body)
	}
	if err == nil || err == io.EOF {
		done()
	}
	return err
}

// rejected answers the errors of readJSON that have nothing to do with what
// the body says: a body that is not declared as JSON (415) and a body that
// did not arrive in time (408). It reports whether it answered.
func (s *Server) rejected(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, errMediaType):
		if s.OnReject != nil {
			s.OnReject(httpsec.ReasonContentType)
		}
		writeErr(w, http.StatusUnsupportedMediaType, "Send the request body with Content-Type: application/json.")
	case errors.Is(err, os.ErrDeadlineExceeded):
		writeErr(w, http.StatusRequestTimeout, "The request body did not arrive in time.")
	default:
		return false
	}
	return true
}

// decode reads an optional JSON body of at most limit bytes into v. An empty
// body is fine and leaves v as it is. A body that is not declared as JSON is
// answered with 415; anything else that is not JSON, or is larger than limit,
// with 400. It returns false when it has answered.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	err := readJSON(w, r, v, limit)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	if !s.rejected(w, err) {
		writeErr(w, 400, "invalid JSON")
	}
	return false
}

// decodeRequired is decode for a request that needs its body: an empty body
// is answered with 400 as well.
func (s *Server) decodeRequired(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	err := readJSON(w, r, v, limit)
	if err == nil {
		return true
	}
	if !s.rejected(w, err) {
		writeErr(w, 400, "invalid JSON")
	}
	return false
}
