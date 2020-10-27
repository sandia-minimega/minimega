package vlan

import (
	"fmt"
	"phenix/api/experiment"
	"phenix/types"
)

// Aliases collects VLAN alias details for all experiments or a given experiment.
// It returns a map of VLAN aliases mapped to experiments and any errors
// encountered while gathering them.
func Aliases(opts ...Option) (map[string]map[string]int, error) {
	var (
		o    = newOptions(opts...)
		exps []types.Experiment
		info = make(map[string]map[string]int)
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
		if exp.Running() {
			info[exp.Metadata.Name] = exp.Status.VLANs()
		} else {
			info[exp.Metadata.Name] = exp.Spec.VLANs().Aliases()
		}
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

	if err := exp.Spec.SetVLANAlias(o.alias, o.id, o.force); err != nil {
		return fmt.Errorf("setting VLAN alias for experiment %s: %w", o.exp, err)
	}

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
		if exp.Running() {
			var (
				min = 0
				max = 0
			)

			for _, k := range exp.Status.VLANs() {
				if min == 0 || k < min {
					min = k
				}

				if max == 0 || k > max {
					max = k
				}
			}

			info[exp.Metadata.Name] = [2]int{min, max}
		} else {
			info[exp.Metadata.Name] = [2]int{exp.Spec.VLANs().Min(), exp.Spec.VLANs().Max()}
		}
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

	if err := exp.Spec.SetVLANRange(o.min, o.max, o.force); err != nil {
		return fmt.Errorf("setting VLAN range for experiment %s: %w", o.exp, err)
	}

	if err := experiment.Save(experiment.SaveWithName(o.exp), experiment.SaveWithSpec(exp.Spec)); err != nil {
		return fmt.Errorf("saving updated spec for experiment %s: %w", o.exp, err)
	}

	return nil
}
