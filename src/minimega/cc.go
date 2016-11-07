// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"path/filepath"
	"ron"
)

const (
	CC_SERIAL_PERIOD = 5
)

var (
	ccNode *ron.Server
	ccPort int
)

func ccMapPrefix(id int) {
	if ccPrefix != "" {
		ccPrefixMap[id] = ccPrefix
		log.Debug("prefix map %v: %v", id, ccPrefix)
	}
}

func ccUnmapPrefix(id int) {
	if prefix, ok := ccPrefixMap[id]; ok {
		delete(ccPrefixMap, id)
		log.Debug("prefix unmap %v: %v", id, prefix)
	}
}

func ccPrefixIDs(prefix string) []int {
	var ret []int
	for k, v := range ccPrefixMap {
		if v == prefix {
			ret = append(ret, k)
		}
	}
	return ret
}

func ccStart() {
	var err error
	ccNode, err = ron.NewServer(*f_ccPort, *f_iomBase)
	if err != nil {
		log.Fatalln(fmt.Errorf("creating cc node %v", err))
	}

	log.Debug("created ron node at %v %v", ccPort, *f_base)
}

func ccClear(what string) (err error) {
	log.Debug("cc clear %v", what)

	namespace := GetNamespaceName()

	switch what {
	case "filter":
		ccFilter = nil
	case "commands":
		if namespace == "" {
			ccNode.ResetCommands()
			ccPrefixMap = make(map[int]string)
			return
		}

		errs := errSlice{}
		for _, v := range ccNode.GetCommands() {
			// only delete commands for the active namespace
			if !ccMatchNamespace(v) {
				continue
			}

			err := ccNode.DeleteCommand(v.ID)
			if err != nil {
				err := fmt.Errorf("cc delete command %v : %v", v.ID, err)
				errs = append(errs, err)
			}
			ccUnmapPrefix(v.ID)
		}
		return errs
	case "responses": // delete everything in miniccc_responses
		base := filepath.Join(*f_iomBase, ron.RESPONSE_PATH)

		// no active namespace => delete everything
		if namespace == "" {
			return os.RemoveAll(base)
		}

		walker := func(path string, info os.FileInfo, err error) error {
			// don't do anything if there was an error or it's a directory that
			// doesn't look like a UUID.
			if err != nil || !info.IsDir() || !isUUID(info.Name()) {
				return err
			}

			if vm := vms.FindVM(info.Name()); vm == nil {
				log.Debug("skipping VM: %v", info.Name())
			} else if err := os.RemoveAll(path); err != nil {
				return err
			}

			return filepath.SkipDir
		}

		return filepath.Walk(base, walker)
	case "prefix":
		ccPrefix = ""
	}

	return
}

func ccHasClient(c string) bool {
	return ccNode != nil && ccNode.HasClient(c)
}

// ccGetFilter returns a filter for cc clients, adding the implicit namespace
// filter, if a namespace is active.
func ccGetFilter() *ron.Filter {
	filter := ron.Filter{}
	if ccFilter != nil {
		filter = *ccFilter
	}

	filter.Namespace = GetNamespaceName()
	return &filter
}

// ccMatchNamespace tests whether a command is relavant to the active
// namespace.
func ccMatchNamespace(c *ron.Command) bool {
	namespace := GetNamespaceName()

	return namespace == "" || c.Filter == nil || c.Filter.Namespace == namespace
}
