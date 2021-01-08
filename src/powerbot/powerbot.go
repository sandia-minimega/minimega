// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	log "minilog"
	"os"
	"ranges"
	"strings"
)

var (
	f_config = flag.String("config", "/etc/powerbot.conf", "path to config file")
	f_PDU    = flag.Bool("pdu", false, "Force PDU only")
	config   Config
)

func usage() {
	fmt.Print(`powerbot: control a PDU.
Usage, <arg> = required, [arg] = optional:
	powerbot on <nodelist>
	powerbot off <nodelist>
	powerbot cycle <nodelist>
        powerbot status [nodelist]
        powerbot                    # equivalent to "powerbot status"
	powerbot temp <nodelist>    # IPMI only - temp sensor info
	powerbot full <nodelist>   # IPMI only - full sensor info

Node lists are in standard range format, i.e. node[1-5,8-10,15]
`)
	log.Fatal("invalid arguments")
}

// Implement your PDU however you like, just so long as
// it acts like this
type PDU interface {
	On(map[string]string) error
	Off(map[string]string) error
	Cycle(map[string]string) error
	Status(map[string]string) error
	Temp() error // IPMI only - noop for PDUs
	Info() error // IPMI only - noop for PDUs
}

// This maps the Device.pdutype variable to a function
// The signature is func(host, port, username, password string)
var PDUtypes = map[string]func(string, string, string, string) (PDU, error){
	"tripplite":  NewTrippLitePDU,
	"servertech": NewServerTechPDU,
}

// One device as read from the config file
type Device struct {
	name     string
	host     string
	port     string
	pdutype  string
	username string
	password string
	outlets  map[string]string // map hostname -> outlet name
}

// IPMI configuration as read from the config file
type IPMIData struct {
	ip       string
	node     string
	password string
	username string
}

// This gets read from the config file
type Config struct {
	nodes    []string
	devices  map[string]Device
	ipmiPath string
	ipmis    map[string]IPMIData // hostname -> IPMIData
	prefix   string              // node name prefix, e.g. "ccc" for "ccc[1-100]"
}

// Parse the config file and store it in the global config
func readConfig(filename string) (Config, error) {
	var ret Config
	ret.devices = make(map[string]Device)
	ret.ipmis = make(map[string]IPMIData)
	ret.ipmiPath = "ipmitool"

	f, err := os.Open(filename)
	if err != nil {
		return ret, err
	}
	b := bufio.NewScanner(f)
	for b.Scan() {
		line := b.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "prefix":
			if len(fields) != 2 {
				return ret, errors.New("must specify a prefix")
			}
			ret.prefix = fields[1]
		case "device":
			if len(fields) != 7 {
				continue
			}
			var d Device
			d.name = fields[1]
			d.pdutype = fields[2]
			d.host = fields[3]
			d.port = fields[4]
			d.username = fields[5]
			d.password = fields[6]
			d.outlets = make(map[string]string)
			ret.devices[d.name] = d
		case "ipmi":
			ret.ipmiPath = fields[1]
		case "node":
			ln := len(fields)
			nodename := fields[1]
			dev := fields[2]
			outlet := fields[3]
			if _, ok := ret.devices[dev]; ok {
				ret.devices[dev].outlets[nodename] = outlet
			}
			ret.nodes = append(ret.nodes, nodename)
			// IPMI Data
			if ln > 4 {
				var ipmi IPMIData
				ipmi.ip = fields[4]
				ipmi.username = fields[5]
				ipmi.password = fields[6]
				ipmi.node = nodename
				ret.ipmis[nodename] = ipmi
			}
		case "loglevel":
			if len(fields) > 1 {
				ll, err := log.ParseLevel(fields[1])
				if err != nil {
					log.Error(err.Error())
					return ret, nil
				}
				log.AddSyslog("local", "", "powerbot", ll)
			}
		}
	}
	return ret, nil
}

