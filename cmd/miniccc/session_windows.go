// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unsafe"

	"github.com/sandia-minimega/minimega/v2/internal/ron"

	"golang.org/x/sys/windows"
)

// setSession configures cmd to run in the logged-in user's desktop session.
// The returned function must be called once cmd has been started.
func setSession(cmd *exec.Cmd, session ron.Session) (func(), error) {
	token, err := sessionToken(session)
	if err != nil {
		return nil, err
	}

	env, err := token.Environ(false)
	if err != nil {
		token.Close()
		return nil, fmt.Errorf("getting user environment: %w", err)
	}

	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Token: syscall.Token(token),
		// keep console programs from opening a window on the user's desktop
		CreationFlags: windows.CREATE_NO_WINDOW,
	}

	return func() { token.Close() }, nil
}

// lookPathEnv is exec.LookPath using the PATH in env, or the client's own
// when env is nil.
func lookPathEnv(file string, env []string) (string, error) {
	if env == nil || strings.ContainsAny(file, `:\/`) {
		return exec.LookPath(file)
	}

	var path string
	for _, kv := range env {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PATH") {
			path = v
		}
	}

	for _, dir := range filepath.SplitList(path) {
		// exec.LookPath does not run programs from relative directories
		if !filepath.IsAbs(dir) {
			continue
		}

		if p, err := exec.LookPath(filepath.Join(dir, file)); err == nil {
			return p, nil
		}
	}

	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// sessionToken returns the logged-in user's token for the given session type.
func sessionToken(session ron.Session) (windows.Token, error) {
	if !slices.Contains(ron.ValidSessions, session) {
		return 0, fmt.Errorf("unknown session: %v", session)
	}

	token, err := userToken()
	if err != nil {
		return 0, err
	}

	if session != ron.SessionElevated || token.IsElevated() {
		return token, nil
	}
	defer token.Close()

	// an administrator's desktop runs with a limited token that links to the
	// elevated one
	linked, err := token.GetLinkedToken()
	if errors.Is(err, windows.ERROR_NO_SUCH_LOGON_SESSION) {
		return 0, errors.New("logged-in user is not an administrator")
	} else if err != nil {
		return 0, fmt.Errorf("getting elevated token: %w", err)
	}

	return linked, nil
}

// userToken returns the token of the user logged in on the console, or
// otherwise of the user in the lowest numbered active session.
func userToken() (windows.Token, error) {
	var token windows.Token

	err := windows.WTSQueryUserToken(windows.WTSGetActiveConsoleSessionId(), &token)
	if err == nil {
		return token, nil
	}
	if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
		return 0, errors.New("miniccc must run as LocalSystem to use user sessions")
	}

	var (
		sessions *windows.WTS_SESSION_INFO
		count    uint32
	)

	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count); err != nil {
		return 0, fmt.Errorf("enumerating sessions: %w", err)
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))

	var ids []uint32
	for _, s := range unsafe.Slice(sessions, count) {
		if s.State == windows.WTSActive {
			ids = append(ids, s.SessionID)
		}
	}
	slices.Sort(ids)

	// a session can briefly lack a token while its user logs on or off, so
	// only fail once every active session has been tried
	err = errors.New("no user is logged in")
	for _, id := range ids {
		qerr := windows.WTSQueryUserToken(id, &token)
		if qerr == nil {
			return token, nil
		}

		err = fmt.Errorf("getting token for session %v: %w", id, qerr)
	}

	return 0, err
}
