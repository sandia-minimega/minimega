package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"text/template"
)

func init() {
	if networkSetFuncs == nil {
		networkSetFuncs = make(map[string]func([]string, int) error)
		networkClearFuncs = make(map[string]func([]string) error)
	}
	networkSetFuncs["arista"] = aristaSet
	networkClearFuncs["arista"] = aristaClear
}

var aristaClearTemplate = `
enable
conf t
int {{ $.Eth }}
no switchport access vlan
switchport mode access
`

var aristaSetTemplate = `
enable
conf t
int {{ $.Eth }}
switchport mode dot1qtunnel
switchport access vlan {{ $.VLAN }}
`

type AristaConfig struct {
	Eth  string
	VLAN int
}

func aristaSet(nodes []string, vlan int) error {
	t := template.Must(template.New("set").Parse(aristaSetTemplate))

	for _, n := range nodes {
		var b bytes.Buffer
		eth, ok := igorConfig.NodeMap[n]
		if !ok {
			return fmt.Errorf("no such node: %v", n)
		}

		c := &AristaConfig{
			Eth:  eth,
			VLAN: vlan,
		}
		err := t.Execute(&b, c)
		if err != nil {
			return err
		}

		cmd := exec.Command("ssh", "-T", igorConfig.NetworkHost)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		go func() {
			defer stdin.Close()
			io.Copy(stdin, &b)
		}()

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("configuring network: %v %s", err, out)
		}
	}

	return nil
}

func aristaClear(nodes []string) error {
	t := template.Must(template.New("set").Parse(aristaClearTemplate))

	for _, n := range nodes {
		var b bytes.Buffer
		eth, ok := igorConfig.NodeMap[n]
		if !ok {
			return fmt.Errorf("no such node: %v", n)
		}

		c := &AristaConfig{
			Eth: eth,
		}
		err := t.Execute(&b, c)
		if err != nil {
			return err
		}

		cmd := exec.Command("ssh", "-T", igorConfig.NetworkHost)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		go func() {
			defer stdin.Close()
			io.Copy(stdin, &b)
		}()

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("configuring network: %v %s", err, out)
		}
	}

	return nil
}