func main() {
	var err error
	var command string
	var nodes string

	// Get flags and arguments
	flag.Parse()
	args := flag.Args()
	log.Init()
	log.AddSyslog("local", "", "powerbot", log.INFO)

	if len(args) == 0 {
		command = "status"
	}

	// Parse configuration file
	config, err = readConfig(*f_config)
	if err != nil {
		log.Fatal(err.Error())
	}

	if len(args) == 2 {
		// First argument is a command: "on", "off", etc.
		// second arg is node list
		command = args[0]
		nodes = args[1]
	} else if len(args) == 1 {
		// If they said "powerbot status", show status for all nodes
		if args[0] == "status" {
			command = "status"
			// Leave nodes unset, we want everything
		} else {
			// Assume they gave a list of nodes for status
			nodes = args[0]
		}
	} else {
		// Assume they want status of everything
		command = "status"
	}

	// Prepare the list of nodes
	var nodeList []string
	if nodes != "" {
		ranger, _ := ranges.NewRange(config.prefix, 0, 1000000)
		nodeList, err = ranger.SplitRange(nodes)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		nodeList = config.nodes
	}

	// Try IPMI first, unless we opt out
	var remainingNodes []string
	if *f_PDU == false {
		remainingNodes = useIPMI(nodeList, command)
	} else {
		remainingNodes = nodeList
	}

	// Stop if we are done, otherwise continue with PDUs
	if len(remainingNodes) == 0 {
		return
	}
	// Find a list of what devices and ports are affected
	// by the command
	devs := make(map[string]Device)
	devs, err = findOutletsAndDevs(remainingNodes)
	if err != nil {
		log.Fatal(err.Error())
	}
	log.Info("Attempting PDU commands...")

	// For each device affected, perform the command
	for _, dev := range devs {
		log.Info("Performing powerbot %v on %v:%v:%v with type %v", command, dev.name, dev.host, dev.port, dev.pdutype)
		var pdu PDU
		// First, let's see if IPMI is available
		pdu, err = PDUtypes[dev.pdutype](dev.host, dev.port, dev.username, dev.password)
		if err != nil {
			log.Error(err.Error())
			continue
		}

		switch command {
		case "on":
			err := pdu.On(dev.outlets)
			if err != nil {
				log.Error(err.Error())
			}
		case "off":
			err := pdu.Off(dev.outlets)
			if err != nil {
				log.Error(err.Error())
			}
		case "cycle":
			err := pdu.Cycle(dev.outlets)
			if err != nil {
				log.Error(err.Error())
			}
		case "status":
			err := pdu.Status(dev.outlets)
			if err != nil {
				log.Error(err.Error())
			}
		case "temp", "info":
			fmt.Println("Invalid PDU command; Remaining nodes skipped.")
		default:
			usage()
		}
	}
}

// Takes a range specification like ccc[1-4,6,8-10], converts
// it to a list of node names (ccc1 ccc2 and so on), then makes
// a collection of Devices which contain only those corresponding
// outlets in their outlet list.
// This makes it handy because you'll generally be calling On(),
// Off(), etc. a device at a time.
func findOutletsAndDevs(nodes []string) (map[string]Device, error) {
	ret := make(map[string]Device)

	// This is really gross but you won't have a ton of devices anyway
	// so it should be pretty fast.
	// For each of the specified nodes...
	for _, n := range nodes {
		// Check in each device...
		for _, d := range config.devices {
			// If that node is connected to this device...
			if o, ok := d.outlets[n]; ok {
				if _, ok := ret[d.name]; ok {
					// either add the outlet to an
					// existing return device...
					ret[d.name].outlets[n] = o
				} else {
					// or create a new device to
					// return, and add the outlet
					tmp := Device{name: d.name, host: d.host, port: d.port, pdutype: d.pdutype, username: d.username, password: d.password}
					tmp.outlets = make(map[string]string)
					tmp.outlets[n] = o
					ret[tmp.name] = tmp
				}
			}
		}
	}
	return ret, nil
}

// This will create a proper node list and execute
// IPMI commands on each
func useIPMI(s []string, c string) []string {
	ipmis := config.ipmis
	var ret []string
	var dummyMap map[string]string //doesn't apply to IPMI
	log.Info("Attempting IPMI commands...")

	for _, n := range s {
		var ipmi PDU
		var err error
		if ipmiData, ok := ipmis[n]; !ok {
			ret = append(ret, n)
			log.Debug("No data for %s, skipping...", n)
			continue
		} else {
			ipmi = NewIPMI(ipmiData.ip, ipmiData.node, ipmiData.password, config.ipmiPath, ipmiData.username)
		}
		switch c {
		case "on":
			err = ipmi.On(dummyMap)
		case "off":
			err = ipmi.Off(dummyMap)
		case "cycle":
			err = ipmi.Cycle(dummyMap)
		case "status":
			err = ipmi.Status(dummyMap)
		case "temp":
			err = ipmi.Temp()
		case "info":
			err = ipmi.Info()
		default:
			usage()
		}
		if err != nil {
			ret = append(ret, n)
			log.Info("Failed to use IPMI for %s, adding to PDU list, if available:", n)
			log.Info(err.Error())
		}
	}
	return ret
}
