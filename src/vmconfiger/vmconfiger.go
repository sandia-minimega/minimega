package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	log "minilog"
	"os"
	"strings"
)

type Generator struct {
	Types []string
}

var (
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_types    = flag.String("type", "", "comma-separated list of type names")
)

func usage() {
	fmt.Printf("USAGE: %v [OPTIONS] [DIR]\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	logSetup()

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}

	if *f_types == "" {
		flag.Usage()
		os.Exit(1)
	}

	g := Generator{
		Types: strings.Split(*f_types, ","),
	}

	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}

	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		log.Fatalln(err)
	}

	fs := token.NewFileSet()

	pkgs, err := parser.ParseDir(fs, pkg.Dir, nil, 0)
	if err != nil {
		log.Fatalln(err)
	}

	if _, ok := pkgs[pkg.Name]; !ok {
		log.Fatal("parsing package did not include %v", pkg.Name)
	}

	if err := checkTypes(pkg.Dir, fs, pkgs[pkg.Name]); err != nil {
		log.Fatalln(err)
	}

	for k, file := range pkgs[pkg.Name].Files {
		log.Info("inspecting %v", k)
		ast.Inspect(file, g.printDecl)
	}
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

func (g *Generator) printDecl(node ast.Node) bool {
	decl, ok := node.(*ast.GenDecl)
	if !ok || decl.Tok != token.TYPE {
		return true
	}

	for _, spec := range decl.Specs {
		tspec := spec.(*ast.TypeSpec)

		var match bool
		for _, v := range g.Types {
			if v == tspec.Name.Name {
				match = true
			}
		}
		if !match {
			log.Info("skipping %v", tspec.Name)
			return true
		}

		strct, ok := tspec.Type.(*ast.StructType)
		if !ok {
			log.Fatal("%v is not a struct", tspec.Name)
		}

		log.Info("found %v", tspec.Name)

		for _, field := range strct.Fields.List {
			log.Info("%#v", field.Names[0].Name)
			switch field := field.Type.(type) {
			case *ast.Ident:
				log.Info("%#v", field)

			case *ast.ArrayType:
				log.Info("%#v", field)
			case *ast.MapType:
				log.Info("%#v", field)
			default:
				log.Info("%#v", field)
			}
		}
	}

	return false
}
