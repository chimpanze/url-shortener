package web

import (
	"net/http"

	"ffs.bz/internal/auth"
)

func (s *Server) handleAdminList(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListLinksWithCounts(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_list.html", http.StatusOK, templateData{
		Title: "Links", CSRF: csrf, Data: list,
	})
}
