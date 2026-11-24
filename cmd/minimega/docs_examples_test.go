// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// docsDir is the documentation source, relative to this package.
const docsDir = "../../doc/content"

// docsSkip lists the directories of docsDir that hold frozen or generated
// pages. Their examples are for old releases.
var docsSkip = map[string]bool{
	"assets":    true,
	"legacy":    true,
	"reference": true,
	"releases":  true,
}

// docsPrompt matches the prompt of a command line in a minimega code block,
// for example "minimega$ " and "minimega[sandwich]$ ".
var docsPrompt = regexp.MustCompile(`^minimega(?:\[[^\]]*\])?\$ ?`)

// docsFiles returns the files below docsDir with the extension ext.
func docsFiles(t *testing.T, ext string) []string {
	t.Helper()

	// a build that copies only the source has no documentation
	if _, err := os.Stat(docsDir); err != nil {
		t.Skipf("no documentation: %v", err)
	}

	var files []string

	err := filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel, _ := filepath.Rel(docsDir, path); docsSkip[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ext {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no %v files in %v", ext, docsDir)
	}

	return files
}

// docsLines returns the lines of a file.
func docsLines(t *testing.T, path string) []string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var lines []string

	s := bufio.NewScanner(f)
	s.Buffer(nil, 1<<20)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}

	return lines
}

// TestDocsScripts checks that each line of each example script in the
// documentation is a valid command, as "read <file> check" does.
func TestDocsScripts(t *testing.T) {
	for _, path := range docsFiles(t, ".mm") {
		for i, line := range docsLines(t, path) {
			if _, err := minicli.Compile(line); err != nil {
				t.Errorf("%v:%d: %v", path, i+1, err)
			}
		}
	}
}

// TestDocsCodeBlocks checks the commands in the minimega code blocks of the
// documentation pages. In a block that has prompts, the lines with a prompt
// are commands and the other lines are output, so the first line must have a
// prompt. A block with no prompt holds only commands. A syntax summary with
// placeholders is not a command: mark its block as text.
func TestDocsCodeBlocks(t *testing.T) {
	var commands int

	for _, path := range docsFiles(t, ".md") {
		lines := docsLines(t, path)

		for i := 0; i < len(lines); i++ {
			if !strings.HasPrefix(strings.TrimSpace(lines[i]), "```minimega") {
				continue
			}

			// find the end of the block and whether it has prompts
			start := i + 1
			end := start
			var prompts bool
			for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "```") {
				if docsPrompt.MatchString(strings.TrimSpace(lines[end])) {
					prompts = true
				}
				end++
			}

			var prompted bool

			for n := start; n < end; n++ {
				line := strings.TrimSpace(lines[n])

				// a block that includes a script holds no commands of its own
				if strings.HasPrefix(line, "--8<--") {
					continue
				}

				if prompts {
					loc := docsPrompt.FindStringIndex(line)
					if loc == nil {
						// output follows a command. A line before the first
						// prompt is a command that the test cannot see.
						if !prompted && line != "" {
							t.Errorf("%v:%d: no prompt on a line before the first prompt of the block", path, n+1)
						}
						continue
					}
					prompted = true
					line = line[loc[1]:]
				}

				commands++

				if _, err := minicli.Compile(line); err != nil {
					t.Errorf("%v:%d: %v", path, n+1, err)
				}
			}

			i = end
		}
	}

	if commands == 0 {
		t.Fatal("found no commands in the minimega code blocks")
	}

	t.Logf("checked %d commands", commands)
}
