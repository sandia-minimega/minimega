// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"minicli"
	"miniclient"
	log "minilog"
	"os/exec"
	"strings"
	"time"
)

const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

const maxRetries = 10

var (
	f_minimega = flag.String("minimega", "bin/minimega", "minimega binary")
	f_flags    = flag.String("flags", "", "set flags for minimega binary")
	f_base     = flag.String("base", "/tmp/minimega", "base path for minimega data (should match -flags)")
	f_exclude  = flag.String("exclude", "quit,read,write,deploy,nuke,shell", "commands to skip")
	f_chars    = flag.String("chars", chars, "chars to use when generating strings for commands")
	f_values   = flag.String("values", "foo,bar,test,0,1,2,3,all", "fixed values to use when generating commands")
)

var (
	handlers []*minicli.Handler
	exclude  []string
	values   []string
)

// dial spins while trying to dial minimega over and over. Will try at most
// retries times.
func dial(retries int) (*miniclient.Conn, error) {
	for i := 0; i < retries; i++ {
		mm, err := miniclient.Dial(*f_base)
		if err == nil {
			return mm, nil
		}

		log.Debug("unable to dial: %v", err)
		time.Sleep(time.Second)
	}

	return nil, errors.New("max retries exceeded")
}

// run a command against a live minimega instance, logging the command and the
// response for inspection.
func run(mm *miniclient.Conn, cmd string) {
	log.Info("minimega: `%v`", cmd)

	var res string

	for resps := range mm.Run(cmd) {
		for _, resp := range resps.Resp {
			if resp.Error != "" {
				res += fmt.Sprintf("E: %v\n", resp.Error)
			}
		}

		if len(resps.Rendered) > 0 {
			res += resps.Rendered + "\n"
		}
	}

	log.Debug(res)
}

// genCmd generates a new command string matching one of the patterns in the
// provided handler.
func genCmd(handler *minicli.Handler) string {
	// Pick a pattern at random
	i := rand.Int() % len(handler.PatternItems)

	log.Debug("generating pattern for `%v`", handler.Patterns[i])

	cmd := []string{}

	for _, item := range handler.PatternItems[i] {
		if item.IsLiteral() {
			cmd = append(cmd, item.Text)
			continue
		}

		// "flip" coin for whether to include the optional field or not
		if item.IsOptional() && rand.Int()%4 == 0 {
			continue
		}

		if item.IsString() {
			// "flip" coin for whether to use random fixed value or not
			if rand.Int()%4 == 0 {
				i := rand.Int() % len(values)

				cmd = append(cmd, values[i])
				continue
			}

			// generate a random string of length [1, 32]
			res := []byte{}

			for i := rand.Uint32()%32 + 1; i > 0; i-- {
				res = append(res, chars[rand.Int()%len(chars)])
			}

			cmd = append(cmd, string(res))
		} else if item.IsChoice() {
			i := rand.Int() % len(item.Options)

			cmd = append(cmd, item.Options[i])
		} else if item.IsCommand() {
			// dat recursion
			i := rand.Int() % len(handlers)

			cmd = append(cmd, genCmd(handlers[i]))
		}
	}

	return strings.Join(cmd, " ")
}

// fuzz starts and connects to minimega and then generates commands until
// minimega dies (which will hopefully be a long time).
func fuzz() error {
	args := strings.Fields(*f_flags)
	log.Info("exec: `%v %v`", *f_minimega, args)
	cmd := exec.Command(*f_minimega, args...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("unable to start: %v", err)
	}

	mm, err := dial(maxRetries)
	if err != nil {
		return fmt.Errorf("unable to dial: %v", err)
	}
	defer mm.Close()

	stop := make(chan bool)

	go func() {
		defer close(stop)

		if err := cmd.Wait(); err != nil {
			log.Error("minimega crashed: %v", err)
		}
	}()

outerLoop:
	for {
		select {
		case <-stop:
			return nil
		default:
			// pick a random handler
			i := rand.Int() % len(handlers)

			cmd := genCmd(handlers[i])

			// make sure that we didn't generate an excluded command
			for _, exclude := range exclude {
				if strings.Contains(cmd, exclude) {
					continue outerLoop
				}
			}

			run(mm, cmd)
		}
	}
}

// cleanup attempts to clean up minimega after a crash using the `-force` flag
// and `nuke`.
func cleanup() error {
	args := strings.Fields(*f_flags)
	args = append(args, "-force")
	log.Info("exec: `%v %v`", *f_minimega, args)
	cmd := exec.Command(*f_minimega, args...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("unable to start: %v", err)
	}

	mm, err := dial(maxRetries)
	if err != nil {
		return fmt.Errorf("unable to dial: %v", err)
	}
	defer mm.Close()

	run(mm, "nuke")

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("wait error: %v", err)
	}

	return nil
}

func main() {
	flag.Parse()

	log.Init()

	log.Debug("using minimega: %v", *f_minimega)

	// invoke minimega and get the doc json
	doc, err := exec.Command(*f_minimega, "-cli").Output()
	if err != nil {
		log.Fatalln(err)
	}
	log.Debug("got doc: %v", string(doc))

	// decode the JSON for our template
	if err := json.Unmarshal(doc, &handlers); err != nil {
		log.Fatalln(err)
	}

	exclude = strings.Split(*f_exclude, ",")
	values = strings.Split(*f_values, ",")

	for {
		if err := fuzz(); err != nil {
			log.Fatal("fuzz: %v", err)
		}
		if err := cleanup(); err != nil {
			log.Fatal("cleanup: %v", err)
		}
	}
}
