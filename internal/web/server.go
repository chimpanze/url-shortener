package web

import (
	"html/template"
	"net/http"

	"ffs.bz/internal/auth"
	"ffs.bz/internal/clicklog"
	"ffs.bz/internal/shortener"
	"ffs.bz/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Store     *store.Store
	Shortener *shortener.Service
	Sessions  *auth.SessionManager
	Clicks    *clicklog.Logger
}

type Server struct {
	deps      Deps
	templates map[string]*template.Template
	router    chi.Router
}

func NewServer(d Deps) *Server {
	tpls, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	s := &Server{deps: d, templates: tpls}
	s.router = s.buildRouter()
	return s
}

func (s *Server) Router() http.Handler { return s.router }

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Handle("/admin/static/*", http.StripPrefix("/admin/static/", http.FileServer(staticFileSystem())))

	r.Route("/admin", func(r chi.Router) {
		r.Get("/login", s.handleLoginGet)
		r.Post("/login", s.handleLoginPost)

		r.Group(func(r chi.Router) {
			r.Use(s.deps.Sessions.RequireAuth)
			r.Get("/", s.handleAdminList)
			r.With(s.csrfProtect).Post("/logout", s.handleLogout)
		})
	})

	r.Get("/", s.handleHome)
	r.Get("/{slug}", s.handleRedirect)

	return r
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// handleRedirect is implemented in public.go (Task 13).
