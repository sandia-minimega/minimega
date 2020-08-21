package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"phenix/api/config"
	"phenix/web/broker"
	"phenix/web/middleware"
	"phenix/web/rbac"
	"phenix/web/util"

	log "github.com/activeshadow/libminimega/minilog"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
)

type ServerOption func(*serverOptions)

type serverOptions struct {
	endpoint  string
	jwtKey    string
	users     []string
	logs      string
	allowCORS bool
}

func newServerOptions(opts ...ServerOption) serverOptions {
	o := serverOptions{
		endpoint:  ":3000",
		users:     []string{"admin@foo.com:foobar:Global Admin"},
		allowCORS: true, // TODO: default to false
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func ServeOnEndpoint(e string) ServerOption {
	return func(o *serverOptions) {
		o.endpoint = e
	}
}

func ServeWithJWTKey(k string) ServerOption {
	return func(o *serverOptions) {
		o.jwtKey = k
	}
}

func ServeWithUsers(u string) ServerOption {
	return func(o *serverOptions) {
		o.users = strings.Split(u, "|")
	}
}

func ServeWithLogs(l string) ServerOption {
	return func(o *serverOptions) {
		o.logs = l
	}
}

func ServeWithCORS(c bool) ServerOption {
	return func(o *serverOptions) {
		o.allowCORS = c
	}
}

var o serverOptions

func Start(opts ...ServerOption) error {
	o = newServerOptions(opts...)

	for _, u := range o.users {
		creds := strings.Split(u, ":")
		uname := creds[0]
		pword := creds[1]
		rname := creds[2]

		if _, err := config.Get("user/" + uname); err == nil {
			continue
		}

		user := rbac.NewUser(uname, pword)

		role, err := rbac.RoleFromConfig(rname)
		if err != nil {
			return fmt.Errorf("getting %s role: %w", rname, err)
		}

		role.SetResourceNames(creds[3:]...)

		// allow user to get their own user details
		role.AddPolicy(
			[]string{"users"},
			[]string{uname},
			[]string{"get"},
		)

		user.SetRole(role)

		log.Debug("creating default user - %+v", user)
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
	//api.HandleFunc("/users/{username}", UpdateUser).Methods("PATCH", "OPTIONS")
	api.HandleFunc("/users/{username}", DeleteUser).Methods("DELETE", "OPTIONS")
	//api.HandleFunc("/signup", Signup).Methods("POST", "OPTIONS")
	api.HandleFunc("/login", Login).Methods("GET", "POST", "OPTIONS")
	api.HandleFunc("/logout", Logout).Methods("GET", "OPTIONS")
	api.HandleFunc("/ws", broker.ServeWS).Methods("GET")

	if o.allowCORS {
		log.Info("CORS is enabled on HTTP API endpoints")
		api.Use(middleware.AllowCORS)
	}

	switch o.logs {
	case "full":
		log.Info("full HTTP logging is enabled")
		api.Use(middleware.LogFull)
	case "requests":
		log.Info("requests-only HTTP logging is enabled")
		api.Use(middleware.LogRequests)
	}

	api.Use(middleware.AuthMiddleware(true, o.jwtKey))

	log.Info("Starting websockets broker")

	go broker.Start()

	log.Info("Starting log publisher")

	go PublishLogs(context.Background())

	log.Info("Starting HTTP server on %s", o.endpoint)

	return http.ListenAndServe(o.endpoint, router)
}
