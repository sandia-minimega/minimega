package main

import (
	log "minilog"
	"path/filepath"
)

var cmdEdit = &Command{
	UsageLine: "edit -r <reservation name> [OPTIONS]",
	Short:     "create a reservation",
	Long: `
Edit an existing reservation. If the nodes are already booted, they will have
to be rebooted for any changes to take effect.

REQUIRED FLAGS:

The -r flag specifies the name of the reservation to edit.

See "igor sub" for the meanings of the -k, -i, -profile, and -c flags. As with
"igor sub", the -profile flag takes precedence over the other flags.`,
}

func init() {
	// break init cycle
	cmdEdit.Run = runEdit

	cmdEdit.Flag.StringVar(&subR, "r", "", "")
	cmdEdit.Flag.StringVar(&subK, "k", "", "")
	cmdEdit.Flag.StringVar(&subI, "i", "", "")
	cmdEdit.Flag.StringVar(&subC, "c", "", "")
	cmdEdit.Flag.StringVar(&subProfile, "profile", "", "")
}

func runEdit(cmd *Command, args []string) {
	if subR == "" {
		help([]string{"sub"})
		log.Fatalln("missing required argument")
	}

	if subProfile != "" && !igorConfig.UseCobbler {
		log.Fatalln("igor is not configured to use Cobbler, cannot specify a Cobbler profile")
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	for _, r := range Reservations {
		if r.ResName != subR {
			continue
		}

		// The reservation name is unique if it exists
		if r.Owner != user.Username && user.Username != "root" {
			log.Fatal("Cannot edit reservation %v: insufficient privileges", subR)
		}

		if !r.Installed {
			log.Fatal("Cannot edit reservation that is not installed")
		}

		// create copy
		r2 := r

		backend := GetBackend()

		if r.CobblerProfile != "" {
			if subProfile != "" {
				// changing from one cobbler profile to another
				if err := backend.SetProfile(r, subProfile); err != nil {
					log.Fatal("unable to change profile: %v", err)
				}

				r.CobblerProfile = subProfile
			} else {
				// changing from cobbler profile to kernel/initrd
				if subK == "" || subI == "" {
					log.Fatal("must specify a kernel & initrd since reservation used profile before")
				}

				log.Fatal("not implemented")
			}
		} else if subProfile != "" {
			// changing from kernel/initrd to cobbler profile
			if err := backend.SetProfile(r, subProfile); err != nil {
				log.Fatal("unable to change profile: %v", err)
			}

			r2.CobblerProfile = subProfile
			r2.Kernel = ""
			r2.KernelHash = ""
			r2.Initrd = ""
			r2.InitrdHash = ""
			r2.KernelArgs = ""
		} else {
			var err error
			dir := filepath.Join(igorConfig.TFTPRoot, "igor")

			// tweaking kernel/initrd/kernel args
			if subK != "" {
				r2.Kernel = subK
				if r2.KernelHash, err = install(subK, dir, "-kernel"); err != nil {
					log.Fatal("edit failed: %v", err)
				}

				if err := backend.SetKernel(r, r2.KernelHash); err != nil {
					// TODO: we may leak a kernel here
					log.Fatal("edit failed: %v", err)
				}
			}

			if subI != "" {
				r2.Initrd = subI
				if r2.InitrdHash, err = install(subI, dir, "-kernel"); err != nil {
					// TODO: we may leak a kernel here
					log.Error("unable to update initrd: %v", err)
				}

				if err := backend.SetInitrd(r, r2.InitrdHash); err != nil {
					// TODO: we may leak a kernel here
					log.Fatal("unable to update initrd: %v", err)
				}
			}

			// always change kernel args if they are different since we don't
			// know if the user intends to clear them or not
			//
			// TODO: is this the right behavior?
			if r.KernelArgs != subC {
				if err := backend.SetKernelArgs(r, subC); err != nil {
					// TODO: we may leak a kernel and initrd here
					log.Error("edit failed: %v", err)
				}
				r2.KernelArgs = subC
			}
		}

		Reservations[r.ID] = r2

		// purgeFiles from the original reservation in case we changed the
		// kernel and/or initrd so they're no longer used.
		if err := purgeFiles(r); err != nil {
			log.Error("unable to purge files: %v", err)
		}

		dirty = true

		emitReservationLog("EDITED", r2)

		return
	}

	// didn't find the reservation
	log.Fatal("Reservation %v does not exist", subR)
}
