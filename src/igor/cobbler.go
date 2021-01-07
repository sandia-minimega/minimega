// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bufio"
	"fmt"
	log "minilog"
	"path/filepath"
	"strings"
)

type CobblerBackend struct {
	profiles map[string]bool
	distros  map[string]bool
}

func NewCobblerBackend() Backend {
	return &CobblerBackend{
		profiles: CobblerProfiles(),
		distros:  CobblerDistros(),
	}
}

// Install configures Cobbler to boot the correct stuff
func (b *CobblerBackend) Install(r *Reservation) error {
	// If we're using a kernel+ramdisk instead of an existing profile, create a
	// profile and set the nodes to boot from it
	if r.CobblerProfile == "" {
		profile := "igor_" + r.Name

		// Try to clean up any leftover profile/distro with this name. Will
		// be a no-op if there are no conflicts.
		if err := b.removeProfile(profile); err != nil {
			return err
		}

		// Create the distro from the kernel+ramdisk
		_, err := processWrapper("cobbler", "distro", "add", "--name="+profile, "--kernel="+filepath.Join(igor.TFTPRoot, "igor", r.KernelHash+"-kernel"), "--initrd="+filepath.Join(igor.TFTPRoot, "igor", r.InitrdHash+"-initrd"), "--kopts="+r.KernelArgs)
		if err != nil {
			return err
		}

		// Create a profile from the distro we just made
		_, err = processWrapper("cobbler", "profile", "add", "--name="+profile, "--distro="+profile)
		if err != nil {
			return err
		}

		// Now set each host to boot from that profile
		if err := b.setProfile(r.Hosts, profile); err != nil {
			return err
		}

		return b.enableNetboot(r.Hosts)
	}

	// install profile by name
	if !b.profiles[r.CobblerProfile] {
		return fmt.Errorf("cobbler profile does not exist: %v", r.CobblerProfile)
	}

	// If the requested profile exists, go ahead and set the nodes to use it
	if err := b.setProfile(r.Hosts, r.CobblerProfile); err != nil {
		return err
	}

	return b.enableNetboot(r.Hosts)
}

func (b *CobblerBackend) Uninstall(r *Reservation) error {
	// Set all nodes in the reservation back to the default profile
	if err := b.setProfile(r.Hosts, igor.CobblerDefaultProfile); err != nil {
		return err
	}

	// Delete the profile and distro we created for this reservation
	if r.CobblerProfile == "" {
		return b.removeProfile("igor_" + r.Name)
	}

	return nil
}

// setProfile sets the cobbler profile for all hosts to the specified profile
func (b *CobblerBackend) setProfile(hosts []string, profile string) error {
	runner := DefaultRunner(func(host string) error {
		_, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--profile="+profile)
		return err
	})

	if err := runner.RunAll(hosts); err != nil {
		return fmt.Errorf("unable to set cobbler profile: %v", err)
	}

	return nil
}

// enableNetboot enables netboot for all hosts
func (b *CobblerBackend) enableNetboot(hosts []string) error {
	runner := DefaultRunner(func(host string) error {
		_, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--netboot-enabled=true")
		return err
	})

	if err := runner.RunAll(hosts); err != nil {
		return fmt.Errorf("unable to enable netboot: %v", err)
	}

	return nil
}

func (b *CobblerBackend) removeProfile(profile string) error {
	log.Info("removing profile: %v", profile)

	var err error
	var hosts []string

	// find list of hosts that are using this profile and reset them to the
	// default. This list should be empty if igor wasn't interrupted
	// mid-install.
	for host := range CobblerSystems(profile) {
		hosts = append(hosts, host)
	}

	if len(hosts) > 0 {
		log.Info("setting hosts to default profile: %v", hosts)

		if err := b.setProfile(hosts, igor.CobblerDefaultProfile); err != nil {
			return err
		}
	}

	// delete the profile, if it exists
	if b.profiles[profile] {
		_, err = processWrapper("cobbler", "profile", "remove", "--name="+profile)
		if err == nil {
			delete(b.profiles, profile)
		}
	}

	// delete the distro, if it exists
	if err == nil && b.distros[profile] {
		_, err = processWrapper("cobbler", "distro", "remove", "--name="+profile)
		if err == nil {
			delete(b.distros, profile)
		}
	}

	return err
}

func CobblerProfiles() map[string]bool {
	return cobblerList("cobbler", "profile", "list")
}

func CobblerDistros() map[string]bool {
	return cobblerList("cobbler", "distro", "list")
}

func CobblerSystems(profile string) map[string]bool {
	return cobblerList("cobbler", "system", "find", "--profile", profile)
}

func cobblerList(args ...string) map[string]bool {
	res := map[string]bool{}

	// Get a list of current profiles
	out, err := processWrapper(args...)
	if err != nil {
		log.Fatal("unable to get list from cobbler: %v\n", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		res[strings.TrimSpace(scanner.Text())] = true
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("unable to read cobbler list: %v", err)
	}

	return res
}
