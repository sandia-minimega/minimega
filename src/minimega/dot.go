// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"strings"
)

type dotVM struct {
	Vlans []string
	State string
	Text  string
}

// dot returns a graphviz 'dotfile' string representing the experiment topology
// from the perspective of this node.
func cliDot(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "viz takes one argument",
		}
	}
	command := makeCommand("vm_info [host,name,id,state,ip,ip6,vlan]")
	localInfo := vms.info(command)
	remoteInfo := meshageBroadcast(command)

	// any errors?
	if localInfo.Error != "" {
		return cliResponse{
			Error: localInfo.Error,
		}
	}
	if remoteInfo.Error != "" {
		return cliResponse{
			Error: remoteInfo.Error,
		}
	}

	// build data
	// ditch the first 'header' line
	var expVms []*dotVM
	lines := strings.Split(localInfo.Response, "\n")
	log.Debug("dot local vms: %v", len(lines))
	if len(lines) >= 2 {
		e := dotProcessInfo(lines)
		if len(e) > 0 {
			expVms = append(expVms, e...)
		}
	}
	lines = strings.Split(remoteInfo.Response, "\n")
	log.Debug("dot remote vms: %v", len(lines))
	if len(lines) >= 2 {
		e := dotProcessInfo(lines)
		if len(e) > 0 {
			expVms = append(expVms, e...)
		}
	}

	var ret string
	vlans := make(map[string]bool)

	ret = "graph minimega {\n"
	ret += "size=\"8,11\";\n"
	//ret += fmt.Sprintf("Legend [shape=box, shape=plaintext, label=\"total=%d\"];\n", len(n.effectiveNetwork))

	for _, v := range expVms {
		var color string
		switch v.State {
		case "building":
			color = "yellow"
		case "running":
			color = "green"
		case "paused":
			color = "yellow"
		case "quit":
			color = "blue"
		case "error":
			color = "red"
		}

		ret += fmt.Sprintf("%s [style=filled, color=%s];\n", v.Text, color)
		for _, c := range v.Vlans {
			ret += fmt.Sprintf("%s -- %s\n", v.Text, c)
			vlans[c] = true
		}
		ret += "\n"
	}

	for k, _ := range vlans {
		ret += fmt.Sprintf("%s;\n\n", k)
	}

	ret += "}"

	err := ioutil.WriteFile(c.Args[0], []byte(ret), 0664)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	return cliResponse{}

}

func dotProcessInfo(info []string) []*dotVM {
	log.Debugln("dotProcessInfo: ", info)
	// ditch the first line
	var ret []*dotVM
	info = info[1:]
	for _, v := range info {
		f := strings.Split(v, "|")
		for i, w := range f {
			f[i] = strings.TrimSpace(w)
		}
		log.Debugln(f)
		// host name id state ip ip6 vlan
		if len(f) != 7 {
			continue
		}
		vlans := strings.Split(f[6][1:len(f[6])-1], " ")
		log.Debugln(vlans)
		ret = append(ret, &dotVM{
			Vlans: vlans,
			State: f[3],
			Text:  fmt.Sprintf("\"%v:%v:%v:%v:%v\"", f[0], f[1], f[2], f[4], f[5]),
		})
	}
	return ret
}
