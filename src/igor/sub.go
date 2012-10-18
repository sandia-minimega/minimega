package main

import (
	"ranges"
)

var cmdSub = &Command{
	UsageLine: "sub -r <reservation name> -k <kernel path> -i <initrd path> {-n <integer> | -w <node list>} [OPTIONS]",
	Short:	"create a reservation",
	Long:`
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
var subN int // -n
var subW string // -w
var subC string // -c
var subO bool // -o
var subT int // -t

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
	cmdSub.Flag.IntVar(&subT, "t", 0, "")
}

func runSub(cmd *Command, args []string) {
	var nodes []string

	// validate arguments
	if subR == "" || subK == "" || subI == "" || (subN == 0 && subW == "") {
		errorf("Missing required argument!")
		help([]string{ "sub" })
		exit()
	}

	// figure out which nodes to reserve
	if subW != nil {
		rnge := ranges.NewRange(PREFIX, START, END)
		nodes := rnge.SplitRange(subW)
	}

	// copy kernel and initrd

	// create appropriate pxe config file in TFTPROOT+/pxelinux.cfg/igor/

	// create individual PXE boot configs i.e. TFTPROOT+/pxelinux.cfg/AC10001B by copying config created above

	// reboot all the nodes in the reservation (unless -O)
}
