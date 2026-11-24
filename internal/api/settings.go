package api

import "net/http"

// registerSettings registers the settings endpoints. There are none yet: this
// is where they are added, so that Handler does not change with them.
func (s *Server) registerSettings(mux *http.ServeMux) {}
