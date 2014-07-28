// +build windows

package ron

import (
	"bytes"
	"fmt"
	log "minilog"
	"os/exec"
	"regexp"
	"strings"
)

func getUUID() (string, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer

	p, err := exec.LookPath("wmic")
	if err != nil {
		return "", fmt.Errorf("wmic path: %v", err)
	}

	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"path",
			"win32_computersystemproduct",
			"get",
			"uuid",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("wmic run: %v", err)
	}

	uuid := strings.TrimSpace(sOut.String())
	uuid = unmangleUUID(uuid)
	if sErr.String() != "" {
		return "", fmt.Errorf("wmic failed: %v %v", sOut.String(), sErr.String())
	}

	log.Debug("got UUID: %v", uuid)
	return uuid, nil
}

func unmangleUUID(uuid string) string {
	// string must be in the form:
	//	XXXXXXXX-XXXX-XXXX-YYYY-YYYYYYYYYYYY
	// the X characters are reversed at 2 byte intervals (big/little endian for a uuid?)
	var ret string
	uuid = strings.ToLower(uuid)
	re := regexp.MustCompile("[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}")

	u := re.FindString(uuid)
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
