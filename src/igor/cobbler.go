// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	log "minilog"
	"os"
	"path/filepath"
	"strings"
)

type CobblerBackend struct {
	profiles map[string]bool
}

func NewCobblerBackend() Backend {
	return &CobblerBackend{
		profiles: CobblerProfiles(),
	}
}

// Install configures Cobbler to boot the correct stuff
func (b *CobblerBackend) Install(r Reservation) error {
	// If we're using a kernel+ramdisk instead of an existing profile, create a
	// profile and set the nodes to boot from it
	if r.CobblerProfile == "" {
		profile := "igor_" + r.ResName

		if b.profiles[profile] {
			// That's strange... a cobbler profile already exists. Perhaps we
			// didn't fully clean up when deleting the profile.
			if err := b.removeProfile(profile); err != nil {
				return err
			}
		}

		// Create the distro from the kernel+ramdisk
		_, err := processWrapper("cobbler", "distro", "add", "--name="+profile, "--kernel="+filepath.Join(igorConfig.TFTPRoot, "igor", r.KernelHash+"-kernel"), "--initrd="+filepath.Join(igorConfig.TFTPRoot, "igor", r.InitrdHash+"-initrd"), "--kopts="+r.KernelArgs)
		if err != nil {
			return err
		}

		// Create a profile from the distro we just made
		_, err = processWrapper("cobbler", "profile", "add", "--name="+profile, "--distro="+profile)
		if err != nil {
			return err
		}

		// Now set each host to boot from that profile
		runner := DefaultRunner(func(host string) error {
			if _, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--profile="+profile); err != nil {
				return err
			}

			// We make sure to set netboot enabled so the nodes can boot
			_, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--netboot-enabled=true")
			return err
		})

		if err := runner.RunAll(r.Hosts); err != nil {
			return fmt.Errorf("unable to set cobbler profile: %v", err)
		}
	} else {
		if !b.profiles[r.CobblerProfile] {
			return fmt.Errorf("cobbler profile does not exist: %v", r.CobblerProfile)
		}

		// If the requested profile exists, go ahead and set the nodes to use it
		runner := DefaultRunner(func(host string) error {
			if _, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--profile="+r.CobblerProfile); err != nil {
				return err
			}

			// We make sure to set netboot enabled so the nodes can boot
			_, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--netboot-enabled=true")
			return err
		})

		if err := runner.RunAll(r.Hosts); err != nil {
			return fmt.Errorf("unable to set cobbler profile: %v", err)
		}
	}

	f, err := os.Create(r.Filename())
	if err == nil {
		f.Close()
	}
	return err
}

func (b *CobblerBackend) Uninstall(r Reservation) error {
	// Set all nodes in the reservation back to the default profile
	runner := DefaultRunner(func(host string) error {
		_, err := processWrapper("cobbler", "system", "edit", "--name="+host, "--profile="+igorConfig.CobblerDefaultProfile)
		return err
	})

	if err := runner.RunAll(r.Hosts); err != nil {
		return fmt.Errorf("unable to set cobbler profile: %v", err)
	}

	// Delete the profile and distro we created for this reservation
	if r.CobblerProfile == "" {
		return b.removeProfile("igor_" + r.ResName)
	}

	return nil
}

func (b *CobblerBackend) removeProfile(profile string) error {
	if _, err := processWrapper("cobbler", "profile", "remove", "--name="+profile); err != nil {
		return err
	}

	_, err := processWrapper("cobbler", "distro", "remove", "--name="+profile)
	return err
}

func (b *CobblerBackend) Power(hosts []string, on bool) error {
	command := "poweroff"
	if on {
		command = "poweron"
	}

	runner := DefaultRunner(func(host string) error {
		_, err := processWrapper("cobbler", "system", command, "--name", host)
		return err
	})

	return runner.RunAll(hosts)
}

func CobblerProfiles() map[string]bool {
	res := map[string]bool{}

	// Get a list of current profiles
	out, err := processWrapper("cobbler", "profile", "list")
	if err != nil {
		log.Fatal("couldn't get list of cobbler profiles: %v\n", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		res[strings.TrimSpace(scanner.Text())] = true
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("unable to read cobbler profiles: %v", err)
	}

	return res
}
