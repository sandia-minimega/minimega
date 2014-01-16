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
	"telnet"
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

type PDU interface {
	On(map[string]string) error
	Off(map[string]string) error
	Cycle(map[string]string) error
	Status(map[string]string) error
}

var PDUtypes = map[string]func(string, string, string, string) (PDU, error){
	"tripplite": NewTrippLitePDU,
}

type Device struct {
	name     string
	host     string
	port     string
	pdutype  string
	username string
	password string
	outlets  map[string]string // map hostname -> outlet name
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
	config, err = ReadConfig(*f_config)
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
	for _, n := range nodes {
		for _, d := range config.devices {
			if o, ok := d.outlets[n]; ok {
				if _, ok := ret[d.name]; ok {
					ret[d.name].outlets[n] = o
				} else {
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

type TrippLitePDU struct {
	//	e *expect.Expecter
	username string
	password string
	c        *telnet.Conn
}

func NewTrippLitePDU(host, port, username, password string) (PDU, error) {
	var tp TrippLitePDU
	conn, err := telnet.Dial("tcp", host+":"+port)
	if err != nil {
		return tp, err
	}
	tp.c = conn
	tp.username = username
	tp.password = password
	return tp, err
}

func (p TrippLitePDU) login() error {
	// wait for login prompt
	_, err := p.c.ReadUntil("login: ")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s\r\n", p.username)
	_, err = p.c.Write([]byte(cmd))
	if err != nil {
		return err
	}
	_, err = p.c.ReadUntil("Password: ")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("%s\r\n", p.password)
	_, err = p.c.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

func (p TrippLitePDU) logout() error {
	// send a blank line to make sure we get a prompt
	_, err := p.c.Write([]byte("\r\n"))
	if err != nil {
		return err
	}
	_, err = p.c.ReadUntil("$> ")
	if err != nil {
		return err
	}
	_, err = p.c.Write([]byte("exit\r\n"))
	if err != nil {
		return err
	}
	return nil
}

func (p TrippLitePDU) On(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl on -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

func (p TrippLitePDU) Off(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl off -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

func (p TrippLitePDU) Cycle(ports map[string]string) error {
	p.login()
	for _, port := range ports {
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte(fmt.Sprintf("loadctl cycle -o %s --force\r\n", port)))
		if err != nil {
			return err
		}
	}
	p.logout()
	return nil
}

func (p TrippLitePDU) Status(ports map[string]string) error {
	fmt.Println("not yet implemented")
	return nil
	// doesn't work right
	/*
		p.login()
		_, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		_, err = p.c.Write([]byte("loadctl status -o\r\n"))
		if err != nil {
			return err
		}
		result, err := p.c.ReadUntil("$> ")
		if err != nil {
			return err
		}
		fmt.Println(string(result))
		p.logout()
		return nil
	*/
}
