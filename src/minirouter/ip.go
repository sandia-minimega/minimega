package main

import (
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	INTERFACE_STATS_PERIOD = time.Second
)

var (
	IPs [][]string
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"ip <add,> <index> <ip>",
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
		ips := make([][]string, len(IPs))
		for i, v := range IPs {
			ips[i] = make([]string, len(v))
			copy(ips[i], v)
		}
		for i, v := range ips {
			for _, ip := range v {
				log.Debug("deleting ip: %v", ip)
				err := ipDel(i, ip)
				if err != nil {
					log.Errorln(err)
				}
			}
		}
	} else if c.BoolArgs["add"] {
		idx, err := strconv.Atoi(c.StringArgs["index"])
		if err != nil {
			log.Errorln(err)
			return
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
	if idx >= len(IPs) {
		return fmt.Errorf("invalid index: %v", idx)
	}

	var found bool
	for _, v := range IPs[idx] {
		if ip == v {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such ip: %v", ip)
	}

	// get an interface from the index
	eth, err := findEth(idx)
	if err != nil {
		return err
	}

	// TODO: what's the right way to remove a dhcp interface?
	if ip == "dhcp" {
		return nil
	}

	out, err := exec.Command("ip", "addr", "del", ip, "dev", eth).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	for i, v := range IPs[idx] {
		if ip == v {
			IPs[idx] = append(IPs[idx][:i], IPs[idx][i+1:]...)
			break
		}
	}

	return nil
}

func ipAdd(idx int, ip string) error {
	// get an interface from the index
	eth, err := findEth(idx)
	if err != nil {
		return err
	}

	out, err := exec.Command("ip", "link", "set", eth, "up").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	if ip == "dhcp" {
		return exec.Command("dhclient", eth).Run()
	}

	out, err = exec.Command("ip", "addr", "add", ip, "dev", eth).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", err, string(out))
	}

	for idx >= len(IPs) {
		IPs = append(IPs, []string{})
	}
	IPs[idx] = append(IPs[idx], ip)

	return nil
}
