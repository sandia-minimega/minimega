// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fcRecordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

type fcTestServer struct {
	mu   sync.Mutex
	reqs []fcRecordedRequest
	srv  *httptest.Server
}

func (s *fcTestServer) last(t *testing.T) fcRecordedRequest {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.reqs) == 0 {
		t.Fatal("server got no request")
	}
	return s.reqs[len(s.reqs)-1]
}

func (s *fcTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := fcRecordedRequest{Method: r.Method, Path: r.URL.EscapedPath()}

	data, _ := io.ReadAll(r.Body)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &rec.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"fault_message": "bad json: " + err.Error()})
			return
		}
	}

	s.mu.Lock()
	s.reqs = append(s.reqs, rec)
	s.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"app_name":    "Firecracker",
			"id":          "vm7",
			"state":       "Not started",
			"vmm_version": "1.17.0",
		})
	case r.Method == http.MethodGet && r.URL.Path == "/version":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"firecracker_version": "1.17.0"})
	case r.URL.Path == "/fail":
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"fault_message": "x"})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func fcListen(t *testing.T, socket string) net.Listener {
	t.Helper()

	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func newFCTestServer(t *testing.T, l net.Listener) *fcTestServer {
	s := &fcTestServer{}
	s.srv = httptest.NewUnstartedServer(s)
	s.srv.Listener.Close()
	s.srv.Listener = l
	s.srv.Start()
	t.Cleanup(s.srv.Close)
	return s
}

func TestFirecrackerAPIMethods(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	s := newFCTestServer(t, fcListen(t, socket))
	c := newFCClient(socket)
	ctx := context.Background()

	cases := []struct {
		name   string
		call   func() error
		method string
		path   string
		body   map[string]any
	}{
		{
			name:   "machine config",
			call:   func() error { return c.PutMachineConfig(2, 512) },
			method: "PUT",
			path:   "/machine-config",
			body: map[string]any{
				"vcpu_count":        2.0,
				"mem_size_mib":      512.0,
				"smt":               false,
				"track_dirty_pages": false,
			},
		},
		{
			name:   "boot source",
			call:   func() error { return c.PutBootSource("/vmlinux", "/initrd", "console=ttyS0") },
			method: "PUT",
			path:   "/boot-source",
			body: map[string]any{
				"kernel_image_path": "/vmlinux",
				"initrd_path":       "/initrd",
				"boot_args":         "console=ttyS0",
			},
		},
		{
			name:   "boot source without initrd",
			call:   func() error { return c.PutBootSource("/vmlinux", "", "") },
			method: "PUT",
			path:   "/boot-source",
			body:   map[string]any{"kernel_image_path": "/vmlinux"},
		},
		{
			name:   "drive",
			call:   func() error { return c.PutDrive("disk0", "/rootfs.raw", true, false, "Writeback") },
			method: "PUT",
			path:   "/drives/disk0",
			body: map[string]any{
				"drive_id":       "disk0",
				"path_on_host":   "/rootfs.raw",
				"is_root_device": true,
				"is_read_only":   false,
				"cache_type":     "Writeback",
			},
		},
		{
			name:   "network interface",
			call:   func() error { return c.PutNetworkInterface("eth0", "fc0", "00:11:22:33:44:55", 1500) },
			method: "PUT",
			path:   "/network-interfaces/eth0",
			body: map[string]any{
				"iface_id":      "eth0",
				"host_dev_name": "fc0",
				"guest_mac":     "00:11:22:33:44:55",
				"mtu":           1500.0,
			},
		},
		{
			name:   "network interface without mtu",
			call:   func() error { return c.PutNetworkInterface("eth1", "fc1", "", 0) },
			method: "PUT",
			path:   "/network-interfaces/eth1",
			body: map[string]any{
				"iface_id":      "eth1",
				"host_dev_name": "fc1",
			},
		},
		{
			name:   "vsock",
			call:   func() error { return c.PutVsock(3, "/v.sock") },
			method: "PUT",
			path:   "/vsock",
			body:   map[string]any{"guest_cid": 3.0, "uds_path": "/v.sock"},
		},
		{
			name:   "entropy",
			call:   c.PutEntropy,
			method: "PUT",
			path:   "/entropy",
			body:   map[string]any{},
		},
		{
			name:   "serial",
			call:   func() error { return c.PutSerial("/console.pipe") },
			method: "PUT",
			path:   "/serial",
			body: map[string]any{
				"serial_out_path": "/console.pipe",
				"rate_limiter": map[string]any{
					"size":        1048576.0,
					"refill_time": 1000.0,
				},
			},
		},
		{
			name:   "logger",
			call:   func() error { return c.PutLogger("/firecracker.log", "Info") },
			method: "PUT",
			path:   "/logger",
			body: map[string]any{
				"log_path":        "/firecracker.log",
				"level":           "Info",
				"show_level":      true,
				"show_log_origin": false,
			},
		},
		{
			name:   "action",
			call:   func() error { return c.Action("InstanceStart") },
			method: "PUT",
			path:   "/actions",
			body:   map[string]any{"action_type": "InstanceStart"},
		},
		{
			name:   "vm state",
			call:   func() error { return c.PatchVMState("Paused") },
			method: "PATCH",
			path:   "/vm",
			body:   map[string]any{"state": "Paused"},
		},
		{
			name:   "create snapshot",
			call:   func() error { return c.CreateSnapshot(ctx, "/out/vm.state", "/out/vm.mem") },
			method: "PUT",
			path:   "/snapshot/create",
			body: map[string]any{
				"snapshot_type":       "Full",
				"snapshot_path":       "/out/vm.state",
				"mem_file_path":       "/out/vm.mem",
				"sync_snapshot_files": false,
			},
		},
		{
			name:   "load snapshot",
			call:   func() error { return c.LoadSnapshot(ctx, "/vm.state", "/vm.mem", false) },
			method: "PUT",
			path:   "/snapshot/load",
			body: map[string]any{
				"snapshot_path": "/vm.state",
				"mem_backend": map[string]any{
					"backend_type": "File",
					"backend_path": "/vm.mem",
				},
				"resume_vm": false,
			},
		},
		{
			name:   "load snapshot with realtime clock",
			call:   func() error { return c.LoadSnapshot(ctx, "/vm.state", "/vm.mem", true) },
			method: "PUT",
			path:   "/snapshot/load",
			body: map[string]any{
				"snapshot_path": "/vm.state",
				"mem_backend": map[string]any{
					"backend_type": "File",
					"backend_path": "/vm.mem",
				},
				"resume_vm":      false,
				"clock_realtime": true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatal(err)
			}

			got := s.last(t)
			if got.Method != tc.method || got.Path != tc.path {
				t.Errorf("got %v %v, want %v %v", got.Method, got.Path, tc.method, tc.path)
			}
			if !reflect.DeepEqual(got.Body, tc.body) {
				t.Errorf("body = %#v, want %#v", got.Body, tc.body)
			}
		})
	}

	t.Run("instance info", func(t *testing.T) {
		info, err := c.GetInstanceInfo()
		if err != nil {
			t.Fatal(err)
		}
		if info.State != "Not started" || info.VMMVersion != "1.17.0" {
			t.Errorf("GetInstanceInfo() = %+v", info)
		}
		if got := s.last(t); got.Method != "GET" || got.Path != "/" {
			t.Errorf("got %v %v, want GET /", got.Method, got.Path)
		}
	})

	t.Run("version", func(t *testing.T) {
		v, err := c.GetVersion()
		if err != nil {
			t.Fatal(err)
		}
		if v != "1.17.0" {
			t.Errorf("GetVersion() = %q, want %q", v, "1.17.0")
		}
	})
}

