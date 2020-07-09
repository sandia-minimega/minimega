package shell

import (
	"context"
)

type Shell interface {
	FindCommandsWithPrefix(string) []string
	CommandExists(string) bool
	ExecCommand(context.Context, ...Option) ([]byte, []byte, error)
}
