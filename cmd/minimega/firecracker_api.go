// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// fcAPITimeout is the default timeout of one Firecracker API request.
const fcAPITimeout = 10 * time.Second

// Serial console rate limit: 1 MiB for each 1000 ms.
const (
	fcSerialRateBytes  = 1 << 20
	fcSerialRefillTime = 1000
)

// fcClient is a client for the Firecracker v1.17 API on a Unix socket.
type fcClient struct {
	socket  string
	timeout time.Duration
	http    *http.Client
}

// fcAPIError is the error for a non-2xx response from the Firecracker API.
type fcAPIError struct {
	Method string
	Path   string
	Status int
	Fault  string
}

func (e *fcAPIError) Error() string {
	return fmt.Sprintf("firecracker API %v %v: %v %v: %v", e.Method, e.Path, e.Status, http.StatusText(e.Status), e.Fault)
}

type fcMachineConfig struct {
	VcpuCount       int  `json:"vcpu_count"`
	MemSizeMib      int  `json:"mem_size_mib"`
	Smt             bool `json:"smt"`
	TrackDirtyPages bool `json:"track_dirty_pages"`
}

type fcBootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	InitrdPath      string `json:"initrd_path,omitempty"`
	BootArgs        string `json:"boot_args,omitempty"`
}

type fcDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
	CacheType    string `json:"cache_type,omitempty"`
}

type fcNetworkInterface struct {
	IfaceID     string `json:"iface_id"`
	HostDevName string `json:"host_dev_name"`
	GuestMAC    string `json:"guest_mac,omitempty"`
	MTU         int    `json:"mtu,omitempty"`
}

type fcVsock struct {
	GuestCID uint32 `json:"guest_cid"`
	UDSPath  string `json:"uds_path"`
}

type fcEntropyDevice struct{}

type fcTokenBucket struct {
	Size       int64 `json:"size"`
	RefillTime int64 `json:"refill_time"`
}

type fcSerialDevice struct {
	SerialOutPath string         `json:"serial_out_path"`
	RateLimiter   *fcTokenBucket `json:"rate_limiter,omitempty"`
}

type fcLogger struct {
	LogPath       string `json:"log_path"`
	Level         string `json:"level,omitempty"`
	ShowLevel     bool   `json:"show_level"`
	ShowLogOrigin bool   `json:"show_log_origin"`
}

type fcInstanceAction struct {
	ActionType string `json:"action_type"`
}

type fcVMState struct {
	State string `json:"state"`
}

type fcSnapshotCreate struct {
	SnapshotType      string `json:"snapshot_type"`
	SnapshotPath      string `json:"snapshot_path"`
	MemFilePath       string `json:"mem_file_path"`
	SyncSnapshotFiles bool   `json:"sync_snapshot_files"`
}

type fcMemoryBackend struct {
	BackendType string `json:"backend_type"`
	BackendPath string `json:"backend_path"`
}

type fcSnapshotLoad struct {
	SnapshotPath  string          `json:"snapshot_path"`
	MemBackend    fcMemoryBackend `json:"mem_backend"`
	ResumeVM      bool            `json:"resume_vm"`
	ClockRealtime bool            `json:"clock_realtime,omitempty"`
}

// fcInstanceInfo is the response of GET /.
type fcInstanceInfo struct {
	AppName    string `json:"app_name"`
	ID         string `json:"id"`
	State      string `json:"state"`
	VMMVersion string `json:"vmm_version"`
}

type fcVersionInfo struct {
	FirecrackerVersion string `json:"firecracker_version"`
}

type fcFault struct {
	FaultMessage string `json:"fault_message"`
}

// newFCClient returns a client for the API socket at socket.
func newFCClient(socket string) *fcClient {
	return &fcClient{
		socket:  socket,
		timeout: fcAPITimeout,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
				MaxIdleConns: 1,
			},
		},
	}
}

