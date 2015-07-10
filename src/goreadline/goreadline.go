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
// extern char** minicli_completion(char* text, int start, int end);
import "C"

import (
	"errors"
	"minicli"
	log "minilog"
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
	C.rl_attempted_completion_function = (*C.rl_completion_func_t)(C.minicli_completion)
}

//export minicliCompletion
//
// From readline documentation:
// Returns an array of (char *) which is a list of completions for text. If
// there are no completions, returns (char **)NULL. The first entry in the
// returned array is the substitution for text. The remaining entries are the
// possible completions. The array is terminated with a NULL pointer.
func minicliCompletion(text *C.char, start, end C.int) **C.char {
	// Determine the size of a pointer on the current system
	var b *C.char
	ptrSize := unsafe.Sizeof(b)

	// Copy the buffer containing the line so far
	line := C.GoString(C.rl_line_buffer)

	// Default is to not change the string
	vals := []string{C.GoString(text)}

	// Generate suggestions
	suggest := minicli.Suggest(line)

	if len(suggest) == 1 {
		// Use only suggestion as substitution for text
		vals[0] = suggest[0]
	} else if len(suggest) == 0 {
		// No suggestions.. fall back on default behavior (filename completion)
		return C.rl_completion_matches(text,
			(*C.rl_compentry_func_t)(C.rl_filename_completion_function))
	}
	vals = append(vals, suggest...)

	// Copy suggestions into char**
	ptr := C.malloc(C.size_t(len(vals)+1) * C.size_t(ptrSize))
	for i, v := range vals {
		element := (**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*ptrSize))
		*element = C.CString(v)
	}
	element := (**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(len(vals))*ptrSize))
	*element = nil

	return (**C.char)(ptr)
}

// Rlwrap prompts the user with the given prompt string and calls the
// underlying readline function. If the input stream closes, Rlwrap returns an
// EOF error.
func Rlwrap(prompt string, record bool) (string, error) {
	p := C.CString(prompt)
	defer C.free(unsafe.Pointer(p))

	ret := C.readline(p)
	if ret == nil {
		return "", errors.New("EOF")
	}
	defer C.free(unsafe.Pointer(ret))

	if record {
		C.add_history(ret)
	}

	return C.GoString(ret), nil
}

// Rlcleanup calls the readline rl_deprep_terminal function, restoring the
// terminal state
func Rlcleanup() {
	log.Info("restoring terminal state from readline")
	C.rl_deprep_terminal()
}
