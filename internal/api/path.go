package api

import "net/http"

// path returns the recommended order through the tracks. Content without a
// path.yaml has none, which the UI treats as "show no path".
func (s *Server) path(w http.ResponseWriter, r *http.Request) {
	if s.Catalog.Path == nil {
		writeErr(w, 404, "no path")
		return
	}
	writeJSON(w, 200, s.Catalog.Path)
}