// do sends one request. When ctx has no deadline, the client timeout applies.
func (c *fcClient) do(ctx context.Context, method, path string, in, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("firecracker API %v %v: %v", method, path, err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, body)
	if err != nil {
		return fmt.Errorf("firecracker API %v %v: %v", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker API %v %v: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("firecracker API %v %v: read response: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &fcAPIError{Method: method, Path: path, Status: resp.StatusCode}

		var fault fcFault
		if json.Unmarshal(data, &fault) == nil && fault.FaultMessage != "" {
			apiErr.Fault = fault.FaultMessage
		} else {
			apiErr.Fault = strings.TrimSpace(string(data))
		}

		return apiErr
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("firecracker API %v %v: decode response: %w", method, path, err)
		}
	}

	return nil
}

func (c *fcClient) put(path string, in any) error {
	return c.do(context.Background(), http.MethodPut, path, in, nil)
}

func (c *fcClient) PutMachineConfig(vcpus, memMiB int) error {
	return c.put("/machine-config", fcMachineConfig{
		VcpuCount:  vcpus,
		MemSizeMib: memMiB,
	})
}

func (c *fcClient) PutBootSource(kernel, initrd, bootArgs string) error {
	return c.put("/boot-source", fcBootSource{
		KernelImagePath: kernel,
		InitrdPath:      initrd,
		BootArgs:        bootArgs,
	})
}

// PutDrive adds a drive. cacheType is "Unsafe" or "Writeback".
func (c *fcClient) PutDrive(id, path string, isRoot, readOnly bool, cacheType string) error {
	return c.put("/drives/"+url.PathEscape(id), fcDrive{
		DriveID:      id,
		PathOnHost:   path,
		IsRootDevice: isRoot,
		IsReadOnly:   readOnly,
		CacheType:    cacheType,
	})
}

// PutNetworkInterface adds a NIC. An mtu of 0 does not advertise an MTU to
// the guest.
func (c *fcClient) PutNetworkInterface(id, hostDev, guestMAC string, mtu int) error {
	return c.put("/network-interfaces/"+url.PathEscape(id), fcNetworkInterface{
		IfaceID:     id,
		HostDevName: hostDev,
		GuestMAC:    guestMAC,
		MTU:         mtu,
	})
}

func (c *fcClient) PutVsock(cid uint32, udsPath string) error {
	return c.put("/vsock", fcVsock{
		GuestCID: cid,
		UDSPath:  udsPath,
	})
}

func (c *fcClient) PutEntropy() error {
	return c.put("/entropy", fcEntropyDevice{})
}

func (c *fcClient) PutSerial(outPath string) error {
	return c.put("/serial", fcSerialDevice{
		SerialOutPath: outPath,
		RateLimiter: &fcTokenBucket{
			Size:       fcSerialRateBytes,
			RefillTime: fcSerialRefillTime,
		},
	})
}

func (c *fcClient) PutLogger(logPath, level string) error {
	return c.put("/logger", fcLogger{
		LogPath:   logPath,
		Level:     level,
		ShowLevel: true,
	})
}

// Action runs an action: "InstanceStart", "SendCtrlAltDel", or
// "FlushMetrics".
func (c *fcClient) Action(actionType string) error {
	return c.put("/actions", fcInstanceAction{ActionType: actionType})
}

// PatchVMState sets the VM state to "Paused" or "Resumed".
func (c *fcClient) PatchVMState(state string) error {
	return c.do(context.Background(), http.MethodPatch, "/vm", fcVMState{State: state}, nil)
}

// CreateSnapshot writes a full snapshot of a paused VM. The deadline of ctx
// applies, else the client timeout.
func (c *fcClient) CreateSnapshot(ctx context.Context, statePath, memPath string) error {
	return c.do(ctx, http.MethodPut, "/snapshot/create", fcSnapshotCreate{
		SnapshotType: "Full",
		SnapshotPath: statePath,
		MemFilePath:  memPath,
	}, nil)
}

// LoadSnapshot loads a snapshot into a new VMM and leaves the VM paused. The
// deadline of ctx applies, else the client timeout.
func (c *fcClient) LoadSnapshot(ctx context.Context, statePath, memPath string, clockRealtime bool) error {
	return c.do(ctx, http.MethodPut, "/snapshot/load", fcSnapshotLoad{
		SnapshotPath: statePath,
		MemBackend: fcMemoryBackend{
			BackendType: "File",
			BackendPath: memPath,
		},
		ClockRealtime: clockRealtime,
	}, nil)
}

func (c *fcClient) GetInstanceInfo() (fcInstanceInfo, error) {
	var info fcInstanceInfo
	err := c.do(context.Background(), http.MethodGet, "/", nil, &info)
	return info, err
}

func (c *fcClient) GetVersion() (string, error) {
	var v fcVersionInfo
	err := c.do(context.Background(), http.MethodGet, "/version", nil, &v)
	return v.FirecrackerVersion, err
}

// WaitForSocket polls until the API socket answers GET / or ctx ends.
func (c *fcClient) WaitForSocket(ctx context.Context) error {
	backoff := 20 * time.Millisecond

	for {
		attempt, cancel := context.WithTimeout(ctx, c.timeout)
		err := c.do(attempt, http.MethodGet, "/", nil, nil)
		cancel()

		var apiErr *fcAPIError
		if err == nil || errors.As(err, &apiErr) {
			return nil
		}

		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("firecracker API socket %v is not ready: %w (last error: %v)", c.socket, ctx.Err(), err)
		case <-t.C:
		}

		if backoff *= 2; backoff > 200*time.Millisecond {
			backoff = 200 * time.Millisecond
		}
	}
}