func TestFirecrackerAPIFault(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	newFCTestServer(t, fcListen(t, socket))
	c := newFCClient(socket)

	err := c.put("/fail", fcInstanceAction{ActionType: "InstanceStart"})
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *fcAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %T is not *fcAPIError", err)
	}
	if apiErr.Fault != "x" || apiErr.Status != http.StatusBadRequest {
		t.Errorf("got %+v", apiErr)
	}
	for _, want := range []string{"PUT", "/fail", "400", "x"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestFirecrackerAPIWaitForSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	c := newFCClient(socket)

	started := make(chan *fcTestServer, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		l, err := net.Listen("unix", socket)
		if err != nil {
			started <- nil
			return
		}
		s := &fcTestServer{}
		s.srv = httptest.NewUnstartedServer(s)
		s.srv.Listener.Close()
		s.srv.Listener = l
		s.srv.Start()
		started <- s
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.WaitForSocket(ctx)

	s := <-started
	if s == nil {
		t.Fatal("unable to listen on the socket")
	}
	defer s.srv.Close()

	if err != nil {
		t.Fatal(err)
	}
}

func TestFirecrackerAPIWaitForSocketTimeout(t *testing.T) {
	c := newFCClient(filepath.Join(t.TempDir(), "api.sock"))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.WaitForSocket(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForSocket() = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("WaitForSocket() returned after %v", d)
	}
}
