// Copyright 2017-2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	emupb "github.com/sandia-minimega/minimega/v2/internal/android/emupb"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeEmulatorServer implements the GetStatus RPC for testing.
type fakeEmulatorServer struct {
	emupb.UnimplementedEmulatorControllerServer
}

func (s *fakeEmulatorServer) GetStatus(_ context.Context, _ *emptypb.Empty) (*emupb.EmulatorStatus, error) {
	return &emupb.EmulatorStatus{
		Version: "fake-grpc",
		Uptime:  123,
		Booted:  true,
		PlatformConfig: map[string]string{
			"hw.lcd.width":  "1080",
			"hw.lcd.height": "1920",
		},
	}, nil
}

func (s *fakeEmulatorServer) SetPhysicalModel(_ context.Context, _ *emupb.PhysicalModelValue) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *fakeEmulatorServer) SetGps(_ context.Context, _ *emupb.GpsState) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (s *fakeEmulatorServer) GetScreenshot(_ context.Context, _ *emupb.ImageFormat) (*emupb.Image, error) {
	return &emupb.Image{Image: []byte("fake-png-data")}, nil
}

func setupFakeGRPC(t *testing.T) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	emupb.RegisterEmulatorControllerServer(grpcServer, &fakeEmulatorServer{})
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	host, portStr, _ := net.SplitHostPort(lis.Addr().String())
	port, _ := strconv.Atoi(portStr)

	oldGRPC := resolveAndroidGRPC
	t.Cleanup(func() { resolveAndroidGRPC = oldGRPC })
	resolveAndroidGRPC = func(r *http.Request, name string) (string, int, error) {
		return host, port, nil
	}

	oldRunTabular := runTabularFunc
	t.Cleanup(func() { runTabularFunc = oldRunTabular })
	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"type": "android"}}
	}
}

// TestVmHandlerAndroidDispatch tests the complete flow through vmHandler
// using a fake gRPC server (status uses gRPC directly).
func TestVmHandlerAndroidDispatch(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("GET", "/vm/android1/android/api/v1/emulator/status", nil)
	w := httptest.NewRecorder()

	vmHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body=%q", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["version"] != "fake-grpc" {
		t.Errorf("got version %v, want fake-grpc", resp["version"])
	}
}

// TestConnectHandlerAndroidServesAndroidHTML tests Android connect route
func TestConnectHandlerAndroidServesAndroidHTML(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"host": "test-host", "type": "android", "vnc_port": "", "console_port": ""}}
	}

	oldRoot := *f_root
	defer func() { *f_root = oldRoot }()

	tmp := t.TempDir()
	*f_root = tmp

	if err := os.WriteFile(filepath.Join(tmp, "android.html"), []byte("android page"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/vm/android1/connect/", nil)
	w := httptest.NewRecorder()

	connectHandler(w, req, "android1")

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body=%q", w.Code, http.StatusOK, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "android page") {
		t.Fatalf("expected android page body, got %q", w.Body.String())
	}
}

// TestConnectHandlerAndroidWebSocketReturns404 tests Android /connect/ws
func TestConnectHandlerAndroidWebSocketReturns404(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"host": "test-host", "type": "android"}}
	}

	req := httptest.NewRequest("GET", "/vm/android1/connect/ws", nil)
	w := httptest.NewRecorder()

	connectHandler(w, req, "android1")

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestAndroidHTMLContract tests that android.html contains required elements
func TestAndroidHTMLContract(t *testing.T) {
	paths := []string{
		"../../web/android.html",
		"../web/android.html",
		"web/android.html",
	}

	var htmlPath string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			htmlPath = p
			break
		}
	}

	if htmlPath == "" {
		t.Skip("android.html not found, skipping contract test")
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read android.html: %v", err)
	}

	htmlContent := string(content)

	if !strings.Contains(htmlContent, "/js/android-display.js") {
		t.Error("android.html missing required script: /js/android-display.js")
	}

	if !strings.Contains(htmlContent, "/css/minimega.css") {
		t.Error("android.html missing required stylesheet: /css/minimega.css")
	}

	requiredIDs := []string{
		"android-display",
		"display-status",
		"android-input-surface",
		"hw-btn-home",
		"hw-btn-landscape",
	}

	for _, id := range requiredIDs {
		if !strings.Contains(htmlContent, `id="`+id+`"`) {
			t.Errorf("android.html missing required element with id=%s", id)
		}
	}

	if strings.Contains(htmlContent, "onclick=") {
		t.Error("android.html should not contain inline onclick handlers")
	}
}

