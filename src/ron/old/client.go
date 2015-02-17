package ron

import (
	log "minilog"
	"net"
	"strings"
)

func getNetworkInfo() ([]string, []string) {
	// process network info
	var macs []string
	var ips []string

	ints, err := net.Interfaces()
	if err != nil {
		log.Errorln(err)
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
