package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"ranges"
	"strings"
)

var (
	f_config = flag.String("config", "/etc/powerbot.conf", "path to config file")
	config   Config
)

func usage() {
	fmt.Print(`Powerbot: control a PDU.
Usage, <arg> = required, [arg] = optional:
	powerbot on <nodelist>
	powerbot off <nodelist>
	powerbot cycle <nodelist>

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

// This gets read from the config file
type Config struct {
	devices map[string]Device
	prefix  string // node name prefix, e.g. "ccc" for "ccc[1-100]"
}

// Parse the config file and store it in the global config
func readConfig(filename string) (Config, error) {
	var ret Config
	ret.devices = make(map[string]Device)

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
			for _, v := range ret.devices {
				if v.name == d.name {
					continue
				}
			}
			ret.devices[d.name] = d
		case "node":
			if len(fields) != 4 {
				continue
			}
			nodename := fields[1]
			dev := fields[2]
			outlet := fields[3]
			if _, ok := ret.devices[dev]; ok {
				ret.devices[dev].outlets[nodename] = outlet
			}
		}
	}
	return ret, nil
}

func main() {
	var err error

	// Get flags and arguments
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		usage()
	}

	// Parse configuration file
	config, err = readConfig(*f_config)
	if err != nil {
		log.Fatal(err.Error())
	}

	// First argument is a command: "on", "off", etc.
	command := args[0]

	// Find a list of what devices and ports are affected
	// by the command
	var nodes string
	devs := make(map[string]Device)
	if len(args) == 2 {
		nodes = args[1]
		devs, err = findOutletsAndDevs(nodes)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	// For each device affected, perform the command
	for _, dev := range devs {
		var pdu PDU
		pdu, err = PDUtypes[dev.pdutype](dev.host, dev.port, dev.username, dev.password)
		if err != nil {
			log.Print(err)
			continue
		}
		switch command {
		case "on":
			pdu.On(dev.outlets)
		case "off":
			pdu.Off(dev.outlets)
		case "cycle":
			pdu.Cycle(dev.outlets)
		case "status":
			pdu.Status(dev.outlets)
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
func findOutletsAndDevs(s string) (map[string]Device, error) {
	ret := make(map[string]Device)
	var nodes []string
	var err error

	ranger, _ := ranges.NewRange(config.prefix, 0, 1000000)
	nodes, err = ranger.SplitRange(s)
	if err != nil {
		return ret, err
	}

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
