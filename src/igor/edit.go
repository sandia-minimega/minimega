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

See "igor sub" for the meanings of the -k, -i, -profile, -vlan, and -c flags.
As with "igor sub", the -profile flag takes precedence over the other flags.

The -owner flag can be used to modify the reservation owner (admin only). If
-owner is specified, all other edits are ignored.

Similarily, the -g flag can be used to modify the reservation group. Only the
owner can modify the group. If -g is specified, all other edits are ignored.`,
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
	cmdEdit.Flag.StringVar(&subVlan, "vlan", "", "")
	cmdEdit.Flag.StringVar(&subOwner, "owner", "", "")
	cmdEdit.Flag.StringVar(&subG, "g", "", "")
}

func runEdit(cmd *Command, args []string) {
	if subR == "" {
		help([]string{"sub"})
		log.Fatalln("missing required argument")
	}

	if subProfile != "" && !igor.UseCobbler {
		log.Fatalln("igor is not configured to use Cobbler, cannot specify a Cobbler profile")
	}

	r := igor.Find(subR)
	if r == nil {
		log.Fatal("reservation does not exist: %v", subR)
	}

	if !r.IsWritable(igor.User) {
		log.Fatal("insufficient privileges to edit reservation: %v", subR)
	}

	if subOwner != "" {
		if igor.Username != "root" {
			log.Fatalln("only root can modify reservation owner")
		}

		igor.EditOwner(r, subOwner)
		return
	}

	if subG != "" {
		if igor.Username != "root" && r.Owner != igor.Username {
			log.Fatalln("only owner or root can modify reservation group")
		}

		g, err := user.LookupGroup(subG)
		if err != nil {
			log.Fatalln(err)
		}

		igor.EditGroup(r, subG, g.Gid)
		return
	}

	// create copy
	r2 := new(Reservation)
	*r2 = *r

	// clear error because maybe the problem has been fixed
	r2.InstallError = ""

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

		if err := r2.SetKernel(subK); err != nil {
			log.Fatalln(err)
		}
		if err := r2.SetInitrd(subI); err != nil {
			if err := igor.PurgeFiles(r2); err != nil {
				log.Error("leaked kernel: %v", subK)
			}
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
				if err := igor.PurgeFiles(r2); err != nil {
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

	if subVlan != "" {
		vlanID, err := parseVLAN(subVlan)
		if err != nil {
			log.Fatalln(err)
		}
		r2.Vlan = vlanID
	}

	// replace reservation with modified version
	igor.Edit(r, r2)

	if r.Installed {
		if err := igor.Uninstall(r); err != nil {
			log.Fatal("unable to uninstall old reservation: %v", err)
		}

		if err := igor.Backend.Install(r2); err != nil {
			log.Fatal("unable to install edited reservation: %v", err)
		}
	}

	// clean up any files that are no longer needed
	if err := igor.PurgeFiles(r); err != nil {
		log.Error("leaked files: %v", err)
	}

	emitReservationLog("EDITED", r2)
}
