// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/minipager"

	"github.com/peterh/liner"
)

type commands struct {
	in  []*minicli.Command
	out chan minicli.Responses
}

// Prevents multiple commands from running at the same time
var cmdChannel chan commands

type wrappedCLIFunc func(*Namespace, *minicli.Command, *minicli.Response) error
type wrappedSuggestFunc func(*Namespace, string, string) []string

// cliSetup registers all the minimega handlers
func cliSetup() {
	minicli.Preprocessor = cliPreprocessor

	registerHandlers("bridge", bridgeCLIHandlers)
	registerHandlers("capture", captureCLIHandlers)
	registerHandlers("cc", ccCLIHandlers)
	registerHandlers("deploy", deployCLIHandlers)
	registerHandlers("disk", diskCLIHandlers)
	registerHandlers("dnsmasq", dnsmasqCLIHandlers)
	registerHandlers("dot", dotCLIHandlers)
	registerHandlers("external", externalCLIHandlers)
	registerHandlers("history", historyCLIHandlers)
	registerHandlers("host", hostCLIHandlers)
	registerHandlers("io", ioCLIHandlers)
	registerHandlers("log", logCLIHandlers)
	registerHandlers("meshage", meshageCLIHandlers)
	registerHandlers("misc", miscCLIHandlers)
	registerHandlers("namespace", namespaceCLIHandlers)
	registerHandlers("nuke", nukeCLIHandlers)
	registerHandlers("optimize", optimizeCLIHandlers)
	registerHandlers("qos", qosCLIHandlers)
	registerHandlers("router", routerCLIHandlers)
	registerHandlers("shell", shellCLIHandlers)
	registerHandlers("vlans", vlansCLIHandlers)
	registerHandlers("vm", vmCLIHandlers)
	registerHandlers("vmconfig", vmconfigCLIHandlers)
	registerHandlers("vmconfiger", vmconfigerCLIHandlers)
	registerHandlers("vnc", vncCLIHandlers)
	registerHandlers("plumb", plumbCLIHandlers)

	cmdChannel = make(chan commands)
	go cmdProcessor()
}

func cmdProcessor() {
	for cmd := range cmdChannel {
		go func(cmd commands) {
			defer close(cmd.out)

			for _, c := range cmd.in {
				forward(minicli.ProcessCommand(c), cmd.out)
			}
		}(cmd)
	}
}

// registerHandlers registers all the provided handlers with minicli, panicking
// if any of the handlers fail to register.
func registerHandlers(name string, handlers []minicli.Handler) {
	for i := range handlers {
		if err := minicli.Register(&handlers[i]); err != nil {
			log.Fatal("invalid handler, %s:%d -- %v", name, i, err)
		}
	}
}

// wrapSimpleCLI wraps handlers that return a single response. This greatly
// reduces boilerplate code with minicli handlers.
func wrapSimpleCLI(fn wrappedCLIFunc) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		ns := GetNamespace()

		resp := &minicli.Response{Host: hostname}
		if err := fn(ns, c, resp); err != nil {
			resp.Error = err.Error()
		}

		respChan <- minicli.Responses{resp}
	}
}

// errResp creates a minicli.Responses from a single error.
func errResp(err error) minicli.Responses {
	resp := &minicli.Response{
		Host: hostname,
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return minicli.Responses{resp}
}

// wrapBroadcastCLI is a namespace-aware wrapper for VM commands that
// broadcasts the command to all hosts in the namespace and collects all the
// responses together.
func wrapBroadcastCLI(fn wrappedCLIFunc) minicli.CLIFunc {
	// for the `local` behavior
	localFunc := wrapSimpleCLI(fn)

	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		ns := GetNamespace()

		// Wrapped commands have two behaviors:
		//   `fan out` -- send the command to all hosts in the active namespace
		//   `local`   -- invoke the underlying handler
		// We use the source field to track whether we have already performed
		// the `fan out` phase for this command. By default, the source is the
		// empty string so the source will not match the active namespace and
		// we will perform the `fan out` phase. We set the source to the active
		// namespace so that when we send the command via mesh, the source will
		// be propagated and they will execute the `local` behavior.
		if c.Source != "" {
			localFunc(c, respChan)
			return
		}

		res := minicli.Responses{}

		for resps := range runCommands(namespaceCommands(ns, c)...) {
			// TODO: we are flattening commands that return multiple responses
			// by doing this... should we implement proper buffering? Only a
			// problem if commands that return multiple responses are wrapped
			// by this (which *currently* is not the case).
			for _, resp := range resps {
				res = append(res, resp)
			}
		}

		respChan <- res
	}
}

