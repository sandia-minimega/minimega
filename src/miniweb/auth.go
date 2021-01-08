// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"net/http"
	"os"
	"strings"

	"github.com/peterh/liner"
	"golang.org/x/crypto/bcrypt"
)

type PasswordEntry struct {
	Path     string `json:"path"`
	Username string `json:"username"`
	Password []byte `json:"password"`
}

var passwords = []PasswordEntry{}

func (p PasswordEntry) Match(username, password string) bool {
	if username != p.Username {
		return false
	}

	err := bcrypt.CompareHashAndPassword(p.Password, []byte(password))
	return err == nil
}

func savePasswords(fname string) error {
	f, err := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "\t")
	return enc.Encode(&passwords)
}

func parsePasswords(fname string) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewDecoder(f).Decode(&passwords)
}

func mustAuth(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// check if URL is password protected
		var matches []PasswordEntry
		for _, entry := range passwords {
			if strings.HasPrefix(r.URL.Path, entry.Path) {
				matches = append(matches, entry)
			}
		}

		// no matches -- must not require auth
		if len(matches) == 0 {
			f(w, r)
			return
		}

		// test all the matches and call f if any match credentials
		username, password, ok := r.BasicAuth()
		if ok {
			for _, match := range matches {
				if match.Match(username, password) {
					f(w, r)
					return
				}
			}
		}

		// all matches failed
		w.Header().Set("WWW-Authenticate", `Basic realm="minimega"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

func bootstrap(fname string) error {
	input := liner.NewLiner()
	defer input.Close()

	first := true

	for {
		path := "/"
		if first {
			fmt.Println("Configure /")

			first = false
		} else {
			fmt.Println()
			fmt.Println("Add additional users (Ctrl-D when finished):")

			path2, err := input.Prompt("Path: ")
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			path = path2
		}

		username, err := input.Prompt("Username: ")
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if username == "" {
			return errors.New("invalid username, must not be empty")
		}

		password, err := input.PasswordPrompt("Password: ")
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		password2, err := input.PasswordPrompt("Confirm Password: ")
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if password == "" {
			return errors.New("invalid password, must not be empty")
		}
		if password != password2 {
			return errors.New("passwords do not match")
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		passwords = append(passwords, PasswordEntry{
			Username: username,
			Password: hashed,
			Path:     path,
		})
	}

	// user Ctrl-D -- start new line
	fmt.Println()

	log.Info("bootstrap complete, saving...")

	if len(passwords) == 0 {
		return errors.New("no passwords recorded")
	}

	return savePasswords(fname)
}
