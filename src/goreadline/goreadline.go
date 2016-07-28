// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// Very simple libreadline binding.
package goreadline

// #cgo LDFLAGS: -lreadline -ltermcap
// #include <stdio.h>
// #include <stdlib.h>
// #include <readline/readline.h>
// #include <readline/history.h>
//
// extern char** minicomplete(char*, int, int);
//
// extern volatile int abort_getc;
// extern int maybe_getc(FILE*);
// extern int mini_redisplay(void);
import "C"

import (
	"io"
	"minicli"
	log "minilog"
	"unsafe"
)

var (
	completionCandidates []string
	listIndex            int
	FilenameCompleter    func(string) []string // optional filename completer to attempt before the readline builtin
)

// disable readline's ability to catch signals, as this will cause a panic
// in the go runtime
func init() {
	C.rl_catch_sigwinch = 0
	C.rl_catch_signals = 0

	C.rl_attempted_completion_function = (*C.rl_completion_func_t)(C.minicomplete)
	C.rl_getc_function = (*C.rl_getc_func_t)(C.maybe_getc)
}

//export minicomplete
//
// From readline documentation:
// Returns an array of (char *) which is a list of completions for text. If
// there are no completions, returns (char **)NULL. The first entry in the
// returned array is the substitution for text. The remaining entries are the
// possible completions. The array is terminated with a NULL pointer.
func minicomplete(text *C.char, start, end C.int) **C.char {
	// Determine the size of a pointer on the current system
	var b *C.char
	ptrSize := unsafe.Sizeof(b)

	// Copy the buffer containing the line so far
	line := C.GoString(C.rl_line_buffer)

	// Generate suggestions
	var suggest []string
	suggest = minicli.Suggest(line)

	if len(suggest) == 0 {
		// No suggestions.. fall back on default behavior (filename completion)
		if FilenameCompleter == nil {
			return C.rl_completion_matches(text,
				(*C.rl_compentry_func_t)(C.rl_filename_completion_function))
		}

		suggest = FilenameCompleter(line)
		if len(suggest) == 0 {
			// no dice, use the builtin
			return C.rl_completion_matches(text,
				(*C.rl_compentry_func_t)(C.rl_filename_completion_function))
		}
	}
	vals := append([]string{lcp(suggest)}, suggest...)

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
func Readline(prompt string, record bool) (string, error) {
	p := C.CString(prompt)
	defer C.free(unsafe.Pointer(p))

	ret := C.readline(p)
	if ret == nil {
		return "", io.EOF
	}
	defer C.free(unsafe.Pointer(ret))

	if record {
		C.add_history(ret)
	}

	return C.GoString(ret), nil
}

// Signal resets readline after a signal, restoring it to a fresh prompt.
func Signal() {
	// Set event hook to redisplay the screen after the current line is aborted
	// by maybe_getc.
	C.rl_event_hook = (*C.rl_hook_func_t)(C.mini_redisplay)

	C.abort_getc = 1
}

// Rlcleanup calls the readline rl_deprep_terminal function, restoring the
// terminal state
func Rlcleanup() {
	log.Info("restoring terminal state from readline")
	C.rl_deprep_terminal()
}

// a simple longest common prefix function
func lcp(s []string) string {
	var lcp string
	var p int

	if len(s) == 0 {
		return ""
	}

	for {
		var c byte
		for _, v := range s {
			if len(v) <= p {
				return lcp
			}
			if c == 0 {
				c = v[p]
				continue
			}
			if c != v[p] {
				return lcp
			}
		}
		lcp += string(s[0][p])
		p++
	}
}
