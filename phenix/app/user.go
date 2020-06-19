package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "phenix/types/version/v1"
	"phenix/util/shell"
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

func (this UserApp) Configure(spec *v1.ExperimentSpec) error {
	if err := this.shellOut(ACTIONCONFIG, spec); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) Start(spec *v1.ExperimentSpec) error {
	if err := this.shellOut(ACTIONSTART, spec); err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	return nil
}

func (this UserApp) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (this UserApp) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}

func (this UserApp) shellOut(action Action, spec *v1.ExperimentSpec) error {
	cmdName := "phenix-" + this.options.Name

	if !shell.CommandExists(cmdName) {
		return fmt.Errorf("external user app %s does not exist in your path: %w", cmdName, ErrUserAppNotFound)
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshaling experiment spec to JSON: %w", err)
	}

	/*
		var (
			stdOut bytes.Buffer
			stdErr bytes.Buffer
		)
	*/

	opts := []shell.Option{
		shell.Command(cmdName),
		shell.Args(string(action)),
		shell.Stdin(data),
		/*
			shell.Stdin(bytes.NewBuffer(data)),
			shell.Stdout(&stdOut),
			shell.Stderr(&stdErr),
		*/
	}

	// if err := shell.ExecCommand(context.Background(), opts...); err != nil {

	stdOut, stdErr, err := shell.ExecCommand(context.Background(), opts...)
	if err != nil {
		// FIXME: improve on this
		fmt.Printf(string(stdErr))

		return fmt.Errorf("user app %s command %s failed: %w", this.options.Name, cmdName, err)
	}

	if err := json.Unmarshal(stdOut, spec); err != nil {
		return fmt.Errorf("unmarshaling experiment spec from JSON: %w", err)
	}

	return nil
}
