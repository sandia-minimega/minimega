// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"reflect"
	"strconv"
	"strings"
)

type VMConfigFns struct {
	Update   func(interface{}, *minicli.Command) error
	Clear    func(interface{})
	Print    func(interface{}) string
	PrintCLI func(interface{}) string // If not specified, Print is used
}

var vmConfigFns = map[string]VMConfigFns{
	"memory": genericVMConfig("memory", VM_MEMORY_DEFAULT),
	"vcpus":  genericVMConfig("vcpus", "1"),
	"net": {
		Update: func(v interface{}, c *minicli.Command) error {
			vm := v.(*VMConfig)
			for _, spec := range c.ListArgs["netspec"] {
				net, err := processVMNet(spec)
				if err != nil {
					return err
				}
				vm.Networks = append(vm.Networks, net)
			}
			return nil
		},
		Clear: func(v interface{}) {
			v.(*VMConfig).Networks = []NetConfig{}
		},
		Print: func(v interface{}) string {
			return v.(*VMConfig).NetworkString()
		},
		PrintCLI: func(v interface{}) string {
			vm := v.(*VMConfig)
			if len(vm.Networks) == 0 {
				return ""
			}

			nics := []string{}
			for _, net := range vm.Networks {
				nic := fmt.Sprintf("%v,%v,%v,%v", net.Bridge, net.VLAN, net.MAC, net.Driver)
				nics = append(nics, nic)
			}
			return "vm config net " + strings.Join(nics, " ")
		},
	},
}

// TODO: This has become a mess... there must be a better way. Perhaps we can
// add an Update, UpdateBool, ... method to the vmInfo struct and then have the
// logic in there to handle the different config types.
var kvmConfigFns = map[string]VMConfigFns{
	"cdrom":       genericVMConfig("cdrom", ""),
	"disk":        genericVMConfig("disk", ""),
	"initrd":      genericVMConfig("initrd", ""),
	"kernel":      genericVMConfig("kernel", ""),
	"migrate":     genericVMConfig("migrate", ""),
	"qemu-append": genericVMConfig("qemu-append", ""),
	"snapshot":    genericVMConfig("snapshot", "true"),
	"uuid":        genericVMConfig("uuid", ""),

	"append": {
		Update: func(v interface{}, c *minicli.Command) error {
			v.(*KVMConfig).Append = strings.Join(c.ListArgs["args"], " ")
			return nil
		},
		Clear: func(v interface{}) { v.(*KVMConfig).Append = "" },
		Print: func(v interface{}) string { return v.(*KVMConfig).Append },
	},
	"qemu": {
		Update: func(_ interface{}, c *minicli.Command) error {
			customExternalProcesses["qemu"] = c.StringArgs["path"]
			return nil
		},
		Clear: func(_ interface{}) { delete(customExternalProcesses, "qemu") },
		Print: func(_ interface{}) string { return process("qemu") },
	},
	"qemu-override": {
		Update: func(_ interface{}, c *minicli.Command) error {
			if c.StringArgs["match"] != "" {
				return addVMQemuOverride(c.StringArgs["match"], c.StringArgs["replacement"])
			} else if c.StringArgs["id"] != "" {
				return delVMQemuOverride(c.StringArgs["id"])
			}

			log.Fatalln("someone goofed the qemu-override patterns")
			return nil
		},
		Clear: func(_ interface{}) { QemuOverrides = make(map[int]*qemuOverride) },
		Print: func(_ interface{}) string {
			return qemuOverrideString()
		},
		PrintCLI: func(_ interface{}) string {
			overrides := []string{}
			for _, q := range QemuOverrides {
				override := fmt.Sprintf("vm config qemu-override add %s %s", q.match, q.repl)
				overrides = append(overrides, override)
			}
			return strings.Join(overrides, "\n")
		},
	},
}

// findField uses reflection to find the appropriate field (by tag and name) in
// the provided struct.
func findField(tag, name string, v interface{}) interface{} {
	fVal := reflect.ValueOf(v).Elem()
	fType := reflect.TypeOf(v).Elem()

	// Loop over all the fields and extract the mm tag value. Return a pointer
	// to the value.
	for i := 0; i < fType.NumField(); i++ {
		log.Info("val: `%v`", fType.Field(i).Tag.Get(tag))
		if fType.Field(i).Tag.Get(tag) == name {
			return fVal.Field(i).Addr().Interface()
		}
	}

	return nil
}

// mustFindField is like findField except that it will cause a Fatal error if
// the field is nil.
func mustFindField(tag, name string, v interface{}) interface{} {
	f := findField(tag, name, v)
	if f == nil {
		log.Fatal("invalid field: `%s`", name)
	}

	return f
}
func genericVMConfig(name string, defaultVal string) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			switch f := mustFindField("mm", name, vm).(type) {
			case *string:
				// Update the value, have to use range since we don't know the key
				for _, v := range c.StringArgs {
					*f = v
				}
			case *bool:
				if c.BoolArgs["true"] || c.BoolArgs["false"] {
					*f = c.BoolArgs["true"]
				} else {
					log.Fatalln("someone goofed on the patterns for `%s`, should be true/false", name)
				}
			case *[]string:
				// Update the value, have to use range since we don't know the key
				for _, v := range c.ListArgs {
					*f = append(*f, v...)
				}
			default:
				log.Fatalln("unknown generic field type: `%s`", name)
			}
			return nil
		},
		Clear: func(vm interface{}) {
			switch f := mustFindField("mm", name, vm).(type) {
			case *string:
				*f = defaultVal
			case *bool:
				var err error
				*f, err = strconv.ParseBool(defaultVal)
				if err != nil {
					log.Fatalln("default value for `%s`, should be true/false", name)
				}
			case *[]string:
				// Ignore defaultVal param...
				*f = []string{}
			default:
				log.Fatalln("unknown generic field type: `%s`", name)
			}
		},
		Print: func(vm interface{}) string {
			switch f := mustFindField("mm", name, vm).(type) {
			case *string:
				return *f
			case *bool:
				return fmt.Sprintf("%v", *f)
			case *[]string:
				return fmt.Sprintf("%v", *f)
			default:
				log.Fatalln("unknown generic field type: `%s`", name)
			}

			return ""
		},
		PrintCLI: func(v interface{}) string {
			switch f := mustFindField("mm", name, v).(type) {
			case *string:
				if *f != "" {
					return fmt.Sprintf("vm config %s %v", *f)
				}
			case *bool:
				return fmt.Sprintf("vm config %s %v", *f)
			case *[]string:
				if len(*f) > 0 {
					return fmt.Sprintf("vm config %s %s", strings.Join(*f, " "))
				}
			default:
				log.Fatalln("unknown generic field type: `%s`", name)
			}

			return ""
		},
	}
}
