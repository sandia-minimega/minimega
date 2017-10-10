// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// igor is a simple command line tool for managing reservations of nodes in a
// cluster. It also will configure the pxeboot environment for booting kernels
// and initrds for cluster nodes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	log "minilog"
	"os"
	"path/filepath"
	"ranges"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Constants
const MINUTES_PER_SLICE = 1 // must be less than 60! 1, 5, 10, or 15 would be good choices
// Minimum schedule length in minutes, 720 = 12 hours
const MIN_SCHED_LEN = 72

// Global Variables
var configpath = flag.String("config", "/etc/igor.conf", "Path to configuration file")
var igorConfig Config

var Reservations map[uint64]Reservation // map ID to reservations
var Schedule []TimeSlice                // The schedule

var resdb *os.File
var scheddb *os.File

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'go help'.
var commands = []*Command{
	cmdDel,
	cmdShow,
	cmdSub,
	cmdPower,
}

var exitStatus = 0
var exitMu sync.Mutex

// The configuration of the system
type Config struct {
	TFTPRoot              string
	Prefix                string
	Start                 int
	End                   int
	Padlen                int
	Rackwidth             int
	Rackheight            int
	PowerOnCommand        string
	PowerOffCommand       string
	UseCobbler            bool
	CobblerDefaultProfile string
	AutoReboot            bool
	VLANMin               int               `json:"vlan_min"`
	VLANMax               int               `json:"vlan_max"`
	NodeMap               map[string]string `json:"node_map"`
	Network               string
	NetworkUser           string
	NetworkPassword       string
	NetworkURL            string `json:"network_url"`
	DNSServer             string
	LogFile               string
	NodeLimit             int
	TimeLimit             int
}

// Represents a slice of time
type TimeSlice struct {
	Start int64    // UNIX time
	End   int64    // UNIX time
	Nodes []uint64 // slice of len(# of nodes), mapping to reservation IDs
}

type Reservation struct {
	ResName        string
	CobblerProfile string   // Optional; if set, use this Cobbler profile instead of a kernel+initrd
	Hosts          []string // separate, not a range
	PXENames       []string // eg C000025B
	StartTime      int64    // UNIX time
	EndTime        int64    // UNIX time
	Duration       float64  // minutes
	Owner          string
	ID             uint64
	KernelArgs     string
	Vlan           int
}

// Sort the slice of reservations based on the start time
type StartSorter []Reservation

func (s StartSorter) Len() int {
	return len(s)
}

func (s StartSorter) Less(i, j int) bool {
	return s[i].StartTime < s[j].StartTime
}

