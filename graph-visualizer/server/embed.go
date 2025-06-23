//go:build prod
// +build prod

package main

import (
	"log"
	"net/http"
	"vibethis/static"
)

func getFrontendHandler() http.Handler {
	fs, err := static.GetFileSystem()
	if err != nil {
		log.Fatal("Failed to get filesystem:", err)
	}
	return http.FileServer(fs)
}