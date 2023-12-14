// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"regexp"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	HEARTBEAT_RATE        = 5
	REAPER_RATE           = 30
	CLIENT_RECONNECT_RATE = 5
	CLIENT_EXPIRED        = 30
	RESPONSE_PATH         = "miniccc_responses"
)

type Process struct {
	PID     int
	Command []string
}

type VM interface {
	GetNamespace() string
	GetUUID() string
	SetCCActive(bool)
	GetTags() map[string]string
	SetTag(string, string)
	Info(string) (string, error)
}

func unmangle(uuid string) string {
	// string must be in the form:
	//	XXXXXXXX-XXXX-XXXX-YYYY-YYYYYYYYYYYY
	// the X characters are reversed at 2 byte intervals (big/little endian for a uuid?)
	var ret string
	re := regexp.MustCompile("[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}")

	u := re.FindString(strings.ToLower(uuid))
	if uuid == "" {
		log.Fatal("uuid failed to match uuid format: %v", uuid)
	}

	ret += u[6:8]
	ret += u[4:6]
	ret += u[2:4]
	ret += u[:2]
	ret += "-"
	ret += u[11:13]
	ret += u[9:11]
	ret += "-"
	ret += u[16:18]
	ret += u[14:16]
	ret += u[18:]

	log.Debug("mangled/unmangled uuid: %v %v", u, ret)
	return ret
}
