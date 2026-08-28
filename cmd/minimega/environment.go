// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"unicode/utf8"
)

const environmentFile = "/etc/minimega/minimega.conf"

type environmentFlag struct {
	environment string
	flag        string
}

var baseEnvironmentFlag = environmentFlag{"MM_BASE", "base"}

var environmentFlags = []environmentFlag{
	baseEnvironmentFlag,
	{"MM_DEGREE", "degree"},
	{"MM_MSA", "msa"},
	{"MM_BROADCAST", "broadcast"},
	{"MM_VLANRANGE", "vlanrange"},
	{"MM_PORT", "port"},
	{"MM_FORCE", "force"},
	{"MM_RECOVER", "recover"},
	{"MM_DAEMON", "nostdin"},
	{"MM_CONTEXT", "context"},
	{"MM_FILEPATH", "filepath"},
	{"MM_LOGLEVEL", "level"},
	{"MM_LOGFILE", "logfile"},
	{"MM_CGROUP", "cgroup"},
	{"MM_PANIC", "panic"},
	{"MM_ABSSNAPSHOT", "abssnapshot"},
}

func setEnvironmentFlagDefault(fs *flag.FlagSet, mapping environmentFlag, value string) error {
	f := fs.Lookup(mapping.flag)
	if f == nil {
		return fmt.Errorf("environment variable %s references unknown flag -%s", mapping.environment, mapping.flag)
	}

	if err := f.Value.Set(value); err != nil {
		return fmt.Errorf("invalid value %q for environment variable %s: %w", value, mapping.environment, err)
	}
	f.DefValue = f.Value.String()
	return nil
}

// setEnvironmentFlagDefaults applies mapped environment values unless their flags were explicitly set.
func setEnvironmentFlagDefaults(fs *flag.FlagSet, mappings []environmentFlag, lookupEnv func(string) (string, bool)) error {
	commandLine := make(map[string]struct{})
	fs.Visit(func(f *flag.Flag) {
		commandLine[f.Name] = struct{}{}
	})

	for _, mapping := range mappings {
		if _, ok := commandLine[mapping.flag]; ok {
			continue
		}

		value, ok := lookupEnv(mapping.environment)
		if !ok {
			continue
		}

		if err := setEnvironmentFlagDefault(fs, mapping, value); err != nil {
			return err
		}
	}

	return nil
}

type environmentFileState int

const (
	environmentPreKey environmentFileState = iota
	environmentKey
	environmentPreValue
	environmentValue
	environmentValueEscape
	environmentSingleQuoteValue
	environmentDoubleQuoteValue
	environmentDoubleQuoteValueEscape
	environmentComment
	environmentCommentEscape
)

// readBaseEnvironmentFile reads MM_BASE from a systemd-compatible environment file.
func readBaseEnvironmentFile(path string) (string, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}

	base, found, err := parseBaseEnvironmentFile(contents)
	if err != nil {
		return "", false, fmt.Errorf("parse %s: %w", path, err)
	}
	return base, found, nil
}