// TestAndroidHandlerGPSWrongMethod tests that GPS endpoint returns 405 for non-POST methods
func TestAndroidHandlerGPSWrongMethod(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"type": "android"}}
	}

	req := httptest.NewRequest("GET", "/vm/android1/android/api/v1/emulator/gps", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "gps"})

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GPS wrong method: got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}

	if w.Header().Get("Allow") != http.MethodPost {
		t.Errorf("GPS wrong method: got Allow %v, want %v", w.Header().Get("Allow"), http.MethodPost)
	}
}

// TestAndroidHandlerNonAndroidVM tests that non-Android VMs return 404
func TestAndroidHandlerNonAndroidVM(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"type": "kvm"}}
	}

	req := httptest.NewRequest("GET", "/vm/nonandroid/android/api/v1/emulator/status", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "nonandroid", []string{"api", "v1", "emulator", "status"})

	if w.Code != http.StatusNotFound {
		t.Errorf("Non-Android VM: got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestAndroidHandlerInvalidVM tests that invalid VM names return 404
func TestAndroidHandlerInvalidVM(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{}
	}

	req := httptest.NewRequest("GET", "/vm/invalid/android/api/v1/emulator/status", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "invalid", []string{"api", "v1", "emulator", "status"})

	if w.Code != http.StatusNotFound {
		t.Errorf("Invalid VM: got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestConnectHandlerUnknownVMTypeReturns404 tests unknown VM type handling
func TestConnectHandlerUnknownVMTypeReturns404(t *testing.T) {
	oldRunTabularFunc := runTabularFunc
	defer func() { runTabularFunc = oldRunTabularFunc }()

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{
			"host": "test-host",
			"type": "weird",
		}}
	}

	req := httptest.NewRequest("GET", "/vm/weird/connect/", nil)
	w := httptest.NewRecorder()

	connectHandler(w, req, "weird")

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAndroidRotationHandler(t *testing.T) {
	setupFakeGRPC(t)

	body := strings.NewReader(`{"landscape":true}`)
	req := httptest.NewRequest("POST", "/vm/android1/android/api/v1/emulator/rotation", body)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "rotation"})

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body=%q", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Fatalf("expected success in response, got %q", w.Body.String())
	}
}

func TestAndroidRotationHandlerWrongMethod(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("GET", "/vm/android1/android/api/v1/emulator/rotation", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "rotation"})

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if w.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("got Allow %q, want %q", w.Header().Get("Allow"), http.MethodPost)
	}
}

func TestAndroidRotationHandlerBadBody(t *testing.T) {
	setupFakeGRPC(t)

	body := strings.NewReader(`{invalid`)
	req := httptest.NewRequest("POST", "/vm/android1/android/api/v1/emulator/rotation", body)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "rotation"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAndroidGPSHandlerHappyPath(t *testing.T) {
	setupFakeGRPC(t)

	body := strings.NewReader(`{"latitude":37.7749,"longitude":-122.4194,"altitude":0}`)
	req := httptest.NewRequest("POST", "/vm/android1/android/api/v1/emulator/gps", body)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "gps"})

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d; body=%q", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Fatalf("expected success in response, got %q", w.Body.String())
	}
}

func TestAndroidGPSHandlerBadBody(t *testing.T) {
	setupFakeGRPC(t)

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("POST", "/vm/android1/android/api/v1/emulator/gps", body)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "gps"})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAndroidScreenshotGRPC(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("GET", "/", nil)
	data, err := androidScreenshotGRPC(req, "android1", 300)
	if err != nil {
		t.Fatalf("androidScreenshotGRPC() error: %v", err)
	}
	if string(data) != "fake-png-data" {
		t.Fatalf("got %q, want %q", string(data), "fake-png-data")
	}
}

func TestAndroidStatusWrongMethod(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("POST", "/vm/android1/android/api/v1/emulator/status", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "status"})

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if w.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("got Allow %q, want %q", w.Header().Get("Allow"), http.MethodGet)
	}
}

func TestAndroidHandlerUnknownRoute(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("GET", "/vm/android1/android/api/v1/emulator/nonexistent", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{"api", "v1", "emulator", "nonexistent"})

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAndroidHandlerEmptyFields(t *testing.T) {
	setupFakeGRPC(t)

	req := httptest.NewRequest("GET", "/vm/android1/android/", nil)
	w := httptest.NewRecorder()
	androidHandler(w, req, "android1", []string{})

	if w.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}
