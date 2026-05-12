package web

import (
	"errors"
	"net/http"
	"strconv"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/store"
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

type newLinkForm struct {
	Slug        string
	Destination string
}

func (s *Server) handleAdminNewGet(w http.ResponseWriter, r *http.Request) {
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_new.html", http.StatusOK, templateData{
		Title: "New link", CSRF: csrf, Data: newLinkForm{},
	})
}

func (s *Server) handleAdminNewPost(w http.ResponseWriter, r *http.Request) {
	// ParseForm already called by csrfProtect.
	slug := r.PostForm.Get("slug")
	dest := r.PostForm.Get("destination")

	_, err := s.deps.Shortener.CreateLink(r.Context(), slug, dest)
	if err != nil {
		csrf := ""
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			csrf = sess.CSRFToken
		}
		s.render(w, "admin_new.html", http.StatusBadRequest, templateData{
			Title: "New link", CSRF: csrf, Flash: err.Error(),
			Data: newLinkForm{Slug: slug, Destination: dest},
		})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

type adminDetailData struct {
	Link   *store.Link
	Total  int
	Clicks []store.ClickRecord
}

func (s *Server) parseID(r *http.Request) (int64, error) {
	v := chiURLParam(r, "id")
	return strconv.ParseInt(v, 10, 64)
}

func (s *Server) handleAdminDetail(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Store.GetLinkByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	total, _ := s.deps.Store.CountClicks(r.Context(), id)
	clicks, _ := s.deps.Store.ListClicks(r.Context(), id, 100)

	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_detail.html", http.StatusOK, templateData{
		Title: link.Slug, CSRF: csrf,
		Data: adminDetailData{Link: link, Total: total, Clicks: clicks},
	})
}

func (s *Server) handleAdminDelete(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := s.deps.Store.DeleteLink(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) handleAdminEditGet(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	link, err := s.deps.Store.GetLinkByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	csrf := ""
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		csrf = sess.CSRFToken
	}
	s.render(w, "admin_edit.html", http.StatusOK, templateData{
		Title: "Edit " + link.Slug, CSRF: csrf, Data: link,
	})
}

func (s *Server) handleAdminEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := s.parseID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	dest := r.PostForm.Get("destination")
	if err := s.deps.Shortener.UpdateDestination(r.Context(), id, dest); err != nil {
		link, _ := s.deps.Store.GetLinkByID(r.Context(), id)
		csrf := ""
		if sess, ok := auth.SessionFromContext(r.Context()); ok {
			csrf = sess.CSRFToken
		}
		if link != nil {
			link.Destination = dest
		}
		s.render(w, "admin_edit.html", http.StatusBadRequest, templateData{
			Title: "Edit", CSRF: csrf, Flash: err.Error(), Data: link,
		})
		return
	}
	http.Redirect(w, r, "/admin/links/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