func (s StartSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func setExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

func emitReservationLog(action string, res Reservation) {
	format := "2006-Jan-2-15:04"
	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	unsplit, _ := rnge.UnsplitRange(res.Hosts)
	log.Info("%s	user=%v	resname=%v	nodes=%v	start=%v	end=%v	duration=%v\n", action, res.Owner, res.ResName, unsplit, time.Unix(res.StartTime, 0).Format(format), time.Unix(res.EndTime, 0).Format(format), res.Duration)
}

// Read the reservations, delete any that are too old.
// Copy in netboot files for any reservations that have just started
func housekeeping() {
	now := time.Now().Unix()

	var cobblerProfiles string
	if igorConfig.UseCobbler {
		var err error
		// Get a list of current profiles
		cobblerProfiles, err = processWrapper("cobbler", "profile", "list")
		if err != nil {
			log.Fatal("couldn't get list of cobbler profiles: %v\n", cobblerProfiles)
		}
	}

	for _, r := range Reservations {
		// Check if $TFTPROOT/pxelinux.cfg/igor/ResName exists. This is how we verify if the reservation is installed or not
		filename := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", r.ResName)
		if r.EndTime < now {
			deleteReservation(false, []string{r.ResName})
		} else if _, err := os.Stat(filename); os.IsNotExist(err) && r.StartTime < now {
			emitReservationLog("INSTALL", r)
			// update network config
			err := networkSet(r.Hosts, r.Vlan)
			if err != nil {
				log.Error("error setting network isolation: %v", err)
			}
			if !igorConfig.UseCobbler {
				log.Info("Installing files for reservation ", r.ResName)

				// create appropriate pxe config file in igorConfig.TFTPRoot+/pxelinux.cfg/igor/
				masterfile, err := os.Create(filename)
				if err != nil {
					log.Fatal("failed to create %v -- %v", filename, err)
				}
				defer masterfile.Close()
				masterfile.WriteString(fmt.Sprintf("default %s\n\n", r.ResName))
				masterfile.WriteString(fmt.Sprintf("label %s\n", r.ResName))
				masterfile.WriteString(fmt.Sprintf("kernel /igor/%s-kernel\n", r.ResName))
				masterfile.WriteString(fmt.Sprintf("append initrd=/igor/%s-initrd %s\n", r.ResName, r.KernelArgs))

				// create individual PXE boot configs i.e. igorConfig.TFTPRoot+/pxelinux.cfg/AC10001B by copying config created above
				for _, pxename := range r.PXENames {
					masterfile.Seek(0, 0)
					fname := filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", pxename)
					f, err := os.Create(fname)
					if err != nil {
						log.Fatal("failed to create %v -- %v", fname, err)
					}
					io.Copy(f, masterfile)
					f.Close()
				}
				powerCycle(r.Hosts)
			} else {
				// If we're not using an existing profile, create one and set the nodes to use it
				if r.CobblerProfile == "" && !strings.Contains(cobblerProfiles, "igor_"+r.ResName) {
					_, err := processWrapper("cobbler", "distro", "add", "--name=igor_"+r.ResName, "--kernel="+filepath.Join(igorConfig.TFTPRoot, "igor", r.ResName+"-kernel"), "--initrd="+filepath.Join(igorConfig.TFTPRoot, "igor", r.ResName+"-initrd"), "--kopts="+r.KernelArgs)
					if err != nil {
						log.Fatal("cobbler: %v", err)
					}
					_, err = processWrapper("cobbler", "profile", "add", "--name=igor_"+r.ResName, "--distro=igor_"+r.ResName)
					if err != nil {
						log.Fatal("cobbler: %v", err)
					}
					done := make(chan bool)
					systemfunc := func(h string) {
						_, err = processWrapper("cobbler", "system", "edit", "--name="+h, "--profile=igor_"+r.ResName)
						if err != nil {
							log.Fatal("cobbler: %v", err)
						}
						_, err = processWrapper("cobbler", "system", "edit", "--name="+h, "--netboot-enabled=true")
						if err != nil {
							log.Fatal("cobbler: %v", err)
						}
						done<- true
					}
					for _, host := range r.Hosts {
						go systemfunc(host)
					}
					for _, _ = range r.Hosts {
						<-done
					}
					powerCycle(r.Hosts)
				} else if r.CobblerProfile != "" && strings.Contains(cobblerProfiles, r.CobblerProfile) {
					// If the requested profile exists, go ahead and set the nodes to use it
					for _, host := range r.Hosts {
						_, err = processWrapper("cobbler", "system", "edit", "--name="+host, "--profile="+r.CobblerProfile)
						if err != nil {
							log.Fatal("cobbler: %v", err)
						}
					}
					powerCycle(r.Hosts)
				}
				os.Create(filename)
			}
		}
	}

	expireSchedule()
	putSchedule()
}

func powerCycle(Hosts []string) {
	if igorConfig.AutoReboot {
		doPower(Hosts, "off")
		doPower(Hosts, "on")
	}
}

func init() {
	Reservations = make(map[uint64]Reservation)
}

func main() {
	var err error

	log.Init()

	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}

	if args[0] == "help" {
		help(args[1:])
		return
	}

	rand.Seed(time.Now().Unix())

	igorConfig = readConfig(*configpath)

	// Add another logger for the logfile, if set
	if igorConfig.LogFile != "" {
		logfile, err := os.OpenFile(igorConfig.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			log.Fatal("Couldn't create logfile %v: %v", igorConfig.LogFile, err)
		}
		log.AddLogger("file", logfile, log.INFO, false)
	}

	// Read in the reservations
	// We open the file here so resdb.Close() doesn't happen until program exit
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/reservations.json")
	resdb, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	// This should prevent anyone else from modifying the reservation file while
	// we're using it. Bonus: Flock goes away if the program crashes so state is easy
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later

	getReservations()

	// Read in the schedule
	path = filepath.Join(igorConfig.TFTPRoot, "/igor/schedule.json")
	scheddb, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Warn("failed to open schedule file: %v", err)
	}
	defer resdb.Close()
	// We probably don't need to lock this too but I'm playing it safe
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later
	getSchedule()

	// Here, we need to go through and delete any reservations which should be expired,
	// and bring in new ones that are just starting
	housekeeping()

	// Now process the command
	for _, cmd := range commands {
		if cmd.Name() == args[0] && cmd.Run != nil {
			cmd.Flag.Usage = func() { cmd.Usage() }
			if cmd.CustomFlags {
				args = args[1:]
			} else {
				cmd.Flag.Parse(args[1:])
				args = cmd.Flag.Args()
			}
			cmd.Run(cmd, args)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "go: unknown subcommand %q\nRun 'go help' for usage.\n", args[0])
	setExitStatus(2)
}

// Read in the reservations from the already-open resdb file
func getReservations() {
	dec := json.NewDecoder(resdb)
	err := dec.Decode(&Reservations)
	// an empty file is OK, but other errors are not
	if err != nil && err != io.EOF {
		log.Fatal("failure parsing reservation file: %v", err)
	}
}

func getSchedule() {
	dec := json.NewDecoder(scheddb)
	err := dec.Decode(&Schedule)
	// an empty file is OK, but other errors are not
	if err != nil && err != io.EOF {
		log.Fatal("failure parsing schedule file: %v", err)
	}
}

func putReservations() {
	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(Reservations)
	resdb.Sync()
}

func putSchedule() {
	// Truncate the existing schedule file
	scheddb.Truncate(0)
	scheddb.Seek(0, 0)
	// Write out the new schedule
	enc := json.NewEncoder(scheddb)
	enc.Encode(Schedule)
	scheddb.Sync()
}

func readConfig(path string) (c Config) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal("Couldn't read config file: %v", err)
	}

	err = json.Unmarshal(b, &c)
	if err != nil {
		log.Fatal("Couldn't parse json: %v", err)
	}
	return
}
