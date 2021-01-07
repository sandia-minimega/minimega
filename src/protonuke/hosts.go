// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"net"
	"regexp"
	"strings"
)

// Based on http://play.golang.org/p/m8TNTtygK0
func enumerateCIDR(s string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}

	var broadcast net.IP
	for i := range ipnet.IP {
		broadcast = append(broadcast, ipnet.IP[i]|^ipnet.Mask[i])
	}

	res := []string{}

	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		if !ip.Equal(ipnet.IP) && !ip.Equal(broadcast) {
			res = append(res, ip.String())
		}
	}

	return res, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// parse hosts from the command line and return as a map[string]string, where
// the key is the hostname/ip we'll actually target, and the value is the
// parameter provided by the user. This way, if the user provides something
// like 10.0.0.0/24, we can populate the map with 254 keys, all with the same
// value, so we can pretty print things related to the user's input.
func parseHosts(input []string) (map[string]string, error) {
	log.Debugln("parseHosts")
	res := make(map[string]string)

	for _, i := range input {
		// input can be either a hostname/ip, a subnet, or a comma separated list of the two
		log.Debugln("parsing ", i)

		if strings.Contains(i, ",") { // recursion on comma lists
			d := strings.Split(i, ",")
			log.Debugln("comma delimited: ", d)
			hosts, err := parseHosts(d)
			if err != nil {
				return nil, err
			}

			for k, v := range hosts {
				res[k] = v
			}
		} else if strings.Contains(i, "/") { // a subnet
			ips, err := enumerateCIDR(i)
			if err != nil {
				return nil, err
			}

			for _, ip := range ips {
				res[ip] = ip
			}
		} else if ip := net.ParseIP(i); ip != nil { // IPv4 or IPv6
			res[ip.String()] = ip.String()
		} else if isValidDNS(i) { // host
			res[i] = i
		} else {
			return nil, fmt.Errorf("invalid host or ip %v", i)
		}
	}

	return res, nil
}

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func isIPv6(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

func isValidDNS(host string) bool {
	// rfc 1123
	expr := `^[[:alnum:]]+[[:alnum:].-]*$`
	matched, err := regexp.MatchString(expr, host)
	if err != nil {
		log.Errorln(err)
		return false
	}
	return matched
}
