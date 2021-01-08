// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/token"
	"go/types"
	"io/ioutil"
	log "minilog"
	"os"
	"strings"
	"unicode"
)

var (
	f_types = flag.String("type", "", "comma-separated list of type names")
)

func usage() {
	fmt.Printf("USAGE: %v [OPTIONS] [DIR]\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}

	if *f_types == "" {
		flag.Usage()
		os.Exit(1)
	}

	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}

	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		log.Fatalln(err)
	}

	g := Generator{
		types: strings.Split(*f_types, ","),
		pkg:   pkg,
	}

	if err := g.Run(); err != nil {
		log.Fatalln(err)
	}

	ioutil.WriteFile("vmconfiger_cli.go", g.Format(), 0644)
}

func checkTypes(dir string, fs *token.FileSet, pkg *ast.Package) error {
	astFiles := []*ast.File{}
	for _, v := range pkg.Files {
		astFiles = append(astFiles, v)
	}

	config := types.Config{
		Importer:         importer.Default(),
		IgnoreFuncBodies: true,
		FakeImportC:      true,
	}
	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
	}

	_, err := config.Check(dir, fs, astFiles, info)
	if err != nil {
		return fmt.Errorf("checking package: %v", err)
	}

	return nil
}

func camelToHyphenated(s string) string {
	var prev int
	var out []rune

	for i, r := range s {
		if unicode.IsUpper(r) {
			if i-prev > 1 {
				out = append(out, '-')
			}
			prev = i
		}

		out = append(out, unicode.ToLower(r))
	}

	return string(out)
}
