//go:build !prod
// +build !prod

package main

import (
	"net/http"
)

func getFrontendHandler() http.Handler {
	return http.FileServer(http.Dir("../web/dist/"))
}