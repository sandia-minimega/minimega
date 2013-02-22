// readline binding
package goreadline

// #cgo LDFLAGS: -lreadline
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

// the readline call proper, called by the cli
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

// make readline restore the terminal state before we exit, which will allow
// us to reclaim our terminal
func Rlcleanup() {
	log.Info("restoring terminal state from readline")
	C.rl_deprep_terminal()
}
