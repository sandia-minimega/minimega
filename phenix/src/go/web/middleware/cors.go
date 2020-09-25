package middleware

import (
	"net/http"
)

const (
	origins = "*"
	methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	headers = "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization"
)

func AllowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origins)
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", headers)

		if r.Method == "OPTIONS" {
			return
		}

		next.ServeHTTP(w, r)
	})
}
