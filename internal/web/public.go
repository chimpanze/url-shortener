package web

import (
	"errors"
	"net/http"
	"time"

	"url-shortener/internal/clicklog"
	"url-shortener/internal/store"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Shortener.ResolveLink(r.Context(), slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.render(w, "notfound.html", http.StatusNotFound, templateData{Title: "Not found"})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, link.Destination, http.StatusFound)
	s.deps.Clicks.Enqueue(clicklog.Event{
		LinkID:    link.ID,
		TS:        time.Now(),
		Referer:   r.Header.Get("Referer"),
		UserAgent: r.Header.Get("User-Agent"),
	})
}
