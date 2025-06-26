//go:build !prod
// +build !prod

package main

import (
	"net/http"
)

func getFrontendHandler() http.Handler {
	return newSPAHandler("../web/dist/")
}