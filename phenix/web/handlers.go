package web

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"phenix/api/experiment"
	"phenix/api/vm"
	"phenix/web/broker"
	"phenix/web/database"
	"phenix/web/rbac"
	"phenix/web/types"
	"phenix/web/util"

	log "github.com/activeshadow/libminimega/minilog"
	"github.com/dgrijalva/jwt-go"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/net/websocket"
	"golang.org/x/sync/errgroup"
)

// GET /experiments
func GetExperiments(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperiments HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		query = r.URL.Query()
		size  = query.Get("screenshot")
	)

	if !role.Allowed("experiments", "list") {
		log.Warn("listing experiments not allowed for %s", ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	experiments, err := experiment.List()
	if err != nil {
		log.Error("getting experiments - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []map[string]interface{}{}

	for _, exp := range experiments {
		if !role.Allowed("experiments", "list", exp.Metadata.Name) {
			continue
		}

		// This will happen if another handler is currently acting on the
		// experiment.
		status := isExperimentLocked(exp.Metadata.Name)

		// TODO: limit per-experiment VMs based on RBAC

		vms, err := vm.List(exp.Spec.ExperimentName)
		if err != nil {
			// TODO
		}

		if exp.Status.Running() && size != "" {
			for i, v := range vms {
				if !v.Running {
					continue
				}

				screenshot, err := util.GetScreenshot(exp.Spec.ExperimentName, v.Name, size)
				if err != nil {
					log.Error("getting screenshot - %v", err)
					continue
				}

				v.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)

				vms[i] = v
			}
		}

		allowed = append(allowed, exp.ToUI(status, vms))
	}

	marshalled, err := json.Marshal(map[string]interface{}{"experiments": allowed})
	if err != nil {
		log.Error("marshaling experiments - %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// POST /experiments
func CreateExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("CreateExperiment HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("experiments", "create") {
		log.Warn("creating experiments not allowed for %s", ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var req types.CreateExperiment
	if err := json.Unmarshal(body, &req); err != nil {
		log.Error("unmashaling request body - %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if err := lockExperimentForCreation(req.Name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockExperiment(req.Name)

	opts := []experiment.CreateOption{
		experiment.CreateWithName(req.Name),
		experiment.CreateWithTopology(req.Topology),
		experiment.CreateWithScenario(req.Scenario),
		experiment.CreateWithVLANMin(req.VLANMin),
		experiment.CreateWithVLANMax(req.VLANMax),
	}

	if err := experiment.Create(opts...); err != nil {
		log.Error("creating experiment %s - %v", req.Name, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	exp, err := experiment.Get(req.Name)
	if err != nil {
		log.Error("getting experiment %s - %v", req.Name, err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(experiment)
	if err != nil {
		log.Error("marshaling experiment %s - %v", experiment.Name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("experiments", "get", experiment.Name),
		broker.NewResource("experiment", experiment.Name, "create"),
		marshalled,
	)
}

// GET /experiments/{name}
func GetExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperiment HTTP handler called")

	var (
		ctx     = r.Context()
		role    = ctx.Value("role").(rbac.Role)
		vars    = mux.Vars(r)
		name    = vars["name"]
		query   = r.URL.Query()
		size    = query.Get("screenshot")
		sortCol = query.Get("sortCol")
		sortDir = query.Get("sortDir")
		pageNum = query.Get("pageNum")
		perPage = query.Get("perPage")
	)

	if !role.Allowed("experiments", "get", name) {
		log.Warn("getting experiment %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	exp, err := experiment.Get(name)
	if err != nil {
		log.Error("getting experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// This will happen if another handler is currently acting on the
	// experiment.
	if status := isExperimentLocked(name); status != "" {
		experiment.Status = status
	}

	allowed := types.VMs{}

	for _, vm := range experiment.VMs {
		if role.Allowed("vms", "list", fmt.Sprintf("%s_%s", name, vm.Name)) {
			if vm.Running && size != "" {
				screenshot, err := util.GetScreenshot(name, vm.Name, size)
				if err != nil {
					log.Error("getting screenshot: %v", err)
				} else {
					vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
				}
			}

			allowed = append(allowed, vm)
		}
	}

	if sortCol != "" && sortDir != "" {
		allowed.SortBy(sortCol, sortDir == "asc")
	}

	if pageNum != "" && perPage != "" {
		n, _ := strconv.Atoi(pageNum)
		s, _ := strconv.Atoi(perPage)

		allowed = allowed.Paginate(n, s)
	}

	experiment.VMs = allowed

	marshalled, err := json.Marshal(experiment)
	if err != nil {
		log.Error("marshaling experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// DELETE /experiments/{name}
func DeleteExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("DeleteExperiment HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments", "delete", name) {
		log.Warn("deleting experiment %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockExperimentForDeletion(name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockExperiment(name)

	if err := experiment.Delete(name); err != nil {
		log.Error("deleting experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("experiments", "delete", name),
		broker.NewResource("experiment", name, "delete"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

// POST /experiments/{name}/start
func StartExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("StartExperiment HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/start", "update", name) {
		log.Warn("starting experiment %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockExperimentForStarting(name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockExperiment(name)

	broker.Broadcast(
		broker.NewRequestPolicy("experiments/start", "update", name),
		broker.NewResource("experiment", name, "starting"),
		nil,
	)

	type result struct {
		exp *types.Experiment
		err error
	}

	status := make(chan result)

	go func() {
		exp, err := api.ControlExperiment(name, "start")
		status <- result{exp, err}
	}()

	var progress float64

	for {
		select {
		case s := <-status:
			if s.err != nil {
				broker.Broadcast(
					broker.NewRequestPolicy("experiments/start", "update", name),
					broker.NewResource("experiment", name, "errorStarting"),
					nil,
				)

				log.Error("starting experiment %s - %v", name, s.err)
				http.Error(w, s.err.Error(), http.StatusBadRequest)
				return
			}

			marshalled, err := json.Marshal(s.exp)
			if err != nil {
				log.Error("marshaling experiment %s - %v", name, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			broker.Broadcast(
				broker.NewRequestPolicy("experiments/start", "update", name),
				broker.NewResource("experiment", name, "start"),
				marshalled,
			)

			w.Write(marshalled)
			return
		default:
			p, err := api.GetExperimentProgress(name)
			if err != nil {
				log.Error("getting progress for experiment %s - %v", name, err)
				continue
			}

			if p > progress {
				progress = p
			}

			log.Info("percent deployed: %v", progress*100.0)

			status := map[string]interface{}{
				"percent": progress,
			}

			marshalled, _ := json.Marshal(status)

			broker.Broadcast(
				broker.NewRequestPolicy("experiments/start", "update", name),
				broker.NewResource("experiment", name, "progress"),
				marshalled,
			)

			time.Sleep(2 * time.Second)
		}
	}
}

// POST /experiments/{name}/stop
func StopExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("StopExperiment HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/stop", "update", name) {
		log.Warn("stopping experiment %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockExperimentForStopping(name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockExperiment(name)

	broker.Broadcast(
		broker.NewRequestPolicy("experiments/stop", "update", name),
		broker.NewResource("experiment", name, "stopping"),
		nil,
	)

	experiment, err := api.ControlExperiment(name, "stop")
	if err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("experiments/stop", "update", name),
			broker.NewResource("experiment", name, "errorStopping"),
			nil,
		)

		log.Error("stopping experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	marshalled, err := json.Marshal(experiment)
	if err != nil {
		log.Error("marshaling experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("experiments/stop", "update", name),
		broker.NewResource("experiment", name, "stop"),
		marshalled,
	)

	w.Write(marshalled)
}

// GET /experiments/{name}/schedule
func GetExperimentSchedule(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperimentSchedule HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/schedule", "get", name) {
		log.Warn("getting experiment schedule for %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if status := isExperimentLocked(name); status != "" {
		msg := fmt.Sprintf("experiment %s is locked with status %s", name, status)

		log.Warn(msg)
		http.Error(w, msg, http.StatusConflict)

		return
	}

	schedule, err := api.GetExperimentSchedule(name)
	if err != nil {
		log.Error("getting schedule for experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(schedule)
	if err != nil {
		log.Error("marshaling schedule for experiment %s - %v", name, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// POST /experiments/{name}/schedule
func ScheduleExperiment(w http.ResponseWriter, r *http.Request) {
	log.Debug("ScheduleExperiment HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/schedule", "create", name) {
		log.Warn("creating experiment schedule for %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if status := isExperimentLocked(name); status != "" {
		msg := fmt.Sprintf("experiment %s is locked with status %s", name, status)

		log.Warn(msg)
		http.Error(w, msg, http.StatusConflict)

		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req types.UpdateSchedule
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Error("unmarshaling request body - %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	schedule, err := api.ScheduleExperiment(name, req.Algorithm)
	if err != nil {
		log.Error("scheduling experiment %s using %s - %v", name, req.Algorithm, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(schedule)
	if err != nil {
		log.Error("marshaling schedule for experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("experiments/schedule", "create", name),
		broker.NewResource("experiment", name, "schedule"),
		marshalled,
	)

	w.Write(marshalled)
}

// GET /experiments/{name}/captures
func GetExperimentCaptures(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperimentCaptures HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/captures", "list", name) {
		log.Warn("listing experiment captures for %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var (
		captures = api.GetExperimentCaptures(name)
		allowed  []types.Capture
	)

	for _, capture := range captures {
		if role.Allowed("experiments/captures", "list", capture.VM) {
			allowed = append(allowed, capture)
		}
	}

	marshalled, err := json.Marshal(types.Captures{Captures: allowed})
	if err != nil {
		log.Error("marshaling captures for experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /experiments/{name}/files
func GetExperimentFiles(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperimentFiles HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
	)

	if !role.Allowed("experiments/files", "list", name) {
		log.Warn("listing experiment files for %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	files, err := api.GetExperimentFiles(name)
	if err != nil {
		log.Error("getting list of files for experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(map[string]interface{}{"files": files})
	if err != nil {
		log.Error("marshaling file list for experiment %s - %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /experiments/{name}/files/{filename}
func GetExperimentFile(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetExperimentFile HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		name = vars["name"]
		file = vars["filename"]
	)

	if !role.Allowed("experiments/files", "get", name) {
		log.Warn("getting experiment file for %s not allowed for %s", name, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	contents, err := api.GetExperimentFile(name, file)
	if err != nil {
		log.Error("getting file %s for experiment %s - %v", file, name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+file)
	http.ServeContent(w, r, "", time.Now(), bytes.NewReader(contents))
}

// GET /experiments/{exp}/vms
func GetVMs(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVMs HTTP handler called")

	var (
		ctx     = r.Context()
		role    = ctx.Value("role").(rbac.Role)
		vars    = mux.Vars(r)
		exp     = vars["exp"]
		query   = r.URL.Query()
		size    = query.Get("screenshot")
		sortCol = query.Get("sortCol")
		sortDir = query.Get("sortDir")
		pageNum = query.Get("pageNum")
		perPage = query.Get("perPage")
	)

	if !role.Allowed("vms", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	vms, err := api.GetVMs(exp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := types.VMs{}

	for _, vm := range vms {
		if role.Allowed("vms", "list", fmt.Sprintf("%s_%s", exp, vm.Name)) {
			if vm.Running && size != "" {
				screenshot, err := util.GetScreenshot(exp, vm.Name, size)
				if err != nil {
					log.Error("getting screenshot: %v", err)
				} else {
					vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
				}
			}

			allowed = append(allowed, vm)
		}
	}

	if sortCol != "" && sortDir != "" {
		allowed.SortBy(sortCol, sortDir == "asc")
	}

	if pageNum != "" && perPage != "" {
		n, _ := strconv.Atoi(pageNum)
		s, _ := strconv.Atoi(perPage)

		allowed = allowed.Paginate(n, s)
	}

	marshalled, err := json.Marshal(util.WithRoot("vms", allowed))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /experiments/{exp}/vms/{name}
func GetVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVM HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		vars  = mux.Vars(r)
		exp   = vars["exp"]
		name  = vars["name"]
		query = r.URL.Query()
		size  = query.Get("screenshot")
	)

	if !role.Allowed("vms", "get", fmt.Sprintf("%s_%s", exp, name)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	vm, err := api.GetVM(exp, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if vm.Running && size != "" {
		screenshot, err := util.GetScreenshot(exp, name, size)
		if err != nil {
			log.Error("getting screenshot: %v", err)
		} else {
			vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
		}
	}

	marshalled, err := json.Marshal(vm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// PATCH /experiments/{exp}/vms/{name}
func UpdateVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("UpdateVM HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms", "patch", fmt.Sprintf("%s_%s", exp, name)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req map[string]interface{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	vm, err := api.UpdateVM(exp, name, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if vm.Running {
		screenshot, err := util.GetScreenshot(exp, name, "215")
		if err != nil {
			log.Error("getting screenshot: %v", err)
		} else {
			vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
		}
	}

	marshalled, err := json.Marshal(vm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms", "patch", fmt.Sprintf("%s_%s", exp, name)),
		broker.NewResource("experiment/vm", fmt.Sprintf("%s/%s", exp, name), "update"),
		marshalled,
	)

	w.Write(marshalled)
}

// DELETE /experiments/{exp}/vms/{name}
func DeleteVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("DeleteVM HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms", "delete", fmt.Sprintf("%s_%s", exp, name)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	experiment, err := api.GetExperiment(exp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !experiment.Running {
		http.Error(w, "experiment not running", http.StatusBadRequest)
		return
	}

	if err := api.DeleteVM(exp, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms", "delete", fmt.Sprintf("%s_%s", exp, name)),
		broker.NewResource("experiment/vm", fmt.Sprintf("%s/%s", exp, name), "delete"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

// POST /experiments/{exp}/vms/{name}/start
func StartVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("StartVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
	)

	if !role.Allowed("vms/start", "update", fullName) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockVMForStarting(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/start", "update", fullName),
		broker.NewResource("experiment/vm", name, "starting"),
		nil,
	)

	if err := api.StartVM(exp, name); err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/start", "update", fullName),
			broker.NewResource("experiment/vm", name, "errorStarting"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	vm, err := api.GetVM(exp, name)
	if err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/start", "update", fullName),
			broker.NewResource("experiment/vm", name, "errorStarting"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	screenshot, err := util.GetScreenshot(exp, name, "215")
	if err != nil {
		log.Error("getting screenshot - %v", err)
	} else {
		vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
	}

	marshalled, err := json.Marshal(vm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/start", "update", fullName),
		broker.NewResource("experiment/vm", exp+"/"+name, "start"),
		marshalled,
	)

	w.Write(marshalled)
}

// POST /experiments/{exp}/vms/{name}/stop
func StopVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("StopVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
	)

	if !role.Allowed("vms/stop", "update", fullName) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockVMForStopping(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/stop", "update", fullName),
		broker.NewResource("experiment/vm", name, "stopping"),
		nil,
	)

	if err := api.StopVM(exp, name); err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/stop", "update", fullName),
			broker.NewResource("experiment/vm", name, "errorStopping"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	vm, err := api.GetVM(exp, name)
	if err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/stop", "update", fullName),
			broker.NewResource("experiment/vm", name, "errorStopping"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(vm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/stop", "update", fullName),
		broker.NewResource("experiment/vm", exp+"/"+name, "stop"),
		marshalled,
	)

	w.Write(marshalled)
}

// POST /experiments/{exp}/vms/{name}/redeploy
func RedeployVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("RedeployVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
		query    = r.URL.Query()
		inject   = query.Get("replicate-injects") != ""
	)

	if !role.Allowed("vms/redeploy", "update", fullName) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockVMForRedeploying(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	vm, err := api.GetVM(exp, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	vm.Redeploying = true

	marshalled, _ := json.Marshal(vm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/redeploy", "update", fullName),
		broker.NewResource("experiment/vm", exp+"/"+name, "redeploying"),
		marshalled,
	)

	redeployed := make(chan error)

	go func() {
		defer close(redeployed)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil && err != io.EOF {
			redeployed <- err
			return
		}

		// `body` will be nil if err above was EOF.
		if body != nil {
			// Update VM struct with values from POST request body.
			if err := json.Unmarshal(body, vm); err != nil {
				redeployed <- err
				return
			}
		}

		if err := api.RedeployVM(exp, *vm, inject); err != nil {
			redeployed <- err
		}

		vm.Redeploying = false
	}()

	// HACK: mandatory sleep time to make it seem like a redeploy is
	// happening client-side, even when the redeploy is fast (like for
	// Linux VMs).
	time.Sleep(5 * time.Second)

	err = <-redeployed
	if err != nil {
		log.Error("redeploying VM %s - %v", fullName, err)

		broker.Broadcast(
			broker.NewRequestPolicy("vms/redeploy", "update", fullName),
			broker.NewResource("experiment/vm", exp+"/"+name, "errorRedeploying"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	screenshot, err := util.GetScreenshot(exp, name, "215")
	if err != nil {
		log.Error("getting screenshot - %v", err)
	} else {
		vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
	}

	marshalled, _ = json.Marshal(vm)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/redeploy", "update", fullName),
		broker.NewResource("experiment/vm", exp+"/"+name, "redeployed"),
		marshalled,
	)

	w.Write(marshalled)
}

// GET /experiments/{exp}/vms/{name}/screenshot.png
func GetScreenshot(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetScreenshot HTTP handler called")

	var (
		ctx    = r.Context()
		role   = ctx.Value("role").(rbac.Role)
		vars   = mux.Vars(r)
		exp    = vars["exp"]
		name   = vars["name"]
		query  = r.URL.Query()
		size   = query.Get("size")
		encode = query.Get("base64") != ""
	)

	if !role.Allowed("vms/screenshot", "get", fmt.Sprintf("%s_%s", exp, name)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if size == "" {
		size = "215"
	}

	screenshot, err := util.GetScreenshot(exp, name, size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if encode {
		encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
		w.Write([]byte(encoded))
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(screenshot)
}

// GET /experiments/{exp}/vms/{name}/vnc
func GetVNC(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVNC HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms/vnc", "get", fmt.Sprintf("%s_%s", exp, name)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	bfs := util.NewBinaryFileSystem(
		&assetfs.AssetFS{
			Asset:     Asset,
			AssetDir:  AssetDir,
			AssetInfo: AssetInfo,
		},
	)

	// set no-cache headers
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // HTTP 1.1.
	w.Header().Set("Pragma", "no-cache")                                   // HTTP 1.0.
	w.Header().Set("Expires", "0")                                         // Proxies.

	bfs.ServeFile(w, r, "vnc.html")
}

// GET /experiments/{exp}/vms/{name}/vnc/ws
func GetVNCWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVNCWebSocket HTTP handler called")

	var (
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	endpoint, err := api.GetVNCEndpoint(exp, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	websocket.Handler(util.ConnectWSHandler(endpoint)).ServeHTTP(w, r)
}

// GET /experiments/{exp}/vms/{name}/captures
func GetVMCaptures(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVMCaptures HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms/captures", "list", fmt.Sprintf("%s_%s", exp, name)) {
		log.Warn("getting captures for VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	captures := api.GetVMCaptures(exp, name)

	marshalled, err := json.Marshal(types.Captures{Captures: captures})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// POST /experiments/{exp}/vms/{name}/captures
func StartVMCapture(w http.ResponseWriter, r *http.Request) {
	log.Debug("StartVMCapture HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms/captures", "create", fmt.Sprintf("%s_%s", exp, name)) {
		log.Warn("starting capture for VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req types.StartCapture
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Error("unmarshaling request body - %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := api.StartVMCapture(exp, name, req.Interface, req.Filename); err != nil {
		log.Error("starting VM capture for VM %s in experiment %s - %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/captures", "create", fmt.Sprintf("%s_%s", exp, name)),
		broker.NewResource("experiment/vm/capture", fmt.Sprintf("%s/%s", exp, name), "start"),
		body,
	)

	w.WriteHeader(http.StatusNoContent)
}

// DELETE /experiments/{exp}/vms/{name}/captures
func StopVMCaptures(w http.ResponseWriter, r *http.Request) {
	log.Debug("StopVMCaptures HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms/captures", "delete", fmt.Sprintf("%s_%s", exp, name)) {
		log.Warn("stopping capture for VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	err := api.StopVMCaptures(exp, name)
	if err != nil && errors.Cause(err) != api.ErrNoCaptures {
		log.Error("stopping VM capture for VM %s in experiment %s - %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/captures", "delete", fmt.Sprintf("%s_%s", exp, name)),
		broker.NewResource("experiment/vm/capture", fmt.Sprintf("%s/%s", exp, name), "stop"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

// GET /experiments/{exp}/vms/{name}/snapshots
func GetVMSnapshots(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetVMSnapshots HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
		vars = mux.Vars(r)
		exp  = vars["exp"]
		name = vars["name"]
	)

	if !role.Allowed("vms/snapshots", "list", fmt.Sprintf("%s_%s", exp, name)) {
		log.Warn("listing snapshots for VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	snapshots, err := api.GetVMSnapshots(exp, name)
	if err != nil {
		log.Error("getting list of snapshots for VM %s in experiment %s: %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(map[string]interface{}{"snapshots": snapshots})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// POST /experiments/{exp}/vms/{name}/snapshots
func SnapshotVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("SnapshotVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
	)

	if !role.Allowed("vms/snapshots", "create", fullName) {
		log.Warn("snapshotting VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req types.CaptureSnapshot
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Error("unmarshaling request body - %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := lockVMForSnapshotting(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/snapshots", "create", fullName),
		broker.NewResource("experiment/vm/snapshot", exp+"/"+name, "creating"),
		nil,
	)

	status := make(chan string)

	go func() {
		for {
			s := <-status

			if s == "completed" {
				return
			}

			progress, err := strconv.ParseFloat(s, 64)
			if err == nil {
				log.Info("snapshot percent complete: %v", progress)

				status := map[string]interface{}{
					"percent": progress / 100,
				}

				marshalled, _ := json.Marshal(status)

				broker.Broadcast(
					broker.NewRequestPolicy("vms/snapshots", "create", fullName),
					broker.NewResource("experiment/vm/snapshot", exp+"/"+name, "progress"),
					marshalled,
				)
			}
		}
	}()

	cb := func(s string) { status <- s }

	if err := api.SnapshotVM(exp, name, req.Filename, cb); err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/snapshots", "create", fullName),
			broker.NewResource("experiment/vm/snapshot", exp+"/"+name, "errorCreating"),
			nil,
		)

		log.Error("snapshotting VM %s in experiment %s - %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/snapshots", "create", fullName),
		broker.NewResource("experiment/vm/snapshot", exp+"/"+name, "create"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

// POST /experiments/{exp}/vms/{name}/snapshots/{snapshot}
func RestoreVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("RestoreVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
		snap     = vars["snapshot"]
	)

	if !role.Allowed("vms/snapshots", "update", fullName) {
		log.Warn("restoring VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := lockVMForRestoring(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/snapshots", "create", fullName),
		broker.NewResource("experiment/vm/snapshot", fmt.Sprintf("%s/%s", exp, name), "restoring"),
		nil,
	)

	if err := api.RestoreVM(exp, name, snap); err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/snapshots", "create", fullName),
			broker.NewResource("experiment/vm/snapshot", fmt.Sprintf("%s/%s", exp, name), "errorRestoring"),
			nil,
		)

		log.Error("restoring VM %s in experiment %s - %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("vms/snapshots", "create", fullName),
		broker.NewResource("experiment/vm/snapshot", exp+"/"+name, "restore"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

// POST /experiments/{exp}/vms/{name}/commit
func CommitVM(w http.ResponseWriter, r *http.Request) {
	log.Debug("CommitVM HTTP handler called")

	var (
		ctx      = r.Context()
		role     = ctx.Value("role").(rbac.Role)
		vars     = mux.Vars(r)
		exp      = vars["exp"]
		name     = vars["name"]
		fullName = exp + "_" + name
	)

	if !role.Allowed("vms/commit", "create", fullName) {
		log.Warn("committing VM %s in experiment %s not allowed for %s", name, exp, ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var filename string

	// If user provided body to this request, expect it to specify the
	// filename to use for the commit. If no body was provided, pass an
	// empty string to `api.CommitToDisk` to let it create a copy based on
	// the existing file name for the base image.
	if len(body) != 0 {
		var req types.BackingImage
		err = json.Unmarshal(body, &req)
		if err != nil {
			log.Error("unmarshaling request body - %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Filename == "" {
			log.Error("missing filename for commit")
			http.Error(w, "missing 'filename' key", http.StatusBadRequest)
			return
		}

		filename = req.Filename
	}

	if err := lockVMForCommitting(exp, name); err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	defer unlockVM(exp, name)

	if filename == "" {
		if filename, err = api.GetNewDiskName(exp, name); err != nil {
			log.Error("failure getting new disk name for commit")
			http.Error(w, "failure getting new disk name for commit", http.StatusInternalServerError)
			return
		}
	}

	payload := map[string]interface{}{"disk": filename}
	marshalled, _ := json.Marshal(payload)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/commit", "create", fullName),
		broker.NewResource("experiment/vm/commit", exp+"/"+name, "committing"),
		marshalled,
	)

	status := make(chan float64)

	go func() {
		for s := range status {
			log.Info("VM commit percent complete: %v", s)

			status := map[string]interface{}{
				"percent": s,
			}

			marshalled, _ := json.Marshal(status)

			broker.Broadcast(
				broker.NewRequestPolicy("vms/commit", "create", fullName),
				broker.NewResource("experiment/vm/commit", exp+"/"+name, "progress"),
				marshalled,
			)
		}
	}()

	cb := func(s float64) { status <- s }

	if filename, err = api.CommitToDisk(exp, name, filename, cb); err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/commit", "create", fullName),
			broker.NewResource("experiment/vm/commit", exp+"/"+name, "errorCommitting"),
			nil,
		)

		log.Error("committing VM %s in experiment %s - %v", name, exp, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	vm, err := api.GetVM(exp, name)
	if err != nil {
		broker.Broadcast(
			broker.NewRequestPolicy("vms/commit", "create", fullName),
			broker.NewResource("experiment/vm/commit", exp+"/"+name, "errorCommitting"),
			nil,
		)

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	payload["vm"] = vm
	marshalled, _ = json.Marshal(payload)

	broker.Broadcast(
		broker.NewRequestPolicy("vms/commit", "create", fmt.Sprintf("%s_%s", exp, name)),
		broker.NewResource("experiment/vm/commit", exp+"/"+name, "commit"),
		marshalled,
	)

	w.Write(marshalled)
}

// GET /vms
func GetAllVMs(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetAllVMs HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		query = r.URL.Query()
		size  = query.Get("screenshot")
	)

	if !role.Allowed("vms", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	experiments, err := api.GetExperiments()
	if err != nil {
		log.Error("getting experiments: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := types.VMs{}
	for _, exp := range experiments {
		if !exp.Running {
			continue
		}

		for _, vm := range exp.VMs {
			if role.Allowed("vms", "list", fmt.Sprintf("%s_%s", exp.Name, vm.Name)) {
				if vm.Running && size != "" {
					screenshot, err := util.GetScreenshot(exp.Name, vm.Name, size)
					if err != nil {
						log.Error("getting screenshot: %v", err)
					} else {
						vm.Screenshot = "data:image/png;base64," + base64.StdEncoding.EncodeToString(screenshot)
					}
				}

				allowed = append(allowed, vm)
			}
		}
	}

	marshalled, err := json.Marshal(util.WithRoot("vms", allowed))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /applications
func GetApplications(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetApplications HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("applications", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	applications, err := api.GetApplications()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []string{}
	for _, app := range applications {
		if role.Allowed("applications", "list", app) {
			allowed = append(allowed, app)
		}
	}

	marshalled, err := json.Marshal(types.Application{Applications: allowed})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /topologies
func GetTopologies(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetTopologies HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("topologies", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	topologies, err := api.GetTopologies()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []string{}
	for _, topo := range topologies {
		if role.Allowed("topologies", "list", topo) {
			allowed = append(allowed, topo)
		}
	}

	marshalled, err := json.Marshal(types.Topology{Topologies: allowed})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /disks
func GetDisks(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetDisks HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("disks", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	disks, err := api.GetDiskImages()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []string{}
	for _, disk := range disks {
		if role.Allowed("disks", "list", disk) {
			allowed = append(allowed, disk)
		}
	}

	marshalled, err := json.Marshal(map[string]interface{}{"disks": allowed})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /hosts
func GetClusterHosts(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetClusterHosts HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("hosts", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	hosts, err := api.GetClusterHosts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []types.Host{}
	for _, host := range hosts {
		if role.Allowed("hosts", "list", host.Name) {
			allowed = append(allowed, host)
		}
	}

	marshalled, err := json.Marshal(types.Cluster{Hosts: allowed})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// GET /logs
func GetLogs(w http.ResponseWriter, r *http.Request) {
	if !f_serviceLogs {
		w.WriteHeader(http.StatusNotImplemented)
	}

	type LogLine struct {
		Source    string `json:"source"`
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		Log       string `json:"log"`

		// Not exported so it doesn't get included in serialized JSON.
		ts time.Time
	}

	var (
		since time.Duration
		limit int

		logs    = make(map[int][]LogLine)
		logChan = make(chan LogLine)
		done    = make(chan struct{})
		wait    errgroup.Group

		logFiles = map[string]string{
			"minimega": f_mmLogFile,
			"phenix":   f_phenixLogFile,
			"gophenix": f_gophenixLogFile,
		}
	)

	// If no since duration is provided, or the value provided is not a
	// valid duration string, since will default to 1h.
	if err := parseDuration(r.URL.Query().Get("since"), &since); err != nil {
		since = 1 * time.Hour
	}

	// If no limit is provided, or the value provided is not an int, limit
	// will default to 0.
	parseInt(r.URL.Query().Get("limit"), &limit)

	go func() {
		for l := range logChan {
			ts := int(l.ts.Unix())

			tl := logs[ts]
			tl = append(tl, l)

			logs[ts] = tl
		}

		close(done)
	}()

	for k := range logFiles {
		name := k
		path := logFiles[k]

		wait.Go(func() error {
			f, err := os.Open(path)
			if err != nil {
				// This *most likely* means the log file doesn't exist yet, so just exit
				// out of the Goroutine cleanly.
				return nil
			}

			defer f.Close()

			var (
				scanner = bufio.NewScanner(f)
				// Used to detect multi-line logs in tailed log files.
				body *LogLine
			)

			for scanner.Scan() {
				parts := logLineRegex.FindStringSubmatch(scanner.Text())

				if len(parts) == 4 {
					ts, err := time.ParseInLocation("2006/01/02 15:04:05", parts[1], time.Local)
					if err != nil {
						continue
					}

					if time.Now().Sub(ts) > since {
						continue
					}

					if parts[2] == "WARNING" {
						parts[2] = "WARN"
					}

					body = &LogLine{
						Source:    name,
						Timestamp: parts[1],
						Level:     parts[2],
						Log:       parts[3],

						ts: ts,
					}
				} else if body != nil {
					body.Log = scanner.Text()
				} else {
					continue
				}

				logChan <- *body
			}

			if err := scanner.Err(); err != nil {
				return errors.WithMessagef(err, "scanning %s log file at %s", name, path)
			}

			return nil
		})
	}

	if err := wait.Wait(); err != nil {
		http.Error(w, "error reading logs", http.StatusInternalServerError)
		return
	}

	// Close log channel, marking it as done.
	close(logChan)
	// Wait for Goroutine processing logs from log channel to be done.
	<-done

	var (
		idx, offset int
		ts          = make([]int, len(logs))
		limited     []LogLine
	)

	// Put log timestamps into slice so they can be sorted.
	for k := range logs {
		ts[idx] = k
		idx++
	}

	// Sort log timestamps.
	sort.Ints(ts)

	// Determine if final log slice should be limited.
	if limit != 0 && limit < len(ts) {
		offset = len(ts) - limit
	}

	// Loop through sorted, limited log timestamps and grab actual logs
	// for given timestamp.
	for _, k := range ts[offset:] {
		limited = append(limited, logs[k]...)
	}

	marshalled, _ := json.Marshal(util.WithRoot("logs", limited))
	w.Write(marshalled)
}

// GET /users
func GetUsers(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetUsers HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("users", "list") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	users, err := database.GetUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allowed := []rbac.User{}
	for _, user := range users {
		if role.Allowed("users", "list", user.Username) {
			allowed = append(allowed, user)
		}
	}

	marshalled, err := json.Marshal(map[string]interface{}{"users": allowed})
	if err != nil {
		log.Error("marshaling users: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// POST /users
func CreateUser(w http.ResponseWriter, r *http.Request) {
	log.Debug("CreateUser HTTP handler called")

	var (
		ctx  = r.Context()
		role = ctx.Value("role").(rbac.Role)
	)

	if !role.Allowed("users", "create") {
		log.Warn("creating users not allowed for %s", ctx.Value("user").(string))
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("reading request body - %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Error("unmashaling request body - %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var user rbac.User

	if uname, ok := data["username"].(string); ok {
		user.Username = uname
	} else {
		log.Error("creating user: no username provided")
		http.Error(w, "must provide username", http.StatusBadRequest)
		return
	}

	if pass, ok := data["password"].(string); ok {
		user.Password = pass
	} else {
		log.Error("creating user: no password provided")
		http.Error(w, "must provide password", http.StatusBadRequest)
		return
	}

	if first, ok := data["first_name"].(string); ok {
		user.FirstName = first
	}

	if last, ok := data["last_name"].(string); ok {
		user.LastName = last
	}

	if rname, ok := data["role_name"].(string); ok {
		policies := rbac.CreateBasePoliciesForRole(rname)

		if resources, ok := data["resource_names"].([]interface{}); ok {
			names := make([]string, len(resources))
			for i, name := range resources {
				if name, ok := name.(string); ok {
					names[i] = name
				}
			}

			for _, policy := range policies {
				policy.SetResourceNames(names...)
			}
		}

		user.Role = rbac.NewRole(rname, policies...)

		// allow user to get their own user details
		user.Role.AddPolicies(&rbac.Policy{
			Resources:     []string{"users"},
			ResourceNames: []string{user.Username},
			Verbs:         []string{"get"},
		})
	}

	// NOTE: The `AddUser` function clears the provided password
	if err := database.AddUser(&user); err != nil {
		http.Error(w, "error creating user", http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(user)
	if err != nil {
		log.Error("marshaling user %s: %v", user.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("users", "create", ""),
		broker.NewResource("user", user.Username, "create"),
		marshalled,
	)

	w.Write(marshalled)
}

// GET /users/{username}
func GetUser(w http.ResponseWriter, r *http.Request) {
	log.Debug("GetUser HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		vars  = mux.Vars(r)
		uname = vars["username"]
	)

	if !role.Allowed("users", "get", uname) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	user, err := database.GetUser(uname)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(user)
	if err != nil {
		log.Error("marshaling user %s: %v", user.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

// PATCH /users/{username}
func UpdateUser(w http.ResponseWriter, r *http.Request) {
	log.Debug("UpdateUser HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		vars  = mux.Vars(r)
		uname = vars["username"]
	)

	if !role.Allowed("users", "patch", uname) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var data map[string]interface{}

	if err := json.Unmarshal(body, &data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if first, ok := data["first_name"].(string); ok {
		if err := database.UpdateUserSetting(uname, "first_name", first); err != nil {
			log.Error("updating first_name for user %s: %v", uname, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if last, ok := data["last_name"].(string); ok {
		if err := database.UpdateUserSetting(uname, "last_name", last); err != nil {
			log.Error("updating last_name for user %s: %v", uname, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if rname, ok := data["role_name"].(string); ok {
		policies := rbac.CreateBasePoliciesForRole(rname)

		if resources, ok := data["resource_names"].([]interface{}); ok {
			names := make([]string, len(resources))
			for i, name := range resources {
				if name, ok := name.(string); ok {
					names[i] = name
				}
			}

			for _, policy := range policies {
				policy.SetResourceNames(names...)
			}
		}

		role := rbac.NewRole(rname, policies...)

		// allow user to get their own user details
		role.AddPolicies(&rbac.Policy{
			Resources:     []string{"users"},
			ResourceNames: []string{uname},
			Verbs:         []string{"get"},
		})

		if err := database.UpdateUserSetting(uname, "role", role); err != nil {
			log.Error("updating role for user %s: %v", uname, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	user, err := database.GetUser(uname)
	if err != nil {
		log.Error("getting user %s: %v", uname, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	marshalled, err := json.Marshal(user)
	if err != nil {
		log.Error("marshaling user %s: %v", uname, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("users", "patch", uname),
		broker.NewResource("user", uname, "update"),
		marshalled,
	)

	w.Write(marshalled)
}

// DELETE /users/{username}
func DeleteUser(w http.ResponseWriter, r *http.Request) {
	log.Debug("DeleteUser HTTP handler called")

	var (
		ctx   = r.Context()
		role  = ctx.Value("role").(rbac.Role)
		vars  = mux.Vars(r)
		uname = vars["username"]
	)

	if !role.Allowed("users", "delete", uname) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := database.DeleteUser(uname); err != nil {
		log.Error("deleting user %s: %v", uname, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	broker.Broadcast(
		broker.NewRequestPolicy("users", "delete", uname),
		broker.NewResource("user", uname, "delete"),
		nil,
	)

	w.WriteHeader(http.StatusNoContent)
}

func Signup(w http.ResponseWriter, r *http.Request) {
	log.Debug("Signup HTTP handler called")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "no data provided in POST", http.StatusBadRequest)
		return
	}

	var user rbac.User

	if err := json.Unmarshal(body, &user); err != nil {
		http.Error(w, "invalid data provided in POST", http.StatusBadRequest)
		return
	}

	// NOTE: The `AddUser` function clears the provided password
	if err := database.AddUser(&user); err != nil {
		http.Error(w, "error creating user", http.StatusInternalServerError)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.Username,
		"uid": user.ID,
	})

	// Sign and get the complete encoded token as a string using the secret
	signed, err := token.SignedString([]byte(f_jwtKey))
	if err != nil {
		http.Error(w, "failed to sign JWT", http.StatusInternalServerError)
		return
	}

	database.AddUserToken(user.Username, signed)

	data := map[string]interface{}{
		"username":   user.Username,
		"first_name": user.FirstName,
		"last_name":  user.LastName,
		"token":      signed,
	}

	marshalled, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

func Login(w http.ResponseWriter, r *http.Request) {
	log.Debug("Login HTTP handler called")

	var user, pass string

	switch r.Method {
	case "GET":
		query := r.URL.Query()

		user = query.Get("user")
		if user == "" {
			http.Error(w, "no username provided", http.StatusBadRequest)
			return
		}

		pass = query.Get("pass")
		if pass == "" {
			http.Error(w, "no password provided", http.StatusBadRequest)
			return
		}

	case "POST":
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "no data provided in POST", http.StatusBadRequest)
			return
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			http.Error(w, "invalid data provided in POST", http.StatusBadRequest)
			return
		}

		var ok bool

		if user, ok = data["user"].(string); !ok {
			http.Error(w, "invalid username provided in POST", http.StatusBadRequest)
			return
		}

		if pass, ok = data["pass"].(string); !ok {
			http.Error(w, "invalid password provided in POST", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}

	if err := database.ValidateUserPassword(user, pass); err != nil {
		http.Error(w, "invalid creds", http.StatusUnauthorized)
		return
	}

	u, err := database.GetUser(user)
	if err != nil {
		http.Error(w, "cannot find user", http.StatusBadRequest)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user,
		"uid": u.ID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	signed, err := token.SignedString([]byte(f_jwtKey))
	if err != nil {
		http.Error(w, "failed to sign JWT", http.StatusInternalServerError)
		return
	}

	database.AddUserToken(user, signed)

	data := map[string]interface{}{
		"username":   u.Username,
		"first_name": u.FirstName,
		"last_name":  u.LastName,
		"token":      signed,
	}

	if u.Role != nil {
		data["role"] = u.Role.Name
	}

	marshalled, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(marshalled)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	log.Debug("Logout HTTP handler called")

	token := r.Context().Value("jwt").(string)
	database.DeleteUserToken(token)

	w.WriteHeader(http.StatusNoContent)
}

func parseDuration(v string, d *time.Duration) error {
	var err error
	*d, err = time.ParseDuration(v)
	return err
}

func parseInt(v string, d *int) error {
	var err error
	*d, err = strconv.Atoi(v)
	return err
}