// parseBaseEnvironmentFile parses MM_BASE from systemd EnvironmentFile contents.
func parseBaseEnvironmentFile(contents []byte) (string, bool, error) {
	var base string
	var found bool
	var key, value []byte
	lastKeyWhitespace := -1
	lastValueWhitespace := -1
	state := environmentPreKey

	reset := func() {
		key = nil
		value = nil
		lastKeyWhitespace = -1
		lastValueWhitespace = -1
	}
	store := func(trimValue bool) error {
		if lastKeyWhitespace >= 0 {
			key = key[:lastKeyWhitespace]
		}
		if trimValue && lastValueWhitespace >= 0 {
			value = value[:lastValueWhitespace]
		}
		if !utf8.Valid(key) || !utf8.Valid(value) {
			return fmt.Errorf("invalid UTF-8 in environment assignment")
		}

		if string(key) == "MM_BASE" {
			base = string(value)
			found = true
		}
		reset()
		return nil
	}

	for _, c := range contents {
		switch state {
		case environmentPreKey:
			if isEnvironmentComment(c) {
				state = environmentComment
			} else if !isEnvironmentWhitespace(c) {
				state = environmentKey
				key = append(key, c)
			}
		case environmentKey:
			if isEnvironmentNewline(c) {
				state = environmentPreKey
				reset()
			} else if c == '=' {
				state = environmentPreValue
				lastValueWhitespace = -1
			} else {
				if isEnvironmentWhitespace(c) {
					if lastKeyWhitespace < 0 {
						lastKeyWhitespace = len(key)
					}
				} else {
					lastKeyWhitespace = -1
				}
				key = append(key, c)
			}
		case environmentPreValue:
			if isEnvironmentNewline(c) {
				if err := store(false); err != nil {
					return "", false, err
				}
				state = environmentPreKey
			} else if c == '\'' {
				state = environmentSingleQuoteValue
			} else if c == '"' {
				state = environmentDoubleQuoteValue
			} else if c == '\\' {
				state = environmentValueEscape
			} else if !isEnvironmentWhitespace(c) {
				state = environmentValue
				value = append(value, c)
			}
		case environmentValue:
			if isEnvironmentNewline(c) {
				if err := store(true); err != nil {
					return "", false, err
				}
				state = environmentPreKey
			} else if c == '\\' {
				state = environmentValueEscape
				lastValueWhitespace = -1
			} else {
				if isEnvironmentWhitespace(c) {
					if lastValueWhitespace < 0 {
						lastValueWhitespace = len(value)
					}
				} else {
					lastValueWhitespace = -1
				}
				value = append(value, c)
			}
		case environmentValueEscape:
			state = environmentValue
			if !isEnvironmentNewline(c) {
				value = append(value, c)
			}
		case environmentSingleQuoteValue:
			if c == '\'' {
				state = environmentPreValue
			} else {
				value = append(value, c)
			}
		case environmentDoubleQuoteValue:
			if c == '"' {
				state = environmentPreValue
			} else if c == '\\' {
				state = environmentDoubleQuoteValueEscape
			} else {
				value = append(value, c)
			}
		case environmentDoubleQuoteValueEscape:
			state = environmentDoubleQuoteValue
			if isEnvironmentDoubleQuoteEscape(c) {
				value = append(value, c)
			} else if c != '\n' {
				value = append(value, '\\', c)
			}
		case environmentComment:
			if c == '\\' {
				state = environmentCommentEscape
			} else if isEnvironmentNewline(c) {
				state = environmentPreKey
			}
		case environmentCommentEscape:
			if isEnvironmentNewline(c) {
				state = environmentPreKey
			} else {
				state = environmentComment
			}
		}
	}

	switch state {
	case environmentPreValue,
		environmentValue,
		environmentValueEscape,
		environmentSingleQuoteValue,
		environmentDoubleQuoteValue,
		environmentDoubleQuoteValueEscape:
		if err := store(state == environmentValue); err != nil {
			return "", false, err
		}
	}

	return base, found, nil
}

func isEnvironmentNewline(c byte) bool {
	return c == '\n' || c == '\r'
}

func isEnvironmentWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || isEnvironmentNewline(c)
}

func isEnvironmentComment(c byte) bool {
	return c == '#' || c == ';'
}

func isEnvironmentDoubleQuoteEscape(c byte) bool {
	return c == '"' || c == '\\' || c == '$' || c == '`'
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		set = set || f.Name == name
	})
	return set
}

// applyEnvironmentFlagDefaultsFrom combines config and process environment defaults.
// lookupEnv is injectable so tests can use an isolated environment.
func applyEnvironmentFlagDefaultsFrom(fs *flag.FlagSet, path string, readConfig bool, lookupEnv func(string) (string, bool)) error {
	_, baseSet := lookupEnv("MM_BASE")
	if readConfig && !flagSet(fs, "base") && !baseSet {
		base, found, err := readBaseEnvironmentFile(path)
		if errors.Is(err, os.ErrPermission) {
			fmt.Fprintf(os.Stderr, "warning: unable to read minimega environment: %v\n", err)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load minimega environment: %w", err)
		}
		if err == nil && found {
			if err := setEnvironmentFlagDefault(fs, baseEnvironmentFlag, base); err != nil {
				return err
			}
		}
	}

	return setEnvironmentFlagDefaults(fs, environmentFlags, lookupEnv)
}

func applyEnvironmentFlagDefaults(readConfig bool) error {
	return applyEnvironmentFlagDefaultsFrom(flag.CommandLine, environmentFile, readConfig, os.LookupEnv)
}
