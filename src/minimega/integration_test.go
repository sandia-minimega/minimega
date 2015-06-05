// +build integration

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"minicli"
	"os"
	"path"
	"strings"
	"testing"
)

var f_preamble = flag.String("preamble", "", "path to file containing minimega commands to run on startup")
var f_testDir = flag.String("dir", "tests", "path to directory containing tests")

// runCommands reads and runs all the commands from a file. Return the
// concatenation of all the Responses or an error.
func runCommands(t *testing.T, mm *MinimegaConn, file string) (string, error) {
	var res string
	var err error

	f, err := os.Open(file)
	if err != nil {
		return "", err
	}

	s := bufio.NewScanner(f)

	for s.Scan() {
		cmd, err := minicli.CompileCommand(s.Text())
		if err != nil {
			return "", fmt.Errorf("unable to compile `%v` -- %v", s.Text(), err)
		}

		for resps := range mm.runCommand(cmd) {
			if err != nil {
				continue
			}

			res += fmt.Sprintf("## %v\n", cmd.Original)

			for _, resp := range resps.Resp {
				if resp.Error != "" {
					res += fmt.Sprintf("E: %v\n", resp.Error)
				}
			}

			if len(resps.Rendered) > 0 {
				res += resps.Rendered + "\n"
			}
		}
	}

	if err := s.Err(); err != nil {
		return "", err
	}

	return res, nil
}

func TestExpected(t *testing.T) {
	mm, err := DialMinimega()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if *f_preamble != "" {
		out, err := runCommands(t, mm, *f_preamble)
		if err != nil {
			t.Fatalf("%v", err)
		}

		t.Log(out)
	}

	// TODO: Should we quit minimega and restart it between each test?
	//quit := mustCompile(t, "quit 2")

	files, err := ioutil.ReadDir(*f_testDir)
	if err != nil {
		t.Fatalf("%v", err)
	}

	for _, info := range files {
		if strings.HasSuffix(info.Name(), ".want") || strings.HasSuffix(info.Name(), ".got") {
			continue
		}

		t.Logf("Running commands from %s", info.Name())
		fpath := path.Join(*f_testDir, info.Name())

		got, err := runCommands(t, mm, fpath)
		if err != nil {
			t.Fatalf("%v", err)
		}

		// Record the output for offline comparison
		if err := ioutil.WriteFile(fpath+".got", []byte(got), os.FileMode(0644)); err != nil {
			t.Errorf("unable to write `%s` -- %v", fpath+".got", err)
		}

		want, err := ioutil.ReadFile(fpath + ".want")
		if err != nil {
			t.Errorf("unable to read file `%s` -- %v", fpath+".want", err)
			continue
		}

		if got != string(want) {
			t.Errorf("got != want for %s", info.Name())
		}

		//mm.runCommand(quit)
	}
}
