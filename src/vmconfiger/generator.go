// Copyright 2016-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	log "minilog"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

const Default = "Default: "

type Field struct {
	Field      string // name of the field in the struct
	ConfigName string // name of the field in the CLI
	Type       string // field type
	Doc        string // field documentation
	Default    string // default value, parsed from doc
	Validate   string // name of function to validate argument
	Suggest    string // name of function to use for Suggest

	Path   bool // if filepath should be checked
	Signed bool // for int64 vs uint64
}

type Fields []Field

func (f Fields) Len() int           { return len(f) }
func (f Fields) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f Fields) Less(i, j int) bool { return f[i].Field < f[j].Field }

type Generator struct {
	types []string
	pkg   *build.Package
	buf   bytes.Buffer

	template *template.Template

	// mapping from type to list of fields
	fields map[string][]Field
}

func (g *Generator) Printf(format string, args ...interface{}) {
	fmt.Fprintf(&g.buf, format, args...)
}

func (g *Generator) Execute(name string, data interface{}) {
	if err := g.template.ExecuteTemplate(&g.buf, name, data); err != nil {
		log.Error("executing %v: %v", name, err)
	}
}

// Format returns the gofmt-ed contents of the Generator's buffer.
func (g *Generator) Format() []byte {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		// Should never happen, but can arise when developing this code.
		// The user can compile the output to see the error.
		log.Error("invalid Go generated: %s", err)
		log.Error("compile the package to analyze the error")
		return g.buf.Bytes()
	}

	return src
}

func (g *Generator) Run() error {
	fs := token.NewFileSet()

	t := template.Must(template.New("header").Parse(headerTemplate))
	template.Must(t.New("funcs").Parse(funcsTemplate))
	template.Must(t.New("clear").Parse(clearTemplate))
	// template name must match the type it intends to generate
	template.Must(t.New("int64").Parse(numTemplate))
	template.Must(t.New("uint64").Parse(numTemplate))
	template.Must(t.New("bool").Parse(boolTemplate))
	template.Must(t.New("string").Parse(stringTemplate))
	template.Must(t.New("slice").Parse(sliceTemplate))
	template.Must(t.New("map").Parse(mapTemplate))

	g.template = t

	filter := func(f os.FileInfo) bool {
		// we don't want to parse what we generate
		return f.Name() != "vmconfiger_cli.go"
	}

	pkgs, err := parser.ParseDir(fs, g.pkg.Dir, filter, parser.ParseComments)
	if err != nil {
		return err
	}

	if _, ok := pkgs[g.pkg.Name]; !ok {
		return fmt.Errorf("parsing package did not include %v", g.pkg.Name)
	}

	if err := checkTypes(g.pkg.Dir, fs, pkgs[g.pkg.Name]); err != nil {
		return err
	}

	// Print the header and package clause.
	g.Execute("header", struct {
		Args, Package string
	}{
		Args:    strings.Join(os.Args[1:], " "),
		Package: g.pkg.Name,
	})

	g.fields = map[string][]Field{}

	// go through files in sorted order to make the output more deterministic
	var files []string
	for f := range pkgs[g.pkg.Name].Files {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		log.Debug("inspecting %v", file)
		ast.Inspect(pkgs[g.pkg.Name].Files[file], g.handleNode)
	}

	if len(g.fields) > 0 {
		var fields Fields
		for _, f := range g.fields {
			fields = append(fields, f...)
		}
		sort.Sort(fields)
		g.Execute("clear", fields)
	}

	g.Printf("}\n")

	if len(g.fields) > 0 {
		g.Execute("funcs", g.fields)
	}

	return nil
}

