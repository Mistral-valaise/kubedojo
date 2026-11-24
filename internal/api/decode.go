package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// readJSON decodes the request body into v and reads at most limit bytes of
// it. It writes no response: a handler with its own error message uses it
// directly. An empty body is io.EOF.
func readJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v)
}

// decode reads an optional JSON body of at most limit bytes into v. An empty
// body is fine and leaves v as it is. On anything else that is not JSON, or
// is larger than limit, it answers 400 and returns false.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if err := readJSON(w, r, v, limit); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "invalid JSON")
		return false
	}
	return true
}

// decodeRequired is decode for a request that needs its body: an empty body
// is answered with 400 as well.
func (s *Server) decodeRequired(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if err := readJSON(w, r, v, limit); err != nil {
		writeErr(w, 400, "invalid JSON")
		return false
	}
	return true
}
