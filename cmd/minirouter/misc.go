package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"sort"
	"strconv"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var (
	trim = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

type SortEth []string

func (a SortEth) Len() int      { return len(a) }
func (a SortEth) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortEth) Less(i, j int) bool {
	// sort on the integer part of an interface name, which we assume will
	// be in the form of string+int
	idxI, err := strconv.Atoi(strings.TrimLeft(a[i], trim))
	if err != nil {
		log.Errorln(err)
		return false
	}
	idxJ, err := strconv.Atoi(strings.TrimLeft(a[j], trim))
	if err != nil {
		log.Errorln(err)
		return false
	}
	return idxI < idxJ
}

func findEth(idx int) (string, error) {
	var ethNames SortEth
	dirs, err := ioutil.ReadDir("/sys/class/net")
	if err != nil {
		return "", err
	} else {
		for _, n := range dirs {
			if n.Name() == "lo" {
				continue
			}
			ethNames = append(ethNames, n.Name())
		}
	}
	sort.Sort(ethNames)
	if idx < len(ethNames) {
		return ethNames[idx], nil
	}
	return "", fmt.Errorf("no such network")
}

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func isIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}