// wrapVMTargetCLI is a namespace-aware wrapper for VM commands that target one
// or more VMs. This is used by commands like `vm start` and `vm kill`.
func wrapVMTargetCLI(fn wrappedCLIFunc) minicli.CLIFunc {
	// for the `local` behavior
	localFunc := wrapSimpleCLI(fn)

	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		ns := GetNamespace()

		// See note in wrapBroadcastCLI.
		if c.Source != "" {
			localFunc(c, respChan)
			return
		}

		res := minicli.Responses{}
		var ok bool

		var notFound string

		for resps := range runCommands(namespaceCommands(ns, c)...) {
			for _, resp := range resps {
				ok = ok || (resp.Error == "")

				if isVMNotFound(resp.Error) {
					notFound = resp.Error
				} else {
					// Record successes and unexpected errors
					res = append(res, resp)
				}
			}
		}

		if !ok && len(res) == 0 {
			// Presumably, we weren't able to find the VM
			respChan <- errResp(errors.New(notFound))
			return
		}

		respChan <- res
	}
}

func wrapSuggest(fn wrappedSuggestFunc) minicli.SuggestFunc {
	return func(raw, val, prefix string) []string {
		if attached != nil {
			return attached.Suggest(raw)
		}

		ns := GetNamespace()

		return fn(ns, val, prefix)
	}
}

func wrapVMSuggest(mask VMState, wild bool) minicli.SuggestFunc {
	return wrapSuggest(func(ns *Namespace, val, prefix string) []string {
		// only make suggestions for VM field
		if val != "vm" {
			return nil
		}

		return cliVMSuggest(ns, prefix, mask, wild)
	})
}

// wrapHostnameSuggest creates a completion function, wrapping
// cliHostnameSuggest.
func wrapHostnameSuggest(local, direct, wild bool) minicli.SuggestFunc {
	return wrapSuggest(func(ns *Namespace, val, prefix string) []string {
		// somewhat hacky... currently only two names for placeholders and
		// unlikely to be too many more.
		if val != "hostname" && val != "value" {
			return nil
		}

		return cliHostnameSuggest(prefix, local, direct, wild)
	})
}

// envCompleter completes environment variables
func envCompleter(s string) []string {
	// handle that begin with a '$' and complete based on the
	// available env variables
	if !strings.HasPrefix(s, "$") {
		return nil
	}

	prefix := strings.TrimPrefix(s, "$")

	var res []string

	for _, env := range os.Environ() {
		k := strings.SplitN(env, "=", 2)[0]
		if strings.HasPrefix(k, prefix) {
			res = append(res, "$"+k)
		}
	}

	return res
}

// fileCompleter
func fileCompleter(path string) []string {
	var res []string

	var dir, prefix string

	if strings.HasSuffix(path, string(os.PathSeparator)) {
		dir = path
		prefix = ""
	} else {
		dir = filepath.Dir(path)
		prefix = filepath.Base(path)
	}

	files, _ := ioutil.ReadDir(dir)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), prefix) {
			name := filepath.Join(dir, f.Name())

			if f.IsDir() {
				name += string(os.PathSeparator)
			}

			res = append(res, name)
		}
	}

	return res
}

func cliCompleter(line string) []string {
	prep := func(s []string) []string {
		if len(s) == 0 {
			return s
		}

		sort.Strings(s)

		// remove the last term from the line
		line := strings.TrimRightFunc(line, func(r rune) bool {
			return !unicode.IsSpace(r)
		})

		// create new result that is line + suggestion + whitespace
		r := make([]string, len(s))
		for i := range s {
			r[i] = line + s[i]
			if !strings.HasSuffix(s[i], string(os.PathSeparator)) {
				r[i] += " "
			}
		}

		return r
	}

	// completing commands has the highest priority
	suggest := minicli.Suggest(line)
	if len(suggest) > 0 {
		return prep(suggest)
	}

	// completing partial word
	if !strings.HasSuffix(line, " ") {
		f := strings.Fields(line)

		if len(f) > 0 {
			last := f[len(f)-1]

			suggest = append(suggest, envCompleter(last)...)
			suggest = append(suggest, iomCompleter(last)...)
			suggest = append(suggest, fileCompleter(last)...)
		}

		return prep(suggest)
	}

	// last resort, complete files from current directory
	if len(suggest) == 0 {
		suggest = append(suggest, fileCompleter(*f_iomBase)...)
	}

	return prep(suggest)
}

// forward receives minicli.Responses from in and forwards them to out.
func forward(in <-chan minicli.Responses, out chan<- minicli.Responses) {
	for v := range in {
		out <- v
	}
}

// consume reads all responses, returning the first error it encounters. Will
// always drain the channel before returning.
func consume(in <-chan minicli.Responses) error {
	var err error

	for resps := range in {
		for _, resp := range resps {
			if resp.Error != "" && err == nil {
				err = errors.New(resp.Error)
			}
		}
	}

	return err
}

// runCommands wraps minicli.ProcessCommand for multiple commands, combining
// their outputs into a single channel.
func runCommands(cmd ...*minicli.Command) <-chan minicli.Responses {
	c := commands{in: cmd, out: make(chan minicli.Responses)}

	cmdChannel <- c
	return c.out
}

