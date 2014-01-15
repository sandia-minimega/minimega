package main

import (
	"bufio"
	"errors"
	"expect"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"ranges"
	"strings"
	"time"
)

var (
	f_config = flag.String("config", "", "path to config file")
	config   Config
)

func usage() {
	fmt.Print(`Powerbot: control a PDU.
Usage, <arg> = required, [arg] = optional:
	powerbot on <nodelist>
	powerbot off <nodelist>
	powerbot cycle <nodelist>
	powerbot query [nodelist]

Node lists are in standard range format, i.e. node[1-5,8-10,15]
`)
	log.Fatal("invalid arguments")
}

type PDU interface {
	On(map[string]string) error
	Off(map[string]string) error
	Cycle(map[string]string) error
	Status(map[string]string) error
}

var PDUtypes = map[string]func(string, string) (PDU, error){
	"tripplite": NewTrippLitePDU,
}

type Device struct {
	name    string
	host    string
	port    string
	pdutype string
	outlets map[string]string // map hostname -> outlet name
}

type Config struct {
	devices map[string]Device
	prefix  string // node name prefix, e.g. "ccc" for "ccc[1-100]"
}

func ReadConfig(filename string) (Config, error) {
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
			if len(fields) != 5 {
				continue
			}
			var d Device
			d.name = fields[1]
			d.pdutype = fields[2]
			d.host = fields[3]
			d.port = fields[4]
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
	config, err = ReadConfig(*f_config)
	if err != nil {
		log.Fatal(err.Error())
	}

	// First argument is a command: "on", "off", etc.
	command := args[0]

	/*
		var pdu PDU
		pdu, err = PDUtypes["tripplite"]("pdu:5214")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(pdu)
	*/

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
		pdu, err = PDUtypes[dev.pdutype](dev.host, dev.port)
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

func findOutletsAndDevs(s string) (map[string]Device, error) {
	ret := make(map[string]Device)
	var nodes []string
	var err error

	ranger, _ := ranges.NewRange(config.prefix, 0, 1000000)
	nodes, err = ranger.SplitRange(s)
	if err != nil {
		return ret, err
	}

	fmt.Printf("nodes = %#v\n", nodes)
	fmt.Printf("config.devices = %#v\n", config.devices)

	// This is really gross but you won't have a ton of devices anyway
	// so it should be pretty fast.
	for _, n := range nodes {
		for _, d := range config.devices {
			if o, ok := d.outlets[n]; ok {
				if _, ok := ret[d.name]; ok {
					ret[d.name].outlets[n] = o
				} else {
					var tmp Device
					tmp.outlets = make(map[string]string)
					tmp.name = d.name
					tmp.host = d.host
					tmp.port = d.port
					tmp.pdutype = d.pdutype
					tmp.outlets[n] = o
					ret[tmp.name] = tmp
				}
			}
		}
	}
	fmt.Println(ret)
	return ret, nil
}

type TrippLitePDU struct {
	e *expect.Expecter
}

func NewTrippLitePDU(host string, port string) (PDU, error) {
	var tp TrippLitePDU
	conn, err := net.Dial("tcp", host+":"+port)
	//	sc, err := sshClientConnect(host, port, "localadmin", "localadmin")
	if err != nil {
		return tp, err
	}
	tp.e = expect.NewExpecter(conn)
	tp.e.SetWriter(conn)
	return tp, err
}

func (p TrippLitePDU) On(ports map[string]string) error {
	p.e.Send(string([]byte{255, 251, 1}))
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("waiting for login\n")
	p.e.Expect("login: ")
	fmt.Printf("got login\n")
	p.e.Send("localadmin\r\n")
	time.Sleep(1000 * time.Millisecond)
	p.e.Expect("Password: ")
	fmt.Printf("got password\n")
	p.e.Send("localadmin\r\n")
	time.Sleep(500 * time.Millisecond)
	p.e.Expect("> ")
	for _, port := range ports {
		p.e.Send(fmt.Sprintf("loadctl on -o %s --force\r\n", port))
		time.Sleep(500 * time.Millisecond)
		p.e.Expect("> ")
		fmt.Printf("turned on %s\n", port)
	}
	time.Sleep(500 * time.Millisecond)
	p.e.Send("exit")
	return nil
}

func (p TrippLitePDU) Off(ports map[string]string) error {
	return nil
}

func (p TrippLitePDU) Cycle(ports map[string]string) error {
	return nil
}

func (p TrippLitePDU) Status(ports map[string]string) error {
	return nil
}
