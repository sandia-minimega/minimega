// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// This simple server is used to test whether a client properly stores cookies
// or not. If the "id" cookie is not set by the client, the server sets it in
// the response to a random string.

package main

import (
	"encoding/hex"
	"log"
	"math/rand"
	"net/http"
)

// generateID returns a random hex string to use as the client ID.
func generateID() string {
	b := make([]byte, 20)
	n, err := rand.Read(b)
	if err != nil {
		log.Fatalf("unable to generate id: %v", err)
	}

	return hex.EncodeToString(b[:n])
}

// handler for all requests, checks for cookie and sets if not already set
func handler(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("id")
	if err == http.ErrNoCookie {
		id := generateID()

		log.Printf("setting cookie for new client to %v", id)

		http.SetCookie(w, &http.Cookie{
			Name:  "id",
			Value: id,
		})
	} else if err != nil {
		log.Printf("unable to get cookie: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else {
		log.Printf("welcome back to %v", c.Value)
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":80", nil)
}
