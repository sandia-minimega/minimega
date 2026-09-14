// Copyright 2017-2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	emupb "github.com/sandia-minimega/minimega/v2/internal/android/emupb"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"golang.org/x/net/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// resolveAndroidGRPC discovers the emulator gRPC endpoint for an Android VM.
var resolveAndroidGRPC = resolveAndroidGRPCDefault

func resolveAndroidGRPCDefault(r *http.Request, name string) (host string, port int, err error) {
	cmd := NewCommand(r)
	cmd.Command = "vm info"
	cmd.Columns = []string{"host", "type", "android_grpc_port"}
	cmd.Filters = []string{fmt.Sprintf("name=%q", name)}

	for _, vm := range runTabularFunc(cmd) {
		if vm["type"] != "android" {
			return "", 0, fmt.Errorf("VM %s is not an Android VM", name)
		}

		h := vm["host"]
		if h == "" {
			return "", 0, fmt.Errorf("host not available for VM %s", name)
		}

		p := vm["android_grpc_port"]
		if p == "" {
			return "", 0, fmt.Errorf("android_grpc_port not configured for VM %s", name)
		}

		pv, err := strconv.Atoi(p)
		if err != nil || pv <= 0 || pv > 65535 {
			return "", 0, fmt.Errorf("invalid android_grpc_port %q for VM %s", p, name)
		}

		return h, pv, nil
	}

	return "", 0, fmt.Errorf("VM %s not found", name)
}

type inputMsg struct {
	Mouse *mouseInput `json:"mouse,omitempty"`
	Key   *keyInput   `json:"key,omitempty"`
	Touch *touchInput `json:"touch,omitempty"`
	Wheel *wheelInput `json:"wheel,omitempty"`
}

type mouseInput struct {
	X       int32 `json:"x"`
	Y       int32 `json:"y"`
	Buttons int32 `json:"buttons"`
	Display int32 `json:"display"`
}

type keyInput struct {
	Key       string `json:"key"`
	EventType string `json:"eventType"`
}

type touchInput struct {
	Touches []touchPoint `json:"touches"`
	Display int32        `json:"display"`
}

type touchPoint struct {
	X          int32 `json:"x"`
	Y          int32 `json:"y"`
	Identifier int32 `json:"identifier"`
	Pressure   int32 `json:"pressure"`
}

type wheelInput struct {
	Dx      int32 `json:"dx"`
	Dy      int32 `json:"dy"`
	Display int32 `json:"display"`
}

func androidDisplayWsHandler(host string, grpcPort int) func(*websocket.Conn) {
	return func(ws *websocket.Conn) {
		addr := fmt.Sprintf("%v:%v", host, grpcPort)
		log.Info("android display: connecting to emulator gRPC at %v", addr)

		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			log.Error("android display: gRPC dial failed: %v", err)
			return
		}
		defer conn.Close()

		stub := emupb.NewEmulatorControllerClient(conn)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream, err := stub.StreamScreenshot(ctx, &emupb.ImageFormat{
			Format: emupb.ImageFormat_PNG,
		})
		if err != nil {
			log.Error("android display: streamScreenshot failed: %v", err)
			return
		}

		wheelStream, err := stub.InjectWheel(ctx)
		if err != nil {
			log.Warn("android display: injectWheel stream failed: %v", err)
		}

		var sendMu sync.Mutex

		// Stream PNG frames to the browser
		go func() {
			defer cancel()
			for {
				img, err := stream.Recv()
				if err != nil {
					if ctx.Err() == nil {
						log.Error("android display: frame recv error: %v", err)
					}
					return
				}

				data := img.GetImage()
				if len(data) == 0 {
					continue
				}

				sendMu.Lock()
				err = websocket.Message.Send(ws, data)
				sendMu.Unlock()
				if err != nil {
					return
				}
			}
		}()

		log.Info("android display: streaming to ws client from %v", addr)

		// Read input events from the browser
		for {
			var raw string
			if err := websocket.Message.Receive(ws, &raw); err != nil {
				break
			}

			var msg inputMsg
			if err := json.Unmarshal([]byte(raw), &msg); err != nil {
				log.Warn("android display: bad input JSON: %v", err)
				continue
			}

			if msg.Mouse != nil {
				stub.SendMouse(ctx, &emupb.MouseEvent{
					X:       msg.Mouse.X,
					Y:       msg.Mouse.Y,
					Buttons: msg.Mouse.Buttons,
					Display: msg.Mouse.Display,
				})
			}

			if msg.Key != nil {
				ev := &emupb.KeyboardEvent{
					Key: msg.Key.Key,
				}
				switch msg.Key.EventType {
				case "keydown":
					ev.EventType = emupb.KeyboardEvent_keydown
				case "keyup":
					ev.EventType = emupb.KeyboardEvent_keyup
				default:
					ev.EventType = emupb.KeyboardEvent_keypress
				}
				stub.SendKey(ctx, ev)
			}

			if msg.Touch != nil {
				touches := make([]*emupb.Touch, len(msg.Touch.Touches))
				for i, t := range msg.Touch.Touches {
					touches[i] = &emupb.Touch{
						X:          t.X,
						Y:          t.Y,
						Identifier: t.Identifier,
						Pressure:   t.Pressure,
					}
				}
				stub.SendTouch(ctx, &emupb.TouchEvent{
					Touches: touches,
					Display: msg.Touch.Display,
				})
			}

			if msg.Wheel != nil && wheelStream != nil {
				if err := wheelStream.Send(&emupb.WheelEvent{
					Dx:      msg.Wheel.Dx,
					Dy:      msg.Wheel.Dy,
					Display: msg.Wheel.Display,
				}); err != nil {
					log.Warn("android display: wheel send failed: %v", err)
					wheelStream = nil
				}
			}
		}

		if wheelStream != nil {
			wheelStream.CloseAndRecv()
		}

		log.Info("android display: ws client disconnected from %v", addr)
	}
}

