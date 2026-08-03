package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFiles embed.FS

func (s *Server) frontend() http.Handler {
	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(assets))
}
