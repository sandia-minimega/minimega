// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"flag"
	"fmt"
	"go/importer"
	"go/types"
	"io/ioutil"
	"os"
	"strings"
	"unicode"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/tools/go/packages"
)

var (
	f_types = flag.String("type", "", "comma-separated list of type names")
)

func usage() {
	fmt.Printf("USAGE: %v [OPTIONS]\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	log.Init()

	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *f_types == "" {
		flag.Usage()
		os.Exit(1)
	}

	config := &packages.Config{Mode: packages.LoadAllSyntax | packages.LoadFiles}

	pkgs, err := packages.Load(config, "github.com/sandia-minimega/minimega/v2/cmd/minimega")
	if err != nil {
		log.Fatalln(err)
	}

	g := Generator{
		types: strings.Split(*f_types, ","),
		pkgs:  pkgs,
	}

	if err := g.Run(); err != nil {
		log.Fatalln(err)
	}

	ioutil.WriteFile("vmconfiger_cli.go", g.Format(), 0644)
}

func checkTypes(pkg *packages.Package) error {
	config := types.Config{
		Importer:         importer.ForCompiler(pkg.Fset, "source", nil),
		IgnoreFuncBodies: true,
		FakeImportC:      true,
	}

	_, err := config.Check(pkg.PkgPath, pkg.Fset, pkg.Syntax, pkg.TypesInfo)
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
