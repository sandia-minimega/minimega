// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// Very simple libreadline binding.
package goreadline

// #cgo LDFLAGS: -lreadline
// #include <stdio.h>
// #include <stdlib.h>
// #include <readline/readline.h>
// #include <readline/history.h>
import "C"

import (
	"errors"
	log "minilog"
	"unsafe"
)

// disable readline's ability to catch signals, as this will cause a panic
// in the go runtime
func init() {
	C.rl_catch_sigwinch = 0
	C.rl_catch_signals = 0
}

// Rlwrap prompts the user with the given prompt string and calls the
// underlying readline function. If the input stream closes, Rlwrap returns an
// EOF error.
func Rlwrap(prompt string) (string, error) {
	p := C.CString(prompt)

	ret := C.readline(p)

	if ret == nil {
		return "", errors.New("EOF")
	}
	C.add_history(ret)
	s := C.GoString(ret)
	C.free(unsafe.Pointer(ret))
	C.free(unsafe.Pointer(p))
	return s, nil
}

// Rlcleanup calls the readline rl_deprep_terminal function, restoring the
// terminal state
func Rlcleanup() {
	log.Info("restoring terminal state from readline")
	C.rl_deprep_terminal()
}
