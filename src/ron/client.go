package ron

import (
	log "minilog"
	"net"
	"strconv"
	"strings"
)

func getNetworkInfo() ([]string, []string) {
	// process network info
	var macs []string
	var ips []string

	ints, err := net.Interfaces()
	if err != nil {
		log.Fatalln(err)
	}
	for _, v := range ints {
		if v.HardwareAddr.String() == "" {
			// skip localhost and other weird interfaces
			continue
		}
		log.Debug("found mac: %v", v.HardwareAddr)
		macs = append(macs, v.HardwareAddr.String())
		addrs, err := v.Addrs()
		if err != nil {
			log.Fatalln(err)
		}
		for _, w := range addrs {
			// trim the cidr from the end
			var ip string
			i := strings.Split(w.String(), "/")
			if len(i) != 2 {
				if !isIPv4(w.String()) {
					log.Error("malformed ip: %v", i, w)
					continue
				}
				ip = w.String()
			} else {
				ip = i[0]
			}
			log.Debug("found ip: %v", ip)
			ips = append(ips, ip)
		}
	}
	return macs, ips
}

func isIPv4(ip string) bool {
	d := strings.Split(ip, ".")
	if len(d) != 4 {
		return false
	}

	for _, v := range d {
		octet, err := strconv.Atoi(v)
		if err != nil {
			return false
		}
		if octet < 0 || octet > 255 {
			return false
		}
	}

	return true
}
