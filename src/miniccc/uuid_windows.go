// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build windows

package main

import (
	log "minilog"
	"os/exec"
	"regexp"
	"strings"
)

func getUUID() string {
	out, err := exec.Command("wmic", "path", "win32_computersystemproduct", "get", "uuid").CombinedOutput()
	if err != nil {
		log.Fatal("wmic run: %v", err)
	}

	uuid := unmangleUUID(strings.TrimSpace(string(out)))
	log.Debug("got UUID: %v", uuid)

	return uuid
}

func unmangleUUID(uuid string) string {
	// string must be in the form:
	//	XXXXXXXX-XXXX-XXXX-YYYY-YYYYYYYYYYYY
	// the X characters are reversed at 2 byte intervals (big/little endian for a uuid?)
	var ret string
	re := regexp.MustCompile("[0-9a-z]{8}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{12}")

	u := re.FindString(strings.ToLower(uuid))
	if uuid == "" {
		log.Fatal("uuid failed to match uuid format: %v", uuid)
	}

	log.Debug("found uuid: %v", u)

	if getOSVer() != "Windows XP" {
		return u
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

func getOSVer() string {
	var fullVersion string

	//Get CurrentVersion
	cmd := exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "CurrentVersion")
	cvBytes, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnln("failed reg query: CurrentVersion")
	}
	cvStr := strings.Split(string(cvBytes), "    ")
	currentVersion := strings.TrimSpace(cvStr[len(cvStr)-1])

	//Get CurrentBuild
	cmd = exec.Command("reg", "query",
		"HKLM\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
		"/v", "CurrentBuild")

	cbBytes, err := cmd.CombinedOutput()

	if err != nil {
		log.Warnln("failed reg query: CurrentBuild")
		fullVersion = currentVersion
	} else {
		cbStr := strings.Split(string(cbBytes), "    ")
		currentBuild := strings.TrimSpace(cbStr[len(cbStr)-1])

		fullVersion = currentVersion + "." + currentBuild
	}

	switch fullVersion {
	case "1.04":
		return "Windows 1.0"
	case "2.11":
		return "Windows 2.0"
	case "3":
		return "Windows 3.0"
	case "3.11":
		return "Windows for Workgroups 3.11"
	case "2250":
		return "Whistler Server"
	case "2257":
		return "Whistler Server"
	case "2267":
		return "Whistler Server"
	case "2410":
		return "Whistler Server"
	case "3.10.528":
		return "Windows NT 3.1"
	case "3.5.807":
		return "Windows NT Workstation 3.5"
	case "3.51.1057":
		return "Windows NT Workstation 3.51"
	case "4.0.1381":
		return "Windows Workstation 4.0"
	case "4.0.950":
		return "Windows 95"
	case "4.00.950":
		return "Windows 95"
	case "4.00.1111":
		return "Windows 95"
	case "4.03.1212-1214":
		return "Windows 95"
	case "4.03.1214":
		return "Windows 95"
	case "4.1.1998":
		return "Windows 98"
	case "4.1.2222":
		return "Windows 98"
	case "4.90.2476":
		return "Windows Millenium"
	case "4.90.3000":
		return "Windows Me"
	case "5.00.1515":
		return "Windows NT 5.00"
	case "5.00.2031":
		return "Windows 2000"
	case "5.00.2128":
		return "Windows 2000"
	case "5.00.2183":
		return "Windows 2000"
	case "5.00.2195":
		return "Windows 2000"
	case "5.0.2195":
		return "Windows 2000"
	case "5.1.2505":
		return "Windows XP"
	case "5.1.2600":
		return "Windows XP"
	case "5.2.3790":
		return "Windows XP"
		//      Conflicts with Windows XP.
		//	case "5.2.3790": return "Windows Home Server"
		//	case "5.2.3790": return "Windows Server 2003"
	case "5.2.3541":
		return "Windows .NET Server"
	case "5.2.3590":
		return "Windows .NET Server"
	case "5.2.3660":
		return "Windows .NET Server"
	case "5.2.3718":
		return "Windows .NET Server 2003"
	case "5.2.3763":
		return "Windows Server 2003"
	case "6.0.5048":
		return "Windows Longhorn"
	case "6.0.5112":
		return "Windows Vista"
	case "6.0.5219":
		return "Windows Vista"
	case "6.0.5259":
		return "Windows Vista"
	case "6.0.5270":
		return "Windows Vista"
	case "6.0.5308":
		return "Windows Vista"
	case "6.0.5342":
		return "Windows Vista"
	case "6.0.5381":
		return "Windows Vista"
	case "6.0.5384":
		return "Windows Vista"
	case "6.0.5456":
		return "Windows Vista"
	case "6.0.5472":
		return "Windows Vista"
	case "6.0.5536":
		return "Windows Vista"
	case "6.0.5600":
		return "Windows Vista"
	case "6.0.5700":
		return "Windows Vista"
	case "6.0.5728":
		return "Windows Vista"
	case "6.0.5744":
		return "Windows Vista"
	case "6.0.5808":
		return "Windows Vista"
	case "6.0.5824":
		return "Windows Vista"
	case "6.0.5840":
		return "Windows Vista"
	case "6.0.6000":
		return "Windows Vista"
	case "6.0.6001":
		return "Windows Server 2008"
	case "6.0.6002":
		return "Windows Vista"
	case "6.1.7600":
		return "Windows 7"
		//      Conflicts with Windows 7.  Need more granularity (.16385)
		//	case "6.1.7600": return "Windows Server 2008 R2, RTM (Release to Manufacturing)"
	case "6.1.7601":
		return "Windows 7"
	case "6.2.9200":
		return "Windows 8"
		//	Conflicts with Windows 8.  Not sure how to tell these apart
		//	case "6.2.9200": return "Windows Server 2012"
	case "6.2.8102":
		return "Windows Server 2012"
	case "6.3.9600":
		return "Windows 8.1"
	}
	return "unknown"
}
