package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "phenix/types/version/v1"
	"phenix/util/shell"
)

var ErrUserSchedulerNotFound = errors.New("user scheduler not found")

type userScheduler struct {
	options Options
}

func (this *userScheduler) Init(opts ...Option) error {
	this.options = NewOptions(opts...)

	return nil
}

func (this userScheduler) Name() string {
	return this.options.Name
}

func (this userScheduler) Schedule(spec *v1.ExperimentSpec) error {
	if err := this.shellOut(spec); err != nil {
		return fmt.Errorf("running user scheduler: %w", err)
	}

	return nil
}

func (this userScheduler) shellOut(spec *v1.ExperimentSpec) error {
	cmdName := "phenix-scheduler-" + this.options.Name

	if !shell.CommandExists(cmdName) {
		return fmt.Errorf("external user scheduler %s does not exist in your path: %w", cmdName, ErrUserSchedulerNotFound)
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshaling experiment spec to JSON: %w", err)
	}

	opts := []shell.Option{
		shell.Command(cmdName),
		shell.Stdin(data),
	}

	stdOut, stdErr, err := shell.ExecCommand(context.Background(), opts...)
	if err != nil {
		// FIXME: improve on this
		fmt.Printf(string(stdErr))

		return fmt.Errorf("user scheduler %s command %s failed: %w", this.options.Name, cmdName, err)
	}

	if err := json.Unmarshal(stdOut, spec); err != nil {
		return fmt.Errorf("unmarshaling experiment spec from JSON: %w", err)
	}

	return nil
}
