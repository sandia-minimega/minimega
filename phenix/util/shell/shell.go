package shell

import (
	"context"
)

type Shell interface {
	CommandExists(string) bool
	ExecCommand(context.Context, ...Option) ([]byte, []byte, error)
}
