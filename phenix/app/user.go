package app

import (
	"encoding/json"
	"errors"
	"fmt"

	v1 "phenix/types/version/v1"
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
	exp, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshaling experiment spec to JSON: %w", err)
	}

	exp, err = this.shellOut(ACTIONCONFIG, exp)
	if err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	if err := json.Unmarshal(exp, spec); err != nil {
		return fmt.Errorf("unmarshaling experiment spec from JSON: %w", err)
	}

	return nil
}

func (this UserApp) Start(spec *v1.ExperimentSpec) error {
	exp, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshaling experiment spec to JSON: %w", err)
	}

	exp, err = this.shellOut(ACTIONSTART, exp)
	if err != nil {
		return fmt.Errorf("running user app: %w", err)
	}

	if err := json.Unmarshal(exp, spec); err != nil {
		return fmt.Errorf("unmarshaling experiment spec from JSON: %w", err)
	}

	return nil
}

func (this UserApp) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (this UserApp) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}

func (this UserApp) shellOut(action Action, data []byte) ([]byte, error) {
	// TODO: this is a stub for shelling out to user apps on the command line. The
	// command-line program called should be assumed to have the name
	// `phenix-<appname>`.
	return nil, fmt.Errorf("user app %s: %w", this.options.Name, ErrUserAppNotFound)
}