// androidStatusGRPCHandler fetches emulator status directly via gRPC.
func androidStatusGRPCHandler(w http.ResponseWriter, r *http.Request, name string) {
	host, port, err := resolveAndroidGRPC(r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		http.Error(w, "gRPC connection failed", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	stub := emupb.NewEmulatorControllerClient(conn)
	status, err := stub.GetStatus(r.Context(), &emptypb.Empty{})
	if err != nil {
		http.Error(w, "emulator status unavailable", http.StatusBadGateway)
		return
	}

	respondJSON(w, map[string]interface{}{
		"version": status.GetVersion(),
		"uptime":  status.GetUptime(),
		"booted":  status.GetBooted(),
		"vmConfig": map[string]interface{}{
			"hypervisorType":   status.GetVmConfig().GetHypervisorType().String(),
			"numberOfCpuCores": status.GetVmConfig().GetNumberOfCpuCores(),
			"ramSizeBytes":     status.GetVmConfig().GetRamSizeBytes(),
		},
		"platformConfig": status.GetPlatformConfig(),
	})
}

// androidRotationGRPCHandler sets the emulator physical rotation via gRPC.
func androidRotationGRPCHandler(w http.ResponseWriter, r *http.Request, name string) {
	host, port, err := resolveAndroidGRPC(r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	var data struct {
		Landscape bool `json:"landscape"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	var zAngle float32
	if data.Landscape {
		zAngle = 90.0
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		http.Error(w, "gRPC connection failed", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	stub := emupb.NewEmulatorControllerClient(conn)
	_, err = stub.SetPhysicalModel(r.Context(), &emupb.PhysicalModelValue{
		Target: emupb.PhysicalModelValue_ROTATION,
		Value:  &emupb.ParameterValue{Data: []float32{0, 0, zAngle}},
	})
	if err != nil {
		http.Error(w, "failed to set rotation", http.StatusBadGateway)
		return
	}

	respondJSON(w, map[string]string{"status": "success"})
}

// androidScreenshotGRPC fetches a single screenshot from the emulator via gRPC.
func androidScreenshotGRPC(r *http.Request, name string, maxSize int) ([]byte, error) {
	host, port, err := resolveAndroidGRPC(r, name)
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("gRPC connection failed: %w", err)
	}
	defer conn.Close()

	imgFmt := &emupb.ImageFormat{Format: emupb.ImageFormat_PNG}
	if maxSize > 0 {
		imgFmt.Width = uint32(maxSize)
		imgFmt.Height = uint32(maxSize)
	}

	stub := emupb.NewEmulatorControllerClient(conn)
	img, err := stub.GetScreenshot(r.Context(), imgFmt)
	if err != nil {
		return nil, fmt.Errorf("getScreenshot failed: %w", err)
	}

	data := img.GetImage()
	if len(data) == 0 {
		return nil, fmt.Errorf("empty screenshot")
	}
	return data, nil
}

// androidGPSGRPCHandler sets emulator GPS coordinates via gRPC.
func androidGPSGRPCHandler(w http.ResponseWriter, r *http.Request, name string) {
	host, port, err := resolveAndroidGRPC(r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	var data struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Altitude  float64 `json:"altitude"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		http.Error(w, "gRPC connection failed", http.StatusBadGateway)
		return
	}
	defer conn.Close()

	stub := emupb.NewEmulatorControllerClient(conn)
	_, err = stub.SetGps(r.Context(), &emupb.GpsState{
		PassiveUpdate: true,
		Latitude:      data.Latitude,
		Longitude:     data.Longitude,
		Altitude:      data.Altitude,
	})
	if err != nil {
		http.Error(w, "failed to set GPS", http.StatusBadGateway)
		return
	}

	respondJSON(w, map[string]string{"status": "success"})
}
