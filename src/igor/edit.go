package main

import (
	log "minilog"
	"os/user"
)

var cmdEdit = &Command{
	UsageLine: "edit -r <reservation name> [OPTIONS]",
	Short:     "create a reservation",
	Long: `
Edit an existing reservation. If the nodes are already booted, they will have
to be rebooted for any changes to take effect.

REQUIRED FLAGS:

The -r flag specifies the name of the reservation to edit.

OPTIONAL FLAGS:

See "igor sub" for the meanings of the -k, -i, -profile, and -c flags. As with
"igor sub", the -profile flag takes precedence over the other flags.

The -owner flag can be used to modify the reservation owner (admin only). If
-owner is specified, all other edits are ignored. Similarily, the -g flag can
be used to modify the reservation group (admin only).`,
}

var subOwner string // -owner

func init() {
	// break init cycle
	cmdEdit.Run = runEdit

	cmdEdit.Flag.StringVar(&subR, "r", "", "")
	cmdEdit.Flag.StringVar(&subK, "k", "", "")
	cmdEdit.Flag.StringVar(&subI, "i", "", "")
	cmdEdit.Flag.StringVar(&subC, "c", "", "")
	cmdEdit.Flag.StringVar(&subProfile, "profile", "", "")
	cmdEdit.Flag.StringVar(&subOwner, "owner", "", "")
	cmdEdit.Flag.StringVar(&subG, "g", "", "")
}

func runEdit(cmd *Command, args []string) {
	if subR == "" {
		help([]string{"sub"})
		log.Fatalln("missing required argument")
	}

	if subProfile != "" && !igorConfig.UseCobbler {
		log.Fatalln("igor is not configured to use Cobbler, cannot specify a Cobbler profile")
	}

	u, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	r := FindReservation(subR)
	if r == nil {
		log.Fatal("reservation does not exist: %v", subR)
	}

	if !r.IsWritable(u) {
		log.Fatal("insufficient privileges to edit reservation: %v", subR)
	}

	if subOwner != "" || subG != "" {
		if u.Username != "root" {
			log.Fatalln("only root can modify reservation owner or group")
		}

		if subOwner != "" {
			r.Owner = subOwner
		}
		if subG != "" {
			g, err := user.LookupGroup(subG)
			if err != nil {
				log.Fatalln(err)
			}

			r.Group = subG
			r.GroupID = g.Gid
		}
		dirty = true
		return
	}

	// create copy
	var r2 *Reservation
	*r2 = *r

	// modify r2 based on the flags
	if r.CobblerProfile != "" && subProfile != "" {
		// changing from one cobbler profile to another
		r2.CobblerProfile = subProfile
	} else if r.CobblerProfile != "" && subProfile == "" {
		// changing from cobbler profile to kernel/initrd
		if subK == "" || subI == "" {
			log.Fatal("must specify a kernel & initrd since reservation used profile before")
		}

		r2.CobblerProfile = ""
		if err := r2.SetKernelInitrd(subK, subI); err != nil {
			log.Fatalln(err)
		}

		r2.KernelArgs = subC
	} else if subProfile != "" {
		// changing from kernel/initrd to cobbler profile
		r2.CobblerProfile = subProfile
		r2.Kernel = ""
		r2.KernelHash = ""
		r2.Initrd = ""
		r2.InitrdHash = ""
		r2.KernelArgs = ""
	} else {
		// tweaking kernel/initrd/kernel args
		if subK != "" {
			if err := r2.SetKernel(subK); err != nil {
				log.Fatal("edit failed: %v", err)
			}
		}

		if subI != "" {
			if err := r2.SetInitrd(subI); err != nil {
				// clean up (possibly) already installed kernel
				if err := r2.PurgeFiles(); err != nil {
					log.Error("leaked kernel: %v", subK)
				}

				log.Fatal("edit failed: %v", err)
			}
		}

		// always change kernel args if they are different since we don't
		// know if the user intends to clear them or not
		//
		// TODO: is this the right behavior?
		if r.KernelArgs != subC {
			r2.KernelArgs = subC
		}
	}

	// replace reservation with modified version
	Reservations[r.ID] = r2
	dirty = true

	backend := GetBackend()

	if r.Installed {
		if err := backend.Uninstall(r); err != nil {
			log.Fatal("unable to uninstall old reservation: %v", err)
		}

		if err := backend.Install(r2); err != nil {
			log.Fatal("unable to install edited reservation: %v", err)
		}
	}

	// clean up any files that are no longer needed
	if err := r.PurgeFiles(); err != nil {
		log.Error("leaked files: %v", err)
	}

	emitReservationLog("EDITED", r2)
}