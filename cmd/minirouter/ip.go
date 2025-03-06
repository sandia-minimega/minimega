package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	INTERFACE_STATS_PERIOD = time.Second
)

var (
	IPs = make(map[int][]string)
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"ip <add,> <index or lo> <ip>",
			"ip <flush,>",
		},
		Call: handleIP,
	})
	go interfaceStats()
}

func interfaceStats() {
	if *f_miniccc == "" {
		return
	}
	for {
		dirs, err := ioutil.ReadDir("/sys/class/net")
		if err != nil {
			log.Errorln(err)
			return
		}

		for _, n := range dirs {
			f := n.Name()
			if f == "lo" {
				continue
			}
			rx, err := ioutil.ReadFile(filepath.Join("/sys/class/net", f, "statistics/rx_bytes"))
			if err != nil {
				log.Errorln(err)
				continue
			}
			tx, err := ioutil.ReadFile(filepath.Join("/sys/class/net", f, "statistics/tx_bytes"))
			if err != nil {
				log.Errorln(err)
				continue
			}
			err = tag("minirouter_"+f+"_rx_bytes", string(rx))
			if err != nil {
				log.Errorln(err)
				continue
			}
			err = tag("minirouter_"+f+"_tx_bytes", string(tx))
			if err != nil {
				log.Errorln(err)
				continue
			}
		}
		time.Sleep(INTERFACE_STATS_PERIOD)
	}
}

func handleIP(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()
	if c.BoolArgs["flush"] {
		for idx, iplist := range IPs {
			for i := len(iplist) - 1; i > -1; i-- {
				log.Debug("deleting ip: %v", iplist[i])
				err := ipDel(idx, iplist[i])
				if err != nil {
					log.Errorln(err)
				}
			}
		}
	} else if c.BoolArgs["add"] {
		var idx int
		var err error
		if c.StringArgs["index"] != "lo" {
			idx, err = strconv.Atoi(c.StringArgs["index"])
			if err != nil {
				log.Errorln(err)
				return
			}
		} else {
			idx = -1
		}
		ip := c.StringArgs["ip"]
		err = ipAdd(idx, ip)
		if err != nil {
			log.Errorln(err)
			return
		}
	}

	log.Debug("IPs: %v", IPs)
}

func ipDel(idx int, ip string) error {
	iface := "lo"
	if idx != -1 {
		// get an interface from the index
		var err error
		iface, err = findEth(idx)
		if err != nil {
			return err
		}
	}

	// TODO: Need to send release ip reservation to dhcp server
	if ip == "dhcp" {
		out, err := exec.Command("dhclient", "-r", iface).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %v", err, string(out))
		}
		return nil
	}
	out, err := exec.Command("ip", "addr", "del", ip, "dev", iface).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	IPs[idx] = IPs[idx][:len(IPs[idx])-1]
	if len(IPs[idx]) == 0 {
		delete(IPs, idx)
	}
	return nil
}

func ipAdd(idx int, ip string) error {
	iface := "lo"
	if idx != -1 {
		// get an interface from the index
		var err error
		iface, err = findEth(idx)
		if err != nil {
			return err
		}
	}

	out, err := exec.Command("ip", "link", "set", iface, "up").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	if ip == "dhcp" {
		if iface == "lo" {
			return errors.New("Cannot put dhcp on Loopback interface")
		}
		return exec.Command("dhclient", iface).Run()
	}

	out, err = exec.Command("ip", "addr", "add", ip, "dev", iface).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	IPs[idx] = append(IPs[idx], ip)
	return nil
}
