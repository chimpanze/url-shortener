package web

import "net/http"

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
