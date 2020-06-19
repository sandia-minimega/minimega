package shell

import (
	"context"
)

var DefaultShell Shell = new(shell)

func CommandExists(cmd string) bool {
	return DefaultShell.CommandExists(cmd)
}

func ExecCommand(ctx context.Context, opts ...Option) ([]byte, []byte, error) {
	return DefaultShell.ExecCommand(ctx, opts...)
}