func (g *Generator) handleNode(node ast.Node) bool {
	decl, ok := node.(*ast.GenDecl)
	if !ok || decl.Tok != token.TYPE {
		return true
	}

	for _, spec := range decl.Specs {
		tspec := spec.(*ast.TypeSpec)

		var match bool
		for _, v := range g.types {
			if v == tspec.Name.Name {
				match = true
			}
		}
		if !match {
			log.Debug("skipping %v", tspec.Name)
			return true
		}

		strct, ok := tspec.Type.(*ast.StructType)
		if !ok {
			log.Fatal("%v is not a struct", tspec.Name)
		}

		log.Info("found %v", tspec.Name)
		strctName := tspec.Name.String()

		for _, field := range strct.Fields.List {
			log.Info("%#v", field)
			name := field.Names[0].Name
			if !ast.IsExported(name) {
				continue
			}
			doc := field.Doc.Text()
			var tag reflect.StructTag
			if field.Tag != nil {
				v, _ := strconv.Unquote(field.Tag.Value)
				tag = reflect.StructTag(v)
			}

			configName := name
			if strings.Contains(name, "Path") {
				// trim both path and paths, should only contain one
				configName = strings.TrimSuffix(configName, "Path")
				configName = strings.TrimSuffix(configName, "Paths")
			}
			configName = camelToHyphenated(configName)

			switch typ := field.Type.(type) {
			case *ast.Ident:
				var zero string
				var unhandled bool
				var signed bool

				switch typ.Name {
				case "string":
					zero = getDefault(doc, `""`)
				case "int64":
					signed = true
					zero = getDefault(doc, `0`)
				case "uint64":
					zero = getDefault(doc, `0`)
				case "bool":
					zero = getDefault(doc, `false`)
				default:
					log.Error("unhandled type: %v", typ)
					unhandled = true
					zero = "nil"
				}

				f := Field{
					Field:      name,
					ConfigName: configName,
					Type:       typ.Name,
					Default:    zero,
					Validate:   tag.Get("validate"),
					Suggest:    tag.Get("suggest"),
					Doc:        doc,
					Signed:     signed,
					Path:       strings.Contains(name, "Path"),
				}

				log.Info("field: %#v", f)

				g.fields[strctName] = append(g.fields[strctName], f)

				if !unhandled {
					g.Execute(typ.Name, f)
				}
			case *ast.ArrayType:
				v, ok := typ.Elt.(*ast.Ident)
				if !ok || v.Name != "string" {
					log.Error("unhandled type: []%v", typ.Elt)
					// always add field, even if we don't generate the handler
					g.fields[strctName] = append(g.fields[strctName], Field{
						Field:      name,
						ConfigName: configName,
						Default:    "nil",
						Validate:   tag.Get("validate"),
						Suggest:    tag.Get("suggest"),
						Doc:        doc,
					})

					continue
				}

				zero := getDefault(doc, "")
				if f := strings.Fields(zero); len(f) > 0 {
					for i := range f {
						f[i] = strings.TrimSuffix(f[i], ",")
					}
					zero = fmt.Sprintf("[]string{%v}", strings.Join(f, ","))
				} else {
					zero = "nil"
				}

				f := Field{
					Field:      name,
					ConfigName: configName,
					Type:       "slice",
					Default:    zero,
					Validate:   tag.Get("validate"),
					Suggest:    tag.Get("suggest"),
					Doc:        doc,
					Path:       strings.Contains(name, "Path"),
				}

				g.fields[strctName] = append(g.fields[strctName], f)

				g.Execute("slice", f)
			case *ast.MapType:
				v, ok := typ.Key.(*ast.Ident)
				v2, ok2 := typ.Value.(*ast.Ident)
				if !ok || v.Name != "string" || !ok2 || v2.Name != "string" {
					log.Error("unhandled type: %v", typ)
					// always add field, even if we don't generate the handler
					g.fields[strctName] = append(g.fields[strctName], Field{
						Field:      name,
						ConfigName: configName,
						Default:    "nil",
						Validate:   tag.Get("validate"),
						Suggest:    tag.Get("suggest"),
						Doc:        doc,
					})

					continue
				}

				zero := "nil"
				if f := strings.Fields(getDefault(doc, "")); len(f) > 0 {
					for i := range f {
						f[i] = strings.TrimSuffix(f[i], ",")
					}
					zero = fmt.Sprintf("map[string]string{%v}", strings.Join(f, ","))
				}

				f := Field{
					Field:      name,
					ConfigName: configName,
					Type:       "map",
					Default:    zero,
					Validate:   tag.Get("validate"),
					Suggest:    tag.Get("suggest"),
					Doc:        doc,
					Path:       strings.Contains(name, "Path"),
				}

				g.fields[strctName] = append(g.fields[strctName], f)

				g.Execute("map", f)
			default:
				log.Error("unhandled type for %v: %v", name, typ)
				// always add field, even if we don't generate the handler
				g.fields[strctName] = append(g.fields[strctName], Field{
					Field:      name,
					ConfigName: configName,
					Doc:        doc,
					Default:    "nil",
				})
			}
		}
	}

	return false
}

// getDefault parses out the "Default: X" in s. If no default is found, d is
// returned instead.
func getDefault(s, d string) string {
	if strings.Contains(s, Default) {
		return strings.TrimSpace(s[strings.Index(s, Default)+len(Default):])
	}

	return d
}
