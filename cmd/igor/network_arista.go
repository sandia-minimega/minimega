package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"
)

func init() {
	if networkSetFuncs == nil {
		networkSetFuncs = make(map[string]func([]string, int) error)
		networkClearFuncs = make(map[string]func([]string, int) error)
	}
	networkSetFuncs["arista"] = aristaSet
	networkClearFuncs["arista"] = aristaClear
}

var aristaClearTemplate = `enable
configure terminal
interface {{ $.Eth }}
no switchport access vlan
switchport mode access`

var aristaSetTemplate = `enable
configure terminal
interface {{ $.Eth }}
switchport mode dot1q-tunnel
switchport access vlan {{ $.VLAN }}`

type AristaConfig struct {
	Eth  string
	VLAN int
}

// Issue the given commands via the specified URL, username, and password.
func aristaJSONRPC(user, password, URL string, commands []string) error {
	data, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "runCmds",
		"id":      1,
		"params":  map[string]interface{}{"version": 1, "cmds": commands},
	})
	if err != nil {
		return fmt.Errorf("marshal: %v", err)
	}

	path := fmt.Sprintf("http://%s:%s@%s", user, password, URL)
	resp, err := http.Post(path, "application/json", strings.NewReader(string(data)))
	if err != nil {
		// replace the password with a placeholder so that it doesn't show up
		// in error logs
		msg := strings.Replace(err.Error(), password, "<PASSWORD>", -1)
		return fmt.Errorf("post failed: %v", msg)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readall: %v", err)
	}

	result := make(map[string]interface{})
	err = json.Unmarshal(body, &result)
	if err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}
	return nil
}

func aristaSet(nodes []string, vlan int) error {
	t := template.Must(template.New("set").Parse(aristaSetTemplate))

	for _, n := range nodes {
		var b bytes.Buffer
		eth, ok := igor.NodeMap[n]
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
		// now split b into strings with newlines
		commands := strings.Split(b.String(), "\n")

		err = aristaJSONRPC(igor.NetworkUser, igor.NetworkPassword, igor.NetworkURL, commands)
		if err != nil {
			return err
		}
	}

	return nil
}

func aristaClear(nodes []string, _ int) error {
	t := template.Must(template.New("set").Parse(aristaClearTemplate))

	for _, n := range nodes {
		var b bytes.Buffer
		eth, ok := igor.NodeMap[n]
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
		// now split b into strings with newlines
		commands := strings.Split(b.String(), "\n")

		err = aristaJSONRPC(igor.NetworkUser, igor.NetworkPassword, igor.NetworkURL, commands)
		if err != nil {
			return err
		}
	}

	return nil
}
