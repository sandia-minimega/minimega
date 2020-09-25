package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"phenix/internal/common"
	"phenix/scheduler"
	"phenix/types"
	"phenix/util/shell"
)

const (
	EXIT_SCHEDULE int = 300
)

var ErrUserAppNotFound = errors.New("user app not found")

type UserApp struct {
	options Options
}

func (this *UserApp) Init(opts ...Option) error {
	this.options = NewOptions(opts...)

	return nil
}

func (this UserApp) Name() string {
	return this.options.Name
}

func (this UserApp) Configure(exp *types.Experiment) error {
	if err := this.shellOut(ACTIONCONFIG, exp); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) PreStart(exp *types.Experiment) error {
	if err := this.shellOut(ACTIONPRESTART, exp); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) PostStart(exp *types.Experiment) error {
	if err := this.shellOut(ACTIONPOSTSTART, exp); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) Cleanup(exp *types.Experiment) error {
	if err := this.shellOut(ACTIONCLEANUP, exp); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) shellOut(action Action, exp *types.Experiment) error {
	cmdName := "phenix-app-" + this.options.Name

	if !shell.CommandExists(cmdName) {
		return fmt.Errorf("external user app %s does not exist in your path: %w", cmdName, ErrUserAppNotFound)
	}

	if action == ACTIONPOSTSTART || action == ACTIONCLEANUP {
		// Ensure status.apps exists for use in user apps.
		if exp.Status.Apps == nil {
			exp.Status.Apps = make(map[string]interface{})
		}
	}

	data, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("marshaling experiment to JSON: %w", err)
	}

	opts := []shell.Option{
		shell.Command(cmdName),
		shell.Args(string(action)),
		shell.Stdin(data),
		shell.Env( // TODO: update to reflect options provided by user
			"PHENIX_LOG_LEVEL=DEBUG",
			"PHENIX_LOG_FILE=/tmp/phenix-apps.log",
			"PHENIX_DIR="+common.PhenixBase,
		),
	}

	stdOut, stdErr, err := shell.ExecCommand(context.Background(), opts...)
	if err != nil {
		var exitErr *exec.ExitError

		// The user app returned a non-zero exit status, so see if it matches any of
		// our special exit codes and handle accordingly.
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case EXIT_SCHEDULE:
				sched := string(stdOut)

				if err := scheduler.Schedule(sched, exp.Spec); err != nil {
					return fmt.Errorf("scheduling experiment with %s: %w", sched, err)
				}

				return this.shellOut(action, exp)
			}
		}

		// FIXME: improve on this
		fmt.Printf(string(stdErr))

		return fmt.Errorf("user app %s command %s failed: %w", this.options.Name, cmdName, err)
	}

	var result types.Experiment

	if err := json.Unmarshal(stdOut, &result); err != nil {
		return fmt.Errorf("unmarshaling experiment from JSON: %w", err)
	}

	switch action {
	case ACTIONCONFIG, ACTIONPRESTART:
		exp.Spec = result.Spec
	case ACTIONPOSTSTART, ACTIONCLEANUP:
		if metadata, ok := result.Status.Apps[this.options.Name]; ok {
			exp.Status.Apps[this.options.Name] = metadata
		}
	}

	return nil
}
