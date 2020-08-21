package middleware

import (
	"context"
	"fmt"
	"net/http"
	"phenix/web/rbac"
	"strings"

	log "github.com/activeshadow/libminimega/minilog"
	jwtmiddleware "github.com/cescoferraro/go-jwt-middleware"
	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
)

func AuthMiddleware(enabled bool, jwtKey string) mux.MiddlewareFunc {
	tokenMiddleware := jwtmiddleware.New(
		jwtmiddleware.Options{
			// Setting this to true since some resource paths don't require
			// authentication. Those that do will be caught in the
			// userMIddleware, which will also check for a `user` context
			// value being present, which is only set if valid credentials
			// were presented.
			CredentialsOptional: true,
			// Most calls to the API will include the JWT in the auth header. However,
			// calls for screenshots and VNC will need the JWT in the URL since they'll
			// be in browser links and image tags.
			Extractor: jwtmiddleware.FromFirst(jwtmiddleware.FromAuthHeader, jwtmiddleware.FromParameter("token")),
			ValidationKeyGetter: func(_ *jwt.Token) (interface{}, error) {
				return []byte(jwtKey), nil
			},
			SigningMethod: jwt.SigningMethodHS256,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, e string) {
				log.Error("Error validating auth token: %s", e)

				// TODO: remove token from user spec?

				http.Error(w, e, http.StatusUnauthorized)
			},
		},
	)

	userMiddleware := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/signup") {
				h.ServeHTTP(w, r)
				return
			}

			if strings.HasSuffix(r.URL.Path, "/login") {
				h.ServeHTTP(w, r)
				return
			}

			if strings.HasSuffix(r.URL.Path, "/vnc/ws") {
				h.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			userToken := ctx.Value("user")
			if userToken == nil {
				log.Error("Rejecting unauthorized request for %s: Missing user token", r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			token := userToken.(*jwt.Token)
			claim := token.Claims.(jwt.MapClaims)

			user, err := rbac.GetUser(claim["sub"].(string))
			if err != nil {
				http.Error(w, "user error", http.StatusUnauthorized)
				return
			}

			if err := user.ValidateToken(token.Raw); err != nil {
				http.Error(w, "user token error", http.StatusUnauthorized)
				return
			}

			role, err := user.Role()
			if err != nil {
				fmt.Println(err)
				// TODO: we will get an error here if the user doesn't yet have a role
				// assigned. Need to figure out how we could redirect the client to a
				// page that talks about needing to wait for an admin to assign a role.
				http.Error(w, "user role error", http.StatusUnauthorized)
				return
			}

			ctx = context.WithValue(ctx, "user", user.Username())
			ctx = context.WithValue(ctx, "role", role)
			ctx = context.WithValue(ctx, "jwt", token.Raw)

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	noAuthMiddleware := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := rbac.RoleFromConfig("global-admin")

			ctx := context.WithValue(r.Context(), "role", *role)

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	devAuthMiddleware := func(h http.Handler) http.Handler {
		creds := strings.Split(jwtKey, "|")

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			ctx = context.WithValue(ctx, "user", creds[1])
			ctx = context.WithValue(ctx, "uid", 0)
			ctx = context.WithValue(ctx, "role", creds[2])

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	if jwtKey == "" {
		log.Debug("no JWT signing key provided -- disabling auth")
		return func(h http.Handler) http.Handler { return noAuthMiddleware(h) }
	} else if strings.HasPrefix(jwtKey, "dev|") {
		log.Debug("development JWT key provided -- enabling dev auth")
		return func(h http.Handler) http.Handler { return devAuthMiddleware(h) }
	}

	// First validate the token itself, then ensure the user in the token is valid.
	return func(h http.Handler) http.Handler { return tokenMiddleware.Handler(userMiddleware(h)) }
}
