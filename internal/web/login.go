package web

import (
	"errors"
	"net/http"

	"url-shortener/internal/auth"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", http.StatusOK, templateData{Title: "Login"})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	pw := r.PostForm.Get("password")
	if err := s.deps.Sessions.Login(r.Context(), w, pw); err != nil {
		if errors.Is(err, auth.ErrLoginFailed) {
			s.render(w, "login.html", http.StatusUnauthorized, templateData{
				Title: "Login", Flash: "Invalid password",
			})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.deps.Sessions.Logout(r.Context(), w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
