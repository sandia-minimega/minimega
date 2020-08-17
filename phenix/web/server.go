package web

import (
	"context"
	"flag"
	"net/http"
	"strings"

	"phenix/web/broker"
	"phenix/web/database"
	"phenix/web/middleware"
	"phenix/web/rbac"
	"phenix/web/util"

	log "github.com/activeshadow/libminimega/minilog"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	f_endpoint  string
	f_jwtKey    string
	f_users     string
	f_log       string
	f_allowCORS bool
)

func init() {
	flag.StringVar(&f_endpoint, "web.endpoint", ":3000", "HTTP endpoint to listen on")
	flag.StringVar(&f_jwtKey, "web.auth.jwt-signing-key", "", "key to sign/verify JWTs (not providing a key disables auth)")
	flag.StringVar(&f_users, "web.users", "admin@foo.com:foobar:Global Admin", "list of default users (separated by pipes)")
	flag.StringVar(&f_log, "web.log", "", "HTTP logs (options are 'full' and 'requests')")
	flag.BoolVar(&f_allowCORS, "web.allow-cors", false, "Allow HTTP CORS")
}

func Start() error {
	if len(f_users) != 0 {
		users := strings.Split(f_users, "|")

		for _, u := range users {
			creds := strings.Split(u, ":")
			uname := creds[0]
			pword := creds[1]
			rname := creds[2]

			user := rbac.User{
				Username: uname,
				Password: pword,
			}

			policies := rbac.CreateBasePoliciesForRole(rname)

			for _, policy := range policies {
				policy.SetResourceNames(creds[3:]...)
			}

			user.Role = rbac.NewRole(rname, policies...)

			// allow user to get their own user details
			user.Role.AddPolicies(&rbac.Policy{
				Resources:     []string{"users"},
				ResourceNames: []string{uname},
				Verbs:         []string{"get"},
			})

			log.Debug("creating default user - %+v", user)

			if err := database.UpsertUser(&user); err != nil {
				return errors.Wrap(err, "adding user to database")
			}
		}
	}

	router := mux.NewRouter().StrictSlash(true)
	assets := &assetfs.AssetFS{
		Asset:     Asset,
		AssetDir:  AssetDir,
		AssetInfo: AssetInfo,
	}

	log.Info("Setting up assets")

	router.PathPrefix("/docs/").Handler(
		http.FileServer(assets),
	)

	router.PathPrefix("/novnc/").Handler(
		http.FileServer(assets),
	)

	router.PathPrefix("/assets/").Handler(
		http.FileServer(assets),
	)

	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		util.NewBinaryFileSystem(assets).ServeFile(w, r, "index.html")
	})

	api := router.PathPrefix("/api/v1").Subrouter()

	// OPTIONS method needed for CORS
	api.HandleFunc("/experiments", GetExperiments).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments", CreateExperiment).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{name}", GetExperiment).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{name}", DeleteExperiment).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/experiments/{name}/start", StartExperiment).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{name}/stop", StopExperiment).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{name}/schedule", GetExperimentSchedule).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{name}/schedule", ScheduleExperiment).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{name}/captures", GetExperimentCaptures).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{name}/files", GetExperimentFiles).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{name}/files/{filename}", GetExperimentFile).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms", GetVMs).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}", GetVM).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}", UpdateVM).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}", DeleteVM).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/start", StartVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/stop", StopVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/redeploy", RedeployVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/screenshot.png", GetScreenshot).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/vnc", GetVNC).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/vnc/ws", GetVNCWebSocket).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/captures", GetVMCaptures).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/captures", StartVMCapture).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/captures", StopVMCaptures).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/snapshots", GetVMSnapshots).Methods("GET", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/snapshots", SnapshotVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/snapshots/{snapshot}", RestoreVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/experiments/{exp}/vms/{name}/commit", CommitVM).Methods("POST", "OPTIONS")
	api.HandleFunc("/vms", GetAllVMs).Methods("GET", "OPTIONS")
	api.HandleFunc("/applications", GetApplications).Methods("GET", "OPTIONS")
	api.HandleFunc("/topologies", GetTopologies).Methods("GET", "OPTIONS")
	api.HandleFunc("/disks", GetDisks).Methods("GET", "OPTIONS")
	api.HandleFunc("/hosts", GetClusterHosts).Methods("GET", "OPTIONS")
	api.HandleFunc("/logs", GetLogs).Methods("GET", "OPTIONS")
	api.HandleFunc("/users", GetUsers).Methods("GET", "OPTIONS")
	api.HandleFunc("/users", CreateUser).Methods("POST", "OPTIONS")
	api.HandleFunc("/users/{username}", GetUser).Methods("GET", "OPTIONS")
	api.HandleFunc("/users/{username}", UpdateUser).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/users/{username}", DeleteUser).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/signup", Signup).Methods("POST", "OPTIONS")
	api.HandleFunc("/login", Login).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/logout", Logout).Methods("GET", "OPTIONS")
	api.HandleFunc("/ws", broker.ServeWS).Methods("GET")

	if f_allowCORS {
		log.Info("CORS is enabled on HTTP API endpoints")
		api.Use(middleware.AllowCORS)
	}

	switch f_log {
	case "full":
		log.Info("full HTTP logging is enabled")
		api.Use(middleware.LogFull)
	case "requests":
		log.Info("requests-only HTTP logging is enabled")
		api.Use(middleware.LogRequests)
	}

	api.Use(middleware.AuthMiddleware(true, f_jwtKey))

	log.Info("Starting websockets broker")

	go broker.Start()

	log.Info("Starting log publisher")

	go PublishLogs(context.Background())

	log.Info("Starting HTTP server on %s", f_endpoint)

	return http.ListenAndServe(f_endpoint, router)
}
