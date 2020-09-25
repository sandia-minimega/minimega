package middleware

import (
	"net/http"

	log "github.com/activeshadow/libminimega/minilog"
)

func LogFull(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("Request: %+v", r)
		h.ServeHTTP(w, r)
		log.Info("Response: %+v", w)
	})
}

func LogRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("Request: %s %s", r.Method, r.RequestURI)
		h.ServeHTTP(w, r)
	})
}
