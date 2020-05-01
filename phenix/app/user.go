package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	v1 "phenix/types/version/v1"
	"phenix/util"
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

	if !util.ShellCommandExists(cmdName) {
		return fmt.Errorf("external user app %s does not exist in your path: %w", cmdName, ErrUserAppNotFound)
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshaling experiment spec to JSON: %w", err)
	}

	var (
		stdOut bytes.Buffer
		stdErr bytes.Buffer
	)

	cmd := exec.Command(cmdName, string(action))
	cmd.Stdin = bytes.NewBuffer(data)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		// FIXME: improve on this
		fmt.Printf(string(stdErr.Bytes()))

		return fmt.Errorf("user app %s command %s failed: %w", this.options.Name, cmdName, err)
	}

	if err := json.Unmarshal(stdOut.Bytes(), spec); err != nil {
		return fmt.Errorf("unmarshaling experiment spec from JSON: %w", err)
	}

	return nil
}