// namespaceCommands creates commands to run the given command on all hosts in
// the namespace including the special case where localhost is included in the
// list. All commands will be prefixed with "namespace <name>", have their
// source set to the namespace name, and be record false.
func namespaceCommands(ns *Namespace, cmd *minicli.Command) []*minicli.Command {
	var cmds = []*minicli.Command{}

	var peers []string

	for host := range ns.Hosts {
		if host == hostname {
			// Create a deep copy of the command by recompiling it
			cmd2 := minicli.MustCompile(cmd.Original)
			cmds = append(cmds, cmd2)
		} else {
			// Quote the hostname in case there are spaces
			peers = append(peers, strconv.Quote(host))
		}
	}

	if len(peers) > 0 {
		targets := strings.Join(peers, ",")

		// use `%q` to quote the namespace name in case there are spaces,
		// targets and original command should be fine as-is
		cmd2 := minicli.MustCompilef("mesh send %v namespace %q %v", targets, ns.Name, cmd.Original)
		cmds = append(cmds, cmd2)
	}

	for _, cmd2 := range cmds {
		cmd2.SetSource(ns.Name)
		cmd2.SetRecord(false)
		cmd2.SetPreprocess(cmd.Preprocess)
	}

	return cmds
}

// local command line interface, wrapping readline
func cliLocal(input *liner.State) {
	input.SetCtrlCAborts(true)
	input.SetTabCompletionStyle(liner.TabPrints)
	input.SetCompleter(cliCompleter)

	for {
		ns := GetNamespace()

		prompt := fmt.Sprintf("minimega[%v]$ ", ns.Name)

		line, err := input.Prompt(prompt)
		if err == liner.ErrPromptAborted {
			continue
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)

		log.Debug("got line from stdin: `%v`", line)

		// skip blank lines
		if line == "" {
			continue
		}

		input.AppendHistory(line)

		// expand aliases
		line = minicli.ExpandAliases(line)

		cmd, err := minicli.Compile(line)
		if err != nil {
			log.Error("%v", err)
			//fmt.Printf("closest match: TODO\n")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		// HAX: Don't record the read command
		if hasCommand(cmd, "read") {
			cmd.SetRecord(false)
		}

		// The namespace changed between when we prompted the user (and could
		// still change before we actually run the command).
		if ns != GetNamespace() {
			// TODO: should we abort the command?
			log.Warn("namespace changed between prompt and execution")
		}

		for resp := range runCommands(cmd) {
			// print the responses
			minipager.DefaultPager.Page(resp.String())

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}

// cliPreprocess performs expansion on a single string and returns the update
// string or an error.
func cliPreprocess(v string) (string, error) {
	// expand any ${var} or $var env variables
	v = os.ExpandEnv(v)

	if u, err := url.Parse(v); err == nil {
		switch u.Scheme {
		case "file":
			log.Debug("file preprocessor")
			return iomHelper(u.Opaque, "")
		case "http", "https":
			log.Debug("http/s preprocessor")

			// Check if we've already downloaded the file
			v2, err := iomHelper(u.Path, "")
			if err == nil {
				return v2, err
			}

			if err.Error() == "file not found" {
				log.Info("attempting to download %v", u)

				// Try to download the file, save to files
				dst := filepath.Join(*f_iomBase, u.Path)
				if err := wget(v, dst); err != nil {
					return "", err
				}

				return dst, nil
			}

			return "", err
		case "tar":
			log.Debug("tar preprocessor")

			path := u.Opaque

			if !filepath.IsAbs(u.Path) {
				// not absolute -- try to fetch via meshage
				v2, err := iomHelper(u.Opaque, "")
				if err != nil {
					return v, err
				}
				path = v2
			}

			// check to see how many things are in the top-level directory
			out, err := processWrapper("tar", "--exclude=*/*", "-tf", path)
			if err != nil {
				return v, err
			}

			if strings.Count(out, "\n") != 1 {
				return v, errors.New("unable to handle tar without a single top-level directory")
			}

			// remove trailing "\n"
			out = out[:len(out)-1]

			// check to see if we already extracted this tar
			dst := filepath.Join(filepath.Dir(path), out)
			if _, err := os.Stat(dst); err == nil {
				return dst, nil
			}

			log.Debug("untar to %v", dst)

			// do the extraction
			_, err = processWrapper("tar", "-C", filepath.Dir(path), "-xf", path)
			if err != nil {
				return v, err
			}

			return dst, nil
		}
	}

	return v, nil
}

// cliPreprocessor allows modifying commands post-compile but pre-process.
// Current preprocessors are: "file:", "http://", and "https://".
func cliPreprocessor(c *minicli.Command) error {
	for k, v := range c.StringArgs {
		v2, err := cliPreprocess(v)
		if err != nil {
			return err
		}

		if v != v2 {
			log.Info("cliPreprocess: [%v] %v -> %v", k, v, v2)
		}
		c.StringArgs[k] = v2
	}

	for k := range c.ListArgs {
		for k2, v := range c.ListArgs[k] {
			v2, err := cliPreprocess(v)
			if err != nil {
				return err
			}

			if v != v2 {
				log.Info("cliPreprocessor: [%v][%v] %v -> %v", k, k2, v, v2)
			}
			c.ListArgs[k][k2] = v2
		}
	}

	return nil
}
