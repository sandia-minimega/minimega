package vlan

import (
	"fmt"
	"phenix/api/experiment"
	"phenix/types"
	v1 "phenix/types/version/v1"
)

// Aliases collects VLAN alias details for all experiments or a given experiment.
// It returns a map of VLAN aliases mapped to experiments and any errors
// encountered while gathering them.
func Aliases(opts ...Option) (map[string]v1.VLANAliases, error) {
	var (
		o    = newOptions(opts...)
		exps []types.Experiment
		info = make(map[string]v1.VLANAliases)
	)

	if o.exp == "" {
		var err error

		exps, err = experiment.List()
		if err != nil {
			return nil, fmt.Errorf("getting list of experiments: %w", err)
		}
	} else {
		exp, err := experiment.Get(o.exp)
		if err != nil {
			return nil, fmt.Errorf("getting experiment %s: %w", o.exp, err)
		}

		exps = []types.Experiment{*exp}
	}

	for _, exp := range exps {
		info[exp.Metadata.Name] = exp.Spec.VLANs.Aliases
	}

	return info, nil
}

// SetAlias sets a VLAN alias for the given experiment as the given VLAN ID.
func SetAlias(opts ...Option) error {
	o := newOptions(opts...)

	if o.exp == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if o.alias == "" {
		return fmt.Errorf("no VLAN alias provided")
	}

	if o.id == 0 {
		return fmt.Errorf("no VLAN ID provided")
	}

	exp, err := experiment.Get(o.exp)
	if err != nil {
		return fmt.Errorf("getting experiment %s: %w", o.exp, err)
	}

	_, ok := exp.Spec.VLANs.Aliases[o.alias]
	if ok && !o.force {
		return fmt.Errorf("VLAN alias %s already exists for experiment %s", o.alias, o.exp)
	}

	if exp.Spec.VLANs.Min != 0 && o.id < exp.Spec.VLANs.Min {
		return fmt.Errorf("VLAN ID %d is less than experiment min VLAN ID of %d", o.id, exp.Spec.VLANs.Min)
	}

	if exp.Spec.VLANs.Max != 0 && o.id > exp.Spec.VLANs.Max {
		return fmt.Errorf("VLAN ID %d is greater than experiment max VLAN ID of %d", o.id, exp.Spec.VLANs.Max)
	}

	exp.Spec.VLANs.Aliases[o.alias] = o.id

	if err := experiment.Save(experiment.SaveWithName(o.exp), experiment.SaveWithSpec(exp.Spec)); err != nil {
		return fmt.Errorf("saving updated spec for experiment %s: %w", o.exp, err)
	}

	return nil
}

// Ranges collects VLAN range details for all experiments or a given experiment.
// It returns a map of VLAN ranges mapped to experiments and any errors
// encountered while gathering them.
func Ranges(opts ...Option) (map[string][2]int, error) {
	var (
		o    = newOptions(opts...)
		exps []types.Experiment
		info = make(map[string][2]int)
	)

	if o.exp == "" {
		var err error

		exps, err = experiment.List()
		if err != nil {
			return nil, fmt.Errorf("getting list of experiments: %w", err)
		}
	} else {
		exp, err := experiment.Get(o.exp)
		if err != nil {
			return nil, fmt.Errorf("getting experiment %s: %w", o.exp, err)
		}

		exps = []types.Experiment{*exp}
	}

	for _, exp := range exps {
		info[exp.Metadata.Name] = [2]int{exp.Spec.VLANs.Min, exp.Spec.VLANs.Max}
	}

	return info, nil
}

// SetRange sets the VLAN range for the given experiment. It returns an error if
// the given range is not in ascending order (ie. if min > max).
func SetRange(opts ...Option) error {
	o := newOptions(opts...)

	if o.exp == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if o.min == 0 {
		return fmt.Errorf("no VLAN min ID provided")
	}

	if o.max == 0 {
		return fmt.Errorf("no VLAN max ID provided")
	}

	if o.min > o.max {
		return fmt.Errorf("min VLAN ID must not be greater than max VLAN ID")
	}

	exp, err := experiment.Get(o.exp)
	if err != nil {
		return fmt.Errorf("getting experiment %s: %w", o.exp, err)
	}

	min := exp.Spec.VLANs.Min
	max := exp.Spec.VLANs.Max

	if min != 0 && max != 0 && !o.force {
		return fmt.Errorf("VLAN range %d-%d already exists for experiment %s", min, max, o.exp)
	}

	for k, v := range exp.Spec.VLANs.Aliases {
		if o.min != 0 && v < o.min {
			return fmt.Errorf("topology VLAN %s (VLAN ID %d) is less than proposed experiment min VLAN ID of %d", k, v, o.min)
		}

		if o.max != 0 && v > o.max {
			return fmt.Errorf("topology VLAN %s (VLAN ID %d) is greater than proposed experiment min VLAN ID of %d", k, v, o.max)
		}
	}

	exp.Spec.VLANs.Min = o.min
	exp.Spec.VLANs.Max = o.max

	if err := experiment.Save(experiment.SaveWithName(o.exp), experiment.SaveWithSpec(exp.Spec)); err != nil {
		return fmt.Errorf("saving updated spec for experiment %s: %w", o.exp, err)
	}

	return nil
}
