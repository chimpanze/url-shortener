package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

type templateData struct {
	Title string
	CSRF  string
	Flash string
	Data  any
}

func loadTemplates() (map[string]*template.Template, error) {
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, err
	}
	layout, err := templatesFS.ReadFile("templates/layout.html")
	if err != nil {
		return nil, err
	}
	out := map[string]*template.Template{}
	for _, e := range entries {
		if e.Name() == "layout.html" {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		t, err := template.New(e.Name()).Parse(string(layout))
		if err != nil {
			return nil, err
		}
		if _, err := t.Parse(string(b)); err != nil {
			return nil, err
		}
		out[e.Name()] = t
	}
	return out, nil
}

func (s *Server) render(w http.ResponseWriter, name string, status int, data templateData) {
	t, ok := s.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("template %s not found", name), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func staticFileSystem() http.FileSystem {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
