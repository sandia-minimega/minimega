package shell

import (
	"bytes"
	context "context"
	"io"
	"os"
	"os/exec"
)

type shell struct{}

func (shell) CommandExists(cmd string) bool {
	err := exec.Command("which", cmd).Run()
	return err == nil
}

func (shell) ExecCommand(ctx context.Context, opts ...Option) ([]byte, []byte, error) {
	o := newOptions(opts...)

	var (
		stdIn  io.Reader
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)

	if o.stdin == nil {
		stdIn = os.Stdin
	} else {
		stdIn = bytes.NewBuffer(o.stdin)
	}

	cmd := exec.CommandContext(ctx, o.cmd, o.args...)
	cmd.Stdin = stdIn
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	err := cmd.Run()

	return stdOut.Bytes(), stdErr.Bytes(), err
}
