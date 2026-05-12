package web

import (
	"net/http"

	"ffs.bz/internal/auth"
)

func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := auth.SessionFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		token := r.PostForm.Get("csrf_token")
		if token == "" || token != sess.CSRFToken {
			http.Error(w, "csrf token mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
