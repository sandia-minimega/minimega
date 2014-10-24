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
// extern char** minimega_completion(char* text, int start, int end);
// extern char** make_string_array(int len);
// extern void set_string_array(char** a, char* s, int i);
import "C"

import (
	"errors"
	log "minilog"
	"strings"
	"unsafe"
)

var (
	completionCandidates []string
	listIndex            int
)

// disable readline's ability to catch signals, as this will cause a panic
// in the go runtime
func init() {
	C.rl_catch_sigwinch = 0
	C.rl_catch_signals = 0
	C.rl_attempted_completion_function = (*C.rl_completion_func_t)(C.minimega_completion)
}

func SetCompletionCandidates(c []string) {
	completionCandidates = c
}

//export minimegaCompletion
func minimegaCompletion(text *C.char, state int) *C.char {
	t := C.GoString(text)

	if state == 0 {
		listIndex = 0
	}

	if len(completionCandidates) == 0 {
		return nil
	}

	if listIndex >= len(completionCandidates) {
		return nil
	}

	// find a match, using listIndex as our current index
	for listIndex < len(completionCandidates) {
		m := completionCandidates[listIndex]
		listIndex++
		if strings.HasPrefix(m, t) {
			return C.CString(m)
		}
	}

	return nil
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
