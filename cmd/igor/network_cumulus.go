package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func init() {
	if networkSetFuncs == nil {
		networkSetFuncs = make(map[string]func([]string, int) error)
		networkClearFuncs = make(map[string]func([]string, int) error)
	}
	networkSetFuncs["cumulus"] = cumulusSet
	networkClearFuncs["cumulus"] = cumulusClear
}

var cumulusSetTemplate = `add interface {{ $.Eth }} bridge access {{ $.VLAN }}
add bridge {{ $.Bridge }} ports {{ $.Eth }}
add bridge {{ $.Bridge }} vids {{ $.VLAN }}
add bridge {{ $.Bridge }} vlan-protocol 802.1ad
commit`

var cumulusClearTemplate = `del interface {{ $.Eth }} bridge access {{ $.VLAN }}
del bridge {{ $.Bridge }} ports {{ $.Eth }}
del bridge {{ $.Bridge }} vids {{ $.VLAN }}
commit`

type CumulusConfig struct {
	Bridge string
	Eth    string
	VLAN   int
}

func cumulusSet(nodes []string, vlan int) error {
	t := template.Must(template.New("set").Parse(cumulusSetTemplate))

	for _, n := range nodes {
		var b bytes.Buffer

		eth, ok := igor.NodeMap[n]
		if !ok {
			return fmt.Errorf("no such node: %v", n)
		}

		// default to bridge name "bridge"
		bridge := "bridge"

		if br, ok := igor.Advanced["cumulus-bridge"]; ok {
			bridge = br.(string)
		}

		c := &CumulusConfig{
			Bridge: bridge,
			Eth:    eth,
			VLAN:   vlan,
		}

		if err := t.Execute(&b, c); err != nil {
			return err
		}

		commands := strings.Split(b.String(), "\n")

		if err := cumulusUpdate(commands); err != nil {
			return err
		}
	}

	return nil
}

func cumulusClear(nodes []string, vlan int) error {
	t := template.Must(template.New("set").Parse(cumulusClearTemplate))

	for _, n := range nodes {
		var b bytes.Buffer

		eth, ok := igor.NodeMap[n]
		if !ok {
			return fmt.Errorf("no such node: %v", n)
		}

		// default to bridge name "bridge"
		bridge := "bridge"

		if br, ok := igor.Advanced["cumulus-bridge"]; ok {
			bridge = br.(string)
		}

		c := &CumulusConfig{
			Bridge: bridge,
			Eth:    eth,
			VLAN:   vlan,
		}

		if err := t.Execute(&b, c); err != nil {
			return err
		}

		commands := strings.Split(b.String(), "\n")

		if err := cumulusUpdate(commands); err != nil {
			return err
		}
	}

	return nil
}

// Issue the given command via the specified URL and API key.
func cumulusUpdate(commands []string) (ferr error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{Transport: transport}

	// anonymous function to send abort command to switch if any errors occurred
	// (to ensure any pending commands get cleared)
	defer func() {
		if ferr == nil {
			return
		}

		log.Error("[Cumulus] Sending abort command to switch due to error")

		data, _ := json.Marshal(map[string]interface{}{"cmd": "abort"})
		req, _ := http.NewRequest("POST", igor.NetworkURL, strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(igor.NetworkUser, igor.NetworkPassword)

		client.Do(req)
	}()

	// local function to cleanly handle closing HTTP response body
	do := func(req *http.Request) error {
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("[Cumulus] HTTP POST failed: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("readall: %v", err)
			}

			return fmt.Errorf("[Cumulus] HTTP POST error response (%d): %s", resp.StatusCode, string(body))
		}

		return nil
	}

	for _, command := range commands {
		log.Info("[Cumulus] Sending command via HTTP: %s", command)

		data, err := json.Marshal(map[string]interface{}{"cmd": command})
		if err != nil {
			return fmt.Errorf("marshal: %v", err)
		}

		req, _ := http.NewRequest("POST", igor.NetworkURL, strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(igor.NetworkUser, igor.NetworkPassword)

		if err := do(req); err != nil {
			return err
		}
	}

	return nil
}
