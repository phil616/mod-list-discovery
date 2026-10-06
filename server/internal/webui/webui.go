package webui

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed index.html static/*
var files embed.FS
var page = template.Must(template.ParseFS(files, "index.html"))

func Page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if e := page.ExecuteTemplate(w, "index.html", nil); e != nil {
		return
	}
}
func Assets() http.Handler {
	sub, e := fs.Sub(files, "static")
	if e != nil {
		panic(e)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
