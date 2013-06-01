package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"ranges"
	"syscall"
	"time"
)

var cmdSub = &Command{
	UsageLine: "sub -r <reservation name> -k <kernel path> -i <initrd path> {-n <integer> | -w <node list>} [OPTIONS]",
	Short:     "create a reservation",
	Long: `
Create a new reservation.

REQUIRED FLAGS:

The -r flag sets the name for the reservation.

The -k flag gives the location of the kernel the nodes should boot. This
kernel will be copied to a separate directory for use.

The -i flag gives the location of the initrd the nodes should boot. This
file will be copied to a separate directory for use.

The -n flag indicates that the specified number of nodes should be
included in the reservation. The first available nodes will be allocated.

The -w flag specifies that the given nodes should be included in the
reservation. This will return an error if the nodes are already reserved.

OPTIONAL FLAGS:

The -c flag sets any kernel command line arguments. (eg "console=tty0").

The -o flag revents nodes from rebooting after a reservation has been successfully placed.

The -t flag is used to specify the reservation time in integer hours. (default = 12)
	`,
}

var subR string // -r flag
var subK string // -k flag
var subI string // -i
var subN int    // -n
var subW string // -w
var subC string // -c
var subO bool   // -o
var subT int    // -t

func init() {
	// break init cycle
	cmdSub.Run = runSub

	cmdSub.Flag.StringVar(&subR, "r", "", "")
	cmdSub.Flag.StringVar(&subK, "k", "", "")
	cmdSub.Flag.StringVar(&subI, "i", "", "")
	cmdSub.Flag.IntVar(&subN, "n", 0, "")
	cmdSub.Flag.StringVar(&subW, "w", "", "")
	cmdSub.Flag.StringVar(&subC, "c", "", "")
	cmdSub.Flag.BoolVar(&subO, "o", false, "")
	cmdSub.Flag.IntVar(&subT, "t", 12, "")
}

func runSub(cmd *Command, args []string) {
	var nodes []string
	var IPs []net.IP
	var pxefiles []string

	// Open and lock the reservation file
	path := igorConfig.TFTPRoot + "/igor/reservations.json"
	resdb, err := os.OpenFile(path, os.O_RDWR, 664)
	if err != nil {
		fatalf("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later

	reservations := getReservations(resdb)

	// validate arguments
	if subR == "" || subK == "" || subI == "" || (subN == 0 && subW == "") {
		errorf("Missing required argument!")
		help([]string{"sub"})
		exit()
	}

	// figure out which nodes to reserve
	if subW != "" {
		rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
		nodes, _ = rnge.SplitRange(subW)
	}

	// Convert list of node names to PXE filenames
	// 1. lookup nodename -> IP
	for _, hostname := range nodes {
		ip, err := net.LookupIP(hostname)
		if err != nil {
			fatalf("failure looking up %v: %v", hostname, err)
		}
		IPs = append(IPs, ip...)
	}

	// 2. IP -> hex
	for _, ip := range IPs {
		pxefiles = append(pxefiles, toPXE(ip))
	}

	// Make sure none of those nodes are reserved
	// Check every reservation...
	for _, res := range reservations {
		// For every node in a reservation...
		for _, node := range res.PXENames {
			// make sure no node in *our* potential reservation conflicts
			for _, pxe := range pxefiles {
				if node == pxe {
					fatalf("Conflict with reservation %v, specific PXE file %v\n", res.ResName, pxe)
				}
			}
		}
	}

	// Ok, build our reservation
	reservation := Reservation{ResName: subR, Hosts: nodes, PXENames: pxefiles}
	user, err := user.Current()
	reservation.Owner = user.Username
	reservation.Expiration = (time.Now().Add(time.Duration(subT) * time.Hour)).Unix()

	// Add it to the list of reservations
	reservations = append(reservations, reservation)

	// copy kernel and initrd
	// 1. Validate and open source files
	ksource, err := os.Open(subK)
	if err != nil {
		fatalf("couldn't open kernel: %v", err)
	}
	isource, err := os.Open(subI)
	if err != nil {
		fatalf("couldn't open initrd: %v", err)
	}

	// make kernel copy
	kdest, err := os.Create(igorConfig.TFTPRoot + "/igor/" + subR + "-kernel")
	if err != nil {
		fatalf("%v", err)
	}
	io.Copy(kdest, ksource)
	kdest.Close()
	ksource.Close()

	// make initrd copy
	idest, err := os.Create(igorConfig.TFTPRoot + "/igor/" + subR + "-initrd")
	if err != nil {
		fatalf("%v", err)
	}
	io.Copy(idest, isource)
	idest.Close()
	isource.Close()

	// create appropriate pxe config file in igorConfig.TFTPRoot+/pxelinux.cfg/igor/
	masterfile, err := os.Create(igorConfig.TFTPRoot + "/pxelinux.cfg/igor/" + subR)
	if err != nil {
		fatalf("failed to create %v: %v", igorConfig.TFTPRoot+"pxelinux.cfg/igor/"+subR, err)
	}
	defer masterfile.Close()
	masterfile.WriteString(fmt.Sprintf("default %s\n\n", subR))
	masterfile.WriteString(fmt.Sprintf("label %s\n", subR))
	masterfile.WriteString(fmt.Sprintf("kernel /igor/%s-kernel\n", subR))
	masterfile.WriteString(fmt.Sprintf("append initrd=/igor/%s-initrd\n", subR))

	// create individual PXE boot configs i.e. igorConfig.TFTPRoot+/pxelinux.cfg/AC10001B by copying config created above
	for _, pxename := range pxefiles {
		masterfile.Seek(0, 0)
		f, err := os.Create(igorConfig.TFTPRoot + "/pxelinux.cfg/" + pxename)
		if err != nil {
			fatalf("%v", err)
		}
		io.Copy(f, masterfile)
		f.Close()
	}

	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(reservations)
	resdb.Sync()
}
