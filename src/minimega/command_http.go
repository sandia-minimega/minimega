// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"context"
	"encoding/json"
	"minicli"
	log "minilog"
	"net/http"
)

var httpServer *http.Server

func commandHttpStart() {
	if *f_httpAddr == "" {
		return
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var request struct {
			Command string `json:"command"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Failed to decode request body", http.StatusBadRequest)
			log.Error("unable to decode request: %v", err)
			return
		}

		var res []minicli.Responses

		cmd, err := minicli.Compile(request.Command)
		if err != nil {
			// invalid command
			resp := &minicli.Response{
				Host:  hostname,
				Error: err.Error(),
			}

			res = append(res, minicli.Responses{resp})
		} else if cmd != nil {
			for resps := range RunCommands(cmd) {
				res = append(res, resps)
			}
		}

		w.Header().Add("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			// shoot
			return
		}
	})

	httpServer = &http.Server{
		Addr:    *f_httpAddr,
		Handler: mux,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				log.Error("http server exited: %v", err)
			}
		}
	}()
}

func commandHttpStop() {
	if httpServer != nil {
		httpServer.Shutdown(context.Background())
	}
}
